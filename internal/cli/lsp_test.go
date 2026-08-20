package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestLSPWorkspaceRetypechecksSamePackageWithoutFullReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	helperPath := filepath.Join(root, "helper.go")
	original := "package sample\n\nfunc run() {\n\tvalue := seed()\n\t_ = value\n}\n"
	changed := "package sample\n\nfunc run() {\n\tvalue := seed()\n\tvalue = value\n}\n"
	writeChangedCLIFile(t, path, original)
	writeChangedCLIFile(t, helperPath, "package sample\n\nfunc seed() int { return 1 }\n")
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	first, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Err != nil || len(first[0].Analysis.Diagnostics) != 0 {
		t.Fatalf("initial workspace analysis = %#v", first)
	}
	document.Version = 2
	document.Text = []byte(changed)
	second, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", second)
	}
	if diagnostics := second[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want one full and one incremental load",
			statistics,
		)
	}

	clean := &lspBackend{registry: registry}
	cleanResult, err := clean.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanResult) != 1 || cleanResult[0].Err != nil {
		t.Fatalf("clean workspace analysis = %#v", cleanResult)
	}
	cleanDiagnostics := cleanResult[0].Analysis.Diagnostics
	changedDiagnostics := second[0].Analysis.Diagnostics
	if len(cleanDiagnostics) != len(changedDiagnostics) ||
		cleanDiagnostics[0].Code != changedDiagnostics[0].Code ||
		cleanDiagnostics[0].Range != changedDiagnostics[0].Range ||
		cleanDiagnostics[0].Message != changedDiagnostics[0].Message {
		t.Fatalf(
			"incremental diagnostics = %#v, clean diagnostics = %#v",
			changedDiagnostics,
			cleanDiagnostics,
		)
	}
}

func TestLSPWorkspaceRetypechecksNestedPackageWithoutFullReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	packageDirectory := filepath.Join(root, "nested")
	if err := os.MkdirAll(packageDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(packageDirectory, "source.go")
	original := "package nested\n\nfunc run() {\n\tvalue := 1\n\t_ = value\n}\n"
	changed := "package nested\n\nfunc run() {\n\tvalue := 1\n\tvalue = value\n}\n"
	writeChangedCLIFile(t, path, original)
	writeChangedCLIFile(
		t,
		filepath.Join(packageDirectory, "ignored.go"),
		"//go:build glippy_never\n\npackage nested\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want nested incremental load",
			statistics,
		)
	}
}

func TestLSPWorkspaceLoadsNewImportWithoutFullRootReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package sample\n\nfunc run() int { return 1 }\n"
	changed := "package sample\n\nimport \"strings\"\n\nfunc run() string {\n\tvalue := strings.TrimSpace(\" value \")\n\tvalue = value\n\treturn value\n}\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 ||
		statistics.IncrementalLoads != 1 ||
		statistics.ImportLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want incremental import load",
			statistics,
		)
	}
	clean := &lspBackend{registry: registry}
	cleanResult, err := clean.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanResult) != 1 || cleanResult[0].Err != nil {
		t.Fatalf("clean workspace analysis = %#v", cleanResult)
	}
	cleanDiagnostics := cleanResult[0].Analysis.Diagnostics
	incrementalDiagnostics := result[0].Analysis.Diagnostics
	if len(cleanDiagnostics) != len(incrementalDiagnostics) ||
		cleanDiagnostics[0].Code != incrementalDiagnostics[0].Code ||
		cleanDiagnostics[0].Range != incrementalDiagnostics[0].Range ||
		cleanDiagnostics[0].Message != incrementalDiagnostics[0].Message {
		t.Fatalf(
			"incremental diagnostics = %#v, clean diagnostics = %#v",
			incrementalDiagnostics,
			cleanDiagnostics,
		)
	}
}

func TestLSPWorkspaceRetainsNewLocalImportAcrossEdits(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	helperDirectory := filepath.Join(root, "helper")
	if err := os.MkdirAll(helperDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(helperDirectory, "helper.go"),
		"package helper\n\nfunc Value() int { return 1 }\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package sample\n\nfunc run() int { return 1 }\n"
	changed := "package sample\n\nimport \"example.com/editor/helper\"\n\nfunc run() int {\n\tvalue := helper.Value()\n\tvalue = value\n\treturn value\n}\n"
	changedAgain := strings.Replace(
		changed,
		"\tvalue := helper.Value()",
		"\t// Latest editor version.\n\tvalue := helper.Value()",
		1,
	)
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	document.Version = 3
	document.Text = []byte(changedAgain)
	result, err = backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("latest workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("latest workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 ||
		statistics.IncrementalLoads != 2 ||
		statistics.ImportLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want retained local import",
			statistics,
		)
	}
}

