package analysis_test

import (
	"context"
	"errors"
	"flag"
	"go/ast"
	"reflect"
	"strings"
	"testing"

	goanalysis "golang.org/x/tools/go/analysis"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type adapterFact struct{}

func (*adapterFact) AFact() {}

func TestAdaptAnalyzerRunsOnAnIsolatedSyntaxViewAndMapsDiagnostics(t *testing.T) {
	t.Parallel()

	input := `package sample

//gox:ignore external-call -- accepted here
func suppressed() { target() }

func visible() { target() }
`
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	upstream := &goanalysis.Analyzer{
		Name: "externalcall",
		Doc:  "reports target calls",
		URL:  "https://example.test/external-call",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Pkg == nil || pass.Pkg.Name() != "sample" || pass.TypesInfo != nil ||
				pass.TypesSizes != nil || len(pass.Files) != 1 || len(pass.ResultOf) != 0 {
				return nil, errors.New("adapter pass exposed an invalid syntax-only package")
			}
			path := pass.Fset.PositionFor(pass.Files[0].Pos(), false).Filename
			contents, err := pass.ReadFile(path)
			if err != nil || string(contents) != input {
				return nil, errors.New("adapter pass did not expose exact source bytes")
			}
			contents[0] = 'X'
			contents, err = pass.ReadFile(path)
			if err != nil || string(contents) != input {
				return nil, errors.New("adapter pass exposed mutable source bytes")
			}
			if _, err := pass.ReadFile("/project/other.go"); err == nil {
				return nil, errors.New("adapter pass read an undeclared file")
			}
			calls := make([]*ast.CallExpr, 0, 2)
			ast.Inspect(pass.Files[0], func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if ok {
					calls = append(calls, call)
				}
				return true
			})
			for index := len(calls) - 1; index >= 0; index-- {
				call := calls[index]
				pass.Report(goanalysis.Diagnostic{
					Pos:      call.Pos(),
					End:      call.End(),
					Category: "target-call",
					Message:  "target call requires review",
					Related: []goanalysis.RelatedInformation{{
						Pos:     call.Fun.Pos(),
						End:     call.Fun.End(),
						Message: "called function",
					}},
					SuggestedFixes: []goanalysis.SuggestedFix{{
						Message: "Replace target call",
						TextEdits: []goanalysis.TextEdit{{
							Pos:     call.Pos(),
							End:     call.End(),
							NewText: []byte("primary()"),
						}},
					}},
				})
			}
			return nil, nil
		},
	}
	metadata := analysisMetadata("external-call", rules.NodeFile, false)
	adapted, err := analysis.AdaptAnalyzer(upstream, analysis.AnalyzerAdapterOptions{
		Metadata: metadata,
		SuggestedFixes: []analysis.AnalyzerFixMapping{{
			Message:     "Replace target call",
			Name:        "replace-target",
			Description: "replace the target call",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Overrides: map[string]rules.Severity{
			"external-call": rules.SeverityError,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Suppressed) != 1 || len(result.Diagnostics) != 1 {
		t.Fatalf("adapter diagnostics = visible %#v, suppressed %#v", result.Diagnostics, result.Suppressed)
	}
	diagnostic := result.Diagnostics[0]
	visibleStart := strings.LastIndex(input, "target()")
	if diagnostic.RuleID != "external-call" || diagnostic.Severity != rules.SeverityError ||
		diagnostic.MessageKey != "target-call" || diagnostic.Range.Start != visibleStart ||
		len(diagnostic.Related) != 1 || diagnostic.Related[0].Message != "called function" ||
		diagnostic.Help != "https://example.test/external-call#target-call" ||
		len(diagnostic.Fixes) != 1 || diagnostic.Fixes[0].Name != "replace-target" ||
		diagnostic.Fixes[0].Safety != rules.FixSuggestion ||
		len(diagnostic.Fixes[0].Edits) != 1 || diagnostic.Fixes[0].Edits[0].NewText != "primary()" {
		t.Fatalf("adapter diagnostic = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerMapsExplicitlyAuditedSafeFixes(t *testing.T) {
	t.Parallel()

	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "safe",
		Doc:  "offers an audited safe fix",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.Report(goanalysis.Diagnostic{
				Pos:     pass.Files[0].Name.Pos(),
				End:     pass.Files[0].Name.End(),
				Message: "package name spelling can be preserved",
				SuggestedFixes: []goanalysis.SuggestedFix{{
					Message: "Preserve package name",
					TextEdits: []goanalysis.TextEdit{{
						Pos: pass.Files[0].Name.Pos(), End: pass.Files[0].Name.End(), NewText: []byte("sample"),
					}},
				}},
			})
			return nil, nil
		},
	}, analysis.AnalyzerAdapterOptions{
		Metadata: analysisMetadata("safe-analyzer", rules.NodeFile, false),
		SuggestedFixes: []analysis.AnalyzerFixMapping{{
			Message: "Preserve package name", Name: "preserve-package",
			Description: "preserve the package name spelling", Safety: rules.FixSafe, Audited: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := adapted.Metadata()
	if len(metadata.Fixes) != 1 || metadata.Fixes[0].Safety != rules.FixSafe {
		t.Fatalf("adapted fix metadata = %#v", metadata.Fixes)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || len(result.Diagnostics[0].Fixes) != 1 ||
		result.Diagnostics[0].Fixes[0].Safety != rules.FixSafe {
		t.Fatalf("adapted diagnostics = %#v", result.Diagnostics)
	}
}

func TestAdaptAnalyzerIsolatesAnalyzerMutationsFromOtherAdapters(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\nfunc run() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	mutator, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "mutator",
		Doc:  "mutates its isolated syntax view",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.Files[0].Name.Name = "changed"
			return nil, nil
		},
	}, analysis.AnalyzerAdapterOptions{Metadata: analysisMetadata("a-mutator", rules.NodeFile, false)})
	if err != nil {
		t.Fatal(err)
	}
	observer, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "observer",
		Doc:  "observes the original isolated syntax view",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Files[0].Name.Name != "sample" {
				return nil, errors.New("another analyzer mutated the syntax view")
			}
			pass.ReportRangef(pass.Files[0].Name, "package name remained isolated")
			return nil, nil
		},
	}, analysis.AnalyzerAdapterOptions{Metadata: analysisMetadata("b-observer", rules.NodeFile, false)})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(observer, mutator)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "b-observer" {
		t.Fatalf("isolated adapter diagnostics = %#v", result.Diagnostics)
	}
}

func TestAdaptAnalyzerIsolatesAnalyzerDescriptorMutationsBetweenRuns(t *testing.T) {
	t.Parallel()

	runs := 0
	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "descriptor",
		Doc:  "mutates the pass analyzer descriptor",
		Run: func(pass *goanalysis.Pass) (any, error) {
			runs++
			pass.Analyzer.Run = func(*goanalysis.Pass) (any, error) {
				return nil, errors.New("mutated analyzer descriptor escaped its run")
			}
			return nil, nil
		},
	}, adapterOptions("descriptor-analyzer"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
			Preset: rules.PresetCorrectness,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if runs != 2 {
		t.Fatalf("original analyzer runs = %d, want 2", runs)
	}
}

func TestAdaptAnalyzerHonorsGeneratedFilePolicy(t *testing.T) {
	t.Parallel()

	var skippedRuns, enabledRuns int
	newAdapter := func(id string, generated bool, runs *int) rules.Rule {
		adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
			Name: strings.ReplaceAll(id, "-", ""),
			Doc:  "records generated-file scheduling",
			Run: func(*goanalysis.Pass) (any, error) {
				(*runs)++
				return nil, nil
			},
		}, analysis.AnalyzerAdapterOptions{
			Metadata: analysisMetadata(id, rules.NodeFile, generated),
		})
		if err != nil {
			t.Fatal(err)
		}
		return adapted
	}
	registry, err := rules.NewRegistry(
		newAdapter("generated-disabled", false, &skippedRuns),
		newAdapter("generated-enabled", true, &enabledRuns),
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load(
		"/project/generated.go",
		[]byte("// Code generated by test. DO NOT EDIT.\npackage generated\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	}); err != nil {
		t.Fatal(err)
	}
	if skippedRuns != 0 || enabledRuns != 1 {
		t.Fatalf("generated analyzer runs = disabled %d, enabled %d", skippedRuns, enabledRuns)
	}
}

func TestAdaptAnalyzerDiagnosticsShareNativeDeterministicOrdering(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\nfunc run() { target() }\n"))
	if err != nil {
		t.Fatal(err)
	}
	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "adapted",
		Doc:  "reports the target call",
		Run: func(pass *goanalysis.Pass) (any, error) {
			ast.Inspect(pass.Files[0], func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					pass.ReportRangef(call, "adapted diagnostic")
				}
				return true
			})
			return nil, nil
		},
	}, adapterOptions("z-adapted"))
	if err != nil {
		t.Fatal(err)
	}
	native := syntaxRule{
		metadata: analysisMetadata("a-native", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{
				MessageKey: "native", Message: "native diagnostic", Range: sourceRange,
			}}, nil
		},
	}
	registry, err := rules.NewRegistry(adapted, native)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("mixed diagnostics = %#v", result.Diagnostics)
	}
	if got, want := []string{result.Diagnostics[0].RuleID, result.Diagnostics[1].RuleID},
		[]string{"a-native", "z-adapted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed diagnostic order = %#v, want %#v", got, want)
	}
}

