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

func TestUncheckedWriterErrorExcludesStableInMemoryWriterChains(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
	"strings"
	"text/tabwriter"
)

func direct() {
	var bytesOutput bytes.Buffer
	bufio.NewWriter(&bytesOutput).Flush()
	gzip.NewWriter(&bytesOutput).Close()
	bufio.NewWriter(bufio.NewWriter(&bytesOutput)).Flush()

	var stringOutput strings.Builder
	tabwriter.NewWriter(&stringOutput, 1, 8, 1, ' ', 0).Flush()
}

func stableChain() {
	var output bytes.Buffer
	table := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	buffered := bufio.NewWriter(table)
	buffered.Flush()

	inner := bufio.NewWriter(&output)
	outer := bufio.NewWriter(inner)
	outer.Flush()
}

func resetChain(output io.Writer) {
	var memory bytes.Buffer
	inner := bufio.NewWriter(&memory)
	pooled := bufio.NewWriter(output)
	pooled.Reset(inner)
	pooled.Flush()
}

func resetGzip(output io.Writer) {
	var memory bytes.Buffer
	pooled := gzip.NewWriter(output)
	pooled.Reset(&memory)
	pooled.Close()
}

func initializeTabwriter(output io.Writer) {
	var memory strings.Builder
	pooled := tabwriter.NewWriter(output, 1, 8, 1, ' ', 0)
	pooled.Init(&memory, 1, 8, 1, ' ', 0)
	pooled.Flush()
}

func invalidGzipHeader() {
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	header := &writer.Header
	header.Name = "snowman: ☃"
	writer.Close()
}

func invalidDirectGzipName() {
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	writer.Name = "snowman: ☃"
	writer.Close()
}

func invalidDirectGzipComment() {
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	writer.Comment = "snowman: ☃"
	writer.Close()
}

func invalidDirectGzipExtra() {
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	writer.Extra = make([]byte, 1<<16)
	writer.Close()
}

func callerOwned(
	buffered *bufio.Writer,
	compressed *gzip.Writer,
	table *tabwriter.Writer,
) {
	var output bytes.Buffer
	buffered.Reset(&output)
	buffered.Flush()
	compressed.Reset(&output)
	compressed.Close()
	table.Init(&output, 1, 8, 1, ' ', 0)
	table.Flush()
}

func identityWriter(writer *bufio.Writer) *bufio.Writer { return writer }

func indirectCallerOwned(writer *bufio.Writer) {
	var output bytes.Buffer
	local := identityWriter(writer)
	local.Reset(&output)
	local.Flush()
}

func conditionalReset(output io.Writer, reset bool) {
	var memory bytes.Buffer
	writer := bufio.NewWriter(output)
	if reset {
		writer.Reset(&memory)
	}
	writer.Flush()
}

func loopReset(output io.Writer, reset bool) {
	var memory bytes.Buffer
	writer := bufio.NewWriter(output)
	for reset {
		writer.Reset(&memory)
		break
	}
	writer.Flush()
}

func consumeWriter(*bufio.Writer) {}

func escapedReset(output io.Writer) {
	var memory bytes.Buffer
	writer := bufio.NewWriter(output)
	consumeWriter(writer)
	writer.Reset(&memory)
	writer.Flush()
}

func independentFinalizer() {
	var output bytes.Buffer
	xml.NewEncoder(&output).Close()
}

func fallible(output io.Writer, pooled *bufio.Writer) {
	bufio.NewWriter(output).Flush()
	gzip.NewWriter(output).Close()
	tabwriter.NewWriter(output, 1, 8, 1, ' ', 0).Flush()
	pooled.Reset(output)
	pooled.Flush()
}

func mutatedInner(output io.Writer) {
	var memory bytes.Buffer
	inner := bufio.NewWriter(&memory)
	outer := bufio.NewWriter(inner)
	inner.Reset(output)
	outer.Flush()
}

func reboundNestedWriter(output io.Writer) {
	var memory bytes.Buffer
	inner := bufio.NewWriterSize(output, 1)
	outer := bufio.NewWriterSize(inner, 1)
	_, _ = outer.Write([]byte("xx"))
	inner.Reset(&memory)
	outer.Flush()
}

