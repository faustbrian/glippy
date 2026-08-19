package analysis

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

// PackageResult is one suppression-aware package analysis run over a shared
// typed load.
type PackageResult struct {
	Requirement rules.Requirement
	Selection []rules.Selection
	RootPackagePaths []string
	DependencyPackagePaths []string
	LoadDiagnostics []PackageDiagnostic
	SourceProblems []PackageSourceProblem
	Sources PackageSourceSet
	Files []Result
}

// RunPackages resolves a typed rule plan, loads its maximum prerequisite once,
// and combines syntax, types, CFG, and SSA diagnostics before suppressing.
func RunPackages(
	ctx context.Context,
	registry *rules.Registry,
	options RunOptions,
	loadOptions PackageLoadOptions,
) (PackageResult, error) {
	if ctx == nil {
		return PackageResult{}, fmt.Errorf("package analysis requires a context")
	}
	if registry == nil {
		return PackageResult{}, fmt.Errorf("package analysis requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return PackageResult{}, err
	}
	ctx = withStatistics(ctx, options.Statistics)
	ctx = withPhaseProfiler(ctx, options.Profiler)
	resolution, err := options.RuleResolution()
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve package analysis rules: %w", err)
	}
	selection, err := registry.ResolveOptions(resolution)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve package analysis rules: %w", err)
	}
	options.Statistics.recordPlan(registry, selection)
	result := PackageResult{
		Requirement: rules.MaximumRequirement(selection),
		Selection: slices.Clone(selection),
		LoadDiagnostics: []PackageDiagnostic{},
		SourceProblems: []PackageSourceProblem{},
		Files: []Result{},
	}
	for _, selected := range selection {
		switch selected.Requirement {
		case rules.RequireSyntax,
			rules.RequireTypes,
			rules.RequireControlFlow,
			rules.RequireSSA:
		default:
			return result, fmt.Errorf(
				"selected rule %q requires %s; %s rules are not implemented by package analysis",
				selected.ID,
				selected.Requirement,
				selected.Requirement,
			)
		}
	}
	switch result.Requirement {
	case rules.RequireLexical, rules.RequireSyntax:
		return result, fmt.Errorf(
			"package analysis requires at least one types-tier, CFG-tier, or SSA-tier rule",
		)
	case rules.RequireTypes, rules.RequireControlFlow, rules.RequireSSA:
	default:
		return result, fmt.Errorf(
			"package analysis has invalid requirement %d",
			result.Requirement,
		)
	}

	loadOptions.Requirement = result.Requirement
	needsNativeDependencies, err := nativePackageRulesNeedDependencies(registry, selection)
	if err != nil {
		return result, err
	}
	loadOptions.LoadDependencySyntax = needsNativeDependencies
	loadOptions.compactDependencySource = false
	needsNativeEffects, err := nativePackageRulesNeedEffects(registry, selection)
	if err != nil {
		return result, err
	}
	loadOptions.LoadEffectFacts = needsNativeEffects
	if err := validatePackageOverlay(loadOptions.Overlay); err != nil {
		return result, err
	}
	loadOptions = clonePackageLoadOptions(loadOptions)
	cachePlan, err := preparePackageCachePlan(options.Cache, selection, loadOptions)
	if err != nil {
		return result, err
	}
	statistics := statisticsFromContext(ctx)
	loadStarted := beginStatisticsMeasurement(statistics)
	loaded, err := options.PackageSession.load(ctx, options.SourceGoVersion, loadOptions)
	if err != nil {
		return result, err
	}
	statistics.recordPackageLoad(loadStarted, loaded)
	result.LoadDiagnostics = slices.Clone(loaded.Diagnostics)
	result.SourceProblems = loaded.Sources.Problems()
	result.Sources = loaded.Sources
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return result, err
	}
	result.RootPackagePaths, result.DependencyPackagePaths, err = packageGraphPaths(packages_)
	if err != nil {
		return result, err
	}
	files, err := canonicalPackageSourceFiles(packages_, loaded.Sources)
	if err != nil {
		return result, err
	}

	syntaxSelection := selectRequirement(selection, rules.RequireSyntax)
	typesSelection := selectRequirement(selection, rules.RequireTypes)
	nativeTypesSelection, packageAnalyzerSelection, err := partitionPackageAnalyzers(
		registry,
		typesSelection,
	)
	if err != nil {
		return result, err
	}
	ordinaryPackageAnalyzers, factPackageAnalyzers, err := partitionFactPackageAnalyzers(
		registry,
		packageAnalyzerSelection,
	)
	if err != nil {
		return result, err
	}
	controlFlowSelection := selectRequirement(selection, rules.RequireControlFlow)
	ssaSelection := selectRequirement(selection, rules.RequireSSA)
	nativeDiagnostics, err := runNativePackageAnalysis(
		ctx,
		loaded,
		loadOptions,
		cachePlan,
		registry,
		nativeTypesSelection,
		controlFlowSelection,
		ssaSelection,
	)
	if err != nil {
		return result, err
	}
	nativeByPath := make(map[string][]rules.Diagnostic)
	for _, diagnostic := range nativeDiagnostics {
		nativeByPath[diagnostic.Path] = append(nativeByPath[diagnostic.Path], diagnostic)
	}
	packageAnalyzerDiagnostics, err := runPackageAnalyzers(
		ctx,
		loaded,
		loadOptions,
		cachePlan,
		registry,
		ordinaryPackageAnalyzers,
	)
	if err != nil {
		return result, err
	}
	if len(factPackageAnalyzers) > 0 {
		packageErrors := len(result.LoadDiagnostics) > 0 || len(result.SourceProblems) > 0
		factPlan, err := preparePackageFactPlan(loaded)
		if err != nil {
			return result, err
		}
		sourceFiles := make([]*source.File, len(files))
		for index := range files {
			sourceFiles[index] = files[index].source
			files[index] = ownedPackageSource{}
		}
		files = nil
		packages_ = nil
		loaded = PackageLoadResult{}
		debug.FreeOSMemory()
		factDiagnostics, err := runPackageFactAnalyzers(
			ctx,
			factPlan,
			result.Sources,
			loadOptions,
			cachePlan,
			registry,
			factPackageAnalyzers,
			packageErrors,
		)
		if err != nil {
			return result, err
		}
		packageAnalyzerDiagnostics = append(packageAnalyzerDiagnostics, factDiagnostics...)
		for _, file := range sourceFiles {
			files = append(files, ownedPackageSource{source: file})
		}
	}
	if err := captureProfilePhase(ctx, ProfilePhasePackageAnalyzers); err != nil {
		return result, err
	}
	for _, diagnostic := range packageAnalyzerDiagnostics {
		nativeByPath[diagnostic.Path] = append(nativeByPath[diagnostic.Path], diagnostic)
	}

	for _, work := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		file := work.source
		diagnostics, err := RunSyntax(ctx, file, registry, syntaxSelection)
		if err != nil {
			return result, err
		}
		diagnostics = append(diagnostics, nativeByPath[file.Path()]...)
		delete(nativeByPath, file.Path())
		fileResolution, err := options.RuleResolutionForPath(file.Path())
		if err != nil {
			return result, fmt.Errorf("resolve package file policy: %w", err)
		}
		fileSelection, err := registry.ResolveOptions(fileResolution)
		if err != nil {
			return result, fmt.Errorf("resolve package file policy: %w", err)
		}
		diagnostics = selectFileDiagnostics(diagnostics, fileSelection)
		diagnostics = OrderDiagnostics(diagnostics)

		index, problems := suppressions.Parse(
			file,
			suppressions.ParseOptions{
				KnownRules: registry.IDs(),
				RequireReason: options.RequireSuppressionReason,
				ExpiryCutoff: options.SuppressionExpiryCutoff,
			},
		)
		application := index.Apply(diagnostics)
		result.Files = append(
			result.Files,
			Result{
				Path: file.Path(),
				Digest: file.Digest(),
				Requirement: rules.MaximumRequirement(fileSelection),
				Selection: slices.Clone(fileSelection),
				Diagnostics: application.Diagnostics,
				Suppressed: application.Suppressed,
				UnusedSuppressions: application.Unused,
				SuppressionProblems: problems,
			},
		)
	}
	if len(nativeByPath) != 0 {
		return result, fmt.Errorf(
			"package diagnostics reference an unselected package source",
		)
	}
	if err := captureProfilePhase(ctx, ProfilePhaseResult); err != nil {
		return result, err
	}
	return result, nil
}

