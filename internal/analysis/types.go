package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"path/filepath"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

type activeTypesRule struct {
	rule rules.TypesRule
	metadata rules.Metadata
	severity rules.Severity
	options rules.OptionSet
}

type activePackageRule struct {
	rule rules.PackageRule
	metadata rules.Metadata
	severity rules.Severity
	options rules.OptionSet
}

type typedSyntaxFile struct {
	path string
	source *source.File
	syntax *ast.File
}

type typedPackageFile struct {
	package_ *packages.Package
	file typedSyntaxFile
}

type ownedPackageSource struct {
	package_ *packages.Package
	source *source.File
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
	dispatch, packageRules, err := prepareTypesRules(registry, selection)
	if err != nil {
		return nil, err
	}
	if len(dispatch) == 0 && len(packageRules) == 0 {
		return []rules.Diagnostic{}, nil
	}
	statistics := statisticsFromContext(ctx)
	tierStarted := beginStatisticsMeasurement(statistics)
	defer statistics.recordTier(rules.RequireTypes, tierStarted)
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return nil, err
	}
	files, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return nil, err
	}
	diagnostics, err := runNativePackageRules(
		ctx,
		packages_,
		files,
		loaded.Sources,
		packageRules,
	)
	if err != nil {
		return nil, err
	}
	for _, work := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pkg, file := work.package_, work.file
		if !anyTypesRuleEligible(dispatch, file.source.Metadata().Generated, pkg.IllTyped) {
			continue
		}
		ruleContexts := make(map[string]*rules.TypesContext)
		var runErr error
		ast.Inspect(
			file.syntax,
			func(node ast.Node) bool {
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
					if file.source.Metadata().Generated &&
						!active.metadata.RunOnGenerated {
						continue
					}
					if pkg.IllTyped && !active.metadata.RunDespiteTypeErrors {
						continue
					}
					ruleContext := ruleContexts[active.metadata.ID]
					if ruleContext == nil {
						ruleContext = rules.NewTypesContext(
							file.source,
							file.syntax,
							pkg.Fset,
							pkg.ID,
							pkg.Types,
							pkg.TypesInfo,
							pkg.IllTyped,
							active.options,
						)
						ruleContexts[active.metadata.ID] = ruleContext
					}
					ruleStarted := beginStatisticsMeasurement(statistics)
					findings, err := active.rule.RunTypes(ruleContext, node)
					statistics.recordRule(
						active.metadata.ID,
						active.metadata.Requirement,
						ruleStarted,
					)
					if contextErr := ctx.Err(); contextErr != nil {
						runErr = contextErr
						return false
					}
					if err != nil {
						runErr = fmt.Errorf(
							"%s: %w",
							active.metadata.ID,
							err,
						)
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
							runErr = fmt.Errorf(
								"%s: %w",
								active.metadata.ID,
								err,
							)
							return false
						}
						diagnostics = append(diagnostics, diagnostic)
					}
				}
				return true
			},
		)
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
) (map[rules.NodeKind][]activeTypesRule, []activePackageRule, error) {
	ordered := slices.Clone(selection)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].ID < ordered[right].ID
		},
	)
	dispatch := make(map[rules.NodeKind][]activeTypesRule)
	packageRules := make([]activePackageRule, 0)
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, nil, fmt.Errorf("selected rule %q more than once", selected.ID)
		}
		previousID = selected.ID
		if selected.Severity != rules.SeverityWarn &&
			selected.Severity != rules.SeverityError {
			return nil, nil, fmt.Errorf(
				"selected rule %q has invalid severity %q",
				selected.ID,
				selected.Severity,
			)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, nil, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, nil, fmt.Errorf(
				"selected rule %q requirement does not match registry",
				selected.ID,
			)
		}
		if metadata.Requirement != rules.RequireTypes {
			return nil, nil, fmt.Errorf(
				"selected rule %q requires %s; types runner requires types rules",
				selected.ID,
				metadata.Requirement,
			)
		}
		typedRule, typed := nativeRule.(rules.TypesRule)
		packageRule, packageWide := nativeRule.(rules.PackageRule)
		if typed && packageWide {
			return nil, nil, fmt.Errorf(
				"selected rule %q implements ambiguous types execution",
				selected.ID,
			)
		}
		if implementsNonTypesExecution(nativeRule) {
			return nil, nil, fmt.Errorf(
				"selected rule %q implements ambiguous types execution",
				selected.ID,
			)
		}
		if packageWide {
			packageRules = append(
				packageRules,
				activePackageRule{
					rule: packageRule,
					metadata: metadata,
					severity: selected.Severity,
					options: selected.Options,
				},
			)
			continue
		}
		if !typed {
			return nil, nil, fmt.Errorf(
				"selected rule %q does not implement types execution",
				selected.ID,
			)
		}
		active := activeTypesRule{
			rule: typedRule,
			metadata: metadata,
			severity: selected.Severity,
			options: selected.Options,
		}
		for _, interest := range metadata.NodeInterests {
			dispatch[interest] = append(dispatch[interest], active)
		}
	}
	return dispatch, packageRules, nil
}

