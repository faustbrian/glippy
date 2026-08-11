package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/faustbrian/gox/internal/discovery"
	"github.com/faustbrian/gox/internal/filesystem"
	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
	goxversion "github.com/faustbrian/gox/internal/version"
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

type explainMetadataRule struct {
	metadata rules.Metadata
}

func (r explainMetadataRule) Metadata() rules.Metadata { return r.metadata }

type formatJSONReport struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Mode          string `json:"mode"`
	Outcome       struct {
		Category string `json:"category"`
		ExitCode int    `json:"exit_code"`
	} `json:"outcome"`
	Summary struct {
		Files    int  `json:"files"`
		Changed  int  `json:"changed"`
		Complete bool `json:"complete"`
	} `json:"summary"`
	Files []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	} `json:"files"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeFormatJSONReport(t *testing.T, output []byte) formatJSONReport {
	t.Helper()
	var report formatJSONReport
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("decode JSON report: %v; output = %q", err, output)
	}
	return report
}

func TestRunReportsResolvedVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitSuccess)
	}
	want := "gox " + goxversion.Current() + "\n"
	if stdout.String() != want {
		t.Fatalf("Run() stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunLintReportsBuiltInDuplicateCondition(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc classify(first, second bool) int {\n\tif first && second {\n\t\treturn 1\n\t} else if first {\n\t\treturn 2\n\t} else if first && second {\n\t\treturn 3\n\t}\n\treturn 0\n}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"lint", "--reporter=json", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("Run(lint) exit = %d, stderr = %q, output = %q", exitCode, stderr.String(), stdout.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint JSON: %v; output = %q", err, stdout.String())
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("Run(lint) diagnostics = %#v, want one duplicate condition", result.Diagnostics)
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.RuleID != "duplicate-condition" ||
		diagnostic.Severity != rules.SeverityWarn ||
		diagnostic.MessageKey != "duplicate-condition" ||
		diagnostic.Message != "condition occurs more than once in this if/else-if chain" ||
		diagnostic.Help != "change the repeated condition or remove the unreachable branch" ||
		len(diagnostic.Related) != 1 || len(diagnostic.Fixes) != 0 {
		t.Fatalf("Run(lint) diagnostic = %#v", diagnostic)
	}
	first := strings.Index(string(input), "first && second")
	repeated := strings.LastIndex(string(input), "first && second")
	if diagnostic.Range.Start != repeated || diagnostic.Range.End != repeated+len("first && second") ||
		diagnostic.Related[0].Range.Start != first ||
		diagnostic.Related[0].Range.End != first+len("first && second") ||
		diagnostic.Related[0].Message != "first occurrence of this condition" {
		t.Fatalf("Run(lint) ranges = %#v, related = %#v", diagnostic.Range, diagnostic.Related)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint) mutated source: %q", got)
	}
}

func TestRunLintFixLeavesBuiltInDuplicateConditionUnchanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(ready bool) { if ready {} else if ready {} }\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"lint", "--fix", "--reporter=json", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("Run(lint --fix) exit = %d, stderr = %q, output = %q", exitCode, stderr.String(), stdout.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Mode != "fix" || len(result.Diagnostics) != 1 ||
		len(result.AppliedFixes) != 0 || len(result.RejectedFixes) != 0 ||
		len(result.Files) != 1 || result.Files[0].Status != goxreport.LintFileUnchanged {
		t.Fatalf("Run(lint --fix) result = %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint --fix) mutated source: %q", got)
	}
}

func TestRunCombinedCheckReportsFormatAndLintFindingsWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(ready bool){if ready{}else if ready{}}\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("Run(check) exit = %d, stderr = %q, stdout = %q", exitCode, stderr.String(), stdout.String())
	}
	first := strings.Index(string(input), "{if ready") + len("{if ")
	repeated := strings.LastIndex(string(input), "ready")
	lineStart := strings.IndexByte(string(input), '\n') + 1
	want := fmt.Sprintf(
		"%s: format differs\n%s:2:%d: warn[duplicate-condition]: condition occurs more than once in this if/else-if chain\n  related %s:2:%d: first occurrence of this condition\n  help: change the repeated condition or remove the unreachable branch\n",
		path,
		path,
		repeated-lineStart+1,
		path,
		first-lineStart+1,
	)
	if stdout.String() != want {
		t.Fatalf("Run(check) stdout = %q, want %q", stdout.String(), want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("Run(check) mutated source, mode, or modification time")
	}
}

func TestRunCombinedCheckReportsSSAFindingAndFormatDifferenceWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/checkssa\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte("package sample\nfunc inspect(pointer *int){if pointer==nil{_ = *pointer}}\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", filepath.Join(root, "...")}, failingReader{}, &stdout, &stderr)

	want := path + ": format differs\n" +
		path + ":2:48: warn[nilness]: nil dereference in load\n" +
		"  help: run `gox explain nilness` for the rule contract and limitations\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("Run(check SSA) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("Run(check SSA) mutated source, mode, or modification time")
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"check", "--reporter=json", path}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("Run(check SSA JSON) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	var result goxreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode SSA check JSON: %v; output = %q", err, stdout.String())
	}
	if result.Summary.Files != 1 || result.Summary.FormattingDifferences != 1 ||
		result.Summary.Diagnostics != 1 || !result.Summary.Complete || len(result.Files) != 1 ||
		len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "nilness" ||
		result.Diagnostics[0].SourceDigest != result.Files[0].SourceDigest {
		t.Fatalf("Run(check SSA JSON) result = %#v", result)
	}
}

func TestRunCombinedCheckReportsPackagePrerequisitesInJSONWithFormatting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/checktypes\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte("package sample\nfunc run(){_ = missing}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("Run(check typed JSON) = exit %d, stderr %q, stdout %q", exitCode, stderr.String(), stdout.String())
	}
	var result goxreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode typed check JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.Category != "source_error" || result.Outcome.ExitCode != ExitSourceError ||
		!result.Summary.Complete || result.Summary.Files != 1 ||
		result.Summary.FormattingDifferences != 1 || result.Summary.PackageDiagnostics == 0 ||
		len(result.PackageDiagnostics) == 0 || len(result.SourceProblems) != 0 || len(result.Errors) != 0 {
		t.Fatalf("Run(check typed JSON) result = %#v", result)
	}
	if len(result.Files) != 1 || result.Files[0].Path != path ||
		result.Files[0].FormatStatus != goxreport.CheckFormatDifferent || result.Files[0].SourceDigest == "" {
		t.Fatalf("Run(check typed JSON) files = %#v", result.Files)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(check typed JSON) mutated source: %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"check", root}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitSourceError || stderr.Len() != 0 ||
		!strings.HasPrefix(stdout.String(), path+": format differs\n") ||
		!strings.Contains(stdout.String(), "package[type]") || !strings.Contains(stdout.String(), "undefined: missing") {
		t.Fatalf("Run(check typed text) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunCombinedCheckReportsTypedSourceProblemsInJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/checksource\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte("package sample\nfunc broken( {\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("Run(check source-problem JSON) = exit %d, stderr %q, stdout %q", exitCode, stderr.String(), stdout.String())
	}
	var result goxreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode typed source-problem JSON: %v; output = %q", err, stdout.String())
	}
	if !result.Summary.Complete || result.Summary.SourceProblems == 0 ||
		len(result.SourceProblems) == 0 || result.SourceProblems[0].Path != path || len(result.Files) != 0 {
		t.Fatalf("Run(check source-problem JSON) result = %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(check source-problem JSON) mutated source: %q", got)
	}
}

func TestRunCombinedCheckReportsVersionedJSONFromOneSourceSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(cleanPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(root, "z.go")
	input := []byte("package sample\nfunc run(ready bool){if ready{}else if ready{}}\n")
	if err := os.WriteFile(changedPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("Run(check JSON) exit = %d, stderr = %q, stdout = %q", exitCode, stderr.String(), stdout.String())
	}
	var result struct {
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Mode          string `json:"mode"`
		Outcome       struct {
			Category string `json:"category"`
			ExitCode int    `json:"exit_code"`
		} `json:"outcome"`
		Summary struct {
			Files                 int  `json:"files"`
			FormattingDifferences int  `json:"formatting_differences"`
			Diagnostics           int  `json:"diagnostics"`
			Complete              bool `json:"complete"`
		} `json:"summary"`
		Files []struct {
			Path         string `json:"path"`
			SourceDigest string `json:"source_digest"`
			FormatStatus string `json:"format_status"`
		} `json:"files"`
		Diagnostics []goxreport.LintDiagnostic `json:"diagnostics"`
		Errors      []goxreport.Error          `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode combined check JSON: %v; output = %q", err, stdout.String())
	}
	if result.SchemaVersion != 1 || result.Command != "check" || result.Mode != "check" ||
		result.Outcome.Category != "findings" || result.Outcome.ExitCode != ExitFindings ||
		result.Summary.Files != 2 || result.Summary.FormattingDifferences != 1 ||
		result.Summary.Diagnostics != 1 || !result.Summary.Complete || len(result.Errors) != 0 {
		t.Fatalf("Run(check JSON) envelope = %#v", result)
	}
	if len(result.Files) != 2 || result.Files[0].Path != cleanPath ||
		result.Files[0].FormatStatus != "unchanged" || result.Files[0].SourceDigest == "" ||
		result.Files[1].Path != changedPath || result.Files[1].FormatStatus != "different" ||
		result.Files[1].SourceDigest == "" {
		t.Fatalf("Run(check JSON) files = %#v", result.Files)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "duplicate-condition" ||
		result.Diagnostics[0].Path != changedPath || result.Diagnostics[0].SourceDigest != result.Files[1].SourceDigest {
		t.Fatalf("Run(check JSON) diagnostics = %#v", result.Diagnostics)
	}
	got, err := os.ReadFile(changedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(check JSON) mutated source: %q", got)
	}
}

