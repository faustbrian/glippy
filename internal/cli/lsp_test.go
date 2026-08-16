package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/lsp"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestRunLSPPublishesTypedDiagnosticsAndValidatedSafeAction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = [\"style\"]\n",
	)
	path := filepath.Join(root, "source.go")
	disk := "package sample\n\nfunc ready() bool { return true }\n"
	writeChangedCLIFile(t, path, disk)
	buffer := "package sample\n\nfunc run(ready, enabled bool) {\n\tif ready == true {\n\t\tprintln(\"ready\")\n\t}\n\tif enabled == true {\n\t\tprintln(\"enabled\")\n\t}\n}\n"
	uri := "file://" + filepath.ToSlash(path)
	input := cliLSPFrames(
		t,
		map[string]any{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "initialize",
			"params": map[string]any{},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
					"languageId": "go",
					"version": 9,
					"text": buffer,
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "textDocument/codeAction",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": uri},
				"range": map[string]any{
					"start": map[string]any{"line": 3, "character": 0},
					"end": map[string]any{"line": 4, "character": 0},
				},
				"context": map[string]any{"diagnostics": []any{}},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext(lsp) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	output := stdout.String()
	if !strings.Contains(output, `"method":"textDocument/publishDiagnostics"`) ||
		!strings.Contains(output, `"code":"redundant-bool-comparison"`) ||
		!strings.Contains(
			output,
			`"codeDescription":{"href":"https://github.com/faustbrian/gox/blob/main/docs/lint-rules.md#redundant-bool-comparison"}`,
		) ||
		!strings.Contains(output, `"version":9`) ||
		!strings.Contains(
			output,
			`"title":"redundant-bool-comparison: simplify-comparison [safe]"`,
		) ||
		!strings.Contains(output, `"title":"Fix all safe Glippy findings"`) ||
		!strings.Contains(output, `"kind":"source.fixAll.glippy"`) ||
		!strings.Contains(
			output,
			`"newText":"package sample\n\nfunc run(ready, enabled bool) {\n\tif ready {`,
		) ||
		!strings.Contains(output, `\n\tif enabled {`) {
		t.Fatalf("LSP output = %q", output)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != disk {
		t.Fatalf("LSP mutated disk source: %q", got)
	}
}

func TestRunLSPUsesBaseBuildSelectionInsteadOfCITargetMatrix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		`version = 1

[analysis]
build-tags = ["selected"]
goos = "linux"
goarch = "amd64"

[[analysis.targets]]
goos = "darwin"
goarch = "arm64"

[lint]
presets = []

[lint.rules]
nil-context = "warn"
`,
	)
	path := filepath.Join(root, "source.go")
	buffer := "//go:build selected && linux\n\npackage sample\n\nimport \"context\"\n\nfunc run() {\n\t_, cancel := context.WithCancel(nil)\n\tcancel()\n}\n"
	writeChangedCLIFile(t, path, buffer)
	uri := "file://" + filepath.ToSlash(path)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
					"languageId": "go",
					"version": 1,
					"text": buffer,
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), `"code":"nil-context"`) {
		t.Fatalf(
			"RunContext(lsp target policy) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunLSPUsesProjectContractsWithOpenBufferOverlay(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy-contracts.toml"),
		"version = 1\n[[functions]]\nsymbol = \"example.com/editor.stop\"\nnoreturn = true\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[analysis]\ncontract-files = [\".glippy-contracts.toml\"]\n[lint]\npresets = []\n[lint.rules]\nunreachable-code = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	disk := "package editor\n\nfunc stop() {}\n"
	buffer := "package editor\n\nfunc stop() {}\nfunc run() {\n\tstop()\n\tprintln(\"unreachable overlay\")\n}\n"
	writeChangedCLIFile(t, path, disk)
	uri := "file://" + filepath.ToSlash(path)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
					"languageId": "go",
					"version": 4,
					"text": buffer,
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	);
		exitCode != ExitSuccess ||
			stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), `"code":"unreachable-code"`) ||
			!strings.Contains(stdout.String(), `"version":4`) {
		t.Fatalf(
			"RunContext(lsp contracts) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != disk {
		t.Fatalf("LSP contract analysis mutated disk source: %q", got)
	}
}

