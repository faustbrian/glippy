package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestWriterNotFinalizedRequiresCompletionOnSuccessfulUsedPaths(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
)

func missingGzip(output io.Writer) error {
	writer := gzip.NewWriter(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func missingMultipart(output io.Writer) error {
	writer := multipart.NewWriter(output)
	if _, err := writer.CreateFormField("value"); err != nil {
		return err
	}
	return nil
}

func missingTarFS(output io.Writer, input fs.FS) error {
	writer := tar.NewWriter(output)
	if err := writer.AddFS(input); err != nil {
		return err
	}
	return nil
}

func partial(output io.Writer, finalize bool) error {
	writer := gzip.NewWriter(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	if finalize {
		return writer.Close()
	}
	return nil
}

func lost(output io.Writer) error {
	writer := gzip.NewWriter(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	writer = gzip.NewWriter(io.Discard)
	return nil
}

func missingWithoutErrorResult(output io.Writer) {
	writer := tar.NewWriter(output)
	_ = writer.WriteHeader(&tar.Header{Name: "entry"})
}

func finalized(output io.Writer) error {
	writer := gzip.NewWriter(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return writer.Close()
}

func deferred(output io.Writer) {
	writer := gzip.NewWriter(output)
	defer writer.Close()
	_, _ = writer.Write([]byte("payload"))
}

func failureOnly(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return errors.New("failed")
}

func unused(output io.Writer) {
	_ = gzip.NewWriter(output)
}

func transferred(output io.Writer) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	consume(writer)
}

func returned(output io.Writer) io.WriteCloser {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return writer
}

func sent(output io.Writer, destination chan<- io.WriteCloser) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	destination <- writer
}

type holder struct { writer io.WriteCloser }
func stored(output io.Writer) holder {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return holder{writer: writer}
}

func methodTransferred(output io.Writer) func() error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return writer.Close
}

func asynchronous(output io.Writer) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	go writer.Close()
}

func bufferedOnly() {
	var output bytes.Buffer
	_, _ = output.WriteString("payload")
}

type localWriter struct{}
func (*localWriter) Write(value []byte) (int, error) { return len(value), nil }
func (*localWriter) Close() error { return nil }
func local() {
	writer := &localWriter{}
	_, _ = writer.Write([]byte("payload"))
}

func consume(io.Writer) {}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/writernotfinalized\n\ngo 1.26.0\n",
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
				"writer-not-finalized": rules.SeverityWarn,
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
		"writer := gzip.NewWriter(output)",
		"writer := multipart.NewWriter(output)",
		"writer := tar.NewWriter(output)",
		"writer := gzip.NewWriter(output)",
		"writer := gzip.NewWriter(output)",
		"writer := tar.NewWriter(output)",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("writer-not-finalized result = %#v", result)
	}
	searchFrom := 0
	for index, snippet := range want {
		relative := strings.Index(input[searchFrom:], snippet)
		if relative < 0 {
			t.Fatalf("missing fixture snippet %q", snippet)
		}
		start := searchFrom + relative + strings.Index(snippet, "writer")
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "writer-not-finalized" ||
			diagnostic.MessageKey != "writer-not-finalized" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("writer") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("writer-not-finalized diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len("writer")
	}
}

func TestWriterNotFinalizedOwnsGenericResourceDiagnostic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/writerownership\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"compress/gzip"
	"io"
)

func encode(output io.Writer) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
}
`,
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("writer-not-finalized")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		metadata.Requirement != rules.RequireControlFlow ||
		len(metadata.Presets) != 1 ||
		metadata.Presets[0] != rules.PresetCorrectness ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("writer-not-finalized metadata = %#v, found = %t", metadata, found)
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
		if selected.ID == "writer-not-finalized" {
			t.Fatalf("pre-minimum writer finalization selection = %#v", selection)
		}
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{rules.PresetCorrectness, rules.PresetSuspicious},
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
	if len(result.Files) != 1 ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "writer-not-finalized" {
		t.Fatalf("writer finalization ownership = %#v", result.Files)
	}
}

func TestWriterNotFinalizedHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/writerpolicy\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import (
	"compress/gzip"
	"io"
)

func suppressed(output io.Writer) {
	//glippy:ignore writer-not-finalized -- caller accepts incomplete output
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import (
	"compress/gzip"
	"io"
)

func generated(output io.Writer) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import (
	"compress/gzip"
	"io"
)

func invalid(output io.Writer) {
	var broken string = 1
	_ = broken
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "sample_test.go"),
		`package sample

import (
	"compress/gzip"
	"io"
	"testing"
)

func TestEncode(t *testing.T) {
	var output io.Writer = io.Discard
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
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
				"writer-not-finalized": rules.SeverityError,
			},
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			Tests: true,
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.LoadDiagnostics) == 0 || len(result.Files) != 4 {
		t.Fatalf("writer finalization policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "writer-not-finalized" {
				t.Fatalf("suppressed writer finalization result = %#v", file)
			}
		case "sample_test.go":
			if len(file.Diagnostics) != 1 ||
				file.Diagnostics[0].RuleID != "writer-not-finalized" ||
				file.Diagnostics[0].Severity != rules.SeverityError {
				t.Fatalf("test writer finalization result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded writer finalization result = %#v", file)
			}
		default:
			t.Fatalf("unexpected writer policy file %q", file.Path)
		}
	}
}

func BenchmarkWriterNotFinalizedSharedCFG(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/writerfinalizationbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport (\"compress/gzip\"; \"io\")\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(output io.Writer) { writer := gzip.NewWriter(output); _, _ = writer.Write([]byte(\"payload\")) }\n",
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
				"writer-not-finalized": rules.SeverityWarn,
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