func implementsNonTypesExecution(rule rules.Rule) bool {
	_, syntax := rule.(rules.SyntaxRule)
	_, syntaxFile := rule.(rules.SyntaxFileRule)
	_, controlFlow := rule.(rules.ControlFlowRule)
	_, ssa := rule.(rules.SSARule)
	return syntax || syntaxFile || controlFlow || ssa
}

func runNativePackageRules(
	ctx context.Context,
	packages_ []*packages.Package,
	ownedFiles []typedPackageFile,
	sources PackageSourceSet,
	activeRules []activePackageRule,
) ([]rules.Diagnostic, error) {
	owners := make(map[string]string, len(ownedFiles))
	for _, owned := range ownedFiles {
		owners[owned.file.path] = owned.package_.ID
	}
	diagnostics := make([]rules.Diagnostic, 0)
	dependencyViews := make(map[*packages.Package][]rules.PackageDependency)
	for _, active := range activeRules {
		for _, pkg := range packages_ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if pkg.TypesSizes == nil {
				return nil, fmt.Errorf(
					"typed package %q is missing type sizes",
					pkg.ID,
				)
			}
			if pkg.IllTyped && !active.metadata.RunDespiteTypeErrors {
				continue
			}
			files, err := packageSyntaxFiles(pkg, sources)
			if err != nil {
				return nil, err
			}
			packageFiles := make([]rules.PackageFile, 0, len(files))
			targetCount := 0
			for _, file := range files {
				target := owners[file.path] == pkg.ID &&
					(!file.source.Metadata().Generated ||
						active.metadata.RunOnGenerated)
				packageFile := rules.NewPackageFile(
					file.source,
					file.syntax,
					pkg.Fset,
					target,
				)
				packageFiles = append(packageFiles, packageFile)
				if target {
					targetCount++
				}
			}
			if targetCount == 0 {
				continue
			}
			var dependencies []rules.PackageDependency
			if active.metadata.RequiresDependencySyntax {
				var found bool
				dependencies, found = dependencyViews[pkg]
				if !found {
					dependencies, err = packageDependencies(pkg, sources)
					if err != nil {
						return nil, err
					}
					dependencyViews[pkg] = dependencies
				}
			}
			ruleContext := rules.NewPackageContext(
				pkg.Fset,
				pkg.ID,
				pkg.Types,
				pkg.TypesInfo,
				pkg.TypesSizes,
				pkg.IllTyped,
				packageFiles,
				dependencies,
				active.options,
			)
			ruleStarted := beginStatisticsMeasurement(statisticsFromContext(ctx))
			findings, err := active.rule.RunPackage(ruleContext)
			statisticsFromContext(
				ctx,
			).recordRule(active.metadata.ID, active.metadata.Requirement, ruleStarted)
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", active.metadata.ID, err)
			}
			for _, finding := range findings {
				file := finding.File.Source()
				if file == nil || !finding.File.Target() {
					return nil, fmt.Errorf(
						"%s: package finding requires an owned target file",
						active.metadata.ID,
					)
				}
				if !ruleContext.OwnsTarget(finding.File) {
					return nil, fmt.Errorf(
						"%s: package finding must use a target from the same package callback",
						active.metadata.ID,
					)
				}
				diagnostic, err := diagnosticForFinding(
					file,
					active.metadata,
					active.severity,
					finding.Finding,
				)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", active.metadata.ID, err)
				}
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	return diagnostics, nil
}

