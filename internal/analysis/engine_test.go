package analysis_test

import (
	"context"
	"errors"
	"go/ast"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type syntaxRule struct {
	metadata rules.Metadata
	visits   *[]string
	run      func(*rules.Context, ast.Node) ([]rules.Finding, error)
}

func (r syntaxRule) Metadata() rules.Metadata { return r.metadata }

func (r syntaxRule) RunSyntax(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
	if r.visits != nil {
		*r.visits = append(*r.visits, r.metadata.ID)
	}
	return r.run(ctx, node)
}

type syntaxFileRule struct {
	metadata rules.Metadata
	run      func(*rules.Context) ([]rules.Finding, error)
}

func (r syntaxFileRule) Metadata() rules.Metadata { return r.metadata }

func (r syntaxFileRule) RunSyntaxFile(ctx *rules.Context) ([]rules.Finding, error) {
	if r.run == nil {
		return nil, nil
	}
	return r.run(ctx)
}

type ambiguousSyntaxRule struct {
	syntaxFileRule
}

func (ambiguousSyntaxRule) RunSyntax(*rules.Context, ast.Node) ([]rules.Finding, error) {
	return nil, nil
}

func TestRunSyntaxDispatchesNodeInterestsAndSortsDiagnostics(t *testing.T) {
	t.Parallel()

	file, err := source.Load("example.go", []byte("package example\nfunc run(){later();earlier()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	var visits []string
	callRule := syntaxRule{
		metadata: analysisMetadata("z-call", rules.NodeCallExpr, false),
		visits:   &visits,
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{
				MessageKey: "call",
				Message:    "call is discouraged",
				Range:      sourceRange,
			}}, nil
		},
	}
	functionRule := syntaxRule{
		metadata: analysisMetadata("a-function", rules.NodeFuncDecl, false),
		visits:   &visits,
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{
				MessageKey: "function",
				Message:    "function is discouraged",
				Range:      sourceRange,
			}}, nil
		},
	}
	registry, err := rules.NewRegistry(callRule, functionRule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}

	diagnostics, err := analysis.RunSyntax(context.Background(), file, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := visits, []string{"a-function", "z-call", "z-call"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rule visits = %#v, want %#v", got, want)
	}
	if len(diagnostics) != 3 {
		t.Fatalf("RunSyntax() returned %d diagnostics, want 3", len(diagnostics))
	}
	if got, want := []string{
		diagnostics[0].RuleID,
		diagnostics[1].RuleID,
		diagnostics[2].RuleID,
	}, []string{"a-function", "z-call", "z-call"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic order = %#v, want %#v", got, want)
	}
	wantSource := []string{"func run(){later();earlier()}", "later()", "earlier()"}
	for index, diagnostic := range diagnostics {
		if diagnostic.Path != "example.go" || diagnostic.Digest != file.Digest() {
			t.Fatalf("diagnostic source identity = %q/%x", diagnostic.Path, diagnostic.Digest)
		}
		if diagnostic.Severity != rules.SeverityWarn {
			t.Fatalf("diagnostic severity = %q", diagnostic.Severity)
		}
		gotSource, valid := file.Slice(diagnostic.Range)
		if !valid || gotSource != wantSource[index] {
			t.Fatalf("diagnostic %d source = %q, want %q", index, gotSource, wantSource[index])
		}
	}
}

