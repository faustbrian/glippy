package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	glippyreport "github.com/faustbrian/glippy/internal/report"
)

func TestParseLintAndCheckNewFrom(t *testing.T) {
	t.Parallel()

	lint, valid := parseLintInvocation([]string{"lint", "--new-from=origin/main", "./..."})
	if !valid ||
		lint.newFrom != "origin/main" ||
		len(lint.paths) != 1 ||
		lint.paths[0] != "./..." {
		t.Fatalf("parseLintInvocation() = %#v, %t", lint, valid)
	}
	check, valid := parseCheckInvocation([]string{"check", "--new-from", "HEAD", "./..."})
	if !valid || check.newFrom != "HEAD" || len(check.paths) != 1 || check.paths[0] != "./..." {
		t.Fatalf("parseCheckInvocation() = %#v, %t", check, valid)
	}
	for _, arguments := range
		[][]string{
			{"lint", "--new-from="},
			{"lint", "--new-from", "--fix"},
			{"lint", "--new-from=HEAD", "--new-from=main"},
			{"lint", "--new-from=HEAD", "--generate-baseline=.glippy-baseline.json"},
			{"check", "--new-from="},
			{"check", "--new-from=HEAD", "--new-from=main"},
		} {
		if arguments[0] == "lint" {
			if _, valid := parseLintInvocation(arguments); valid {
				t.Fatalf(
					"parseLintInvocation(%q) accepted invalid arguments",
					arguments,
				)
			}
			continue
		}
		if _, valid := parseCheckInvocation(arguments); valid {
			t.Fatalf("parseCheckInvocation(%q) accepted invalid arguments", arguments)
		}
	}
}

func TestRunLintNewFromReportsOnlyChangedLineDiagnostics(t *testing.T) {
	t.Parallel()

	root := initializeChangedCLIRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc run() {\n\tfirst := 1\n\tsecond := 2\n\tfirst = first\n\tsecond = second\n\t_, _ = first, second\n}\n"
	current := "package sample\n\nfunc run() {\n\tfirst := 1\n\tsecond := 2\n\tfirst = first // changed\n\tsecond = second\n\t_, _ = first, second\n}\n"
	writeChangedCLIFile(t, path, baseline)
	commitChangedCLIBaseline(t, root)
	writeChangedCLIFile(t, path, current)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lint", "--new-from=HEAD", "--reporter=json", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Diagnostics != 1 ||
		result.Summary.PreexistingDiagnostics != 1 ||
		len(result.Diagnostics) != 1 {
		t.Fatalf(
			"changed-code lint summary = %#v, diagnostics = %#v",
			result.Summary,
			result.Diagnostics,
		)
	}
	if result.Diagnostics[0].RuleID != "self-assignment" ||
		result.Diagnostics[0].Range.Start != strings.Index(current, "first = first") {
		t.Fatalf("changed-code diagnostic = %#v", result.Diagnostics[0])
	}
}

func TestRunLintNewFromFixesOnlyCompletelyOwnedEdits(t *testing.T) {
	t.Parallel()

	root := initializeChangedCLIRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc changed(ready bool) bool {\n\treturn ready == true\n}\n\nfunc existing(stable bool) bool {\n\treturn stable == true\n}\n"
	current := "package sample\n\nfunc changed(ready bool) bool {\n\treturn (ready == true)\n}\n\nfunc existing(stable bool) bool {\n\treturn stable == true\n}\n"
	writeChangedCLIFile(t, path, baseline)
	commitChangedCLIBaseline(t, root)
	writeChangedCLIFile(t, path, current)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lint", "--fix", "--new-from=HEAD", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("ready == true")) ||
		!bytes.Contains(got, []byte("stable == true")) {
		t.Fatalf("changed-code fixed source = %q", got)
	}
}

func TestRunLintNewFromRefusesFixWhenFormattingTouchesPreexistingLines(t *testing.T) {
	t.Parallel()

	root := initializeChangedCLIRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc changed(ready bool) bool {\n\treturn ready == true\n}\n\nfunc existing(stable bool) bool { return stable == true }\n"
	current := "package sample\n\nfunc changed(ready bool) bool {\n\treturn (ready == true)\n}\n\nfunc existing(stable bool) bool { return stable == true }\n"
	writeChangedCLIFile(t, path, baseline)
	commitChangedCLIBaseline(t, root)
	writeChangedCLIFile(t, path, current)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lint", "--fix", "--new-from=HEAD", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != current {
		t.Fatalf("changed-code unsafe-format source = %q, want original %q", got, current)
	}
}

