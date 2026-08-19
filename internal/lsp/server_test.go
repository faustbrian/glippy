package lsp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
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
					WithheldFixes: []lsp.WithheldFix{
						{
							Name: "replace-commented-emoji",
							Reason: "comments",
							Message: "replacing the emoji would remove comments",
						},
					},
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
				DiagnosticWithheldFixes: []lsp.WithheldFix{
					{
						Name: "replace-commented-emoji",
						Reason: "comments",
						Message: "replacing the emoji would remove comments",
					},
				},
				NewText: []byte("package sample\nfunc run() {}\n"),
			},
		},
		formatted: []byte("package sample\n\nfunc run() { _ = \"😀\" }\n"),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
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
	)
	if !output.waitContains(ctx, []byte("emoji-rule")) ||
		!output.waitContains(ctx, []byte(`"version":7`)) {
		t.Fatalf("initial diagnostics were not published: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
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
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	messages := decodeFrames(t, output.bytes())
	if len(messages) != 5 {
		t.Fatalf("Serve() emitted %d messages:\n%s", len(messages), output.bytes())
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
	data := diagnostics[0].(map[string]any)["data"].(map[string]any)
	withheld := data["withheld_fixes"].([]any)
	if len(withheld) != 1 ||
		withheld[0].(map[string]any)["name"] != "replace-commented-emoji" ||
		withheld[0].(map[string]any)["reason"] != "comments" ||
		withheld[0].(map[string]any)["message"] !=
			"replacing the emoji would remove comments" {
		t.Fatalf("withheld fix diagnostic data = %#v", data)
	}

	actions := messages[2]["result"].([]any)
	if len(actions) != 1 {
		t.Fatalf("code actions = %#v", actions)
	}
	action := actions[0].(map[string]any)
	actionDiagnostic := action["diagnostics"].([]any)[0].(map[string]any)
	if _, found := actionDiagnostic["data"]; !found {
		t.Fatalf("code action diagnostic omitted withheld fix data: %#v", actionDiagnostic)
	}
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
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
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
	if !output.waitContains(ctx, []byte(`"version":1`)) {
		t.Fatalf("opened diagnostics were not published: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
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
	)
	if !output.waitContains(ctx, []byte(`"version":2`)) ||
		!output.waitContains(ctx, []byte("changed-package")) {
		t.Fatalf("changed diagnostics were not published: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
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
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	messages := decodeFrames(t, output.bytes())
	if len(messages) != 5 {
		t.Fatalf("Serve() emitted %d messages:\n%s", len(messages), output.bytes())
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
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	var output bytes.Buffer
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- lsp.Serve(ctx, reader, &output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
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
	err := <-serveDone
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve() error = %v", err)
	}
	if strings.Contains(output.String(), "publishDiagnostics") ||
		strings.Contains(output.String(), "late-result") {
		t.Fatalf("Serve() published canceled analysis: %s", output.Bytes())
	}
}

func TestServeCancelsSupersededAnalysisAndPublishesOnlyNewestVersion(t *testing.T) {
	t.Parallel()

	backend := &supersededAnalysisBackend{
		started: make(chan int, 4),
		canceled: make(chan int, 4),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package first\n",
				},
			},
		},
	)
	if version := receiveVersion(t, backend.started); version != 1 {
		t.Fatalf("first analysis version = %d, want 1", version)
	}
	writeLSPMessages(
		t,
		writer,
		changedDocumentMessage(2, "package second\n"),
		changedDocumentMessage(3, "package newest\n"),
	)
	if version := receiveVersion(t, backend.canceled); version != 1 {
		t.Fatalf("canceled analysis version = %d, want 1", version)
	}
	if version := receiveVersion(t, backend.started); version != 3 {
		t.Fatalf("next analysis version = %d, want debounced version 3", version)
	}
	if !output.waitContains(ctx, []byte(`"version":3`)) ||
		!output.waitContains(ctx, []byte("newest-version")) {
		t.Fatalf("newest diagnostics were not published: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	for _, message := range decodeFrames(t, output.bytes()) {
		if message["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params := message["params"].(map[string]any)
		if params["version"] != float64(3) {
			t.Fatalf("published superseded diagnostics: %#v", params)
		}
	}
}

func TestServeRejectsQueuedCodeActionWhenAnalysisIsSuperseded(t *testing.T) {
	t.Parallel()

	backend := &supersededAnalysisBackend{
		started: make(chan int, 4),
		canceled: make(chan int, 4),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package first\n",
				},
			},
		},
	)
	if version := receiveVersion(t, backend.started); version != 1 {
		t.Fatalf("first analysis version = %d, want 1", version)
	}
	writeLSPMessages(
		t,
		writer,
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
		changedDocumentMessage(3, "package newest\n"),
	)
	if !output.waitContains(ctx, []byte(`"code":-32801`)) {
		t.Fatalf("queued action was not rejected as content-modified: %s", output.bytes())
	}
	if version := receiveVersion(t, backend.canceled); version != 1 {
		t.Fatalf("canceled analysis version = %d, want 1", version)
	}
	if version := receiveVersion(t, backend.started); version != 3 {
		t.Fatalf("next analysis version = %d, want 3", version)
	}
	if !output.waitContains(ctx, []byte(`"version":3`)) {
		t.Fatalf("newest diagnostics were not published: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range decodeFrames(t, output.bytes()) {
		if message["id"] != float64(2) {
			continue
		}
		error_, _ := message["error"].(map[string]any)
		if error_["code"] == float64(-32801) {
			found = true
		}
	}
	if !found {
		t.Fatalf("content-modified response missing: %s", output.bytes())
	}
}

func TestServeHonorsQueuedCodeActionCancellationBeforeSupersession(t *testing.T) {
	t.Parallel()

	backend := &supersededAnalysisBackend{
		started: make(chan int, 4),
		canceled: make(chan int, 4),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package first\n",
				},
			},
		},
	)
	if version := receiveVersion(t, backend.started); version != 1 {
		t.Fatalf("first analysis version = %d, want 1", version)
	}
	writeLSPMessages(
		t,
		writer,
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
		map[string]any{
			"jsonrpc": "2.0",
			"method": "$/cancelRequest",
			"params": map[string]any{"id": 2},
		},
		changedDocumentMessage(3, "package newest\n"),
	)
	if !output.waitContains(ctx, []byte(`"code":-32800`)) {
		t.Fatalf("queued action cancellation was not honored: %s", output.bytes())
	}
	if version := receiveVersion(t, backend.canceled); version != 1 {
		t.Fatalf("canceled analysis version = %d, want 1", version)
	}
	if version := receiveVersion(t, backend.started); version != 3 {
		t.Fatalf("next analysis version = %d, want 3", version)
	}
	if !output.waitContains(ctx, []byte(`"version":3`)) {
		t.Fatalf("newest diagnostics were not published: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	for _, message := range decodeFrames(t, output.bytes()) {
		if message["id"] != float64(2) {
			continue
		}
		error_, _ := message["error"].(map[string]any)
		if error_["code"] != float64(-32800) {
			t.Fatalf("queued action response = %#v, want request canceled", message)
		}
	}
}

func TestServeRejectsQueuedCodeActionWhenFinalDocumentCloses(t *testing.T) {
	t.Parallel()

	backend := &supersededAnalysisBackend{
		started: make(chan int, 4),
		canceled: make(chan int, 4),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package first\n",
				},
			},
		},
	)
	if version := receiveVersion(t, backend.started); version != 1 {
		t.Fatalf("first analysis version = %d, want 1", version)
	}
	writeLSPMessages(
		t,
		writer,
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
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didClose",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
			},
		},
	)
	if version := receiveVersion(t, backend.canceled); version != 1 {
		t.Fatalf("canceled analysis version = %d, want 1", version)
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}

	foundClear := false
	foundContentModified := false
	for _, message := range decodeFrames(t, output.bytes()) {
		if message["method"] == "textDocument/publishDiagnostics" {
			params := message["params"].(map[string]any)
			if params["uri"] == "file:///project/source.go" &&
				len(params["diagnostics"].([]any)) == 0 {
				foundClear = true
			}
		}
		if message["id"] == float64(2) {
			error_, _ := message["error"].(map[string]any)
			if error_["code"] == float64(-32801) {
				foundContentModified = true
			}
		}
	}
	if !foundClear {
		t.Fatalf("closed document diagnostics were not cleared: %s", output.bytes())
	}
	if !foundContentModified {
		t.Fatalf("queued action was not rejected as content-modified: %s", output.bytes())
	}
}

