package fix

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

type importIdentity struct {
	name string
	path string
}

type unusedImportEdit struct {
	start int
	end int
	identities []importIdentity
}

func validateRequiredImportBindings(
	file *source.File,
	requirements []rules.ImportRequirement,
) error {
	if file == nil {
		return fmt.Errorf("required import validation requires source")
	}
	seen := make(map[rules.ImportRequirement]struct{}, len(requirements))
	names := make(map[string]string, len(requirements))
	paths := make(map[string]string, len(requirements))
	for _, requirement := range requirements {
		if err := validateImportRequirement(requirement); err != nil {
			return err
		}
		if _, duplicate := seen[requirement]; duplicate {
			return fmt.Errorf(
				"required import path %q with name %q is duplicated",
				requirement.Path,
				requirement.Name,
			)
		}
		if path, found := names[requirement.Name]; found && path != requirement.Path {
			return fmt.Errorf(
				"required import name %q has incompatible paths",
				requirement.Name,
			)
		}
		if name, found := paths[requirement.Path]; found && name != requirement.Name {
			return fmt.Errorf(
				"required import path %q has incompatible names",
				requirement.Path,
			)
		}
		seen[requirement] = struct{}{}
		names[requirement.Name] = requirement.Path
		paths[requirement.Path] = requirement.Name
	}
	return file.ReadSyntax(
		func(syntax *ast.File) error {
			bindings, paths := importedBindings(syntax)
			declared := declaredPackageNames(syntax)
			for _, requirement := range requirements {
				if path, found := bindings[requirement.Name]; found {
					if path != requirement.Path {
						return fmt.Errorf(
							"required import name %q already binds %q",
							requirement.Name,
							path,
						)
					}
					continue
				}
				if name, found := paths[requirement.Path];
					found && name != requirement.Name {
					return fmt.Errorf(
						"required import path %q already uses name %q",
						requirement.Path,
						name,
					)
				}
				if declared[requirement.Name] {
					return fmt.Errorf(
						"required import name %q conflicts with a source binding",
						requirement.Name,
					)
				}
			}
			return nil
		},
	)
}

func coordinateImports(
	original *source.File,
	edited *source.File,
	requirements []rules.ImportRequirement,
) ([]byte, []ImportChange, error) {
	if err := validateNewRequiredSelectorBindings(original, edited, requirements); err != nil {
		return nil, nil, err
	}
	withImports, additions, err := addRequiredImports(edited, requirements)
	if err != nil {
		return nil, nil, err
	}
	coordinated, err := source.Load(edited.Path(), withImports)
	if err != nil {
		return nil, nil, fmt.Errorf("parse required imports: %w", err)
	}
	required := make(map[importIdentity]bool, len(requirements))
	for _, requirement := range requirements {
		required[importIdentity{name: requirement.Name, path: requirement.Path}] = true
	}
	withoutUnused, removals, err := pruneNewlyUnusedImports(original, coordinated, required)
	if err != nil {
		return nil, nil, err
	}
	changes := append(additions, removals...)
	sortImportChanges(changes)
	return withoutUnused, changes, nil
}

