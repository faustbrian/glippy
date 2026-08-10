package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

var errStream = errors.New("stream failure")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errStream
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errStream
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}

func TestRunFormatsCompleteFileFromStdinToStdout(t *testing.T) {
	t.Parallel()

	stdin := bytes.NewBufferString("package sample\nfunc run(){if ready{work()}}\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, stdin, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	want := "package sample\n\nfunc run() {\n\tif ready {\n\t\twork()\n\t}\n}\n"
	if stdout.String() != want {
		t.Fatalf("Run() stdout =\n%s\nwant:\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunRejectsInvalidCompleteFileWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	stdin := bytes.NewBufferString("package sample\nfunc")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, stdin, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stdin.go:2") {
		t.Fatalf("Run() stderr = %q, want stdin source location", stderr.String())
	}
}

func TestRunRejectsUnsupportedInvocation(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{nil, {"lint"}, {"fmt", "file.go"}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(arguments, bytes.NewReader(nil), &stdout, &stderr)

		if exitCode != ExitInvalidInvocation {
			t.Fatalf("Run(%q) exit code = %d, want %d", arguments, exitCode, ExitInvalidInvocation)
		}
		if stdout.Len() != 0 {
			t.Fatalf("Run(%q) stdout = %q, want empty", arguments, stdout.String())
		}
		if stderr.String() != "gox: expected 'fmt' with standard input\n" {
			t.Fatalf("Run(%q) stderr = %q", arguments, stderr.String())
		}
	}
}

func TestRunRejectsMissingStreamsWithoutPanicking(t *testing.T) {
	t.Parallel()

	validInput := "package sample\nfunc run(){}\n"
	tests := []struct {
		name   string
		stdin  io.Reader
		stdout io.Writer
		stderr io.Writer
	}{
		{name: "stdin", stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}},
		{name: "stdout", stdin: bytes.NewReader([]byte(validInput)), stderr: &bytes.Buffer{}},
		{name: "stderr", stdin: bytes.NewReader([]byte(validInput)), stdout: &bytes.Buffer{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exitCode := Run([]string{"fmt"}, test.stdin, test.stdout, test.stderr)
			if exitCode != ExitFilesystemError {
				t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
			}
		})
	}
}

func TestRunReportsStandardInputReadFailure(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "read standard input: stream failure") {
		t.Fatalf("Run() stderr = %q, want read failure", stderr.String())
	}
}

func TestRunReportsStandardOutputWriteFailures(t *testing.T) {
	t.Parallel()

	validInput := "package sample\nfunc run(){}\n"
	tests := []struct {
		name   string
		stdout io.Writer
	}{
		{name: "error", stdout: failingWriter{}},
		{name: "short write", stdout: shortWriter{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer

			exitCode := Run([]string{"fmt"}, strings.NewReader(validInput), test.stdout, &stderr)

			if exitCode != ExitFilesystemError {
				t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
			}
			if !strings.Contains(stderr.String(), "write standard output") {
				t.Fatalf("Run() stderr = %q, want write failure", stderr.String())
			}
		})
	}
}

func TestDiagnosticWriteFailureUsesExitSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		want     int
	}{
		{name: "promotes less severe category", exitCode: ExitInvalidInvocation, want: ExitFilesystemError},
		{name: "preserves more severe category", exitCode: ExitInternalError, want: ExitInternalError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := report(failingWriter{}, test.exitCode, "diagnostic\n")
			if got != test.want {
				t.Fatalf("report() exit code = %d, want %d", got, test.want)
			}
		})
	}
}
