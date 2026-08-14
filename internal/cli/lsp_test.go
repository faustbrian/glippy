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
