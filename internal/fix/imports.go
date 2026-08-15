package fix

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	pathpkg "path"
	"sort"
	"strconv"

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

func pruneNewlyUnusedImports(
	original *source.File,
	edited *source.File,
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
	sort.Slice(
		changes,
		func(left, right int) bool {
			if changes[left].Path != changes[right].Path {
				return changes[left].Path < changes[right].Path
			}
			return changes[left].Name < changes[right].Name
		},
	)
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
	if specification == nil || specification.Path == nil {
		return importIdentity{}, false
	}
	path, err := strconv.Unquote(specification.Path.Value)
	if err != nil || path == "" || path == "C" {
		return importIdentity{}, false
	}
	name := pathpkg.Base(path)
	if specification.Name != nil {
		name = specification.Name.Name
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