func TestServeDrainsShutdownWhenAnalysisSnapshotIsStale(t *testing.T) {
	t.Parallel()

	backend := &staleWorkspaceBackend{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package current\n",
				},
			},
		},
	)
	select {
	case <-backend.started:
	case <-ctx.Done():
		t.Fatal("timed out waiting for workspace analysis")
	}
	writeLSPMessages(
		t,
		writer,
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
		map[string]any{
			"jsonrpc": "2.0",
			"id": 4,
			"method": "textDocument/formatting",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
			},
		},
	)
	if !output.waitContains(ctx, []byte(`"id":4`)) {
		t.Fatalf("formatting barrier response missing: %s", output.bytes())
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
		map[string]any{
			"jsonrpc": "2.0",
			"id": 5,
			"method": "textDocument/formatting",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
			},
		},
	)
	if !output.waitContains(ctx, []byte(`"id":5`)) ||
		!output.waitContains(ctx, []byte(`"code":-32002`)) {
		t.Fatalf("shutdown barrier response missing: %s", output.bytes())
	}
	writeLSPMessages(t, writer, map[string]any{"jsonrpc": "2.0", "method": "exit"})
	close(backend.release)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}

	foundContentModified := false
	foundShutdown := false
	for _, message := range decodeFrames(t, output.bytes()) {
		if message["id"] == float64(2) {
			error_, _ := message["error"].(map[string]any)
			if error_["code"] == float64(-32801) {
				foundContentModified = true
			}
		}
		if message["id"] == float64(3) {
			_, foundShutdown = message["result"]
		}
	}
	if !foundContentModified {
		t.Fatalf("stale action was not rejected as content-modified: %s", output.bytes())
	}
	if !foundShutdown {
		t.Fatalf("shutdown response missing after stale analysis: %s", output.bytes())
	}
}