func TestRunCombinedCheckReturnsSuccessSilentlyForCleanSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(check clean) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunCombinedCheckOrdersTextFindingsByPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	secondPath := filepath.Join(root, "z.go")
	for _, path := range []string{secondPath, firstPath} {
		if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", root}, failingReader{}, &stdout, &stderr)

	want := firstPath + ": format differs\n" + secondPath + ": format differs\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("Run(check ordered) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunCombinedCheckSourceFailureDoesNotEmitPartialText(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	brokenPath := filepath.Join(root, "z.go")
	brokenInput := []byte("package sample\nfunc broken(\n")
	if err := os.WriteFile(brokenPath, brokenInput, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError || stdout.Len() != 0 || !strings.Contains(stderr.String(), brokenPath) {
		t.Fatalf("Run(check broken) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	for path, input := range map[string][]byte{firstPath: firstInput, brokenPath: brokenInput} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, input) {
			t.Fatalf("Run(check broken) mutated %s", path)
		}
	}
}

func TestRunCombinedCheckSourceFailureReportsIncompleteJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	brokenPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(brokenPath, []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("Run(check broken JSON) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode check failure JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError || result.Outcome.Category != "source_error" ||
		result.Summary.Complete || result.Summary.Files != 1 || len(result.Files) != 1 ||
		result.Files[0].Path != firstPath || len(result.Errors) != 1 ||
		!strings.Contains(result.Errors[0].Message, brokenPath) {
		t.Fatalf("Run(check broken JSON) result = %#v", result)
	}
}

func TestRunCombinedCheckInvalidJSONInvocationReturnsJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--write", "--reporter=json"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation || stderr.Len() != 0 {
		t.Fatalf("Run(check invalid JSON) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode invalid check JSON: %v; output = %q", err, stdout.String())
	}
	if result.Command != "check" || result.Outcome.ExitCode != ExitInvalidInvocation ||
		result.Outcome.Category != "invalid_invocation" || result.Summary.Complete ||
		len(result.Files) != 0 || len(result.Errors) != 1 {
		t.Fatalf("Run(check invalid JSON) result = %#v", result)
	}
}

func TestRunCombinedCheckHonorsLintAndFormatConfiguration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := "version = 1\n[format]\nline-width = 30\n[lint.rules]\nduplicate-condition = \"off\"\n"
	if err := os.WriteFile(filepath.Join(root, ".gox.toml"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	widthPath := filepath.Join(root, "width.go")
	widthInput := []byte("package sample\n\nfunc run() {\n\tif firstCondition && secondCondition && thirdCondition {\n\t\twork()\n\t}\n}\n")
	if err := os.WriteFile(widthPath, widthInput, 0o600); err != nil {
		t.Fatal(err)
	}
	duplicatePath := filepath.Join(root, "duplicate.go")
	duplicateInput := []byte("package sample\n\nfunc d(x bool) {\n\tif x {} else if x {}\n}\n")
	if err := os.WriteFile(duplicatePath, duplicateInput, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stdout.String() != widthPath+": format differs\n" || stderr.Len() != 0 {
		t.Fatalf("Run(check configured) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunCombinedCheckReportsOutputFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("Run(check output failure) = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunCombinedCheckReportsJSONOutputFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"check", "--reporter=json", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "write JSON report") {
		t.Fatalf("Run(check JSON output failure) = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunCombinedCheckReportsPreCanceledJSONInvocation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunContext(ctx, []string{"check", "--reporter=json"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitCanceled || stderr.Len() != 0 {
		t.Fatalf("RunContext(check JSON) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode canceled check JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.Category != "canceled" || result.Outcome.ExitCode != ExitCanceled ||
		result.Summary.Complete || len(result.Files) != 0 || len(result.Errors) != 1 {
		t.Fatalf("RunContext(check JSON) result = %#v", result)
	}
}

func TestParseCheckInvocationDefaultsToCurrentDirectory(t *testing.T) {
	t.Parallel()

	invocation, valid := parseCheckInvocation([]string{"check"})

	if !valid || len(invocation.paths) != 1 || invocation.paths[0] != "." ||
		invocation.reporter != goxreport.Text {
		t.Fatalf("parseCheckInvocation(check) = %#v, %t", invocation, valid)
	}
}

func TestRunCombinedCheckRejectsInvalidTextInvocationWithCheckUsage(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{{"check", "--write"}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(arguments, failingReader{}, &stdout, &stderr)

		if exitCode != ExitInvalidInvocation || stdout.Len() != 0 || stderr.String() != checkUsage {
			t.Fatalf("Run(%q) = exit %d, stdout %q, stderr %q", arguments, exitCode, stdout.String(), stderr.String())
		}
	}
}

func TestRunExplainDocumentsBuiltInDuplicateCondition(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"explain", "duplicate-condition"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(explain) exit = %d, stderr = %q", exitCode, stderr.String())
	}
	for _, contract := range []string{
		"duplicate-condition\n",
		"default severity: warn\n",
		"minimum Go: 1.26\n",
		"analysis tier: syntax\n",
		"generated files: excluded\n",
		"fixes:\n  none\n",
		"Calls, channel receives, address operations",
	} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf("Run(explain) output = %q, want %q", stdout.String(), contract)
		}
	}
}

func TestRunExplainRendersRegisteredRuleMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(explainMetadataRule{metadata: rules.Metadata{
		ID:               "explain-rule",
		Summary:          "explains one rule",
		Documentation:    "Canonical rule documentation.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSyntax,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{{
			Incorrect: "bad()",
			Correct:   "good()",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runExplain(
		context.Background(),
		[]string{"explain", "explain-rule"},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSuccess {
		t.Fatalf("runExplain() exit = %d, want %d", exitCode, ExitSuccess)
	}
	if !strings.HasPrefix(stdout.String(), "explain-rule\nexplains one rule\n") {
		t.Fatalf("runExplain() stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplain() stderr = %q, want empty", stderr.String())
	}
}

func TestRunExplainRejectsInvalidOrUnknownRules(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"explain"},
		{"explain", "first", "second"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
		if exitCode != ExitInvalidInvocation || stdout.Len() != 0 || stderr.String() != explainUsage {
			t.Fatalf("Run(%q) = exit %d, stdout %q, stderr %q", arguments, exitCode, stdout.String(), stderr.String())
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"explain", "missing-rule"}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitInvalidInvocation || stdout.Len() != 0 ||
		stderr.String() != "gox explain: unknown rule \"missing-rule\"\n" {
		t.Fatalf("Run(explain missing) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunExplainHonorsCancellationAndOutputFailures(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(explainMetadataRule{metadata: rules.Metadata{
		ID:               "explain-rule",
		Summary:          "explains one rule",
		Documentation:    "Canonical rule documentation.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSyntax,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var canceledOutput bytes.Buffer
	var canceledError bytes.Buffer
	if exitCode := runExplain(
		ctx,
		[]string{"explain", "explain-rule"},
		&canceledOutput,
		&canceledError,
		registry,
	); exitCode != ExitCanceled || canceledOutput.Len() != 0 {
		t.Fatalf("runExplain(canceled) = exit %d, stdout %q", exitCode, canceledOutput.String())
	}

	var stderr bytes.Buffer
	if exitCode := runExplain(
		context.Background(),
		[]string{"explain", "explain-rule"},
		failingWriter{},
		&stderr,
		registry,
	); exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("runExplain(failing output) = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunVersionRejectsArguments(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version", "extra"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != versionUsage {
		t.Fatalf("Run() stderr = %q, want version usage", stderr.String())
	}
}

func TestRunVersionHonorsCancellationBeforeOutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stderr bytes.Buffer

	exitCode := RunContext(ctx, []string{"version"}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitCanceled {
		t.Fatalf("RunContext() exit = %d, want %d", exitCode, ExitCanceled)
	}
	if !strings.Contains(stderr.String(), context.Canceled.Error()) {
		t.Fatalf("RunContext() stderr = %q, want cancellation", stderr.String())
	}
}

func TestRunVersionReportsOutputFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	exitCode := Run([]string{"version"}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("Run() stderr = %q, want output failure", stderr.String())
	}
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

func TestRunContextRefusesCanceledInvocationBeforeReadingInput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunContext(ctx, []string{"fmt"}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitCanceled {
		t.Fatalf("RunContext() exit code = %d, want %d", exitCode, ExitCanceled)
	}
	if stdout.Len() != 0 {
		t.Fatalf("RunContext() stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), context.Canceled.Error()) || strings.Contains(stderr.String(), errStream.Error()) {
		t.Fatalf("RunContext() stderr = %q, want cancellation without input read", stderr.String())
	}
}

func TestRunContextReportsPreCanceledJSONInvocationAsJSON(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunContext(
		ctx,
		[]string{"fmt", "--check", "--reporter=json", "source.go"},
		failingReader{},
		&stdout,
		&stderr,
	)

	if exitCode != ExitCanceled {
		t.Fatalf("RunContext() exit = %d, want %d", exitCode, ExitCanceled)
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Mode != "check" || report.Outcome.Category != "canceled" ||
		report.Outcome.ExitCode != ExitCanceled || report.Summary.Complete || len(report.Errors) != 1 {
		t.Fatalf("RunContext() report = %#v", report)
	}
	if stderr.Len() != 0 {
		t.Fatalf("RunContext() stderr = %q, want empty", stderr.String())
	}
}

func TestMapFormatTasksBoundsConcurrencyAndPreservesTaskOrder(t *testing.T) {
	t.Parallel()

	tasks := []formatTask{
		{file: discoveryFile("a.go")},
		{file: discoveryFile("b.go")},
		{file: discoveryFile("c.go")},
		{file: discoveryFile("d.go")},
	}
	started := make(chan string, len(tasks))
	release := make(chan struct{}, len(tasks))
	var active atomic.Int64
	var maximum atomic.Int64
	type outcome struct{ path string }
	result := make(chan struct {
		values []outcome
		err    error
	}, 1)
	go func() {
		values, err := mapFormatTasks(context.Background(), tasks, 2, func(_ context.Context, task formatTask) (outcome, error) {
			current := active.Add(1)
			for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
			}
			started <- task.file.Path
			<-release
			active.Add(-1)
			return outcome{path: task.file.Path}, nil
		})
		result <- struct {
			values []outcome
			err    error
		}{values: values, err: err}
	}()

	<-started
	<-started
	if got := maximum.Load(); got != 2 {
		t.Fatalf("mapFormatTasks() maximum concurrency = %d, want 2", got)
	}
	for range tasks {
		release <- struct{}{}
	}
	got := <-result
	if got.err != nil {
		t.Fatalf("mapFormatTasks() error = %v", got.err)
	}
	for index, task := range tasks {
		if got.values[index].path != task.file.Path {
			t.Fatalf("mapFormatTasks() result[%d] = %q, want %q", index, got.values[index].path, task.file.Path)
		}
	}
}

func TestBoundedFormatWorkerLimitUsesEveryResourceBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		resourceLimit int
		taskCount     int
		want          int
	}{
		{name: "resource", resourceLimit: 4, taskCount: 20, want: 4},
		{name: "selection", resourceLimit: 8, taskCount: 3, want: 3},
		{name: "hard ceiling", resourceLimit: 64, taskCount: 100, want: maximumFormatWorkers},
		{name: "empty selection", resourceLimit: 8, taskCount: 0, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := boundedFormatWorkerLimit(test.resourceLimit, test.taskCount); got != test.want {
				t.Fatalf("boundedFormatWorkerLimit(%d, %d) = %d, want %d", test.resourceLimit, test.taskCount, got, test.want)
			}
		})
	}
}

func TestMapFormatTasksChoosesFirstTaskErrorDeterministically(t *testing.T) {
	t.Parallel()

	firstErr := errors.New("first task failed")
	secondErr := errors.New("second task failed")
	secondFinished := make(chan struct{})
	tasks := []formatTask{{file: discoveryFile("a.go")}, {file: discoveryFile("z.go")}}

	_, err := mapFormatTasks(context.Background(), tasks, 2, func(_ context.Context, task formatTask) (string, error) {
		if task.file.Path == "a.go" {
			<-secondFinished
			return "", firstErr
		}
		close(secondFinished)
		return "", secondErr
	})

	if !errors.Is(err, firstErr) {
		t.Fatalf("mapFormatTasks() error = %v, want first task error", err)
	}
}

func TestMapFormatTasksChoosesSeverityBeforeTaskOrder(t *testing.T) {
	t.Parallel()

	sourceErr := errors.New("source failed")
	filesystemErr := errors.New("filesystem failed")
	tasks := []formatTask{{file: discoveryFile("a.go")}, {file: discoveryFile("z.go")}}

	_, err := mapFormatTasks(context.Background(), tasks, 2, func(_ context.Context, task formatTask) (string, error) {
		if task.file.Path == "a.go" {
			return "", &formatTaskError{exitCode: ExitSourceError, err: sourceErr}
		}
		return "", &formatTaskError{exitCode: ExitFilesystemError, err: filesystemErr}
	})

	if !errors.Is(err, filesystemErr) {
		t.Fatalf("mapFormatTasks() error = %v, want higher-severity filesystem error", err)
	}
}

func discoveryFile(path string) discovery.File {
	return discovery.File{Path: path}
}

func TestRunFormatsOneExplicitFileToStdoutWithoutMutation(t *testing.T) {
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
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){if firstCondition && secondCondition && thirdCondition {work()}}\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "firstCondition &&\n") {
		t.Fatalf("Run() stdout =\n%s\nwant configured width break", stdout.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() mutated stdout-mode file: %q", got)
	}
}

func TestRunWriteFormatsFileAndPreservesPermissions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package sample\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("Run() file = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("Run() permissions = %o, want 640", info.Mode().Perm())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run() stdout = %q, stderr = %q, want empty", stdout.String(), stderr.String())
	}
}

func TestRunWriteDoesNotTouchFormattedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("Run() touched formatted file: before = %#v, after = %#v", before, after)
	}
}

func TestRunWriteReportsVersionedJSONOutcomesInPathOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(changedPath, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchangedPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(unchangedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.SchemaVersion != 1 || report.Command != "fmt" || report.Mode != "write" ||
		report.Outcome.Category != "success" || report.Outcome.ExitCode != ExitSuccess {
		t.Fatalf("Run() report header = %#v", report)
	}
	if report.Summary.Files != 2 || report.Summary.Changed != 1 || !report.Summary.Complete || len(report.Files) != 2 || len(report.Errors) != 0 {
		t.Fatalf("Run() report totals = %#v", report)
	}
	if report.Files[0].Path != changedPath || report.Files[0].Status != "formatted" ||
		report.Files[1].Path != unchangedPath || report.Files[1].Status != "unchanged" {
		t.Fatalf("Run() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunWriteValidatesEverySourceBeforeAnyReplacement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(invalidPath, []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSourceError, stderr.String())
	}
	got, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, firstInput) {
		t.Fatalf("Run() replaced %q before validating later source: %q", firstPath, got)
	}
}

func TestRunWriteRefusesGeneratedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "generated.go")
	input := []byte("// Code generated by fixture. DO NOT EDIT.\npackage sample\nfunc run(){}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "refusing to write generated file") {
		t.Fatalf("Run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() changed generated file: %q", got)
	}
}

func TestRunWriteRefusesExplicitSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(target, input, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "refusing to write symlink") {
		t.Fatalf("Run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() changed symlink target: %q", got)
	}
}

func TestRunWriteRefusesPathThroughSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetDirectory := t.TempDir()
	target := filepath.Join(targetDirectory, "source.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(target, input, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(targetDirectory, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(link, "source.go")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "refusing to write symlink") {
		t.Fatalf("Run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() changed file through symlinked directory: %q", got)
	}
}

