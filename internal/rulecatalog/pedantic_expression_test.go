package rulecatalog_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestUnnecessaryConversionReportsIdentityConversions(t *testing.T) {
	t.Parallel()

	input := `package sample

type Name string

func convert(string) string { return "" }

func run(text string, name Name, bytes []byte, number int) {
	_ = string(text)
	_ = Name(name)
	_ = []byte(bytes)
	_ = string(name)
	_ = int64(number)
	_ = int(1)
	_ = convert(text)
}
`
	result := runOnePedanticRule(t, "unnecessary-conversion", input)
	want := []string{"string(text)", "Name(name)", "[]byte(bytes)"}
	assertRuleRanges(t, input, result, "unnecessary-conversion", "identity-conversion", want)
}

func TestUnnecessarySprintfReportsDirectStringRepresentations(t *testing.T) {
	t.Parallel()

	input := `package sample

import "fmt"

type Name string

func run(text string, name Name, bytes []byte, value any, format string) {
	_ = fmt.Sprintf("%s", text)
	_ = fmt.Sprintf("%s", name)
	_ = fmt.Sprintf("%s", bytes)
	_ = fmt.Sprintf("%s", value)
	_ = fmt.Sprintf(format, text)
	_ = fmt.Sprintf("%q", text)
	_ = fmt.Sprintf("%s %s", text, text)
}
`
	result := runOnePedanticRule(t, "unnecessary-sprintf", input)
	want := []string{
		`fmt.Sprintf("%s", text)`,
		`fmt.Sprintf("%s", name)`,
		`fmt.Sprintf("%s", bytes)`,
	}
	assertRuleRanges(
		t,
		input,
		result,
		"unnecessary-sprintf",
		"direct-string-representation",
		want,
	)
}

func TestPedanticExpressionMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]rules.NodeKind{
		"unnecessary-conversion": {rules.NodeCallExpr},
		"unnecessary-sprintf": {rules.NodeCallExpr},
	}
	for id, interests := range want {
		metadata, found := registry.Metadata(id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(
				metadata.Presets,
				[]rules.Preset{rules.PresetPedantic},
			) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != rules.RequireTypes ||
			!reflect.DeepEqual(metadata.NodeInterests, interests) ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors ||
			len(metadata.Fixes) != 0 {
			t.Fatalf("%s metadata = %#v, found = %v", id, metadata, found)
		}
	}
}

func BenchmarkUnnecessaryConversionPackageAnalysis(b *testing.B) {
	benchmarkPedanticExpressionRule(
		b,
		"unnecessary-conversion",
		"package sample\nfunc run(text string) { _ = string(text) }\n",
	)
}

func BenchmarkUnnecessarySprintfPackageAnalysis(b *testing.B) {
	benchmarkPedanticExpressionRule(
		b,
		"unnecessary-sprintf",
		"package sample\nimport \"fmt\"\nfunc run(text string) { _ = fmt.Sprintf(\"%s\", text) }\n",
	)
}

func benchmarkPedanticExpressionRule(b *testing.B, ruleID, input string) {
	b.Helper()
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/pedanticexpressionbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(b, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	benchmarkPackageRuns(
		b,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		1,
	)
}

func runOnePedanticRule(t *testing.T, ruleID, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/pedanticexpression\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertRuleRanges(
	t *testing.T,
	input string,
	result analysis.PackageResult,
	ruleID string,
	messageKey string,
	want []string,
) {
	t.Helper()
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("%s result = %#v", ruleID, result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing diagnostic text %q", want[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != ruleID ||
			diagnostic.MessageKey != messageKey ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("%s diagnostic[%d] = %#v", ruleID, index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}