func TestServeDoesNotReplacePendingShutdownRequest(t *testing.T) {
	t.Parallel()

	backend := &delayedAnalysisBackend{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package current\n",
				},
			},
		},
	)
	select {
	case <-backend.started:
	case <-ctx.Done():
		t.Fatal("timed out waiting for document analysis")
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"},
	)
	if !output.waitContains(ctx, []byte(`"id":3`)) ||
		!output.waitContains(ctx, []byte(`"code":-32002`)) {
		t.Fatalf("repeated shutdown response missing: %s", output.bytes())
	}
	writeLSPMessages(t, writer, map[string]any{"jsonrpc": "2.0", "method": "exit"})
	close(backend.release)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}

	foundFirstShutdown := false
	foundSecondError := false
	for _, message := range decodeFrames(t, output.bytes()) {
		if message["id"] == float64(2) {
			_, foundFirstShutdown = message["result"]
		}
		if message["id"] == float64(3) {
			error_, _ := message["error"].(map[string]any)
			foundSecondError = error_["code"] == float64(-32002)
		}
	}
	if !foundFirstShutdown {
		t.Fatalf("first shutdown response missing: %s", output.bytes())
	}
	if !foundSecondError {
		t.Fatalf("repeated shutdown was not rejected: %s", output.bytes())
	}
}

func TestServePropagatesQueuedActionRejectionWriteFailure(t *testing.T) {
	t.Parallel()

	backend := &supersededAnalysisBackend{
		started: make(chan int, 4),
		canceled: make(chan int, 4),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	writeErr := errors.New("write failed")
	output := &failAfterWriter{remaining: 2, err: writeErr}
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didOpen",
			"params": map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///project/source.go",
					"version": 1,
					"text": "package current\n",
				},
			},
		},
	)
	if version := receiveVersion(t, backend.started); version != 1 {
		t.Fatalf("first analysis version = %d, want 1", version)
	}
	writeLSPMessages(
		t,
		writer,
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
		map[string]any{
			"jsonrpc": "2.0",
			"method": "textDocument/didClose",
			"params": map[string]any{
				"textDocument": map[string]any{"uri": "file:///project/source.go"},
			},
		},
	)
	if err := <-serveDone; !errors.Is(err, writeErr) {
		t.Fatalf("Serve() error = %v, want output failure", err)
	}
}

func TestServeCancelsAnActiveRequestWithoutEndingTheSession(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	backend := &testBackend{actionStarted: started}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
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
	)
	request := framedMessages(
		t,
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
		if !output.waitContains(ctx, []byte(`"version":1`)) {
			return
		}
		if _, err := writer.Write(request); err != nil {
			return
		}
		<-started
		_, _ = writer.Write(remaining)
		_ = writer.Close()
	}()
	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	messages := decodeFrames(t, output.bytes())
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