func addRequiredImports(
	edited *source.File,
	requirements []rules.ImportRequirement,
) ([]byte, []ImportChange, error) {
	if edited == nil {
		return nil, nil, fmt.Errorf("required import coordination requires source")
	}
	if len(requirements) == 0 {
		return edited.Bytes(), []ImportChange{}, nil
	}
	for _, requirement := range requirements {
		if err := validateImportRequirement(requirement); err != nil {
			return nil, nil, err
		}
	}

	input := edited.Bytes()
	missing := make([]rules.ImportRequirement, 0, len(requirements))
	var insertion int
	var grouped *ast.GenDecl
	err := edited.ReadSyntaxView(
		func(fileSet *token.FileSet, syntax *ast.File) error {
			bindings, paths := importedBindings(syntax)
			declared := declaredPackageNames(syntax)
			for _, requirement := range requirements {
				if path, found := bindings[requirement.Name]; found {
					if path != requirement.Path {
						return fmt.Errorf(
							"required import name %q already binds %q",
							requirement.Name,
							path,
						)
					}
					continue
				}
				if name, found := paths[requirement.Path]; found {
					return fmt.Errorf(
						"required import path %q already uses name %q",
						requirement.Path,
						name,
					)
				}
				if declared[requirement.Name] {
					return fmt.Errorf(
						"required import name %q conflicts with a source binding",
						requirement.Name,
					)
				}
				missing = append(missing, requirement)
			}
			if len(missing) == 0 {
				return nil
			}
			imports := make([]*ast.GenDecl, 0)
			for _, declaration := range syntax.Decls {
				candidate, ok := declaration.(*ast.GenDecl)
				if ok && candidate.Tok == token.IMPORT {
					imports = append(imports, candidate)
				}
			}
			if len(imports) == 1 &&
				safeGroupedImportInsertion(fileSet, input, syntax, imports[0]) {
				grouped = imports[0]
				insertion = fileSet.PositionFor(grouped.Rparen, false).Offset
				return nil
			}
			if len(imports) != 0 {
				insertion = fullLineEnd(
					fileSet,
					input,
					imports[len(imports) - 1].End(),
				)
				return nil
			}
			insertion = fullLineEnd(fileSet, input, syntax.Name.End())
			return nil
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("read required import bindings: %w", err)
	}
	if len(missing) == 0 {
		return input, []ImportChange{}, nil
	}

	var addition strings.Builder
	if grouped != nil {
		for _, requirement := range missing {
			addition.WriteString("\t")
			addition.WriteString(renderImportRequirement(requirement))
			addition.WriteByte('\n')
		}
	} else {
		for _, requirement := range missing {
			addition.WriteString("\nimport ")
			addition.WriteString(renderImportRequirement(requirement))
			addition.WriteByte('\n')
		}
	}
	result := make([]byte, 0, len(input) + addition.Len())
	result = append(result, input[:insertion]...)
	result = append(result, addition.String()...)
	result = append(result, input[insertion:]...)
	changes := make([]ImportChange, len(missing))
	for index, requirement := range missing {
		changes[index] = ImportChange{
			Action: ImportAdd,
			Path: requirement.Path,
			Name: requirement.Name,
		}
	}
	sortImportChanges(changes)
	return result, changes, nil
}

func importedBindings(syntax *ast.File) (map[string]string, map[string]string) {
	bindings := make(map[string]string)
	paths := make(map[string]string)
	for _, specification := range syntax.Imports {
		path, name, found := rawImportBinding(specification)
		if found {
			paths[path] = name
		}
		identity, eligible := importSpecIdentity(specification)
		if !eligible {
			continue
		}
		bindings[identity.name] = identity.path
	}
	return bindings, paths
}

func rawImportBinding(specification *ast.ImportSpec) (string, string, bool) {
	if specification == nil || specification.Path == nil {
		return "", "", false
	}
	path, err := strconv.Unquote(specification.Path.Value)
	if err != nil || path == "" {
		return "", "", false
	}
	name := pathpkg.Base(path)
	if specification.Name != nil {
		name = specification.Name.Name
	}
	return path, name, true
}

func validateImportRequirement(requirement rules.ImportRequirement) error {
	return requirement.Validate()
}

func declaredPackageNames(syntax *ast.File) map[string]bool {
	result := make(map[string]bool)
	for _, declaration := range syntax.Decls {
		switch value := declaration.(type) {
		case *ast.GenDecl:
			collectDeclaredGenNames(result, value)
		case *ast.FuncDecl:
			if value.Recv == nil {
				collectDeclaredIdent(result, value.Name)
			}
		}
	}
	return result
}

func collectDeclaredGenNames(result map[string]bool, declaration *ast.GenDecl) {
	if declaration == nil || declaration.Tok == token.IMPORT {
		return
	}
	for _, raw := range declaration.Specs {
		switch specification := raw.(type) {
		case *ast.TypeSpec:
			collectDeclaredIdent(result, specification.Name)
		case *ast.ValueSpec:
			for _, name := range specification.Names {
				collectDeclaredIdent(result, name)
			}
		}
	}
}

func collectDeclaredIdent(result map[string]bool, identifier *ast.Ident) {
	if identifier != nil && identifier.Name != "_" {
		result[identifier.Name] = true
	}
}

func validateNewRequiredSelectorBindings(
	original *source.File,
	edited *source.File,
	requirements []rules.ImportRequirement,
) error {
	if len(requirements) == 0 {
		return nil
	}
	if original == nil || edited == nil {
		return fmt.Errorf("required selector validation requires source versions")
	}
	originalCounts, err := localSelectorCounts(original.Path(), original.Bytes())
	if err != nil {
		return err
	}
	editedCounts, err := localSelectorCounts(edited.Path(), edited.Bytes())
	if err != nil {
		return err
	}
	for _, requirement := range requirements {
		for identity, count := range editedCounts {
			if identity.name == requirement.Name && count > originalCounts[identity] {
				return fmt.Errorf(
					"required import name %q resolves to a local source binding",
					requirement.Name,
				)
			}
		}
	}
	return nil
}

type localSelectorIdentity struct {
	name string
	selector string
}

func localSelectorCounts(path string, input []byte) (map[localSelectorIdentity]int, error) {
	fileSet := token.NewFileSet()
	syntax, err := parser.ParseFile(fileSet, path, input, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("resolve required import selectors: %w", err)
	}
	counts := make(map[localSelectorIdentity]int)
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, _ := ast.Unparen(selector.X).(*ast.Ident)
			if identifier != nil && identifier.Obj != nil {
				counts[localSelectorIdentity{
					name: identifier.Name,
					selector: selector.Sel.Name,
				}]++
			}
			return true
		},
	)
	return counts, nil
}

