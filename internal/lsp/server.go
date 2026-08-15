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
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/faustbrian/glippy/internal/source"
)

const maximumMessageSize = source.MaxFileSize + 1 << 20

const maximumHeaderSize = 4 << 10

const maximumPendingCancellations = 1024

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
	initialized bool
	shutdown bool
}

// Serve runs one LSP 3.17-compatible stdio session until exit or EOF.
func Serve(ctx context.Context, input io.Reader, output io.Writer, backend Backend) error {
	if ctx == nil || input == nil || output == nil || backend == nil {
		return errors.New("LSP requires context, streams, and backend")
	}
	state := &server{
		ctx: ctx,
		reader: bufio.NewReader(input),
		writer: output,
		backend: backend,
		documents: make(map[string]Document),
		analyses: make(map[string]Analysis),
		requests: make(map[string]context.CancelFunc),
		pendingCancellations: make(map[string]struct{}),
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
		case result, available := <-readResults:
			if !available {
				return nil
			}
			if errors.Is(result.err, io.EOF) {
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
	if message.JSONRPC != "2.0" || message.Method == "" {
		return false, errors.New("invalid JSON-RPC message")
	}
	switch message.Method {
	case "initialize":
		if len(message.ID) == 0 {
			return false, errors.New("initialize requires a request ID")
		}
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
	case "initialized", "$/setTrace":
		return false, nil
	case "shutdown":
		if len(message.ID) == 0 {
			return false, errors.New("shutdown requires a request ID")
		}
		s.shutdown = true
		return false, s.respond(message.ID, nil)
	case "exit":
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
		return false, s.analyzeAndPublish(document)
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
		return false, s.analyzeAndPublish(document)
	case "textDocument/didClose":
		var params didCloseParams
		if err := decodeParams(message.Params, &params); err != nil {
			return false, err
		}
		delete(s.documents, params.TextDocument.URI)
		delete(s.analyses, params.TextDocument.URI)
		return false, s.notify(
			"textDocument/publishDiagnostics",
			publishDiagnosticsParams{
				URI: params.TextDocument.URI,
				Diagnostics: []protocolDiagnostic{},
			},
		)
	case "textDocument/codeAction":
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

func (s *server) analyzeAndPublish(document Document) error {
	analysis, err := s.backend.Analyze(s.ctx, cloneDocument(document))
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
