package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestParseLintAndCheckPreserveOrderedLintLevels(t *testing.T) {
	t.Parallel()

	want := []rules.LintLevelDirective{
		{Level: rules.LintWarn, Targets: []string{"performance"}},
		{Level: rules.LintDeny, Targets: []string{"correctness", "warnings"}},
		{Level: rules.LintForbid, Targets: []string{"pedantic-rule"}},
		{Level: rules.LintAllow, Targets: []string{"redundant-closure"}},
	}
	arguments := []string{
		"-Wperformance",
		"-Dcorrectness,warnings",
		"-Fpedantic-rule",
		"--allow",
		"redundant-closure",
		"source.go",
	}
	lint, valid := parseLintInvocation(append([]string{"lint"}, arguments...))
	if !valid || !equalLintLevels(lint.lintLevels, want) {
		t.Fatalf("parseLintInvocation(levels) = %#v, %t", lint, valid)
	}
	check, valid := parseCheckInvocation(append([]string{"check"}, arguments...))
	if !valid || !equalLintLevels(check.lintLevels, want) {
		t.Fatalf("parseCheckInvocation(levels) = %#v, %t", check, valid)
	}
}

func TestRunLintLevelsOverrideProjectPolicyAndGroups(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/lintlevels\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npresets = [\"correctness\"]\n"),
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
	if value { println("first") } else if value { println("second") }
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
		[]string{"lint", "--allow=correctness", "--warn=suspicious", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "warn[identical-branches]") ||
		strings.Contains(stdout.String(), "duplicate-condition") {
		t.Fatalf(
			"Run(lint levels) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"lint", "--deny=warnings", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "error[duplicate-condition]") {
		t.Fatalf(
			"Run(lint --deny=warnings) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"lint", "--forbid=duplicate-condition", "--allow=correctness", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitInvalidInvocation ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "cannot lower forbidden rule") {
		t.Fatalf(
			"Run(lint forbidden lowering) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestLintLevelsEnableTypedGroupsForLintAndCombinedCheckReporters(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/typedlintlevels\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npresets = []\n"),
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

import "regexp"

func match(values []string) {
	for _, value := range values {
		regexp.MatchString("constant", value)
	}
}
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"lint", "check"} {
		command := command
		t.Run(
			command,
			func(t *testing.T) {
				t.Parallel()
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(
					[]string{
						command,
						"--deny=performance",
						"--reporter=json",
						path,
					},
					strings.NewReader(""),
					&stdout,
					&stderr,
				)
				if exitCode != ExitFindings || stderr.Len() != 0 {
					t.Fatalf(
						"Run(%s typed lint level) = exit %d, stdout %q, stderr %q",
						command,
						exitCode,
						stdout.String(),
						stderr.String(),
					)
				}
				var result struct {
					Diagnostics []glippyreport.LintDiagnostic `json:"diagnostics"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf("decode %s lint-level JSON: %v", command, err)
				}
				if len(result.Diagnostics) != 1 ||
					result.Diagnostics[0].RuleID != "regexp-compile-in-loop" ||
					result.Diagnostics[0].Severity != "error" {
					t.Fatalf(
						"Run(%s typed lint level) diagnostics = %#v",
						command,
						result.Diagnostics,
					)
				}
			},
		)
	}
}

func TestLintLevelFlagsRejectMalformedTargets(t *testing.T) {
	t.Parallel()

	for _, arguments := range
		[][]string{
			{"lint", "--allow"},
			{"lint", "--deny="},
			{"lint", "-W", "--only=duplicate-condition"},
			{"lint", "-A"},
			{"check", "--forbid", ""},
		} {
		arguments := arguments
		t.Run(
			strings.Join(arguments, "_"),
			func(t *testing.T) {
				t.Parallel()
				if arguments[0] == "lint" {
					if _, valid := parseLintInvocation(arguments); valid {
						t.Fatalf(
							"parseLintInvocation(%q) succeeded",
							arguments,
						)
					}
					return
				}
				if _, valid := parseCheckInvocation(arguments); valid {
					t.Fatalf("parseCheckInvocation(%q) succeeded", arguments)
				}
			},
		)
	}
}

func equalLintLevels(left, right []rules.LintLevelDirective) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Level != right[index].Level ||
			!equalStrings(left[index].Targets, right[index].Targets) {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
