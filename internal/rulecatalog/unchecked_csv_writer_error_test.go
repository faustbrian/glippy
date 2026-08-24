package rulecatalog_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestUncheckedCSVWriterErrorRequiresObservationOnEveryNormalReturn(t *testing.T) {
	t.Parallel()

	input := `package sample

import "encoding/csv"

func unchecked(writer *csv.Writer) {
	writer.Flush()
}

func partial(writer *csv.Writer, observe bool) error {
	writer.Flush()
	if observe {
		return writer.Error()
	}
	return nil
}

func blank(writer *csv.Writer) {
	writer.Flush()
	_ = writer.Error()
}

func expression(writer *csv.Writer) {
	writer.Flush()
	writer.Error()
}

func reassigned(writer, replacement *csv.Writer) error {
	writer.Flush()
	writer = replacement
	return writer.Error()
}

func checked(writer *csv.Writer) error {
	writer.Flush()
	return writer.Error()
}

func checkedCondition(writer *csv.Writer) error {
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return nil
}

func checkedArgument(writer *csv.Writer) {
	writer.Flush()
	report(writer.Error())
}

func checkedAfterTwoFlushes(writer *csv.Writer) error {
	writer.Flush()
	writer.Flush()
	return writer.Error()
}

func checkedBeforeOnly(writer *csv.Writer) {
	_ = writer.Error()
	writer.Flush()
}

func noReturnPath(writer *csv.Writer, fail bool) error {
	writer.Flush()
	if fail {
		panic("failed")
	}
	return writer.Error()
}

const disabled = false

func constantFalse(writer *csv.Writer) {
	if false {
		writer.Flush()
	}
}

func namedConstantFalse(writer *csv.Writer) {
	if disabled {
		writer.Flush()
	}
}

func constantTrueElse(writer *csv.Writer) {
	if true {
		return
	} else {
		writer.Flush()
	}
}

func constantFalseLoop(writer *csv.Writer) {
	for false {
		writer.Flush()
	}
}

func conditional(writer *csv.Writer, enabled bool) {
	if enabled {
		writer.Flush()
	}
}

func transferred(writer *csv.Writer) {
	writer.Flush()
	consume(writer)
}

func aliased(writer *csv.Writer) {
	writer.Flush()
	other := writer
	consume(other)
}

func deferred(writer *csv.Writer) { defer writer.Flush() }
func asynchronous(writer *csv.Writer) { go writer.Flush() }

func terminal(writer *csv.Writer) {
	writer.Flush()
	stop()
}

func stop() { panic("stopped") }

type localWriter struct{}
func (*localWriter) Flush() {}
func (*localWriter) Error() error { return nil }
func unrelated(writer *localWriter) { writer.Flush() }

type holder struct { writer *csv.Writer }
func field(value *holder) { value.writer.Flush() }

func report(error) {}
func consume(*csv.Writer) {}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedcsvwriter\n\ngo 1.26.0\n",
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
				"unchecked-csv-writer-error": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 7 {
		t.Fatalf("unchecked-csv-writer-error result = %#v", result.Files)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantStarts := lifecycleDiagnosticStarts(
		t,
		input,
		"writer.Flush()",
		"unchecked",
		"partial",
		"blank",
		"expression",
		"reassigned",
		"checkedBeforeOnly",
		"conditional",
	)
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "unchecked-csv-writer-error" ||
			diagnostic.MessageKey != "csv-writer-error-not-checked" ||
			string(content[diagnostic.Range.Start:diagnostic.Range.End]) !=
				"writer.Flush()" ||
			!wantStarts[diagnostic.Range.Start] ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("unchecked CSV writer diagnostic = %#v", diagnostic)
		}
		delete(wantStarts, diagnostic.Range.Start)
	}
	if len(wantStarts) != 0 {
		t.Fatalf("missing unchecked CSV writer diagnostics at %#v", wantStarts)
	}
}

func lifecycleDiagnosticStarts(
	t *testing.T,
	input string,
	operation string,
	functions ...string,
) map[int]bool {
	t.Helper()
	result := make(map[int]bool, len(functions))
	for _, function := range functions {
		functionStart := strings.Index(input, "func " + function + "(")
		if functionStart < 0 {
			t.Fatalf("missing fixture function %q", function)
		}
		operationStart := strings.Index(input[functionStart:], operation)
		if operationStart < 0 {
			t.Fatalf("missing %q in fixture function %q", operation, function)
		}
		result[functionStart + operationStart] = true
	}
	return result
}

func TestUncheckedCSVWriterErrorUsesExactTypeAcrossHelperPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedcsvhelper\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import "encoding/csv"

func Open() *csv.Writer { return nil }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import "example.com/uncheckedcsvhelper/helper"

func run() {
	writer := helper.Open()
	writer.Flush()
}
`,
	)
	result := runUncheckedCSVWriterError(t, root, "go1.26", false, nil)
	if countPackageDiagnostics(result) != 1 {
		t.Fatalf("helper-returned CSV writer result = %#v", result.Files)
	}
}

func TestUncheckedCSVWriterErrorHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedcsvpolicy\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import "encoding/csv"

func suppressed(writer *csv.Writer) {
	//glippy:ignore unchecked-csv-writer-error -- output is deliberately best effort
	writer.Flush()
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import "encoding/csv"

func generated(writer *csv.Writer) { writer.Flush() }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import "encoding/csv"

func invalid(writer *csv.Writer) {
	var broken string = 1
	_ = broken
	writer.Flush()
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "sample_test.go"),
		`package sample

import (
	"encoding/csv"
	"testing"
)

func TestFlush(t *testing.T) {
	var writer *csv.Writer
	writer.Flush()
}
`,
	)
	result := runUncheckedCSVWriterError(
		t,
		root,
		"go1.26",
		true,
		map[string]rules.Severity{"unchecked-csv-writer-error": rules.SeverityError},
	)
	if len(result.LoadDiagnostics) == 0 || len(result.Files) != 4 {
		t.Fatalf("unchecked CSV writer policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID !=
					"unchecked-csv-writer-error" {
				t.Fatalf("suppressed CSV writer result = %#v", file)
			}
		case "sample_test.go":
			if len(file.Diagnostics) != 1 ||
				file.Diagnostics[0].RuleID != "unchecked-csv-writer-error" ||
				file.Diagnostics[0].Severity != rules.SeverityError {
				t.Fatalf("test CSV writer result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded CSV writer result = %#v", file)
			}
		default:
			t.Fatalf("unexpected CSV policy file %q", file.Path)
		}
	}

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("unchecked-csv-writer-error")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireControlFlow ||
		!metadata.RequiresEffectFacts ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("unchecked-csv-writer-error metadata = %#v, found = %t", metadata, found)
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range selection {
		if selected.ID == "unchecked-csv-writer-error" {
			t.Fatalf("pre-minimum CSV writer selection = %#v", selection)
		}
	}
}

func runUncheckedCSVWriterError(
	t testing.TB,
	root string,
	goVersion string,
	tests bool,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	if overrides == nil {
		overrides = map[string]rules.Severity{
			"unchecked-csv-writer-error": rules.SeverityWarn,
		}
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: goVersion,
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			Tests: tests,
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func BenchmarkUncheckedCSVWriterErrorSharedCFG(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedcsvbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"encoding/csv\"\n")
	for index := range 100 {
		fmt.Fprintf(&input, "func run%d(writer *csv.Writer) { writer.Flush() }\n", index)
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
				"unchecked-csv-writer-error": rules.SeverityWarn,
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