func TestParseLSPInvocationControlsExplicitFixClasses(t *testing.T) {
	t.Parallel()

	invocation, valid := parseLSPInvocation(
		[]string{"lsp", "--fix-suggestions", "--fix-unsafe", "--config=glippy.toml"},
	)
	if !valid ||
		!invocation.fixSuggestions ||
		!invocation.fixUnsafe ||
		invocation.configPath != "glippy.toml" {
		t.Fatalf("parseLSPInvocation() = %#v, %t", invocation, valid)
	}
	for _, arguments := range
		[][]string{
			{"lsp", "--fix"},
			{"lsp", "--fix-suggestions", "--fix-suggestions"},
			{"lsp", "source.go"},
		} {
		if _, valid := parseLSPInvocation(arguments); valid {
			t.Fatalf("parseLSPInvocation(%q) accepted invalid invocation", arguments)
		}
	}
}

func TestRunLSPHidesSuggestionActionsUnlessExplicitlyEnabled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = [\"pedantic\"]\n",
	)
	path := filepath.Join(root, "source.go")
	buffer := "package sample\n\nimport \"time\"\n\nfunc elapsed(start time.Time) time.Duration {\n\treturn time.Now().Sub(start)\n}\n"
	writeChangedCLIFile(t, path, buffer)
	uri := "file://" + filepath.ToSlash(path)
	conversation := func() []byte {
		return cliLSPFrames(
			t,
			map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
			map[string]any{
				"jsonrpc": "2.0",
				"method": "textDocument/didOpen",
				"params": map[string]any{
					"textDocument": map[string]any{
						"uri": uri,
						"languageId": "go",
						"version": 3,
						"text": buffer,
					},
				},
			},
			map[string]any{
				"jsonrpc": "2.0",
				"id": 2,
				"method": "textDocument/codeAction",
				"params": map[string]any{
					"textDocument": map[string]any{"uri": uri},
					"range": map[string]any{
						"start": map[string]any{"line": 5, "character": 0},
						"end": map[string]any{"line": 6, "character": 0},
					},
				},
			},
			map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
			map[string]any{"jsonrpc": "2.0", "method": "exit"},
		)
	}
	for _, test := range
		[]struct {
			name string
			arguments []string
			wantAction bool
		}{
			{name: "default", arguments: []string{"lsp"}},
			{
				name: "suggestions enabled",
				arguments: []string{"lsp", "--fix-suggestions"},
				wantAction: true,
			},
		} {
		test := test
		t.Run(
			test.name,
			func(t *testing.T) {
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := RunContext(
					context.Background(),
					test.arguments,
					bytes.NewReader(conversation()),
					&stdout,
					&stderr,
				)
				if exitCode != ExitSuccess || stderr.Len() != 0 {
					t.Fatalf(
						"RunContext(lsp) = exit %d, stderr %q",
						exitCode,
						stderr.String(),
					)
				}
				hasAction := strings.Contains(
					stdout.String(),
					`"title":"time-since: use-time-since [suggestion]"`,
				)
				if hasAction != test.wantAction {
					t.Fatalf(
						"suggestion action present = %t, output %q",
						hasAction,
						stdout.String(),
					)
				}
			},
		)
	}
}

func TestRunLSPPublishesRuleLevelWithheldFixReasons(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editorwithheld\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = [\"style\"]\n",
	)
	path := filepath.Join(root, "source.go")
	buffer := "package sample\n\nfunc run(ready bool) {\n\t_ = ready /* keep */ == true\n}\n"
	writeChangedCLIFile(t, path, buffer)
	uri := "file://" + filepath.ToSlash(path)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
					"languageId": "go",
					"version": 4,
					"text": buffer,
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"RunContext(lsp) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	output := stdout.String()
	if !strings.Contains(
		output,
		`"withheld_fixes":[{"name":"simplify-comparison","reason":"comments","message":"simplifying this comparison would remove comments"}]`,
	) {
		t.Fatalf("LSP output omitted withheld fix reason: %q", output)
	}
}

func TestRunLSPUsesConfiguredPersistentCacheForTypedOverlays(t *testing.T) {
	root := t.TempDir()
	cacheRoot := t.TempDir()
	t.Setenv(cacheDirectoryEnvironment, cacheRoot)
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = [\"style\"]\n[cache]\nenabled = true\nmax-entries = 64\nmax-bytes = 10485760\n",
	)
	path := filepath.Join(root, "source.go")
	disk := "package sample\n"
	buffer := "package sample\n\nfunc run(ready bool) {\n\tif ready == true {\n\t}\n}\n"
	writeChangedCLIFile(t, path, disk)
	uri := "file://" + filepath.ToSlash(path)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
					"languageId": "go",
					"version": 1,
					"text": buffer,
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	);
		exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("RunContext(lsp) = exit %d, stderr %q", exitCode, stderr.String())
	}
	entries := 0
	err := filepath.WalkDir(
		cacheRoot,
		func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				entries++
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if entries == 0 {
		t.Fatal("LSP typed analysis did not populate the configured persistent cache")
	}
}