func safeGroupedImportInsertion(
	fileSet *token.FileSet,
	input []byte,
	syntax *ast.File,
	declaration *ast.GenDecl,
) bool {
	if fileSet == nil ||
		syntax == nil ||
		declaration == nil ||
		!declaration.Lparen.IsValid() ||
		!declaration.Rparen.IsValid() {
		return false
	}
	offset := fileSet.PositionFor(declaration.Rparen, false).Offset
	if offset < 0 || offset > len(input) {
		return false
	}
	lineStart := bytes.LastIndexByte(input[:offset], '\n') + 1
	if len(bytes.TrimSpace(input[lineStart:offset])) != 0 {
		return false
	}
	if len(declaration.Specs) == 0 {
		return false
	}
	lastLine := fileSet.
		PositionFor(declaration.Specs[len(declaration.Specs) - 1].End(), false).
		Line
	for _, group := range syntax.Comments {
		if group.Pos() <= declaration.Specs[len(declaration.Specs) - 1].End() ||
			group.End() >= declaration.Rparen {
			continue
		}
		if fileSet.PositionFor(group.Pos(), false).Line > lastLine {
			return false
		}
	}
	return true
}

func fullLineEnd(fileSet *token.FileSet, input []byte, position token.Pos) int {
	offset := fileSet.PositionFor(position, false).Offset
	if offset < 0 || offset > len(input) {
		return len(input)
	}
	for offset < len(input) && input[offset] != '\n' {
		offset++
	}
	if offset < len(input) {
		offset++
	}
	return offset
}

func renderImportRequirement(requirement rules.ImportRequirement) string {
	quoted := strconv.Quote(requirement.Path)
	if requirement.Name == pathpkg.Base(requirement.Path) {
		return quoted
	}
	return requirement.Name + " " + quoted
}

func sortImportChanges(changes []ImportChange) {
	sort.Slice(
		changes,
		func(left, right int) bool {
			if changes[left].Path != changes[right].Path {
				return changes[left].Path < changes[right].Path
			}
			if changes[left].Name != changes[right].Name {
				return changes[left].Name < changes[right].Name
			}
			return changes[left].Action < changes[right].Action
		},
	)
}