func transitiveReboundNestedWriter(output io.Writer) {
	var memory bytes.Buffer
	inner := bufio.NewWriterSize(output, 1)
	middle := bufio.NewWriterSize(inner, 1)
	outer := bufio.NewWriterSize(middle, 1)
	_, _ = outer.Write([]byte("xxx"))
	inner.Reset(&memory)
	outer.Flush()
}

func resetReboundNestedWriter(output io.Writer) {
	var memory bytes.Buffer
	inner := bufio.NewWriterSize(output, 1)
	outer := bufio.NewWriterSize(&memory, 1)
	outer.Reset(inner)
	_, _ = outer.Write([]byte("xx"))
	inner.Reset(&memory)
	outer.Flush()
}

func inlineReboundNestedWriter(output io.Writer) {
	var memory bytes.Buffer
	inner := bufio.NewWriterSize(output, 1)
	outer := bufio.NewWriterSize(bufio.NewWriterSize(inner, 1), 1)
	_, _ = outer.Write([]byte("xxx"))
	inner.Reset(&memory)
	outer.Flush()
}

func interfaceSink() {
	var output io.Writer = &bytes.Buffer{}
	bufio.NewWriter(output).Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterchains\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{
			"writer.Close()",
			"writer.Close()",
			"writer.Close()",
			"writer.Close()",
			"buffered.Flush()",
			"compressed.Close()",
			"table.Flush()",
			"local.Flush()",
			"writer.Flush()",
			"writer.Flush()",
			"writer.Flush()",
			"xml.NewEncoder(&output).Close()",
			"bufio.NewWriter(output).Flush()",
			"gzip.NewWriter(output).Close()",
			"tabwriter.NewWriter(output, 1, 8, 1, ' ', 0).Flush()",
			"pooled.Flush()",
			"outer.Flush()",
			"outer.Flush()",
			"outer.Flush()",
			"outer.Flush()",
			"outer.Flush()",
			"bufio.NewWriter(output).Flush()",
		},
	)
}

func TestUncheckedWriterErrorExcludesStableInMemoryWriterChainsInTestVariants(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwritertestvariants\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), "package sample\n")
	input := `package sample

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
	"text/tabwriter"
)

func TestStableInMemoryWriters(t *testing.T) {
	var bytesOutput bytes.Buffer
	buffered := bufio.NewWriter(&bytesOutput)
	buffered.Flush()

	compressedOutput := bytes.NewBuffer(nil)
	compressed := gzip.NewWriter(compressedOutput)
	compressed.Close()

	var stringOutput strings.Builder
	table := tabwriter.NewWriter(&stringOutput, 1, 8, 1, ' ', 0)
	table.Flush()

	inner := tabwriter.NewWriter(&bytesOutput, 1, 8, 1, ' ', 0)
	outer := bufio.NewWriter(inner)
	outer.Flush()
}
`
	writeFixture(t, filepath.Join(root, "sample_test.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", true)
	for _, file := range result.Files {
		if len(file.Diagnostics) != 0 {
			t.Fatalf("test-variant in-memory writer result = %#v", result.Files)
		}
	}
}

func TestUncheckedWriterErrorPreservesInMemoryProvenanceThroughWriterConsumers(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"text/tabwriter"
	"text/template"
)

type namedValue uint32

func writeXML(output io.Writer) error {
	_, err := output.Write([]byte("payload"))
	return err
}

func localWriterInterfaceConsumer() {
	var output bytes.Buffer
	writer := bufio.NewWriter(&output)
	_ = writeXML(writer)
	writer.Flush()
}

func standardWriterInterfaceConsumers() {
	var output bytes.Buffer
	compressed := gzip.NewWriter(&output)
	_, _ = io.Copy(compressed, bytes.NewBufferString("payload"))
	compressed.Close()

	buffered := bufio.NewWriter(&output)
	_ = template.Must(template.New("value").Parse("value")).Execute(buffered, nil)
	buffered.Flush()
}

