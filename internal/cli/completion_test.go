package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunCompletionRendersSupportedShellsWithoutProjectInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell string
		marker string
	}{
		{shell: "bash", marker: "complete -F _glippy_completion glippy"},
		{shell: "zsh", marker: "#compdef glippy"},
		{shell: "fish", marker: "complete -c glippy -f"},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.shell,
			func(t *testing.T) {
				t.Parallel()

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(
					[]string{"completion", test.shell},
					failingReader{},
					&stdout,
					&stderr,
				)
				if exitCode != ExitSuccess || stderr.Len() != 0 {
					t.Fatalf(
						"Run(completion %s) = exit %d, stderr %q",
						test.shell,
						exitCode,
						stderr.String(),
					)
				}
				if !strings.Contains(stdout.String(), test.marker) ||
					!strings.Contains(stdout.String(), "duplicate-condition") {
					t.Fatalf(
						"Run(completion %s) output is incomplete:\n%s",
						test.shell,
						stdout.String(),
					)
				}
			},
		)
	}
}

func TestRunCompletionRejectsInvalidInvocation(t *testing.T) {
	t.Parallel()

	for _, arguments := range
		[][]string{
			{"completion"},
			{"completion", "powershell"},
			{"completion", "bash", "extra"},
		} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
		if exitCode != ExitInvalidInvocation ||
			stdout.Len() != 0 ||
			stderr.String() != "glippy: expected 'completion <bash|zsh|fish>'\n" {
			t.Fatalf(
				"Run(%q) = exit %d, stdout %q, stderr %q",
				arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunCompletionPreservesCancellationAndOutputFailures(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		canceled,
		[]string{"completion", "bash"},
		failingReader{},
		&stdout,
		&stderr,
	)
	if exitCode != ExitCanceled ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf(
			"RunContext(canceled completion) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	stderr.Reset()
	exitCode = Run([]string{"completion", "bash"}, failingReader{}, failingWriter{}, &stderr)
	if exitCode != ExitFilesystemError ||
		!strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf(
			"Run(completion output failure) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
}