func TestAdaptAnalyzerPreservesCancellationObservedDuringAnalyzerRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "canceling",
		Doc:  "observes cancellation after analyzer execution",
		Run: func(pass *goanalysis.Pass) (any, error) {
			cancel()
			pass.ReportRangef(pass.Files[0].Name, "diagnostic after cancellation")
			return nil, nil
		},
	}, adapterOptions("canceling-analyzer"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := analysis.Run(ctx, file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
}

func TestAdaptAnalyzerRejectsUnsupportedAnalyzerContracts(t *testing.T) {
	t.Parallel()

	validRun := func(*goanalysis.Pass) (any, error) { return nil, nil }
	prerequisite := &goanalysis.Analyzer{Name: "prerequisite", Doc: "prerequisite", Run: validRun}
	flagged := &goanalysis.Analyzer{Name: "flagged", Doc: "flagged", Run: validRun}
	flagged.Flags.Init("flagged", flag.ContinueOnError)
	flagged.Flags.Bool("enabled", false, "enable analysis")
	tests := []struct {
		name      string
		analyzer  *goanalysis.Analyzer
		options   analysis.AnalyzerAdapterOptions
		wantError string
	}{
		{name: "nil", analyzer: nil, options: adapterOptions("nil-analyzer"), wantError: "nil analyzer"},
		{
			name: "prerequisites",
			analyzer: &goanalysis.Analyzer{
				Name: "requires", Doc: "requires another analyzer", Run: validRun,
				Requires: []*goanalysis.Analyzer{prerequisite},
			},
			options: adapterOptions("requires-analyzer"), wantError: "prerequisite",
		},
		{
			name: "facts",
			analyzer: &goanalysis.Analyzer{
				Name: "facts", Doc: "uses facts", Run: validRun, FactTypes: []goanalysis.Fact{new(adapterFact)},
			},
			options: adapterOptions("facts-analyzer"), wantError: "facts",
		},
		{
			name: "result",
			analyzer: &goanalysis.Analyzer{
				Name: "result", Doc: "returns a result", Run: validRun, ResultType: reflect.TypeFor[string](),
			},
			options: adapterOptions("result-analyzer"), wantError: "result",
		},
		{name: "flags", analyzer: flagged, options: adapterOptions("flagged-analyzer"), wantError: "flags"},
		{
			name:     "predeclared native fixes",
			analyzer: &goanalysis.Analyzer{Name: "fixes", Doc: "declares fixes twice", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("fix-metadata")
				options.Metadata.Fixes = []rules.FixMetadata{{
					Name: "rewrite", Description: "rewrite source", Safety: rules.FixSuggestion,
				}}
				return options
			}(),
			wantError: "fix metadata",
		},
		{
			name:     "unaudited safe fix",
			analyzer: &goanalysis.Analyzer{Name: "safe", Doc: "offers a safe fix", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("safe-analyzer")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
					Message: "Rewrite", Name: "rewrite", Description: "rewrite source", Safety: rules.FixSafe,
				}}
				return options
			}(),
			wantError: "audit",
		},
		{
			name:     "audit on suggestion",
			analyzer: &goanalysis.Analyzer{Name: "suggestion", Doc: "offers a suggestion", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("suggestion-analyzer")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
					Message: "Rewrite", Name: "rewrite", Description: "rewrite source", Audited: true,
				}}
				return options
			}(),
			wantError: "audit applies only",
		},
		{
			name:     "incomplete fix mapping",
			analyzer: &goanalysis.Analyzer{Name: "incomplete", Doc: "offers an incomplete fix", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("incomplete-analyzer")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{Message: "Rewrite"}}
				return options
			}(),
			wantError: "incomplete",
		},
		{
			name:     "duplicate fix message",
			analyzer: &goanalysis.Analyzer{Name: "duplicatemessage", Doc: "duplicates a fix message", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("duplicate-message")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{
					{Message: "Rewrite", Name: "rewrite-one", Description: "rewrite source one"},
					{Message: "Rewrite", Name: "rewrite-two", Description: "rewrite source two"},
				}
				return options
			}(),
			wantError: "duplicate suggested-fix message",
		},
		{
			name:     "duplicate native fix name",
			analyzer: &goanalysis.Analyzer{Name: "duplicatename", Doc: "duplicates a native fix name", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("duplicate-name")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{
					{Message: "Rewrite one", Name: "rewrite", Description: "rewrite source one"},
					{Message: "Rewrite two", Name: "rewrite", Description: "rewrite source two"},
				}
				return options
			}(),
			wantError: "duplicate native fix name",
		},
		{
			name:     "invalid fix safety",
			analyzer: &goanalysis.Analyzer{Name: "invalidsafety", Doc: "offers an invalid fix safety", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("invalid-safety")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
					Message: "Rewrite", Name: "rewrite", Description: "rewrite source", Safety: "trusted",
				}}
				return options
			}(),
			wantError: "invalid fix safety",
		},
		{
			name:     "typed metadata",
			analyzer: &goanalysis.Analyzer{Name: "typed", Doc: "declares typed metadata", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("typed-analyzer")
				options.Metadata.Requirement = rules.RequireTypes
				return options
			}(),
			wantError: "syntax requirement and only file interest",
		},
		{
			name:     "node metadata",
			analyzer: &goanalysis.Analyzer{Name: "node", Doc: "declares node metadata", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("node-analyzer")
				options.Metadata.NodeInterests = []rules.NodeKind{rules.NodeCallExpr}
				return options
			}(),
			wantError: "syntax requirement and only file interest",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := analysis.AdaptAnalyzer(test.analyzer, test.options); err == nil ||
				!strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("AdaptAnalyzer() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestAdaptAnalyzerRejectsUnsupportedRuntimeResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		analyzerURL string
		run         func(*goanalysis.Pass) (any, error)
		wantError   string
	}{
		{
			name: "panic",
			run: func(*goanalysis.Pass) (any, error) {
				panic("boom")
			},
			wantError: "panicked",
		},
		{
			name: "undeclared suggested fix",
			run: func(pass *goanalysis.Pass) (any, error) {
				pass.Report(goanalysis.Diagnostic{
					Pos: pass.Files[0].Name.Pos(), Message: "problem",
					SuggestedFixes: []goanalysis.SuggestedFix{{Message: "Unknown fix"}},
				})
				return nil, nil
			},
			wantError: "undeclared suggested fix",
		},
		{
			name: "foreign position",
			run: func(pass *goanalysis.Pass) (any, error) {
				other := pass.Fset.AddFile("other.go", -1, 4)
				other.SetLinesForContent([]byte("bad\n"))
				pass.Reportf(other.Pos(0), "foreign diagnostic")
				return nil, nil
			},
			wantError: "outside the adapted source",
		},
		{
			name: "unexpected result",
			run: func(*goanalysis.Pass) (any, error) {
				return "result", nil
			},
			wantError: "unexpected result",
		},
		{
			name:        "invalid analyzer URL",
			analyzerURL: ":not a URL",
			run: func(pass *goanalysis.Pass) (any, error) {
				pass.Report(goanalysis.Diagnostic{
					Pos: pass.Files[0].Name.Pos(), Message: "problem", URL: "#relative",
				})
				return nil, nil
			},
			wantError: "invalid analyzer URL",
		},
		{
			name:        "invalid diagnostic URL",
			analyzerURL: "https://example.test/analyzer",
			run: func(pass *goanalysis.Pass) (any, error) {
				pass.Report(goanalysis.Diagnostic{
					Pos: pass.Files[0].Name.Pos(), Message: "problem", URL: ":not a URL",
				})
				return nil, nil
			},
			wantError: "invalid diagnostic URL",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
				Name: "runtime", Doc: "exercises runtime validation", URL: test.analyzerURL, Run: test.run,
			}, adapterOptions("runtime-analyzer"))
			if err != nil {
				t.Fatal(err)
			}
			registry, err := rules.NewRegistry(adapted)
			if err != nil {
				t.Fatal(err)
			}
			file, err := source.Load("/project/source.go", []byte("package sample\n"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
				Preset: rules.PresetCorrectness,
			}); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func adapterOptions(id string) analysis.AnalyzerAdapterOptions {
	return analysis.AnalyzerAdapterOptions{Metadata: analysisMetadata(id, rules.NodeFile, false)}
}
