package analysis

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/faustbrian/glippy/internal/baseline"
	"github.com/faustbrian/glippy/internal/config"
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
	PathRoot string
	PathOverrides []config.LintOverride
	LintLevels []rules.LintLevelDirective
	Only []string
	Except []string
	RequireSuppressionReason bool
	SuppressionExpiryCutoff string
	Cache *PackageCacheOptions
	Statistics *Statistics
}

// Result is one reporter-ready syntax analysis result over one source version.
type Result struct {
	Path string
	Digest source.Digest
	Requirement rules.Requirement
	Selection []rules.Selection
	Diagnostics []rules.Diagnostic
	PreexistingDiagnostics []rules.Diagnostic
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
	ctx = withStatistics(ctx, options.Statistics)
	resolution, err := options.RuleResolutionForPath(file.Path())
	if err != nil {
		return Result{}, fmt.Errorf("resolve analysis rules: %w", err)
	}
	selection, err := registry.ResolveOptions(resolution)
	if err != nil {
		return Result{}, fmt.Errorf("resolve analysis rules: %w", err)
	}
	options.Statistics.recordPlan(registry, selection)
	options.Statistics.recordSourceFile(file.Path())
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
	policy, err := options.lintPolicy()
	if err != nil {
		return rules.ResolveOptions{}, err
	}
	return options.ruleResolution(policy.LintForExecution()), nil
}

// RuleResolutionForPath resolves the exact policy for one physical source file.
func (options RunOptions) RuleResolutionForPath(sourcePath string) (rules.ResolveOptions, error) {
	policy, err := options.lintPolicy()
	if err != nil {
		return rules.ResolveOptions{}, err
	}
	if len(options.PathOverrides) == 0 {
		return options.ruleResolution(policy.Lint), nil
	}
	relative, err := projectRelativeAnalysisPath(options.PathRoot, sourcePath)
	if err != nil {
		return rules.ResolveOptions{}, err
	}
	resolved, _, err := policy.LintForPath(relative)
	if err != nil {
		return rules.ResolveOptions{}, err
	}
	return options.ruleResolution(resolved), nil
}

func (options RunOptions) lintPolicy() (config.Config, error) {
	if options.Presets != nil && options.Preset != "" {
		return config.Config{}, fmt.Errorf(
			"singular and plural preset policy cannot both be configured",
		)
	}
	presets := options.Presets
	if presets == nil {
		presets = []rules.Preset{options.Preset}
	}
	return config.Config{
		Lint: config.Lint{
			Presets: slices.Clone(presets),
			WarningsAsErrors: options.WarningsAsErrors,
			Rules: cloneSeverityOverrides(options.Overrides),
			RuleOptions: cloneRuleOptions(options.RuleOptions),
			Overrides: clonePathOverrides(options.PathOverrides),
		},
	}, nil
}

func (options RunOptions) ruleResolution(policy config.Lint) rules.ResolveOptions {
	return rules.ResolveOptions{
		Presets: slices.Clone(policy.Presets),
		Overrides: cloneSeverityOverrides(policy.Rules),
		RuleOptions: cloneRuleOptions(policy.RuleOptions),
		SourceGoVersion: options.SourceGoVersion,
		WarningsAsErrors: policy.WarningsAsErrors,
		LintLevels: cloneLintLevelDirectives(options.LintLevels),
		Only: slices.Clone(options.Only),
		Except: slices.Clone(options.Except),
	}
}

func projectRelativeAnalysisPath(root, sourcePath string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("path-scoped lint policy requires a project root")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve path-scoped lint root %q: %w", root, err)
	}
	absoluteSource := sourcePath
	if filepath.IsAbs(sourcePath) {
		absoluteSource, err = filepath.Abs(sourcePath)
		if err != nil {
			return "", fmt.Errorf(
				"resolve path-scoped lint source %q: %w",
				sourcePath,
				err,
			)
		}
	} else {
		absoluteSource = filepath.Join(absoluteRoot, sourcePath)
	}
	relative, err := filepath.Rel(absoluteRoot, absoluteSource)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".." + string(filepath.Separator)) {
		return "", fmt.Errorf(
			"path-scoped lint source %q is outside project root %q",
			sourcePath,
			root,
		)
	}
	return filepath.ToSlash(relative), nil
}

func cloneSeverityOverrides(values map[string]rules.Severity) map[string]rules.Severity {
	if values == nil {
		return nil
	}
	result := make(map[string]rules.Severity, len(values))
	for id, severity := range values {
		result[id] = severity
	}
	return result
}

func cloneRuleOptions(values map[string]rules.OptionSet) map[string]rules.OptionSet {
	if values == nil {
		return nil
	}
	result := make(map[string]rules.OptionSet, len(values))
	for id, options := range values {
		result[id] = options
	}
	return result
}

func clonePathOverrides(values []config.LintOverride) []config.LintOverride {
	if values == nil {
		return nil
	}
	result := make([]config.LintOverride, len(values))
	for index, override := range values {
		result[index] = config.LintOverride{
			Paths: slices.Clone(override.Paths),
			Rules: cloneSeverityOverrides(override.Rules),
		}
	}
	return result
}

func cloneLintLevelDirectives(directives []rules.LintLevelDirective) []rules.LintLevelDirective {
	if directives == nil {
		return nil
	}
	result := make([]rules.LintLevelDirective, len(directives))
	for index, directive := range directives {
		result[index] = rules.LintLevelDirective{
			Level: directive.Level,
			Targets: slices.Clone(directive.Targets),
		}
	}
	return result
}