func TestServeReanalyzesOpenWorkspaceAfterWatchedFilesChange(t *testing.T) {
	t.Parallel()

	backend := &watchedWorkspaceBackend{
		analyses: make(chan int, 2),
		changes: make(chan []string, 1),
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	output := newWaitBuffer()
	serveDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()
	go func() {
		serveDone <- lsp.Serve(ctx, reader, output, backend)
	}()
	writeLSPMessages(
		t,
		writer,
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
	if count := receiveVersion(t, backend.analyses); count != 1 {
		t.Fatalf("initial workspace analysis = %d, want 1", count)
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{
			"jsonrpc": "2.0",
			"method": "workspace/didChangeWatchedFiles",
			"params": map[string]any{
				"changes": []any{
					map[string]any{"uri": "file:///project/z.go", "type": 1},
					map[string]any{
						"uri": "file:///project/helper.go",
						"type": 2,
					},
					map[string]any{
						"uri": "file:///project/removed.go",
						"type": 3,
					},
					map[string]any{
						"uri": "file:///project/helper.go",
						"type": 2,
					},
				},
			},
		},
	)
	select {
	case paths := <-backend.changes:
		want := []string{"/project/helper.go", "/project/removed.go", "/project/z.go"}
		if !slices.Equal(paths, want) {
			t.Fatalf("watched paths = %q", paths)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for watched-file invalidation")
	}
	if count := receiveVersion(t, backend.analyses); count != 2 {
		t.Fatalf("refreshed workspace analysis = %d, want 2", count)
	}
	writeLSPMessages(
		t,
		writer,
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
}

func TestServeRegistersWorkspaceFileWatching(t *testing.T) {
	t.Parallel()

	input := framedMessages(
		t,
		map[string]any{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "initialize",
			"params": map[string]any{
				"capabilities": map[string]any{
					"workspace": map[string]any{
						"didChangeWatchedFiles": map[string]any{
							"dynamicRegistration": true,
						},
					},
				},
			},
		},
		map[string]any{"jsonrpc": "2.0", "method": "initialized"},
		map[string]any{"jsonrpc": "2.0", "id": "glippy-watch-files", "result": nil},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var output bytes.Buffer
	if err := lsp.Serve(context.Background(), bytes.NewReader(input), &output, &testBackend{});
		err != nil {
		t.Fatal(err)
	}
	messages := decodeFrames(t, output.Bytes())
	registered := false
	for _, message := range messages {
		if message["id"] != "glippy-watch-files" ||
			message["method"] != "client/registerCapability" {
			continue
		}
		params := message["params"].(map[string]any)
		registrations := params["registrations"].([]any)
		if len(registrations) != 1 {
			t.Fatalf("registrations = %#v", registrations)
		}
		registration := registrations[0].(map[string]any)
		if registration["method"] != "workspace/didChangeWatchedFiles" {
			t.Fatalf("file watch registration = %#v", registration)
		}
		registered = true
	}
	if !registered {
		t.Fatalf("workspace file watching was not registered: %#v", messages)
	}
}

func TestServeDoesNotRegisterUnsupportedWorkspaceFileWatching(t *testing.T) {
	t.Parallel()

	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{"jsonrpc": "2.0", "method": "initialized"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var output bytes.Buffer
	if err := lsp.Serve(context.Background(), bytes.NewReader(input), &output, &testBackend{});
		err != nil {
		t.Fatal(err)
	}
	for _, message := range decodeFrames(t, output.Bytes()) {
		if message["method"] == "client/registerCapability" {
			t.Fatalf("unsupported workspace file watching was registered: %#v", message)
		}
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

type supersededAnalysisBackend struct {
	started chan int
	canceled chan int
}

type staleWorkspaceBackend struct {
	started chan struct{}
	release chan struct{}
}

type delayedAnalysisBackend struct {
	started chan struct{}
	release chan struct{}
}

type watchedWorkspaceBackend struct {
	analyses chan int
	changes chan []string
	count int
}

func (b *supersededAnalysisBackend) Analyze(
	ctx context.Context,
	document lsp.Document,
) (lsp.Analysis, error) {
	b.started <- document.Version
	if document.Version != 3 {
		<-ctx.Done()
		b.canceled <- document.Version
		return lsp.Analysis{}, ctx.Err()
	}
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return lsp.Analysis{}, err
	}
	return lsp.Analysis{
		File: file,
		Diagnostics: []lsp.Diagnostic{
			{
				Range: source.Range{},
				Severity: lsp.SeverityWarning,
				Code: "newest-version",
				Message: "newest-version",
			},
		},
	}, nil
}

func (*supersededAnalysisBackend) CodeActions(
	context.Context,
	lsp.Document,
	lsp.Analysis,
	source.Range,
) ([]lsp.CodeAction, error) {
	return nil, nil
}

func (*supersededAnalysisBackend) Format(context.Context, lsp.Document) ([]byte, error) {
	return nil, nil
}

func (*staleWorkspaceBackend) Analyze(context.Context, lsp.Document) (lsp.Analysis, error) {
	return lsp.Analysis{}, errors.New("unexpected single-document analysis")
}

func (b *staleWorkspaceBackend) AnalyzeWorkspace(
	_ context.Context,
	documents []lsp.Document,
) ([]lsp.WorkspaceAnalysis, error) {
	close(b.started)
	<-b.release
	documents[0].Text = []byte("package stale\n")
	file, err := source.Load(documents[0].Path, documents[0].Text)
	if err != nil {
		return nil, err
	}
	return []lsp.WorkspaceAnalysis{
		{Document: documents[0], Analysis: lsp.Analysis{File: file}},
	}, nil
}

func (*staleWorkspaceBackend) CodeActions(
	context.Context,
	lsp.Document,
	lsp.Analysis,
	source.Range,
) ([]lsp.CodeAction, error) {
	return nil, nil
}

func (*staleWorkspaceBackend) Format(context.Context, lsp.Document) ([]byte, error) {
	return nil, nil
}

func (b *delayedAnalysisBackend) Analyze(
	ctx context.Context,
	document lsp.Document,
) (lsp.Analysis, error) {
	close(b.started)
	select {
	case <-ctx.Done():
		return lsp.Analysis{}, ctx.Err()
	case <-b.release:
	}
	file, err := source.Load(document.Path, document.Text)
	if err != nil {
		return lsp.Analysis{}, err
	}
	return lsp.Analysis{File: file}, nil
}

func (*delayedAnalysisBackend) CodeActions(
	context.Context,
	lsp.Document,
	lsp.Analysis,
	source.Range,
) ([]lsp.CodeAction, error) {
	return nil, nil
}

func (*delayedAnalysisBackend) Format(context.Context, lsp.Document) ([]byte, error) {
	return nil, nil
}

func (*watchedWorkspaceBackend) Analyze(context.Context, lsp.Document) (lsp.Analysis, error) {
	return lsp.Analysis{}, errors.New("unexpected single-document analysis")
}

func (b *watchedWorkspaceBackend) AnalyzeWorkspace(
	_ context.Context,
	documents []lsp.Document,
) ([]lsp.WorkspaceAnalysis, error) {
	b.count++
	results := make([]lsp.WorkspaceAnalysis, len(documents))
	for index, document := range documents {
		file, err := source.Load(document.Path, document.Text)
		if err != nil {
			return nil, err
		}
		results[index] = lsp.WorkspaceAnalysis{
			Document: document,
			Analysis: lsp.Analysis{File: file},
		}
	}
	b.analyses <- b.count
	return results, nil
}

func (b *watchedWorkspaceBackend) WorkspaceFilesChanged(_ context.Context, paths []string) error {
	b.changes <- append([]string(nil), paths...)
	return nil
}

func (*watchedWorkspaceBackend) CodeActions(
	context.Context,
	lsp.Document,
	lsp.Analysis,
	source.Range,
) ([]lsp.CodeAction, error) {
	return nil, nil
}

func (*watchedWorkspaceBackend) Format(context.Context, lsp.Document) ([]byte, error) {
	return nil, nil
}

type waitBuffer struct {
	mu sync.Mutex
	changed chan struct{}
	data bytes.Buffer
}

type failAfterWriter struct {
	remaining int
	err error
}

func (w *failAfterWriter) Write(input []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(input), nil
}

func newWaitBuffer() *waitBuffer {
	return &waitBuffer{changed: make(chan struct{}, 1)}
}

func (b *waitBuffer) Write(input []byte) (int, error) {
	b.mu.Lock()
	written, err := b.data.Write(input)
	b.mu.Unlock()
	select {
	case b.changed <- struct{}{}:
	default:
	}
	return written, err
}

func (b *waitBuffer) waitContains(ctx context.Context, value []byte) bool {
	for {
		b.mu.Lock()
		found := bytes.Contains(b.data.Bytes(), value)
		b.mu.Unlock()
		if found {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-b.changed:
		}
	}
}

func (b *waitBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.data.Bytes())
}

func changedDocumentMessage(version int, text string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"method": "textDocument/didChange",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///project/source.go",
				"version": version,
			},
			"contentChanges": []any{map[string]any{"text": text}},
		},
	}
}

func writeLSPMessages(t *testing.T, writer io.Writer, messages ...map[string]any) {
	t.Helper()
	if _, err := writer.Write(framedMessages(t, messages...)); err != nil {
		t.Fatal(err)
	}
}

func receiveVersion(t *testing.T, versions <-chan int) int {
	t.Helper()
	select {
	case version := <-versions:
		return version
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for analysis version")
		return 0
	}
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

func (*testBackend) WorkspaceFilesChanged(context.Context, []string) error {
	return nil
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