func TestRunLSPAnalyzesEachDocumentAgainstAllOpenWorkspaceOverlays(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	definitionPath := filepath.Join(root, "definition.go")
	consumerPath := filepath.Join(root, "consumer.go")
	writeChangedCLIFile(t, definitionPath, "package sample\n\nfunc value() int { return 1 }\n")
	writeChangedCLIFile(t, consumerPath, "package sample\n\nvar _ int = value()\n")
	definitionURI := "file://" + filepath.ToSlash(definitionPath)
	consumerURI := "file://" + filepath.ToSlash(consumerPath)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": definitionURI,
					"languageId": "go",
					"version": 1,
					"text": "package sample\n\nfunc value() int { return 1 }\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": consumerURI,
					"languageId": "go",
					"version": 1,
					"text": "package sample\n\nvar _ int = value()\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didChange",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": definitionURI, "version": 2},
				"contentChanges": []any{
					map[string]any{
						"text": "package sample\n\nfunc value() string { return \"text\" }\n",
					},
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	);
		exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("RunContext(lsp) = exit %d, stderr %q", exitCode, stderr.String())
	}
	if got := strings.Count(stdout.String(), `"code":"glippy"`); got != 2 {
		t.Fatalf(
			"typed workspace diagnostics = %d, want 2; output = %q",
			got,
			stdout.String(),
		)
	}
}

func TestLSPWorkspaceBatchesCompatibleTypedDocumentsIntoOnePackageLoad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	firstPath := filepath.Join(root, "first.go")
	secondPath := filepath.Join(root, "second.go")
	first := "package sample\n\nfunc first() int { return second() }\n"
	second := "package sample\n\nfunc second() int { return 2 }\n"
	writeChangedCLIFile(t, firstPath, first)
	writeChangedCLIFile(t, secondPath, second)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			runs++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(firstPath),
			Path: firstPath,
			Version: 1,
			Text: []byte(first),
		},
		{
			URI: "file://" + filepath.ToSlash(secondPath),
			Path: secondPath,
			Version: 1,
			Text: []byte(second),
		},
	}
	results, err := backend.AnalyzeWorkspace(context.Background(), documents)
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("typed workspace package loads = %d, want 1", runs)
	}
	if len(results) != len(documents) {
		t.Fatalf("workspace results = %d, want %d", len(results), len(documents))
	}
	for index, result := range results {
		if result.Err != nil ||
			result.Analysis.File == nil ||
			result.Analysis.File.Path() != documents[index].Path {
			t.Fatalf("workspace result %d = %#v", index, result)
		}
	}
}

func TestLSPWorkspaceIsolatesUnrelatedPackageFailures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	badPath := filepath.Join(root, "bad", "bad.go")
	goodPath := filepath.Join(root, "good", "good.go")
	if err := os.MkdirAll(filepath.Dir(badPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(goodPath), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "package bad\n\nvar _ int = \"bad\"\n"
	good := "package good\n\nfunc Value() int { return 1 }\n"
	writeChangedCLIFile(t, badPath, bad)
	writeChangedCLIFile(t, goodPath, good)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(badPath),
			Path: badPath,
			Version: 1,
			Text: []byte(bad),
		},
		{
			URI: "file://" + filepath.ToSlash(goodPath),
			Path: goodPath,
			Version: 1,
			Text: []byte(good),
		},
	}
	results, err := backend.AnalyzeWorkspace(context.Background(), documents)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Err == nil {
		t.Fatalf("bad package result = %#v", results)
	}
	if results[1].Err != nil || results[1].Analysis.File == nil {
		t.Fatalf("good package inherited unrelated failure: %#v", results[1])
	}
}

func TestLSPWorkspaceIsolatesExternalTestPackageFailures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	packagePath := filepath.Join(root, "value.go")
	testPath := filepath.Join(root, "value_test.go")
	packageSource := "package sample\n\nfunc Value() int { return 1 }\n"
	testSource := "package sample_test\n\nvar _ int = \"bad\"\n"
	writeChangedCLIFile(t, packagePath, packageSource)
	writeChangedCLIFile(t, testPath, testSource)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(packagePath),
			Path: packagePath,
			Version: 1,
			Text: []byte(packageSource),
		},
		{
			URI: "file://" + filepath.ToSlash(testPath),
			Path: testPath,
			Version: 1,
			Text: []byte(testSource),
		},
	}
	results, err := backend.AnalyzeWorkspace(context.Background(), documents)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Err != nil || results[0].Analysis.File == nil {
		t.Fatalf("ordinary package inherited external-test failure: %#v", results)
	}
	if results[1].Err == nil {
		t.Fatalf("external test package result = %#v", results[1])
	}
}

