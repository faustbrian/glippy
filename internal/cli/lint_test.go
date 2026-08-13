package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/filesystem"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"github.com/faustbrian/glippy/internal/suppressions"
)

type cliSyntaxRule struct {
	metadata rules.Metadata
	optionName string
}

type cliTypesRule struct {
	metadata rules.Metadata
	runs *atomic.Int64
	cancel context.CancelFunc
}

type cliTypesFixRule struct {
	metadata rules.Metadata
	target string
	replacement string
}

type cliPackageStateFixRule struct {
	metadata rules.Metadata
}

type cliPackageEnablingFixRule struct {
	metadata rules.Metadata
}

type cliControlFlowRule struct {
	metadata rules.Metadata
}

type cliSSARule struct {
	metadata rules.Metadata
}

type cliFixRule struct {
	metadata rules.Metadata
	target string
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
	calls int
	mutateAt int
	mutate func()
}

type lintDiskChangeContext struct {
	context.Context
	cancel context.CancelFunc
	path string
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/nilness\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`package sample
func inspect(pointer *int) {
	if pointer == nil {
		_ = *pointer
	}
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	want := path +
		":4:7: warn[nilness]: nil dereference in load\n" +
		"  help: run `glippy explain nilness` for the rule contract and limitations\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint nilness) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
	for _, contract := range
		[]string{
			"nilness\n",
			"analysis tier: SSA\n",
			"generated files: excluded\n",
			"type-error packages: excluded\n",
			"fixes:\n  none\n",
		} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf(
				"Run(explain nilness) output does not contain %q:\n%s",
				contract,
				stdout.String(),
			)
		}
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(explain nilness) = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunExposesBuiltInContextKeyThroughLintAndExplain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/contextkey\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`package sample
import "context"
func attach(ctx context.Context) context.Context {
	return context.WithValue(ctx, "request-id", 1)
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	want := path +
		":4:32: warn[context-key]: context.WithValue key has built-in type string and may collide across packages\n" +
		"  help: use a comparable package-specific defined key type\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint context-key) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint context-key) mutated source: %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"explain", "context-key"}, strings.NewReader(""), &stdout, &stderr)
	for _, contract := range
		[]string{
			"context-key\n",
			"analysis tier: types\n",
			"generated files: excluded\n",
			"type-error packages: excluded\n",
			"fixes:\n  none\n",
		} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf(
				"Run(explain context-key) output does not contain %q:\n%s",
				contract,
				stdout.String(),
			)
		}
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(explain context-key) = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunExposesBuiltInErrorsIsArgumentsThroughLintAndExplain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/errorsisarguments\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`package sample

import (
	"errors"
	"io"
)

func match(err error) bool {
	return errors.Is(io.EOF, err)
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npreset = \"suspicious\"\n[lint.rules]\ncontext-key = \"off\"\ndefer-in-infinite-loop = \"off\"\nnilness = \"off\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	want := path +
		":9:19: warn[errors-is-arguments]: errors.Is arguments appear to be reversed\n" +
		"  help: pass the error value first and the package sentinel second\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint errors-is-arguments) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint errors-is-arguments) mutated source: %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"explain", "errors-is-arguments"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	for _, contract := range
		[]string{
			"errors-is-arguments\n",
			"analysis tier: types\n",
			"generated files: excluded\n",
			"type-error packages: excluded\n",
			"fixes:\n  none\n",
		} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf(
				"Run(explain errors-is-arguments) output does not contain %q:\n%s",
				contract,
				stdout.String(),
			)
		}
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"Run(explain errors-is-arguments) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
}

func TestRunExposesBuiltInDeferInInfiniteLoopThroughLintAndExplain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/deferloop\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`package sample
func cleanup() {}
func run() {
	for {
		defer cleanup()
	}
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npreset = \"suspicious\"\n[lint.rules]\nnilness = \"off\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	want := path +
		":5:3: warn[defer-in-infinite-loop]: defer in this infinite loop cannot reach function exit and will never run\n" +
		"  help: invoke the cleanup explicitly in each iteration or make a function exit reachable\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint defer-in-infinite-loop) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint defer-in-infinite-loop) mutated source: %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"explain", "defer-in-infinite-loop"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	for _, contract := range
		[]string{
			"defer-in-infinite-loop\n",
			"analysis tier: control flow\n",
			"generated files: excluded\n",
			"type-error packages: excluded\n",
			"fixes:\n  none\n",
		} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf(
				"Run(explain defer-in-infinite-loop) output does not contain %q:\n%s",
				contract,
				stdout.String(),
			)
		}
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"Run(explain defer-in-infinite-loop) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
}