func packageGraphPaths(roots []*packages.Package) ([]string, []string, error) {
	rootPaths := make(map[string]struct{}, len(roots))
	for _, pkg := range roots {
		if pkg == nil ||
			strings.TrimSpace(pkg.ID) == "" ||
			strings.TrimSpace(pkg.PkgPath) == "" {
			return nil, nil, fmt.Errorf(
				"package analysis graph contains an unidentified root package",
			)
		}
		rootPaths[pkg.PkgPath] = struct{}{}
	}
	visited := make(map[string]*packages.Package)
	stack := slices.Clone(roots)
	for len(stack) > 0 {
		pkg := stack[len(stack) - 1]
		stack = stack[:len(stack) - 1]
		if pkg == nil ||
			strings.TrimSpace(pkg.ID) == "" ||
			strings.TrimSpace(pkg.PkgPath) == "" {
			return nil, nil, fmt.Errorf(
				"package analysis graph contains an unidentified package",
			)
		}
		if previous, found := visited[pkg.ID]; found {
			if previous != pkg {
				return nil, nil, fmt.Errorf(
					"package analysis graph has conflicting package ID %q",
					pkg.ID,
				)
			}
			continue
		}
		visited[pkg.ID] = pkg
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		slices.Sort(imports)
		for index := len(imports) - 1; index >= 0; index-- {
			stack = append(stack, pkg.Imports[imports[index]])
		}
	}
	rootsResult := make([]string, 0, len(rootPaths))
	for path := range rootPaths {
		rootsResult = append(rootsResult, path)
	}
	slices.Sort(rootsResult)
	dependencies := make(map[string]struct{})
	for _, pkg := range visited {
		if _, root := rootPaths[pkg.PkgPath]; !root {
			dependencies[pkg.PkgPath] = struct{}{}
		}
	}
	dependencyResult := make([]string, 0, len(dependencies))
	for path := range dependencies {
		dependencyResult = append(dependencyResult, path)
	}
	slices.Sort(dependencyResult)
	return rootsResult, dependencyResult, nil
}

