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
			metadata.RunDespiteTypeErrors {
			t.Fatalf("%s metadata = %#v, found = %v", id, metadata, found)
		}
	}
	conversion, _ := registry.Metadata("unnecessary-conversion")
	if !reflect.DeepEqual(
		conversion.Fixes,
		[]rules.FixMetadata{
			{
				Name: "remove-unnecessary-conversion",
				Description: "replace the identity conversion with its value",
				Safety: rules.FixSuggestion,
			},
		},
	) {
		t.Fatalf("unnecessary-conversion fixes = %#v", conversion.Fixes)
	}
	sprintf, _ := registry.Metadata("unnecessary-sprintf")
	if !reflect.DeepEqual(
		sprintf.Fixes,
		[]rules.FixMetadata{
			{
				Name: "replace-unnecessary-sprintf",
				Description: "replace fmt.Sprintf with the direct string representation",
				Safety: rules.FixSuggestion,
			},
		},
	) {
		t.Fatalf("unnecessary-sprintf fixes = %#v", sprintf.Fixes)
	}
}

func TestPedanticExpressionSuggestionsPreserveResultTypes(t *testing.T) {
	t.Parallel()

	input := `package sample

import "fmt"

type Name string

func run(text string, name Name, bytes []byte) {
	_ = string(text)
	_ = []byte(bytes)
	_ = string(text + text)
	_ = fmt.Sprintf("%s", text)
	_ = fmt.Sprintf("%s", name)
	_ = fmt.Sprintf("%s", bytes)
}
`
	tests := []struct {
		id string
		fix string
		replacements []string
	}{
		{
			"unnecessary-conversion",
			"remove-unnecessary-conversion",
			[]string{"text", "bytes", "(text + text)"},
		},
		{
			"unnecessary-sprintf",
			"replace-unnecessary-sprintf",
			[]string{"text", "string(name)", "string(bytes)"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.id,
			func(t *testing.T) {
				t.Parallel()
				result := runOnePedanticRule(t, test.id, input)
				if len(result.Files) != 1 ||
					len(result.Files[0].Diagnostics) != len(test.replacements) {
					t.Fatalf("%s result = %#v", test.id, result)
				}
				for index, diagnostic := range result.Files[0].Diagnostics {
					if len(diagnostic.Fixes) != 1 ||
						diagnostic.Fixes[0].Name != test.fix ||
						diagnostic.Fixes[0].Safety != rules.FixSuggestion ||
						len(diagnostic.Fixes[0].Edits) != 1 ||
						diagnostic.Fixes[0].Edits[0].NewText !=
							test.replacements[index] {
						t.Fatalf(
							"%s diagnostic[%d] fixes = %#v",
							test.id,
							index,
							diagnostic.Fixes,
						)
					}
				}
			},
		)
	}
}

func TestUnnecessarySprintfPreservesCustomStringFormatting(t *testing.T) {
	t.Parallel()

	input := `package sample

import "fmt"

type FormatterValue string
func (FormatterValue) Format(fmt.State, rune) {}

type StringValue string
func (StringValue) String() string { return "formatted" }

type ErrorValue string
func (ErrorValue) Error() string { return "failed" }

func run(plain string, formatter FormatterValue, stringer StringValue, err ErrorValue) {
	_ = fmt.Sprintf("%s", plain)
	_ = fmt.Sprintf("%s", formatter)
	_ = fmt.Sprintf("%s", stringer)
	_ = fmt.Sprintf("%s", err)
}
`
	result := runOnePedanticRule(t, "unnecessary-sprintf", input)
	assertRuleRanges(
		t,
		input,
		result,
		"unnecessary-sprintf",
		"direct-string-representation",
		[]string{`fmt.Sprintf("%s", plain)`},
	)
}

func TestPedanticExpressionSuggestionsPreserveComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id string
		input string
	}{
		{
			"unnecessary-conversion",
			"package sample\nfunc run(text string) { _ = string(/* keep */ text) }\n",
		},
		{
			"unnecessary-sprintf",
			"package sample\nimport \"fmt\"\nfunc run(text string) { _ = fmt.Sprintf(\"%s\", /* keep */ text) }\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.id,
			func(t *testing.T) {
				t.Parallel()
				result := runOnePedanticRule(t, test.id, test.input)
				if len(result.Files) != 1 ||
					len(result.Files[0].Diagnostics) != 1 ||
					len(result.Files[0].Diagnostics[0].Fixes) != 0 {
					t.Fatalf("%s comment result = %#v", test.id, result)
				}
			},
		)
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
	if len(result.LoadDiagnostics) != 0 || len(result.SourceProblems) != 0 {
		t.Fatalf(
			"%s package load = diagnostics %#v, source problems %#v",
			ruleID,
			result.LoadDiagnostics,
			result.SourceProblems,
		)
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
			diagnostic.Range.End != start + len(want[index]) {
			t.Fatalf("%s diagnostic[%d] = %#v", ruleID, index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}