func TestRunCheckNewFromDistinguishesChangedAndPreexistingFormatting(t *testing.T) {
	t.Parallel()

	t.Run(
		"changed",
		func(t *testing.T) {
			root := initializeChangedCLIRepository(t)
			path := filepath.Join(root, "source.go")
			baseline := "package sample\n\nfunc changed() {\n\tprintln(\"old\")\n}\n"
			current := "package sample\n\nfunc changed(){println(\"new\")}\n"
			writeChangedCLIFile(t, path, baseline)
			commitChangedCLIBaseline(t, root)
			writeChangedCLIFile(t, path, current)

			result, exitCode, stderr := runChangedCheckJSON(t, root)
			if exitCode != ExitFindings ||
				stderr != "" ||
				result.Summary.FormattingDifferences != 1 ||
				result.Summary.PreexistingFormattingDifferences != 0 ||
				result.Files[0].FormatStatus != glippyreport.CheckFormatDifferent {
				t.Fatalf(
					"changed format result = exit %d, stderr %q, result %#v",
					exitCode,
					stderr,
					result,
				)
			}
		},
	)

	t.Run(
		"preexisting",
		func(t *testing.T) {
			root := initializeChangedCLIRepository(t)
			path := filepath.Join(root, "source.go")
			baseline := "package sample\n\nfunc existing(){println(\"stable\")}\n\nfunc changed() {\n\tprintln(\"old\")\n}\n"
			current := "package sample\n\nfunc existing(){println(\"stable\")}\n\nfunc changed() {\n\tprintln(\"new\")\n}\n"
			writeChangedCLIFile(t, path, baseline)
			commitChangedCLIBaseline(t, root)
			writeChangedCLIFile(t, path, current)

			result, exitCode, stderr := runChangedCheckJSON(t, root)
			if exitCode != ExitSuccess ||
				stderr != "" ||
				result.Summary.FormattingDifferences != 0 ||
				result.Summary.PreexistingFormattingDifferences != 1 ||
				result.Files[0].FormatStatus !=
					glippyreport.CheckFormatPreexisting {
				t.Fatalf(
					"preexisting format result = exit %d, stderr %q, result %#v",
					exitCode,
					stderr,
					result,
				)
			}
		},
	)
}

func TestRunCheckNewFromCountsPreexistingDiagnostics(t *testing.T) {
	t.Parallel()

	root := initializeChangedCLIRepository(t)
	path := filepath.Join(root, "source.go")
	baseline := "package sample\n\nfunc run() {\n\tfirst := 1\n\tsecond := 2\n\tfirst = first\n\t_, _ = first, second\n}\n"
	current := "package sample\n\nfunc run() {\n\tfirst := 1\n\tsecond := 2 // changed\n\tfirst = first\n\t_, _ = first, second\n}\n"
	writeChangedCLIFile(t, path, baseline)
	commitChangedCLIBaseline(t, root)
	writeChangedCLIFile(t, path, current)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"check", "--new-from=HEAD", "--reporter=json", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	var result struct {
		Summary struct {
			Diagnostics int `json:"diagnostics"`
			PreexistingDiagnostics int `json:"preexisting_diagnostics"`
		} `json:"summary"`
		Diagnostics []glippyreport.LintDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Diagnostics != 0 ||
		result.Summary.PreexistingDiagnostics != 1 ||
		len(result.Diagnostics) != 0 {
		t.Fatalf("changed-code check diagnostics = %#v", result)
	}
}

func runChangedCheckJSON(t *testing.T, root string) (glippyreport.CheckResult, int, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"check", "--new-from=HEAD", "--reporter=json", root},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	var result glippyreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode check JSON %q: %v", stdout.String(), err)
	}
	return result, exitCode, stderr.String()
}

func initializeChangedCLIRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runChangedCLIGit(t, root, "init", "-b", "main")
	runChangedCLIGit(t, root, "config", "user.name", "Glippy Test")
	runChangedCLIGit(t, root, "config", "user.email", "glippy@example.invalid")
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/changed\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\nredundant-bool-comparison = \"warn\"\n",
	)
	return root
}

func commitChangedCLIBaseline(t *testing.T, root string) {
	t.Helper()
	runChangedCLIGit(t, root, "add", "go.mod", ".glippy.toml", "source.go")
	runChangedCLIGit(t, root, "commit", "-m", "baseline")
}

func runChangedCLIGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.CommandContext(
		context.Background(),
		"git",
		append([]string{"-C", root}, arguments...)...,
	)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func writeChangedCLIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