func formattingCallbacksCannotRebindWriter() {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	_, _ = fmt.Fprintf(writer, "%v", namedValue(1))
	writer.Flush()
}

func harmlessCallbackCapture(run func(func() bool)) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	run(func() bool {
		_, _ = fmt.Fprint(writer, "payload")
		return true
	})
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterconsumers\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(t, input, result, nil)
}

func TestUncheckedWriterErrorRejectsWriterConsumerRebindingAndExposure(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"io"
	"text/tabwriter"

	"example.com/writerconsumer"
)

type writerMutatingSource struct {
	fallback io.Writer
}

func (writerMutatingSource) Read([]byte) (int, error) { return 0, io.EOF }

func (source writerMutatingSource) WriteTo(destination io.Writer) (int64, error) {
	destination.(*tabwriter.Writer).Init(source.fallback, 1, 8, 1, ' ', 0)
	return 0, nil
}

type readerMutatingSource struct {
	destination *tabwriter.Writer
	fallback    io.Writer
}

func (source readerMutatingSource) Read([]byte) (int, error) {
	source.destination.Init(source.fallback, 1, 8, 1, ' ', 0)
	return 0, io.EOF
}

func rebind(output io.Writer, fallback io.Writer) {
	output.(*tabwriter.Writer).Init(fallback, 1, 8, 1, ' ', 0)
}

var storedWriter io.Writer

func storeWriter(output io.Writer) { storedWriter = output }

func rebindStoredWriter(fallback io.Writer) {
	storedWriter.(*tabwriter.Writer).Init(fallback, 1, 8, 1, ' ', 0)
}

func relayImported(output io.Writer, fallback io.Writer) {
	writerconsumer.Rebind(output, fallback)
}

func relayDynamic(output io.Writer, consume func(io.Writer)) { consume(output) }

func interfaceRebinding(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	rebind(writer, fallback)
	writer.Flush()
}

func callbackExposure(run func(func() *tabwriter.Writer), fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	run(func() *tabwriter.Writer { return writer })
	writer.Flush()
}

func importedInterfaceRebinding(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	writerconsumer.Rebind(writer, fallback)
	writer.Flush()
}

func localInterfaceExposure(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	storeWriter(writer)
	rebindStoredWriter(fallback)
	writer.Flush()
}

func importedWrapperRebinding(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	relayImported(writer, fallback)
	writer.Flush()
}

func dynamicWrapperRebinding(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	relayDynamic(writer, func(output io.Writer) {
		output.(*tabwriter.Writer).Init(fallback, 1, 8, 1, ' ', 0)
	})
	writer.Flush()
}

func copySourceRebinding(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	_, _ = io.Copy(writer, writerMutatingSource{fallback: fallback})
	writer.Flush()
}

func copyReaderRebinding(fallback io.Writer) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	_, _ = io.Copy(writer, readerMutatingSource{destination: writer, fallback: fallback})
	writer.Flush()
}

func dynamicCopySource(source io.Reader) {
	var output bytes.Buffer
	writer := tabwriter.NewWriter(&output, 1, 8, 1, ' ', 0)
	_, _ = io.Copy(writer, source)
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterconsumerescape\n\ngo 1.26.0\n\n" +
			"require example.com/writerconsumer v0.0.0\n" +
			"replace example.com/writerconsumer => ./writerconsumer\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "writerconsumer", "go.mod"),
		"module example.com/writerconsumer\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "writerconsumer", "consumer.go"),
		`package writerconsumer

import (
	"io"
	"text/tabwriter"
)

func Rebind(output io.Writer, fallback io.Writer) {
	output.(*tabwriter.Writer).Init(fallback, 1, 8, 1, ' ', 0)
}
`,
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
			"writer.Flush()",
			"writer.Flush()",
			"writer.Flush()",
			"writer.Flush()",
		},
	)
}

func TestUncheckedWriterErrorTracksStraightLineReturnedWriterRebinding(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bufio"
	"bytes"
	"io"
	"sync"
)

type writerPool struct{}

