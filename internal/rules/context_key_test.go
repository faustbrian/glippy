package rules_test

import (
	"context"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type benchmarkContextKeyTypesRule struct{}

func (benchmarkContextKeyTypesRule) Metadata() rules.Metadata {
	return rules.Metadata{
		ID: "benchmark-context-key-types", Summary: "provides a types traversal baseline",
		Documentation:    "Provides a no-op typed call traversal for benchmark comparison.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetSuspicious},
		MinimumGoVersion: "1.26",
		Requirement:      rules.RequireTypes,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}
}

func (benchmarkContextKeyTypesRule) RunTypes(*rules.TypesContext, ast.Node) ([]rules.Finding, error) {
	return nil, nil
}

func TestContextKeyReportsUnsafeContextWithValueKeysAtExactRanges(t *testing.T) {
	t.Parallel()

	input := `package sample
import c "context"
type keyAlias = string
type badStruct struct { values []int }
func run(ctx c.Context) {
	c.WithValue(ctx, "text", 1)
	c.WithValue(ctx, keyAlias("alias"), 1)
	c.WithValue(ctx, []byte(nil), 1)
	c.WithValue(ctx, badStruct{}, 1)
	c.WithValue(ctx, struct{}{}, 1)
	c.WithValue(ctx, nil, 1)
	(c.WithValue)(ctx, "parenthesized", 1)
}
func genericSlice[P ~[]byte](ctx c.Context, key P) {
	c.WithValue(ctx, (key), 1)
}
`
	result := runContextKey(t, input, nil)
	want := []struct {
		expression string
		messageKey string
	}{
		{expression: `"text"`, messageKey: "built-in"},
		{expression: `keyAlias("alias")`, messageKey: "built-in"},
		{expression: `[]byte(nil)`, messageKey: "not-comparable"},
		{expression: `badStruct{}`, messageKey: "not-comparable"},
		{expression: `struct{}{}`, messageKey: "empty-struct"},
		{expression: `nil`, messageKey: "nil"},
		{expression: `"parenthesized"`, messageKey: "built-in"},
		{expression: `(key)`, messageKey: "not-comparable"},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("result = %#v, want %d context-key diagnostics", result, len(want))
	}
	searchStart := 0
	for index, expected := range want {
		offset := strings.Index(input[searchStart:], expected.expression)
		if offset < 0 {
			t.Fatalf("input does not contain %q after %d", expected.expression, searchStart)
		}
		offset += searchStart
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "context-key" || diagnostic.MessageKey != expected.messageKey ||
			diagnostic.Range != (source.Range{Start: offset, End: offset + len(expected.expression)}) ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
		searchStart = offset + len(expected.expression)
	}
}

func TestContextKeyAcceptsDefinedComparableAndUnresolvedDynamicKeys(t *testing.T) {
	t.Parallel()

	input := `package sample
import "context"
type stringKey string
type emptyKey struct{}
type alias = stringKey
type localContext struct{}
func (localContext) WithValue(context.Context, any, any) {}
func WithValue(context.Context, any, any) {}
func generic[P any](ctx context.Context, key P) {
	context.WithValue(ctx, key, 1)
}
func genericMixed[P interface{ ~string | ~[]byte }](ctx context.Context, key P) {
	context.WithValue(ctx, key, 1)
}
func genericImpossible[P interface{ ~[]byte; comparable }](ctx context.Context, key P) {
	context.WithValue(ctx, key, 1)
}
func run(ctx context.Context, dynamic any, named stringKey) {
	context.WithValue(ctx, named, 1)
	context.WithValue(ctx, alias("key"), 1)
	context.WithValue(ctx, emptyKey{}, 1)
	context.WithValue(ctx, struct{ key string }{}, 1)
	context.WithValue(ctx, &struct{}{}, 1)
	context.WithValue(ctx, dynamic, 1)
	WithValue(ctx, "local", 1)
	localContext{}.WithValue(ctx, "selector", 1)
}
`
	result := runContextKey(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func BenchmarkContextKeySharedTypes(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/contextkeybenchmark\n\ngo 1.26.0\n"),
		0o600,
	); err != nil {
		b.Fatal(err)
	}
	var input strings.Builder
	input.WriteString("package sample\nimport \"context\"\nfunc attach(ctx context.Context) {\n")
	for index := range 100 {
		fmt.Fprintf(&input, "context.WithValue(ctx, \"key-%d\", %d)\n", index, index)
	}
	input.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(input.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	loaded, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir: root, Patterns: []string{"."}, Requirement: rules.RequireTypes,
	})
	if err != nil {
		b.Fatal(err)
	}
	contextRegistry, err := rules.NewDefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	contextSelection, err := contextRegistry.Resolve(
		rules.PresetSuspicious,
		map[string]rules.Severity{
			"defer-in-infinite-loop": rules.SeverityOff,
			"errors-is-arguments":    rules.SeverityOff,
			"nilness":                rules.SeverityOff,
		},
	)
	if err != nil {
		b.Fatal(err)
	}
	baselineRegistry, err := rules.NewRegistry(benchmarkContextKeyTypesRule{})
	if err != nil {
		b.Fatal(err)
	}
	baselineSelection, err := baselineRegistry.Resolve(rules.PresetSuspicious, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("baseline", func(b *testing.B) {
		benchmarkContextKeyExecution(b, loaded, baselineRegistry, baselineSelection, 0)
	})
	b.Run("context-key", func(b *testing.B) {
		benchmarkContextKeyExecution(b, loaded, contextRegistry, contextSelection, 100)
	})
}

func benchmarkContextKeyExecution(
	b *testing.B,
	loaded analysis.PackageLoadResult,
	registry *rules.Registry,
	selection []rules.Selection,
	wantDiagnostics int,
) {
	b.Helper()
	b.ReportAllocs()
	for range b.N {
		diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
		if err != nil {
			b.Fatal(err)
		}
		if len(diagnostics) != wantDiagnostics {
			b.Fatalf("diagnostics = %d, want %d", len(diagnostics), wantDiagnostics)
		}
	}
}

func TestContextKeyHonorsSuppressionGeneratedTypeErrorAndSeverityPolicies(t *testing.T) {
	t.Parallel()

	suppressed := runContextKey(t, `package sample
import "context"
func run(ctx context.Context) {
	//gox:ignore context-key -- compatibility boundary
	context.WithValue(ctx, "key", 1)
}
`, nil)
	if len(suppressed.Files) != 1 || len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 ||
		suppressed.Files[0].Suppressed[0].Diagnostic.RuleID != "context-key" {
		t.Fatalf("suppressed result = %#v", suppressed)
	}

	generated := runContextKey(t, `// Code generated by fixture. DO NOT EDIT.
package sample
import "context"
func run(ctx context.Context) { context.WithValue(ctx, "key", 1) }
`, nil)
	if len(generated.Files) != 1 || len(generated.Files[0].Diagnostics) != 0 {
		t.Fatalf("generated result = %#v", generated)
	}

	illTyped := runContextKey(t, `package sample
import "context"
func run(ctx context.Context) { missing(); context.WithValue(ctx, "key", 1) }
`, nil)
	if len(illTyped.LoadDiagnostics) == 0 || len(illTyped.Files) != 1 ||
		len(illTyped.Files[0].Diagnostics) != 0 {
		t.Fatalf("ill-typed result = %#v", illTyped)
	}

	severity := runContextKey(t, `package sample
import "context"
func run(ctx context.Context) { context.WithValue(ctx, "key", 1) }
`, map[string]rules.Severity{"context-key": rules.SeverityError})
	if len(severity.Files) != 1 || len(severity.Files[0].Diagnostics) != 1 ||
		severity.Files[0].Diagnostics[0].Severity != rules.SeverityError {
		t.Fatalf("severity result = %#v", severity)
	}
}

func TestDefaultRegistryDocumentsContextKeyWithoutAFix(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("context-key")
	if !found || metadata.Requirement != rules.RequireTypes ||
		metadata.DefaultSeverity != rules.SeverityWarn || metadata.MinimumGoVersion != "1.25" ||
		metadata.RunOnGenerated || metadata.RunDespiteTypeErrors || len(metadata.Fixes) != 0 ||
		len(metadata.Presets) != 1 || metadata.Presets[0] != rules.PresetSuspicious ||
		len(metadata.Examples) == 0 || len(metadata.KnownLimitations) == 0 {
		t.Fatalf("metadata = %#v, found = %v", metadata, found)
	}
}

func runContextKey(
	t *testing.T,
	input string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/contextkey\n\ngo 1.26.0\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if overrides == nil {
		overrides = make(map[string]rules.Severity)
	}
	overrides["nilness"] = rules.SeverityOff
	overrides["defer-in-infinite-loop"] = rules.SeverityOff
	overrides["errors-is-arguments"] = rules.SeverityOff
	if _, configured := overrides["context-key"]; !configured {
		overrides["context-key"] = rules.SeverityWarn
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetSuspicious, Overrides: overrides},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
