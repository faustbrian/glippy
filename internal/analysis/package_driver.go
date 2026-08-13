package analysis

import (
	"context"
	"fmt"
	"slices"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/suppressions"
)

// PackageResult is one suppression-aware package analysis run over a shared
// typed load.
type PackageResult struct {
	Requirement     rules.Requirement
	Selection       []rules.Selection
	LoadDiagnostics []PackageDiagnostic
	SourceProblems  []PackageSourceProblem
	Sources         PackageSourceSet
	Files           []Result
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
	selection, err := registry.ResolveConfiguredForGoVersion(
		options.Preset,
		options.Overrides,
		options.RuleOptions,
		options.SourceGoVersion,
	)
	if err != nil {
		return PackageResult{}, fmt.Errorf("resolve package analysis rules: %w", err)
	}
	result := PackageResult{
		Requirement:     rules.MaximumRequirement(selection),
		Selection:       slices.Clone(selection),
		LoadDiagnostics: []PackageDiagnostic{},
		SourceProblems:  []PackageSourceProblem{},
		Files:           []Result{},
	}
	for _, selected := range selection {
		switch selected.Requirement {
		case rules.RequireSyntax, rules.RequireTypes, rules.RequireControlFlow, rules.RequireSSA:
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
		return result, fmt.Errorf("package analysis requires at least one types-tier, CFG-tier, or SSA-tier rule")
	case rules.RequireTypes, rules.RequireControlFlow, rules.RequireSSA:
	default:
		return result, fmt.Errorf("package analysis has invalid requirement %d", result.Requirement)
	}

	loadOptions.Requirement = result.Requirement
	needsFacts, err := packageAnalyzersNeedFacts(registry, selection)
	if err != nil {
		return result, err
	}
	needsNativeDependencies, err := nativePackageRulesNeedDependencies(registry, selection)
	if err != nil {
		return result, err
	}
	loadOptions.LoadDependencySyntax = needsFacts || needsNativeDependencies
	if err := validatePackageOverlay(loadOptions.Overlay); err != nil {
		return result, err
	}
	loadOptions = clonePackageLoadOptions(loadOptions)
	cachePlan, err := preparePackageCachePlan(options.Cache, selection, loadOptions)
	if err != nil {
		return result, err
	}
	loaded, err := LoadPackages(ctx, loadOptions)
	if err != nil {
		return result, err
	}
	result.LoadDiagnostics = slices.Clone(loaded.Diagnostics)
	result.SourceProblems = loaded.Sources.Problems()
	result.Sources = loaded.Sources
	packages_, err := canonicalPackages(loaded.Packages)
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
		packageAnalyzerSelection,
	)
	if err != nil {
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
		diagnostics = OrderDiagnostics(diagnostics)

		index, problems := suppressions.Parse(file, suppressions.ParseOptions{
			KnownRules:    registry.IDs(),
			RequireReason: options.RequireSuppressionReason,
			ExpiryCutoff:  options.SuppressionExpiryCutoff,
		})
		application := index.Apply(diagnostics)
		result.Files = append(result.Files, Result{
			Path:                file.Path(),
			Digest:              file.Digest(),
			Requirement:         result.Requirement,
			Selection:           slices.Clone(selection),
			Diagnostics:         application.Diagnostics,
			Suppressed:          application.Suppressed,
			UnusedSuppressions:  application.Unused,
			SuppressionProblems: problems,
		})
	}
	if len(nativeByPath) != 0 {
		return result, fmt.Errorf("package diagnostics reference an unselected package source")
	}
	return result, nil
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

func selectRequirement(selection []rules.Selection, requirement rules.Requirement) []rules.Selection {
	result := make([]rules.Selection, 0, len(selection))
	for _, selected := range selection {
		if selected.Requirement == requirement {
			result = append(result, selected)
		}
	}
	return result
}
