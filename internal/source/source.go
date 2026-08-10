// Package source owns immutable physical Go source units and their lexical
// reconstruction data.
package source

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Range is a half-open physical byte range.
type Range struct {
	Start int
	End   int
}

// SemicolonKind records whether a semicolon was present in source or inserted
// by the Go scanner.
type SemicolonKind uint8

const (
	SemicolonNone SemicolonKind = iota
	SemicolonExplicit
	SemicolonInserted
)

// Token is one physical lexical token.
type Token struct {
	Kind      token.Token
	Range     Range
	Raw       string
	Semicolon SemicolonKind
}

// PieceKind identifies the role of a physical reconstruction segment.
type PieceKind uint8

const (
	PieceTrivia PieceKind = iota
	PieceToken
)

// Piece is one non-overlapping physical source segment.
type Piece struct {
	Kind  PieceKind
	Range Range
	Raw   []byte
}

// TriviaKind identifies physical bytes between lexical tokens.
type TriviaKind uint8

const (
	TriviaWhitespace TriviaKind = iota
	TriviaBOM
)

// Trivia is one exact physical token gap or accepted byte-order mark.
type Trivia struct {
	Kind  TriviaKind
	Range Range
	Raw   string
}

// CommentID is a stable per-file comment identity.
type CommentID uint32

// Comment is one exact physical source comment.
type Comment struct {
	ID    CommentID
	Range Range
	Raw   string
}

// DirectiveKind identifies comments whose placement has tool or language
// semantics.
type DirectiveKind uint8

const (
	DirectiveBuildConstraint DirectiveKind = iota + 1
	DirectiveGoGenerate
	DirectiveGoEmbed
	DirectiveCompiler
	DirectiveLine
	DirectiveGenerated
	DirectiveGoxSuppression
	DirectiveCgoPreamble
)

// Directive is a classified physical source comment.
type Directive struct {
	Kind  DirectiveKind
	Range Range
	Raw   string
}

type directiveLineAnchor struct {
	Before uint8
	After  uint8
}

// NewlineStyle is the physical line-ending policy observed in a source file.
type NewlineStyle uint8

const (
	NewlineNone NewlineStyle = iota
	NewlineLF
	NewlineCRLF
	NewlineMixed
)

// Metadata describes physical file properties relevant to formatting.
type Metadata struct {
	HasBOM       bool
	Newlines     NewlineStyle
	FinalNewline bool
	Generated    bool
}

// Digest identifies the exact bytes used to construct a File.
type Digest [sha256.Size]byte

// File is an immutable physical source unit.
type File struct {
	path       string
	bytes      []byte
	digest     Digest
	fileSet    *token.FileSet
	tokenFile  *token.File
	syntax     *ast.File
	tokens     []Token
	pieces     []Piece
	trivia     []Trivia
	comments   []Comment
	directives []Directive
	metadata   Metadata
	parseErr   error
}