func selectFileDiagnostics(
	diagnostics []rules.Diagnostic,
	selection []rules.Selection,
) []rules.Diagnostic {
	severities := make(map[string]rules.Severity, len(selection))
	for _, selected := range selection {
		severities[selected.ID] = selected.Severity
	}
	result := make([]rules.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		severity, selected := severities[diagnostic.RuleID]
		if !selected {
			continue
		}
		diagnostic.Severity = severity
		result = append(result, diagnostic)
	}
	return result
}

func nativePackageRulesNeedDependencies(
	registry *rules.Registry,
	selection []rules.Selection,
) (bool, error) {
	for _, selected := range selection {
		metadata, found := registry.Metadata(selected.ID)
		if !found {
			return false, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		if metadata.RequiresDependencySyntax {
			return true, nil
		}
	}
	return false, nil
}

func nativePackageRulesNeedEffects(
	registry *rules.Registry,
	selection []rules.Selection,
) (bool, error) {
	for _, selected := range selection {
		metadata, found := registry.Metadata(selected.ID)
		if !found {
			return false, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		if metadata.RequiresEffectFacts {
			return true, nil
		}
	}
	return false, nil
}

func selectRequirement(
	selection []rules.Selection,
	requirement rules.Requirement,
) []rules.Selection {
	result := make([]rules.Selection, 0, len(selection))
	for _, selected := range selection {
		if selected.Requirement == requirement {
			result = append(result, selected)
		}
	}
	return result
}
