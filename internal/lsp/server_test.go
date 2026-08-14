package lsp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/faustbrian/glippy/internal/lsp"
	"github.com/faustbrian/glippy/internal/source"
)

func TestServePublishesUTF16DiagnosticsAndVersionedCodeActions(t *testing.T) {
	t.Parallel()

	text := "package sample\nfunc run() { _ = \"😀\" }\n"
	emoji := strings.Index(text, "😀")
	backend := &testBackend{
		analysis: lsp.Analysis{
			Diagnostics: []lsp.Diagnostic{
				{
					Range: source.Range{Start: emoji, End: emoji + len("😀")},
					Severity: lsp.SeverityWarning,
					Code: "emoji-rule",
					Message: "review emoji",
				},
			},
		},
		actions: []lsp.CodeAction{
			{
				Title: "emoji-rule: replace emoji",
				Kind: "quickfix",
				Preferred: true,
				DiagnosticCode: "emoji-rule",
				DiagnosticRange: source.Range{Start: emoji, End: emoji + len("😀")},
				DiagnosticSeverity: lsp.SeverityWarning,
				DiagnosticMessage: "review emoji",
				NewText: []byte("package sample\nfunc run() {}\n"),
			},
		},
		formatted: []byte("package sample\n\nfunc run() { _ = \"😀\" }\n"),
	}
	input := framedMessages(
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
					"uri": "file:///project/source.go",
					"languageId": "go",
					"version": 7,
					"text": text,
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "textDocument/codeAction",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
				"range": map[string]any{
					"start": map[string]any{"line": 1, "character": 0},
					"end": map[string]any{"line": 2, "character": 0},
				},
				"context": map[string]any{"diagnostics": []any{}},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 3,
			"method": "textDocument/formatting",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
				"options": map[string]any{"tabSize": 8, "insertSpaces": false},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var output bytes.Buffer
	if err := lsp.Serve(context.Background(), bytes.NewReader(input), &output, backend);
		err != nil {
		t.Fatal(err)
	}
	messages := decodeFrames(t, output.Bytes())
	if len(messages) != 5 {
		t.Fatalf("Serve() emitted %d messages:\n%s", len(messages), output.Bytes())
	}

	initialize := messages[0]
	capabilities := initialize["result"].(map[string]any)["capabilities"].(map[string]any)
	if capabilities["textDocumentSync"] != float64(1) ||
		capabilities["codeActionProvider"] != true ||
		capabilities["documentFormattingProvider"] != true {
		t.Fatalf("initialize capabilities = %#v", capabilities)
	}
	published := messages[1]
	params := published["params"].(map[string]any)
	diagnostics := params["diagnostics"].([]any)
	if params["version"] != float64(7) || len(diagnostics) != 1 {
		t.Fatalf("publishDiagnostics params = %#v", params)
	}
	range_ := diagnostics[0].(map[string]any)["range"].(map[string]any)
	start := range_["start"].(map[string]any)
	end := range_["end"].(map[string]any)
	if start["line"] != float64(1) ||
		start["character"] != float64(18) ||
		end["line"] != float64(1) ||
		end["character"] != float64(20) {
		t.Fatalf("UTF-16 diagnostic range = %#v", range_)
	}

	actions := messages[2]["result"].([]any)
	if len(actions) != 1 {
		t.Fatalf("code actions = %#v", actions)
	}
	action := actions[0].(map[string]any)
	changes := action["edit"].(map[string]any)["documentChanges"].([]any)
	documentEdit := changes[0].(map[string]any)
	versioned := documentEdit["textDocument"].(map[string]any)
	if versioned["version"] != float64(7) || versioned["uri"] != "file:///project/source.go" {
		t.Fatalf("versioned code action = %#v", documentEdit)
	}

	formatEdits := messages[3]["result"].([]any)
	if len(formatEdits) != 1 ||
		formatEdits[0].(map[string]any)["newText"] != string(backend.formatted) {
		t.Fatalf("format edits = %#v", formatEdits)
	}
	shutdownResult, present := messages[4]["result"]
	if !present || shutdownResult != nil {
		t.Fatalf("shutdown response = %#v", messages[4])
	}
}

func TestServeReanalyzesFullDocumentChangesAndClearsClosedDiagnostics(t *testing.T) {
	t.Parallel()

	backend := &testBackend{
		analysisFor: func(document lsp.Document) lsp.Analysis {
			start := strings.Index(string(document.Text), "changed")
			if start < 0 {
				return lsp.Analysis{}
			}
			return lsp.Analysis{
				Diagnostics: []lsp.Diagnostic{
					{
						Range: source.Range{
							Start: start,
							End: start + len("changed"),
						},
						Severity: lsp.SeverityWarning,
						Code: "changed-package",
						Message: "changed package name",
					},
				},
			}
		},
	}
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package sample\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didChange",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 2,
				},
				"contentChanges": []any{
					map[string]any{"text": "package changed\n"},
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didClose",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
			},
		},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var output bytes.Buffer
	err := lsp.Serve(context.Background(), bytes.NewReader(input), &output, backend)
	if err != nil {
		t.Fatal(err)
	}
	messages := decodeFrames(t, output.Bytes())
	if len(messages) != 5 {
		t.Fatalf("Serve() emitted %d messages:\n%s", len(messages), output.Bytes())
	}
	changed := messages[2]["params"].(map[string]any)
	if changed["version"] != float64(2) || len(changed["diagnostics"].([]any)) != 1 {
		t.Fatalf("changed diagnostics = %#v", changed)
	}
	closed := messages[3]["params"].(map[string]any)
	if _, present := closed["version"]; present || len(closed["diagnostics"].([]any)) != 0 {
		t.Fatalf("closed diagnostics = %#v", closed)
	}
}

