package rulecatalog_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestNeedlessBlankIdentifierReportsDiscardOnlyForms(t *testing.T) {
	t.Parallel()

	input := `package sample

func run(values []string, receive <-chan string) {
	for _ = range values {}
	for _, _ = range values {}
	for index, _ := range values { _ = index }
	_ = <-receive
	_, _ = <-receive
	for _, value := range values { _ = value }
	value, ok := <-receive
	_, _ = value, ok
}
`
	result := runOnePedanticRule(t, "needless-blank-identifier", input)
	want := []string{
		"_ = range values",
		"_, _ = range values",
		"index, _ := range values",
		"_ = <-receive",
		"_, _ = <-receive",
	}
	assertPedanticSimplificationRanges(
		t,
		input,
		result,
		"needless-blank-identifier",
		"omit-blank-identifier",
		want,
	)
	for _, diagnostic := range result.Files[0].Diagnostics {
		if len(diagnostic.Fixes) != 1 ||
			diagnostic.Fixes[0].Name != "remove-blank-identifier" ||
			diagnostic.Fixes[0].Safety != rules.FixSuggestion {
			t.Fatalf("needless-blank-identifier fixes = %#v", diagnostic.Fixes)
		}
	}
}

func TestRedundantClosureReportsDirectDelegationOnly(t *testing.T) {
	t.Parallel()

	input := `package sample

import "strings"

func sink(string) {}
func transform(func(string) string) {}

func run(prefix string) {
	transform(func(value string) string { return strings.TrimSpace(value) })
	callback := func(value string) { sink(value) }
	_ = callback
	transform(func(value string) string { return strings.TrimSpace(prefix + value) })
	_ = func(value string) string { value = prefix + value; return strings.TrimSpace(value) }
}
`
	result := runOnePedanticRule(t, "redundant-closure", input)
	want := []string{
		"func(value string) string { return strings.TrimSpace(value) }",
		"func(value string) { sink(value) }",
	}
	assertPedanticSimplificationRanges(
		t,
		input,
		result,
		"redundant-closure",
		"direct-delegation",
		want,
	)
}

func TestRedundantNilCheckReportsLenEquivalentConditions(t *testing.T) {
	t.Parallel()

	input := `package sample

const one = 1

func run(values []string, mapping map[string]int, receive chan string, array *[2]string) {
	if values != nil && len(values) > 0 {}
	if mapping == nil || len(mapping) == 0 {}
	if receive != nil && len(receive) >= one {}
	if values != nil && len(values) == 0 {}
	if array != nil && len(array) > 0 {}
	if values == nil || len(values) == one {}
	if values == nil || len(values) <= -1 {}
	if values == nil || len(values) < -1 {}
	if values != nil && len(values) >= -1 {}
	if values != nil && len(values) > -1 {}
}
`
	result := runOnePedanticRule(t, "redundant-nil-check", input)
	want := []string{
		"values != nil && len(values) > 0",
		"mapping == nil || len(mapping) == 0",
		"receive != nil && len(receive) >= one",
	}
	assertPedanticSimplificationRanges(
		t,
		input,
		result,
		"redundant-nil-check",
		"omit-nil-check",
		want,
	)
}

func TestTimeConvenienceRulesReportExactCallsAndSuggestions(t *testing.T) {
	t.Parallel()

	input := `package sample

import clock "time"

func run(start, deadline clock.Time) {
	_ = clock.Now().Sub(start)
	_ = deadline.Sub(clock.Now())
	_ = start.Sub(deadline)
}
`
	tests := []struct {
		id string
		messageKey string
		text string
		fix string
		replacement string
	}{
		{
			"time-since",
			"use-time-since",
			"clock.Now().Sub(start)",
			"use-time-since",
			"clock.Since(start)",
		},
		{
			"time-until",
			"use-time-until",
			"deadline.Sub(clock.Now())",
			"use-time-until",
			"clock.Until(deadline)",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.id,
			func(t *testing.T) {
				t.Parallel()
				result := runOnePedanticRule(t, test.id, input)
				assertPedanticSimplificationRanges(
					t,
					input,
					result,
					test.id,
					test.messageKey,
					[]string{test.text},
				)
				diagnostic := result.Files[0].Diagnostics[0]
				if len(diagnostic.Fixes) != 1 ||
					diagnostic.Fixes[0].Name != test.fix ||
					diagnostic.Fixes[0].Safety != rules.FixSuggestion ||
					len(diagnostic.Fixes[0].Edits) != 1 ||
					diagnostic.Fixes[0].Edits[0].NewText != test.replacement {
					t.Fatalf("%s fixes = %#v", test.id, diagnostic.Fixes)
				}
			},
		)
	}
}

