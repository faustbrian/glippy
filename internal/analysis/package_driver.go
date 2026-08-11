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
	Files           []Result
}

// RunPackages resolves a typed rule plan, loads its maximum prerequisite once,
// and combines syntax and types diagnostics before applying suppressions.
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
	selection, err := registry.Resolve(options.Preset, options.Overrides)
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
		case rules.RequireSyntax, rules.RequireTypes:
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
		return result, fmt.Errorf("package analysis requires at least one types-tier rule")
	case rules.RequireTypes:
	default:
		return result, fmt.Errorf("package analysis has invalid requirement %d", result.Requirement)
	}

	loadOptions.Requirement = result.Requirement
	loaded, err := LoadPackages(ctx, loadOptions)
	if err != nil {
		return result, err
	}
	result.LoadDiagnostics = slices.Clone(loaded.Diagnostics)
	result.SourceProblems = loaded.Sources.Problems()
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return result, err
	}
	files, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return result, err
	}

	syntaxSelection := selectRequirement(selection, rules.RequireSyntax)
	typesSelection := selectRequirement(selection, rules.RequireTypes)
	typedDiagnostics, err := RunTypes(ctx, loaded, registry, typesSelection)
	if err != nil {
		return result, err
	}
	typedByPath := make(map[string][]rules.Diagnostic)
	for _, diagnostic := range typedDiagnostics {
		typedByPath[diagnostic.Path] = append(typedByPath[diagnostic.Path], diagnostic)
	}

	for _, work := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		file := work.file.source
		diagnostics, err := RunSyntax(ctx, file, registry, syntaxSelection)
		if err != nil {
			return result, err
		}
		diagnostics = append(diagnostics, typedByPath[file.Path()]...)
		delete(typedByPath, file.Path())
		diagnostics = OrderDiagnostics(diagnostics)

		index, problems := suppressions.Parse(file, suppressions.ParseOptions{
			KnownRules:    registry.IDs(),
			RequireReason: options.RequireSuppressionReason,
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
	if len(typedByPath) != 0 {
		return result, fmt.Errorf("typed diagnostics reference an unselected package source")
	}
	return result, nil
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