func TestRunWriteValidatesEveryConfigurationBeforeAnyReplacement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package root\nfunc run(){}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gox.toml"), []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "z.go"), []byte("package nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitInvalidInvocation, stderr.String())
	}
	got, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, firstInput) {
		t.Fatalf("Run() replaced %q before validating later configuration: %q", firstPath, got)
	}
}

func TestRunFormatWriteReportsStaleSourceWithoutOverwritingNewBytes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte("package sample\n\nfunc newer() {}\n")
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{path}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := os.WriteFile(snapshot.Path(), newer, 0o600); err != nil {
				return err
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict || !strings.Contains(stderr.String(), filesystem.ErrStale.Error()) {
		t.Fatalf("runFormatWrite() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newer) {
		t.Fatalf("runFormatWrite() source = %q, want newer bytes preserved", got)
	}
}

func TestRunFormatWriteCannotEscapeRootThroughParentSymlinkRace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "nested")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "source.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	externalDirectory := t.TempDir()
	externalPath := filepath.Join(externalDirectory, "source.go")
	if err := os.Link(path, externalPath); err != nil {
		t.Fatal(err)
	}
	movedDirectory := filepath.Join(root, "moved")
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{path}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := os.Rename(directory, movedDirectory); err != nil {
				return err
			}
			if err := os.Symlink(externalDirectory, directory); err != nil {
				return err
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict || !strings.Contains(stderr.String(), filesystem.ErrStale.Error()) {
		t.Fatalf("runFormatWrite() exit = %d, stderr = %q, want stale conflict", exitCode, stderr.String())
	}
	got, err := os.ReadFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runFormatWrite() changed outside-root hard link: %q", got)
	}
}