func TestPedanticSimplificationSuggestionsPreserveComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id string
		input string
	}{
		{
			"needless-blank-identifier",
			"package sample\nfunc run(values []int) { for _, /* keep */ _ = range values {} }\n",
		},
		{
			"time-since",
			"package sample\nimport \"time\"\nfunc run(start time.Time) { _ = time.Now().Sub(/* keep */ start) }\n",
		},
		{
			"time-until",
			"package sample\nimport \"time\"\nfunc run(deadline time.Time) { _ = deadline.Sub(/* keep */ time.Now()) }\n",
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

func TestBufferStringConversionReportsDirectBufferMethods(t *testing.T) {
	t.Parallel()

	input := `package sample

import "bytes"

type other struct{}
func (other) Bytes() []byte { return nil }
func (other) String() string { return "" }

func run(buffer *bytes.Buffer, values map[string]int, custom other) {
	_ = string(buffer.Bytes())
	_ = []byte(buffer.String())
	_ = values[string(buffer.Bytes())]
	_ = string(custom.Bytes())
	_ = []byte(custom.String())
}
`
	result := runOnePedanticRule(t, "buffer-string-conversion", input)
	want := []string{"string(buffer.Bytes())", "[]byte(buffer.String())"}
	assertPedanticSimplificationRanges(
		t,
		input,
		result,
		"buffer-string-conversion",
		"use-buffer-method",
		want,
	)
}

func TestUnnecessaryFormatReportsLiteralFormatsWithoutArguments(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"fmt"
	"log"
	"testing"
)

func run(t *testing.T, value, format string) {
	_ = fmt.Sprintf("literal")
	const constant = "constant"
	_ = fmt.Sprintf(constant)
	fmt.Printf("literal")
	log.Printf("literal")
	t.Logf("literal")
	_ = fmt.Sprintf("value=%s", value)
	_ = fmt.Sprintf("literal", value)
	_ = fmt.Sprintf(format)
	_ = fmt.Sprintf("100%%")
}
`
	result := runOnePedanticRule(t, "unnecessary-format", input)
	want := []string{`fmt.Sprintf("literal")`, "fmt.Sprintf(constant)"}
	assertPedanticSimplificationRanges(
		t,
		input,
		result,
		"unnecessary-format",
		"omit-formatting",
		want,
	)
}

func TestInefficientStringComparisonReportsMatchingCaseNormalization(t *testing.T) {
	t.Parallel()

	input := `package sample

import "strings"

func run(left, right string) {
	_ = strings.ToLower(left) == strings.ToLower(right)
	_ = strings.ToUpper(left) != strings.ToUpper(right)
	_ = strings.ToLower(left) == strings.ToUpper(right)
	_ = strings.ToLower(left) == strings.ToLower(left)
}
`
	result := runOnePedanticRule(t, "inefficient-string-comparison", input)
	want := []string{
		"strings.ToLower(left) == strings.ToLower(right)",
		"strings.ToUpper(left) != strings.ToUpper(right)",
	}
	assertPedanticSimplificationRanges(
		t,
		input,
		result,
		"inefficient-string-comparison",
		"use-equal-fold",
		want,
	)
}

func TestPedanticSimplificationMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		interests []rules.NodeKind
		fixes []rules.FixMetadata
	}{
		"needless-blank-identifier": {
			[]rules.NodeKind{rules.NodeAssignStmt, rules.NodeRangeStmt},
			[]rules.FixMetadata{
				{
					Name: "remove-blank-identifier",
					Description: "remove the unnecessary blank-identifier assignment",
					Safety: rules.FixSuggestion,
				},
			},
		},
		"redundant-closure": {[]rules.NodeKind{rules.NodeFuncLit}, nil},
		"redundant-nil-check": {[]rules.NodeKind{rules.NodeBinaryExpr}, nil},
		"time-since": {
			[]rules.NodeKind{rules.NodeCallExpr},
			[]rules.FixMetadata{
				{
					Name: "use-time-since",
					Description: "replace time.Now().Sub with time.Since",
					Safety: rules.FixSuggestion,
				},
			},
		},
		"time-until": {
			[]rules.NodeKind{rules.NodeCallExpr},
			[]rules.FixMetadata{
				{
					Name: "use-time-until",
					Description: "replace Time.Sub(time.Now()) with time.Until",
					Safety: rules.FixSuggestion,
				},
			},
		},
		"buffer-string-conversion": {[]rules.NodeKind{rules.NodeFile}, nil},
		"unnecessary-format": {[]rules.NodeKind{rules.NodeCallExpr}, nil},
		"inefficient-string-comparison": {[]rules.NodeKind{rules.NodeBinaryExpr}, nil},
	}
	for id, expected := range want {
		metadata, found := registry.Metadata(id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(
				metadata.Presets,
				[]rules.Preset{rules.PresetPedantic},
			) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != rules.RequireTypes ||
			!reflect.DeepEqual(metadata.NodeInterests, expected.interests) ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors ||
			!reflect.DeepEqual(metadata.Fixes, expected.fixes) {
			t.Fatalf("%s metadata = %#v, found = %v", id, metadata, found)
		}
	}
}

func BenchmarkPedanticSimplificationPackageAnalysis(b *testing.B) {
	tests := []struct {
		id string
		input string
	}{
		{
			"needless-blank-identifier",
			"package sample\nfunc run(values []int) { for _, _ = range values {} }\n",
		},
		{
			"redundant-closure",
			"package sample\nfunc sink(string) {}\nfunc run() { _ = func(value string) { sink(value) } }\n",
		},
		{
			"redundant-nil-check",
			"package sample\nfunc run(values []int) { if values != nil && len(values) > 0 {} }\n",
		},
		{
			"time-since",
			"package sample\nimport \"time\"\nfunc run(start time.Time) { _ = time.Now().Sub(start) }\n",
		},
		{
			"time-until",
			"package sample\nimport \"time\"\nfunc run(deadline time.Time) { _ = deadline.Sub(time.Now()) }\n",
		},
		{
			"buffer-string-conversion",
			"package sample\nimport \"bytes\"\nfunc run(buffer *bytes.Buffer) { _ = string(buffer.Bytes()) }\n",
		},
		{
			"unnecessary-format",
			"package sample\nimport \"fmt\"\nfunc run() { _ = fmt.Sprintf(\"literal\") }\n",
		},
		{
			"inefficient-string-comparison",
			"package sample\nimport \"strings\"\nfunc run(left, right string) { _ = strings.ToLower(left) == strings.ToLower(right) }\n",
		},
	}
	for _, test := range tests {
		test := test
		b.Run(
			test.id,
			func(b *testing.B) {
				benchmarkPedanticExpressionRule(b, test.id, test.input)
			},
		)
	}
}

func assertPedanticSimplificationRanges(
	t *testing.T,
	input string,
	result analysis.PackageResult,
	ruleID string,
	messageKey string,
	want []string,
) {
	t.Helper()
	if len(result.Files) != 1 {
		t.Fatalf("%s result files = %#v", ruleID, result.Files)
	}
	diagnostics := result.Files[0].Diagnostics
	if len(diagnostics) != len(want) {
		t.Fatalf("%s diagnostics = %#v, want %d", ruleID, diagnostics, len(want))
	}
	searchFrom := 0
	for index, diagnostic := range diagnostics {
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
