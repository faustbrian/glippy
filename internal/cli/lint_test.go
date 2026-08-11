package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/filesystem"
	fixengine "github.com/faustbrian/gox/internal/fix"
	goxreport "github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type cliSyntaxRule struct {
	metadata rules.Metadata
}

type cliTypesRule struct {
	metadata rules.Metadata
}

type cliControlFlowRule struct {
	metadata rules.Metadata
}

type cliSSARule struct {
	metadata rules.Metadata
}

type cliFixRule struct {
	metadata    rules.Metadata
	target      string
	replacement string
}

type cliPostFixFailureRule struct {
	cliFixRule
}

type cliAlternativeFixRule struct {
	cliFixRule
	secondSafety rules.FixSafety
}

type lintMutationContext struct {
	context.Context
	calls    int
	mutateAt int
	mutate   func()
}

type lintDiskChangeContext struct {
	context.Context
	cancel context.CancelFunc
	path   string
	needle string
}

func (c *lintMutationContext) Err() error {
	c.calls++
	if c.calls == c.mutateAt {
		c.mutate()
	}
	return c.Context.Err()
}

func (c *lintDiskChangeContext) Err() error {
	input, err := os.ReadFile(c.path)
	if err == nil && bytes.Contains(input, []byte(c.needle)) {
		c.cancel()
	}
	return c.Context.Err()
}

func TestRunExposesBuiltInNilnessThroughLintAndExplain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/nilness\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(`package sample
func inspect(pointer *int) {
	if pointer == nil {
		_ = *pointer
	}
}
`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	want := path + ":4:7: warn[nilness]: nil dereference in load\n" +
		"  help: run `gox explain nilness` for the rule contract and limitations\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("Run(lint nilness) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint nilness) mutated source: %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"explain", "nilness"}, strings.NewReader(""), &stdout, &stderr)
	for _, contract := range []string{
		"nilness\n",
		"analysis tier: SSA\n",
		"generated files: excluded\n",
		"type-error packages: excluded\n",
		"fixes:\n  none\n",
	} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf("Run(explain nilness) output does not contain %q:\n%s", contract, stdout.String())
		}
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(explain nilness) = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunAppliesConfiguredSuppressionReasonPolicyAcrossSyntaxCommands(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/reasons\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint.suppressions]\nrequire-reason = true\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(`//gox:ignore-file duplicate-condition
package sample

func run(ready bool) {
	if ready {
	} else if ready {
	}
}
`)
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}

	for _, arguments := range [][]string{
		{"lint", path},
		{"check", path},
		{"lint", "--fix", path},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitFindings || stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), "suppression[missing-reason]: suppression requires a non-empty reason") {
			t.Fatalf("Run(%q) = exit %d, stdout %q, stderr %q", arguments, exitCode, stdout.String(), stderr.String())
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, input) {
			t.Fatalf("Run(%q) mutated source: %q", arguments, got)
		}
	}
}

func TestRunAppliesConfiguredSuppressionReasonPolicyToPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/typedreasons\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n[lint.suppressions]\nrequire-reason = true\nexpiry-cutoff = \"2026-08-11\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(`//gox:ignore-file nilness
//gox:ignore-file nilness -- expires=2026-08-11 temporary compatibility
package sample

func inspect(pointer *int) {
	if pointer == nil {
		_ = *pointer
	}
}
`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"lint", "check"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run([]string{command, "--reporter=json", path}, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitFindings || stderr.Len() != 0 {
			t.Fatalf("Run(%s typed reasons) = exit %d, stdout %q, stderr %q", command, exitCode, stdout.String(), stderr.String())
		}
		if command == "lint" {
			var result goxreport.LintResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode typed lint JSON: %v; output = %q", err, stdout.String())
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "nilness" ||
				len(result.SuppressionProblems) != 2 ||
				string(result.SuppressionProblems[0].Kind) != "missing-reason" ||
				string(result.SuppressionProblems[1].Kind) != "expired" {
				t.Fatalf("Run(typed lint reasons) result = %#v", result)
			}
		} else {
			var result goxreport.CheckResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode typed check JSON: %v; output = %q", err, stdout.String())
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "nilness" ||
				len(result.SuppressionProblems) != 2 ||
				string(result.SuppressionProblems[0].Kind) != "missing-reason" ||
				string(result.SuppressionProblems[1].Kind) != "expired" {
				t.Fatalf("Run(typed check reasons) result = %#v", result)
			}
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, input) {
			t.Fatalf("Run(%s typed reasons) mutated source: %q", command, got)
		}
	}
}

func TestRunAppliesConfiguredSuppressionExpiryCutoff(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/expiry\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gox.toml"),
		[]byte("version = 1\n[lint.suppressions]\nexpiry-cutoff = \"2026-08-11\"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(`//gox:ignore-file duplicate-condition -- expires=2026-08-11 temporary compatibility
