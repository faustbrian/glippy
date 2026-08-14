package rules_test

import (
	"context"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestComplexityRulesReportConfiguredThresholds(t *testing.T) {
	t.Parallel()

	input := `package sample

func complex(first, second, third, fourth int) (int, int, int, error) {
	if first > 0 {
		for second > 0 {
			if third > 0 {
				fourth++
			}
			second--
		}
	}
	return first, second, third, nil
}
`
	result := runComplexityRules(
		t,
		input,
		map[string]rules.OptionSet{
			"excessive-nesting": complexityOptions("maximum", 2),
			"too-many-lines": complexityOptions("maximum", 5),
			"too-many-parameters": complexityOptions("maximum", 3),
			"too-many-results": complexityOptions("maximum", 3),
		},
	)
	if len(result.Diagnostics) != 4 {
		t.Fatalf("complexity diagnostics = %#v", result.Diagnostics)
	}
	want := map[string]string{
		"excessive-nesting": "complex",
		"too-many-lines": "complex",
		"too-many-parameters": "first, second, third, fourth int",
		"too-many-results": "int, int, int, error",
	}
	for _, diagnostic := range result.Diagnostics {
		got, valid := sourceText(input, diagnostic.Range.Start, diagnostic.Range.End)
		if !valid || got != want[diagnostic.RuleID] {
			t.Fatalf(
				"%s range = %q, want %q",
				diagnostic.RuleID,
				got,
				want[diagnostic.RuleID],
			)
		}
		if !strings.Contains(diagnostic.Message, "/") || len(diagnostic.Fixes) != 0 {
			t.Fatalf("%s diagnostic = %#v", diagnostic.RuleID, diagnostic)
		}
	}
}

func TestComplexityRulesRespectBoundariesAndGoSyntax(t *testing.T) {
	t.Parallel()

	input := `package sample

type callback func(first, second, third int) (int, int)

func external()

type receiver struct{}
func (receiver) method(first, second, third int) (int, int) {
	// comments and blank lines are not code lines

	if first > 0 {
		if second > 0 {
			third++
		}
	}
	_ = func(a, b, c int) (int, int) {
		if a > 0 {
			if b > 0 {
				return a, b
			}
		}
		return c, 0
	}
	return first, second
}
`
	result := runComplexityRules(
		t,
		input,
		map[string]rules.OptionSet{
			"excessive-nesting": complexityOptions("maximum", 2),
			"too-many-lines": complexityOptions("maximum", 7),
			"too-many-parameters": complexityOptions("maximum", 3),
			"too-many-results": complexityOptions("maximum", 2),
		},
	)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("boundary diagnostics = %#v", result.Diagnostics)
	}
}

func TestComplexityRulesUseDocumentedDefaults(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc concise(first, second int) error { return nil }\n"
	result := runComplexityRules(t, input, nil)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("default complexity diagnostics = %#v", result.Diagnostics)
	}
}

func TestTooManyLinesCountsNonEnclosingBraceOnlyLines(t *testing.T) {
	t.Parallel()

	input := `package sample

func braces() {
	value := struct {
		field int
	}{
		field: 1,
	}
	_ = value
}
`
	result := runComplexityRules(
		t,
		input,
		map[string]rules.OptionSet{"too-many-lines": complexityOptions("maximum", 5)},
	)
	if len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].RuleID != "too-many-lines" ||
		!strings.Contains(result.Diagnostics[0].Message, "(6/5)") {
		t.Fatalf("brace-line diagnostics = %#v", result.Diagnostics)
	}
}

func runComplexityRules(
	t *testing.T,
	input string,
	options map[string]rules.OptionSet,
) analysis.Result {
	t.Helper()
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(
		rules.NewExcessiveNestingRule(),
		rules.NewTooManyLinesRule(),
		rules.NewTooManyParametersRule(),
		rules.NewTooManyResultsRule(),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{rules.PresetComplexity},
			RuleOptions: options,
			SourceGoVersion: "go1.25",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func complexityOptions(name string, value int64) rules.OptionSet {
	return rules.NewOptionSet(map[string]rules.OptionValue{name: rules.IntegerOption(value)})
}

func sourceText(input string, start, end int) (string, bool) {
	if start < 0 || end < start || end > len(input) {
		return "", false
	}
	return input[start:end], true
}
