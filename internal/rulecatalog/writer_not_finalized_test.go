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
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
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

func missingBase64(output io.Writer) error {
	writer := base64.NewEncoder(base64.StdEncoding, output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func missingDeclaredBase64(output io.Writer) error {
	var writer = base64.NewEncoder(base64.StdEncoding, output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func missingDeclaredGzipLevel(output io.Writer) error {
	var writer, err = gzip.NewWriterLevel(output, gzip.BestSpeed)
	if err != nil {
		return err
	}
	if _, err = writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func missingAscii85(output io.Writer) error {
	writer := ascii85.NewEncoder(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func missingBase32(output io.Writer) error {
	writer := base32.NewEncoder(base32.StdEncoding, output)
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

func missingInlineSink() {
	writer := tar.NewWriter(bytes.NewBuffer(nil))
	_ = writer.WriteHeader(&tar.Header{Name: "entry"})
}

func finalized(output io.Writer) error {
	writer := gzip.NewWriter(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return writer.Close()
}

func finalizedBase64(output io.Writer) error {
	writer := base64.NewEncoder(base64.StdEncoding, output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return writer.Close()
}

func finalizedDeclaredBase64(output io.Writer) error {
	var writer = base64.NewEncoder(base64.StdEncoding, output)
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
	_ = base64.NewEncoder(base64.StdEncoding, output)
}

func transferred(output io.Writer) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	consume(writer)
	encoder := base64.NewEncoder(base64.StdEncoding, output)
	_, _ = encoder.Write([]byte("payload"))
	consume(encoder)
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
		"writer := base64.NewEncoder(base64.StdEncoding, output)",
		"var writer = base64.NewEncoder(base64.StdEncoding, output)",
		"var writer, err = gzip.NewWriterLevel(output, gzip.BestSpeed)",
		"writer := ascii85.NewEncoder(output)",
		"writer := base32.NewEncoder(base32.StdEncoding, output)",
		"writer := multipart.NewWriter(output)",
		"writer := tar.NewWriter(output)",
		"writer := gzip.NewWriter(output)",
		"writer := gzip.NewWriter(output)",
		"writer := tar.NewWriter(output)",
		"writer := tar.NewWriter(bytes.NewBuffer(nil))",
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

func TestWriterNotFinalizedRecognizesPipeWriterAbortPaths(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"

	pkgerrors "github.com/pkg/errors"
)

func aborted() io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		if err := writer.WriteHeader(&tar.Header{Name: "entry"}); err != nil {
			_ = output.CloseWithError(fmt.Errorf("write archive: %w", err))
			return
		}
		if err := writer.Close(); err != nil {
			_ = output.CloseWithError(fmt.Errorf("finalize archive: %w", err))
			return
		}
		_ = output.Close()
	}()
	return reader
}

func nilAbort() io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_ = output.CloseWithError(nil)
	}()
	return reader
}

func uncertainAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_ = output.CloseWithError(failure)
	}()
	return reader
}

func constructedAbort() io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_ = output.CloseWithError(errors.New("abort archive"))
	}()
	return reader
}

func pkgConstructedAbort() io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_ = output.CloseWithError(pkgerrors.Errorf("abort archive"))
	}()
	return reader
}

func guardedAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		if failure != nil {
			_ = output.CloseWithError(failure)
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func wrappedAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		if failure == io.EOF {
			_ = output.CloseWithError(pkgerrors.Wrap(failure, "abort archive"))
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func uncertainWrappedAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_ = output.CloseWithError(pkgerrors.Wrap(failure, "abort archive"))
	}()
	return reader
}

func reassignedWrappedAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		if failure != nil {
			failure = nil
			_ = output.CloseWithError(pkgerrors.Wrapf(failure, "abort archive"))
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func clearFailure(target *error) { *target = nil }

func clearedAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		if failure != nil {
			clearFailure(&failure)
			_ = output.CloseWithError(failure)
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func preGuardAliasAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		alias := &failure
		if failure != nil {
			*alias = nil
			_ = output.CloseWithError(failure)
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func preGuardClosureAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		clear := func() { failure = nil }
		if failure != nil {
			clear()
			_ = output.CloseWithError(failure)
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func guardInitializerAliasAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		if alias := &failure; failure != nil {
			*alias = nil
			_ = output.CloseWithError(failure)
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func outerErrorAliasAbort(failure error) io.Reader {
	reader, output := io.Pipe()
	alias := &failure
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		if failure != nil {
			*alias = nil
			_ = output.CloseWithError(failure)
			return
		}
		_ = writer.Close()
	}()
	return reader
}

func preConstructionAliasAbort() io.Reader {
	reader, output := io.Pipe()
	go func() {
		alias := &output
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_, replacement := io.Pipe()
		*alias = replacement
		_ = output.CloseWithError(errors.New("abort replacement"))
	}()
	return reader
}

func preConstructionClosureAbort() io.Reader {
	reader, output := io.Pipe()
	go func() {
		rebind := func(replacement *io.PipeWriter) { output = replacement }
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_, replacement := io.Pipe()
		rebind(replacement)
		_ = output.CloseWithError(errors.New("abort replacement"))
	}()
	return reader
}

func outerSinkAliasAbort() io.Reader {
	reader, output := io.Pipe()
	alias := &output
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_, replacement := io.Pipe()
		*alias = replacement
		_ = output.CloseWithError(errors.New("abort replacement"))
	}()
	return reader
}

func replacementPipeAbort() io.Reader {
	reader, output := io.Pipe()
	go func() {
		writer := tar.NewWriter(output)
		_ = writer.WriteHeader(&tar.Header{Name: "entry"})
		_, output = io.Pipe()
		_ = output.CloseWithError(errors.New("abort replacement"))
	}()
	return reader
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/writerpipeabort\n\ngo 1.26.0\n\n" +
			"require github.com/pkg/errors v0.0.0\n" +
			"replace github.com/pkg/errors => ./pkgerrors\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "pkgerrors", "go.mod"),
		"module github.com/pkg/errors\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "pkgerrors", "errors.go"),
		`package errors

import "fmt"

func Errorf(format string, arguments ...any) error {
	return fmt.Errorf(format, arguments...)
}

func Wrap(cause error, message string) error {
	if cause == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func Wrapf(cause error, format string, arguments ...any) error {
	if cause == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(arguments, cause)...)
}
`,
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
	wantFunctions := []string{
		"nilAbort",
		"uncertainAbort",
		"uncertainWrappedAbort",
		"reassignedWrappedAbort",
		"clearedAbort",
		"preGuardAliasAbort",
		"preGuardClosureAbort",
		"guardInitializerAliasAbort",
		"outerErrorAliasAbort",
		"preConstructionAliasAbort",
		"preConstructionClosureAbort",
		"outerSinkAliasAbort",
		"replacementPipeAbort",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(wantFunctions) {
		t.Fatalf("pipe-writer abort result = %#v", result.Files)
	}
	for index, function := range wantFunctions {
		functionStart := strings.Index(input, "func " + function + "(")
		if functionStart < 0 {
			t.Fatalf("missing function %q", function)
		}
		relative := strings.Index(input[functionStart:], "writer := tar.NewWriter(output)")
		if relative < 0 {
			t.Fatalf("missing writer acquisition in %q", function)
		}
		start := functionStart + relative
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "writer-not-finalized" ||
			diagnostic.MessageKey != "writer-not-finalized" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("writer") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("pipe-writer abort diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestWriterNotFinalizedRecognizesProvenNilNamedErrorReturns(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"compress/gzip"
	"errors"
	"io"
)

func untouched(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	if _, writeErr := writer.Write([]byte("payload")); writeErr != nil {
		return writeErr
	}
	return
}

func explicit(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return err
}

func assignedNil(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	err = nil
	return
}

func explicitNil(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return nil
}

func selfAssigned(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	err = err
	return
}

func guardedAfterAcquisition(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, err = writer.Write([]byte("payload"))
	if err != nil {
		return
	}
	return
}

func guardedBeforeAcquisition(output io.Writer) (err error) {
	err = externalError()
	if err != nil {
		return
	}
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return
}

func equalGuardBeforeAcquisition(output io.Writer) (err error) {
	err = externalError()
	if err == nil {
		writer := gzip.NewWriter(output)
		_, _ = writer.Write([]byte("payload"))
		return
	}
	return err
}

func reverseGuardBeforeAcquisition(output io.Writer) (err error) {
	err = externalError()
	if nil != err {
		return
	}
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return
}

func unknown(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	err = externalError()
	return
}

func failure(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	err = errors.New("failed")
	return
}

func deferredMutation(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	defer func() { err = errors.New("failed") }()
	return
}

func deferredExplicitNil(output io.Writer) (err error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	defer func() { err = errors.New("failed") }()
	return nil
}

func assignedBeforeAcquisition(output io.Writer) (err error) {
	err = externalError()
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return
}

func escapedBeforeAcquisition(output io.Writer) (err error) {
	destination := &err
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	_ = destination
	return
}

func nonNilBranch(output io.Writer) (err error) {
	err = externalError()
	if err == nil {
		return
	}
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return
}

func escapedGuard(output io.Writer) (err error) {
	destination := &err
	err = externalError()
	if err != nil {
		return
	}
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	_ = destination
	return
}

func compoundGuard(output io.Writer, ready bool) (err error) {
	err = externalError()
	if err != nil && ready {
		return
	}
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return
}

func reassignedAfterGuard(output io.Writer) (err error) {
	err = externalError()
	if err != nil {
		return
	}
	err = externalError()
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return
}

func multipleErrors(output io.Writer) (err error, closeErr error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return nil, nil
}

func externalError() error { return nil }
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/writernamedreturn\n\ngo 1.26.0\n",
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
		"func untouched",
		"func explicit",
		"func assignedNil",
		"func explicitNil",
		"func selfAssigned",
		"func guardedAfterAcquisition",
		"func guardedBeforeAcquisition",
		"func equalGuardBeforeAcquisition",
		"func reverseGuardBeforeAcquisition",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("named writer finalization result = %#v", result)
	}
	for index, prefix := range want {
		function := strings.Index(input, prefix)
		writer := strings.Index(input[function:], "writer :=") + function
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "writer-not-finalized" ||
			diagnostic.Range.Start != writer ||
			diagnostic.Range.End != writer + len("writer") {
			t.Fatalf("named writer diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestWriterNotFinalizedRecognizesExactDelegatedSuccessfulReturns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/writerdelegatedreturn\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import "errors"

func Success() error { return nil }
func TupleSuccess() (int, error) { return 1, nil }
func Failure() error { return errors.New("failed") }
func Unknown(err error) error { return err }
`,
	)
	input := `package sample

import (
	"compress/gzip"
	"errors"
	"io"

	"example.com/writerdelegatedreturn/helper"
)

type typedError struct{}
func (*typedError) Error() string { return "typed" }

func localSuccess() error { return nil }
func localTupleSuccess() (int, error) { return 1, nil }
func localFailure() error { return errors.New("failed") }
func localUnknown(err error) error { return err }
func localRecursive() error { return localRecursive() }
func localTypedNil() error {
	var err *typedError
	return err
}

func delegatedLocal(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return localSuccess()
}

func delegatedLocalTuple(output io.Writer) (int, error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return localTupleSuccess()
}

func delegatedImported(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return helper.Success()
}

func delegatedImportedTuple(output io.Writer) (int, error) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return helper.TupleSuccess()
}

func delegatedFailure(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return localFailure()
}

func delegatedImportedFailure(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return helper.Failure()
}

func delegatedUnknown(output io.Writer, err error) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return localUnknown(err)
}

func delegatedImportedUnknown(output io.Writer, err error) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return helper.Unknown(err)
}

func delegatedDynamic(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	complete := localSuccess
	return complete()
}

func delegatedRecursive(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return localRecursive()
}

func delegatedTypedNil(output io.Writer) error {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	return localTypedNil()
}
`
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
		"func delegatedLocal",
		"func delegatedLocalTuple",
		"func delegatedImported",
		"func delegatedImportedTuple",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("delegated writer finalization result = %#v", result)
	}
	for index, prefix := range want {
		function := strings.Index(input, prefix)
		writer := strings.Index(input[function:], "writer :=") + function
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "writer-not-finalized" ||
			diagnostic.Range.Start != writer ||
			diagnostic.Range.End != writer + len("writer") {
			t.Fatalf("delegated writer diagnostic[%d] = %#v", index, diagnostic)
		}
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
	"encoding/base64"
	"io"
)

func encode(output io.Writer) {
	writer := gzip.NewWriter(output)
	_, _ = writer.Write([]byte("payload"))
	encoder := base64.NewEncoder(base64.StdEncoding, output)
	_, _ = encoder.Write([]byte("payload"))
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
		!metadata.RequiresEffectFacts ||
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("writer finalization ownership = %#v", result.Files)
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "writer-not-finalized" {
			t.Fatalf("writer finalization ownership = %#v", result.Files)
		}
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
			"func run%d(output io.Writer) (err error) { writer := gzip.NewWriter(output); _, err = writer.Write([]byte(\"payload\")); if err != nil { return }; return }\n",
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