// Load constructs a physical source unit. Invalid Go returns the lossless
// diagnostic-only File together with the parse or scan error.
func Load(path string, input []byte) (*File, error) {
	physical := bytes.Clone(input)
	cleanPath := filepath.Clean(path)
	fileSet := token.NewFileSet()
	syntax, parseErr := parser.ParseFile(
		fileSet,
		cleanPath,
		physical,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	tokenFile := parsedTokenFile(fileSet, syntax)
	if tokenFile == nil {
		return nil, errors.New("Go parser did not register the physical source file")
	}

	tokens, scanErr := scanTokens(tokenFile, physical)
	pieces, ledgerErr := buildPieces(physical, tokens)
	trivia := buildTrivia(physical, tokens)
	comments := buildComments(tokens)
	directives := classifyDirectives(tokens, syntax, tokenFile)
	metadata := inspectMetadata(physical, directives)
	directiveErr := validateDirectives(directives)
	loadErr := errors.Join(parseErr, scanErr, ledgerErr, directiveErr)

	return &File{
		path:       cleanPath,
		bytes:      physical,
		digest:     sha256.Sum256(physical),
		fileSet:    fileSet,
		tokenFile:  tokenFile,
		syntax:     syntax,
		tokens:     tokens,
		pieces:     pieces,
		trivia:     trivia,
		comments:   comments,
		directives: directives,
		metadata:   metadata,
		parseErr:   loadErr,
	}, loadErr
}

// Path returns the normalized physical source identity supplied to Load.
func (f *File) Path() string { return f.path }

// Digest returns the exact source digest.
func (f *File) Digest() Digest { return f.digest }

// Bytes returns an independent copy of the physical source bytes.
func (f *File) Bytes() []byte { return bytes.Clone(f.bytes) }

// Tokens returns the physical token ledger.
func (f *File) Tokens() []Token { return slices.Clone(f.tokens) }

// Pieces returns the ordered physical reconstruction ledger.
func (f *File) Pieces() []Piece {
	result := make([]Piece, len(f.pieces))
	for index, piece := range f.pieces {
		result[index] = piece
		result[index].Raw = bytes.Clone(piece.Raw)
	}
	return result
}

// Trivia returns exact physical token gaps in source order.
func (f *File) Trivia() []Trivia { return slices.Clone(f.trivia) }

// Comments returns exact source comments with stable per-file identities.
func (f *File) Comments() []Comment { return slices.Clone(f.comments) }

// Directives returns classified directive comments in physical order.
func (f *File) Directives() []Directive { return slices.Clone(f.directives) }

// Metadata returns physical file metadata.
func (f *File) Metadata() Metadata { return f.metadata }

// CanFormat reports whether parsing and physical reconstruction accepted the
// complete file.
func (f *File) CanFormat() bool { return f.parseErr == nil }

// ReadSyntax provides an isolated parsed syntax view to a run-owned consumer.
func (f *File) ReadSyntax(read func(*ast.File) error) error {
	if f.parseErr != nil {
		return f.parseErr
	}
	fileSet := token.NewFileSet()
	syntax, err := parser.ParseFile(
		fileSet,
		f.path,
		f.bytes,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		return fmt.Errorf("construct immutable syntax view: %w", err)
	}
	return read(syntax)
}

// RawToken returns the exact physical spelling of the token at position.
func (f *File) RawToken(position token.Pos) (string, bool) {
	offset := f.tokenFile.Offset(position)
	index, found := slices.BinarySearchFunc(f.tokens, offset, func(item Token, target int) int {
		return item.Range.Start - target
	})
	if !found {
		return "", false
	}
	return f.tokens[index].Raw, true
}

// PhysicalOffset maps a parsed position to the exact source byte offset.
func (f *File) PhysicalOffset(position token.Pos) (int, bool) {
	if !position.IsValid() || f.tokenFile == nil {
		return 0, false
	}
	offset := f.tokenFile.Offset(position)
	return offset, offset >= 0 && offset <= len(f.bytes)
}

// Slice returns exact physical source text for a valid byte range.
func (f *File) Slice(sourceRange Range) (string, bool) {
	if sourceRange.Start < 0 || sourceRange.End < sourceRange.Start || sourceRange.End > len(f.bytes) {
		return "", false
	}
	return string(f.bytes[sourceRange.Start:sourceRange.End]), true
}

// ValidateEquivalent verifies the normalized syntax and source-accounting
// invariants required before formatted output can be accepted.
func ValidateEquivalent(before, after *File) error {
	if before == nil || after == nil || !before.CanFormat() || !after.CanFormat() {
		return errors.New("equivalence requires two valid source units")
	}
	if before.metadata.HasBOM != after.metadata.HasBOM {
		return errors.New("byte-order mark identity changed")
	}
	if !equivalentTokens(before.tokens, after.tokens) {
		return errors.New("normalized lexical tokens changed")
	}
	if !slices.EqualFunc(before.comments, after.comments, func(left, right Comment) bool {
		return left.Raw == right.Raw
	}) {
		return errors.New("comment identity or ordering changed")
	}
	if !slices.Equal(commentOwnershipFingerprint(before.tokens), commentOwnershipFingerprint(after.tokens)) {
		return errors.New("comment source ownership changed")
	}
	if !slices.EqualFunc(before.directives, after.directives, func(left, right Directive) bool {
		return left.Kind == right.Kind && left.Raw == right.Raw
	}) {
		return errors.New("directive identity or ordering changed")
	}
	beforeAnchors, err := directiveLineAnchors(before.bytes, before.tokens, before.directives)
	if err != nil {
		return err
	}
	afterAnchors, err := directiveLineAnchors(after.bytes, after.tokens, after.directives)
	if err != nil {
		return err
	}
	if !slices.Equal(beforeAnchors, afterAnchors) {
		return errors.New("directive source anchor changed")
	}
	beforeSyntax, err := syntaxFingerprint(before.syntax)
	if err != nil {
		return err
	}
	afterSyntax, err := syntaxFingerprint(after.syntax)
	if err != nil {
		return err
	}
	if beforeSyntax != afterSyntax {
		return errors.New("normalized syntax tree changed")
	}
	return nil
}

func directiveLineAnchors(
	physical []byte,
	tokens []Token,
	directives []Directive,
) ([]directiveLineAnchor, error) {
	tokenByRange := make(map[Range]int, len(tokens))
	for index, item := range tokens {
		if item.Kind == token.COMMENT {
			tokenByRange[item.Range] = index
		}
	}
	anchors := make([]directiveLineAnchor, 0, len(directives))
	for _, directive := range directives {
		index, found := tokenByRange[directive.Range]
		if !found || tokens[index].Raw != directive.Raw {
			return nil, fmt.Errorf("directive at byte %d has no physical comment token", directive.Range.Start)
		}
		previousEnd := -1
		for previous := index - 1; previous >= 0; previous-- {
			if tokens[previous].Range.Start != tokens[previous].Range.End {
				previousEnd = tokens[previous].Range.End
				break
			}
		}
		nextStart := -1
		for next := index + 1; next < len(tokens); next++ {
			if tokens[next].Range.Start != tokens[next].Range.End {
				nextStart = tokens[next].Range.Start
				break
			}
		}
		beforeBreaks, err := boundedLineBreaks(physical, previousEnd, directive.Range.Start)
		if err != nil {
			return nil, err
		}
		if directive.Kind != DirectiveGoxSuppression && beforeBreaks == 2 {
			beforeBreaks = 1
		}
		afterBreaks, err := boundedLineBreaks(physical, directive.Range.End, nextStart)
		if err != nil {
			return nil, err
		}
		anchors = append(anchors, directiveLineAnchor{Before: beforeBreaks, After: afterBreaks})
	}
	return anchors, nil
}

func boundedLineBreaks(physical []byte, start, end int) (uint8, error) {
	const missingBoundary = uint8(3)
	if start < 0 || end < 0 {
		return missingBoundary, nil
	}
	if end < start || end > len(physical) {
		return 0, errors.New("directive anchor has an invalid physical range")
	}
	count := bytes.Count(physical[start:end], []byte{'\n'})
	if count > 2 {
		count = 2
	}
	return uint8(count), nil
}

func commentOwnershipFingerprint(tokens []Token) []int {
	result := make([]int, 0)
	significantTokens := 0
	for _, item := range tokens {
		switch item.Kind {
		case token.COMMENT:
			result = append(result, significantTokens)
		case token.SEMICOLON, token.COMMA:
		default:
			significantTokens++
		}
	}
	return result
}

func equivalentTokens(before, after []Token) bool {
	filtered := func(tokens []Token) []Token {
		result := make([]Token, 0, len(tokens))
		for _, item := range tokens {
			switch item.Kind {
			case token.COMMENT, token.SEMICOLON, token.COMMA:
				continue
			default:
				result = append(result, item)
			}
		}
		return result
	}
	return slices.EqualFunc(filtered(before), filtered(after), func(left, right Token) bool {
		return left.Kind == right.Kind && left.Raw == right.Raw
	})
}

func syntaxFingerprint(file *ast.File) (string, error) {
	return syntaxNodeFingerprint(file)
}

func syntaxNodeFingerprint(node ast.Node) (string, error) {
	var output bytes.Buffer
	positionType := reflect.TypeFor[token.Pos]()
	err := ast.Fprint(&output, nil, node, func(name string, value reflect.Value) bool {
		if value.IsValid() && value.Type() == positionType {
			return false
		}
		switch name {
		case "Doc", "Comment", "Comments", "Obj", "Scope", "Unresolved":
			return false
		default:
			return true
		}
	})
	if err != nil {
		return "", fmt.Errorf("fingerprint normalized syntax: %w", err)
	}
	return output.String(), nil
}

func parsedTokenFile(fileSet *token.FileSet, syntax *ast.File) *token.File {
	if syntax != nil {
		if file := fileSet.File(syntax.Pos()); file != nil {
			return file
		}
	}
	var result *token.File
	fileSet.Iterate(func(file *token.File) bool {
		result = file
		return false
	})
	return result
}

func scanTokens(file *token.File, input []byte) ([]Token, error) {
	var scanErrors scanner.ErrorList
	var lexer scanner.Scanner
	lexer.Init(file, input, func(position token.Position, message string) {
		scanErrors.Add(position, message)
	}, scanner.ScanComments)

	result := make([]Token, 0, len(input)/4)
	for {
		position, kind, literal := lexer.Scan()
		if kind == token.EOF {
			break
		}
		start := file.Offset(position)
		end, err := physicalTokenEnd(input, start, kind, literal)
		if err != nil {
			return result, errors.Join(scanErrors.Err(), err)
		}
		semicolon := SemicolonNone
		if kind == token.SEMICOLON {
			if literal == ";" {
				semicolon = SemicolonExplicit
			} else {
				semicolon = SemicolonInserted
			}
		}
		result = append(result, Token{
			Kind:      kind,
			Range:     Range{Start: start, End: end},
			Raw:       string(input[start:end]),
			Semicolon: semicolon,
		})
	}
	return result, scanErrors.Err()
}

func physicalTokenEnd(input []byte, start int, kind token.Token, literal string) (int, error) {
	if start < 0 || start > len(input) {
		return 0, fmt.Errorf("token starts outside physical source: %d", start)
	}
	if kind == token.SEMICOLON && literal != ";" {
		return start, nil
	}
	if start == len(input) {
		return start, nil
	}

	switch kind {
	case token.COMMENT:
		return commentEnd(input, start)
	case token.ILLEGAL:
		_, width := utf8.DecodeRune(input[start:])
		if width == 0 {
			return 0, errors.New("illegal token has no physical width")
		}
		return start + width, nil
	case token.STRING:
		if input[start] == '`' {
			return rawStringEnd(input, start)
		}
		return quotedEnd(input, start, '"')
	case token.CHAR:
		return quotedEnd(input, start, '\'')
	}

	length := len(literal)
	if length == 0 {
		length = len(kind.String())
	}
	end := start + length
	if end > len(input) {
		return 0, fmt.Errorf("token %s extends outside physical source", kind)
	}
	return end, nil
}

func commentEnd(input []byte, start int) (int, error) {
	if bytes.HasPrefix(input[start:], []byte("//")) {
		end := start + 2
		for end < len(input) && input[end] != '\r' && input[end] != '\n' {
			end++
		}
		return end, nil
	}
	if bytes.HasPrefix(input[start:], []byte("/*")) {
		closing := bytes.Index(input[start+2:], []byte("*/"))
		if closing < 0 {
			return len(input), errors.New("unterminated block comment")
		}
		return start + 2 + closing + 2, nil
	}
	return 0, fmt.Errorf("comment token at byte %d has no comment prefix", start)
}

func rawStringEnd(input []byte, start int) (int, error) {
	closing := bytes.IndexByte(input[start+1:], '`')
	if closing < 0 {
		return len(input), errors.New("unterminated raw string")
	}
	return start + 1 + closing + 1, nil
}

func quotedEnd(input []byte, start int, quote byte) (int, error) {
	for offset := start + 1; offset < len(input); offset++ {
		switch input[offset] {
		case '\\':
			offset++
		case quote:
			return offset + 1, nil
		case '\r', '\n':
			return offset, errors.New("unterminated quoted literal")
		}
	}
	return len(input), errors.New("unterminated quoted literal")
}

func buildPieces(input []byte, tokens []Token) ([]Piece, error) {
	result := make([]Piece, 0, len(tokens)*2+1)
	cursor := 0
	for _, item := range tokens {
		if item.Range.End < item.Range.Start || item.Range.End > len(input) {
			return result, fmt.Errorf("invalid or overlapping token range [%d,%d)", item.Range.Start, item.Range.End)
		}
		if item.Range.Start == item.Range.End {
			continue
		}
		if item.Range.Start < cursor {
			return result, fmt.Errorf("invalid or overlapping token range [%d,%d)", item.Range.Start, item.Range.End)
		}
		if item.Range.Start > cursor {
			result = append(result, physicalPiece(PieceTrivia, input, cursor, item.Range.Start))
		}
		if item.Range.End > item.Range.Start {
			result = append(result, physicalPiece(PieceToken, input, item.Range.Start, item.Range.End))
			cursor = item.Range.End
		}
	}
	if cursor < len(input) {
		result = append(result, physicalPiece(PieceTrivia, input, cursor, len(input)))
	}
	return result, nil
}

func physicalPiece(kind PieceKind, input []byte, start, end int) Piece {
	return Piece{Kind: kind, Range: Range{Start: start, End: end}, Raw: bytes.Clone(input[start:end])}
}

func buildTrivia(input []byte, tokens []Token) []Trivia {
	result := make([]Trivia, 0, len(tokens)+1)
	appendGap := func(start, end int) {
		if start == end {
			return
		}
		if start == 0 && end >= 3 && bytes.Equal(input[:3], []byte{0xef, 0xbb, 0xbf}) {
			result = append(result, Trivia{Kind: TriviaBOM, Range: Range{Start: 0, End: 3}, Raw: string(input[:3])})
			start = 3
		}
		if start < end {
			result = append(result, Trivia{Kind: TriviaWhitespace, Range: Range{Start: start, End: end}, Raw: string(input[start:end])})
		}
	}

	cursor := 0
	for _, item := range tokens {
		if item.Range.Start == item.Range.End {
			continue
		}
		appendGap(cursor, item.Range.Start)
		cursor = item.Range.End
	}
	appendGap(cursor, len(input))
	return result
}

func buildComments(tokens []Token) []Comment {
	result := make([]Comment, 0)
	for _, item := range tokens {
		if item.Kind == token.COMMENT {
			result = append(result, Comment{ID: CommentID(len(result)), Range: item.Range, Raw: item.Raw})
		}
	}
	return result
}

func classifyDirectives(tokens []Token, syntax *ast.File, tokenFile *token.File) []Directive {
	result := make([]Directive, 0)
	for _, item := range tokens {
		if item.Kind != token.COMMENT {
			continue
		}
		kind, found := directiveKind(item.Raw)
		if found {
			result = append(result, Directive{Kind: kind, Range: item.Range, Raw: item.Raw})
		}
	}
	result = append(result, cgoDirectives(tokens, syntax, func(position token.Pos) (int, bool) {
		if !position.IsValid() || tokenFile == nil {
			return 0, false
		}
		offset := tokenFile.Offset(position)
		return offset, offset >= 0
	})...)
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].Range.Start != result[right].Range.Start {
			return result[left].Range.Start < result[right].Range.Start
		}
		return result[left].Kind < result[right].Kind
	})
	return result
}