func TestLSPWorkspaceAdmitsRetainedTransitiveImportWithoutFullReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	leafDirectory := filepath.Join(root, "leaf")
	middleDirectory := filepath.Join(root, "middle")
	if err := os.MkdirAll(leafDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(middleDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(leafDirectory, "leaf.go"),
		"package leaf\n\nfunc Value() int { return 1 }\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(middleDirectory, "middle.go"),
		"package middle\n\nimport \"example.com/editor/leaf\"\n\nfunc Value() int { return leaf.Value() }\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package editor\n\nimport \"example.com/editor/middle\"\n\nfunc run() int { return middle.Value() }\n"
	changed := "package editor\n\nimport (\n\t\"example.com/editor/leaf\"\n\t\"example.com/editor/middle\"\n)\n\nfunc run() int {\n\t_ = middle.Value()\n\tvalue := leaf.Value()\n\tvalue = value\n\treturn value\n}\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want retained-import recheck",
			statistics,
		)
	}
	clean := &lspBackend{registry: registry}
	cleanResult, err := clean.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanResult) != 1 || cleanResult[0].Err != nil {
		t.Fatalf("clean workspace analysis = %#v", cleanResult)
	}
	cleanDiagnostics := cleanResult[0].Analysis.Diagnostics
	incrementalDiagnostics := result[0].Analysis.Diagnostics
	if len(cleanDiagnostics) != len(incrementalDiagnostics) ||
		cleanDiagnostics[0].Code != incrementalDiagnostics[0].Code ||
		cleanDiagnostics[0].Range != incrementalDiagnostics[0].Range ||
		cleanDiagnostics[0].Message != incrementalDiagnostics[0].Message {
		t.Fatalf(
			"incremental diagnostics = %#v, clean diagnostics = %#v",
			incrementalDiagnostics,
			cleanDiagnostics,
		)
	}
}

func TestLSPWorkspaceFallsBackWhenBuildConstraintChanges(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package sample\n\nfunc run() int { return 1 }\n"
	changed := "//go:build !glippy_never\n\npackage sample\n\nfunc run() int { return 1 }\n"
	writeChangedCLIFile(t, path, original)
	writeChangedCLIFile(t, filepath.Join(root, "helper.go"), "package sample\n")
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want build-selection fallback",
			statistics,
		)
	}
}

func TestLSPWorkspaceFallsBackWhenIgnoredRootSelectionChanges(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	ignoredPath := filepath.Join(root, "ignored.go")
	original := "package sample\n\nfunc run() int { return 1 }\n"
	changed := "package sample\n\nfunc run() int {\n\tvalue := 1\n\tvalue = value\n\treturn value\n}\n"
	writeChangedCLIFile(t, path, original)
	writeChangedCLIFile(
		t,
		ignoredPath,
		"//go:build glippy_never\n\npackage sample\n\nfunc helper() int { return 1 }\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		ignoredPath,
		"//go:build !glippy_never\n\npackage sample\n\nfunc helper() int { return 1 }\n",
	)
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want ignored-root fallback",
			statistics,
		)
	}
}

func TestLSPWorkspaceRetypechecksInternalTestVariantWithoutFullReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package sample\n\nfunc value() int { return 1 }\n"
	changed := "package sample\n\nfunc value() int {\n\tresult := 1\n\tresult = result\n\treturn result\n}\n"
	writeChangedCLIFile(t, path, original)
	writeChangedCLIFile(
		t,
		filepath.Join(root, "source_test.go"),
		"package sample\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n\tif value() != 1 {\n\t\tt.Fatal(\"unexpected value\")\n\t}\n}\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want incremental test variant",
			statistics,
		)
	}
	clean := &lspBackend{registry: registry}
	cleanResult, err := clean.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanResult) != 1 || cleanResult[0].Err != nil {
		t.Fatalf("clean workspace analysis = %#v", cleanResult)
	}
	cleanDiagnostics := cleanResult[0].Analysis.Diagnostics
	incrementalDiagnostics := result[0].Analysis.Diagnostics
	if len(cleanDiagnostics) != len(incrementalDiagnostics) ||
		cleanDiagnostics[0].Code != incrementalDiagnostics[0].Code ||
		cleanDiagnostics[0].Range != incrementalDiagnostics[0].Range ||
		cleanDiagnostics[0].Message != incrementalDiagnostics[0].Message {
		t.Fatalf(
			"incremental diagnostics = %#v, clean diagnostics = %#v",
			incrementalDiagnostics,
			cleanDiagnostics,
		)
	}
}

func TestLSPWorkspaceRetypechecksChangedInternalTestBufferWithoutFullReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, "source.go"),
		"package sample\n\nfunc value() int { return 1 }\n",
	)
	path := filepath.Join(root, "source_test.go")
	original := `package sample

import "testing"

func TestValue(t *testing.T) {
	if value() != 1 {
		t.Fatal("unexpected value")
	}
}
`
	changed := `package sample

import "testing"

func TestValue(t *testing.T) {
	result := value()
	result = result
	if result != 1 {
		t.Fatal("unexpected value")
	}
}
`
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed internal-test analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed internal-test diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want changed-test recheck",
			statistics,
		)
	}
}

func TestLSPWorkspaceRetypechecksChangedExternalTestBufferWithoutFullReload(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, "source.go"),
		"package sample\n\nfunc Value() int { return 1 }\n",
	)
	path := filepath.Join(root, "external_test.go")
	original := `package sample_test

import (
	"testing"

	sample "example.com/editor"
)

func TestValue(t *testing.T) {
	if sample.Value() != 1 {
		t.Fatal("unexpected value")
	}
}
`
	changed := `package sample_test

import (
	"testing"

	sample "example.com/editor"
)

func TestValue(t *testing.T) {
	result := sample.Value()
	result = result
	if result != 1 {
		t.Fatal("unexpected value")
	}
}
`
	changedAgain := strings.Replace(
		changed,
		"\tresult := sample.Value()",
		"\t// Latest editor version.\n\tresult := sample.Value()",
		1,
	)
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed external-test analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed external-test diagnostics = %#v", diagnostics)
	}
	document.Version = 3
	document.Text = []byte(changedAgain)
	result, err = backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("latest external-test analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("latest external-test diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 2 {
		t.Fatalf(
			"typed package session statistics = %#v, want external-test recheck",
			statistics,
		)
	}
}