func TestRunLSPClosingOverlayRefreshesRemainingWorkspaceDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	definitionPath := filepath.Join(root, "definition.go")
	consumerPath := filepath.Join(root, "consumer.go")
	writeChangedCLIFile(t, definitionPath, "package sample\n\nfunc value() int { return 1 }\n")
	writeChangedCLIFile(t, consumerPath, "package sample\n\nvar _ int = value()\n")
	definitionURI := "file://" + filepath.ToSlash(definitionPath)
	consumerURI := "file://" + filepath.ToSlash(consumerPath)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": definitionURI,
					"languageId": "go",
					"version": 1,
					"text": "package sample\n\nfunc value() string { return \"text\" }\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": consumerURI,
					"languageId": "go",
					"version": 1,
					"text": "package sample\n\nvar _ int = value()\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didClose",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": definitionURI},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	);
		exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("RunContext(lsp) = exit %d, stderr %q", exitCode, stderr.String())
	}
	if got := strings.Count(stdout.String(), `"diagnostics":[]`); got != 2 {
		t.Fatalf(
			"workspace diagnostic clears = %d, want 2; output = %q",
			got,
			stdout.String(),
		)
	}
}

func TestRunLSPValidatesTypedActionsAgainstTheWorkspaceSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = [\"style\"]\n",
	)
	definitionPath := filepath.Join(root, "definition.go")
	consumerPath := filepath.Join(root, "consumer.go")
	writeChangedCLIFile(t, definitionPath, "package sample\n")
	consumer := "package sample\n\nfunc run() {\n\tif ready() == true {\n\t}\n}\n"
	writeChangedCLIFile(t, consumerPath, consumer)
	definitionURI := "file://" + filepath.ToSlash(definitionPath)
	consumerURI := "file://" + filepath.ToSlash(consumerPath)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": definitionURI,
					"languageId": "go",
					"version": 1,
					"text": "package sample\n\nfunc ready() bool { return true }\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": consumerURI,
					"languageId": "go",
					"version": 1,
					"text": consumer,
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "textDocument/codeAction",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": consumerURI},
				"range": map[string]any{
					"start": map[string]any{"line": 3, "character": 0},
					"end": map[string]any{"line": 4, "character": 0},
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	);
		exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("RunContext(lsp) = exit %d, stderr %q", exitCode, stderr.String())
	}
	if !strings.Contains(
		stdout.String(),
		`"title":"redundant-bool-comparison: simplify-comparison [safe]"`,
	) {
		t.Fatalf("workspace-backed code action missing: %q", stdout.String())
	}
}

func TestRunLSPFormatsTheExactBufferWithoutMutatingDisk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "source.go")
	disk := "package sample\n"
	buffer := "package sample\nfunc run(){println(\"buffer\")}\n"
	writeChangedCLIFile(t, path, disk)
	uri := "file://" + filepath.ToSlash(path)
	input := cliLSPFrames(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
					"languageId": "go",
					"version": 2,
					"text": buffer,
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "textDocument/formatting",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": uri},
				"options": map[string]any{"tabSize": 8, "insertSpaces": false},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := RunContext(
		context.Background(),
		[]string{"lsp"},
		bytes.NewReader(input),
		&stdout,
		&stderr,
	);
		exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("RunContext(lsp) = exit %d, stderr %q", exitCode, stderr.String())
	}
	if !strings.Contains(
		stdout.String(),
		`"newText":"package sample\n\nfunc run() {\n\tprintln(\"buffer\")\n}\n"`,
	) {
		t.Fatalf("formatting response = %q", stdout.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != disk {
		t.Fatalf("LSP formatting mutated disk source: %q", got)
	}
}

func cliLSPFrames(t *testing.T, messages ...map[string]any) []byte {
	t.Helper()
	var result bytes.Buffer
	for _, message := range messages {
		encoded, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&result, "Content-Length: %d\r\n\r\n", len(encoded))
		result.Write(encoded)
	}
	return result.Bytes()
}
