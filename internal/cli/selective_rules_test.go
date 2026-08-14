package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLintOnlyAndExceptApplyAfterProjectPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/selective\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npresets = [\"correctness\"]\n" +
				"[lint.rules]\nidentical-branches = \"off\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		path,
		[]byte(
			`package sample

func run(value bool) {
	if value { println("same") } else { println("same") }
	if value { println("first") }
	if value { println("second") }
}
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"lint", "--only", "identical-branches", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "warn[identical-branches]") ||
		strings.Contains(stdout.String(), "duplicate-condition") {
		t.Fatalf(
			"Run(lint --only) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"lint", "--except=duplicate-condition,identical-branches", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint --except) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintOnlyLimitsSuggestionFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/selectivefix\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npresets = []\n" +
				"[lint.rules]\ntime-since = \"off\"\ntime-until = \"warn\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		path,
		[]byte(
			`package sample

import "time"

func run(start, deadline time.Time) {
	_ = time.Now().Sub(start)
	_ = deadline.Sub(time.Now())
}
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"lint", "--fix-suggestions", "--only=time-since", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint selective fix) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(contents, []byte("time.Since(start)")) ||
		!bytes.Contains(contents, []byte("deadline.Sub(time.Now())")) ||
		bytes.Contains(contents, []byte("time.Until(deadline)")) {
		t.Fatalf("selectively fixed source =\n%s", contents)
	}
}

func TestRunLintRejectsMalformedAndUnknownRuleFilters(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/selectiveinvalid\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, arguments := range
		[][]string{
			{"lint", "--only=", path},
			{"lint", "--only=duplicate-condition,", path},
			{"lint", "--only=duplicate-condition", "--only=identical-branches", path},
			{"lint", "--except=missing-rule", path},
		} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitInvalidInvocation || stdout.Len() != 0 || stderr.Len() == 0 {
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