func TestLSPWorkspaceFallsBackWhenInternalTestBuildConstraintChanges(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, "source.go"),
		"package sample\n\nfunc value() int { return 1 }\n",
	)
	path := filepath.Join(root, "source_test.go")
	original := "package sample\n\nfunc TestValue() { _ = value() }\n"
	changed := "//go:build !glippy_never\n\npackage sample\n\nfunc TestValue() { _ = value() }\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed internal-test analysis = %#v", result)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want test-selection fallback",
			statistics,
		)
	}
}

func TestLSPWorkspaceFallsBackWhenClosedDependencyChangesOnDisk(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	consumerPath := filepath.Join(root, "consumer", "consumer.go")
	if err := os.MkdirAll(filepath.Dir(dependencyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(consumerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		dependencyPath,
		"package dependency\n\nfunc Value() int { return 1 }\n",
	)
	original := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc run() {\n\tvalue := dependency.Value()\n\t_ = value\n}\n"
	changed := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc run() {\n\tvalue := dependency.Value()\n\tvalue = value\n}\n"
	writeChangedCLIFile(t, consumerPath, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(consumerPath),
		Path: consumerPath,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		dependencyPath,
		"package dependency\n\nfunc Value() string { return \"value\" }\n",
	)
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want dependency-change fallback",
			statistics,
		)
	}
}

func TestLSPWorkspaceFallsBackWhenIgnoredDependencySelectionChanges(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	dependencyDirectory := filepath.Join(root, "dependency")
	if err := os.MkdirAll(dependencyDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(dependencyDirectory, "dependency.go"),
		"package dependency\n\nfunc Value() int { return 1 }\n",
	)
	ignoredPath := filepath.Join(dependencyDirectory, "ignored.go")
	writeChangedCLIFile(
		t,
		ignoredPath,
		"//go:build glippy_never\n\npackage dependency\n\nfunc Extra() int { return 2 }\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package editor\n\nimport \"example.com/editor/dependency\"\n\nfunc run() {\n\tvalue := dependency.Value()\n\t_ = value\n}\n"
	changed := "package editor\n\nimport \"example.com/editor/dependency\"\n\nfunc run() {\n\tvalue := dependency.Value()\n\tvalue = value\n}\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		ignoredPath,
		"//go:build !glippy_never\n\npackage dependency\n\nfunc Extra() int { return 2 }\n",
	)
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want ignored-dependency fallback",
			statistics,
		)
	}
}

func TestLSPWorkspaceFallsBackWhenExternalLocalDependencyChangesOnDisk(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "editor")
	dependencyRoot := filepath.Join(parent, "dependency")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dependencyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n\nrequire example.com/dependency v0.0.0\n\nreplace example.com/dependency => ../dependency\n",
	)
	writeChangedCLIFile(
		t,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	dependencyPath := filepath.Join(dependencyRoot, "dependency.go")
	writeChangedCLIFile(
		t,
		filepath.Join(dependencyRoot, "go.mod"),
		"module example.com/dependency\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		t,
		dependencyPath,
		"package dependency\n\nfunc Value() int { return 1 }\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package editor\n\nimport \"example.com/dependency\"\n\nfunc run() {\n\tvalue := dependency.Value()\n\t_ = value\n}\n"
	changed := "package editor\n\nimport \"example.com/dependency\"\n\nfunc run() {\n\tvalue := dependency.Value()\n\tvalue = value\n}\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		dependencyPath,
		"package dependency\n\nfunc Value() string { return \"value\" }\n",
	)
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "self-assignment" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want external dependency fallback",
			statistics,
		)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(dependencyRoot, "go.mod"),
		"module example.com/dependency\n\ngo 1.26.0\n\n// refreshed\n",
	)
	document.Version = 3
	document.Text = []byte(original)
	result, err = backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("module-changed workspace analysis = %#v", result)
	}
	statistics = backend.packageSession.Statistics()
	if statistics.FullLoads != 3 || statistics.IncrementalLoads != 0 {
		t.Fatalf(
			"typed package session statistics = %#v, want external module fallback",
			statistics,
		)
	}
}

func TestLSPWorkspaceRebuildsControlFlowFromIncrementalTypes(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nunreachable-code = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package sample\n\nfunc stop() { panic(\"stop\") }\n\nfunc run() { stop() }\n"
	changed := "package sample\n\nfunc stop() { panic(\"stop\") }\n\nfunc run() {\n\tstop()\n\tprintln(\"unreachable\")\n}\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "unreachable-code" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want incremental CFG rebuild",
			statistics,
		)
	}
}

func TestLSPWorkspaceRebuildsSSAFromIncrementalTypes(t *testing.T) {
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
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nnilness = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	original := "package sample\n\nfunc run() {\n\tvar value *int\n\t_ = value\n}\n"
	changed := "package sample\n\nfunc run() {\n\tvar value *int\n\t_ = *value\n}\n"
	writeChangedCLIFile(t, path, original)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(original),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		t.Fatal(err)
	}
	document.Version = 2
	document.Text = []byte(changed)
	result, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Err != nil {
		t.Fatalf("changed workspace analysis = %#v", result)
	}
	if diagnostics := result[0].Analysis.Diagnostics;
		len(diagnostics) != 1 || diagnostics[0].Code != "nilness" {
		t.Fatalf("changed workspace diagnostics = %#v", diagnostics)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want incremental SSA rebuild",
			statistics,
		)
	}
}