func (*writerPool) Get(output io.Writer) *bufio.Writer {
	writer := bufio.NewWriter(io.Discard)
	writer.Reset(output)
	return writer
}

type syncWriterPool struct {
	values sync.Pool
}

func (pool *syncWriterPool) Get(output io.Writer) *bufio.Writer {
	writer := pool.values.Get().(*bufio.Writer)
	writer.Reset(output)
	return writer
}

func stable() {
	var output bytes.Buffer
	inner := bufio.NewWriter(&output)
	outer := new(writerPool).Get(inner)
	outer.Flush()
	inner.Flush()

	pooled := new(syncWriterPool).Get(&output)
	pooled.Flush()
}

var sharedWriter = bufio.NewWriter(io.Discard)

func exposed(output io.Writer) *bufio.Writer {
	writer := sharedWriter
	writer.Reset(output)
	return writer
}

func exposedBeforeReset(output io.Writer) *bufio.Writer {
	writer := bufio.NewWriter(io.Discard)
	sharedWriter = writer
	writer.Reset(output)
	return writer
}

var writerSlot **bufio.Writer

func exposedBeforeAcquisition(output io.Writer) *bufio.Writer {
	var writer *bufio.Writer
	writerSlot = &writer
	writer = bufio.NewWriter(io.Discard)
	writer.Reset(output)
	return writer
}

func exposedUse() {
	var memory bytes.Buffer
	writer := exposed(&memory)
	writer.Flush()

	writer = exposedBeforeReset(&memory)
	writer.Flush()

	writer = exposedBeforeAcquisition(&memory)
	writer.Flush()
}

func conditional(output io.Writer, useOutput bool) *bufio.Writer {
	writer := bufio.NewWriter(io.Discard)
	if useOutput {
		writer.Reset(output)
	}
	return writer
}

func uncertain(output io.Writer, useOutput bool) {
	var memory bytes.Buffer
	writer := conditional(&memory, useOutput)
	writer.Flush()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedwriterreturnedrebind\n\ngo 1.26.0\n",
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

func TestUncheckedWriterErrorExcludesOnlyUnusedInMemoryTarFinalization(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"strings"
)

func empty() {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	writer.Close()
	tar.NewWriter(&output).Close()
	var text strings.Builder
	tar.NewWriter(&text).Close()
}

func used() {
	var output bytes.Buffer
	usedWriter := tar.NewWriter(&output)
	_ = usedWriter.WriteHeader(&tar.Header{Name: "entry"})
	usedWriter.Close()
}

func interfaceSink() {
	var output io.Writer = &bytes.Buffer{}
	interfaceWriter := tar.NewWriter(output)
	interfaceWriter.Close()
}

func argumentUse() {
	var output bytes.Buffer
	argumentWriter := tar.NewWriter(&output)
	fmt.Fprint(argumentWriter, "payload")
	argumentWriter.Close()
}

func methodValueUse() {
	var output bytes.Buffer
	methodValueWriter := tar.NewWriter(&output)
	writeHeader := methodValueWriter.WriteHeader
	_ = writeHeader(&tar.Header{Name: "entry"})
	methodValueWriter.Close()
}

func deferredEmpty() {
	var output bytes.Buffer
	defer tar.NewWriter(&output).Close()
}

func asynchronousEmpty() {
	var output bytes.Buffer
	go tar.NewWriter(&output).Close()
}

func deferredUsed() {
	var output bytes.Buffer
	deferredWriter := tar.NewWriter(&output)
	defer deferredWriter.Close()
	_ = deferredWriter.WriteHeader(&tar.Header{Name: "entry"})
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/uncheckedemptytar\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runUncheckedWriterError(t, root, "go1.26", false)
	assertUncheckedWriterDiagnostics(
		t,
		input,
		result,
		[]string{
			"usedWriter.Close()",
			"interfaceWriter.Close()",
			"argumentWriter.Close()",
			"methodValueWriter.Close()",
			"tar.NewWriter(&output).Close()",
			"tar.NewWriter(&output).Close()",
			"deferredWriter.Close()",
		},
	)
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