func cgoDirectives(
	tokens []Token,
	syntax *ast.File,
	physicalOffset func(token.Pos) (int, bool),
) []Directive {
	if syntax == nil {
		return nil
	}
	commentsByStart := make(map[int]Token)
	for _, item := range tokens {
		if item.Kind == token.COMMENT {
			commentsByStart[item.Range.Start] = item
		}
	}
	var result []Directive
	for _, declaration := range syntax.Decls {
		imports, ok := declaration.(*ast.GenDecl)
		if !ok || imports.Tok != token.IMPORT {
			continue
		}
		for _, rawSpec := range imports.Specs {
			spec, ok := rawSpec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || path != "C" {
				continue
			}
			group := spec.Doc
			if group == nil && len(imports.Specs) == 1 {
				group = imports.Doc
			}
			if group == nil {
				continue
			}
			for _, item := range group.List {
				physicalStart, mapped := physicalOffset(item.Slash)
				if !mapped {
					continue
				}
				comment, found := commentsByStart[physicalStart]
				if found {
					result = append(result, Directive{Kind: DirectiveCgoPreamble, Range: comment.Range, Raw: comment.Raw})
				}
			}
		}
	}
	return result
}

func directiveKind(raw string) (DirectiveKind, bool) {
	switch {
	case constraint.IsGoBuild(raw), constraint.IsPlusBuild(raw):
		return DirectiveBuildConstraint, true
	case hasDirectivePrefix(raw, "//go:generate"):
		return DirectiveGoGenerate, true
	case hasDirectivePrefix(raw, "//go:embed"):
		return DirectiveGoEmbed, true
	case strings.HasPrefix(raw, "//line "), strings.HasPrefix(raw, "/*line "):
		return DirectiveLine, true
	case strings.HasPrefix(raw, "//gox:"):
		return DirectiveGoxSuppression, true
	case strings.HasPrefix(raw, "//go:"):
		return DirectiveCompiler, true
	case strings.HasPrefix(raw, "// Code generated ") && strings.HasSuffix(raw, " DO NOT EDIT."):
		return DirectiveGenerated, true
	default:
		return 0, false
	}
}