func TestLSPWorkspaceReusesUnaffectedPackageAcrossDocumentSnapshots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	leftPath := filepath.Join(root, "left", "left.go")
	rightPath := filepath.Join(root, "right", "right.go")
	if err := os.MkdirAll(filepath.Dir(leftPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rightPath), 0o755); err != nil {
		t.Fatal(err)
	}
	left := "package left\n\nfunc Value() int { return 1 }\n"
	right := "package right\n\nfunc Value() int { return 1 }\n"
	writeChangedCLIFile(t, leftPath, left)
	writeChangedCLIFile(t, rightPath, right)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			if len(task.patterns) != 1 {
				t.Fatalf(
					"package patterns = %q, want one file pattern",
					task.patterns,
				)
			}
			runs[task.patterns[0]]++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(leftPath),
			Path: leftPath,
			Version: 1,
			Text: []byte(left),
		},
		{
			URI: "file://" + filepath.ToSlash(rightPath),
			Path: rightPath,
			Version: 1,
			Text: []byte(right),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	changedLeft := "package left\n\nfunc Value() int { return 2 }\n"
	documents[0].Version = 2
	documents[0].Text = []byte(changedLeft)
	results, err := backend.AnalyzeWorkspace(context.Background(), documents)
	if err != nil {
		t.Fatal(err)
	}
	leftPattern := "file=" + leftPath
	rightPattern := "file=" + rightPath
	if runs[leftPattern] != 2 || runs[rightPattern] != 1 {
		t.Fatalf(
			"package runs = %#v, want changed left twice and unaffected right once",
			runs,
		)
	}
	state, ok := results[1].Analysis.State.(*lspAnalysisState)
	if !ok || state == nil || !bytes.Equal(state.overlay[leftPath], []byte(changedLeft)) {
		t.Fatalf("reused analysis does not retain the current workspace overlay")
	}
}

func TestLSPWorkspaceOpeningUnrelatedPackageReusesKnownEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	leftPath := filepath.Join(root, "left", "left.go")
	rightPath := filepath.Join(root, "right", "right.go")
	if err := os.MkdirAll(filepath.Dir(leftPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rightPath), 0o755); err != nil {
		t.Fatal(err)
	}
	left := "package left\n\nfunc Value() int { return 1 }\n"
	right := "package right\n\nfunc Value() int { return 2 }\n"
	writeChangedCLIFile(t, leftPath, left)
	writeChangedCLIFile(t, rightPath, right)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			runs[task.patterns[0]]++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	leftDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(leftPath),
		Path: leftPath,
		Version: 1,
		Text: []byte(left),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{leftDocument});
		err != nil {
		t.Fatal(err)
	}
	rightDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(rightPath),
		Path: rightPath,
		Version: 1,
		Text: []byte(right),
	}
	if _, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{leftDocument, rightDocument},
	);
		err != nil {
		t.Fatal(err)
	}
	leftPattern := "file=" + leftPath
	rightPattern := "file=" + rightPath
	if runs[leftPattern] != 1 || runs[rightPattern] != 1 {
		t.Fatalf(
			"package runs = %#v, want known left reused and new right loaded once",
			runs,
		)
	}
}

func TestLSPWorkspaceOpeningDependencyInvalidatesKnownReverseDependant(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	consumerPath := filepath.Join(root, "consumer", "consumer.go")
	if err := os.MkdirAll(filepath.Dir(dependencyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(consumerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	dependency := "package dependency\n\nfunc Value() int { return 1 }\n"
	consumer := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc Value() int { return dependency.Value() }\n"
	writeChangedCLIFile(t, dependencyPath, dependency)
	writeChangedCLIFile(t, consumerPath, consumer)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			runs[task.patterns[0]]++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	consumerDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(consumerPath),
		Path: consumerPath,
		Version: 1,
		Text: []byte(consumer),
	}
	if _, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{consumerDocument},
	);
		err != nil {
		t.Fatal(err)
	}
	dependencyDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(dependencyPath),
		Path: dependencyPath,
		Version: 1,
		Text: []byte(dependency),
	}
	if _, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{consumerDocument, dependencyDocument},
	);
		err != nil {
		t.Fatal(err)
	}
	consumerPattern := "file=" + consumerPath
	dependencyPattern := "file=" + dependencyPath
	if runs[consumerPattern] != 2 || runs[dependencyPattern] != 1 {
		t.Fatalf(
			"package runs = %#v, want new dependency to invalidate known consumer",
			runs,
		)
	}
}

func TestLSPWorkspaceMetadataDiscoveryScopesFailedNewPackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	consumerPath := filepath.Join(root, "consumer", "consumer.go")
	unrelatedPath := filepath.Join(root, "unrelated", "unrelated.go")
	for _, path := range []string{dependencyPath, consumerPath, unrelatedPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dependency := "package dependency\n\nfunc Value() int { return 1 }\n"
	consumer := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc Value() int { return dependency.Value() }\n"
	unrelated := "package unrelated\n\nfunc Value() int { return 2 }\n"
	writeChangedCLIFile(t, dependencyPath, dependency)
	writeChangedCLIFile(t, consumerPath, consumer)
	writeChangedCLIFile(t, unrelatedPath, unrelated)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	discoveries := 0
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			pattern := task.patterns[0]
			runs[pattern]++
			if pattern == "file=" + dependencyPath {
				return analysis.PackageResult{}, errors.New(
					"typed load failed before graph identity",
				)
			}
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
		runPackageGraphDiscovery: func(
			ctx context.Context,
			options analysis.PackageLoadOptions,
		) (analysis.PackageGraphResult, error) {
			discoveries++
			return analysis.DiscoverPackageGraph(ctx, options)
		},
	}
	consumerDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(consumerPath),
		Path: consumerPath,
		Version: 1,
		Text: []byte(consumer),
	}
	unrelatedDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(unrelatedPath),
		Path: unrelatedPath,
		Version: 1,
		Text: []byte(unrelated),
	}
	if _, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{consumerDocument, unrelatedDocument},
	);
		err != nil {
		t.Fatal(err)
	}
	dependencyDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(dependencyPath),
		Path: dependencyPath,
		Version: 1,
		Text: []byte(dependency),
	}
	results, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{consumerDocument, dependencyDocument, unrelatedDocument},
	)
	if err != nil {
		t.Fatal(err)
	}
	if results[1].Err == nil {
		t.Fatal("failed new package analysis did not retain its error")
	}
	if discoveries != 1 {
		t.Fatalf("metadata discoveries = %d, want 1", discoveries)
	}
	consumerPattern := "file=" + consumerPath
	dependencyPattern := "file=" + dependencyPath
	unrelatedPattern := "file=" + unrelatedPath
	if runs[consumerPattern] != 2 ||
		runs[dependencyPattern] != 1 ||
		runs[unrelatedPattern] != 1 {
		t.Fatalf(
			"package runs = %#v, want reverse dependant rerun and unrelated package reused",
			runs,
		)
	}
}