func TestServeRejectsIncrementalDocumentChanges(t *testing.T) {
	t.Parallel()

	backend := &testBackend{}
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package sample\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didChange",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 2,
				},
				"contentChanges": []any{
					map[string]any{
						"range": map[string]any{},
						"text": "package changed\n",
					},
				},
			},
		},
	)
	var output bytes.Buffer
	err := lsp.Serve(context.Background(), bytes.NewReader(input), &output, backend)
	if err == nil || !strings.Contains(err.Error(), "full document") {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServeRejectsStaleDocumentVersions(t *testing.T) {
	t.Parallel()

	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 4,
					"text": "package sample\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didChange",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 4,
				},
				"contentChanges": []any{map[string]any{"text": "package stale\n"}},
			},
		},
	)
	var output bytes.Buffer
	err := lsp.Serve(context.Background(), bytes.NewReader(input), &output, &testBackend{})
	if err == nil || !strings.Contains(err.Error(), "newer document version") {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServeRejectsAmbiguousAndOversizedHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		want string
	}{
		{
			name: "duplicate content length",
			input: "Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}",
			want: "duplicate Content-Length",
		},
		{
			name: "oversized header",
			input: strings.Repeat("X", 5000) + ": value\r\n\r\n",
			want: "header line exceeds",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.name,
			func(t *testing.T) {
				var output bytes.Buffer
				err := lsp.Serve(
					context.Background(),
					strings.NewReader(test.input),
					&output,
					&testBackend{},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("Serve() error = %v, want %q", err, test.want)
				}
			},
		)
	}
}

func TestServeDoesNotPublishAnalysisCompletedAfterCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	backend := &testBackend{
		cancel: cancel,
		analysis: lsp.Analysis{
			Diagnostics: []lsp.Diagnostic{
				{
					Range: source.Range{},
					Severity: lsp.SeverityWarning,
					Code: "late-result",
					Message: "must not publish",
				},
			},
		},
	}
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package sample\n",
				},
			},
		},
	)
	var output bytes.Buffer
	err := lsp.Serve(ctx, bytes.NewReader(input), &output, backend)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve() error = %v", err)
	}
	if strings.Contains(output.String(), "publishDiagnostics") ||
		strings.Contains(output.String(), "late-result") {
		t.Fatalf("Serve() published canceled analysis: %s", output.Bytes())
	}
}

func TestServeCancelsAnActiveRequestWithoutEndingTheSession(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	backend := &testBackend{actionStarted: started}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	var output bytes.Buffer
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, &output, backend)
	}()
	first := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package sample\n",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "textDocument/codeAction",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
				"range": map[string]any{
					"start": map[string]any{"line": 0, "character": 0},
					"end": map[string]any{"line": 0, "character": 0},
				},
			},
		},
	)
	remaining := framedMessages(
		t,
		map[string]any{
			"jsonrpc": "2.0",
			"method": "$/cancelRequest",
			"params": map[string]any{"id": 2},
		},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	go func() {
		if _, err := writer.Write(first); err != nil {
			return
		}
		<-started
		_, _ = writer.Write(remaining)
		_ = writer.Close()
	}()
	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	messages := decodeFrames(t, output.Bytes())
	foundCancellation := false
	for _, message := range messages {
		if message["id"] != float64(2) {
			continue
		}
		if _, present := message["result"]; present {
			t.Fatalf("canceled response contains both result and error: %#v", message)
		}
		error_, _ := message["error"].(map[string]any)
		if error_["code"] == float64(-32800) {
			foundCancellation = true
		}
	}
	if !foundCancellation {
		t.Fatalf("Serve() did not return request-canceled response: %#v", messages)
	}
}

type testBackend struct {
	analysis lsp.Analysis
	analysisFor func(lsp.Document) lsp.Analysis
	actions []lsp.CodeAction
	formatted []byte
	cancel context.CancelFunc
	actionStarted chan struct{}
}

func (b *testBackend) Analyze(_ context.Context, document lsp.Document) (lsp.Analysis, error) {
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return lsp.Analysis{}, err
	}
	result := b.analysis
	if b.analysisFor != nil {
		result = b.analysisFor(document)
	}
	if b.cancel != nil {
		b.cancel()
	}
	result.File = file
	return result, nil
}

func (b *testBackend) CodeActions(
	ctx context.Context,
	_ lsp.Document,
	_ lsp.Analysis,
	_ source.Range,
) ([]lsp.CodeAction, error) {
	if b.actionStarted != nil {
		close(b.actionStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return append([]lsp.CodeAction(nil), b.actions...), nil
}

func (b *testBackend) Format(_ context.Context, _ lsp.Document) ([]byte, error) {
	return append([]byte(nil), b.formatted...), nil
}

func framedMessages(t *testing.T, messages ...map[string]any) []byte {
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

func decodeFrames(t *testing.T, input []byte) []map[string]any {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(input))
	result := make([]map[string]any, 0)
	for {
		length := 0
		for {
			line, err := reader.ReadString('\n')
			if err == io.EOF && line == "" {
				return result
			}
			if err != nil {
				t.Fatal(err)
			}
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if line == "" {
				break
			}
			name, value, found := strings.Cut(line, ":")
			if found && strings.EqualFold(name, "Content-Length") {
				length, err = strconv.Atoi(strings.TrimSpace(value))
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatal(err)
		}
		result = append(result, message)
	}
}