func TestRunSyntaxRoutesTypedOptionsToFileRules(t *testing.T) {
	t.Parallel()

	file, err := source.Load("file.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := analysisMetadata("file-options", rules.NodeFile, false)
	metadata.Options = []rules.OptionMetadata{{
		Name: "enabled", Summary: "enable the rule", Kind: rules.OptionBoolean, Required: true,
	}}
	registry, err := rules.NewRegistry(syntaxFileRule{
		metadata: metadata,
		run: func(ctx *rules.Context) ([]rules.Finding, error) {
			enabled, found := ctx.BooleanOption("enabled")
			if !found || !enabled || ctx.File() != file {
				t.Fatalf("file rule context = %#v, enabled %t, %t", ctx, enabled, found)
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveConfigured(
		rules.PresetCorrectness,
		nil,
		map[string]rules.OptionSet{
			"file-options": rules.NewOptionSet(map[string]rules.OptionValue{
				"enabled": rules.BooleanOption(true),
			}),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSyntax(context.Background(), file, registry, selection); err != nil {
		t.Fatal(err)
	}
}

func TestRunSyntaxUsesTotalDiagnosticOrdering(t *testing.T) {
	t.Parallel()

	file, err := source.Load("ordering.go", []byte("package example\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	nativeRule := syntaxRule{
		metadata: analysisMetadata("ordering-rule", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "same-key", Message: "z message", Range: sourceRange},
				{MessageKey: "same-key", Message: "a message", Range: sourceRange},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(nativeRule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunSyntax(context.Background(), file, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := []string{diagnostics[0].Message, diagnostics[1].Message},
		[]string{"a message", "z message"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic tie order = %#v, want %#v", got, want)
	}
}

func TestRunSyntaxPreservesCancellationObservedDuringNativeRuleRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	file, err := source.Load("canceled.go", []byte("package example\n"))
	if err != nil {
		t.Fatal(err)
	}
	nativeRule := syntaxRule{
		metadata: analysisMetadata("canceling-native", rules.NodeFile, false),
		run: func(ruleContext *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ruleContext.Range(node)
			if err != nil {
				return nil, err
			}
			cancel()
			return []rules.Finding{{
				MessageKey: "canceled", Message: "diagnostic after cancellation", Range: sourceRange,
			}}, nil
		},
	}
	registry, err := rules.NewRegistry(nativeRule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := analysis.RunSyntax(ctx, file, registry, selection); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunSyntax() error = %v, want context cancellation", err)
	}
}

func TestRunSyntaxRejectsAmbiguousOrMisdeclaredExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rule      rules.Rule
		wantError string
	}{
		{
			name: "ambiguous execution",
			rule: ambiguousSyntaxRule{syntaxFileRule{
				metadata: analysisMetadata("ambiguous-execution", rules.NodeFile, false),
			}},
			wantError: "ambiguous syntax execution",
		},
		{
			name: "file rule with node interest",
			rule: syntaxFileRule{
				metadata: analysisMetadata("misdeclared-file", rules.NodeCallExpr, false),
			},
			wantError: "must declare only file interest",
		},
		{
			name: "metadata without execution",
			rule: metadataRuleAdapter{
				metadata: analysisMetadata("missing-execution", rules.NodeCallExpr, false),
			},
			wantError: "does not implement syntax execution",
		},
	}
	file, err := source.Load("execution.go", []byte("package example\nfunc run() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, err := rules.NewRegistry(test.rule)
			if err != nil {
				t.Fatal(err)
			}
			selection, err := registry.Resolve(rules.PresetCorrectness, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := analysis.RunSyntax(context.Background(), file, registry, selection); err == nil ||
				!strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("RunSyntax() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestRunSyntaxPreservesRichFindingSequence(t *testing.T) {
	t.Parallel()

	file, err := source.Load("rich.go", []byte("package example\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := analysisMetadata("rich-finding", rules.NodeCallExpr, false)
	metadata.Fixes = []rules.FixMetadata{{
		Name:        "rewrite",
		Description: "rewrite the call",
		Safety:      rules.FixSafe,
	}}
	nativeRule := syntaxRule{
		metadata: metadata,
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{
				MessageKey: "rich",
				Message:    "rich finding",
				Range:      sourceRange,
				Related: []rules.Related{
					{Range: sourceRange, Message: "z related first"},
					{Range: sourceRange, Message: "a related second"},
				},
				Notes: []string{"z note first", "a note second"},
				Help:  "review the ordered context",
				Fixes: []rules.Fix{{
					Name:   "rewrite",
					Safety: rules.FixSafe,
					Edits: []rules.Edit{
						{Range: sourceRange, NewText: "z replacement first"},
						{Range: sourceRange, NewText: "a replacement second"},
					},
				}},
			}}, nil
		},
	}
	diagnostic := runOneDiagnostic(t, file, nativeRule)
	if got, want := []string{diagnostic.Related[0].Message, diagnostic.Related[1].Message},
		[]string{"z related first", "a related second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("related order = %#v, want %#v", got, want)
	}
	if got, want := diagnostic.Notes, []string{"z note first", "a note second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("note order = %#v, want %#v", got, want)
	}
	if diagnostic.Help != "review the ordered context" {
		t.Fatalf("help = %q", diagnostic.Help)
	}
	if got, want := []string{
		diagnostic.Fixes[0].Edits[0].NewText,
		diagnostic.Fixes[0].Edits[1].NewText,
	}, []string{"z replacement first", "a replacement second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fix edit order = %#v, want %#v", got, want)
	}
}

func TestRunSyntaxRejectsMalformedRichFindings(t *testing.T) {
	t.Parallel()

	file, err := source.Load("malformed.go", []byte("package example\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		finding   rules.Finding
		wantError string
	}{
		{
			name: "invalid related range",
			finding: rules.Finding{
				MessageKey: "related",
				Message:    "invalid related range",
				Range:      source.Range{Start: 0, End: 1},
				Related:    []rules.Related{{Range: source.Range{Start: -1, End: 1}}},
			},
			wantError: "invalid related range",
		},
		{
			name: "undeclared fix",
			finding: rules.Finding{
				MessageKey: "undeclared",
				Message:    "undeclared fix",
				Range:      source.Range{Start: 0, End: 1},
				Fixes:      []rules.Fix{{Name: "missing", Safety: rules.FixSafe}},
			},
			wantError: "uses undeclared fix \"missing\"",
		},
		{
			name: "safety mismatch",
			finding: rules.Finding{
				MessageKey: "safety",
				Message:    "mismatched fix safety",
				Range:      source.Range{Start: 0, End: 1},
				Fixes:      []rules.Fix{{Name: "rewrite", Safety: rules.FixUnsafe}},
			},
			wantError: "safety does not match metadata",
		},
		{
			name: "duplicate fix",
			finding: rules.Finding{
				MessageKey: "duplicate",
				Message:    "duplicate fix",
				Range:      source.Range{Start: 0, End: 1},
				Fixes: []rules.Fix{
					{Name: "rewrite", Safety: rules.FixSafe},
					{Name: "rewrite", Safety: rules.FixSafe},
				},
			},
			wantError: "repeats fix \"rewrite\"",
		},
		{
			name: "invalid edit range",
			finding: rules.Finding{
				MessageKey: "edit",
				Message:    "invalid edit range",
				Range:      source.Range{Start: 0, End: 1},
				Fixes: []rules.Fix{{
					Name:   "rewrite",
					Safety: rules.FixSafe,
					Edits:  []rules.Edit{{Range: source.Range{Start: -1, End: 1}}},
				}},
			},
			wantError: "invalid edit range",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			metadata := analysisMetadata("malformed-finding", rules.NodeCallExpr, false)
			metadata.Fixes = []rules.FixMetadata{{
				Name:        "rewrite",
				Description: "rewrite the call",
				Safety:      rules.FixSafe,
			}}
			nativeRule := syntaxRule{
				metadata: metadata,
				run: func(*rules.Context, ast.Node) ([]rules.Finding, error) {
					return []rules.Finding{test.finding}, nil
				},
			}
			registry, err := rules.NewRegistry(nativeRule)
			if err != nil {
				t.Fatal(err)
			}
			selection, err := registry.Resolve(rules.PresetCorrectness, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := analysis.RunSyntax(context.Background(), file, registry, selection); err == nil ||
				!strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("RunSyntax() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func runOneDiagnostic(t *testing.T, file *source.File, nativeRule rules.Rule) rules.Diagnostic {
	t.Helper()
	registry, err := rules.NewRegistry(nativeRule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunSyntax(context.Background(), file, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("RunSyntax() returned %d diagnostics, want 1", len(diagnostics))
	}
	return diagnostics[0]
}

func TestRunSyntaxSkipsGeneratedFilesUnlessRuleOptsIn(t *testing.T) {
	t.Parallel()

	file, err := source.Load("generated.go", []byte("// Code generated by test. DO NOT EDIT.\npackage generated\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	var visits []string
	newRule := func(id string, generated bool) syntaxRule {
		return syntaxRule{
			metadata: analysisMetadata(id, rules.NodeCallExpr, generated),
			visits:   &visits,
			run: func(*rules.Context, ast.Node) ([]rules.Finding, error) {
				return nil, nil
			},
		}
	}
	registry, err := rules.NewRegistry(
		newRule("generated-disabled", false),
		newRule("generated-enabled", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSyntax(context.Background(), file, registry, selection); err != nil {
		t.Fatal(err)
	}
	if got, want := visits, []string{"generated-enabled"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("generated-file visits = %#v, want %#v", got, want)
	}
}

func TestRunSyntaxRejectsInvalidFindingsAndUnsupportedTiers(t *testing.T) {
	t.Parallel()

	file, err := source.Load("invalid_finding.go", []byte("package example\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	invalid := syntaxRule{
		metadata: analysisMetadata("invalid-finding", rules.NodeCallExpr, false),
		run: func(*rules.Context, ast.Node) ([]rules.Finding, error) {
			return []rules.Finding{{
				MessageKey: "invalid",
				Message:    "invalid range",
				Range:      source.Range{Start: -1, End: 1},
			}}, nil
		},
	}
	registry, err := rules.NewRegistry(invalid)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSyntax(context.Background(), file, registry, selection); err == nil ||
		!strings.Contains(err.Error(), "invalid-finding: finding has invalid primary range") {
		t.Fatalf("RunSyntax() invalid finding error = %v", err)
	}

	typedMetadata := analysisMetadata("typed-rule", rules.NodeCallExpr, false)
	typedMetadata.Requirement = rules.RequireTypes
	typedRegistry, err := rules.NewRegistry(metadataRuleAdapter{metadata: typedMetadata})
	if err != nil {
		t.Fatal(err)
	}
	typedSelection, err := typedRegistry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSyntax(context.Background(), file, typedRegistry, typedSelection); err == nil ||
		!strings.Contains(err.Error(), "requires types") {
		t.Fatalf("RunSyntax() typed tier error = %v", err)
	}

	invalidSeverity := slices.Clone(selection)
	invalidSeverity[0].Severity = "fatal"
	if _, err := analysis.RunSyntax(context.Background(), file, registry, invalidSeverity); err == nil ||
		!strings.Contains(err.Error(), "invalid severity") {
		t.Fatalf("RunSyntax() invalid severity error = %v", err)
	}
}

type metadataRuleAdapter struct {
	metadata rules.Metadata
}

func (r metadataRuleAdapter) Metadata() rules.Metadata { return r.metadata }

func analysisMetadata(id string, interest rules.NodeKind, generated bool) rules.Metadata {
	return rules.Metadata{
		ID:               id,
		Summary:          "reports syntax",
		Documentation:    "Full syntax rule documentation.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSyntax,
		NodeInterests:    []rules.NodeKind{interest},
		RunOnGenerated:   generated,
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{{
			Incorrect: "bad()",
			Correct:   "good()",
		}},
	}
}