func TestLSPWorkspaceMetadataDiscoveryPreservesCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "sample", "sample.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	input := "package sample\n\nfunc Value() int { return 1 }\n"
	writeChangedCLIFile(t, path, input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			context.Context,
			*rules.Registry,
			lintPackageTask,
			map[string][]byte,
		) (analysis.PackageResult, error) {
			return analysis.PackageResult{}, errors.New(
				"typed load failed before graph identity",
			)
		},
		runPackageGraphDiscovery: func(
			context.Context,
			analysis.PackageLoadOptions,
		) (analysis.PackageGraphResult, error) {
			return analysis.PackageGraphResult{}, context.Canceled
		},
	}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(input),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		!errors.Is(err, context.Canceled) {
		t.Fatalf("AnalyzeWorkspace() error = %v, want context cancellation", err)
	}
}

func TestLSPWorkspaceMetadataDiscoveryFailureInvalidatesRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	knownPath := filepath.Join(root, "known", "known.go")
	newPath := filepath.Join(root, "newpkg", "new.go")
	for _, path := range []string{knownPath, newPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	known := "package known\n\nfunc Value() int { return 1 }\n"
	newSource := "package newpkg\n\nfunc Value() int { return 2 }\n"
	writeChangedCLIFile(t, knownPath, known)
	writeChangedCLIFile(t, newPath, newSource)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			pattern := task.patterns[0]
			runs[pattern]++
			if pattern == "file=" + newPath {
				return analysis.PackageResult{}, errors.New(
					"typed load failed before graph identity",
				)
			}
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
		runPackageGraphDiscovery: func(
			context.Context,
			analysis.PackageLoadOptions,
		) (analysis.PackageGraphResult, error) {
			return analysis.PackageGraphResult{}, errors.New("metadata unavailable")
		},
	}
	knownDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(knownPath),
		Path: knownPath,
		Version: 1,
		Text: []byte(known),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{knownDocument});
		err != nil {
		t.Fatal(err)
	}
	newDocument := lsp.Document{
		URI: "file://" + filepath.ToSlash(newPath),
		Path: newPath,
		Version: 1,
		Text: []byte(newSource),
	}
	results, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{knownDocument, newDocument},
	)
	if err != nil {
		t.Fatal(err)
	}
	if results[1].Err == nil {
		t.Fatal("failed new package analysis did not retain its error")
	}
	knownPattern := "file=" + knownPath
	newPattern := "file=" + newPath
	if runs[knownPattern] != 2 || runs[newPattern] != 1 {
		t.Fatalf("package runs = %#v, want conservative root-wide invalidation", runs)
	}
}

func TestLSPWorkspaceMetadataDiscoveryRequiresExactOverlay(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "sample", "sample.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(t, path, "package sample\n")
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	discoveries := 0
	backend := &lspBackend{
		registry: registry,
		runPackageGraphDiscovery: func(
			context.Context,
			analysis.PackageLoadOptions,
		) (analysis.PackageGraphResult, error) {
			discoveries++
			return analysis.PackageGraphResult{
				RootPackagePaths: []string{"example.com/editor/sample"},
			}, nil
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(path),
			Path: path,
			Version: 1,
			Text: []byte("package sample\n\nconst Value = 1\n"),
		},
		{
			URI: "file://" + filepath.ToSlash(path),
			Path: path,
			Version: 2,
			Text: []byte("package sample\n\nconst Value = 2\n"),
		},
	}
	results, err := backend.AnalyzeWorkspace(context.Background(), documents)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Err == nil || results[1].Err == nil {
		t.Fatalf("incompatible overlay results = %#v", results)
	}
	if discoveries != 0 {
		t.Fatalf(
			"metadata discoveries = %d, want none without an exact overlay",
			discoveries,
		)
	}
}

func TestLSPPackageSnapshotRetainsGraphOnAnalysisFailure(t *testing.T) {
	t.Parallel()

	want := analysis.PackageResult{RootPackagePaths: []string{"example.com/editor/sample"}}
	sentinel := errors.New("typed package validation failed")
	backend := &lspBackend{
		runPackageAnalysis: func(
			context.Context,
			*rules.Registry,
			lintPackageTask,
			map[string][]byte,
		) (analysis.PackageResult, error) {
			return want, sentinel
		},
	}
	result, err := backend.analyzePackageSnapshot(context.Background(), lintPackageTask{}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("analyzePackageSnapshot() error = %v", err)
	}
	if !slices.Equal(result.RootPackagePaths, want.RootPackagePaths) {
		t.Fatalf("retained root package paths = %q", result.RootPackagePaths)
	}
}

