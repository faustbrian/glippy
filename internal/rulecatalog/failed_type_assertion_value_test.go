package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestFailedTypeAssertionValueReportsShadowedElseRead(t *testing.T) {
	t.Parallel()

	input := `package sample

import "fmt"

func describe(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		return fmt.Sprintf("unexpected %T", value)
	}
}
`
	result := runFailedTypeAssertionValue(t, input)
	if len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("failed-type-assertion-value result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	marker := `fmt.Sprintf("unexpected %T", value)`
	wantStart := strings.Index(input, marker) + strings.LastIndex(marker, "value")
	shadowStart := strings.Index(input, "value, ok :=")
	originalStart := strings.Index(input, "value.(string)")
	if diagnostic.RuleID != "failed-type-assertion-value" ||
		diagnostic.Severity != rules.SeverityWarn ||
		diagnostic.MessageKey != "failed-type-assertion-value" ||
		diagnostic.Range.Start != wantStart ||
		diagnostic.Range.End != wantStart + len("value") ||
		len(diagnostic.Related) != 2 ||
		diagnostic.Related[0].Range.Start != shadowStart ||
		diagnostic.Related[0].Range.End != shadowStart + len("value") ||
		diagnostic.Related[1].Range.Start != originalStart ||
		diagnostic.Related[1].Range.End != originalStart + len("value") ||
		len(diagnostic.Fixes) != 0 {
		t.Fatalf("failed-type-assertion-value diagnostic = %#v", diagnostic)
	}
}

func TestFailedTypeAssertionValueRaisesCorrectnessPresetToSSA(t *testing.T) {
	t.Parallel()

	input := `package sample

func describe(value any) string {
	if value, ok := value.(string); ok {
		return "ok: " + value
	} else {
		return value
	}
}
`
	result := runFailedTypeAssertionValueWithOptions(
		t,
		input,
		analysis.RunOptions{Preset: rules.PresetCorrectness, SourceGoVersion: "go1.25"},
	)
	if result.Requirement != rules.RequireSSA ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "failed-type-assertion-value" {
		t.Fatalf("correctness preset failed-type-assertion-value result = %#v", result)
	}
}

func TestFailedTypeAssertionValueReportsPointerInterfaceAndElseIfReads(t *testing.T) {
	t.Parallel()

	input := `package sample

import "fmt"

type item struct{}

func pointer(value any) bool {
	if value, ok := value.(*item); ok {
		return value != nil
	} else {
		return value == nil
	}
}

func interfaceValue(value any) bool {
	if value, ok := value.(fmt.Stringer); ok {
		return value != nil
	} else {
		return value == nil
	}
}

func chained(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else if value == "" {
		return "zero"
	}
	return "unreachable"
}
`
	result := runFailedTypeAssertionValue(t, input)
	markers := []string{"return value == nil", "return value == nil", `else if value == ""`}
	if len(result.Files[0].Diagnostics) != len(markers) {
		t.Fatalf("failed-type-assertion-value result = %#v", result)
	}
	searchStart := 0
	for index, marker := range markers {
		relative := strings.Index(input[searchStart:], marker)
		if relative < 0 {
			t.Fatalf(
				"fixture does not contain marker %q after byte %d",
				marker,
				searchStart,
			)
		}
		markerStart := searchStart + relative
		valueStart := markerStart + strings.Index(marker, "value")
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.Range.Start != valueStart ||
			diagnostic.Range.End != valueStart + len("value") {
			t.Fatalf(
				"failed-type-assertion-value diagnostic[%d] = %#v",
				index,
				diagnostic,
			)
		}
		searchStart = markerStart + len(marker)
	}
}

func TestFailedTypeAssertionValueExcludesUnprovenForms(t *testing.T) {
	t.Parallel()

	input := `package sample

func consume(*string) {}

func renamed(value any) string {
	if text, ok := value.(string); ok {
		return text
	} else {
		return "not " + value.(string)
	}
}

func assignment(value any) string {
	var text string
	var ok bool
	if text, ok = value.(string); ok {
		return text
	}
	return ""
}

func typeSwitch(value any) string {
	switch value := value.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func compound(value any) string {
	if value, ok := value.(string); ok && value != "" {
		return value
	} else {
		return value
	}
}

func negated(value any) string {
	if value, ok := value.(string); !ok {
		return value
	}
	return value
}

func reassigned(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		value = "fallback"
		return value
	}
}

func addressed(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		consume(&value)
		return ""
	}
}

func captured(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		return func() string { return value }()
	}
}

func joined(value any, condition bool) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		if condition {
			value = "fallback"
		}
		return value
	}
}

func noElse(value any) string {
	if value, ok := value.(string); ok {
		return value
	}
	return ""
}
`
	result := runFailedTypeAssertionValue(t, input)
	if len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("unproven failed-type-assertion-value result = %#v", result)
	}
}

func TestFailedTypeAssertionValueMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("failed-type-assertion-value")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!slices.Equal(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSSA ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!slices.Equal(
			metadata.Categories,
			[]rules.Category{rules.CategoryCorrectness, rules.CategorySafety},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("failed-type-assertion-value metadata = %#v, found = %v", metadata, found)
	}
}

func TestFailedTypeAssertionValueHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/failedtypeassertionvaluepolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

func suppressed(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		//glippy:ignore failed-type-assertion-value -- compatibility preserves the zero-value fallback
		return value
	}
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

func generated(value any) string {
	if value, ok := value.(string); ok {
		return value
	} else {
		return value
	}
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

func invalid(value any) string {
	var broken string = 1
	_ = broken
	if value, ok := value.(string); ok {
		return value
	} else {
		return value
	}
}
`,
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	overrides := map[string]rules.Severity{"failed-type-assertion-value": rules.SeverityError}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 || len(result.LoadDiagnostics) == 0 {
		t.Fatalf("failed-type-assertion-value policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID !=
					"failed-type-assertion-value" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf(
					"suppressed failed-type-assertion-value result = %#v",
					file,
				)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded failed-type-assertion-value result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("pre-minimum failed-type-assertion-value selection = %#v", selection)
	}
}

func runFailedTypeAssertionValue(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	return runFailedTypeAssertionValueWithOptions(
		t,
		input,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"failed-type-assertion-value": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
	)
}

func runFailedTypeAssertionValueWithOptions(
	t *testing.T,
	input string,
	options analysis.RunOptions,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/failedtypeassertionvalue\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		options,
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || !strings.HasSuffix(result.Files[0].Path, "sample.go") {
		t.Fatalf("failed-type-assertion-value result = %#v", result)
	}
	return result
}

func BenchmarkFailedTypeAssertionValuePackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/failedtypeassertionvaluebenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func describe%d(value any) any { if value, ok := value.(string); ok { return value } else { return value } }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	benchmarkPackageRuns(
		b,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"failed-type-assertion-value": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		100,
	)
}