func TestRunAppliesConfiguredSuppressionReasonPolicyAcrossSyntaxCommands(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/reasons\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint.suppressions]\nrequire-reason = true\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`//glippy:ignore-file duplicate-condition
package sample

func run(ready bool) {
	if ready {
	} else if ready {
	}
}
`,
	)
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}

	for _, arguments := range
		[][]string{{"lint", path}, {"check", path}, {"lint", "--fix", path}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitFindings ||
			stderr.Len() != 0 ||
			!strings.Contains(
				stdout.String(),
				"suppression[missing-reason]: suppression requires a non-empty reason",
			) {
			t.Fatalf(
				"Run(%q) = exit %d, stdout %q, stderr %q",
				arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/typedreasons\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npreset = \"suspicious\"\n[lint.suppressions]\nrequire-reason = true\nexpiry-cutoff = \"2026-08-11\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`//glippy:ignore-file nilness
//glippy:ignore-file nilness -- expires=2026-08-11 temporary compatibility
package sample

func inspect(pointer *int) {
	if pointer == nil {
		_ = *pointer
	}
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"lint", "check"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{command, "--reporter=json", path},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
		if exitCode != ExitFindings || stderr.Len() != 0 {
			t.Fatalf(
				"Run(%s typed reasons) = exit %d, stdout %q, stderr %q",
				command,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
		if command == "lint" {
			var result glippyreport.LintResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf(
					"decode typed lint JSON: %v; output = %q",
					err,
					stdout.String(),
				)
			}
			if len(result.Diagnostics) != 1 ||
				result.Diagnostics[0].RuleID != "nilness" ||
				len(result.SuppressionProblems) != 2 ||
				string(result.SuppressionProblems[0].Kind) != "missing-reason" ||
				string(result.SuppressionProblems[1].Kind) != "expired" {
				t.Fatalf("Run(typed lint reasons) result = %#v", result)
			}
		} else {
			var result glippyreport.CheckResult
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf(
					"decode typed check JSON: %v; output = %q",
					err,
					stdout.String(),
				)
			}
			if len(result.Diagnostics) != 1 ||
				result.Diagnostics[0].RuleID != "nilness" ||
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/expiry\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint.suppressions]\nexpiry-cutoff = \"2026-08-11\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`//glippy:ignore-file duplicate-condition -- expires=2026-08-11 temporary compatibility
package sample

func run(ready bool) {
	if ready {
	} else if ready {
	}
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, arguments := range
		[][]string{{"lint", path}, {"check", path}, {"lint", "--fix", path}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitFindings ||
			stderr.Len() != 0 ||
			!strings.Contains(
				stdout.String(),
				"suppression[expired]: suppression expired on 2026-08-11",
			) ||
			!strings.Contains(stdout.String(), "duplicate-condition") {
			t.Fatalf(
				"Run(%q expiry) = exit %d, stdout %q, stderr %q",
				arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
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

func (r cliSyntaxRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliSyntaxRule) RunSyntax(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
	if r.optionName != "" {
		enabled, found := ctx.BooleanOption(r.optionName)
		if !found {
			return nil, errors.New("configured CLI rule did not receive its option")
		}
		if !enabled {
			return nil, nil
		}
	}
	sourceRange, err := ctx.Range(node)
	if err != nil {
		return nil, err
	}
	return []rules.Finding{
		{MessageKey: "call", Message: "call requires review", Range: sourceRange},
	}, nil
}

func TestRunLintCheckAppliesTypedRuleOptionsFromConfiguration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/configured\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "configured.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			`version = 1

[lint.rule-options."configured-rule"]
enabled = true
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	metadata := rules.Metadata{
		ID: "configured-rule",
		Summary: "reports configured calls",
		Documentation: "Reports calls when configured.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement: rules.RequireSyntax,
		NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
		Categories: []rules.Category{rules.CategoryCorrectness},
		Options: []rules.OptionMetadata{
			{
				Name: "enabled",
				Summary: "enable reporting",
				Kind: rules.OptionBoolean,
				Required: true,
			},
		},
		Examples: []rules.Example{{Incorrect: "target()", Correct: "reviewed()"}},
	}
	registry, err := rules.NewRegistry(cliSyntaxRule{metadata: metadata, optionName: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "configured-rule") {
		t.Fatalf(
			"runLintCheck() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	if err := os.WriteFile(configurationPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exitCode = runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitInvalidInvocation ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "missing required option \"enabled\"") {
		t.Fatalf(
			"runLintCheck() missing option = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func (r cliTypesRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliTypesRule) RunTypes(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
	if r.runs != nil {
		r.runs.Add(1)
	}
	if r.cancel != nil {
		r.cancel()
	}
	sourceRange, err := ctx.Range(node)
	if err != nil {
		return nil, err
	}
	return []rules.Finding{
		{
			MessageKey: "typed-call",
			Message: "typed call requires review",
			Range: sourceRange,
		},
	}, nil
}

func (r cliTypesFixRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliTypesFixRule) RunTypes(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
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
	return []rules.Finding{
		{
			MessageKey: "typed-target-call",
			Message: "typed target call requires replacement",
			Range: sourceRange,
			Fixes: []rules.Fix{
				{
					Name: fix.Name,
					Safety: fix.Safety,
					Edits: []rules.Edit{
						{Range: sourceRange, NewText: r.replacement + "()"},
					},
				},
			},
		},
	}, nil
}

func (r cliPackageStateFixRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliPackageStateFixRule) RunTypes(
	ctx *rules.TypesContext,
	node ast.Node,
) ([]rules.Finding, error) {
	gate, ok := ctx.Package().Scope().Lookup("Gate").(*types.Const)
	if !ok || gate.Val().ExactString() != "true" {
		return nil, nil
	}
	var replacement string
	switch current := node.(type) {
	case *ast.Ident:
		if current.Name != "true" {
			return nil, nil
		}
		replacement = "false"
	case *ast.CallExpr:
		identifier, ok := current.Fun.(*ast.Ident)
		if !ok || identifier.Name != "target" {
			return nil, nil
		}
		replacement = "primary()"
	default:
		return nil, nil
	}
	sourceRange, err := ctx.Range(node)
	if err != nil {
		return nil, err
	}
	fix := r.metadata.Fixes[0]
	return []rules.Finding{
		{
			MessageKey: "package-state",
			Message: "package state permits a rewrite",
			Range: sourceRange,
			Fixes: []rules.Fix{
				{
					Name: fix.Name,
					Safety: fix.Safety,
					Edits: []rules.Edit{
						{Range: sourceRange, NewText: replacement},
					},
				},
			},
		},
	}, nil
}

func (r cliPackageEnablingFixRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliPackageEnablingFixRule) RunTypes(
	ctx *rules.TypesContext,
	node ast.Node,
) ([]rules.Finding, error) {
	gate, ok := ctx.Package().Scope().Lookup("Gate").(*types.Const)
	if !ok {
		return nil, nil
	}
	gateEnabled := gate.Val().ExactString() == "true"
	var replacement string
	switch current := node.(type) {
	case *ast.Ident:
		if gateEnabled || current.Name != "false" {
			return nil, nil
		}
		replacement = "true"
	case *ast.CallExpr:
		identifier, ok := current.Fun.(*ast.Ident)
		if !gateEnabled || !ok || identifier.Name != "target" {
			return nil, nil
		}
	default:
		return nil, nil
	}
	sourceRange, err := ctx.Range(node)
	if err != nil {
		return nil, err
	}
	finding := rules.Finding{
		MessageKey: "package-enabled",
		Message: "enabled package state requires review",
		Range: sourceRange,
	}
	if replacement != "" {
		fix := r.metadata.Fixes[0]
		finding.Fixes = []rules.Fix{
			{
				Name: fix.Name,
				Safety: fix.Safety,
				Edits: []rules.Edit{{Range: sourceRange, NewText: replacement}},
			},
		}
	}
	return []rules.Finding{finding}, nil
}

func (r cliControlFlowRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliControlFlowRule) RunControlFlow(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
	sourceRange, err := ctx.Range(ctx.Function())
	if err != nil {
		return nil, err
	}
	return []rules.Finding{
		{
			MessageKey: "cfg-function",
			Message: "function requires control-flow review",
			Range: sourceRange,
		},
	}, nil
}

func (r cliSSARule) Metadata() rules.Metadata {
	return r.metadata
}

func (r cliSSARule) RunSSA(ctx *rules.SSAContext) ([]rules.Finding, error) {
	sourceRange, err := ctx.Range(ctx.Syntax())
	if err != nil {
		return nil, err
	}
	return []rules.Finding{
		{
			MessageKey: "ssa-function",
			Message: "function requires SSA review",
			Range: sourceRange,
		},
	}, nil
}

func (r cliFixRule) Metadata() rules.Metadata {
	return r.metadata
}

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
	return []rules.Finding{
		{
			MessageKey: "target-call",
			Message: "target call requires replacement",
			Range: sourceRange,
			Fixes: []rules.Fix{
				{
					Name: fix.Name,
					Safety: fix.Safety,
					Edits: []rules.Edit{
						{Range: sourceRange, NewText: r.replacement + "()"},
					},
				},
			},
		},
	}, nil
}

func (r cliPostFixFailureRule) RunSyntax(
	ctx *rules.Context,
	node ast.Node,
) ([]rules.Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if ok {
		identifier, isIdentifier := call.Fun.(*ast.Ident)
		if isIdentifier && identifier.Name == r.replacement {
			return nil, errors.New("post-fix analysis failed")
		}
	}
	return r.cliFixRule.RunSyntax(ctx, node)
}

func (r cliAlternativeFixRule) RunSyntax(
	ctx *rules.Context,
	node ast.Node,
) ([]rules.Finding, error) {
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
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[lint.rules]\ncall-rule = \"error\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	registry := newCLISyntaxRegistry(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{
			configPath: configurationPath,
			paths: []string{path},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitFindings {
		t.Fatalf(
			"runLintCheck() exit = %d, want %d; stderr = %q",
			exitCode,
			ExitFindings,
			stderr.String(),
		)
	}
	want := path + ":2:12: error[call-rule]: call requires review\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck() stdout = %q, stderr = %q",
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintCheck() mutated source: %q", got)
	}
}

func TestRunLintGenerateBaselineWritesVisibleDiagnostics(t *testing.T) {
	t.Parallel()

	invocation, valid := parseLintInvocation(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", "source.go"},
	)
	if !valid ||
		invocation.generateBaseline != ".glippy-baseline.json" ||
		len(invocation.paths) != 1 {
		t.Fatalf("parseLintInvocation() = %#v, %t", invocation, valid)
	}
	if _, valid := parseLintInvocation(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", "--fix", "source.go"},
	);
		valid {
		t.Fatal("parseLintInvocation() accepted baseline generation with fixes")
	}
	if _, valid := parseLintInvocation(
		[]string{
			"lint",
			"--generate-baseline=.glippy-baseline.json",
			"--reporter=json",
			"source.go",
		},
	);
		valid {
		t.Fatal("parseLintInvocation() accepted JSON baseline generation")
	}

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/sample\n\ngo 1.25\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[lint.rules]\ncall-rule = \"error\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	invocation.configPath = configurationPath
	invocation.paths = []string{path}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintGenerateBaseline(
		context.Background(),
		invocation,
		&stdout,
		&stderr,
		newCLISyntaxRegistry(t),
	)

	baselinePath := filepath.Join(root, ".glippy-baseline.json")
	encoded, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != ExitSuccess ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "wrote baseline") ||
		!bytes.Contains(encoded, []byte(`"rule_id": "call-rule"`)) ||
		bytes.Contains(encoded, []byte("target()")) {
		t.Fatalf(
			"runLintGenerateBaseline() = exit %d, stdout %q, stderr %q, baseline %q",
			exitCode,
			stdout.String(),
			stderr.String(),
			encoded,
		)
	}
}

func TestRunLintCheckAppliesConfiguredBaselineAndReportsStaleEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/sample\n\ngo 1.25\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[lint.rules]\ncall-rule = \"error\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	registry := newCLISyntaxRegistry(t)
	invocation := lintInvocation{
		configPath: configurationPath,
		generateBaseline: ".glippy-baseline.json",
		paths: []string{path},
		reporter: glippyreport.Text,
	}
	if exitCode := runLintGenerateBaseline(
		context.Background(),
		invocation,
		io.Discard,
		io.Discard,
		registry,
	);
		exitCode != ExitSuccess {
		t.Fatalf("runLintGenerateBaseline() exit = %d", exitCode)
	}
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			"version = 1\n" +
				"[lint.rules]\ncall-rule = \"error\"\n" +
				"[lint.baseline]\npath = \".glippy-baseline.json\"\nreport-stale = true\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	invocation.generateBaseline = ""
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := runLintCheck(context.Background(), invocation, &stdout, &stderr, registry);
		exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(baselined) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runCombinedCheck(
		context.Background(),
		checkInvocation{
			configPath: configurationPath,
			paths: []string{path},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		registry,
	);
		exitCode != ExitFindings ||
			strings.Contains(stdout.String(), "call-rule") ||
			!strings.Contains(stdout.String(), "format differs") ||
			stderr.Len() != 0 {
		t.Fatalf(
			"runCombinedCheck(baselined) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	if err := os.WriteFile(path, []byte("package sample\nfunc run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runLintCheck(context.Background(), invocation, &stdout, &stderr, registry);
		exitCode != ExitFindings ||
			!strings.Contains(stdout.String(), "baseline[stale]") ||
			stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(stale baseline) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintFixDoesNotApplyBaselinedFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/sample\n\ngo 1.25\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){target()}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	writeConfig := func(baselined bool) {
		t.Helper()
		contents := "version = 1\n[lint.rules]\nfix-rule = \"error\"\n"
		if baselined {
			contents += "[lint.baseline]\npath = \".glippy-baseline.json\"\n"
		}
		if err := os.WriteFile(configurationPath, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(false)
	registry := newCLIFixRegistry(t, rules.FixSafe)
	invocation := lintInvocation{
		configPath: configurationPath,
		generateBaseline: ".glippy-baseline.json",
		paths: []string{path},
		reporter: glippyreport.Text,
	}
	if exitCode := runLintGenerateBaseline(
		context.Background(),
		invocation,
		io.Discard,
		io.Discard,
		registry,
	);
		exitCode != ExitSuccess {
		t.Fatalf("runLintGenerateBaseline() exit = %d", exitCode)
	}
	writeConfig(true)
	invocation.generateBaseline = ""
	invocation.fix = true
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := runLintFix(context.Background(), invocation, &stdout, &stderr, registry);
		exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintFix() applied baselined fix: %q", got)
	}
}

func TestRunLintCheckReportsMissingBaselineAsFilesystemFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/sample\n\ngo 1.25\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n" + "[lint.baseline]\npath = \".glippy-baseline.json\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{
			configPath: configurationPath,
			paths: []string{path},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLISyntaxRegistry(t),
	)
	if exitCode != ExitFilesystemError ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "read lint baseline") {
		t.Fatalf(
			"runLintCheck() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintCheckComposesPresetsAndEscalatesWarnings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			"version = 1\n[lint]\npresets = [\"style\", \"pedantic\"]\nwarnings-as-errors = true\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	style, found := newCLISyntaxRegistry(t).Lookup("call-rule")
	if !found {
		t.Fatal("call-rule is not registered")
	}
	metadata := style.Metadata()
	metadata.Presets = []rules.Preset{rules.PresetStyle}
	registry, err := rules.NewRegistry(cliSyntaxRule{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "error[call-rule]") {
		t.Fatalf(
			"runLintCheck() = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintRejectsUnsupportedSourceVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.24\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "source.go"),
		[]byte("package project\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"lint", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSourceError {
		t.Fatalf("Run() exit code = %d, want %d", exitCode, ExitSourceError)
	}
	if stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "supports Go 1.25 through Go 1.26") {
		t.Fatalf("Run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestPrepareLintInputPlansBindsResolvedSourceVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.25.4\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "source.go"),
		[]byte("package project\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npreset = \"suspicious\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	plans, exitCode, err := prepareLintInputPlans(
		context.Background(),
		lintInvocation{paths: []string{root}},
		registry,
	)
	if err != nil || exitCode != ExitSuccess || len(plans) != 1 {
		t.Fatalf("prepareLintInputPlans() = %#v, exit %d, error %v", plans, exitCode, err)
	}
	if plans[0].options.sourceGoVersion != "go1.25" ||
		plans[0].options.analysis.SourceGoVersion != "go1.25" ||
		plans[0].requirement != rules.RequireSSA {
		t.Fatalf(
			"source versions = %q and %q, requirement = %s; want go1.25 and SSA",
			plans[0].options.sourceGoVersion,
			plans[0].options.analysis.SourceGoVersion,
			plans[0].requirement,
		)
	}
}

func TestRunLintCheckUsesOneConfigurationSnapshotForTierAndExecution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[lint.rules]\ncall-rule = \"error\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	ctx := &lintMutationContext{
		Context: context.Background(),
		mutateAt: 3,
		mutate: func() {
			if err := os.WriteFile(
				configurationPath,
				[]byte("version = 1\n[lint.rules]\ncall-rule = \"off\"\n"),
				0o600,
			);
				err != nil {
				t.Error(err)
			}
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		ctx,
		lintInvocation{
			configPath: configurationPath,
			paths: []string{path},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLISyntaxRegistry(t),
	)

	want := path + ":2:12: error[call-rule]: call requires review\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(config snapshot) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintCheckEmitsVersionedJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: []string{path}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		newCLISyntaxRegistry(t),
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("runLintCheck() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.SchemaVersion != 1 ||
		result.Command != "lint" ||
		result.Mode != "check" ||
		result.Outcome.ExitCode != ExitFindings ||
		len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].RuleID != "call-rule" {
		t.Fatalf("lint JSON = %#v", result)
	}
}

func TestRunLintUsesDefaultRegistryForCleanAndSuppressionOutcomes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSyntaxOnlyProductConfig(t, root)
	cleanPath := filepath.Join(root, "clean.go")
	if err := os.WriteFile(cleanPath, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{"lint", cleanPath}, failingReader{}, &stdout, &stderr);
		exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint clean) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := Run([]string{"lint", "--fix", cleanPath}, failingReader{}, &stdout, &stderr);
		exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint --fix clean) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	suppressedPath := filepath.Join(root, "suppressed.go")
	if err := os.WriteFile(
		suppressedPath,
		[]byte("package sample\n//glippy:ignore unknown-rule\nfunc run(){}\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exitCode := Run([]string{"lint", suppressedPath}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), ":2:1: suppression[unknown-rule]:") {
		t.Fatalf(
			"Run(lint suppression) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintReportsLegacyGoxSuppressionMigrationInTextAndJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSyntaxOnlyProductConfig(t, root)
	path := filepath.Join(root, "legacy.go")
	if err := os.WriteFile(
		path,
		[]byte(
			"package sample\n//gox:ignore duplicate-condition -- migrate alias\nfunc run() {}\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{"lint", path}, failingReader{}, &stdout, &stderr);
		exitCode != ExitFindings ||
			stderr.Len() != 0 ||
			!strings.Contains(
				stdout.String(),
				"suppression[legacy-directive]: legacy //gox: suppression; migrate to //glippy:",
			) {
		t.Fatalf(
			"Run(lint legacy text) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode := Run(
		[]string{"lint", "--reporter=json", path},
		failingReader{},
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf("Run(lint legacy JSON) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode legacy JSON: %v; output = %q", err, stdout.String())
	}
	if len(result.SuppressionProblems) != 1 ||
		result.SuppressionProblems[0].Kind != suppressions.ProblemLegacyDirective {
		t.Fatalf("legacy JSON suppression problems = %#v", result.SuppressionProblems)
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
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.Command != "lint" ||
		result.Mode != "invalid" ||
		result.Outcome.ExitCode != ExitInvalidInvocation ||
		result.Summary.Complete {
		t.Fatalf("invalid lint JSON = %#v", result)
	}
}

func TestRunLintPackagePatternKeepsSyntaxOnlyRulesOffPackageLoading(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSyntaxOnlyProductConfig(t, root)
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte(
		"package project\nimport _ \"example.invalid/missing\"\nfunc run(ready bool) { if ready {} else if ready {} }\n",
	)
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

	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "warn[duplicate-condition]") ||
		strings.Contains(stdout.String(), "package[") {
		t.Fatalf(
			"Run(lint package pattern) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintCheckRoutesTypedPackagePatternsToPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
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
			lintInvocation{paths: []string{input}, reporter: glippyreport.Text},
			&stdout,
			&stderr,
			newCLITypesRegistry(t),
		)
		if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf(
				"runLintCheck(typed input %q) = exit %d, stdout %q, stderr %q",
				input,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
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

func TestRunPackageCommandsReuseConfiguredPersistentCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/cachedcli\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("package cachedcli\n\nfunc run() {\n\ttarget()\n}\n\nfunc target() {}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			`version = 1

[cache]
enabled = true
max-entries = 64
max-bytes = 1048576
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "analysis-cache")
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)
	runs := new(atomic.Int64)
	registry := newCLITypesRegistryWithRuns(t, runs)

	var lintOutput bytes.Buffer
	var lintError bytes.Buffer
	if exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&lintOutput,
		&lintError,
		registry,
	);
		exitCode != ExitFindings || lintError.Len() != 0 {
		t.Fatalf(
			"runLintCheck(cold cache) = exit %d, stdout %q, stderr %q",
			exitCode,
			lintOutput.String(),
			lintError.String(),
		)
	}
	coldRuns := runs.Load()
	if coldRuns == 0 {
		t.Fatal("cold package command did not execute the typed rule")
	}

	var checkOutput bytes.Buffer
	var checkError bytes.Buffer
	if exitCode := runCombinedCheck(
		context.Background(),
		checkInvocation{configPath: configurationPath, paths: []string{path}},
		&checkOutput,
		&checkError,
		registry,
	);
		exitCode != ExitFindings || checkError.Len() != 0 {
		t.Fatalf(
			"runCombinedCheck(warm cache) = exit %d, stdout %q, stderr %q",
			exitCode,
			checkOutput.String(),
			checkError.String(),
		)
	}
	if runs.Load() != coldRuns {
		t.Fatalf("warm package command reran typed rule %d times", runs.Load() - coldRuns)
	}
	if lintOutput.String() != checkOutput.String() {
		t.Fatalf(
			"cached command outputs differ: lint %q, check %q",
			lintOutput.String(),
			checkOutput.String(),
		)
	}
	if _, err := os.Stat(cacheRoot); err != nil {
		t.Fatalf("inspect CLI-owned cache root: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("cached package commands mutated source: %q", got)
	}
}

func TestRunPackageCommandsUseOneConfiguredBuildSelection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/buildselection\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "selected.go")
	input := []byte(
		`//go:build selected && linux && cgo

package buildselection

func run() {
	target()
}

func target() {}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	configuration := `version = 1

[analysis]
build-tags = ["selected"]
goos = "linux"
goarch = "amd64"
cgo-enabled = true
`
	if err := os.WriteFile(configurationPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "analysis-cache")
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOARCH", "arm64")
	t.Setenv("CGO_ENABLED", "0")
	runs := new(atomic.Int64)
	registry := newCLITypesRegistryWithRuns(t, runs)
	want := path + ":6:2: warn[typed-call]: typed call requires review\n"

	var uncachedOutput bytes.Buffer
	var uncachedError bytes.Buffer
	if exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{root}},
		&uncachedOutput,
		&uncachedError,
		registry,
	);
		exitCode != ExitFindings ||
			uncachedOutput.String() != want ||
			uncachedError.Len() != 0 {
		t.Fatalf(
			"runLintCheck(configured build selection) = exit %d, stdout %q, stderr %q",
			exitCode,
			uncachedOutput.String(),
			uncachedError.String(),
		)
	}

	configuration += `
[cache]
enabled = true
max-entries = 64
max-bytes = 1048576
`
	if err := os.WriteFile(configurationPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	var coldOutput bytes.Buffer
	var coldError bytes.Buffer
	if exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{root}},
		&coldOutput,
		&coldError,
		registry,
	);
		exitCode != ExitFindings || coldOutput.String() != want || coldError.Len() != 0 {
		t.Fatalf(
			"runLintCheck(cached build selection) = exit %d, stdout %q, stderr %q",
			exitCode,
			coldOutput.String(),
			coldError.String(),
		)
	}
	coldRuns := runs.Load()
	var warmOutput bytes.Buffer
	var warmError bytes.Buffer
	if exitCode := runCombinedCheck(
		context.Background(),
		checkInvocation{configPath: configurationPath, paths: []string{root}},
		&warmOutput,
		&warmError,
		registry,
	);
		exitCode != ExitFindings || warmOutput.String() != want || warmError.Len() != 0 {
		t.Fatalf(
			"runCombinedCheck(cached build selection) = exit %d, stdout %q, stderr %q",
			exitCode,
			warmOutput.String(),
			warmError.String(),
		)
	}
	if runs.Load() != coldRuns {
		t.Fatalf(
			"warm configured build selection reran typed rule %d times",
			runs.Load() - coldRuns,
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("configured package commands mutated source: %q", got)
	}
}

func TestResolvedCacheToolIdentityBindsDevelopmentBinary(t *testing.T) {
	first, err := resolvedCacheToolIdentity("devel", strings.NewReader("first binary"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolvedCacheToolIdentity("devel", strings.NewReader("second binary"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second ||
		!strings.HasPrefix(first, "devel-sha256:") ||
		!strings.HasPrefix(second, "devel-sha256:") {
		t.Fatalf("development cache identities = %q and %q", first, second)
	}
	release, err := resolvedCacheToolIdentity("v1.2.3", nil)
	if err != nil || release != "v1.2.3" {
		t.Fatalf("release cache identity = %q, %v", release, err)
	}
}

func TestRunPackageCommandPrunesConfiguredPersistentCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/prunedcli\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(
		path,
		[]byte("package prunedcli\n\nfunc run() {\n\ttarget()\n}\n\nfunc target() {}\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			`version = 1

[cache]
enabled = true
max-entries = 1
max-bytes = 0
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "analysis-cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	for index := byte(1); index <= 3; index++ {
		key := cache.Key{index}
		if err := store.Put(context.Background(), key, []byte{index}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&stdout,
		&stderr,
		newCLITypesRegistry(t),
	);
		exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(prune cache) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	store, err = cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	pruned, pruneErr := store.Prune(context.Background(), cache.PruneOptions{MaxEntries: 100})
	closeErr := store.Close()
	if pruneErr != nil {
		t.Fatal(pruneErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if pruned.EntriesBefore != 1 || pruned.EntriesRemoved != 0 {
		t.Fatalf("CLI cache prune result = %#v", pruned)
	}
}

func TestRunPackageCommandRemovesStaleCacheTemporaryEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/staletemporary\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package staletemporary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[cache]\nenabled = true\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "analysis-cache")
	key := cache.Key{1}
	temporary := filepath.Join(
		cacheRoot,
		"v1",
		key.String()[:2],
		"." + key.String() + ".0000000000000001.tmp",
	)
	if err := os.MkdirAll(filepath.Dir(temporary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("abandoned"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(temporary, stale, stale); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := runLintCheck(
		context.Background(),
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&stdout,
		&stderr,
		newCLITypesRegistry(t),
	);
		exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(stale cache temporary) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
		t.Fatalf("stale cache temporary remains: %v", err)
	}
}

func TestRunPackageCommandSkipsCachePruningAfterCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/canceledcache\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(
		path,
		[]byte("package canceledcache\n\nfunc run() { target() }\nfunc target() {}\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			`version = 1

[cache]
enabled = true
max-entries = 1
max-bytes = 0
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "analysis-cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	for index := byte(1); index <= 3; index++ {
		if err := store.Put(context.Background(), cache.Key{index}, []byte{index});
			err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)
	ctx, cancel := context.WithCancel(context.Background())
	registry := newCLITypesRegistryWithHooks(t, nil, cancel)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := runLintCheck(
		ctx,
		lintInvocation{configPath: configurationPath, paths: []string{path}},
		&stdout,
		&stderr,
		registry,
	);
		exitCode != ExitCanceled || stdout.Len() != 0 {
		t.Fatalf(
			"runLintCheck(canceled cache) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	store, err = cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	pruned, pruneErr := store.Prune(context.Background(), cache.PruneOptions{MaxEntries: 100})
	closeErr := store.Close()
	if pruneErr != nil {
		t.Fatal(pruneErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if pruned.EntriesBefore != 3 || pruned.EntriesRemoved != 0 {
		t.Fatalf("canceled CLI cache was pruned: %#v", pruned)
	}
}

func TestRunSyntaxCommandsRemainIndependentOfConfiguredPersistentCache(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package syntaxcache\n\nfunc run() { target() }\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte(
			`version = 1

[analysis]
build-tags = ["syntax_only"]
goos = "plan9"
goarch = "amd64"
cgo-enabled = false

[cache]
enabled = true
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(root, ".cache", "analysis")
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)
	registry := newCLISyntaxRegistry(t)

	for _, command := range []string{"lint", "check", "fix"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		var exitCode int
		switch command {
		case "lint":
			exitCode = runLintCheck(
				context.Background(),
				lintInvocation{
					configPath: configurationPath,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		case "check":
			exitCode = runCombinedCheck(
				context.Background(),
				checkInvocation{
					configPath: configurationPath,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		case "fix":
			exitCode = runLintFix(
				context.Background(),
				lintInvocation{
					configPath: configurationPath,
					fix: true,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		}
		if exitCode != ExitFindings || stderr.Len() != 0 {
			t.Fatalf(
				"run syntax %s with cache configured = exit %d, stdout %q, stderr %q",
				command,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
	if _, err := os.Lstat(cacheRoot); !os.IsNotExist(err) {
		t.Fatalf("syntax command created cache root: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("syntax commands with cache configured mutated source: %q", got)
	}
}

func TestRunPackageCommandRefusesCacheInsideProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/containedcache\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package containedcache\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[cache]\nenabled = true\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(root, ".cache", "analysis")
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)

	registry := newCLITypesRegistry(t)
	for _, command := range []string{"lint", "check"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		var exitCode int
		if command == "lint" {
			exitCode = runLintCheck(
				context.Background(),
				lintInvocation{
					configPath: configurationPath,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		} else {
			exitCode = runCombinedCheck(
				context.Background(),
				checkInvocation{
					configPath: configurationPath,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		}
		if exitCode != ExitInvalidInvocation ||
			stdout.Len() != 0 ||
			!strings.Contains(
				stderr.String(),
				"analysis cache root must remain outside project root",
			) {
			t.Fatalf(
				"run%sCheck(contained cache) = exit %d, stdout %q, stderr %q",
				command,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
	if _, err := os.Lstat(cacheRoot); !os.IsNotExist(err) {
		t.Fatalf("contained cache root was created: %v", err)
	}
}

func TestRunPackageCommandReportsCacheOpenFailureAsFilesystemError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/cacheopenfailure\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package cacheopenfailure\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[cache]\nenabled = true\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(cacheRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLIPPY_CACHE_DIR", cacheRoot)

	registry := newCLITypesRegistry(t)
	for _, command := range []string{"lint", "check"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		var exitCode int
		if command == "lint" {
			exitCode = runLintCheck(
				context.Background(),
				lintInvocation{
					configPath: configurationPath,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		} else {
			exitCode = runCombinedCheck(
				context.Background(),
				checkInvocation{
					configPath: configurationPath,
					paths: []string{path},
				},
				&stdout,
				&stderr,
				registry,
			)
		}
		if exitCode != ExitFilesystemError ||
			stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), "open analysis cache") {
			t.Fatalf(
				"run%sCheck(cache open failure) = exit %d, stdout %q, stderr %q",
				command,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunLintCheckRoutesControlFlowRulesThroughPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
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
		lintInvocation{paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLIControlFlowRegistry(t),
	)
	want := path + ":2:1: warn[cfg-function]: function requires control-flow review\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(CFG) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
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
		lintInvocation{paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLISSARegistry(t),
	)
	want := path + ":2:1: warn[ssa-function]: function requires SSA review\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(SSA) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "source.go"),
		[]byte("package project\nfunc run() { _ = missing; target() }\nfunc target() {}\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	registry := newCLITypesRegistry(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.JSON,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(typed error) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode typed lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError ||
		result.Outcome.Category != "source_error" ||
		!result.Summary.Complete ||
		result.Summary.PackageDiagnostics == 0 ||
		len(result.PackageDiagnostics) == 0 ||
		len(result.Errors) != 0 {
		t.Fatalf("typed source-error JSON = %#v", result)
	}
}

func TestRunLintCheckReportsTypedSourceModelFailuresAsSourceErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "source.go"),
		[]byte("package project\nfunc broken( {\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	registry := newCLITypesRegistry(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.JSON,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf(
			"runLintCheck(typed source problem) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode typed source-problem JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError ||
		!result.Summary.Complete ||
		result.Summary.SourceProblems == 0 ||
		len(result.SourceProblems) == 0 ||
		result.SourceProblems[0].Path != filepath.Join(root, "source.go") {
		t.Fatalf("typed source-problem JSON = %#v", result)
	}
}

func TestRunLintCheckRejectsHeterogeneousTypedPackageRoots(t *testing.T) {
	t.Parallel()

	paths := make([]string, 0, 2)
	for _, module := range []string{"one", "two"} {
		root := t.TempDir()
		if err := os.WriteFile(
			filepath.Join(root, "go.mod"),
			[]byte("module example.com/" + module + "\n\ngo 1.26.0\n"),
			0o600,
		);
			err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(root, "source.go"),
			[]byte("package " + module + "\n"),
			0o600,
		);
			err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Join(root, "..."))
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintCheck(
		context.Background(),
		lintInvocation{paths: paths, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLITypesRegistry(t),
	)

	if exitCode != ExitInvalidInvocation ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "one project root and configuration") {
		t.Fatalf(
			"runLintCheck(heterogeneous roots) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLintFixAppliesFormatsAndReanalyzesOneSafeTypedFix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte(
		"package project\nfunc run() { target() }\nfunc target() {}\nfunc primary() {}\n",
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix: true,
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLITypesFixRegistry(t, "primary"),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix(typed) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	want := "package project\n\nfunc run() {\n\tprimary()\n}\n\nfunc target() {}\n\nfunc primary() {}\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(typed) source = %q, error = %v", got, err)
	}
}

func TestRunLintFixUsesConfiguredBuildSelectionForPlanningAndValidation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/buildselectionfix\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "selected.go")
	input := []byte(
		`//go:build selected && linux && cgo

package buildselectionfix

func run() { target() }
func target() {}
func primary() {}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, ".glippy.toml")
	configuration := `version = 1

[analysis]
build-tags = ["selected"]
goos = "linux"
goarch = "amd64"
cgo-enabled = true
`
	if err := os.WriteFile(configurationPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			configPath: configurationPath,
			fix: true,
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLITypesFixRegistry(t, "primary"),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix(configured build selection) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	want := `//go:build selected && linux && cgo

package buildselectionfix

func run() {
	primary()
}

func target() {}

func primary() {}
`
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(configured build selection) source = %q, error = %v", got, err)
	}
}

func TestRunLintFixRejectsTypedFixThatFailsPackageValidation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
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
		lintInvocation{
			fix: true,
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLITypesFixRegistry(t, "missing"),
	)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "rejected fix[typed-fix/rewrite/validation]") ||
		!strings.Contains(stdout.String(), "undefined: missing") ||
		!bytes.Equal(got, input) {
		t.Fatalf(
			"runLintFix(invalid typed fix) = exit %d, stdout %q, stderr %q, source %q",
			exitCode,
			stdout.String(),
			stderr.String(),
			got,
		)
	}
}

func TestRunLintFixReselectsTypedFixesAfterEachPackageWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	if err := os.WriteFile(
		firstPath,
		[]byte("package project\nconst Gate = true\nfunc target() {}\nfunc primary() {}\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "b.go")
	secondInput := []byte("package project\nfunc run() { target() }\n")
	if err := os.WriteFile(secondPath, secondInput, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix: true,
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLIPackageStateFixRegistry(t),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix(package state) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first, []byte("const Gate = false")) {
		t.Fatalf("runLintFix(package state) first source = %q", first)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, secondInput) {
		t.Fatalf("runLintFix(package state) applied stale second selection: %q", second)
	}
}

func TestRunLintFixReportsFindingEnabledInEarlierFileByLaterWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "a.go")
	firstInput := []byte("package project\nfunc run() { target() }\nfunc target() {}\n")
	if err := os.WriteFile(firstPath, firstInput, 0o600); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "b.go")
	if err := os.WriteFile(secondPath, []byte("package project\nconst Gate = false\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix: true,
			paths: []string{filepath.Join(root, "...")},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLIPackageEnablingFixRegistry(t),
	)

	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(
			stdout.String(),
			"warn[package-enabling-fix]: enabled package state requires review",
		) {
		t.Fatalf(
			"runLintFix(enabled earlier finding) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, firstInput) {
		t.Fatalf("runLintFix(enabled earlier finding) changed first source: %q", first)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(second, []byte("const Gate = true")) {
		t.Fatalf("runLintFix(enabled earlier finding) second source = %q", second)
	}
}

func TestRunLintFixRejectsTypedSourceThroughSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	realDirectory := filepath.Join(root, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realDirectory, "source.go")
	input := []byte(
		"package real\nfunc run() { target() }\nfunc target() {}\nfunc primary() {}\n",
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	linkedDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix: true,
			paths: []string{filepath.Join(linkedDirectory, "source.go")},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLITypesFixRegistry(t, "primary"),
	)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != ExitFilesystemError ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "symlink") ||
		!bytes.Equal(got, input) {
		t.Fatalf(
			"runLintFix(typed symlink) = exit %d, stdout %q, stderr %q, source %q",
			exitCode,
			stdout.String(),
			stderr.String(),
			got,
		)
	}
}

func TestPrepareLintPackageSnapshotRejectsBytesChangedAfterPackageAnalysis(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package project\nfunc target() {}\n")
	file, err := source.Load(path, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package project\nfunc changed() {}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}

	snapshot, exitCode, err := prepareLintPackageSnapshot(context.Background(), root, file)

	if snapshot != nil || exitCode != ExitConflict || !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf(
			"prepareLintPackageSnapshot(stale) = snapshot %#v, exit %d, error %v",
			snapshot,
			exitCode,
			err,
		)
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
	if err := os.WriteFile(invalidPath, []byte("package sample\nfunc broken( {\n"), 0o600);
		err != nil {
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
		lintInvocation{
			paths: []string{invalidPath, validPath},
			reporter: glippyreport.JSON,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("runLintCheck() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode incomplete lint JSON: %v; output = %q", err, stdout.String())
	}
	if result.Outcome.ExitCode != ExitSourceError ||
		result.Summary.Complete ||
		result.Summary.Files != 1 ||
		len(result.Files) != 1 ||
		result.Files[0].Path != validPath ||
		len(result.Errors) != 1 {
		t.Fatalf("incomplete lint JSON = %#v", result)
	}
}

func TestRunLintCheckTreatsSuppressedOnlyDiagnosticsAsSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "suppressed.go")
	input := []byte(
		"package sample\n//glippy:ignore call-rule -- accepted here\nfunc run(){target()}\n",
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := newCLISyntaxRegistry(t)

	for _, reporter := range []glippyreport.Format{glippyreport.Text, glippyreport.JSON} {
		reporter := reporter
		t.Run(
			string(reporter),
			func(t *testing.T) {
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
					t.Fatalf(
						"runLintCheck() exit = %d, stderr = %q",
						exitCode,
						stderr.String(),
					)
				}
				if reporter == glippyreport.Text {
					if stdout.Len() != 0 {
						t.Fatalf(
							"runLintCheck() stdout = %q, want empty",
							stdout.String(),
						)
					}
					return
				}
				var result glippyreport.LintResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf(
						"decode suppressed lint JSON: %v; output = %q",
						err,
						stdout.String(),
					)
				}
				if result.Outcome.ExitCode != ExitSuccess ||
					!result.Summary.Complete ||
					result.Summary.Suppressed != 1 ||
					result.Summary.Diagnostics != 0 {
					t.Fatalf("suppressed lint JSON = %#v", result)
				}
			},
		)
	}
}

func TestPrepareLintTasksBindsOneConfigurationSnapshotToEverySelectedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configurationPath := filepath.Join(root, ".glippy.toml")
	writeConfiguration := func(severity string) {
		t.Helper()
		if err := os.WriteFile(
			configurationPath,
			[]byte("version = 1\n[lint.rules]\ncall-rule = \"" + severity + "\"\n"),
			0o600,
		);
			err != nil {
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
		Context: context.Background(),
		mutateAt: 6,
		mutate: func() {
			writeConfiguration("error")
		},
	}

	tasks, exitCode, err := prepareLintTasks(
		ctx,
		lintInvocation{
			configPath: configurationPath,
			paths: []string{firstPath, secondPath},
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
			t.Fatalf(
				"task %q severity = %q, want one bound warn snapshot",
				task.file.Path,
				got,
			)
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
		lintInvocation{paths: []string{path}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitSourceError || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(invalid) = exit %d, stderr %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode source-error JSON: %v", err)
	}
	if result.Outcome.ExitCode != ExitSourceError ||
		result.Summary.Complete ||
		len(result.Errors) != 1 {
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
		lintInvocation{paths: []string{path}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitCanceled || stderr.Len() != 0 {
		t.Fatalf("runLintCheck(canceled) = exit %d, stderr %q", exitCode, stderr.String())
	}
	result = glippyreport.LintResult{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode canceled JSON: %v", err)
	}
	if result.Outcome.ExitCode != ExitCanceled ||
		result.Summary.Complete ||
		len(result.Errors) != 1 {
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
		lintInvocation{paths: []string{"."}, reporter: glippyreport.JSON},
		failingWriter{},
		&stderr,
		registry,
	)

	if exitCode != ExitCanceled {
		t.Fatalf(
			"runLintCheck() exit = %d, want %d; stderr = %q",
			exitCode,
			ExitCanceled,
			stderr.String(),
		)
	}
	if !strings.Contains(stderr.String(), "write JSON report") {
		t.Fatalf("runLintCheck() stderr = %q, want JSON output failure", stderr.String())
	}
}

func TestRunLintFixAppliesAndFormatsOneSafeFix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o640);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix() exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSuggestion),
	)

	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(
			stdout.String(),
			"warn[fix-rule]: target call requires replacement",
		) ||
		!strings.Contains(stdout.String(), "fix[suggestion]: rewrite") {
		t.Fatalf(
			"runLintFix() exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fixSuggestions: true,
			paths: []string{path},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSuggestion),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix(suggestion) exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n}\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(suggestion) source = %q, error = %v", got, err)
	}
}

func TestRunLintFixLeavesBuiltInIneffectiveBreakSuggestionUnchanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte("package sample\nfunc run(){for{select{default:break}}}\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "warn[ineffective-break]") ||
		!strings.Contains(stdout.String(), "fix[suggestion]: remove-break") {
		t.Fatalf(
			"runLintFix() exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("runLintFix() changed suggestion-only source: %q", got)
	}
}

func TestRunLintFixAppliesBuiltInIneffectiveBreakSuggestion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	input := []byte(
		`package sample
func run() {
	for {
		select {
		default:
			break // the select already ends here
		}
	}
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fixSuggestions: true,
			paths: []string{path},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix(suggestion) exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	want := "package sample\n\nfunc run() {\n\tfor {\n\t\tselect {\n\t\tdefault:\n\t\t// the select already ends here\n\t\t}\n\t}\n}\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(suggestion) source = %q, error = %v", got, err)
	}
}

func TestRunLintFixAppliesOnlyExplicitUnsafeFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fixUnsafe: true, paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixUnsafe),
	)

	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runLintFix(unsafe) exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n}\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("runLintFix(unsafe) source = %q, error = %v", got, err)
	}
}

func TestParseLintInvocationAcceptsIndependentFixModes(t *testing.T) {
	t.Parallel()

	invocation, valid := parseLintInvocation(
		[]string{"lint", "--fix", "--fix-suggestions", "--fix-unsafe", "source.go"},
	)

	if !valid ||
		!invocation.fix ||
		!invocation.fixSuggestions ||
		!invocation.fixUnsafe ||
		len(invocation.paths) != 1 ||
		invocation.paths[0] != "source.go" {
		t.Fatalf("parseLintInvocation() = %#v, %t", invocation, valid)
	}
}

func TestRunLintExplicitFixModesUseFixReporter(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--fix-suggestions", "--fix-unsafe"} {
		flag := flag
		t.Run(
			flag,
			func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				writeSyntaxOnlyProductConfig(t, root)
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
					t.Fatalf(
						"Run(lint %s) exit = %d, stderr = %q",
						flag,
						exitCode,
						stderr.String(),
					)
				}
				var result glippyreport.LintResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf(
						"decode lint %s JSON: %v; output = %q",
						flag,
						err,
						stdout.String(),
					)
				}
				if result.Mode != "fix" ||
					!result.Summary.Complete ||
					result.Outcome.ExitCode != ExitSuccess {
					t.Fatalf("Run(lint %s) result = %#v", flag, result)
				}
				if got, err := os.ReadFile(path);
					err != nil || !bytes.Equal(got, input) {
					t.Fatalf(
						"Run(lint %s) source = %q, error = %v",
						flag,
						got,
						err,
					)
				}
			},
		)
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
		cliFixRule: newCLIFixRuleFor(
			"second-fix",
			"secondTarget",
			"secondary",
			rules.FixSafe,
		),
		secondSafety: rules.FixSuggestion,
	}
	secondRule.metadata.Fixes = append(
		secondRule.metadata.Fixes,
		rules.FixMetadata{
			Name: "alternative",
			Description: "replace the target call with an alternative",
			Safety: rules.FixSuggestion,
		},
	)
	registry, err := rules.NewRegistry(firstRule, secondRule)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix: true,
			fixSuggestions: true,
			paths: []string{firstPath, secondPath},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitInternalError ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "multiple authorized fixes") {
		t.Fatalf(
			"runLintFix(ambiguous) exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitConflict || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Mode != "fix" ||
		result.Outcome.ExitCode != ExitConflict ||
		!result.Summary.Complete ||
		result.Summary.RejectedFixes != 2 ||
		len(result.Files) != 1 ||
		result.Files[0].Status != glippyreport.LintFileConflict ||
		len(result.RejectedFixes) != 2 {
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
	rule := cliPostFixFailureRule{
		cliFixRule: newCLIFixRule("fix-rule", "primary", rules.FixSafe),
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitInternalError || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode failed lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Summary.Complete ||
		result.Outcome.ExitCode != ExitInternalError ||
		len(result.Errors) != 1 ||
		!strings.Contains(result.Errors[0].Message, "post-fix analysis failed") {
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
	if err := os.WriteFile(
		path,
		[]byte("package sample\nfunc run(){target();other()}\n"),
		0o600,
	);
		err != nil {
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
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.Text},
		failingWriter{},
		&stderr,
		registry,
	)

	if exitCode != ExitFilesystemError ||
		!strings.Contains(stderr.String(), "files fixed before failure") ||
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
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.JSON},
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
	if err := os.WriteFile(path, []byte("package sample\nfunc run(){target()}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Mode != "fix" ||
		result.Outcome.ExitCode != ExitSuccess ||
		!result.Summary.Complete ||
		result.Summary.FixedFiles != 1 ||
		result.Summary.AppliedFixes != 1 ||
		len(result.Files) != 1 ||
		result.Files[0].Status != glippyreport.LintFileFixed ||
		result.Files[0].SourceDigest == result.Files[0].ResultDigest ||
		len(result.AppliedFixes) != 1 {
		t.Fatalf("lint fix JSON = %#v", result)
	}
}

func TestRunLintFixPrevalidatesEverySourceBeforeWriting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n"),
		0o600,
	);
		err != nil {
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
		[]byte(
			"// Code generated by fixture. DO NOT EDIT.\npackage sample\nfunc generated(){target()}\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		context.Background(),
		lintInvocation{
			fix: true,
			paths: []string{generatedPath, firstPath},
			reporter: glippyreport.Text,
		},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitFilesystemError ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "refusing to fix generated file") {
		t.Fatalf(
			"runLintFix() exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n"),
		0o600,
	);
		err != nil {
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
		lintInvocation{fix: true, paths: []string{path}, reporter: glippyreport.Text},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitFilesystemError ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "refusing to fix symlink") {
		t.Fatalf(
			"runLintFix() exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
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
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n"),
		0o600,
	);
		err != nil {
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
		cancel: cancel,
		path: firstPath,
		needle: "primary()",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runLintFix(
		ctx,
		lintInvocation{fix: true, paths: []string{root}, reporter: glippyreport.JSON},
		&stdout,
		&stderr,
		newCLIFixRegistry(t, rules.FixSafe),
	)

	if exitCode != ExitCanceled || stderr.Len() != 0 {
		t.Fatalf("runLintFix() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode canceled lint fix JSON: %v; output = %q", err, stdout.String())
	}
	if result.Summary.Complete ||
		len(result.Files) != 2 ||
		result.Files[0].Status != glippyreport.LintFileFixed ||
		result.Files[1].Status != glippyreport.LintFilePending {
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

	before, err := source.Load(
		"/project/source.go",
		[]byte("package sample\nfunc run(){target()}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	after, err := source.Load(
		"/project/source.go",
		[]byte("package sample\n\nfunc run() {\n\tprimary()\n}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	execution := lintFixExecution{
		file: before,
		resultFile: before,
		result: analysis.Result{Path: before.Path(), Digest: before.Digest()},
		outcome: glippyreport.LintFixOutcome{
			Path: before.Path(),
			SourceDigest: before.Digest(),
			Status: glippyreport.LintFilePending,
		},
	}
	transaction := fixengine.Transaction{
		Result: fixengine.Result{
			Applied: []fixengine.Applied{
				{
					RuleID: "fix-rule",
					FixName: "rewrite",
					Range: source.Range{Start: 26, End: 34},
				},
			},
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

	if execution.result.Digest != before.Digest() ||
		execution.resultFile != before ||
		execution.outcome.Status != glippyreport.LintFileConflict ||
		len(execution.outcome.Applied) != 1 ||
		len(execution.outcome.Rejected) != 1 ||
		execution.outcome.Rejected[0].Reason != fixengine.RejectionStaleSource ||
		lintFixExitCode([]lintFixExecution{execution}) != ExitConflict {
		t.Fatalf("stale lint fix execution = %#v", execution)
	}
}

func TestLintFixFileStatusPreservesReplacementCertainty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		transaction fixengine.Transaction
		want glippyreport.LintFileStatus
	}{
		{
			name: "completed",
			transaction: fixengine.Transaction{Status: fixengine.WriteCompleted},
			want: glippyreport.LintFileFixed,
		},
		{
			name: "possible",
			transaction: fixengine.Transaction{
				Status: fixengine.WritePossiblyCompleted,
			},
			want: glippyreport.LintFilePossiblyFixed,
		},
		{
			name: "conflict",
			transaction: fixengine.Transaction{
				Result: fixengine.Result{
					Rejected: []fixengine.Rejection{
						{Reason: fixengine.RejectionConflict},
					},
				},
			},
			want: glippyreport.LintFileConflict,
		},
		{
			name: "unchanged",
			transaction: fixengine.Transaction{Status: fixengine.WriteNotPerformed},
			want: glippyreport.LintFileUnchanged,
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				if got := lintFixFileStatus(test.transaction); got != test.want {
					t.Fatalf(
						"lintFixFileStatus() = %q, want %q",
						got,
						test.want,
					)
				}
			},
		)
	}
}

func TestReportLintFixJSONDisclosesCompletedWritesWhenResultConstructionFails(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	executions := []lintFixExecution{
		{
			file: file,
			resultFile: file,
			result: analysis.Result{Path: "/project/other.go", Digest: file.Digest()},
			outcome: glippyreport.LintFixOutcome{
				Path: file.Path(),
				SourceDigest: file.Digest(),
				Status: glippyreport.LintFileFixed,
			},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := reportLintFixJSON(&stdout, &stderr, ExitSuccess, true, executions, nil)

	if exitCode != ExitInternalError ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "files fixed before reporting failure") ||
		!strings.Contains(stderr.String(), file.Path()) {
		t.Fatalf(
			"reportLintFixJSON() exit = %d, stdout = %q, stderr = %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func newCLISyntaxRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		cliSyntaxRule{
			metadata: rules.Metadata{
				ID: "call-rule",
				Summary: "reports calls",
				Documentation: "Reports calls that require review.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSyntax,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{
					{Incorrect: "target()", Correct: "reviewed()"},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLITypesRegistry(t *testing.T) *rules.Registry {
	return newCLITypesRegistryWithRuns(t, nil)
}

func newCLITypesFixRegistry(t *testing.T, replacement string) *rules.Registry {
	t.Helper()
	rule := cliTypesFixRule{
		target: "target",
		replacement: replacement,
		metadata: rules.Metadata{
			ID: "typed-fix",
			Summary: "replaces typed target calls",
			Documentation: "Replaces typed target calls with an admitted alternative.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.22",
			Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
			Categories: []rules.Category{rules.CategoryCorrectness},
			Fixes: []rules.FixMetadata{
				{
					Name: "rewrite",
					Description: "replace the typed target call",
					Safety: rules.FixSafe,
				},
			},
			Examples: []rules.Example{
				{Incorrect: "target()", Correct: replacement + "()"},
			},
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLIPackageStateFixRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		cliPackageStateFixRule{
			metadata: rules.Metadata{
				ID: "package-state-fix",
				Summary: "rewrites package state",
				Documentation: "Rewrites calls while the package gate is enabled.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireTypes,
				NodeInterests: []rules.NodeKind{
					rules.NodeIdent,
					rules.NodeCallExpr,
				},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Fixes: []rules.FixMetadata{
					{
						Name: "rewrite",
						Description: "rewrite while the package gate is enabled",
						Safety: rules.FixSafe,
					},
				},
				Examples: []rules.Example{
					{
						Incorrect: "const Gate = true",
						Correct: "const Gate = false",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLIPackageEnablingFixRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		cliPackageEnablingFixRule{
			metadata: rules.Metadata{
				ID: "package-enabling-fix",
				Summary: "enables package findings",
				Documentation: "Enables a package state that requires another source review.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireTypes,
				NodeInterests: []rules.NodeKind{
					rules.NodeIdent,
					rules.NodeCallExpr,
				},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Fixes: []rules.FixMetadata{
					{
						Name: "enable",
						Description: "enable the package gate",
						Safety: rules.FixSafe,
					},
				},
				Examples: []rules.Example{
					{
						Incorrect: "const Gate = false",
						Correct: "const Gate = true",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLITypesRegistryWithRuns(t *testing.T, runs *atomic.Int64) *rules.Registry {
	return newCLITypesRegistryWithHooks(t, runs, nil)
}

func newCLITypesRegistryWithHooks(
	t *testing.T,
	runs *atomic.Int64,
	cancel context.CancelFunc,
) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		cliTypesRule{
			metadata: rules.Metadata{
				ID: "typed-call",
				Summary: "reports typed calls",
				Documentation: "Reports calls that require typed review.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireTypes,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{
					{Incorrect: "target()", Correct: "reviewed()"},
				},
			},
			runs: runs,
			cancel: cancel,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLIControlFlowRegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		cliControlFlowRule{
			metadata: rules.Metadata{
				ID: "cfg-function",
				Summary: "reports functions",
				Documentation: "Reports functions that require control-flow review.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireControlFlow,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{
					{Incorrect: "func bad() {}", Correct: "func good() {}"},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func newCLISSARegistry(t *testing.T) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		cliSSARule{
			metadata: rules.Metadata{
				ID: "ssa-function",
				Summary: "reports SSA functions",
				Documentation: "Reports functions that require SSA review.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSSA,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{
					{Incorrect: "func bad() {}", Correct: "func good() {}"},
				},
			},
		},
	)
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
		target: target,
		replacement: replacement,
		metadata: rules.Metadata{
			ID: ruleID,
			Summary: "replaces target calls",
			Documentation: "Replaces target calls with an admitted alternative.",
			DefaultSeverity: rules.SeverityWarn,
			Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.22",
			Requirement: rules.RequireSyntax,
			NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
			Categories: []rules.Category{rules.CategoryCorrectness},
			Fixes: []rules.FixMetadata{
				{
					Name: "rewrite",
					Description: "replace the target call",
					Safety: safety,
				},
			},
			Examples: []rules.Example{
				{Incorrect: "target()", Correct: replacement + "()"},
			},
		},
	}
}

func TestRunExposesStandardSelfAssignmentAnalyzer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/selfassignment\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`package sample

func reset(value int) int {
    value = value
    return value
}

func replace(value, replacement int) int {
    value = replacement
    return value
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npreset = \"correctness\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	want := path +
		":4:5: warn[self-assignment]: self-assignment of value\n" +
		"  help: https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/assign\n" +
		"  fix[suggestion]: remove-self-assignment\n"
	if exitCode != ExitFindings || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint self-assignment) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Run(lint self-assignment) mutated source: %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"explain", "self-assignment"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	for _, contract := range
		[]string{
			"self-assignment\n",
			"presets: correctness\n",
			"analysis tier: types\n",
			"generated files: excluded\n",
			"type-error packages: excluded\n",
			"remove-self-assignment [suggestion]",
		} {
		if !strings.Contains(stdout.String(), contract) {
			t.Fatalf(
				"Run(explain self-assignment) output does not contain %q:\n%s",
				contract,
				stdout.String(),
			)
		}
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"Run(explain self-assignment) = exit %d, stderr %q",
			exitCode,
			stderr.String(),
		)
	}
}

func TestRunAppliesStandardSelfAssignmentSuggestionOnlyWhenRequested(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/selfassignmentfix\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		path,
		[]byte(
			"package sample\n\nfunc reset(value int) int {\n\tvalue = value\n\treturn value\n}\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npreset = \"correctness\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint --fix-suggestions self-assignment) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	want := "package sample\n\nfunc reset(value int) int {\n\treturn value\n}\n"
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("fixed self-assignment source = %q, error = %v", got, err)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != ExitSuccess ||
		stdout.Len() != 0 ||
		stderr.Len() != 0 ||
		!bytes.Equal(second, got) {
		t.Fatalf(
			"second self-assignment fix = exit %d, stdout %q, stderr %q, source %q",
			exitCode,
			stdout.String(),
			stderr.String(),
			second,
		)
	}
}

func TestRunBaselinesStandardSelfAssignmentDeterministically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/selfassignmentbaseline\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		path,
		[]byte(
			"package sample\nfunc reset(value int) int { value = value; return value }\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configPath,
		[]byte("version = 1\n[lint]\npreset = \"correctness\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(root, ".glippy-baseline.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess ||
		stdout.String() !=
			"glippy lint: wrote baseline " + baselinePath + " (1 diagnostics)\n" ||
		stderr.Len() != 0 {
		t.Fatalf(
			"Run(generate self-assignment baseline) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	baselineBytes, err := os.ReadFile(baselinePath)
	if err != nil || !bytes.Contains(baselineBytes, []byte(`"rule_id": "self-assignment"`)) {
		t.Fatalf("self-assignment baseline = %q, error = %v", baselineBytes, err)
	}
	if err := os.WriteFile(
		configPath,
		[]byte(
			"version = 1\n[lint]\npreset = \"correctness\"\n" +
				"[lint.baseline]\npath = \".glippy-baseline.json\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint baselined self-assignment) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunBaselinesCorrectnessAnalyzerCatalogDeterministically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/standardcatalogbaseline\n\ngo 1.26.0\n"),
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

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
)

type state struct { lock sync.Mutex }
type reader interface { Read() }
type writer interface { Read(int) }

func copied(value *state) state { return *value }
func asserted(value reader) { _ = value.(writer) }
func canceled(parent context.Context) context.Context {
	child, _ := context.WithCancel(parent)
	return child
}
func fetched() error {
	response, err := http.Get("https://example.test")
	defer response.Body.Close()
	if err != nil { return err }
	return nil
}
func incremented(value *uint64) { *value = atomic.AddUint64(value, 1) }
func captured(values []int) {
	var value int
	for _, value = range values { go func() { _ = value }() }
}
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, ".glippy.toml")
	if err := os.WriteFile(
		configPath,
		[]byte("version = 1\n[lint]\npreset = \"correctness\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	baselinePath := filepath.Join(root, ".glippy-baseline.json")
	generate := func() []byte {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{"lint", "--generate-baseline=.glippy-baseline.json", path},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
		if exitCode != ExitSuccess ||
			stdout.String() !=
				"glippy lint: wrote baseline " +
					baselinePath +
					" (6 diagnostics)\n" ||
			stderr.Len() != 0 {
			t.Fatalf(
				"Run(generate correctness catalog baseline) = exit %d, stdout %q, stderr %q",
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
		encoded, err := os.ReadFile(baselinePath)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	first := generate()
	second := generate()
	if !bytes.Equal(first, second) {
		t.Fatal("correctness analyzer baseline generation is not deterministic")
	}
	for _, ruleID := range
		[]string{
			"atomic-update-assignment",
			"context-cancel-leak",
			"copied-lock",
			"http-response-before-error",
			"impossible-type-assertion",
			"loop-capture",
		} {
		if !bytes.Contains(first, []byte(`"rule_id": "` + ruleID + `"`)) {
			t.Fatalf("correctness analyzer baseline omits %s: %q", ruleID, first)
		}
	}
	if err := os.WriteFile(
		configPath,
		[]byte(
			"version = 1\n[lint]\npreset = \"correctness\"\n" +
				"[lint.baseline]\npath = \".glippy-baseline.json\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", path}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint baselined correctness catalog) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunExposesCorrectnessAnalyzerCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ruleID string
		message string
		needle string
		rangeLength int
		help string
		source string
	}{
		{
			name: "copied lock",
			ruleID: "copied-lock",
			message: "return copies lock value",
			needle: "*value",
			rangeLength: len("*value"),
			help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/copylock",
			source: `package sample

import "sync"

type state struct { lock sync.Mutex }

func copyState(value *state) state {
	return *value
}

func shareState(value *state) *state {
	return value
}
`,
		},
		{
			name: "impossible type assertion",
			ruleID: "impossible-type-assertion",
			message: "impossible type assertion",
			needle: "writer)",
			rangeLength: 0,
			help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/ifaceassert",
			source: `package sample

type reader interface { Read() }
type writer interface { Read(int) }

func cast(value reader) {
	_ = value.(writer)
	_ = value.(reader)
}
`,
		},
		{
			name: "context cancel leak",
			ruleID: "context-cancel-leak",
			message: "should be called, not discarded",
			needle: "_ := context.WithCancel",
			rangeLength: 1,
			help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel",
			source: `package sample

import "context"

func derive(parent context.Context) context.Context {
	child, _ := context.WithCancel(parent)
	return child
}

func deriveSafely(parent context.Context) context.Context {
	child, cancel := context.WithCancel(parent)
	defer cancel()
	return child
}
`,
		},
		{
			name: "HTTP response before error",
			ruleID: "http-response-before-error",
			message: "using response before checking for errors",
			needle: "response.Body.Close",
			rangeLength: len("response"),
			help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/httpresponse",
			source: `package sample

import "net/http"

func fetch() error {
	response, err := http.Get("https://example.test")
	defer response.Body.Close()
	if err != nil {
		return err
	}
	return nil
}

func fetchSafely() error {
	response, err := http.Get("https://example.test")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}
`,
		},
		{
			name: "atomic update assignment",
			ruleID: "atomic-update-assignment",
			message: "direct assignment to atomic value",
			needle: "*value = atomic.AddUint64",
			rangeLength: len("*value"),
			help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/atomic",
			source: `package sample

import "sync/atomic"

func increment(value *uint64) {
	*value = atomic.AddUint64(value, 1)
}

func incrementSafely(value *uint64) {
	atomic.AddUint64(value, 1)
}
`,
		},
		{
			name: "loop capture",
			ruleID: "loop-capture",
			message: "loop variable value captured by func literal",
			needle: "value }()",
			rangeLength: len("value"),
			help: "declare the iteration variable in the loop or pass it as a closure argument",
			source: `package sample

func capture(values []int) {
	var value int
	for _, value = range values {
		go func() { _ = value }()
	}
}

func safe(values []int) {
	for _, value := range values {
		go func() { _ = value }()
	}
}
`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				if err := os.WriteFile(
					filepath.Join(root, "go.mod"),
					[]byte("module example.com/standardcatalog\n\ngo 1.26.0\n"),
					0o600,
				);
					err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, "sample.go")
				if err := os.WriteFile(path, []byte(test.source), 0o600);
					err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(root, ".glippy.toml"),
					[]byte("version = 1\n[lint]\npreset = \"correctness\"\n"),
					0o600,
				);
					err != nil {
					t.Fatal(err)
				}

				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(
					[]string{"lint", "--reporter=json", path},
					strings.NewReader(""),
					&stdout,
					&stderr,
				)
				if exitCode != ExitFindings || stderr.Len() != 0 {
					t.Fatalf(
						"Run(lint %s) = exit %d, stdout %q, stderr %q",
						test.ruleID,
						exitCode,
						stdout.String(),
						stderr.String(),
					)
				}
				var result glippyreport.LintResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf(
						"decode %s JSON: %v; output = %q",
						test.ruleID,
						err,
						stdout.String(),
					)
				}
				if len(result.Diagnostics) != 1 {
					t.Fatalf(
						"%s diagnostics = %#v",
						test.ruleID,
						result.Diagnostics,
					)
				}
				diagnostic := result.Diagnostics[0]
				start := strings.Index(test.source, test.needle)
				if diagnostic.RuleID != test.ruleID ||
					diagnostic.Path != path ||
					!strings.Contains(diagnostic.Message, test.message) ||
					diagnostic.Help != test.help ||
					diagnostic.Range.Start != start ||
					diagnostic.Range.End != start + test.rangeLength {
					t.Fatalf("%s diagnostic = %#v", test.ruleID, diagnostic)
				}
			},
		)
	}
}

func TestRunExposesAndBaselinesAlmostSwapped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/almostswappedcli\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	input := `package sample

func swap(left, right int) {
	left = right
	right = left
}
`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint.rules]\nalmost-swapped = \"warn\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"lint", "--reporter=json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint almost-swapped) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "almost-swapped" {
		t.Fatalf("almost-swapped diagnostics = %#v", result.Diagnostics)
	}

	stdout.Reset()
	baselinePath := filepath.Join(root, ".glippy-baseline.json")
	exitCode = Run(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess ||
		stdout.String() !=
			"glippy lint: wrote baseline " + baselinePath + " (1 diagnostics)\n" ||
		stderr.Len() != 0 {
		t.Fatalf(
			"Run(baseline almost-swapped) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(baseline, []byte(`"rule_id": "almost-swapped"`)) {
		t.Fatalf("almost-swapped baseline = %q", baseline)
	}
}

func writeSyntaxOnlyProductConfig(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/syntaxfixture\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint.rules]\n" +
				"atomic-update-assignment = \"off\"\n" +
				"context-cancel-leak = \"off\"\n" +
				"copied-lock = \"off\"\n" +
				"http-response-before-error = \"off\"\n" +
				"impossible-type-assertion = \"off\"\n" +
				"loop-capture = \"off\"\n" +
				"self-assignment = \"off\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
}
