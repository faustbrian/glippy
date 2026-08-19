// Package lsp owns Glippy's bounded JSON-RPC editor protocol surface.
package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/faustbrian/glippy/internal/source"
)

const maximumMessageSize = source.MaxFileSize + 1 << 20

const maximumHeaderSize = 4 << 10

const maximumPendingCancellations = 1024

const maximumPendingAnalysisRequests = 64

const maximumWatchedFileChanges = 4096

const documentAnalysisDebounce = 20 * time.Millisecond

const watchedFilesRequestID = "glippy-watch-files"

// Severity is an LSP diagnostic severity.
type Severity int

const (
	SeverityError Severity = 1
	SeverityWarning Severity = 2
)

// Document is one exact editor-owned buffer version.
type Document struct {
	URI string
	Path string
	Version int
	Text []byte
}

// Related identifies secondary context in the same exact source version.
type Related struct {
	Range source.Range
	Message string
}

// WithheldFix is one rule-declared fix that could not be offered safely for
// the current source.
type WithheldFix struct {
	Name string `json:"name"`
	Reason string `json:"reason"`
	Message string `json:"message"`
}

// Diagnostic is one editor-facing finding over physical UTF-8 byte ranges.
type Diagnostic struct {
	Range source.Range
	Severity Severity
	Code string
	DocumentationURI string
	Message string
	Related []Related
	WithheldFixes []WithheldFix
}

// Analysis binds editor findings and backend state to one immutable source.
type Analysis struct {
	File *source.File
	Diagnostics []Diagnostic
	State any
}

// CodeAction is one validated whole-document replacement.
type CodeAction struct {
	Title string
	Kind string
	Preferred bool
	DiagnosticCode string
	DiagnosticRange source.Range
	DiagnosticSeverity Severity
	DiagnosticMessage string
	DiagnosticDocumentationURI string
	DiagnosticWithheldFixes []WithheldFix
	NewText []byte
}

// Backend reuses Glippy's formatter, analyzer, and fix transaction contracts.
type Backend interface {
	Analyze(context.Context, Document) (Analysis, error)
	CodeActions(context.Context, Document, Analysis, source.Range) ([]CodeAction, error)
	Format(context.Context, Document) ([]byte, error)
}

// WorkspaceAnalysis binds one backend result to the exact document snapshot
// that produced it.
type WorkspaceAnalysis struct {
	Document Document
	Analysis Analysis
	Err error
}

// WorkspaceBackend analyzes one immutable snapshot of every editor-owned
// buffer. Backends that do not need workspace state may implement Backend
// alone.
type WorkspaceBackend interface {
	AnalyzeWorkspace(context.Context, []Document) ([]WorkspaceAnalysis, error)
}

// WorkspaceFileBackend invalidates persistent workspace state after editor
// filesystem notifications. Paths are absolute, cleaned, sorted, and unique.
type WorkspaceFileBackend interface {
	WorkspaceFilesChanged(context.Context, []string) error
}