func pruneNewlyUnusedImports(
	original *source.File,
	edited *source.File,
	required map[importIdentity]bool,
) ([]byte, []ImportChange, error) {
	if original == nil || edited == nil {
		return nil, nil, fmt.Errorf("import coordination requires source versions")
	}
	originalUses, err := importUses(original)
	if err != nil {
		return nil, nil, err
	}
	input := edited.Bytes()
	edits := make([]unusedImportEdit, 0)
	err = edited.ReadSyntaxView(
		func(fileSet *token.FileSet, syntax *ast.File) error {
			editedUses := selectorNames(syntax)
			for _, declaration := range syntax.Decls {
				imports, ok := declaration.(*ast.GenDecl)
				if !ok || imports.Tok != token.IMPORT {
					continue
				}
				unused := make(map[int]importIdentity)
				for index, raw := range imports.Specs {
					specification, _ := raw.(*ast.ImportSpec)
					identity, eligible := importSpecIdentity(specification)
					if !eligible ||
						required[identity] ||
						!originalUses[identity] ||
						editedUses[identity.name] {
						continue
					}
					unused[index] = identity
				}
				if len(unused) == len(imports.Specs) && len(unused) != 0 {
					edit, ok := removableImportDeclarationRange(
						fileSet,
						input,
						imports,
					)
					if ok {
						edit.identities = make(
							[]importIdentity,
							0,
							len(unused),
						)
						for index := range imports.Specs {
							edit.identities = append(
								edit.identities,
								unused[index],
							)
						}
						edits = append(edits, edit)
					}
					continue
				}
				for index, identity := range unused {
					specification, _ := imports.Specs[index].(*ast.ImportSpec)
					edit, ok := removableImportRange(
						fileSet,
						input,
						imports,
						index,
						specification,
					)
					if ok {
						edit.identities = []importIdentity{identity}
						edits = append(edits, edit)
					}
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("read edited imports: %w", err)
	}
	if len(edits) == 0 {
		return input, []ImportChange{}, nil
	}
	sort.Slice(
		edits,
		func(left, right int) bool {
			return edits[left].start > edits[right].start
		},
	)
	result := bytes.Clone(input)
	changes := make([]ImportChange, 0, len(edits))
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(result) {
			return nil, nil, fmt.Errorf("coordinated import range is invalid")
		}
		result = append(result[:edit.start], result[edit.end:]...)
		for _, identity := range edit.identities {
			changes = append(
				changes,
				ImportChange{
					Action: ImportRemove,
					Path: identity.path,
					Name: identity.name,
				},
			)
		}
	}
	sortImportChanges(changes)
	return result, changes, nil
}

func removableImportDeclarationRange(
	fileSet *token.FileSet,
	input []byte,
	declaration *ast.GenDecl,
) (unusedImportEdit, bool) {
	if fileSet == nil || declaration == nil {
		return unusedImportEdit{}, false
	}
	start := declaration.Pos()
	if declaration.Doc != nil {
		start = declaration.Doc.Pos()
	}
	end := declaration.End()
	return fullPhysicalLines(fileSet, input, start, end)
}

func importUses(file *source.File) (map[importIdentity]bool, error) {
	result := make(map[importIdentity]bool)
	err := file.ReadSyntax(
		func(syntax *ast.File) error {
			uses := selectorNames(syntax)
			for _, specification := range syntax.Imports {
				identity, eligible := importSpecIdentity(specification)
				if eligible && uses[identity.name] {
					result[identity] = true
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("read original imports: %w", err)
	}
	return result, nil
}

func selectorNames(syntax *ast.File) map[string]bool {
	result := make(map[string]bool)
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, _ := ast.Unparen(selector.X).(*ast.Ident)
			if identifier != nil {
				result[identifier.Name] = true
			}
			return true
		},
	)
	return result
}

func importSpecIdentity(specification *ast.ImportSpec) (importIdentity, bool) {
	path, name, found := rawImportBinding(specification)
	if !found || path == "C" {
		return importIdentity{}, false
	}
	if name == "" || name == "_" || name == "." {
		return importIdentity{}, false
	}
	return importIdentity{name: name, path: path}, true
}

func removableImportRange(
	fileSet *token.FileSet,
	input []byte,
	declaration *ast.GenDecl,
	index int,
	specification *ast.ImportSpec,
) (unusedImportEdit, bool) {
	if fileSet == nil || declaration == nil || specification == nil {
		return unusedImportEdit{}, false
	}
	if len(declaration.Specs) == 1 {
		start := declaration.Pos()
		if declaration.Doc != nil {
			start = declaration.Doc.Pos()
		}
		end := declaration.End()
		if specification.Comment != nil {
			end = specification.Comment.End()
		}
		return fullPhysicalLines(fileSet, input, start, end)
	}
	position := fileSet.PositionFor(specification.Pos(), false)
	if position.Offset < 0 || position.Offset >= len(input) {
		return unusedImportEdit{}, false
	}
	line := position.Line
	if index > 0 &&
		fileSet.PositionFor(declaration.Specs[index - 1].End(), false).Line == line {
		return unusedImportEdit{}, false
	}
	if index + 1 < len(declaration.Specs) &&
		fileSet.PositionFor(declaration.Specs[index + 1].Pos(), false).Line == line {
		return unusedImportEdit{}, false
	}
	start := specification.Pos()
	if specification.Doc != nil {
		start = specification.Doc.Pos()
	}
	end := specification.End()
	if specification.Comment != nil {
		end = specification.Comment.End()
	}
	return fullPhysicalLines(fileSet, input, start, end)
}

func fullPhysicalLines(
	fileSet *token.FileSet,
	input []byte,
	startPosition token.Pos,
	endPosition token.Pos,
) (unusedImportEdit, bool) {
	start := fileSet.PositionFor(startPosition, false).Offset
	end := fileSet.PositionFor(endPosition, false).Offset
	if start < 0 || end < start || end > len(input) {
		return unusedImportEdit{}, false
	}
	for start > 0 && input[start - 1] != '\n' {
		start--
	}
	for end < len(input) && input[end] != '\n' {
		end++
	}
	if end < len(input) {
		end++
	}
	return unusedImportEdit{start: start, end: end}, true
}
