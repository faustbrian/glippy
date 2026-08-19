package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestUncheckedWriterErrorReportsDiscardedFinalizationErrors(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
	"encoding/xml"
	"mime/multipart"
	"mime/quotedprintable"
	"text/tabwriter"
)

func ignored(
	buffer *bufio.Writer,
	tarWriter *tar.Writer,
	zipWriter *zip.Writer,
	flateWriter *flate.Writer,
	gzipWriter *gzip.Writer,
	lzwWriter *lzw.Writer,
	zlibWriter *zlib.Writer,
	xmlEncoder *xml.Encoder,
	tabWriter *tabwriter.Writer,
	quotedWriter *quotedprintable.Writer,
	multipartWriter *multipart.Writer,
) {
	buffer.Flush()
	defer tarWriter.Close()
	tarWriter.Flush()
	go zipWriter.Flush()
	zipWriter.Close()
	_ = flateWriter.Close()
	flateWriter.Flush()
	gzipWriter.Close()
	gzipWriter.Flush()
	lzwWriter.Close()
	zlibWriter.Flush()
	zlibWriter.Close()
	xmlEncoder.Close()
	xmlEncoder.Flush()
	tabWriter.Flush()
	quotedWriter.Close()
	multipartWriter.Close()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwritererror\n\ngo 1.26.0\n",
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
				"unchecked-writer-error": rules.SeverityWarn,
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
	want := []string{
		"buffer.Flush()",
		"tarWriter.Close()",
		"tarWriter.Flush()",
		"zipWriter.Flush()",
		"zipWriter.Close()",
		"flateWriter.Close()",
		"flateWriter.Flush()",
		"gzipWriter.Close()",
		"gzipWriter.Flush()",
		"lzwWriter.Close()",
		"zlibWriter.Flush()",
		"zlibWriter.Close()",
		"xmlEncoder.Close()",
		"xmlEncoder.Flush()",
		"tabWriter.Flush()",
		"quotedWriter.Close()",
		"multipartWriter.Close()",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unchecked-writer-error result = %#v", result)
	}
	searchFrom := strings.Index(input, "func ignored")
	for index, snippet := range want {
		relative := strings.Index(input[searchFrom:], snippet)
		if relative < 0 {
			t.Fatalf("missing fixture snippet %q", snippet)
		}
		start := searchFrom + relative
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "unchecked-writer-error" ||
			diagnostic.MessageKey != "unchecked-writer-error" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(snippet) ||
			!strings.Contains(diagnostic.Message, "buffered output") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(snippet)
	}
}

func TestUncheckedWriterErrorOwnsOverlappingErrorDiscardDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriteroverlap\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
)

func run(buffer *bufio.Writer, zipWriter *zip.Writer, gzipWriter *gzip.Writer) {
	buffer.Flush()
	defer zipWriter.Close()
	_ = gzipWriter.Close()
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
			Presets: []rules.Preset{rules.PresetCorrectness, rules.PresetSuspicious},
			Overrides: map[string]rules.Severity{
				"blank-error-discard": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 3 {
		t.Fatalf("overlapping writer diagnostics = %#v", result.Files)
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "unchecked-writer-error" {
			t.Fatalf("overlapping writer diagnostic = %#v", diagnostic)
		}
	}
}

func TestUncheckedWriterErrorExcludesHandledAndUnprovenFinalizers(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"io"
	"os"
)

type localWriter struct{}
func (*localWriter) Flush() error { return nil }
func (*localWriter) Close() error { return nil }

type gzipWrapper struct { *gzip.Writer }

func handled(buffer *bufio.Writer, gzipWriter *gzip.Writer) error {
	if err := buffer.Flush(); err != nil { return err }
	err := gzipWriter.Close()
	return err
}

func lookalikes(local *localWriter, file *os.File, closer io.Closer, csvWriter *csv.Writer) {
	local.Flush()
	local.Close()
	file.Close()
	closer.Close()
	csvWriter.Flush()
}

func exactForms(gzipWriter *gzip.Writer, wrapped *gzipWrapper) {
	(*gzip.Writer).Close(gzipWriter)
	wrapped.Close()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterprecision\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	want := []string{"(*gzip.Writer).Close(gzipWriter)", "wrapped.Close()"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unchecked-writer-error precision result = %#v", result.Files)
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		content := input[diagnostic.Range.Start:diagnostic.Range.End]
		if content != want[index] {
			t.Fatalf(
				"diagnostic[%d] content = %q, want %q",
				index,
				content,
				want[index],
			)
		}
	}
}

func TestUncheckedWriterErrorHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterpolicy\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import "bufio"

func suppressed(writer *bufio.Writer) {
	//glippy:ignore unchecked-writer-error -- caller accepts best-effort output
	writer.Flush()
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import "bufio"

func generated(writer *bufio.Writer) { writer.Flush() }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import "bufio"

func invalid(writer *bufio.Writer) {
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
	"bufio"
	"testing"
)

func TestFlush(t *testing.T) {
	var writer *bufio.Writer
	writer.Flush()
}
`,
	)
	result := runUncheckedWriterError(t, root, "go1.26", true)
	if len(result.LoadDiagnostics) == 0 || len(result.Files) != 4 {
		t.Fatalf("unchecked-writer-error policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "unchecked-writer-error" {
				t.Fatalf("suppressed writer result = %#v", file)
			}
		case "sample_test.go":
			if len(file.Diagnostics) != 1 ||
				file.Diagnostics[0].RuleID != "unchecked-writer-error" {
				t.Fatalf("test writer result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded writer result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("unchecked-writer-error")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(
			metadata.NodeInterests,
			[]rules.NodeKind{
				rules.NodeExprStmt,
				rules.NodeAssignStmt,
				rules.NodeGoStmt,
				rules.NodeDeferStmt,
			},
		) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("unchecked-writer-error metadata = %#v, found = %t", metadata, found)
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
		if selected.ID == "unchecked-writer-error" {
			t.Fatalf("pre-minimum writer selection = %#v", selection)
		}
	}
}

func runUncheckedWriterError(
	t *testing.T,
	root string,
	goVersion string,
	tests bool,
) analysis.PackageResult {
	t.Helper()
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
				"unchecked-writer-error": rules.SeverityWarn,
			},
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

func BenchmarkUncheckedWriterErrorPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"compress/gzip\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(writer *gzip.Writer) { defer writer.Close() }\n",
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
				"unchecked-writer-error": rules.SeverityWarn,
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
