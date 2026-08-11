package analysis_test

import (
	"context"
	"go/ast"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
	"github.com/faustbrian/gox/internal/suppressions"
)

func TestRunResolvesSyntaxRulesAndAppliesSuppressions(t *testing.T) {
	t.Parallel()

	input := `package sample

//gox:ignore call-rule -- accepted here
func suppressed() { target() }

func visible() { target() }

//gox:ignore unused-rule -- no matching finding
//gox:ignore unknown-rule -- misspelled rule
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
			return []rules.Finding{{
				MessageKey: "call",
				Message:    "call requires review",
				Range:      sourceRange,
			}}, nil
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

	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Overrides: map[string]rules.Severity{
			"call-rule": rules.SeverityError,
		},
	})
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
		t.Fatalf("visible diagnostic start = %d, want %d", result.Diagnostics[0].Range.Start, visibleStart)
	}
	if len(result.Suppressed) != 1 || result.Suppressed[0].Directive.RuleID != "call-rule" {
		t.Fatalf("Run() suppressed = %#v", result.Suppressed)
	}
	if len(result.UnusedSuppressions) != 1 || result.UnusedSuppressions[0].RuleID != "unused-rule" {
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

	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err == nil || !strings.Contains(err.Error(), "requires types") {
		t.Fatalf("Run() error = %v, want unsupported types tier", err)
	}
	if result.Requirement != rules.RequireTypes {
		t.Fatalf("Run() requirement = %s, want types", result.Requirement)
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
	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requirement != rules.RequireLexical || len(result.Selection) != 0 ||
		result.Path != file.Path() || result.Digest != file.Digest() ||
		len(result.Diagnostics) != 0 || len(result.Suppressed) != 0 ||
		len(result.UnusedSuppressions) != 0 || len(result.SuppressionProblems) != 0 {
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
	metadata.Options = []rules.OptionMetadata{{
		Name: "enabled", Summary: "enable reporting", Kind: rules.OptionBoolean, Required: true,
	}}
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
	_, err = analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		RuleOptions: map[string]rules.OptionSet{
			"configured-rule": rules.NewOptionSet(map[string]rules.OptionValue{
				"enabled": rules.BooleanOption(true),
			}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
