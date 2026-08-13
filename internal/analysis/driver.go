package analysis

import (
	"context"
	"fmt"
	"slices"

	"github.com/faustbrian/glippy/internal/baseline"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

// RunOptions selects native rules and suppression policy for one source file.
type RunOptions struct {
	SourceGoVersion string
	Preset rules.Preset
	Presets []rules.Preset
	WarningsAsErrors bool
	Overrides map[string]rules.Severity
	RuleOptions map[string]rules.OptionSet
	RequireSuppressionReason bool
	SuppressionExpiryCutoff string
	Cache *PackageCacheOptions
}

// Result is one reporter-ready syntax analysis result over one source version.
type Result struct {
	Path string
	Digest source.Digest
	Requirement rules.Requirement
	Selection []rules.Selection
	Diagnostics []rules.Diagnostic
	Baselined []rules.Diagnostic
	BaselineProblems []baseline.Problem
	Suppressed []suppressions.SuppressedDiagnostic
	UnusedSuppressions []suppressions.Directive
	SuppressionProblems []suppressions.Problem
}

// Run resolves, schedules, and suppresses native syntax diagnostics.
func Run(
	ctx context.Context,
	file *source.File,
	registry *rules.Registry,
	options RunOptions,
) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("analysis run requires a context")
	}
	if file == nil {
		return Result{}, fmt.Errorf("analysis run requires a source file")
	}
	if registry == nil {
		return Result{}, fmt.Errorf("analysis run requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	resolution, err := options.RuleResolution()
	if err != nil {
		return Result{}, fmt.Errorf("resolve analysis rules: %w", err)
	}
	selection, err := registry.ResolveOptions(resolution)
	if err != nil {
		return Result{}, fmt.Errorf("resolve analysis rules: %w", err)
	}
	result := Result{
		Path: file.Path(),
		Digest: file.Digest(),
		Requirement: rules.MaximumRequirement(selection),
		Selection: slices.Clone(selection),
		Diagnostics: []rules.Diagnostic{},
		Baselined: []rules.Diagnostic{},
		BaselineProblems: []baseline.Problem{},
		Suppressed: []suppressions.SuppressedDiagnostic{},
		UnusedSuppressions: []suppressions.Directive{},
		SuppressionProblems: []suppressions.Problem{},
	}
	for _, selected := range selection {
		if selected.Requirement != rules.RequireSyntax {
			return result, fmt.Errorf(
				"selected rule %q requires %s; analysis driver currently supports syntax rules only",
				selected.ID,
				selected.Requirement,
			)
		}
	}

	index, problems := suppressions.Parse(
		file,
		suppressions.ParseOptions{
			KnownRules: registry.IDs(),
			RequireReason: options.RequireSuppressionReason,
			ExpiryCutoff: options.SuppressionExpiryCutoff,
		},
	)
	diagnostics, err := RunSyntax(ctx, file, registry, selection)
	if err != nil {
		return result, err
	}
	application := index.Apply(diagnostics)
	result.Diagnostics = application.Diagnostics
	result.Suppressed = application.Suppressed
	result.UnusedSuppressions = application.Unused
	result.SuppressionProblems = problems
	return result, nil
}

// RuleResolution returns the canonical registry selection bound to this run.
func (options RunOptions) RuleResolution() (rules.ResolveOptions, error) {
	if options.Presets != nil && options.Preset != "" {
		return rules.ResolveOptions{}, fmt.Errorf(
			"singular and plural preset policy cannot both be configured",
		)
	}
	presets := options.Presets
	if presets == nil {
		presets = []rules.Preset{options.Preset}
	}
	return rules.ResolveOptions{
		Presets: presets,
		Overrides: options.Overrides,
		RuleOptions: options.RuleOptions,
		SourceGoVersion: options.SourceGoVersion,
		WarningsAsErrors: options.WarningsAsErrors,
	}, nil
}
