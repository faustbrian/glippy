package rulecatalog_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestExactSuffixAsCutsetReportsGuardedTrimRight(t *testing.T) {
	t.Parallel()

	input := `package sample

import "strings"

func trim(value, suffix string) string {
	if strings.HasSuffix(value, suffix) {
		value = strings.TrimRight(value, suffix)
	}
	return value
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/exactsuffixcutset\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeFixture(t, path, input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"exact-suffix-as-cutset": rules.SeverityWarn,
			},
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
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("exact-suffix-as-cutset result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	start := strings.Index(input, "strings.TrimRight")
	if start < 0 {
		t.Fatal("fixture does not contain strings.TrimRight")
	}
	start += len("strings.")
	if diagnostic.RuleID != "exact-suffix-as-cutset" ||
		diagnostic.Severity != rules.SeverityWarn ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len("TrimRight") ||
		diagnostic.MessageKey != "exact-suffix-as-cutset" ||
		len(diagnostic.Fixes) != 1 ||
		diagnostic.Fixes[0].Name != "use-trim-suffix" ||
		diagnostic.Fixes[0].Safety != rules.FixUnsafe ||
		len(diagnostic.Fixes[0].Edits) != 1 ||
		diagnostic.Fixes[0].Edits[0].Range != diagnostic.Range ||
		diagnostic.Fixes[0].Edits[0].NewText != "TrimSuffix" {
		t.Fatalf("exact-suffix-as-cutset diagnostic = %#v", diagnostic)
	}
}

func TestExactSuffixAsCutsetMatchesGrafanaCorpusShape(t *testing.T) {
	t.Parallel()

	input := `package sample

import "strings"

type column struct { Default string }

func normalize(col *column, text, timestamp bool) {
	if text {
		if strings.HasSuffix(col.Default, "::character varying") {
			col.Default = strings.TrimRight(col.Default, "::character varying")
		}
	} else if timestamp {
		if strings.HasSuffix(col.Default, "::timestamp without time zone") {
			col.Default = strings.TrimRight(col.Default, "::timestamp without time zone")
		}
	}
}
`
	result := runExactSuffixAsCutset(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("Grafana-shape exact-suffix-as-cutset result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], "strings.TrimRight")
		if relative < 0 {
			t.Fatalf("missing Grafana-shape TrimRight call %d", index)
		}
		start := searchFrom + relative + len("strings.")
		if diagnostic.RuleID != "exact-suffix-as-cutset" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("TrimRight") {
			t.Fatalf("Grafana-shape diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len("TrimRight")
	}
}

func TestExactSuffixAsCutsetRequiresExactGuardAndValueIdentity(t *testing.T) {
	t.Parallel()

	input := `package sample

import text "strings"

type helper struct{}

func (helper) HasSuffix(string, string) bool { return true }
func (helper) TrimRight(value, _ string) string { return value }

var fake helper

func alias(value, suffix string) string {
	if text.HasSuffix(value, suffix) {
		return text.TrimRight(value, suffix)
	}
	return value
}

func constants(value string) string {
	if text.HasSuffix(value, "::text") {
		return text.TrimRight(value, "::text")
	}
	return value
}

func differentValue(first, second, suffix string) string {
	if text.HasSuffix(first, suffix) {
		return text.TrimRight(second, suffix)
	}
	return first
}

func differentSuffix(value, first, second string) string {
	if text.HasSuffix(value, first) {
		return text.TrimRight(value, second)
	}
	return value
}

func combined(value, suffix string, ready bool) string {
	if ready && text.HasSuffix(value, suffix) {
		return text.TrimRight(value, suffix)
	}
	return value
}

func delayed(value, suffix string) string {
	if text.HasSuffix(value, suffix) {
		println(value)
		return text.TrimRight(value, suffix)
	}
	return value
}

func deliberate(value, cutset string) string {
	return text.TrimRight(value, cutset)
}

func lookalike(value, suffix string) string {
	if fake.HasSuffix(value, suffix) {
		return fake.TrimRight(value, suffix)
	}
	return value
}

func indirect(value, suffix string) string {
	hasSuffix := text.HasSuffix
	trimRight := text.TrimRight
	if hasSuffix(value, suffix) {
		return trimRight(value, suffix)
	}
	return value
}
`
	result := runExactSuffixAsCutset(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("exact-suffix-as-cutset boundary result = %#v", result)
	}
	wantStarts := []int{
		strings.Index(input, "text.TrimRight(value, suffix)") + len("text."),
		strings.Index(input, "text.TrimRight(value, \"::text\")") + len("text."),
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if wantStarts[index] < len("text.") ||
			diagnostic.RuleID != "exact-suffix-as-cutset" ||
			diagnostic.Range.Start != wantStarts[index] ||
			diagnostic.Range.End != wantStarts[index] + len("TrimRight") {
			t.Fatalf(
				"exact-suffix-as-cutset boundary diagnostic[%d] = %#v",
				index,
				diagnostic,
			)
		}
	}
}

func TestExactSuffixAsCutsetRejectsEarlierOperandMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

import "strings"

func mutate(value, suffix *string) string {
	*value = "changed"
	*suffix = "ged"
	return "prefix"
}

func trim(value, suffix string) string {
	if strings.HasSuffix(value, suffix) {
		return mutate(&value, &suffix) + strings.TrimRight(value, suffix)
	}
	return value
}
`
	result := runExactSuffixAsCutset(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("operand-mutating exact-suffix-as-cutset result = %#v", result)
	}
}

