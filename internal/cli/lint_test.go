package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
)

type cliSyntaxRule struct {
	metadata rules.Metadata
}

type lintMutationContext struct {
	context.Context
	calls    int
	mutateAt int
	mutate   func()
}

func (c *lintMutationContext) Err() error {
	c.calls++
	if c.calls == c.mutateAt {
		c.mutate()
	}
	return c.Context.Err()
}

func (r cliSyntaxRule) Metadata() rules.Metadata { return r.metadata }

func (r cliSyntaxRule) RunSyntax(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
	sourceRange, err := ctx.Range(node)
	if err != nil {
		return nil, err
	}
	return []rules.Finding{{
		MessageKey: "call",
		Message:    "call requires review",
		Range:      sourceRange,
	}}, nil
}

func TestRunLintCheckAnalyzesConfiguredSyntaxRulesWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".gox.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[lint.rules]\ncall-rule = \"error\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	registry := newCLISyntaxRegistry(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitFindings {
		t.Fatalf("runLintCheck() exit = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	want := path + ":2:12: error[call-rule]: call requires review\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runLintCheck() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintCheck() mutated source: %q", got)
	}
}

func TestRunLintCheckEmitsVersionedJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{path}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		newCLISyntaxRegistry(t),
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("runLintCheck() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.SchemaVersion != 1 || result.Command != "lint" || result.Mode != "check" ||
		result.Outcome.ExitCode != ExitFindings || len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].RuleID != "call-rule" {
		t.Fatalf("lint JSON = %#v", result)
	}
}

func TestRunLintUsesEmptyAdmissionGatedRegistry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cleanPath := filepath.Join(root, "clean.go")
	if err := os.WriteFile(cleanPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{"lint", cleanPath}, failingReader{}, &stdout, &stderr); exitCode != ExitSuccess ||
		stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(lint clean) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}

	suppressedPath := filepath.Join(root, "suppressed.go")
	if err := os.WriteFile(
		suppressedPath,
		[]byte("package sample\n//gox:ignore unknown-rule\nfunc run(){}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exitCode := Run([]string{"lint", suppressedPath}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitFindings || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), ":2:1: suppression[unknown-rule]:") {
		t.Fatalf("Run(lint suppression) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunLintInvalidJSONInvocationReturnsJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"lint", "--fix", "--reporter=json", "source.go"},
		failingReader{},
		&stdout,
		&stderr,
	)
	if exitCode != ExitInvalidInvocation || stderr.Len() != 0 {
		t.Fatalf("Run(invalid lint JSON) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.Command != "lint" || result.Mode != "invalid" ||
		result.Outcome.ExitCode != ExitInvalidInvocation || result.Summary.Complete {
		t.Fatalf("invalid lint JSON = %#v", result)
	}
}

func TestRunLintRejectsPackagePatternsAsInvalidInvocations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		arguments []string
		json      bool
	}{
		{name: "text", arguments: []string{"lint", "./..."}},
		{name: "json", arguments: []string{"lint", "--reporter=json", "./..."}, json: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run(test.arguments, failingReader{}, &stdout, &stderr)

			if exitCode != ExitInvalidInvocation {
				t.Fatalf("Run(%q) exit = %d, want %d", test.arguments, exitCode, ExitInvalidInvocation)
			}
			if !test.json {
				if stdout.Len() != 0 || stderr.String() != lintUsage {
					t.Fatalf("Run(%q) stdout = %q, stderr = %q", test.arguments, stdout.String(), stderr.String())
				}
				return
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run(%q) stderr = %q, want empty", test.arguments, stderr.String())
			}
			var result goxreport.LintResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode invalid lint JSON: %v; output = %q", err, stdout.String())
			}
			if result.Mode != "invalid" || result.Outcome.ExitCode != ExitInvalidInvocation ||
				result.Summary.Complete {
				t.Fatalf("invalid lint JSON = %#v", result)
			}
		})
	}
}

