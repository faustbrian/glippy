package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"path/filepath"
	"slices"
	"sort"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"golang.org/x/tools/go/packages"
)

type activeTypesRule struct {
	rule     rules.TypesRule
	metadata rules.Metadata
	severity rules.Severity
}

type typedSyntaxFile struct {
	path   string
	source *source.File
	syntax *ast.File
}

type typedPackageFile struct {
	package_ *packages.Package
	file     typedSyntaxFile
}

// RunTypes executes selected types-tier rules through one shared package AST
// traversal over the exact source versions captured by LoadPackages.
func RunTypes(
	ctx context.Context,
	loaded PackageLoadResult,
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Diagnostic, error) {
	if ctx == nil {
		return nil, fmt.Errorf("types analysis requires a context")
	}
	if registry == nil {
		return nil, fmt.Errorf("types analysis requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if loaded.Requirement < rules.RequireTypes {
		return nil, fmt.Errorf("types analysis requires a typed package load")
	}
	dispatch, err := prepareTypesRules(registry, selection)
	if err != nil {
		return nil, err
	}
	if len(dispatch) == 0 {
		return []rules.Diagnostic{}, nil
	}
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return nil, err
	}
	files, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return nil, err
	}
	diagnostics := make([]rules.Diagnostic, 0)
	for _, work := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pkg, file := work.package_, work.file
		if !anyTypesRuleEligible(
			dispatch,
			file.source.Metadata().Generated,
			pkg.IllTyped,
		) {
			continue
		}
		ruleContext := rules.NewTypesContext(
			file.source,
			pkg.Fset,
			pkg.ID,
			pkg.Types,
			pkg.TypesInfo,
			pkg.IllTyped,
		)
		var runErr error
		ast.Inspect(file.syntax, func(node ast.Node) bool {
			if runErr != nil {
				return false
			}
			if node == nil {
				return true
			}
			if err := ctx.Err(); err != nil {
				runErr = err
				return false
			}
			kind, found := rules.KindOf(node)
			if !found {
				return true
			}
			for _, active := range dispatch[kind] {
				if file.source.Metadata().Generated && !active.metadata.RunOnGenerated {
					continue
				}
				if pkg.IllTyped && !active.metadata.RunDespiteTypeErrors {
					continue
				}
				findings, err := active.rule.RunTypes(ruleContext, node)
				if contextErr := ctx.Err(); contextErr != nil {
					runErr = contextErr
					return false
				}
				if err != nil {
					runErr = fmt.Errorf("%s: %w", active.metadata.ID, err)
					return false
				}
				for _, finding := range findings {
					diagnostic, err := diagnosticForFinding(
						file.source,
						active.metadata,
						active.severity,
						finding,
					)
					if err != nil {
						runErr = fmt.Errorf("%s: %w", active.metadata.ID, err)
						return false
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
			return true
		})
		if runErr != nil {
			return nil, runErr
		}
	}
	return OrderDiagnostics(diagnostics), nil
}

func anyTypesRuleEligible(
	dispatch map[rules.NodeKind][]activeTypesRule,
	generated bool,
	illTyped bool,
) bool {
	for _, active := range dispatch {
		for _, rule := range active {
			if generated && !rule.metadata.RunOnGenerated {
				continue
			}
			if illTyped && !rule.metadata.RunDespiteTypeErrors {
				continue
			}
			return true
		}
	}
	return false
}

func prepareTypesRules(
	registry *rules.Registry,
	selection []rules.Selection,
) (map[rules.NodeKind][]activeTypesRule, error) {
	ordered := slices.Clone(selection)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].ID < ordered[right].ID })
	dispatch := make(map[rules.NodeKind][]activeTypesRule)
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, fmt.Errorf("selected rule %q more than once", selected.ID)
		}
		previousID = selected.ID
		if selected.Severity != rules.SeverityWarn && selected.Severity != rules.SeverityError {
			return nil, fmt.Errorf("selected rule %q has invalid severity %q", selected.ID, selected.Severity)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, fmt.Errorf("selected rule %q requirement does not match registry", selected.ID)
		}
		if metadata.Requirement != rules.RequireTypes {
			return nil, fmt.Errorf(
				"selected rule %q requires %s; types runner requires types rules",
				selected.ID,
				metadata.Requirement,
			)
		}
		typedRule, found := nativeRule.(rules.TypesRule)
		if !found {
			return nil, fmt.Errorf("selected rule %q does not implement types execution", selected.ID)
		}
		if _, ambiguous := nativeRule.(rules.SyntaxRule); ambiguous {
			return nil, fmt.Errorf("selected rule %q implements ambiguous syntax and types execution", selected.ID)
		}
		active := activeTypesRule{rule: typedRule, metadata: metadata, severity: selected.Severity}
		for _, interest := range metadata.NodeInterests {
			dispatch[interest] = append(dispatch[interest], active)
		}
	}
	return dispatch, nil
}

func packageSyntaxFiles(pkg *packages.Package, sources PackageSourceSet) ([]typedSyntaxFile, error) {
	files := make([]typedSyntaxFile, 0, len(pkg.Syntax))
	seen := make(map[string]struct{}, len(pkg.Syntax))
	for index, syntax := range pkg.Syntax {
		if syntax == nil {
			return nil, fmt.Errorf("typed package %q syntax file %d is nil", pkg.ID, index)
		}
		position := pkg.Fset.PositionFor(syntax.Pos(), false)
		path := filepath.Clean(position.Filename)
		if !filepath.IsAbs(path) || path != position.Filename {
			return nil, fmt.Errorf("typed package %q has non-normalized source path %q", pkg.ID, position.Filename)
		}
		if _, duplicate := seen[path]; duplicate {
			return nil, fmt.Errorf("typed package %q repeats source path %q", pkg.ID, path)
		}
		seen[path] = struct{}{}
		physical, found := sources.Lookup(path)
		if !found {
			return nil, fmt.Errorf("typed package %q source %q was not captured", pkg.ID, path)
		}
		if !physical.CanFormat() {
			continue
		}
		files = append(files, typedSyntaxFile{path: path, source: physical, syntax: syntax})
	}
	sort.Slice(files, func(left, right int) bool { return files[left].path < files[right].path })
	return files, nil
}

func canonicalTypedFiles(
	packages_ []*packages.Package,
	sources PackageSourceSet,
) ([]typedPackageFile, error) {
	byPath := make(map[string]typedPackageFile)
	for _, pkg := range packages_ {
		if pkg.Fset == nil || pkg.Types == nil || pkg.TypesInfo == nil {
			return nil, fmt.Errorf("typed package %q is missing required type information", pkg.ID)
		}
		files, err := packageSyntaxFiles(pkg, sources)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			candidate := typedPackageFile{package_: pkg, file: file}
			current, found := byPath[file.path]
			if !found || preferTypedPackage(candidate.package_, current.package_) {
				byPath[file.path] = candidate
			}
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]typedPackageFile, len(paths))
	for index, path := range paths {
		result[index] = byPath[path]
	}
	return result, nil
}

func preferTypedPackage(candidate, current *packages.Package) bool {
	candidateIsTest, currentIsTest := candidate.ForTest != "", current.ForTest != ""
	if candidateIsTest != currentIsTest {
		return !candidateIsTest
	}
	return candidate.ID < current.ID
}