func TestExactSuffixAsCutsetUnsafeFixIsFormattedAndIdempotent(t *testing.T) {
	t.Parallel()

	input := "package sample\nimport \"strings\"\nfunc trim(value,suffix string)string{if strings.HasSuffix(value,suffix){return strings.TrimRight(value,suffix)};return value}\n"
	result := runExactSuffixAsCutset(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("exact-suffix-as-cutset fix result = %#v", result)
	}
	path := result.Files[0].Path
	file, found := result.Sources.Lookup(path)
	if !found {
		t.Fatal("exact-suffix-as-cutset source is missing")
	}
	selection := fixengine.Selection{
		Diagnostic: result.Files[0].Diagnostics[0],
		FixName: "use-trim-suffix",
	}
	applied, err := fixengine.Coordinate(
		file,
		[]fixengine.Selection{selection},
		fixengine.Options{
			Format: glippyformat.Options{Width: 100, TabWidth: 8, FitBudget: 10_000},
			AllowUnsafe: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := `package sample

import "strings"

func trim(value, suffix string) string {
	if strings.HasSuffix(value, suffix) {
		return strings.TrimSuffix(value, suffix)
	}
	return value
}
`
	if string(applied.Bytes) != want ||
		len(applied.Applied) != 1 ||
		len(applied.Rejected) != 0 {
		t.Fatalf(
			"exact-suffix-as-cutset applied fix = %#v, bytes = %q",
			applied,
			applied.Bytes,
		)
	}
	second := runExactSuffixAsCutset(t, string(applied.Bytes))
	if len(second.Files) != 1 || len(second.Files[0].Diagnostics) != 0 {
		t.Fatalf("fixed exact-suffix-as-cutset result = %#v", second)
	}
}

func TestExactSuffixAsCutsetMetadataAndGoVersion(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("exact-suffix-as-cutset")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetNursery}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeIfStmt}) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!reflect.DeepEqual(
			metadata.Fixes,
			[]rules.FixMetadata{
				{
					Name: "use-trim-suffix",
					Description: "replace strings.TrimRight with strings.TrimSuffix",
					Safety: rules.FixUnsafe,
				},
			},
		) {
		t.Fatalf("exact-suffix-as-cutset metadata = %#v, found = %v", metadata, found)
	}
	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"exact-suffix-as-cutset": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil || len(older) != 0 {
		t.Fatalf("go1.24 exact-suffix-as-cutset selection = %#v, %v", older, err)
	}
}

func TestExactSuffixAsCutsetHonorsSharedSourcePolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/exactsuffixcutsetpolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample
import "strings"
func suppressed(value, suffix string) string {
	if strings.HasSuffix(value, suffix) {
		//glippy:ignore exact-suffix-as-cutset -- compatibility requires legacy trimming
		return strings.TrimRight(value, suffix)
	}
	return value
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample
import "strings"
func generated(value, suffix string) string {
	if strings.HasSuffix(value, suffix) { return strings.TrimRight(value, suffix) }
	return value
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid
import "strings"
func invalid(value, suffix string) string {
	if strings.HasSuffix(value, suffix) { return strings.TrimRight(value, suffix) }
	var broken string = 1
	return value + broken
}
`,
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"exact-suffix-as-cutset": rules.SeverityError,
			},
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
		t.Fatalf("exact-suffix-as-cutset policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "exact-suffix-as-cutset" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed exact-suffix-as-cutset result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded exact-suffix-as-cutset result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy path %q", file.Path)
		}
	}
}

func runExactSuffixAsCutset(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/exactsuffixcutsetfixture\n\ngo 1.25.0\n",
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
			Overrides: map[string]rules.Severity{
				"exact-suffix-as-cutset": rules.SeverityWarn,
			},
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
