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

func TestUncheckedWriterErrorTracksStandardLibraryEncoderAcquisitions(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"io"
)

func discarded(output io.Writer) {
	ascii := ascii85.NewEncoder(output)
	ascii.Close()
	base32Writer := base32.NewEncoder(base32.StdEncoding, output)
	defer base32Writer.Close()
	base64Writer := base64.NewEncoder(base64.StdEncoding, output)
	go base64Writer.Close()
	var declared = base64.NewEncoder(base64.RawStdEncoding, output)
	_ = declared.Close()
	base64.NewEncoder(base64.URLEncoding, output).Close()
}

func handled(output io.Writer) error {
	writer := base64.NewEncoder(base64.StdEncoding, output)
	return writer.Close()
}

func reassigned(output io.Writer, replacement io.WriteCloser) {
	writer := base64.NewEncoder(base64.StdEncoding, output)
	writer = replacement
	writer.Close()
}

func unproven(writer io.WriteCloser) { writer.Close() }

type localEncoder struct{}
func (*localEncoder) Write(value []byte) (int, error) { return len(value), nil }
func (*localEncoder) Close() error { return nil }
func NewEncoder(io.Writer) io.WriteCloser { return &localEncoder{} }
func lookalike(output io.Writer) { NewEncoder(output).Close() }
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedencodererror\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	want := []string{
		"ascii.Close()",
		"base32Writer.Close()",
		"base64Writer.Close()",
		"declared.Close()",
		"base64.NewEncoder(base64.URLEncoding, output).Close()",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unchecked encoder result = %#v", result)
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		content := input[diagnostic.Range.Start:diagnostic.Range.End]
		if content != want[index] || diagnostic.RuleID != "unchecked-writer-error" {
			t.Fatalf(
				"encoder diagnostic[%d] = %#v for %q, want %q",
				index,
				diagnostic,
				content,
				want[index],
			)
		}
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
	"encoding/base64"
	"io"
)

func run(
	buffer *bufio.Writer,
	zipWriter *zip.Writer,
	gzipWriter *gzip.Writer,
	output io.Writer,
) {
	buffer.Flush()
	defer zipWriter.Close()
	_ = gzipWriter.Close()
	encoder := base64.NewEncoder(base64.StdEncoding, output)
	encoder.Close()
	blankEncoder := base64.NewEncoder(base64.RawStdEncoding, output)
	_ = blankEncoder.Close()
	deferredEncoder := base64.NewEncoder(base64.URLEncoding, output)
	defer deferredEncoder.Close()
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 6 {
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

func TestUncheckedWriterErrorExcludesTabwriterFlushToBytesBuffer(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"fmt"
	"io"
	"text/tabwriter"
)

func inMemory(parameter *bytes.Buffer) {
	var value bytes.Buffer
	writer := tabwriter.NewWriter(&value, 1, 8, 1, ' ', 0)
	writer.Flush()
	alias := writer
	_ = alias

	declared := tabwriter.NewWriter(parameter, 1, 8, 1, ' ', 0)
	defer declared.Flush()

	tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0).Flush()
}

func inMemoryFormatted() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	fmt.Fprintln(writer, "value")
	writer.Flush()
}

type reinitializer struct {
	writer *tabwriter.Writer
	output io.Writer
}

func (value reinitializer) String() string {
	value.writer.Init(value.output, 1, 8, 1, ' ', 0)
	return ""
}

func formattedEscape(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	fmt.Fprintln(writer, reinitializer{writer: writer, output: output})
	writer.Flush()
}

func inMemoryAlias() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	alias := writer
	alias.Flush()
}

