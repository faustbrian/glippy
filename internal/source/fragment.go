package source

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// FragmentKind identifies one explicitly selected standard-input grammar
// boundary.
type FragmentKind uint8

const (
	FragmentDeclaration FragmentKind = iota + 1
	FragmentStatement
	FragmentExpression
)

// FragmentSyntax is the single AST boundary selected from a synthetic parse.
// The field corresponding to the fragment kind is selected; an empty
// declaration or statement list may be nil.
type FragmentSyntax struct {
	Declarations []ast.Decl
	Statements   []ast.Stmt
	Expression   ast.Expr
}

// FragmentError reports one syntax failure at a physical fragment byte offset.
type FragmentError struct {
	Path    string
	Offset  int
	Line    int
	Column  int
	Message string
}

type fragmentBoundaryError struct {
	offset  int
	message string
}

func (e *fragmentBoundaryError) Error() string { return e.message }

func (e *FragmentError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Column, e.Message)
}

// Fragment is an immutable physical source fragment parsed through a fixed
// synthetic Go file. Synthetic wrapper bytes never enter its physical ledger.
type Fragment struct {
	kind       FragmentKind
	path       string
	bytes      []byte
	digest     Digest
	tokenFile  *token.File
	prefixSize int
	mappedSize int
	syntax     FragmentSyntax
	tokens     []Token
	pieces     []Piece
	trivia     []Trivia
	comments   []Comment
	directives []Directive
	metadata   Metadata
	parseErr   error
}

type fragmentWrapper struct {
	prefix                 string
	suffix                 string
	trimTrailingWhitespace bool
}