func packageDependencies(
	root *packages.Package,
	sources PackageSourceSet,
) ([]rules.PackageDependency, error) {
	state := make(map[*packages.Package]uint8)
	result := make([]rules.PackageDependency, 0)
	var visit func(*packages.Package) error
	visit = func(pkg *packages.Package) error {
		if pkg == nil {
			return fmt.Errorf("typed package %q has a nil dependency", root.ID)
		}
		switch state[pkg] {
		case 1:
			return fmt.Errorf(
				"native dependency graph contains an import cycle at %q",
				pkg.ID,
			)
		case 2:
			return nil
		}
		state[pkg] = 1
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)
		for _, path := range imports {
			if err := visit(pkg.Imports[path]); err != nil {
				return err
			}
		}
		state[pkg] = 2
		if pkg == root {
			return nil
		}
		if pkg.Fset == nil ||
			pkg.Types == nil ||
			pkg.TypesInfo == nil ||
			pkg.TypesSizes == nil {
			return fmt.Errorf(
				"typed dependency %q is missing required type information",
				pkg.ID,
			)
		}
		files, err := packageSyntaxFiles(pkg, sources)
		if err != nil {
			return err
		}
		packageFiles := make([]rules.PackageFile, len(files))
		for index, file := range files {
			packageFiles[index] = rules.NewPackageFile(
				file.source,
				file.syntax,
				pkg.Fset,
				false,
			)
		}
		result = append(
			result,
			rules.NewPackageDependency(
				pkg.Fset,
				pkg.ID,
				pkg.Types,
				pkg.TypesInfo,
				pkg.TypesSizes,
				pkg.IllTyped,
				packageFiles,
			),
		)
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return result, nil
}

func packageSyntaxFiles(
	pkg *packages.Package,
	sources PackageSourceSet,
) ([]typedSyntaxFile, error) {
	files := make([]typedSyntaxFile, 0, len(pkg.Syntax))
	seen := make(map[string]struct{}, len(pkg.Syntax))
	editable := make(map[string]struct{}, len(pkg.GoFiles))
	for _, path := range pkg.GoFiles {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) || path == "" {
			return nil, fmt.Errorf(
				"typed package %q has invalid original source path %q",
				pkg.ID,
				path,
			)
		}
		editable[path] = struct{}{}
	}
	for index, syntax := range pkg.Syntax {
		if syntax == nil {
			return nil, fmt.Errorf(
				"typed package %q syntax file %d is nil",
				pkg.ID,
				index,
			)
		}
		position := pkg.Fset.PositionFor(syntax.Pos(), false)
		path := filepath.Clean(position.Filename)
		if !filepath.IsAbs(path) || path != position.Filename {
			return nil, fmt.Errorf(
				"typed package %q has non-normalized source path %q",
				pkg.ID,
				position.Filename,
			)
		}
		if _, duplicate := seen[path]; duplicate {
			return nil, fmt.Errorf(
				"typed package %q repeats source path %q",
				pkg.ID,
				path,
			)
		}
		seen[path] = struct{}{}
		if _, found := editable[path]; !found {
			continue
		}
		physical, found := sources.Lookup(path)
		if !found {
			return nil, fmt.Errorf(
				"typed package %q source %q was not captured",
				pkg.ID,
				path,
			)
		}
		if !physical.CanAnalyze() {
			continue
		}
		files = append(files, typedSyntaxFile{path: path, source: physical, syntax: syntax})
	}
	sort.Slice(
		files,
		func(left, right int) bool {
			return files[left].path < files[right].path
		},
	)
	return files, nil
}

func canonicalPackageSourceFiles(
	packages_ []*packages.Package,
	sources PackageSourceSet,
) ([]ownedPackageSource, error) {
	byPath := make(map[string]ownedPackageSource)
	for _, pkg := range packages_ {
		for _, path := range pkg.GoFiles {
			path = filepath.Clean(path)
			if !filepath.IsAbs(path) || path == "" {
				return nil, fmt.Errorf(
					"typed package %q has invalid original source path %q",
					pkg.ID,
					path,
				)
			}
			physical, found := sources.Lookup(path)
			if !found {
				return nil, fmt.Errorf(
					"typed package %q original source %q was not captured",
					pkg.ID,
					path,
				)
			}
			if !physical.CanFormat() {
				continue
			}
			candidate := ownedPackageSource{package_: pkg, source: physical}
			current, found := byPath[path]
			if !found || preferTypedPackage(candidate.package_, current.package_) {
				byPath[path] = candidate
			}
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]ownedPackageSource, len(paths))
	for index, path := range paths {
		result[index] = byPath[path]
	}
	return result, nil
}

func canonicalTypedFiles(
	packages_ []*packages.Package,
	sources PackageSourceSet,
) ([]typedPackageFile, error) {
	byPath := make(map[string]typedPackageFile)
	for _, pkg := range packages_ {
		if pkg.Fset == nil || pkg.Types == nil || pkg.TypesInfo == nil {
			return nil, fmt.Errorf(
				"typed package %q is missing required type information",
				pkg.ID,
			)
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