func TestRunFormatWriteReportsFilesReplacedBeforeLaterConflict(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(secondPath, []byte("package sample\nfunc second(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte("package sample\n\nfunc newer() {}\n")
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{root}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if snapshot.Path() == secondPath {
				if err := os.WriteFile(secondPath, newer, 0o600); err != nil {
					return err
				}
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict {
		t.Fatalf("runFormatWrite() exit = %d, want %d", exitCode, ExitConflict)
	}
	if !strings.Contains(stderr.String(), "files replaced before failure") || !strings.Contains(stderr.String(), firstPath) {
		t.Fatalf("runFormatWrite() stderr = %q, want prior replacement report", stderr.String())
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "package sample\n\nfunc first() {}\n" {
		t.Fatalf("runFormatWrite() first file = %q, want formatted", first)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, newer) {
		t.Fatalf("runFormatWrite() second file = %q, want newer bytes preserved", second)
	}
}

func TestRunFormatWriteReportsPartialConflictAsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(secondPath, []byte("package sample\nfunc second(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte("package sample\n\nfunc newer() {}\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runFormatWriteReported(
		context.Background(),
		formatInvocation{paths: []string{root}, reporter: "json", write: true},
		&stdout,
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if snapshot.Path() == secondPath {
				if err := os.WriteFile(secondPath, newer, 0o600); err != nil {
					return err
				}
			}
			return snapshot.Replace(output)
		},
	)

	if exitCode != ExitConflict {
		t.Fatalf("runFormatWriteReported() exit = %d, want %d", exitCode, ExitConflict)
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Outcome.Category != "conflict" || report.Summary.Complete ||
		report.Summary.Changed != 2 || len(report.Errors) != 1 {
		t.Fatalf("runFormatWriteReported() report = %#v", report)
	}
	if report.Files[0].Path != firstPath || report.Files[0].Status != "formatted" ||
		report.Files[1].Path != secondPath || report.Files[1].Status != "conflict" {
		t.Fatalf("runFormatWriteReported() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runFormatWriteReported() stderr = %q, want empty", stderr.String())
	}
}

func TestRunFormatWriteStopsBeforeNextReplacementWhenCanceled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	secondInput := []byte("package sample\nfunc second(){}\n")
	if err := os.WriteFile(secondPath, secondInput, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var replacements atomic.Int64
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		ctx,
		formatInvocation{paths: []string{root}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			replacements.Add(1)
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			cancel()
			return nil
		},
	)

	if exitCode != ExitCanceled {
		t.Fatalf("runFormatWrite() exit = %d, want %d", exitCode, ExitCanceled)
	}
	if replacements.Load() != 1 {
		t.Fatalf("runFormatWrite() replacements = %d, want 1", replacements.Load())
	}
	if !strings.Contains(stderr.String(), context.Canceled.Error()) ||
		!strings.Contains(stderr.String(), "files replaced before failure") ||
		!strings.Contains(stderr.String(), firstPath) {
		t.Fatalf("runFormatWrite() stderr = %q, want cancellation and prior replacement", stderr.String())
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, secondInput) {
		t.Fatalf("runFormatWrite() second file = %q, want unchanged", second)
	}
}

func TestRunFormatWriteReportsPartialCancellationAsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	secondInput := []byte("package sample\nfunc second(){}\n")
	if err := os.WriteFile(secondPath, secondInput, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runFormatWriteReported(
		ctx,
		formatInvocation{paths: []string{root}, reporter: "json", write: true},
		&stdout,
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			cancel()
			return nil
		},
	)

	if exitCode != ExitCanceled {
		t.Fatalf("runFormatWriteReported() exit = %d, want %d", exitCode, ExitCanceled)
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Outcome.Category != "canceled" || report.Summary.Complete ||
		report.Summary.Changed != 2 || len(report.Errors) != 1 {
		t.Fatalf("runFormatWriteReported() report = %#v", report)
	}
	if report.Files[0].Path != firstPath || report.Files[0].Status != "formatted" ||
		report.Files[1].Path != secondPath || report.Files[1].Status != "pending" {
		t.Fatalf("runFormatWriteReported() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runFormatWriteReported() stderr = %q, want empty", stderr.String())
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, secondInput) {
		t.Fatalf("runFormatWriteReported() second file = %q, want unchanged", second)
	}
}

func TestRunFormatWriteReportsUncertainStateAfterReplacementError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runFormatWrite(
		context.Background(),
		formatInvocation{paths: []string{path}, write: true},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			return errStream
		},
	)

	if exitCode != ExitFilesystemError {
		t.Fatalf("runFormatWrite() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "files replaced or possibly replaced before failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("runFormatWrite() stderr = %q, want uncertain replacement report", stderr.String())
	}
}

func TestRunFormatWriteDisclosesPossiblyFormattedFileWhenJSONOutputFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runFormatWriteReported(
		context.Background(),
		formatInvocation{paths: []string{path}, reporter: "json", write: true},
		failingWriter{},
		&stderr,
		func(snapshot *filesystem.Snapshot, output []byte) error {
			if err := snapshot.Replace(output); err != nil {
				return err
			}
			return errStream
		},
	)

	if exitCode != ExitFilesystemError {
		t.Fatalf("runFormatWriteReported() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write JSON report") ||
		!strings.Contains(stderr.String(), "files replaced or possibly replaced before reporting failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("runFormatWriteReported() stderr = %q, want reporting failure and uncertain replacement", stderr.String())
	}
}

func TestRunRejectsInvalidWriteModeCombinations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := [][]string{
		{"fmt", "--write"},
		{"fmt", "--write", "--check", path},
		{"fmt", "--write", "--diff", path},
		{"fmt", "--check", "--diff", path},
		{"fmt", "--write", "--fragment=statement", path},
		{"fmt", "--write", "--stdin-filepath=source.go", path},
	}
	for _, arguments := range tests {
		arguments := arguments
		t.Run(strings.Join(arguments[1:], "_"), func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
			if exitCode != ExitInvalidInvocation {
				t.Fatalf("Run(%q) exit = %d, want %d", arguments, exitCode, ExitInvalidInvocation)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(%q) stdout = %q, want empty", arguments, stdout.String())
			}
		})
	}
}

func TestRunRefusesExcludedFileInStdoutMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vendor := filepath.Join(root, "vendor")
	if err := os.Mkdir(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vendor, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunRejectsDirectoryInStdoutMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want empty", stdout.String())
	}
}

func TestRunCheckReportsDifferencesWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	formattedPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(formattedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unformattedPath := filepath.Join(root, "z.go")
	unformatted := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(unformattedPath, unformatted, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendor", "ignored.go"), unformatted, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter=text", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	if stdout.String() != unformattedPath+"\n" {
		t.Fatalf("Run() stdout = %q, want changed path", stdout.String())
	}
	got, err := os.ReadFile(unformattedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, unformatted) {
		t.Fatalf("Run() mutated check-mode file: %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiffReportsUnifiedDifferenceWithoutMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", path}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	want := "--- " + path + ".orig\n" +
		"+++ " + path + "\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package sample\n" +
		"-func run(){}\n" +
		"+\n" +
		"+func run() {}\n"
	if stdout.String() != want {
		t.Fatalf("Run() stdout = %q, want %q", stdout.String(), want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run() mutated diff-mode file: %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiffReportsChangedFilesInPathOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(firstPath, []byte("package sample\nfunc first(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchangedPath := filepath.Join(root, "m.go")
	if err := os.WriteFile(unchangedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lastPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(lastPath, []byte("package sample\nfunc last(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	firstHeader := "--- " + firstPath + ".orig\n"
	lastHeader := "--- " + lastPath + ".orig\n"
	firstIndex := strings.Index(stdout.String(), firstHeader)
	lastIndex := strings.Index(stdout.String(), lastHeader)
	if firstIndex < 0 || lastIndex <= firstIndex {
		t.Fatalf("Run() stdout = %q, want changed files in path order", stdout.String())
	}
	if strings.Contains(stdout.String(), unchangedPath) {
		t.Fatalf("Run() stdout = %q, want unchanged file omitted", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiffReturnsSuccessWithoutOutputForFormattedSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess {
		t.Fatalf("Run() exit code = %d, want %d; stderr = %q", exitCode, ExitSuccess, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run() stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

func TestRunDiffReportsSourceFailureWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(root, "a.go")
	validInput := []byte("package sample\nfunc run(){}\n")
	if err := os.WriteFile(validPath, validInput, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z.go"), []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run() stdout = %q, want no partial difference", stdout.String())
	}
	if !strings.Contains(stderr.String(), "z.go") {
		t.Fatalf("Run() stderr = %q, want source failure", stderr.String())
	}
	got, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, validInput) {
		t.Fatalf("Run() mutated valid file before source failure: %q", got)
	}
}

func TestRunDiffReportsStandardOutputFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--diff", path}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("Run() stderr = %q, want output failure", stderr.String())
	}
}

func TestRunCheckReportsVersionedJSONOutcomesInPathOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unchangedPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(unchangedPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(changedPath, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter", "json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings {
		t.Fatalf("Run() exit = %d, want %d; stderr = %q", exitCode, ExitFindings, stderr.String())
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.SchemaVersion != 1 || report.Command != "fmt" || report.Mode != "check" ||
		report.Outcome.Category != "findings" || report.Outcome.ExitCode != ExitFindings {
		t.Fatalf("Run() report header = %#v", report)
	}
	if report.Summary.Files != 2 || report.Summary.Changed != 1 || !report.Summary.Complete || len(report.Files) != 2 || len(report.Errors) != 0 {
		t.Fatalf("Run() report totals = %#v", report)
	}
	if report.Files[0].Path != unchangedPath || report.Files[0].Status != "unchanged" ||
		report.Files[1].Path != changedPath || report.Files[1].Status != "different" {
		t.Fatalf("Run() file outcomes = %#v", report.Files)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckReportsSourceFailureAsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "broken.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter=json", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit = %d, want %d; stderr = %q", exitCode, ExitSourceError, stderr.String())
	}
	report := decodeFormatJSONReport(t, stdout.Bytes())
	if report.Outcome.Category != "source_error" || report.Outcome.ExitCode != ExitSourceError || report.Summary.Complete || len(report.Errors) != 1 {
		t.Fatalf("Run() report = %#v", report)
	}
	if !strings.Contains(report.Errors[0].Message, path) || len(report.Files) != 0 {
		t.Fatalf("Run() report error = %#v", report)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckResolvesConfigurationPerDiscoveredFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gox.toml"), []byte("version = 1\n[format]\nline-width = 30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flat := []byte("package sample\n\nfunc run() {\n\tif firstCondition && secondCondition && thirdCondition {\n\t\twork()\n\t}\n}\n")
	outerPath := filepath.Join(root, "outer.go")
	if err := os.WriteFile(outerPath, flat, 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gox.toml"), []byte("version = 1\n[format]\nline-width = 100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.go"), flat, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitFindings || stdout.String() != outerPath+"\n" {
		t.Fatalf("Run() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunCheckValidatesAllConfigurationBeforeReporting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gox.toml"), []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "z.go"), []byte("package nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCheckValidatesConfigurationForEmptySelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gox.toml"), []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitInvalidInvocation)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
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

	for _, arguments := range [][]string{nil, {"unknown"}} {
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

func TestRunReportsInvalidJSONInvocationAsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arguments []string
		mode      string
	}{
		{arguments: []string{"fmt", "--reporter=json"}, mode: "stdout"},
		{arguments: []string{"fmt", "--reporter=json", "source.go"}, mode: "stdout"},
		{arguments: []string{"fmt", "--reporter", "json", "--unsupported"}, mode: "invalid"},
		{arguments: []string{"fmt", "--check", "--reporter=json", "--unsupported"}, mode: "check"},
		{arguments: []string{"fmt", "--diff", "--reporter=json", "source.go"}, mode: "diff"},
		{arguments: []string{"fmt", "--write", "--reporter=json", "--unsupported"}, mode: "write"},
		{arguments: []string{"fmt", "--check", "--write", "--reporter=json"}, mode: "invalid"},
	}
	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(test.arguments, bytes.NewReader(nil), &stdout, &stderr)

		if exitCode != ExitInvalidInvocation {
			t.Fatalf("Run(%q) exit = %d, want %d", test.arguments, exitCode, ExitInvalidInvocation)
		}
		report := decodeFormatJSONReport(t, stdout.Bytes())
		if report.Mode != test.mode || report.Outcome.Category != "invalid_invocation" || report.Outcome.ExitCode != ExitInvalidInvocation ||
			report.Summary.Complete || len(report.Errors) != 1 {
			t.Fatalf("Run(%q) report = %#v, want mode %q", test.arguments, report, test.mode)
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%q) stderr = %q, want empty", test.arguments, stderr.String())
		}
	}
}

func TestRunReportsJSONOutputFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--check", "--reporter=json", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write JSON report") {
		t.Fatalf("Run() stderr = %q, want JSON write failure", stderr.String())
	}
}

func TestRunWriteDisclosesReplacementWhenJSONOutputFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := Run([]string{"fmt", "--write", "--reporter=json", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError {
		t.Fatalf("Run() exit = %d, want %d", exitCode, ExitFilesystemError)
	}
	if !strings.Contains(stderr.String(), "write JSON report") ||
		!strings.Contains(stderr.String(), "files replaced before reporting failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("Run() stderr = %q, want JSON failure and replacement disclosure", stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package sample\n\nfunc run() {}\n" {
		t.Fatalf("Run() file = %q, want formatted replacement", got)
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