func TestRunLintCheckRetainsCompletedResultsBeforeLaterSourceFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	validPath := filepath.Join(root, "a-valid.go")
	invalidPath := filepath.Join(root, "z-invalid.go")
	if err := os.WriteFile(validPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalidPath, []byte("package sample\nfunc broken( {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{invalidPath, validPath}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("runLintCheck() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode incomplete lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError || result.Summary.Complete ||
		result.Summary.Files != 1 || len(result.Files) != 1 || result.Files[0].Path != validPath ||
		len(result.Errors) != 1 {
		t.Fatalf("incomplete lint JSON = %#v", result)
	}
}

func TestRunLintCheckTreatsSuppressedOnlyDiagnosticsAsSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "suppressed.go")
	input := []byte("package sample\n//gox:ignore call-rule -- accepted here\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := newCLISyntaxRegistry(t)

	for _, reporter := range []goxreport.Format{goxreport.Text, goxreport.JSON} {
		reporter := reporter
		t.Run(string(reporter), func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := runLintCheck(
				context.Background(),
				lintInvocation{paths: []string{path}, reporter: reporter},
				&stdout,
				&stderr,
				registry,
			)

			if exitCode != ExitSuccess || stderr.Len() != 0 {
				t.Fatalf("runLintCheck() exit = %d, stderr = %q", exitCode, stderr.String())
			}
			if reporter == goxreport.Text {
				if stdout.Len() != 0 {
					t.Fatalf("runLintCheck() stdout = %q, want empty", stdout.String())
				}
				return
			}
			var result goxreport.LintResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode suppressed lint JSON: %v; output = %q", err, stdout.String())
			}
			if result.Outcome.ExitCode != ExitSuccess || !result.Summary.Complete ||
				result.Summary.Suppressed != 1 || result.Summary.Diagnostics != 0 {
				t.Fatalf("suppressed lint JSON = %#v", result)
			}
		})
	}
}

func TestPrepareLintTasksBindsOneConfigurationSnapshotToEverySelectedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configurationPath := filepath.Join(root, ".gox.toml")
	writeConfiguration := func(severity string) {
		t.Helper()
		if err := os.WriteFile(
			configurationPath,
			[]byte("version = 1\n[lint.rules]\ncall-rule = \""+severity+"\"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	writeConfiguration("warn")
	firstPath := filepath.Join(root, "a.go")
	secondPath := filepath.Join(root, "b.go")
	for _, path := range []string{firstPath, secondPath} {
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &lintMutationContext{
		Context:  context.Background(),
		mutateAt: 6,
		mutate:   func() { writeConfiguration("error") },
	}

	tasks, exitCode, err := prepareLintTasks(
		ctx,
		lintInvocation{
			configPath: configurationPath,
			paths:      []string{firstPath, secondPath},
		},
		newCLISyntaxRegistry(t),
	)

	if err != nil || exitCode != ExitSuccess {
		t.Fatalf("prepareLintTasks() exit = %d, error = %v", exitCode, err)
	}
	if len(tasks) != 2 {
		t.Fatalf("prepareLintTasks() returned %d tasks, want 2", len(tasks))
	}
	for _, task := range tasks {
		if got := task.options.Overrides["call-rule"]; got != rules.SeverityWarn {
			t.Fatalf("task %q severity = %q, want one bound warn snapshot", task.file.Path, got)
		}
	}
}

func TestRunLintCheckReportsSourceFailureAndCancellationInJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "invalid.go")
	input := []byte("package sample\nfunc broken( {\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{path}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(invalid) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode source-error JSON: %v", err)
	}
	if result.Outcome.ExitCode != ExitSourceError || result.Summary.Complete || len(result.Errors) != 1 {
		t.Fatalf("source-error lint JSON = %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintCheck(invalid) mutated source: %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout.Reset()
	stderr.Reset()
	exitCode = runLintCheck(
		ctx,
		lintInvocation{paths: []string{path}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitCanceled || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(canceled) = exit %d, stderr %q", exitCode, stderr.String())
	}
	result = goxreport.LintResult{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode canceled JSON: %v", err)
	}
	if result.Outcome.ExitCode != ExitCanceled || result.Summary.Complete || len(result.Errors) != 1 {
		t.Fatalf("canceled lint JSON = %#v", result)
	}
}

func TestRunLintCheckPreservesCancellationWhenJSONOutputFails(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	registry, err := rules.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		ctx,
		lintInvocation{paths: []string{"."}, reporter: goxreport.JSON},
		failingWriter{},
		&stderr,
		registry,
	)

	if exitCode != ExitCanceled {
		t.Fatalf("runLintCheck() exit = %d, want %d; stderr = %q", exitCode, ExitCanceled, stderr.String())
	}
	if !strings.Contains(stderr.String(), "write JSON report") {
		t.Fatalf("runLintCheck() stderr = %q, want JSON output failure", stderr.String())
	}
}

func newCLISyntaxRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(cliSyntaxRule{metadata: rules.Metadata{
		ID:               "call-rule",
		Summary:          "reports calls",
		Documentation:    "Reports calls that require review.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSyntax,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "target()", Correct: "reviewed()"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