func fallible(output io.Writer) {
	writer := tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func reassigned(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	writer = tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func reassignedToBuffer(output io.Writer) {
	writer := tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	writer = tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	writer.Flush()
}

func conditionallyReassignedToBuffer(output io.Writer, replace bool) {
	writer := tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	if replace {
		writer = tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	}
	writer.Flush()
}

func tupleReassigned(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	writer, _ = nextWriter(output)
	writer.Flush()
}

func reinitialized(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	writer.Init(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func aliasReinitialized(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	alias := writer
	alias.Init(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func methodExpressionReinitialized(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	(*tabwriter.Writer).Init(writer, output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func methodValueReinitialized(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	initialize := writer.Init
	initialize(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func rangeReassigned(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for _, writer = range []*tabwriter.Writer{tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)} {
		break
	}
	writer.Flush()
}

func addressReassigned(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	pointer := &writer
	*pointer = tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func escaped(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	reinitialize(writer, output)
	writer.Flush()
}

func aliasEscaped(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	alias := writer
	reinitialize(alias, output)
	writer.Flush()
}

func reinitialize(writer *tabwriter.Writer, output io.Writer) {
	writer.Init(output, 1, 8, 1, ' ', 0)
}

func nextWriter(output io.Writer) (*tabwriter.Writer, error) {
	return tabwriter.NewWriter(output, 1, 8, 1, ' ', 0), nil
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterbuffer\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	want := []string{
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
		"writer.Flush()",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unchecked in-memory writer result = %#v", result.Files)
	}
	searchFrom := strings.Index(input, "func formattedEscape")
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing fixture snippet %q", want[index])
		}
		start := searchFrom + relative
		content := input[diagnostic.Range.Start:diagnostic.Range.End]
		if content != want[index] || diagnostic.Range.Start != start {
			t.Fatalf(
				"diagnostic[%d] = %#v for %q, want %q at %d",
				index,
				diagnostic,
				content,
				want[index],
				start,
			)
		}
		searchFrom = start + len(want[index])
	}
}

func TestUncheckedWriterErrorDiagnosesDeferredTabwriterReinitialization(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func deferred(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	writer.Init(output, 1, 8, 1, ' ', 0)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterdefer\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorDiagnosesEscapedTabwriterCandidates(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

type holder struct {
	writer *tabwriter.Writer
}

func assignedToField(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	var value holder
	value.writer = writer
	value.writer.Init(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func storedInKeyedLiteral() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	_ = holder{writer: writer}
	writer.Flush()
}

func storedInUnkeyedLiteral() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	_ = holder{writer}
	writer.Flush()
}

func storedInSliceLiteral() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	_ = []*tabwriter.Writer{writer}
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterescape\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"writer.Flush()", "writer.Flush()", "writer.Flush()", "writer.Flush()"},
	)
}

func TestUncheckedWriterErrorDiagnosesEscapedTabwriterClosures(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

type callbackHolder struct {
	callback func()
}

func consume(func()) {}

func passedNamed(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	mutate := func() { writer.Init(output, 1, 8, 1, ' ', 0) }
	consume(mutate)
	writer.Flush()
}

func storedNamed(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	mutate := func() { writer.Init(output, 1, 8, 1, ' ', 0) }
	var value callbackHolder
	value.callback = mutate
	writer.Flush()
}

func passedLiteral(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	consume(func() { writer.Init(output, 1, 8, 1, ' ', 0) })
	writer.Flush()
}

func storedLiteral(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	var value callbackHolder
	value.callback = func() { writer.Init(output, 1, 8, 1, ' ', 0) }
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterclosureescape\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"writer.Flush()", "writer.Flush()", "writer.Flush()", "writer.Flush()"},
	)
}

func TestUncheckedWriterErrorDiagnosesPackageVariableEscape(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

var leaked *tabwriter.Writer

func escaped(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	leaked = writer
	mutate(output)
	writer.Flush()
}

func mutate(output io.Writer) {
	leaked.Init(output, 1, 8, 1, ' ', 0)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterglobal\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorPreservesHarmlessBlankAliasReference(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"text/tabwriter"
)

func inMemory() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	alias := writer
	_ = alias
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterblank\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("harmless blank alias result = %#v", result.Files)
	}
}

func TestUncheckedWriterErrorDiagnosesInterfaceReinitializedTabwriter(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

type initializer interface {
	Init(io.Writer, int, int, int, byte, uint) *tabwriter.Writer
}

func reinitialized(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	var alias initializer = writer
	alias.Init(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterinterface\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorDiagnosesReturnedAndSentTabwriters(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

type initializer interface {
	Init(io.Writer, int, int, int, byte, uint) *tabwriter.Writer
}

func returned(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	escape := func() initializer { return writer }
	alias := escape()
	alias.Init(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}

func sent(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	values := make(chan initializer, 1)
	values <- writer
	alias := <-values
	alias.Init(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterreturn\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"writer.Flush()", "writer.Flush()"},
	)
}

func TestUncheckedWriterErrorDiagnosesIndirectTabwriterReplacement(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func replaced(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	*writer = *tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterindirect\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorModelsDeferredAndClosureExecution(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func deferredRebind(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	writer = tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
}

func unusedMutation(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	mutate := func() { writer.Init(output, 1, 8, 1, ' ', 0) }
	_ = mutate
	writer.Flush()
}

func invokedAfterMutation(output io.Writer) {
	externalWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	flush := func() { externalWriter.Flush() }
	externalWriter.Init(output, 1, 8, 1, ' ', 0)
	flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterexecution\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"externalWriter.Flush()"})
}

func TestUncheckedWriterErrorModelsExactExecutionTimeline(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func completedBeforeMutation(output io.Writer) {
	completedWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	completedWriter.Flush()
	completedWriter.Init(output, 1, 8, 1, ' ', 0)
}

func immediatelyMutated(output io.Writer) {
	iifeWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	func() { iifeWriter.Init(output, 1, 8, 1, ' ', 0) }()
	iifeWriter.Flush()
}

func deferredLIFO(output io.Writer) {
	deferredWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer deferredWriter.Flush()
	defer func() { deferredWriter.Init(output, 1, 8, 1, ' ', 0) }()
}

func stableIIFE() {
	func() {
		localWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
		localWriter.Flush()
	}()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwritertimeline\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"iifeWriter.Flush()", "deferredWriter.Flush()"},
	)
}

func TestUncheckedWriterErrorModelsLoopBackedgeMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func repeated(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for range 2 {
		writer.Flush()
		writer.Init(output, 1, 8, 1, ' ', 0)
	}
}

func repeatedThroughOuterLoop(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for range 2 {
		for range 1 {
			writer.Flush()
		}
		writer.Init(output, 1, 8, 1, ' ', 0)
	}
}

func repeatedInPost(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for index := 0; index < 2; writer.Flush() {
		writer.Init(output, 1, 8, 1, ' ', 0)
		index++
	}
}

func reboundBeforePost(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for index := 0; index < 2; writer.Flush() {
		writer = tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
		index++
	}
}

func reacquiredEachIteration(output io.Writer) {
	for range 2 {
		writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
		writer.Flush()
		writer.Init(output, 1, 8, 1, ' ', 0)
	}
}

func completedBeforeMutation(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for range 2 {
		writer.Flush()
	}
	writer.Init(output, 1, 8, 1, ' ', 0)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterloop\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"writer.Flush()", "writer.Flush()", "writer.Flush()", "writer.Flush()"},
	)
}

func TestUncheckedWriterErrorIgnoresUnreachableLoopBackedges(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func completed(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for {
		writer.Flush()
		writer.Init(output, 1, 8, 1, ' ', 0)
		break
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterunreachablebackedge\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, nil)
}

func TestUncheckedWriterErrorIgnoresNestedBreakLoopBackedges(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func completed(output io.Writer, condition bool) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for {
		writer.Flush()
		if condition {
			writer.Init(output, 1, 8, 1, ' ', 0)
			break
		}
	}
}

func repeatedAfterSwitch(output io.Writer, condition bool) {
	switchWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for {
		switchWriter.Flush()
		switch {
		case condition:
			switchWriter.Init(output, 1, 8, 1, ' ', 0)
			break
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriternestedbreak\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"switchWriter.Flush()"})
}

func TestUncheckedWriterErrorModelsNestedPerIterationReacquisition(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func nested(output io.Writer) {
	for range 2 {
		for range 1 {
			writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
			writer.Flush()
			writer.Init(output, 1, 8, 1, ' ', 0)
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriternestedreacquisition\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, nil)
}

func TestUncheckedWriterErrorModelsDeferredNamedClosureMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func named(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	mutate := func() { writer.Init(output, 1, 8, 1, ' ', 0) }
	defer writer.Flush()
	defer mutate()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterdeferrednamed\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorModelsTabwriterMethodExpressions(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"text/tabwriter"
)

func infallible() {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	(*tabwriter.Writer).Flush(writer)
}

func fallible(fallibleWriter *tabwriter.Writer) {
	(*tabwriter.Writer).Flush(fallibleWriter)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwritermethodexpression\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"(*tabwriter.Writer).Flush(fallibleWriter)"},
	)
}

func TestUncheckedWriterErrorModelsDelayedLoopRebinding(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func deferred(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for range 2 {
		defer writer.Flush()
		writer = tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	}
}

func asynchronous(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	for range 2 {
		go writer.Flush()
		writer = tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterdelayedloop\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{"writer.Flush()", "writer.Flush()"},
	)
}

func TestUncheckedWriterErrorModelsDeferredParameterMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func reinitialize(writer *tabwriter.Writer, output io.Writer) {
	writer.Init(output, 1, 8, 1, ' ', 0)
}

func reinitializeTransitively(writer *tabwriter.Writer, output io.Writer) {
	reinitialize(writer, output)
}

func reinitializeAlias(writer *tabwriter.Writer, output io.Writer) {
	alias := writer
	alias.Init(output, 1, 8, 1, ' ', 0)
}

func named(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitialize(writer, output)
}

func literal(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	reinitialize := func(target *tabwriter.Writer) {
		target.Init(output, 1, 8, 1, ' ', 0)
	}
	defer writer.Flush()
	defer reinitialize(writer)
}

func nested(output io.Writer) {
	run := func() {
		writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
		defer writer.Flush()
		defer reinitialize(writer, output)
	}
	_ = run
}

func transitive(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitializeTransitively(writer, output)
}

func aliased(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitializeAlias(writer, output)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterdeferredparameter\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{
			"writer.Flush()",
			"writer.Flush()",
			"writer.Flush()",
			"writer.Flush()",
			"writer.Flush()",
		},
	)
}

func TestUncheckedWriterErrorModelsConditionalParameterAliasMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func reinitialize(
	writer *tabwriter.Writer,
	other *tabwriter.Writer,
	output io.Writer,
	replace bool,
) {
	alias := writer
	if replace {
		alias = other
	}
	alias.Init(output, 1, 8, 1, ' ', 0)
}

func reinitializeOther(
	writer *tabwriter.Writer,
	other *tabwriter.Writer,
	output io.Writer,
) {
	alias := writer
	alias = other
	alias.Init(output, 1, 8, 1, ' ', 0)
}

func deferred(output io.Writer, replace bool) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	other := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitialize(writer, other, output, replace)
}

func stable(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	other := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitializeOther(writer, other, output)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterconditionalalias\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorModelsNamedLocalFunctionParameterMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func reinitialize(writer *tabwriter.Writer, output io.Writer) {
	mutate := func(target *tabwriter.Writer) {
		target.Init(output, 1, 8, 1, ' ', 0)
	}
	mutate(writer)
}

func deferred(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitialize(writer, output)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriternamedlocal\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorTracksParameterAliasesWithinExactBranches(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func mutateOtherBranch(
	writer *tabwriter.Writer,
	other *tabwriter.Writer,
	output io.Writer,
	choose bool,
) {
	alias := writer
	if choose {
		alias = other
		alias.Init(output, 1, 8, 1, ' ', 0)
	}
}

func mutateWriterBranch(
	writer *tabwriter.Writer,
	other *tabwriter.Writer,
	output io.Writer,
	choose bool,
) {
	alias := writer
	if choose {
		alias = other
	} else {
		alias.Init(output, 1, 8, 1, ' ', 0)
	}
}

func stable(output io.Writer, choose bool) {
	stableWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	other := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer stableWriter.Flush()
	defer mutateOtherBranch(stableWriter, other, output, choose)
}

func fallible(output io.Writer, choose bool) {
	fallibleWriter := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	other := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer fallibleWriter.Flush()
	defer mutateWriterBranch(fallibleWriter, other, output, choose)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterbranchalias\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"fallibleWriter.Flush()"})
}

func TestUncheckedWriterErrorTracksParameterAliasesAcrossLoopBackedges(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func reinitialize(
	writer *tabwriter.Writer,
	other *tabwriter.Writer,
	output io.Writer,
) {
	alias := other
	for range 2 {
		alias.Init(output, 1, 8, 1, ' ', 0)
		alias = writer
	}
}

func deferred(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	other := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitialize(writer, other, output)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterloopalias\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, []string{"writer.Flush()"})
}

func TestUncheckedWriterErrorModelsCrossFileDeferredParameterMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwritercrossfile\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper.go"),
		`package sample

import (
	"io"
	"text/tabwriter"
)

func reinitialize(writer *tabwriter.Writer, output io.Writer) {
	writer.Init(output, 1, 8, 1, ' ', 0)
}
`,
	)
	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"
)

func deferred(output io.Writer) {
	writer := tabwriter.NewWriter(bytes.NewBuffer(nil), 1, 8, 1, ' ', 0)
	defer writer.Flush()
	defer reinitialize(writer, output)
}
`
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	diagnostics := make([]rules.Diagnostic, 0)
	for _, file := range result.Files {
		diagnostics = append(diagnostics, file.Diagnostics...)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("cross-file deferred mutation result = %#v", result.Files)
	}
	diagnostic := diagnostics[0]
	if content := input[diagnostic.Range.Start:diagnostic.Range.End];
		content != "writer.Flush()" {
		t.Fatalf("cross-file deferred mutation diagnostic = %q", content)
	}
}

func assertUncheckedWriterDiagnostics(
	t *testing.T,
	input string,
	result analysis.PackageResult,
	want []string,
) {
	t.Helper()
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unchecked-writer-error result = %#v", result.Files)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing fixture snippet %q", want[index])
		}
		start := searchFrom + relative
		content := input[diagnostic.Range.Start:diagnostic.Range.End]
		if diagnostic.RuleID != "unchecked-writer-error" ||
			content != want[index] ||
			diagnostic.Range.Start != start {
			t.Fatalf(
				"diagnostic[%d] = %#v for %q, want %q at %d",
				index,
				diagnostic,
				content,
				want[index],
				start,
			)
		}
		searchFrom = start + len(want[index])
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

func BenchmarkUncheckedWriterErrorTabwriterStability(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedtabwriterbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport (\"bytes\"; \"text/tabwriter\")\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() { var output bytes.Buffer; writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0); writer.Flush() }\n",
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
		0,
	)
}

func BenchmarkUncheckedWriterErrorEncoderAcquisition(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedencoderbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport (\"encoding/base64\"; \"io\")\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(output io.Writer) { writer := base64.NewEncoder(base64.StdEncoding, output); defer writer.Close() }\n",
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

func BenchmarkUncheckedWriterErrorNestedClosures(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriternestedclosures\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString(
		"package sample\n\nimport (\"io\"; \"text/tabwriter\")\n\n" +
			"func nested(writer *tabwriter.Writer, output io.Writer) {\n",
	)
	for depth := range 100 {
		fmt.Fprintf(&input, "f%d := func(writer *tabwriter.Writer) {\n", depth)
	}
	input.WriteString("writer.Init(output, 1, 8, 1, ' ', 0)\n")
	for depth := 99; depth >= 0; depth-- {
		fmt.Fprintf(&input, "}\n_ = f%d\n", depth)
	}
	input.WriteString("}\n")
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
		0,
	)
}