// LoadFragment constructs a physical source fragment using the fixed wrapper
// for kind. Invalid fragments retain their physical ledger for diagnostics.
func LoadFragment(path string, kind FragmentKind, input []byte) (*Fragment, error) {
	wrapper, err := wrapperForFragment(kind)
	if err != nil {
		return nil, err
	}
	physical := bytes.Clone(input)
	cleanPath := filepath.Clean(path)
	parsedInput := physical
	if wrapper.trimTrailingWhitespace {
		parsedInput = bytes.TrimRight(physical, " \t\r\n")
	}
	synthetic := make([]byte, 0, len(wrapper.prefix)+len(parsedInput)+len(wrapper.suffix))
	synthetic = append(synthetic, wrapper.prefix...)
	synthetic = append(synthetic, parsedInput...)
	synthetic = append(synthetic, wrapper.suffix...)

	fileSet := token.NewFileSet()
	// Retain the parser's lightweight object links only for statement fragments:
	// they distinguish a reference to the wrapper function from a user-owned
	// local shadow without loading types.
	parsed, parseErr := parser.ParseFile(fileSet, cleanPath, synthetic, parser.ParseComments)
	tokenFile := parsedTokenFile(fileSet, parsed)
	if tokenFile == nil {
		return nil, errors.New("Go parser did not register the synthetic fragment file")
	}

	physicalSet := token.NewFileSet()
	physicalFile := physicalSet.AddFile(cleanPath, -1, len(physical))
	tokens, scanErr := scanTokens(physicalFile, physical)
	pieces, ledgerErr := buildPieces(physical, tokens)
	trivia := buildTrivia(physical, tokens)
	comments := buildComments(tokens)
	directives := classifyFragmentDirectives(tokens, parsed, tokenFile, len(wrapper.prefix))
	metadata := inspectMetadata(physical, directives)
	directiveErr := errors.Join(
		validateDirectives(directives),
		validateFragmentDirectives(cleanPath, physical, directives),
	)

	var syntax FragmentSyntax
	boundaryErr := error(nil)
	if parseErr == nil {
		syntax, boundaryErr = selectFragmentSyntax(
			kind,
			parsed,
			tokenFile,
			len(wrapper.prefix),
			len(parsedInput),
		)
	}
	mappedParseErr := mapFragmentParseError(cleanPath, physical, len(wrapper.prefix), len(parsedInput), parseErr)
	mappedBoundaryErr := mapFragmentBoundaryError(cleanPath, physical, boundaryErr)
	loadErr := errors.Join(mappedParseErr, scanErr, ledgerErr, directiveErr, mappedBoundaryErr)

	return &Fragment{
		kind:       kind,
		path:       cleanPath,
		bytes:      physical,
		digest:     sha256.Sum256(physical),
		tokenFile:  tokenFile,
		prefixSize: len(wrapper.prefix),
		mappedSize: len(parsedInput),
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

func wrapperForFragment(kind FragmentKind) (fragmentWrapper, error) {
	switch kind {
	case FragmentDeclaration:
		return fragmentWrapper{prefix: "package goxfragment\n", suffix: "\n"}, nil
	case FragmentStatement:
		return fragmentWrapper{
			prefix: "package goxfragment\nfunc goxfragment() {\n",
			suffix: "\n}\n",
		}, nil
	case FragmentExpression:
		return fragmentWrapper{
			prefix:                 "package goxfragment\nvar _ = (\n",
			suffix:                 " )\n",
			trimTrailingWhitespace: true,
		}, nil
	default:
		return fragmentWrapper{}, fmt.Errorf("unknown fragment kind %d", kind)
	}
}

func selectFragmentSyntax(
	kind FragmentKind,
	file *ast.File,
	tokenFile *token.File,
	prefixSize int,
	physicalSize int,
) (FragmentSyntax, error) {
	if file == nil {
		return FragmentSyntax{}, errors.New("synthetic fragment parse has no syntax tree")
	}
	switch kind {
	case FragmentDeclaration:
		if err := validateFragmentNodes(file.Decls, tokenFile, prefixSize, physicalSize); err != nil {
			return FragmentSyntax{}, err
		}
		return FragmentSyntax{Declarations: slices.Clone(file.Decls)}, nil
	case FragmentStatement:
		if len(file.Decls) == 0 {
			return FragmentSyntax{}, &fragmentBoundaryError{message: "statement fragment changed its selected boundary"}
		}
		function, ok := file.Decls[0].(*ast.FuncDecl)
		if !ok || function.Name.Name != "goxfragment" || function.Body == nil {
			return FragmentSyntax{}, &fragmentBoundaryError{message: "statement fragment changed its selected boundary"}
		}
		closing := tokenFile.Offset(function.Body.Rbrace) - prefixSize
		if closing >= 0 && closing < physicalSize {
			return FragmentSyntax{}, &fragmentBoundaryError{
				offset: closing, message: "statement fragment escaped its selected boundary",
			}
		}
		if len(file.Decls) != 1 {
			offset := tokenFile.Offset(file.Decls[1].Pos()) - prefixSize
			return FragmentSyntax{}, &fragmentBoundaryError{
				offset: offset, message: "statement fragment escaped its selected boundary",
			}
		}
		if reference := syntheticFunctionReference(function); reference.IsValid() {
			return FragmentSyntax{}, &fragmentBoundaryError{
				offset:  tokenFile.Offset(reference) - prefixSize,
				message: "statement fragment relies on content outside its selected boundary",
			}
		}
		if err := validateFragmentNodes(function.Body.List, tokenFile, prefixSize, physicalSize); err != nil {
			return FragmentSyntax{}, err
		}
		return FragmentSyntax{Statements: slices.Clone(function.Body.List)}, nil
	case FragmentExpression:
		if len(file.Decls) == 0 {
			return FragmentSyntax{}, &fragmentBoundaryError{message: "expression fragment changed its selected boundary"}
		}
		declaration, ok := file.Decls[0].(*ast.GenDecl)
		if !ok || declaration.Tok != token.VAR || len(declaration.Specs) != 1 {
			return FragmentSyntax{}, &fragmentBoundaryError{message: "expression fragment changed its selected boundary"}
		}
		specification, ok := declaration.Specs[0].(*ast.ValueSpec)
		if !ok || len(specification.Values) != 1 {
			return FragmentSyntax{}, &fragmentBoundaryError{message: "expression fragment changed its selected boundary"}
		}
		parenthesized, ok := specification.Values[0].(*ast.ParenExpr)
		if !ok || parenthesized.X == nil {
			return FragmentSyntax{}, &fragmentBoundaryError{message: "expression fragment changed its selected boundary"}
		}
		closing := tokenFile.Offset(parenthesized.Rparen) - prefixSize
		if closing < physicalSize {
			return FragmentSyntax{}, &fragmentBoundaryError{
				offset: closing, message: "expression fragment escaped its selected boundary",
			}
		}
		if len(file.Decls) != 1 {
			offset := tokenFile.Offset(file.Decls[1].Pos()) - prefixSize
			return FragmentSyntax{}, &fragmentBoundaryError{
				offset: offset, message: "expression fragment escaped its selected boundary",
			}
		}
		if err := validateFragmentNodes([]ast.Expr{parenthesized.X}, tokenFile, prefixSize, physicalSize); err != nil {
			return FragmentSyntax{}, err
		}
		return FragmentSyntax{Expression: parenthesized.X}, nil
	default:
		return FragmentSyntax{}, fmt.Errorf("unknown fragment kind %d", kind)
	}
}

func validateFragmentNodes[T ast.Node](
	nodes []T,
	tokenFile *token.File,
	prefixSize int,
	physicalSize int,
) error {
	for _, node := range nodes {
		start := tokenFile.Offset(node.Pos()) - prefixSize
		end := tokenFile.Offset(node.End()) - prefixSize
		if start < 0 || end < start || end > physicalSize {
			if start < 0 {
				start = 0
			}
			if start > physicalSize {
				start = physicalSize
			}
			return &fragmentBoundaryError{offset: start, message: "fragment syntax escaped its selected boundary"}
		}
	}
	return nil
}

func syntheticFunctionReference(function *ast.FuncDecl) token.Pos {
	wrapper := function.Name.Obj
	if wrapper == nil {
		return token.NoPos
	}
	position := token.NoPos
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.CompositeLit); ok && !fragmentCompositeIsProvenStruct(literal.Type, nil) {
			for _, element := range literal.Elts {
				keyed, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				identifier, ok := keyed.Key.(*ast.Ident)
				if ok && identifier.Name == "goxfragment" && identifier.Obj == nil {
					position = identifier.Pos()
					return false
				}
			}
		}
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Obj == wrapper {
			position = identifier.Pos()
			return false
		}
		return !position.IsValid()
	})
	return position
}

