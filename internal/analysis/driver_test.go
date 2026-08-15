package analysis_test

import (
	"context"
	"go/ast"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

func TestRunAppliesPathScopedRulePolicyBeforeCommandLineLevels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file, err := source.Load(
		filepath.Join(root, "internal", "client_test.go"),
		[]byte("package sample\nfunc run(){ target() }\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	rule := syntaxRule{
		metadata: analysisMetadata("call-rule", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			range_, rangeErr := ctx.Range(node)
			if rangeErr != nil {
				return nil, rangeErr
			}
			return []rules.Finding{
				{
					MessageKey: "call",
					Message: "call requires review",
					Range: range_,
				},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	options := analysis.RunOptions{
		Presets: []rules.Preset{},
		Overrides: map[string]rules.Severity{"call-rule": rules.SeverityOff},
		WarningsAsErrors: true,
		PathRoot: root,
		PathOverrides: []config.LintOverride{
			{
				Paths: []string{"**/*_test.go"},
				Rules: map[string]config.Severity{"call-rule": config.SeverityWarn},
			},
		},
	}

	result, err := analysis.Run(context.Background(), file, registry, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Severity != rules.SeverityError ||
		len(result.Selection) != 1 ||
		result.Selection[0].Severity != rules.SeverityError {
		t.Fatalf("Run(path override) = %#v", result)
	}

	options.LintLevels = []rules.LintLevelDirective{
		{Level: rules.LintAllow, Targets: []string{"call-rule"}},
	}
	result, err = analysis.Run(context.Background(), file, registry, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 || len(result.Selection) != 0 {
		t.Fatalf("Run(command-line allow) = %#v, want no selected rule", result)
	}
}

func TestRunResolvesRelativeSourcePathsAgainstPathPolicyRoot(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		filepath.Join("internal", "client_test.go"),
		[]byte("package sample\nfunc run(){ target() }\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	rule := syntaxRule{
		metadata: analysisMetadata("call-rule", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			range_, rangeErr := ctx.Range(node)
			if rangeErr != nil {
				return nil, rangeErr
			}
			return []rules.Finding{
				{
					MessageKey: "call",
					Message: "call requires review",
					Range: range_,
				},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			PathRoot: t.TempDir(),
			PathOverrides: []config.LintOverride{
				{
					Paths: []string{"internal/**"},
					Rules: map[string]config.Severity{
						"call-rule": config.SeverityWarn,
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("Run(relative path policy) diagnostics = %#v", result.Diagnostics)
	}
}

func TestRunResolvesSyntaxRulesAndAppliesSuppressions(t *testing.T) {
	t.Parallel()

	input := `package sample

//glippy:ignore call-rule -- accepted here
func suppressed() { target() }

func visible() { target() }

//glippy:ignore unused-rule -- no matching finding
//glippy:ignore unknown-rule -- misspelled rule
`
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	callRule := syntaxRule{
		metadata: analysisMetadata("call-rule", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{
					MessageKey: "call",
					Message: "call requires review",
					Range: sourceRange,
				},
			}, nil
		},
	}
	unusedRule := syntaxRule{
		metadata: analysisMetadata("unused-rule", rules.NodeFuncDecl, false),
		run: func(*rules.Context, ast.Node) ([]rules.Finding, error) {
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(unusedRule, callRule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Preset: rules.PresetCorrectness,
			Overrides: map[string]rules.Severity{"call-rule": rules.SeverityError},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Requirement != rules.RequireSyntax {
		t.Fatalf("Run() requirement = %s, want syntax", result.Requirement)
	}
	if result.Path != file.Path() || result.Digest != file.Digest() {
		t.Fatalf("Run() source identity = %q/%x", result.Path, result.Digest)
	}
	wantSelection := []rules.Selection{
		{ID: "call-rule", Severity: rules.SeverityError, Requirement: rules.RequireSyntax},
		{ID: "unused-rule", Severity: rules.SeverityWarn, Requirement: rules.RequireSyntax},
	}
	if !reflect.DeepEqual(result.Selection, wantSelection) {
		t.Fatalf("Run() selection = %#v, want %#v", result.Selection, wantSelection)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Severity != rules.SeverityError {
		t.Fatalf("Run() diagnostics = %#v", result.Diagnostics)
	}
	visibleStart := strings.LastIndex(input, "target()")
	if result.Diagnostics[0].Range.Start != visibleStart {
		t.Fatalf(
			"visible diagnostic start = %d, want %d",
			result.Diagnostics[0].Range.Start,
			visibleStart,
		)
	}
	if len(result.Suppressed) != 1 || result.Suppressed[0].Directive.RuleID != "call-rule" {
		t.Fatalf("Run() suppressed = %#v", result.Suppressed)
	}
	if len(result.UnusedSuppressions) != 1 ||
		result.UnusedSuppressions[0].RuleID != "unused-rule" {
		t.Fatalf("Run() unused suppressions = %#v", result.UnusedSuppressions)
	}
	if len(result.SuppressionProblems) != 1 ||
		result.SuppressionProblems[0].Kind != suppressions.ProblemUnknownRule {
		t.Fatalf("Run() suppression problems = %#v", result.SuppressionProblems)
	}
}

func TestRunRefusesUnsupportedRuleTiersBeforeExecution(t *testing.T) {
	t.Parallel()

	file, err := source.Load("typed.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := analysisMetadata("typed-rule", rules.NodeFile, false)
	metadata.Requirement = rules.RequireTypes
	registry, err := rules.NewRegistry(metadataRuleAdapter{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
	)
	if err == nil || !strings.Contains(err.Error(), "requires types") {
		t.Fatalf("Run() error = %v, want unsupported types tier", err)
	}
	if result.Requirement != rules.RequireTypes {
		t.Fatalf("Run() requirement = %s, want types", result.Requirement)
	}
}

func TestRunRejectsAmbiguousSingularAndPluralPresetPolicy(t *testing.T) {
	t.Parallel()

	file, err := source.Load("sample.go", []byte("package sample\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(
		syntaxRule{
			metadata: analysisMetadata("call-rule", rules.NodeCallExpr, false),
			run: func(*rules.Context, ast.Node) ([]rules.Finding, error) {
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Preset: rules.PresetCorrectness,
			Presets: []rules.Preset{rules.PresetStyle},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "singular and plural preset policy") {
		t.Fatalf("Run() error = %v, want ambiguous preset policy rejection", err)
	}
}

func TestRunOptionsSnapshotsLintLevelDirectives(t *testing.T) {
	t.Parallel()

	options := analysis.RunOptions{
		Presets: []rules.Preset{rules.PresetCorrectness},
		LintLevels: []rules.LintLevelDirective{
			{Level: rules.LintDeny, Targets: []string{"warnings", "correctness"}},
		},
	}
	resolution, err := options.RuleResolution()
	if err != nil {
		t.Fatal(err)
	}
	options.LintLevels[0].Targets[0] = "mutated-input"
	if resolution.LintLevels[0].Targets[0] != "warnings" {
		t.Fatalf("RuleResolution() retained input alias: %#v", resolution.LintLevels)
	}
	resolution.LintLevels[0].Targets[1] = "mutated-output"
	if options.LintLevels[0].Targets[1] != "correctness" {
		t.Fatalf("RuleResolution() returned aliased targets: %#v", options.LintLevels)
	}
}

func TestRunEmptyRegistryProducesOneCompleteEmptyResult(t *testing.T) {
	t.Parallel()

	file, err := source.Load("empty.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Requirement != rules.RequireLexical ||
		len(result.Selection) != 0 ||
		result.Path != file.Path() ||
		result.Digest != file.Digest() ||
		len(result.Diagnostics) != 0 ||
		len(result.Suppressed) != 0 ||
		len(result.UnusedSuppressions) != 0 ||
		len(result.SuppressionProblems) != 0 {
		t.Fatalf("Run() = %#v", result)
	}
}

func TestRunRoutesTypedOptionsToSyntaxRules(t *testing.T) {
	t.Parallel()

	file, err := source.Load("configured.go", []byte("package sample\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := analysisMetadata("configured-rule", rules.NodeCallExpr, false)
	metadata.Options = []rules.OptionMetadata{
		{
			Name: "enabled",
			Summary: "enable reporting",
			Kind: rules.OptionBoolean,
			Required: true,
		},
	}
	rule := syntaxRule{
		metadata: metadata,
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			enabled, found := ctx.BooleanOption("enabled")
			if !found || !enabled {
				t.Fatalf("syntax option enabled = %t, %t", enabled, found)
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Preset: rules.PresetCorrectness,
			RuleOptions: map[string]rules.OptionSet{
				"configured-rule": rules.NewOptionSet(
					map[string]rules.OptionValue{
						"enabled": rules.BooleanOption(true),
					},
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}
