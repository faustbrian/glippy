package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func TestRunUsesDiscoveredConfigurationForStandardInputPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[format]\nline-width = 30\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	stdinPath := filepath.Join(root, "new", "source.go")
	input := "package sample\nfunc run(){if firstCondition && secondCondition && thirdCondition {work()}}\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--stdin-filepath", stdinPath},
		strings.NewReader(input),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "firstCondition &&\n") {
		t.Fatalf("Run() stdout =\n%s\nwant configured width break", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunUsesExplicitConfigurationBeforeReadingStandardInput(t *testing.T) {
	t.Parallel()

	configurationPath := filepath.Join(t.TempDir(), "explicit.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[format]\nline-width = 30\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	input := "package sample\nfunc run(){if firstCondition && secondCondition && thirdCondition {work()}}\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--config=" + configurationPath},
		strings.NewReader(input),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess || !strings.Contains(stdout.String(), "firstCondition &&\n") {
		t.Fatalf("Run() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunAcceptsSeparatedConfigurationFlag(t *testing.T) {
	t.Parallel()

	configurationPath := filepath.Join(t.TempDir(), "explicit.toml")
	if err := os.WriteFile(configurationPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--config", configurationPath},
		strings.NewReader("package sample\n"),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
}

func TestRunRejectsInvalidConfigurationBeforeReadingStandardInput(t *testing.T) {
	t.Parallel()

	configurationPath := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(configurationPath, []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--config=" + configurationPath},
		failingReader{},
		&stdout,
		&stderr,
	)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "stream failure") {
		t.Fatalf("Run() read stdin before configuration validation: %q", stderr.String())
	}
}

func TestRunClassifiesDirectoryStdinFilepathAsInvalidInvocation(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"fmt", "--stdin-filepath=" + t.TempDir()},
		strings.NewReader("package sample\n"),
		&stdout,
		&stderr,
	)
	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitInvalidInvocation, stderr.String())
	}
}

func TestRunFormatsExplicitStandardInputFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		argument string
		input    string
		want     string
	}{
		{
			name:     "declarations",
			argument: "--fragment=declaration",
			input:    "var answer=42\nfunc run(){}",
			want:     "var answer = 42\n\nfunc run() {}\n",
		},
		{
			name:     "statements",
			argument: "--fragment=statement",
			input:    "value:=1;value++",
			want:     "value := 1\nvalue++\n",
		},
		{
			name:     "expression",
			argument: "--fragment=expression",
			input:    "client.call(first,second)\n",
			want:     "client.call(first, second)\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run(
				[]string{"fmt", test.argument},
				strings.NewReader(test.input),
				&stdout,
				&stderr,
			)

			if exitCode != ExitSuccess {
				t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
			}
			if stdout.String() != test.want {
				t.Fatalf("Run() stdout = %q, want %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run() stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunDoesNotInferStandardInputFragmentKind(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt"}, strings.NewReader("value++"), &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunRejectsInvalidFragmentSelections(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"fmt", "--fragment"},
		{"fmt", "--fragment=unknown"},
		{"fmt", "--fragment=statement", "extra"},
		{"fmt", "--config", "--fragment=statement"},
		{"fmt", "--stdin-filepath", "--config=project.toml"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(arguments, strings.NewReader("value++"), &stdout, &stderr)

		if exitCode != ExitInvalidInvocation {
			t.Fatalf("Run(%q) exit code = %d, want %d", arguments, exitCode, ExitInvalidInvocation)
		}
		if stdout.Len() != 0 {
			t.Fatalf("Run(%q) stdout = %q, want empty", arguments, stdout.String())
		}
		if !strings.Contains(stderr.String(), "--fragment=declaration|statement|expression") {
			t.Fatalf("Run(%q) stderr = %q, want supported fragment kinds", arguments, stderr.String())
		}
	}
}

func TestRunRejectsInvalidFragmentWithoutPartialOutputOrSyntheticLocations(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--fragment=expression"},
		strings.NewReader("first +"),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stdin.go:1:") {
		t.Fatalf("Run() stderr = %q, want physical fragment location", stderr.String())
	}
	if strings.Contains(stderr.String(), "goxfragment") {
		t.Fatalf("Run() stderr exposed synthetic wrapper: %q", stderr.String())
	}
}

func TestRunRejectsFilePlacementDirectiveInFragment(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"fmt", "--fragment=declaration"},
		strings.NewReader("//go:build linux\nvar value int"),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "requires complete-file placement") {
		t.Fatalf("Run() stderr = %q, want directive boundary diagnostic", stderr.String())
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
		if stderr.String() != formatUsage {
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