func hasDirectivePrefix(raw, prefix string) bool {
	return raw == prefix || strings.HasPrefix(raw, prefix+" ") || strings.HasPrefix(raw, prefix+"\t")
}

func validateDirectives(directives []Directive) error {
	var result error
	for _, directive := range directives {
		if directive.Kind != DirectiveBuildConstraint {
			continue
		}
		if _, err := constraint.Parse(directive.Raw); err != nil {
			result = errors.Join(result, fmt.Errorf("invalid build constraint at byte %d: %w", directive.Range.Start, err))
		}
	}
	return result
}

func inspectMetadata(input []byte, directives []Directive) Metadata {
	lf := 0
	crlf := 0
	for index, value := range input {
		if value != '\n' {
			continue
		}
		if index > 0 && input[index-1] == '\r' {
			crlf++
		} else {
			lf++
		}
	}
	newlines := NewlineNone
	switch {
	case lf > 0 && crlf > 0:
		newlines = NewlineMixed
	case lf > 0:
		newlines = NewlineLF
	case crlf > 0:
		newlines = NewlineCRLF
	}
	generated := false
	for _, directive := range directives {
		generated = generated || directive.Kind == DirectiveGenerated
	}
	return Metadata{
		HasBOM:       bytes.HasPrefix(input, []byte{0xef, 0xbb, 0xbf}),
		Newlines:     newlines,
		FinalNewline: len(input) > 0 && input[len(input)-1] == '\n',
		Generated:    generated,
	}
}