type rpcMessage struct {
	JSONRPC string `json:"jsonrpc"`
	ID json.RawMessage `json:"id,omitempty"`
	Method string `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcSuccessResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID json.RawMessage `json:"id"`
	Result any `json:"result"`
}

type rpcErrorResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID json.RawMessage `json:"id"`
	Error *rpcError `json:"error"`
}

type rpcError struct {
	Code int `json:"code"`
	Message string `json:"message"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID string `json:"id"`
	Method string `json:"method"`
	Params any `json:"params"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type versionedTextDocumentIdentifier struct {
	URI string `json:"uri"`
	Version int `json:"version"`
}

type textDocumentItem struct {
	URI string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version int `json:"version"`
	Text string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Range json.RawMessage `json:"range,omitempty"`
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type watchedFileChange struct {
	URI string `json:"uri"`
	Type int `json:"type"`
}

type didChangeWatchedFilesParams struct {
	Changes []watchedFileChange `json:"changes"`
}

type documentRequestParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range protocolRange `json:"range"`
}

type cancelRequestParams struct {
	ID json.RawMessage `json:"id"`
}

type initializeParams struct {
	Capabilities struct {
		Workspace struct {
			DidChangeWatchedFiles struct {
				DynamicRegistration bool `json:"dynamicRegistration"`
			} `json:"didChangeWatchedFiles"`
		} `json:"workspace"`
	} `json:"capabilities"`
}

type protocolPosition struct {
	Line int `json:"line"`
	Character int `json:"character"`
}

type protocolRange struct {
	Start protocolPosition `json:"start"`
	End protocolPosition `json:"end"`
}

type protocolDiagnostic struct {
	Range protocolRange `json:"range"`
	Severity Severity `json:"severity"`
	Code string `json:"code"`
	CodeDescription *protocolCodeDescription `json:"codeDescription,omitempty"`
	Source string `json:"source"`
	Message string `json:"message"`
	RelatedInformation []protocolRelated `json:"relatedInformation,omitempty"`
	Data *protocolDiagnosticData `json:"data,omitempty"`
}

type protocolDiagnosticData struct {
	WithheldFixes []WithheldFix `json:"withheld_fixes"`
}

type protocolCodeDescription struct {
	Href string `json:"href"`
}

type protocolRelated struct {
	Location protocolLocation `json:"location"`
	Message string `json:"message"`
}

type protocolLocation struct {
	URI string `json:"uri"`
	Range protocolRange `json:"range"`
}

type publishDiagnosticsParams struct {
	URI string `json:"uri"`
	Version *int `json:"version,omitempty"`
	Diagnostics []protocolDiagnostic `json:"diagnostics"`
}

type protocolTextEdit struct {
	Range protocolRange `json:"range"`
	NewText string `json:"newText"`
}

type protocolTextDocumentEdit struct {
	TextDocument versionedTextDocumentIdentifier `json:"textDocument"`
	Edits []protocolTextEdit `json:"edits"`
}

type protocolWorkspaceEdit struct {
	DocumentChanges []protocolTextDocumentEdit `json:"documentChanges"`
}

type protocolCodeAction struct {
	Title string `json:"title"`
	Kind string `json:"kind"`
	Diagnostics []protocolDiagnostic `json:"diagnostics,omitempty"`
	IsPreferred bool `json:"isPreferred,omitempty"`
	Edit protocolWorkspaceEdit `json:"edit"`
}

type server struct {
	ctx context.Context
	reader *bufio.Reader
	writer io.Writer
	backend Backend
	documents map[string]Document
	analyses map[string]Analysis
	requestMu sync.Mutex
	requests map[string]context.CancelFunc
	pendingCancellations map[string]struct{}
	pendingClientRequests map[string]struct{}
	analysisResults chan analysisCompletion
	analysisGeneration uint64
	analysisCancel context.CancelFunc
	pendingAnalysisRequests []rpcMessage
	shutdownID json.RawMessage
	exitAfterShutdown bool
	exitRequested bool
	initialized bool
	shutdown bool
	watchingDynamicRegistration bool
	watchingRegistered bool
}

type analysisCompletion struct {
	generation uint64
	trigger Document
	documents []Document
	analysis Analysis
	workspace []WorkspaceAnalysis
	workspaceSupported bool
	err error
}

// Serve runs one LSP 3.17-compatible stdio session until exit or EOF.
func Serve(ctx context.Context, input io.Reader, output io.Writer, backend Backend) error {
	if ctx == nil || input == nil || output == nil || backend == nil {
		return errors.New("LSP requires context, streams, and backend")
	}
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	state := &server{
		ctx: sessionCtx,
		reader: bufio.NewReader(input),
		writer: output,
		backend: backend,
		documents: make(map[string]Document),
		analyses: make(map[string]Analysis),
		requests: make(map[string]context.CancelFunc),
		pendingCancellations: make(map[string]struct{}),
		pendingClientRequests: make(map[string]struct{}),
		analysisResults: make(chan analysisCompletion, 16),
	}
	type readResult struct {
		message rpcMessage
		err error
	}
	readResults := make(chan readResult, 16)
	go func() {
		defer close(readResults)
		for {
			message, err := readMessage(state.reader)
			if err == nil && message.Method == "$/cancelRequest" {
				if cancelErr := state.cancelRequest(message.Params);
					cancelErr != nil {
					readResults <- readResult{err: cancelErr}
					return
				}
				continue
			}
			readResults <- readResult{message: message, err: err}
			if err != nil || message.Method == "exit" {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case completion := <-state.analysisResults:
			if err := state.completeAnalysis(completion); err != nil {
				return err
			}
			if state.exitRequested {
				return nil
			}
			if readResults == nil && state.analysisCancel == nil {
				return nil
			}
		case result, available := <-readResults:
			if !available {
				if state.analysisCancel != nil {
					readResults = nil
					continue
				}
				return nil
			}
			if errors.Is(result.err, io.EOF) {
				if state.analysisCancel != nil {
					readResults = nil
					continue
				}
				return nil
			}
			if result.err != nil {
				return result.err
			}
			exit, err := state.handle(result.message)
			if err != nil {
				return err
			}
			if exit {
				return nil
			}
		}
	}
}

func (s *server) handle(message rpcMessage) (bool, error) {
	if message.JSONRPC != "2.0" {
		return false, errors.New("invalid JSON-RPC message")
	}
	if message.Method == "" {
		if _, found := s.pendingClientRequests[string(message.ID)]; !found {
			return false, errors.New("invalid JSON-RPC response")
		}
		delete(s.pendingClientRequests, string(message.ID))
		return false, nil
	}
	switch message.Method {
	case "initialize":
		if len(message.ID) == 0 {
			return false, errors.New("initialize requires a request ID")
		}
		var params initializeParams
		if err := decodeParams(message.Params, &params); err != nil {
			return false, err
		}
		s.watchingDynamicRegistration = params.
			Capabilities.
			Workspace.
			DidChangeWatchedFiles.
			DynamicRegistration
		s.initialized = true
		return false, s.respond(
			message.ID,
			map[string]any{
				"capabilities": map[string]any{
					"textDocumentSync": 1,
					"codeActionProvider": true,
					"documentFormattingProvider": true,
				},
				"serverInfo": map[string]any{"name": "Glippy"},
			},
		)
	case "initialized":
		return false, s.registerWorkspaceFileWatching()
	case "$/setTrace":
		return false, nil
	case "shutdown":
		if len(message.ID) == 0 {
			return false, errors.New("shutdown requires a request ID")
		}
		if s.shutdown {
			return false, s.respondError(message.ID, -32002, "server is not available")
		}
		s.shutdown = true
		if s.analysisCancel != nil {
			s.shutdownID = bytes.Clone(message.ID)
			return false, nil
		}
		return false, s.respond(message.ID, nil)
	case "exit":
		if len(s.shutdownID) != 0 {
			s.exitAfterShutdown = true
			return false, nil
		}
		s.cancelDocumentAnalysis()
		return true, nil
	}
	if !s.initialized || s.shutdown {
		if len(message.ID) == 0 {
			return false, nil
		}
		return false, s.respondError(message.ID, -32002, "server is not available")
	}
	switch message.Method {
	case "textDocument/didOpen":
		var params didOpenParams
		if err := decodeParams(message.Params, &params); err != nil {
			return false, err
		}
		document, err := openDocument(params.TextDocument)
		if err != nil {
			return false, err
		}
		s.documents[document.URI] = document
		return false, s.scheduleAnalysis(document, false)
	case "textDocument/didChange":
		var params didChangeParams
		if err := decodeParams(message.Params, &params); err != nil {
			return false, err
		}
		document, found := s.documents[params.TextDocument.URI]
		if !found {
			return false, fmt.Errorf(
				"change references unopened document %q",
				params.TextDocument.URI,
			)
		}
		if len(params.ContentChanges) != 1 || len(params.ContentChanges[0].Range) != 0 {
			return false, errors.New(
				"Glippy LSP requires one full document content change",
			)
		}
		if params.TextDocument.Version <= document.Version {
			return false, fmt.Errorf(
				"Glippy LSP requires a newer document version than %d",
				document.Version,
			)
		}
		document.Version = params.TextDocument.Version
		document.Text = []byte(params.ContentChanges[0].Text)
		s.documents[document.URI] = document
		return false, s.scheduleAnalysis(document, true)
	case "textDocument/didClose":
		var params didCloseParams
		if err := decodeParams(message.Params, &params); err != nil {
			return false, err
		}
		if err := s.rejectPendingAnalysisRequests(); err != nil {
			return false, err
		}
		delete(s.documents, params.TextDocument.URI)
		delete(s.analyses, params.TextDocument.URI)
		s.cancelDocumentAnalysis()
		if err := s.notify(
			"textDocument/publishDiagnostics",
			publishDiagnosticsParams{
				URI: params.TextDocument.URI,
				Diagnostics: []protocolDiagnostic{},
			},
		);
			err != nil {
			return false, err
		}
		if _, supported := s.backend.(WorkspaceBackend); supported {
			if next, found := s.firstOpenDocument(); found {
				if err := s.scheduleAnalysis(next, false); err != nil {
					return false, err
				}
			}
		}
		return false, nil
	case "workspace/didChangeWatchedFiles":
		var params didChangeWatchedFilesParams
		if err := decodeParams(message.Params, &params); err != nil {
			return false, err
		}
		paths, err := watchedFilePaths(params.Changes)
		if err != nil {
			return false, err
		}
		backend, supported := s.backend.(WorkspaceFileBackend)
		if !supported || len(paths) == 0 {
			return false, nil
		}
		if err := s.rejectPendingAnalysisRequests(); err != nil {
			return false, err
		}
		s.cancelDocumentAnalysis()
		clear(s.analyses)
		if err := backend.WorkspaceFilesChanged(s.ctx, paths); err != nil {
			return false, err
		}
		next, found := s.firstOpenDocument()
		if !found {
			return false, nil
		}
		return false, s.scheduleAnalysis(next, true)
	case "textDocument/codeAction":
		if len(message.ID) == 0 {
			return false, errors.New("code action requires a request ID")
		}
		if s.analysisCancel != nil {
			if len(s.pendingAnalysisRequests) >= maximumPendingAnalysisRequests {
				return false, s.respondError(
					message.ID,
					-32000,
					"too many pending analysis requests",
				)
			}
			s.pendingAnalysisRequests = append(s.pendingAnalysisRequests, message)
			return false, nil
		}
		return false, s.handleCodeAction(message)
	case "textDocument/formatting":
		return false, s.handleFormatting(message)
	default:
		if len(message.ID) == 0 {
			return false, nil
		}
		return false, s.respondError(message.ID, -32601, "method not found")
	}
}

func (s *server) registerWorkspaceFileWatching() error {
	if _, supported := s.backend.(WorkspaceFileBackend); !supported {
		return nil
	}
	if !s.watchingDynamicRegistration {
		return nil
	}
	if s.watchingRegistered {
		return nil
	}
	id, err := json.Marshal(watchedFilesRequestID)
	if err != nil {
		return err
	}
	key := string(id)
	if _, pending := s.pendingClientRequests[key]; pending {
		return nil
	}
	s.watchingRegistered = true
	s.pendingClientRequests[key] = struct{}{}
	return writeMessage(
		s.writer,
		rpcRequest{
			JSONRPC: "2.0",
			ID: watchedFilesRequestID,
			Method: "client/registerCapability",
			Params: map[string]any{
				"registrations": []any{
					map[string]any{
						"id": "glippy-workspace-files",
						"method": "workspace/didChangeWatchedFiles",
						"registerOptions": map[string]any{
							"watchers": []any{
								map[string]any{
									"globPattern": "**/*.go",
									"kind": 7,
								},
								map[string]any{
									"globPattern": "**/{go.mod,go.sum,go.work,go.work.sum,.glippy.toml}",
									"kind": 7,
								},
								map[string]any{
									"globPattern": "**/*.{toml,json}",
									"kind": 7,
								},
							},
						},
					},
				},
			},
		},
	)
}

func watchedFilePaths(changes []watchedFileChange) ([]string, error) {
	if len(changes) > maximumWatchedFileChanges {
		return nil, fmt.Errorf(
			"workspace file notification exceeds %d changes",
			maximumWatchedFileChanges,
		)
	}
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Type < 1 || change.Type > 3 {
			return nil, fmt.Errorf(
				"unsupported workspace file change type %d",
				change.Type,
			)
		}
		path, err := filePath(change.URI)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return slices.Compact(paths), nil
}

func (s *server) firstOpenDocument() (Document, bool) {
	var result Document
	for _, open := range s.documents {
		if result.URI == "" ||
			open.Path < result.Path ||
			(open.Path == result.Path && open.URI < result.URI) {
			result = open
		}
	}
	return result, result.URI != ""
}

func (s *server) scheduleAnalysis(document Document, debounce bool) error {
	if err := s.rejectPendingAnalysisRequests(); err != nil {
		return err
	}
	s.cancelDocumentAnalysis()
	backend, workspaceSupported := s.backend.(WorkspaceBackend)
	if workspaceSupported {
		clear(s.analyses)
	} else {
		delete(s.analyses, document.URI)
	}
	documents := make([]Document, 0, len(s.documents))
	for _, open := range s.documents {
		documents = append(documents, cloneDocument(open))
	}
	sort.Slice(
		documents,
		func(left, right int) bool {
			if documents[left].Path != documents[right].Path {
				return documents[left].Path < documents[right].Path
			}
			return documents[left].URI < documents[right].URI
		},
	)
	s.analysisGeneration++
	generation := s.analysisGeneration
	analysisCtx, cancel := context.WithCancel(s.ctx)
	s.analysisCancel = cancel
	go s.runAnalysis(
		analysisCtx,
		generation,
		cloneDocument(document),
		documents,
		backend,
		workspaceSupported,
		debounce,
	)
	return nil
}

func (s *server) cancelDocumentAnalysis() {
	s.analysisGeneration++
	if s.analysisCancel != nil {
		s.analysisCancel()
		s.analysisCancel = nil
	}
}

func (s *server) runAnalysis(
	ctx context.Context,
	generation uint64,
	trigger Document,
	documents []Document,
	workspaceBackend WorkspaceBackend,
	workspaceSupported bool,
	debounce bool,
) {
	if debounce {
		timer := time.NewTimer(documentAnalysisDebounce)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
	completion := analysisCompletion{
		generation: generation,
		trigger: trigger,
		documents: documents,
		workspaceSupported: workspaceSupported,
	}
	if workspaceSupported {
		completion.workspace, completion.err = workspaceBackend.AnalyzeWorkspace(
			ctx,
			documents,
		)
	} else {
		completion.analysis, completion.err = s.backend.Analyze(ctx, trigger)
	}
	select {
	case s.analysisResults <- completion:
	case <-s.ctx.Done():
	}
}

func (s *server) completeAnalysis(completion analysisCompletion) error {
	if completion.generation != s.analysisGeneration {
		return nil
	}
	if s.analysisCancel != nil {
		s.analysisCancel()
		s.analysisCancel = nil
	}
	if completion.err != nil {
		if errors.Is(completion.err, context.Canceled) {
			return s.finishAnalysisCycle()
		}
		return completion.err
	}
	if !s.analysisSnapshotCurrent(completion.documents) {
		if err := s.rejectPendingAnalysisRequests(); err != nil {
			return err
		}
		return s.finishAnalysisCycle()
	}
	if !completion.workspaceSupported {
		if err := s.publishAnalysis(
			completion.trigger,
			completion.analysis,
			completion.err,
		);
			err != nil {
			return err
		}
		return s.finishAnalysisCycle()
	}
	byURI := make(map[string]WorkspaceAnalysis, len(completion.workspace))
	for _, result := range completion.workspace {
		if _, found := byURI[result.Document.URI]; found {
			return fmt.Errorf(
				"LSP workspace backend repeated document %q",
				result.Document.URI,
			)
		}
		byURI[result.Document.URI] = result
	}
	for _, open := range completion.documents {
		result, found := byURI[open.URI]
		if !found ||
			result.Document.Path != open.Path ||
			result.Document.Version != open.Version ||
			!bytes.Equal(result.Document.Text, open.Text) {
			return fmt.Errorf(
				"LSP workspace backend omitted exact document %q",
				open.URI,
			)
		}
		if err := s.publishAnalysis(open, result.Analysis, result.Err); err != nil {
			return err
		}
	}
	if len(byURI) != len(completion.documents) {
		return errors.New("LSP workspace backend returned unknown documents")
	}
	return s.finishAnalysisCycle()
}

func (s *server) rejectPendingAnalysisRequests() error {
	requests := s.pendingAnalysisRequests
	s.pendingAnalysisRequests = nil
	for _, message := range requests {
		canceled := false
		s.requestMu.Lock()
		if _, found := s.pendingCancellations[string(message.ID)]; found {
			delete(s.pendingCancellations, string(message.ID))
			canceled = true
		}
		s.requestMu.Unlock()
		if canceled {
			if err := s.respondError(message.ID, -32800, "request canceled");
				err != nil {
				return err
			}
			continue
		}
		if err := s.respondError(message.ID, -32801, "document analysis was superseded");
			err != nil {
			return err
		}
	}
	return nil
}

func (s *server) finishAnalysisCycle() error {
	requests := s.pendingAnalysisRequests
	s.pendingAnalysisRequests = nil
	for _, message := range requests {
		if err := s.handleCodeAction(message); err != nil {
			return err
		}
	}
	if len(s.shutdownID) == 0 {
		return nil
	}
	id := bytes.Clone(s.shutdownID)
	s.shutdownID = nil
	if err := s.respond(id, nil); err != nil {
		return err
	}
	if s.exitAfterShutdown {
		s.exitRequested = true
	}
	return nil
}

func (s *server) analysisSnapshotCurrent(documents []Document) bool {
	if len(documents) != len(s.documents) {
		return false
	}
	for _, expected := range documents {
		current, found := s.documents[expected.URI]
		if !found ||
			current.Path != expected.Path ||
			current.Version != expected.Version ||
			!bytes.Equal(current.Text, expected.Text) {
			return false
		}
	}
	return true
}

func (s *server) publishAnalysis(document Document, analysis Analysis, err error) error {
	if contextErr := s.ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		delete(s.analyses, document.URI)
		return s.publishError(document, err)
	}
	if analysis.File == nil ||
		analysis.File.Path() != document.Path ||
		!bytes.Equal(analysis.File.Bytes(), document.Text) {
		return errors.New("LSP backend analysis does not match the document source")
	}
	diagnostics, err := protocolDiagnostics(document.URI, analysis)
	if err != nil {
		return err
	}
	s.analyses[document.URI] = analysis
	version := document.Version
	return s.notify(
		"textDocument/publishDiagnostics",
		publishDiagnosticsParams{
			URI: document.URI,
			Version: &version,
			Diagnostics: diagnostics,
		},
	)
}

func (s *server) publishError(document Document, analysisErr error) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	file, _ := source.Load(document.Path, document.Text)
	range_ := protocolRange{}
	if file != nil {
		position, valid := offsetPosition(file.Bytes(), 0)
		if valid {
			range_ = protocolRange{Start: position, End: position}
		}
	}
	version := document.Version
	return s.notify(
		"textDocument/publishDiagnostics",
		publishDiagnosticsParams{
			URI: document.URI,
			Version: &version,
			Diagnostics: []protocolDiagnostic{
				{
					Range: range_,
					Severity: SeverityError,
					Code: "glippy",
					Source: "glippy",
					Message: analysisErr.Error(),
				},
			},
		},
	)
}

func (s *server) handleCodeAction(message rpcMessage) error {
	if len(message.ID) == 0 {
		return errors.New("code action requires a request ID")
	}
	requestCtx, finishRequest := s.beginRequest(message.ID)
	defer finishRequest()
	if err := requestCtx.Err(); err != nil {
		return s.respondError(message.ID, -32800, "request canceled")
	}
	var params codeActionParams
	if err := decodeParams(message.Params, &params); err != nil {
		return s.respondError(message.ID, -32602, err.Error())
	}
	document, found := s.documents[params.TextDocument.URI]
	if !found {
		return s.respondError(message.ID, -32602, "document is not open")
	}
	analysis, found := s.analyses[document.URI]
	if !found {
		return s.respond(message.ID, []protocolCodeAction{})
	}
	range_, valid := byteRange(document.Text, params.Range)
	if !valid {
		return s.respondError(message.ID, -32602, "code action range is not valid UTF-16")
	}
	actions, err := s.backend.CodeActions(requestCtx, cloneDocument(document), analysis, range_)
	if contextErr := s.ctx.Err(); contextErr != nil {
		return contextErr
	}
	if requestErr := requestCtx.Err(); requestErr != nil {
		return s.respondError(message.ID, -32800, "request canceled")
	}
	if err != nil {
		return s.respondError(message.ID, -32603, err.Error())
	}
	converted := make([]protocolCodeAction, 0, len(actions))
	for _, action := range actions {
		if action.Title == "" || action.Kind == "" || !utf8.Valid(action.NewText) {
			return s.respondError(
				message.ID,
				-32603,
				"backend returned an invalid code action",
			)
		}
		var diagnostic protocolDiagnostic
		if action.DiagnosticCode != "" {
			if action.DiagnosticMessage == "" {
				return s.respondError(
					message.ID,
					-32603,
					"backend returned an invalid code action diagnostic",
				)
			}
			var err error
			diagnostic, err = convertDiagnostic(
				document.URI,
				analysis.File,
				Diagnostic{
					Range: action.DiagnosticRange,
					Severity: action.DiagnosticSeverity,
					Code: action.DiagnosticCode,
					DocumentationURI: action.DiagnosticDocumentationURI,
					Message: action.DiagnosticMessage,
					WithheldFixes: action.DiagnosticWithheldFixes,
				},
			)
			if err != nil {
				return s.respondError(message.ID, -32603, err.Error())
			}
		}
		converted = append(
			converted,
			protocolCodeAction{
				Title: action.Title,
				Kind: action.Kind,
				IsPreferred: action.Preferred,
				Edit: protocolWorkspaceEdit{
					DocumentChanges: []protocolTextDocumentEdit{
						{
							TextDocument: versionedTextDocumentIdentifier{
								URI: document.URI,
								Version: document.Version,
							},
							Edits: []protocolTextEdit{
								{
									Range: fullDocumentRange(
										document.Text,
									),
									NewText: string(
										action.NewText,
									),
								},
							},
						},
					},
				},
			},
		)
		if action.DiagnosticCode != "" {
			converted[len(converted) - 1].Diagnostics = []protocolDiagnostic{diagnostic}
		}
	}
	return s.respond(message.ID, converted)
}

func (s *server) handleFormatting(message rpcMessage) error {
	if len(message.ID) == 0 {
		return errors.New("formatting requires a request ID")
	}
	requestCtx, finishRequest := s.beginRequest(message.ID)
	defer finishRequest()
	if err := requestCtx.Err(); err != nil {
		return s.respondError(message.ID, -32800, "request canceled")
	}
	var params documentRequestParams
	if err := decodeParams(message.Params, &params); err != nil {
		return s.respondError(message.ID, -32602, err.Error())
	}
	document, found := s.documents[params.TextDocument.URI]
	if !found {
		return s.respondError(message.ID, -32602, "document is not open")
	}
	formatted, err := s.backend.Format(requestCtx, cloneDocument(document))
	if contextErr := s.ctx.Err(); contextErr != nil {
		return contextErr
	}
	if requestErr := requestCtx.Err(); requestErr != nil {
		return s.respondError(message.ID, -32800, "request canceled")
	}
	if err != nil {
		return s.respondError(message.ID, -32603, err.Error())
	}
	if bytes.Equal(formatted, document.Text) {
		return s.respond(message.ID, []protocolTextEdit{})
	}
	return s.respond(
		message.ID,
		[]protocolTextEdit{
			{Range: fullDocumentRange(document.Text), NewText: string(formatted)},
		},
	)
}

func (s *server) beginRequest(id json.RawMessage) (context.Context, func()) {
	key := string(id)
	ctx, cancel := context.WithCancel(s.ctx)
	s.requestMu.Lock()
	if _, canceled := s.pendingCancellations[key]; canceled {
		delete(s.pendingCancellations, key)
		cancel()
	}
	s.requests[key] = cancel
	s.requestMu.Unlock()
	return ctx, func() {
		s.requestMu.Lock()
		delete(s.requests, key)
		s.requestMu.Unlock()
		cancel()
	}
}

func (s *server) cancelRequest(input json.RawMessage) error {
	var params cancelRequestParams
	if err := decodeParams(input, &params); err != nil {
		return err
	}
	if len(params.ID) == 0 {
		return errors.New("cancel request requires an ID")
	}
	key := string(params.ID)
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	if cancel, found := s.requests[key]; found {
		cancel()
		return nil
	}
	if len(s.pendingCancellations) >= maximumPendingCancellations {
		return errors.New("too many pending request cancellations")
	}
	s.pendingCancellations[key] = struct{}{}
	return nil
}

func protocolDiagnostics(uri string, analysis Analysis) ([]protocolDiagnostic, error) {
	result := make([]protocolDiagnostic, len(analysis.Diagnostics))
	for index, diagnostic := range analysis.Diagnostics {
		converted, err := convertDiagnostic(uri, analysis.File, diagnostic)
		if err != nil {
			return nil, err
		}
		result[index] = converted
	}
	return result, nil
}

func convertDiagnostic(
	uri string,
	file *source.File,
	diagnostic Diagnostic,
) (protocolDiagnostic, error) {
	if diagnostic.Code == "" ||
		diagnostic.Message == "" ||
		(diagnostic.Severity != SeverityError && diagnostic.Severity != SeverityWarning) {
		return protocolDiagnostic{}, errors.New("diagnostic metadata is incomplete")
	}
	range_, valid := protocolRangeForBytes(file.Bytes(), diagnostic.Range)
	if !valid {
		return protocolDiagnostic{}, fmt.Errorf(
			"diagnostic %q has invalid source range",
			diagnostic.Code,
		)
	}
	related := make([]protocolRelated, len(diagnostic.Related))
	for index, item := range diagnostic.Related {
		relatedRange, valid := protocolRangeForBytes(file.Bytes(), item.Range)
		if !valid {
			return protocolDiagnostic{}, fmt.Errorf(
				"diagnostic %q has invalid related range",
				diagnostic.Code,
			)
		}
		related[index] = protocolRelated{
			Location: protocolLocation{URI: uri, Range: relatedRange},
			Message: item.Message,
		}
	}
	result := protocolDiagnostic{
		Range: range_,
		Severity: diagnostic.Severity,
		Code: diagnostic.Code,
		Source: "glippy",
		Message: diagnostic.Message,
		RelatedInformation: related,
	}
	if len(diagnostic.WithheldFixes) > 0 {
		withheldFixes := make([]WithheldFix, len(diagnostic.WithheldFixes))
		copy(withheldFixes, diagnostic.WithheldFixes)
		for _, fix := range withheldFixes {
			if strings.TrimSpace(fix.Name) == "" ||
				strings.TrimSpace(fix.Reason) == "" ||
				strings.TrimSpace(fix.Message) == "" {
				return protocolDiagnostic{}, fmt.Errorf(
					"diagnostic %q has invalid withheld fix metadata",
					diagnostic.Code,
				)
			}
		}
		result.Data = &protocolDiagnosticData{WithheldFixes: withheldFixes}
	}
	if diagnostic.DocumentationURI != "" {
		parsed, err := url.ParseRequestURI(diagnostic.DocumentationURI)
		if err != nil || !parsed.IsAbs() {
			return protocolDiagnostic{}, fmt.Errorf(
				"diagnostic %q has invalid documentation URI",
				diagnostic.Code,
			)
		}
		result.CodeDescription = &protocolCodeDescription{Href: diagnostic.DocumentationURI}
	}
	return result, nil
}

func openDocument(item textDocumentItem) (Document, error) {
	path, err := filePath(item.URI)
	if err != nil {
		return Document{}, err
	}
	return Document{
		URI: item.URI,
		Path: path,
		Version: item.Version,
		Text: []byte(item.Text),
	}, nil
}

func filePath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse document URI: %w", err)
	}
	if parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") {
		return "", fmt.Errorf("unsupported document URI %q", uri)
	}
	path := filepath.Clean(filepath.FromSlash(parsed.Path))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("document URI %q is not absolute", uri)
	}
	return path, nil
}

func protocolRangeForBytes(input []byte, range_ source.Range) (protocolRange, bool) {
	if range_.Start < 0 || range_.End < range_.Start || range_.End > len(input) {
		return protocolRange{}, false
	}
	start, valid := offsetPosition(input, range_.Start)
	if !valid {
		return protocolRange{}, false
	}
	end, valid := offsetPosition(input, range_.End)
	return protocolRange{Start: start, End: end}, valid
}

func offsetPosition(input []byte, offset int) (protocolPosition, bool) {
	if offset < 0 ||
		offset > len(input) ||
		(offset < len(input) && !utf8.RuneStart(input[offset])) {
		return protocolPosition{}, false
	}
	line := 0
	lineStart := 0
	for index, value := range input[:offset] {
		if value == '\n' {
			line++
			lineStart = index + 1
		}
	}
	units := 0
	for remaining := input[lineStart:offset]; len(remaining) > 0; {
		rune_, size := utf8.DecodeRune(remaining)
		if rune_ == utf8.RuneError && size == 1 {
			return protocolPosition{}, false
		}
		if rune_ > 0xffff {
			units += 2
		} else {
			units++
		}
		remaining = remaining[size:]
	}
	return protocolPosition{Line: line, Character: units}, true
}

func byteRange(input []byte, range_ protocolRange) (source.Range, bool) {
	start, valid := positionOffset(input, range_.Start)
	if !valid {
		return source.Range{}, false
	}
	end, valid := positionOffset(input, range_.End)
	if !valid || end < start {
		return source.Range{}, false
	}
	return source.Range{Start: start, End: end}, true
}

func positionOffset(input []byte, position protocolPosition) (int, bool) {
	if position.Line < 0 || position.Character < 0 {
		return 0, false
	}
	line := 0
	offset := 0
	for line < position.Line {
		newline := bytes.IndexByte(input[offset:], '\n')
		if newline < 0 {
			return 0, false
		}
		offset += newline + 1
		line++
	}
	units := 0
	for offset < len(input) && input[offset] != '\n' {
		if units == position.Character {
			return offset, true
		}
		rune_, size := utf8.DecodeRune(input[offset:])
		if rune_ == utf8.RuneError && size == 1 {
			return 0, false
		}
		width := 1
		if rune_ > 0xffff {
			width = 2
		}
		if units + width > position.Character {
			return 0, false
		}
		units += width
		offset += size
	}
	return offset, units == position.Character
}

func fullDocumentRange(input []byte) protocolRange {
	end, _ := offsetPosition(input, len(input))
	return protocolRange{Start: protocolPosition{}, End: end}
}

func cloneDocument(document Document) Document {
	result := document
	result.Text = bytes.Clone(document.Text)
	return result
}

func decodeParams(input json.RawMessage, target any) error {
	if len(input) == 0 {
		input = []byte("{}")
	}
	if err := json.Unmarshal(input, target); err != nil {
		return fmt.Errorf("decode request parameters: %w", err)
	}
	return nil
}

func (s *server) respond(id json.RawMessage, result any) error {
	return writeMessage(
		s.writer,
		rpcSuccessResponse{JSONRPC: "2.0", ID: bytes.Clone(id), Result: result},
	)
}

func (s *server) respondError(id json.RawMessage, code int, message string) error {
	return writeMessage(
		s.writer,
		rpcErrorResponse{
			JSONRPC: "2.0",
			ID: bytes.Clone(id),
			Error: &rpcError{Code: code, Message: message},
		},
	)
}

func (s *server) notify(method string, params any) error {
	return writeMessage(
		s.writer,
		struct {
			JSONRPC string `json:"jsonrpc"`
			Method string `json:"method"`
			Params any `json:"params"`
		}{JSONRPC: "2.0", Method: method, Params: params},
	)
}

func readMessage(reader *bufio.Reader) (rpcMessage, error) {
	length := int64(-1)
	headerBytes := 0
	for {
		lineBytes, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			return rpcMessage{}, fmt.Errorf(
				"LSP header line exceeds %d bytes",
				maximumHeaderSize,
			)
		}
		line := string(lineBytes)
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return rpcMessage{}, io.EOF
			}
			return rpcMessage{}, fmt.Errorf("read LSP header: %w", err)
		}
		headerBytes += len(lineBytes)
		if headerBytes > maximumHeaderSize {
			return rpcMessage{}, fmt.Errorf(
				"LSP headers exceed %d bytes",
				maximumHeaderSize,
			)
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return rpcMessage{}, errors.New("invalid LSP header")
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if length >= 0 {
				return rpcMessage{}, errors.New("duplicate Content-Length header")
			}
			parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return rpcMessage{}, fmt.Errorf("parse LSP content length: %w", err)
			}
			length = parsed
		}
	}
	if length < 0 || length > maximumMessageSize {
		return rpcMessage{}, fmt.Errorf("invalid LSP content length %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return rpcMessage{}, fmt.Errorf("read LSP payload: %w", err)
	}
	var message rpcMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return rpcMessage{}, fmt.Errorf("decode LSP payload: %w", err)
	}
	return message, nil
}

func writeMessage(writer io.Writer, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode LSP message: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return fmt.Errorf("write LSP header: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		return fmt.Errorf("write LSP payload: %w", err)
	}
	return nil
}