func fragmentCompositeIsProvenStruct(expression ast.Expr, seen map[*ast.Object]struct{}) bool {
	switch value := expression.(type) {
	case *ast.StructType:
		return true
	case *ast.Ident:
		if value.Obj == nil {
			return false
		}
		if seen == nil {
			seen = make(map[*ast.Object]struct{})
		}
		if _, found := seen[value.Obj]; found {
			return false
		}
		seen[value.Obj] = struct{}{}
		specification, ok := value.Obj.Decl.(*ast.TypeSpec)
		if !ok {
			return false
		}
		return fragmentCompositeIsProvenStruct(specification.Type, seen)
	default:
		return false
	}
}

func classifyFragmentDirectives(
	tokens []Token,
	syntax *ast.File,
	tokenFile *token.File,
	prefixSize int,
) []Directive {
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
		offset := tokenFile.Offset(position) - prefixSize
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

func validateFragmentDirectives(path string, physical []byte, directives []Directive) error {
	var result error
	for _, directive := range directives {
		if directive.Kind == DirectiveGoxSuppression {
			continue
		}
		result = errors.Join(
			result,
			newFragmentError(
				path,
				physical,
				directive.Range.Start,
				fmt.Sprintf("directive at fragment byte %d requires complete-file placement", directive.Range.Start),
			),
		)
	}
	return result
}

func mapFragmentBoundaryError(path string, physical []byte, boundaryErr error) error {
	if boundaryErr == nil {
		return nil
	}
	var located *fragmentBoundaryError
	if !errors.As(boundaryErr, &located) {
		return newFragmentError(path, physical, 0, sanitizeFragmentMessage(boundaryErr.Error()))
	}
	return newFragmentError(path, physical, located.offset, sanitizeFragmentMessage(located.message))
}

func mapFragmentParseError(
	path string,
	physical []byte,
	prefixSize int,
	parsedSize int,
	parseErr error,
) error {
	if parseErr == nil {
		return nil
	}
	var parseErrors scanner.ErrorList
	if !errors.As(parseErr, &parseErrors) {
		return newFragmentError(path, physical, 0, sanitizeFragmentMessage(parseErr.Error()))
	}
	var result error
	for _, parseError := range parseErrors {
		offset := parseError.Pos.Offset - prefixSize
		if offset < 0 {
			offset = 0
		}
		if offset > parsedSize {
			offset = parsedSize
		}
		if offset > len(physical) {
			offset = len(physical)
		}
		result = errors.Join(
			result,
			newFragmentError(path, physical, offset, sanitizeFragmentMessage(parseError.Msg)),
		)
	}
	return result
}

func newFragmentError(path string, physical []byte, offset int, message string) *FragmentError {
	if offset < 0 {
		offset = 0
	}
	if offset > len(physical) {
		offset = len(physical)
	}
	line, column := fragmentLineColumn(physical, offset)
	return &FragmentError{
		Path: path, Offset: offset, Line: line, Column: column, Message: message,
	}
}

func fragmentLineColumn(physical []byte, offset int) (int, int) {
	line := 1
	lineStart := 0
	for index, value := range physical[:offset] {
		if value == '\n' {
			line++
			lineStart = index + 1
		}
	}
	return line, offset - lineStart + 1
}

func sanitizeFragmentMessage(message string) string {
	return strings.ReplaceAll(message, "goxfragment", "fragment wrapper")
}

// ValidateFragmentEquivalent verifies normalized syntax and physical
// source-accounting invariants for two fragments of the same kind.
func ValidateFragmentEquivalent(before, after *Fragment) error {
	if before == nil || after == nil || !before.CanFormat() || !after.CanFormat() {
		return errors.New("fragment equivalence requires two valid source units")
	}
	if before.kind != after.kind {
		return errors.New("fragment kind changed")
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
	beforeSyntax, err := fragmentSyntaxFingerprint(before.kind, before.syntax)
	if err != nil {
		return err
	}
	afterSyntax, err := fragmentSyntaxFingerprint(after.kind, after.syntax)
	if err != nil {
		return err
	}
	if beforeSyntax != afterSyntax {
		return errors.New("normalized fragment syntax changed")
	}
	return nil
}

func fragmentSyntaxFingerprint(kind FragmentKind, syntax FragmentSyntax) (string, error) {
	switch kind {
	case FragmentExpression:
		return syntaxNodeFingerprint(syntax.Expression)
	case FragmentStatement:
		return syntaxNodeFingerprint(&ast.BlockStmt{List: syntax.Statements})
	case FragmentDeclaration:
		return syntaxNodeFingerprint(&ast.File{Decls: syntax.Declarations})
	default:
		return "", fmt.Errorf("unknown fragment kind %d", kind)
	}
}

// Kind returns the explicit grammar boundary used to parse the fragment.
func (f *Fragment) Kind() FragmentKind { return f.kind }

// Path returns the normalized physical source identity supplied to LoadFragment.
func (f *Fragment) Path() string { return f.path }

// Digest returns the exact physical fragment digest.
func (f *Fragment) Digest() Digest { return f.digest }

// Bytes returns an independent copy of the physical fragment bytes.
func (f *Fragment) Bytes() []byte { return bytes.Clone(f.bytes) }

// Tokens returns the physical token ledger without synthetic wrapper tokens.
func (f *Fragment) Tokens() []Token { return slices.Clone(f.tokens) }

// Pieces returns the physical reconstruction ledger without wrapper bytes.
func (f *Fragment) Pieces() []Piece {
	result := make([]Piece, len(f.pieces))
	for index, piece := range f.pieces {
		result[index] = piece
		result[index].Raw = bytes.Clone(piece.Raw)
	}
	return result
}

// Trivia returns exact physical token gaps in source order.
func (f *Fragment) Trivia() []Trivia { return slices.Clone(f.trivia) }

// Comments returns exact physical comments with stable fragment identities.
func (f *Fragment) Comments() []Comment { return slices.Clone(f.comments) }

// Directives returns classified physical comment directives.
func (f *Fragment) Directives() []Directive { return slices.Clone(f.directives) }

// Metadata returns physical fragment metadata.
func (f *Fragment) Metadata() Metadata { return f.metadata }

// CanFormat reports whether parsing and physical reconstruction accepted the
// selected fragment boundary.
func (f *Fragment) CanFormat() bool { return f.parseErr == nil }

// ReadSyntax provides an isolated selected AST boundary to a run-owned
// consumer.
func (f *Fragment) ReadSyntax(read func(FragmentSyntax) error) error {
	if f.parseErr != nil {
		return f.parseErr
	}
	wrapper, err := wrapperForFragment(f.kind)
	if err != nil {
		return err
	}
	parsedInput := f.bytes
	if wrapper.trimTrailingWhitespace {
		parsedInput = bytes.TrimRight(f.bytes, " \t\r\n")
	}
	synthetic := make([]byte, 0, len(wrapper.prefix)+len(parsedInput)+len(wrapper.suffix))
	synthetic = append(synthetic, wrapper.prefix...)
	synthetic = append(synthetic, parsedInput...)
	synthetic = append(synthetic, wrapper.suffix...)
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, f.path, synthetic, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("construct immutable fragment syntax view: %w", err)
	}
	tokenFile := parsedTokenFile(fileSet, parsed)
	if tokenFile == nil {
		return errors.New("immutable fragment syntax view has no token file")
	}
	syntax, err := selectFragmentSyntax(
		f.kind,
		parsed,
		tokenFile,
		len(wrapper.prefix),
		len(parsedInput),
	)
	if err != nil {
		return fmt.Errorf("construct immutable fragment syntax view: %w", err)
	}
	return read(syntax)
}

// RawToken returns the exact physical token spelling at a synthetic AST
// position owned by the fragment.
func (f *Fragment) RawToken(position token.Pos) (string, bool) {
	offset, found := f.PhysicalOffset(position)
	if !found {
		return "", false
	}
	index, found := slices.BinarySearchFunc(f.tokens, offset, func(item Token, target int) int {
		return item.Range.Start - target
	})
	if !found {
		return "", false
	}
	return f.tokens[index].Raw, true
}

// PhysicalOffset maps a selected synthetic AST position to the exact fragment
// byte offset. Wrapper positions are deliberately unmappable.
func (f *Fragment) PhysicalOffset(position token.Pos) (int, bool) {
	if !position.IsValid() || f.tokenFile == nil {
		return 0, false
	}
	offset := f.tokenFile.Offset(position) - f.prefixSize
	return offset, offset >= 0 && offset <= f.mappedSize
}

// Slice returns exact physical fragment text for a valid byte range.
func (f *Fragment) Slice(sourceRange Range) (string, bool) {
	if sourceRange.Start < 0 || sourceRange.End < sourceRange.Start || sourceRange.End > len(f.bytes) {
		return "", false
	}
	return string(f.bytes[sourceRange.Start:sourceRange.End]), true
}