func TestLSPWorkspaceInvalidatesReverseDependentPackage(t *testing.T) {
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
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	consumerPath := filepath.Join(root, "consumer", "consumer.go")
	if err := os.MkdirAll(filepath.Dir(dependencyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(consumerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	dependency := "package dependency\n\nfunc Value() int { return 1 }\n"
	consumer := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc Value() int { return dependency.Value() }\n"
	writeChangedCLIFile(t, dependencyPath, dependency)
	writeChangedCLIFile(t, consumerPath, consumer)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			runs[task.patterns[0]]++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(dependencyPath),
			Path: dependencyPath,
			Version: 1,
			Text: []byte(dependency),
		},
		{
			URI: "file://" + filepath.ToSlash(consumerPath),
			Path: consumerPath,
			Version: 1,
			Text: []byte(consumer),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	documents[0].Version = 2
	documents[0].Text = []byte("package dependency\n\nfunc Value() int { return 2 }\n")
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	dependencyPattern := "file=" + dependencyPath
	consumerPattern := "file=" + consumerPath
	if runs[dependencyPattern] != 2 || runs[consumerPattern] != 2 {
		t.Fatalf(
			"package runs = %#v, want dependency and reverse dependant invalidated",
			runs,
		)
	}
}

func TestLSPWorkspaceInvalidatesReverseDependentAfterDiskSourceChange(t *testing.T) {
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
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	helperPath := filepath.Join(root, "dependency", "helper.go")
	consumerPath := filepath.Join(root, "consumer", "consumer.go")
	if err := os.MkdirAll(filepath.Dir(dependencyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(consumerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	dependency := "package dependency\n\nfunc Value() int { return helper() }\n"
	consumer := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc Value() int { return dependency.Value() }\n"
	writeChangedCLIFile(t, dependencyPath, dependency)
	writeChangedCLIFile(t, helperPath, "package dependency\n\nfunc helper() int { return 1 }\n")
	writeChangedCLIFile(t, consumerPath, consumer)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			runs[task.patterns[0]]++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(dependencyPath),
			Path: dependencyPath,
			Version: 1,
			Text: []byte(dependency),
		},
		{
			URI: "file://" + filepath.ToSlash(consumerPath),
			Path: consumerPath,
			Version: 1,
			Text: []byte(consumer),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(t, helperPath, "package dependency\n\nfunc helper() int { return 2 }\n")
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	dependencyPattern := "file=" + dependencyPath
	consumerPattern := "file=" + consumerPath
	if runs[dependencyPattern] != 2 || runs[consumerPattern] != 2 {
		t.Fatalf(
			"package runs = %#v, want disk-changed dependency and reverse dependant invalidated",
			runs,
		)
	}
}

func TestLSPWorkspaceWatchInvalidatesPackageAndReverseDependent(t *testing.T) {
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
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	helperPath := filepath.Join(root, "dependency", "helper.go")
	consumerPath := filepath.Join(root, "consumer", "consumer.go")
	unrelatedPath := filepath.Join(root, "unrelated", "unrelated.go")
	if err := os.MkdirAll(filepath.Dir(dependencyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(consumerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(unrelatedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	dependency := "package dependency\n\nfunc Value() int { return helper() }\n"
	consumer := "package consumer\n\nimport \"example.com/editor/dependency\"\n\nfunc Value() int { return dependency.Value() }\n"
	writeChangedCLIFile(t, dependencyPath, dependency)
	writeChangedCLIFile(t, helperPath, "package dependency\n\nfunc helper() int { return 1 }\n")
	writeChangedCLIFile(t, consumerPath, consumer)
	writeChangedCLIFile(t, unrelatedPath, "package unrelated\n\nconst Value = 1\n")
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	runs := make(map[string]int)
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			runs[task.patterns[0]]++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(dependencyPath),
			Path: dependencyPath,
			Version: 1,
			Text: []byte(dependency),
		},
		{
			URI: "file://" + filepath.ToSlash(consumerPath),
			Path: consumerPath,
			Version: 1,
			Text: []byte(consumer),
		},
		{
			URI: "file://" + filepath.ToSlash(unrelatedPath),
			Path: unrelatedPath,
			Version: 1,
			Text: []byte("package unrelated\n\nconst Value = 1\n"),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	if err := backend.WorkspaceFilesChanged(context.Background(), []string{helperPath});
		err != nil {
		t.Fatal(err)
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	dependencyPattern := "file=" + dependencyPath
	consumerPattern := "file=" + consumerPath
	unrelatedPattern := "file=" + unrelatedPath
	if runs[dependencyPattern] != 2 ||
		runs[consumerPattern] != 2 ||
		runs[unrelatedPattern] != 1 {
		t.Fatalf(
			"package runs = %#v, want watched dependency and reverse dependant invalidated",
			runs,
		)
	}
}

func TestLSPWorkspaceWatchOverflowInvalidatesAllPackages(t *testing.T) {
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
	firstPath := filepath.Join(root, "first", "first.go")
	secondPath := filepath.Join(root, "second", "second.go")
	if err := os.MkdirAll(filepath.Dir(firstPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(secondPath), 0o755); err != nil {
		t.Fatal(err)
	}
	first := "package first\n\nconst Value = 1\n"
	second := "package second\n\nconst Value = 2\n"
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
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	changes := make([]string, maximumLSPWorkspaceChangedFiles + 1)
	for index := range changes {
		changes[index] = filepath.Join(root, "changes", fmt.Sprintf("%d.go", index))
	}
	if err := backend.WorkspaceFilesChanged(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	if runs != 4 {
		t.Fatalf("package runs = %d, want every package invalidated after overflow", runs)
	}
}

func TestLSPWorkspaceWatchRetainsChangesAfterCanceledAnalysis(t *testing.T) {
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
	helperPath := filepath.Join(root, "helper.go")
	text := "package editor\n\nfunc Value() int { return helper() }\n"
	writeChangedCLIFile(t, path, text)
	writeChangedCLIFile(t, helperPath, "package editor\n\nfunc helper() int { return 1 }\n")
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
			URI: "file://" + filepath.ToSlash(path),
			Path: path,
			Version: 1,
			Text: []byte(text),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	if err := backend.WorkspaceFilesChanged(context.Background(), []string{helperPath});
		err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.AnalyzeWorkspace(canceled, documents);
		!errors.Is(err, context.Canceled) {
		t.Fatalf("canceled workspace analysis error = %v", err)
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("package runs = %d, want watched change retained after cancellation", runs)
	}
}

func TestLSPWorkspaceWatchDoesNotWaitForCanceledAnalysis(t *testing.T) {
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
	text := "package editor\n\nvar Ready = true\n"
	writeChangedCLIFile(t, path, text)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			_ *rules.Registry,
			_ lintPackageTask,
			_ map[string][]byte,
		) (analysis.PackageResult, error) {
			close(started)
			<-ctx.Done()
			return analysis.PackageResult{}, ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	analysisDone := make(chan error, 1)
	go func() {
		results, analysisErr := backend.AnalyzeWorkspace(
			ctx,
			[]lsp.Document{
				{
					URI: "file://" + filepath.ToSlash(path),
					Path: path,
					Version: 1,
					Text: []byte(text),
				},
			},
		)
		if analysisErr == nil && len(results) == 1 {
			analysisErr = results[0].Err
		}
		analysisDone <- analysisErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for package analysis")
	}
	changedDone := make(chan error, 1)
	go func() {
		changedDone <- backend.WorkspaceFilesChanged(context.Background(), []string{path})
	}()
	var changedErr error
	returnedPromptly := false
	select {
	case changedErr = <-changedDone:
		returnedPromptly = true
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	if analysisErr := <-analysisDone; !errors.Is(analysisErr, context.Canceled) {
		t.Fatalf("workspace analysis error = %v", analysisErr)
	}
	if !returnedPromptly {
		changedErr = <-changedDone
	}
	if changedErr != nil {
		t.Fatal(changedErr)
	}
	if !returnedPromptly {
		t.Fatal("watched-file invalidation waited for package analysis")
	}
}

func TestLSPWorkspaceInvalidatesChangedDiskPackageSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "source.go")
	helperPath := filepath.Join(root, "helper.go")
	sourceText := "package editor\n\nfunc Value() int { return helper() }\n"
	writeChangedCLIFile(t, path, sourceText)
	writeChangedCLIFile(t, helperPath, "package editor\n\nfunc helper() int { return 1 }\n")
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
			URI: "file://" + filepath.ToSlash(path),
			Path: path,
			Version: 1,
			Text: []byte(sourceText),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(t, helperPath, "package editor\n\nfunc helper() int { return 2 }\n")
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(root, "added.go"),
		"package editor\n\nconst Added = true\n",
	)
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n\n// refreshed\n",
	)
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	writeChangedCLIFile(t, filepath.Join(root, "go.work"), "go 1.26.0\n\nuse .\n")
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		t.Fatal(err)
	}
	if runs != 5 {
		t.Fatalf(
			"package runs = %d, want reload after source, directory, and module/workspace changes",
			runs,
		)
	}
	statistics := backend.packageSession.Statistics()
	if statistics.FullLoads != 4 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want sibling recheck and structural fallbacks",
			statistics,
		)
	}
}

func TestLSPWorkspacePackageCacheRetainsMostRecentBoundedEntries(t *testing.T) {
	t.Parallel()

	entries := make(map[lspPackageGroupKey]lspWorkspacePackageEntry)
	for index := 0; index < maximumLSPWorkspacePackageEntries + 2; index++ {
		key := lspPackageGroupKey{
			root: "/workspace",
			packageDirectory: fmt.Sprintf("/workspace/package-%02d", index),
		}
		entries[key] = lspWorkspacePackageEntry{used: uint64(index + 1)}
	}
	bounded := boundLSPWorkspaceEntries(entries)
	if len(bounded) != maximumLSPWorkspacePackageEntries {
		t.Fatalf(
			"bounded entries = %d, want %d",
			len(bounded),
			maximumLSPWorkspacePackageEntries,
		)
	}
	for index := 0; index < 2; index++ {
		key := lspPackageGroupKey{
			root: "/workspace",
			packageDirectory: fmt.Sprintf("/workspace/package-%02d", index),
		}
		if _, found := bounded[key]; found {
			t.Fatalf("least-recently-used entry %q was retained", key.packageDirectory)
		}
	}
}

func TestLSPWorkspacePackageCacheEvictsToRetainedByteBudget(t *testing.T) {
	t.Parallel()

	olderKey := lspPackageGroupKey{root: "/workspace", packageDirectory: "/workspace/older"}
	newerKey := lspPackageGroupKey{root: "/workspace", packageDirectory: "/workspace/newer"}
	entries := map[lspPackageGroupKey]lspWorkspacePackageEntry{
		olderKey: {accountedBytes: maximumLSPWorkspaceAccountedBytes * 3 / 4, used: 1},
		newerKey: {accountedBytes: maximumLSPWorkspaceAccountedBytes * 3 / 4, used: 2},
	}

	bounded := boundLSPWorkspaceEntries(entries)
	if len(bounded) != 1 {
		t.Fatalf("bounded entries = %d, want 1 within retained-byte budget", len(bounded))
	}
	if _, found := bounded[newerKey]; !found {
		t.Fatal("most-recently-used entry was evicted before an older entry")
	}

	entries[newerKey] = lspWorkspacePackageEntry{
		accountedBytes: maximumLSPWorkspaceAccountedBytes + 1,
		used: 2,
	}
	bounded = boundLSPWorkspaceEntries(entries)
	if len(bounded) != 1 {
		t.Fatalf("oversized bounded entries = %d, want newest entry alone", len(bounded))
	}
	if _, found := bounded[newerKey]; !found {
		t.Fatal("newest oversized entry was not retained alone")
	}
}

func TestLSPWorkspacePackageCacheAccountsIndexedSourceWeight(t *testing.T) {
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
	text := "package editor\n\nconst Value = 1\n"
	writeChangedCLIFile(t, path, text)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	if _, err := backend.AnalyzeWorkspace(
		context.Background(),
		[]lsp.Document{
			{
				URI: "file://" + filepath.ToSlash(path),
				Path: path,
				Version: 1,
				Text: []byte(text),
			},
		},
	);
		err != nil {
		t.Fatal(err)
	}
	if len(backend.workspace.entries) != 1 {
		t.Fatalf("workspace entries = %d, want 1", len(backend.workspace.entries))
	}
	for _, entry := range backend.workspace.entries {
		if entry.accountedBytes <= int64(len(text)) {
			t.Fatalf(
				"accounted bytes = %d, want indexed source weight above %d",
				entry.accountedBytes,
				len(text),
			)
		}
	}
}

func BenchmarkLSPWorkspaceSamePackageIncremental(b *testing.B) {
	root := b.TempDir()
	writeChangedCLIFile(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	writeChangedCLIFile(
		b,
		filepath.Join(root, ".glippy.toml"),
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nself-assignment = \"warn\"\n",
	)
	path := filepath.Join(root, "source.go")
	sources := []string{
		"package sample\n\nfunc value() int { return 1 }\n",
		"package sample\n\nfunc value() int { return 2 }\n",
	}
	writeChangedCLIFile(b, path, sources[0])
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	backend := &lspBackend{registry: registry}
	document := lsp.Document{
		URI: "file://" + filepath.ToSlash(path),
		Path: path,
		Version: 1,
		Text: []byte(sources[0]),
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), []lsp.Document{document});
		err != nil {
		b.Fatal(err)
	}
	before := backend.packageSession.Statistics()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		document.Version++
		document.Text = []byte(sources[(index + 1) % len(sources)])
		if _, err := backend.AnalyzeWorkspace(
			context.Background(),
			[]lsp.Document{document},
		);
			err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	after := backend.packageSession.Statistics()
	fullLoads := after.FullLoads - before.FullLoads
	incrementalLoads := after.IncrementalLoads - before.IncrementalLoads
	if fullLoads != 0 || incrementalLoads != uint64(b.N) {
		b.Fatalf(
			"package session loads = full %d, incremental %d; want full 0, incremental %d",
			fullLoads,
			incrementalLoads,
			b.N,
		)
	}
	b.ReportMetric(float64(fullLoads) / float64(b.N), "full-package-loads/op")
	b.ReportMetric(float64(incrementalLoads) / float64(b.N), "incremental-loads/op")
}

func BenchmarkLSPWorkspaceUnrelatedDocumentChange(b *testing.B) {
	root := b.TempDir()
	writeChangedCLIFile(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/editor\n\ngo 1.26.0\n",
	)
	leftPath := filepath.Join(root, "left", "left.go")
	rightPath := filepath.Join(root, "right", "right.go")
	if err := os.MkdirAll(filepath.Dir(leftPath), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rightPath), 0o755); err != nil {
		b.Fatal(err)
	}
	leftSources := [2]string{
		"package left\n\nfunc Value() int { return 1 }\n",
		"package left\n\nfunc Value() int { return 2 }\n",
	}
	right := "package right\n\nfunc Value() int { return 1 }\n"
	writeChangedCLIFile(b, leftPath, leftSources[0])
	writeChangedCLIFile(b, rightPath, right)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	packageRuns := 0
	backend := &lspBackend{
		registry: registry,
		runPackageAnalysis: func(
			ctx context.Context,
			registry *rules.Registry,
			task lintPackageTask,
			overlay map[string][]byte,
		) (analysis.PackageResult, error) {
			packageRuns++
			return runPackageAnalysisWithOverlay(ctx, registry, task, overlay)
		},
	}
	documents := []lsp.Document{
		{
			URI: "file://" + filepath.ToSlash(leftPath),
			Path: leftPath,
			Version: 1,
			Text: []byte(leftSources[0]),
		},
		{
			URI: "file://" + filepath.ToSlash(rightPath),
			Path: rightPath,
			Version: 1,
			Text: []byte(right),
		},
	}
	if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		documents[0].Version++
		documents[0].Text = []byte(leftSources[(index + 1) % len(leftSources)])
		if _, err := backend.AnalyzeWorkspace(context.Background(), documents); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	measuredRuns := packageRuns - 2
	if measuredRuns != b.N {
		b.Fatalf("measured package runs = %d, want %d", measuredRuns, b.N)
	}
	b.ReportMetric(float64(measuredRuns) / float64(b.N), "package-loads/op")
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