package sample

func run(ready bool) {
	if ready {
	} else if ready {
	}
}
`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, arguments := range [][]string{
		{"lint", path},
		{"check", path},
		{"lint", "--fix", path},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitFindings || stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), "suppression[expired]: suppression expired on 2026-08-11") ||
			!strings.Contains(stdout.String(), "duplicate-condition") {
			t.Fatalf("Run(%q expiry) = exit %d, stdout %q, stderr %q", arguments, exitCode, stdout.String(), stderr.String())
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, input) {
			t.Fatalf("Run(%q expiry) mutated source: %q", arguments, got)
		}
	}
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

func (r cliTypesRule) Metadata() rules.Metadata { return r.metadata }

func (r cliTypesRule) RunTypes(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
	sourceRange, err := ctx.Range(node)
	if err != nil {
		return nil, err
	}
	return []rules.Finding{{
		MessageKey: "typed-call",
		Message:    "typed call requires review",
		Range:      sourceRange,
	}}, nil
}

func (r cliControlFlowRule) Metadata() rules.Metadata { return r.metadata }

func (r cliControlFlowRule) RunControlFlow(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
	sourceRange, err := ctx.Range(ctx.Function())
	if err != nil {
		return nil, err
	}
	return []rules.Finding{{
		MessageKey: "cfg-function",
		Message:    "function requires control-flow review",
		Range:      sourceRange,
	}}, nil
}

func (r cliSSARule) Metadata() rules.Metadata { return r.metadata }

func (r cliSSARule) RunSSA(ctx *rules.SSAContext) ([]rules.Finding, error) {
	sourceRange, err := ctx.Range(ctx.Syntax())
	if err != nil {
		return nil, err
	}
	return []rules.Finding{{
		MessageKey: "ssa-function",
		Message:    "function requires SSA review",
		Range:      sourceRange,
	}}, nil
}

func (r cliFixRule) Metadata() rules.Metadata { return r.metadata }

func (r cliFixRule) RunSyntax(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, nil
	}
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok || identifier.Name != r.target {
		return nil, nil
	}
	sourceRange, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	fix := r.metadata.Fixes[0]
	return []rules.Finding{{
		MessageKey: "target-call",
		Message:    "target call requires replacement",
		Range:      sourceRange,
		Fixes: []rules.Fix{{
			Name:   fix.Name,
			Safety: fix.Safety,
			Edits:  []rules.Edit{{Range: sourceRange, NewText: r.replacement + "()"}},
		}},
	}}, nil
}

func (r cliPostFixFailureRule) RunSyntax(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if ok {
		identifier, isIdentifier := call.Fun.(*ast.Ident)
		if isIdentifier && identifier.Name == r.replacement {
			return nil, errors.New("post-fix analysis failed")
		}
	}
	return r.cliFixRule.RunSyntax(ctx, node)
}

func (r cliAlternativeFixRule) RunSyntax(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
	findings, err := r.cliFixRule.RunSyntax(ctx, node)
	if err != nil || len(findings) == 0 {
		return findings, err
	}
	alternative := findings[0].Fixes[0]
	alternative.Name = "alternative"
	alternative.Safety = r.secondSafety
	findings[0].Fixes = append(findings[0].Fixes, alternative)
	return findings, nil
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

func TestRunLintCheckUsesOneConfigurationSnapshotForTierAndExecution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600); err != nil {
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
	ctx := &lintMutationContext{
		Context:  context.Background(),
		mutateAt: 3,
		mutate: func() {
			if err := os.WriteFile(
				configurationPath,
				[]byte("version = 1\n[lint.rules]\ncall-rule = \"off\"\n"),
				0o600,
			); err != nil {
				t.Error(err)
			}
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		ctx,
		lintInvocation{configPath: configurationPath, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLISyntaxRegistry(t),
	)

	want := path + ":2:12: error[call-rule]: call requires review\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(config snapshot) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
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

func TestRunLintUsesDefaultRegistryForCleanAndSuppressionOutcomes(t *testing.T) {
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
	stdout.Reset()
	stderr.Reset()
	if exitCode := Run([]string{"lint", "--fix", cleanPath}, failingReader{}, &stdout, &stderr); exitCode != ExitSuccess ||
		stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(lint --fix clean) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
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
		[]string{"lint", "--unsafe-fix", "--reporter=json", "source.go"},
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

func TestRunLintPackagePatternKeepsSyntaxOnlyRulesOffPackageLoading(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package project\nimport _ \"example.invalid/missing\"\nfunc run(ready bool) { if ready {} else if ready {} }\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"lint", filepath.Join(root, "...")},
		failingReader{},
		&stdout,
		&stderr,
	)

	if exitCode != ExitFindings || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "warn[duplicate-condition]") ||
		strings.Contains(stdout.String(), "package[") {
		t.Fatalf("Run(lint package pattern) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunLintCheckRoutesTypedPackagePatternsToPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package project\nfunc run() { target() }\nfunc target() {}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	want := path + ":2:14: warn[typed-call]: typed call requires review\n"
	for _, input := range []string{path, root, filepath.Join(root, "...")} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runLintCheck(
			context.Background(),
			lintInvocation{paths: []string{input}, reporter: goxreport.Text},
			&stdout,
			&stderr,
			newCLITypesRegistry(t),
		)
		if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runLintCheck(typed input %q) = exit %d, stdout %q, stderr %q", input, exitCode, stdout.String(), stderr.String())
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintCheck(typed inputs) mutated source: %q", got)
	}
}

func TestRunLintCheckRoutesControlFlowRulesThroughPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package project\nfunc run() {}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIControlFlowRegistry(t),
	)
	want := path + ":2:1: warn[cfg-function]: function requires control-flow review\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(CFG) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintCheck(CFG) mutated source: %q", got)
	}
}

func TestRunLintCheckRoutesSSARulesThroughPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package project\nfunc run() {}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLISSARegistry(t),
	)
	want := path + ":2:1: warn[ssa-function]: function requires SSA review\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(SSA) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintCheck(SSA) mutated source: %q", got)
	}
}

func TestRunLintCheckReportsTypedPrerequisiteFailuresAsSourceErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "source.go"),
		[]byte("package project\nfunc run() { _ = missing; target() }\nfunc target() {}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	registry := newCLITypesRegistry(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{filepath.Join(root, "...")}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(typed error) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode typed lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError || result.Outcome.Category != "source_error" ||
		!result.Summary.Complete || result.Summary.PackageDiagnostics == 0 ||
		len(result.PackageDiagnostics) == 0 || len(result.Errors) != 0 {
		t.Fatalf("typed source-error JSON = %#v", result)
	}
}

func TestRunLintCheckReportsTypedSourceModelFailuresAsSourceErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "source.go"),
		[]byte("package project\nfunc broken( {\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	registry := newCLITypesRegistry(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{filepath.Join(root, "...")}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(typed source problem) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode typed source-problem JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError || !result.Summary.Complete ||
		result.Summary.SourceProblems == 0 || len(result.SourceProblems) == 0 ||
		result.SourceProblems[0].Path != filepath.Join(root, "source.go") {
		t.Fatalf("typed source-problem JSON = %#v", result)
	}
}

func TestRunLintCheckRejectsHeterogeneousTypedPackageRoots(t *testing.T) {
	t.Parallel()

	paths := make([]string, 0, 2)
	for _, module := range []string{"one", "two"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/"+module+"\n\ngo 1.26.0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package "+module+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Join(root, "..."))
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: paths, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLITypesRegistry(t),
	)

	if exitCode != ExitInvalidInvocation || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "one project root and configuration") {
		t.Fatalf("runLintCheck(heterogeneous roots) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunLintFixRejectsTypedPackageAnalysisBeforeMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package project\nfunc run() { target() }\nfunc target() {}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{filepath.Join(root, "...")}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLITypesRegistry(t),
	)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != ExitInvalidInvocation || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "typed lint fixes are not supported") || !bytes.Equal(got, input) {
		t.Fatalf("runLintFix(typed) = exit %d, stdout %q, stderr %q, source %q", exitCode, stdout.String(), stderr.String(), got)
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
		if got := task.options.analysis.Overrides["call-rule"]; got != rules.SeverityWarn {
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

func TestRunLintFixAppliesAndFormatsOneSafeFix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n}\n"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("runLintFix() source = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("runLintFix() permissions = %o, want 640", info.Mode().Perm())
	}
}

func TestRunLintFixLeavesSuggestionDiagnosticsUnchanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSuggestion),
	)

	if exitCode != ExitFindings || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "warn[fix-rule]: target call requires replacement") ||
		!strings.Contains(stdout.String(), "fix[suggestion]: rewrite") {
		t.Fatalf("runLintFix() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintFix() changed suggestion-only source: %q", got)
	}
}

func TestRunLintFixAppliesOnlyExplicitSuggestionFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fixSuggestions: true, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSuggestion),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("runLintFix(suggestion) exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n}\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(suggestion) source = %q, error = %v", got, err)
	}
}

func TestRunLintFixAppliesOnlyExplicitUnsafeFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fixUnsafe: true, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixUnsafe),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("runLintFix(unsafe) exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n}\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(unsafe) source = %q, error = %v", got, err)
	}
}

func TestParseLintInvocationAcceptsIndependentFixModes(t *testing.T) {
	t.Parallel()

	invocation, valid := parseLintInvocation([]string{
		"lint",
		"--fix",
		"--fix-suggestions",
		"--fix-unsafe",
		"source.go",
	})

	if !valid || !invocation.fix || !invocation.fixSuggestions || !invocation.fixUnsafe ||
		len(invocation.paths) != 1 || invocation.paths[0] != "source.go" {
		t.Fatalf("parseLintInvocation() = %#v, %t", invocation, valid)
	}
}

func TestRunLintExplicitFixModesUseFixReporter(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--fix-suggestions", "--fix-unsafe"} {
		flag := flag
		t.Run(flag, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "source.go")
			input := []byte("package sample\n")
			if err := os.WriteFile(path, input, 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run(
				[]string{"lint", flag, "--reporter=json", path},
				failingReader{},
				&stdout,
				&stderr,
			)

			if exitCode != ExitSuccess || stderr.Len() != 0 {
				t.Fatalf("Run(lint %s) exit = %d, stderr = %q", flag, exitCode, stderr.String())
			}
			var result goxreport.LintResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode lint %s JSON: %v; output = %q", flag, err, stdout.String())
			}
			if result.Mode != "fix" || !result.Summary.Complete || result.Outcome.ExitCode != ExitSuccess {
				t.Fatalf("Run(lint %s) result = %#v", flag, result)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, input) {
				t.Fatalf("Run(lint %s) source = %q, error = %v", flag, got, err)
			}
		})
	}
}

func TestRunLintFixPrevalidatesAmbiguousEnabledAlternativesBeforeWriting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package sample\nfunc first(){firstTarget()}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "z.go")
	secondInput := []byte("package sample\nfunc second(){secondTarget()}\n")
	if err := os.WriteFile(secondPath, secondInput, 0o600); err != nil {
		t.Fatal(err)
	}
	firstRule := newCLIFixRuleFor("first-fix", "firstTarget", "primary", rules.FixSafe)
	secondRule := cliAlternativeFixRule{
		cliFixRule:   newCLIFixRuleFor("second-fix", "secondTarget", "secondary", rules.FixSafe),
		secondSafety: rules.FixSuggestion,
	}
	secondRule.metadata.Fixes = append(secondRule.metadata.Fixes, rules.FixMetadata{
		Name:        "alternative",
		Description: "replace the target call with an alternative",
		Safety:      rules.FixSuggestion,
	})
	registry, err := rules.NewRegistry(firstRule, secondRule)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix:            true,
			fixSuggestions: true,
			paths:          []string{firstPath, secondPath},
			reporter:       goxreport.Text,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitInternalError || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "multiple authorized fixes") {
		t.Fatalf("runLintFix(ambiguous) exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	for path, input := range map[string][]byte{firstPath: firstInput, secondPath: secondInput} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, input) {
			t.Fatalf("runLintFix(ambiguous) changed %s: %q", path, got)
		}
	}
}

func TestRunLintFixReportsConflictsInJSONWithoutWriting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(
		newCLIFixRule("first-fix", "first", rules.FixSafe),
		newCLIFixRule("second-fix", "second", rules.FixSafe),
	)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitConflict || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Mode != "fix" || result.Outcome.ExitCode != ExitConflict || !result.Summary.Complete ||
		result.Summary.RejectedFixes != 2 || len(result.Files) != 1 ||
		result.Files[0].Status != goxreport.LintFileConflict || len(result.RejectedFixes) != 2 {
		t.Fatalf("lint fix JSON = %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintFix() changed conflicting source: %q", got)
	}
}

func TestRunLintFixReportsPostFormatAnalysisFailureAsInternalWithoutWriting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	rule := cliPostFixFailureRule{cliFixRule: newCLIFixRule("fix-rule", "primary", rules.FixSafe)}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitInternalError || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode failed lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Summary.Complete || result.Outcome.ExitCode != ExitInternalError ||
		len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "post-fix analysis failed") {
		t.Fatalf("failed lint fix JSON = %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintFix() changed source after analysis failure: %q", got)
	}
}

func TestRunLintFixDisclosesCompletedWritesWhenTextReportingFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target();other()}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(
		newCLIFixRule("first-fix", "first", rules.FixSafe),
		newCLIFixRule("second-fix", "second", rules.FixSafe),
		newCLIFixRuleFor("independent-fix", "other", "independent", rules.FixSafe),
	)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.Text},
		failingWriter{},
		&stderr,
		registry,
	)

	if exitCode != ExitFilesystemError || !strings.Contains(stderr.String(), "files fixed before failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "independent()") {
		t.Fatalf("runLintFix() source = %q, want completed independent fix", got)
	}
}

func TestRunLintFixDisclosesCompletedWritesWhenJSONReportingFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.JSON},
		failingWriter{},
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitFilesystemError ||
		!strings.Contains(stderr.String(), "write fix JSON report") ||
		!strings.Contains(stderr.String(), "files fixed before reporting failure") ||
		!strings.Contains(stderr.String(), path) {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
}

func TestRunLintFixJSONReportsConfirmedReplacement(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Mode != "fix" || result.Outcome.ExitCode != ExitSuccess || !result.Summary.Complete ||
		result.Summary.FixedFiles != 1 || result.Summary.AppliedFixes != 1 ||
		len(result.Files) != 1 || result.Files[0].Status != goxreport.LintFileFixed ||
		result.Files[0].SourceDigest == result.Files[0].ResultDigest || len(result.AppliedFixes) != 1 {
		t.Fatalf("lint fix JSON = %#v", result)
	}
}

func TestRunLintFixPrevalidatesEverySourceBeforeWriting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	generatedPath := filepath.Join(root, "z.go")
	if err := os.WriteFile(
		generatedPath,
		[]byte("// Code generated by fixture. DO NOT EDIT.\npackage sample\nfunc generated(){target()}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{generatedPath, firstPath}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitFilesystemError || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "refusing to fix generated file") {
		t.Fatalf("runLintFix() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, firstInput) {
		t.Fatalf("runLintFix() wrote earlier source before complete prevalidation: %q", got)
	}
}

func TestRunLintFixRefusesPathThroughSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetDirectory := t.TempDir()
	target := filepath.Join(targetDirectory, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
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

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: goxreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitFilesystemError || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "refusing to fix symlink") {
		t.Fatalf("runLintFix() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintFix() changed file through symlinked directory: %q", got)
	}
}

func TestRunLintFixCancellationReportsConfirmedWriteAndPendingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	secondPath := filepath.Join(root, "b.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	for _, path := range []string{firstPath, secondPath} {
		if err := os.WriteFile(path, input, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	base, cancel := context.WithCancel(context.Background())
	ctx := &lintDiskChangeContext{
		Context: base,
		cancel:  cancel,
		path:    firstPath,
		needle:  "primary()",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		ctx,
		lintInvocation{fix: true, paths: []string{root}, reporter: goxreport.JSON},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitCanceled || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result goxreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode canceled lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Summary.Complete || len(result.Files) != 2 ||
		result.Files[0].Status != goxreport.LintFileFixed ||
		result.Files[1].Status != goxreport.LintFilePending {
		t.Fatalf("canceled lint fix JSON = %#v", result)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first, []byte("primary()")) || !bytes.Equal(second, input) {
		t.Fatalf("canceled lint fix sources = first %q, second %q", first, second)
	}
}

func TestRecordLintFixTransactionKeepsOriginalResultAfterStaleWrite(t *testing.T) {
	t.Parallel()

	before, err := source.Load("/project/source.go", []byte("package sample\nfunc run(){target()}\n"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := source.Load("/project/source.go", []byte("package sample\n\nfunc run() {\n\tprimary()\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	execution := lintFixExecution{
		file:       before,
		resultFile: before,
		result:     analysis.Result{Path: before.Path(), Digest: before.Digest()},
		outcome: goxreport.LintFixOutcome{
			Path:         before.Path(),
			SourceDigest: before.Digest(),
			Status:       goxreport.LintFilePending,
		},
	}
	transaction := fixengine.Transaction{
		Result: fixengine.Result{
			Applied: []fixengine.Applied{{
				RuleID:  "fix-rule",
				FixName: "rewrite",
				Range:   source.Range{Start: 26, End: 34},
			}},
		},
		Status: fixengine.WriteNotPerformed,
	}

	recordLintFixTransaction(
		&execution,
		analysis.Result{Path: after.Path(), Digest: after.Digest()},
		after,
		transaction,
		filesystem.ErrStale,
	)

	if execution.result.Digest != before.Digest() || execution.resultFile != before ||
		execution.outcome.Status != goxreport.LintFileConflict || len(execution.outcome.Applied) != 1 ||
		len(execution.outcome.Rejected) != 1 ||
		execution.outcome.Rejected[0].Reason != fixengine.RejectionStaleSource ||
		lintFixExitCode([]lintFixExecution{execution}) != ExitConflict {
		t.Fatalf("stale lint fix execution = %#v", execution)
	}
}

func TestLintFixFileStatusPreservesReplacementCertainty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		transaction fixengine.Transaction
		want        goxreport.LintFileStatus
	}{
		{name: "completed", transaction: fixengine.Transaction{Status: fixengine.WriteCompleted}, want: goxreport.LintFileFixed},
		{name: "possible", transaction: fixengine.Transaction{Status: fixengine.WritePossiblyCompleted}, want: goxreport.LintFilePossiblyFixed},
		{
			name: "conflict",
			transaction: fixengine.Transaction{Result: fixengine.Result{Rejected: []fixengine.Rejection{{
				Reason: fixengine.RejectionConflict,
			}}}},
			want: goxreport.LintFileConflict,
		},
		{name: "unchanged", transaction: fixengine.Transaction{Status: fixengine.WriteNotPerformed}, want: goxreport.LintFileUnchanged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := lintFixFileStatus(test.transaction); got != test.want {
				t.Fatalf("lintFixFileStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReportLintFixJSONDisclosesCompletedWritesWhenResultConstructionFails(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	executions := []lintFixExecution{{
		file:       file,
		resultFile: file,
		result:     analysis.Result{Path: "/project/other.go", Digest: file.Digest()},
		outcome: goxreport.LintFixOutcome{
			Path:         file.Path(),
			SourceDigest: file.Digest(),
			Status:       goxreport.LintFileFixed,
		},
	}}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := reportLintFixJSON(&stdout, &stderr, ExitSuccess, true, executions, nil)

	if exitCode != ExitInternalError || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "files fixed before reporting failure") ||
		!strings.Contains(stderr.String(), file.Path()) {
		t.Fatalf("reportLintFixJSON() exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
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

func newCLITypesRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(cliTypesRule{metadata: rules.Metadata{
		ID:               "typed-call",
		Summary:          "reports typed calls",
		Documentation:    "Reports calls that require typed review.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireTypes,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "target()", Correct: "reviewed()"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLIControlFlowRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(cliControlFlowRule{metadata: rules.Metadata{
		ID:               "cfg-function",
		Summary:          "reports functions",
		Documentation:    "Reports functions that require control-flow review.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireControlFlow,
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "func bad() {}", Correct: "func good() {}"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLISSARegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(cliSSARule{metadata: rules.Metadata{
		ID:               "ssa-function",
		Summary:          "reports SSA functions",
		Documentation:    "Reports functions that require SSA review.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSSA,
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "func bad() {}", Correct: "func good() {}"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLIFixRegistry(t *testing.T, safety rules.FixSafety) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(newCLIFixRule("fix-rule", "primary", safety))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLIFixRule(ruleID, replacement string, safety rules.FixSafety) cliFixRule {
	return newCLIFixRuleFor(ruleID, "target", replacement, safety)
}

func newCLIFixRuleFor(ruleID, target, replacement string, safety rules.FixSafety) cliFixRule {
	return cliFixRule{
		target:      target,
		replacement: replacement,
		metadata: rules.Metadata{
			ID:               ruleID,
			Summary:          "replaces target calls",
			Documentation:    "Replaces target calls with an admitted alternative.",
			DefaultSeverity:  rules.SeverityWarn,
			Presets:          []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.22",
			Requirement:      rules.RequireSyntax,
			NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
			Categories:       []rules.Category{rules.CategoryCorrectness},
			Fixes: []rules.FixMetadata{{
				Name:        "rewrite",
				Description: "replace the target call",
				Safety:      safety,
			}},
			Examples: []rules.Example{{Incorrect: "target()", Correct: replacement + "()"}},
		},
	}
}
