package source_test

import (
	"bytes"
	"errors"
	"go/ast"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/source"
)

func TestSourceSyntaxViewsCannotMutateStoredState(t *testing.T) {
	t.Parallel()

	t.Run("complete file", func(t *testing.T) {
		file, err := source.Load("immutable.go", []byte("package original\nvar value = 1\n"))
		if err != nil {
			t.Fatal(err)
		}
		if err := file.ReadSyntax(func(syntax *ast.File) error {
			syntax.Name.Name = "mutated"
			syntax.Decls = nil
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		reference, err := source.Load("immutable.go", []byte("package original\nvar value = 1\n"))
		if err != nil {
			t.Fatal(err)
		}
		if err := source.ValidateEquivalent(file, reference); err != nil {
			t.Fatalf("stored file state changed through syntax view: %v", err)
		}
		if err := file.ReadSyntax(func(syntax *ast.File) error {
			if syntax.Name.Name != "original" || len(syntax.Decls) != 1 {
				t.Fatalf("stored file syntax was mutated: package %q, declarations %d", syntax.Name.Name, len(syntax.Decls))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("fragment", func(t *testing.T) {
		fragment, err := source.LoadFragment(
			"immutable.go",
			source.FragmentExpression,
			[]byte("left + right"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := fragment.ReadSyntax(func(syntax source.FragmentSyntax) error {
			binary := syntax.Expression.(*ast.BinaryExpr)
			binary.X.(*ast.Ident).Name = "mutated"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		reference, err := source.LoadFragment(
			"immutable.go",
			source.FragmentExpression,
			[]byte("left + right"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := source.ValidateFragmentEquivalent(fragment, reference); err != nil {
			t.Fatalf("stored fragment state changed through syntax view: %v", err)
		}
		if err := fragment.ReadSyntax(func(syntax source.FragmentSyntax) error {
			binary := syntax.Expression.(*ast.BinaryExpr)
			if got := binary.X.(*ast.Ident).Name; got != "left" {
				t.Fatalf("stored fragment syntax was mutated: left operand %q", got)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFileReadSyntaxViewProvidesMatchingFileSetAndRequiresCallbacks(t *testing.T) {
	t.Parallel()

	file, err := source.Load("nested/../source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.ReadSyntax(nil); err == nil {
		t.Fatal("ReadSyntax(nil) error = nil")
	}
	if err := file.ReadSyntaxView(nil); err == nil {
		t.Fatal("ReadSyntaxView(nil) error = nil")
	}
	if err := file.ReadSyntaxView(func(fileSet *token.FileSet, syntax *ast.File) error {
		tokenFile := fileSet.File(syntax.Pos())
		if tokenFile == nil {
			t.Fatal("syntax view has no matching token file")
		}
		if tokenFile.Name() != "source.go" || tokenFile.Offset(syntax.Name.Pos()) != len("package ") {
			t.Fatalf(
				"syntax view file = %q, package offset = %d",
				tokenFile.Name(),
				tokenFile.Offset(syntax.Name.Pos()),
			)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBuildsALosslessPhysicalSourceLedger(t *testing.T) {
	t.Parallel()

	input := []byte("\xef\xbb\xbf//go:build linux\r\n\r\npackage hostile\r\n\r\n//go:generate echo generated\r\nfunc run() { value := `first\rsecond`; value++ }\r\n")
	file, err := source.Load("./fixture.go", input)
	if err != nil {
		t.Fatal(err)
	}

	if got := file.Metadata(); !got.HasBOM || got.Newlines != source.NewlineCRLF || !got.FinalNewline {
		t.Fatalf("Metadata() = %#v, want BOM, CRLF, and final newline", got)
	}
	if got := reconstruct(file.Pieces()); !bytes.Equal(got, input) {
		t.Fatalf("physical ledger reconstructed %q, want exact input %q", got, input)
	}

	var explicit, inserted bool
	var rawLiteral string
	for _, item := range file.Tokens() {
		switch item.Semicolon {
		case source.SemicolonExplicit:
			explicit = true
		case source.SemicolonInserted:
			inserted = true
		}
		if item.Raw == "`first\rsecond`" {
			rawLiteral = item.Raw
		}
	}
	if !explicit || !inserted || rawLiteral == "" {
		t.Fatalf("token ledger lost semicolon origin or raw literal: explicit=%t inserted=%t raw=%q", explicit, inserted, rawLiteral)
	}
	packageOffset := bytes.Index(input, []byte("package"))
	packageRange, found := file.TokenRangeAtOffset(packageOffset)
	if !found || packageRange != (source.Range{Start: packageOffset, End: packageOffset + len("package")}) {
		t.Fatalf("TokenRangeAtOffset(package) = %#v, %v", packageRange, found)
	}
	if _, found := file.TokenRangeAtOffset(packageOffset + 1); found {
		t.Fatal("TokenRangeAtOffset() accepted an offset inside a token")
	}

	wantDirectives := []source.DirectiveKind{source.DirectiveBuildConstraint, source.DirectiveGoGenerate}
	gotDirectives := make([]source.DirectiveKind, 0, len(file.Directives()))
	for _, directive := range file.Directives() {
		gotDirectives = append(gotDirectives, directive.Kind)
	}
	if !slices.Equal(gotDirectives, wantDirectives) {
		t.Fatalf("Directives() kinds = %v, want %v", gotDirectives, wantDirectives)
	}
}

func TestLoadKeepsPhysicalOffsetsWhenLineDirectivesAdjustDiagnostics(t *testing.T) {
	t.Parallel()

	input := []byte("//line generated.go:100\npackage physical\nvar value = 1\n")
	file, err := source.Load("physical.go", input)
	if err != nil {
		t.Fatal(err)
	}

	var packageToken source.Token
	for _, item := range file.Tokens() {
		if item.Raw == "package" {
			packageToken = item
			break
		}
	}
	if packageToken.Range.Start != bytes.Index(input, []byte("package")) {
		t.Fatalf("package physical offset = %d, want %d", packageToken.Range.Start, bytes.Index(input, []byte("package")))
	}
	directives := file.Directives()
	if len(directives) != 1 || directives[0].Kind != source.DirectiveLine {
		t.Fatalf("Directives() = %#v, want one line directive", directives)
	}
}

func TestFilePositionUsesPhysicalByteLinesAndColumns(t *testing.T) {
	t.Parallel()

	input := []byte("//line generated.go:100\r\npackage sample\r\nvar β = 1\r\n")
	file, err := source.Load("physical.go", input)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		target string
		line   int
		column int
	}{
		{target: "package", line: 2, column: 1},
		{target: "β", line: 3, column: 5},
		{target: "=", line: 3, column: 8},
	}
	for _, test := range tests {
		offset := bytes.Index(input, []byte(test.target))
		position, found := file.Position(offset)
		if !found || position.Offset != offset || position.Line != test.line || position.Column != test.column {
			t.Fatalf("Position(%q) = %#v, %v", test.target, position, found)
		}
	}
	insideBeta := bytes.Index(input, []byte("β")) + 1
	if _, found := file.Position(insideBeta); found {
		t.Fatal("Position() accepted an offset inside a UTF-8 encoding")
	}
	if _, found := file.Position(len(input) + 1); found {
		t.Fatal("Position() accepted an out-of-bounds offset")
	}
}

func TestLoadReturnsDiagnosticOnlyStateForInvalidSource(t *testing.T) {
	t.Parallel()

	input := []byte("package invalid\nfunc broken( {\n")
	file, err := source.Load("invalid.go", input)
	if err == nil {
		t.Fatal("Load() must report invalid Go")
	}
	if file == nil || file.CanFormat() {
		t.Fatalf("Load() file = %#v, want a diagnostic-only source unit", file)
	}
	if got := reconstruct(file.Pieces()); !bytes.Equal(got, input) {
		t.Fatalf("invalid-source ledger reconstructed %q, want %q", got, input)
	}
}

func TestLoadFragmentKeepsSyntheticWrappersOutsideThePhysicalLedger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		kind             source.FragmentKind
		input            string
		wantDeclarations int
		wantStatements   int
		wantExpression   bool
	}{
		{
			name:             "declarations",
			kind:             source.FragmentDeclaration,
			input:            "var answer=42\nfunc run(){}",
			wantDeclarations: 2,
		},
		{
			name:           "statements",
			kind:           source.FragmentStatement,
			input:          "value:=1;value++",
			wantStatements: 2,
		},
		{
			name:           "expression",
			kind:           source.FragmentExpression,
			input:          "client.call(first, second)",
			wantExpression: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fragment, err := source.LoadFragment("stdin.go", test.kind, []byte(test.input))
			if err != nil {
				t.Fatal(err)
			}
			if got := reconstruct(fragment.Pieces()); !bytes.Equal(got, []byte(test.input)) {
				t.Fatalf("physical ledger reconstructed %q, want exact input %q", got, test.input)
			}
			for _, item := range fragment.Tokens() {
				if item.Range.Start < 0 || item.Range.End > len(test.input) {
					t.Fatalf("token range %#v escapes physical input", item.Range)
				}
				if item.Raw == "package" || item.Raw == "goxfragment" {
					t.Fatalf("physical token ledger contains synthetic token %q", item.Raw)
				}
			}
			err = fragment.ReadSyntax(func(syntax source.FragmentSyntax) error {
				if len(syntax.Declarations) != test.wantDeclarations {
					t.Fatalf("declaration count = %d, want %d", len(syntax.Declarations), test.wantDeclarations)
				}
				if len(syntax.Statements) != test.wantStatements {
					t.Fatalf("statement count = %d, want %d", len(syntax.Statements), test.wantStatements)
				}
				if (syntax.Expression != nil) != test.wantExpression {
					t.Fatalf("expression present = %t, want %t", syntax.Expression != nil, test.wantExpression)
				}
				var position int
				var found bool
				switch {
				case len(syntax.Declarations) > 0:
					position, found = fragment.PhysicalOffset(syntax.Declarations[0].Pos())
				case len(syntax.Statements) > 0:
					position, found = fragment.PhysicalOffset(syntax.Statements[0].Pos())
				case syntax.Expression != nil:
					position, found = fragment.PhysicalOffset(syntax.Expression.Pos())
				}
				if !found || position != 0 {
					t.Fatalf("first user node physical offset = %d, %t; want 0, true", position, found)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadFragmentRejectsBoundaryEscapeAndWrapperReliance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kind       source.FragmentKind
		input      string
		wantOffset int
	}{
		{
			name:       "statement boundary escape",
			kind:       source.FragmentStatement,
			input:      "}\nvar escaped = 1\nfunc reopened(){",
			wantOffset: 0,
		},
		{
			name:       "expression boundary escape",
			kind:       source.FragmentExpression,
			input:      "1)\nvar escaped = (2",
			wantOffset: 1,
		},
		{
			name:       "statement wrapper declaration",
			kind:       source.FragmentStatement,
			input:      "goxfragment()",
			wantOffset: 0,
		},
		{
			name:       "statement wrapper used as map key",
			kind:       source.FragmentStatement,
			input:      "_ = map[func()]int{goxfragment: 1}",
			wantOffset: len("_ = map[func()]int{"),
		},
		{
			name:       "ambiguous statement wrapper key",
			kind:       source.FragmentStatement,
			input:      "_ = T{goxfragment: 1}",
			wantOffset: len("_ = T{"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fragment, err := source.LoadFragment("stdin.go", test.kind, []byte(test.input))
			if err == nil {
				t.Fatal("LoadFragment() must reject content outside the selected user boundary")
			}
			var positionError *source.FragmentError
			if !errors.As(err, &positionError) {
				t.Fatalf("LoadFragment() error = %T %v, want physical FragmentError", err, err)
			}
			if positionError.Offset != test.wantOffset {
				t.Fatalf("fragment error offset = %d, want %d", positionError.Offset, test.wantOffset)
			}
			if strings.Contains(err.Error(), "goxfragment") {
				t.Fatalf("LoadFragment() error exposed synthetic identifier: %q", err)
			}
			if fragment == nil || fragment.CanFormat() {
				t.Fatalf("LoadFragment() fragment = %#v, want diagnostic-only state", fragment)
			}
		})
	}

	for _, input := range []string{
		"goxfragment := func(){}; goxfragment()",
		"_ = struct{ goxfragment int }{goxfragment: 1}",
	} {
		fragment, err := source.LoadFragment("stdin.go", source.FragmentStatement, []byte(input))
		if err != nil {
			t.Fatalf("LoadFragment(%q) rejected user-owned syntax: %v", input, err)
		}
		if !fragment.CanFormat() {
			t.Fatalf("user-owned wrapper-name syntax %q must remain formatable", input)
		}
	}
}

func TestLoadFragmentMapsParseErrorsToPhysicalInput(t *testing.T) {
	t.Parallel()

	input := []byte("value :=\n")
	fragment, err := source.LoadFragment("stdin.go", source.FragmentStatement, input)
	if err == nil {
		t.Fatal("LoadFragment() must report invalid statement syntax")
	}
	if fragment == nil || fragment.CanFormat() {
		t.Fatalf("LoadFragment() fragment = %#v, want diagnostic-only state", fragment)
	}
	var positionError *source.FragmentError
	if !errors.As(err, &positionError) {
		t.Fatalf("LoadFragment() error = %T %v, want FragmentError", err, err)
	}
	if positionError.Offset != len(input) {
		t.Fatalf("fragment error offset = %d, want %d", positionError.Offset, len(input))
	}
	if !strings.Contains(err.Error(), "stdin.go:2:1") {
		t.Fatalf("LoadFragment() error = %q, want physical line and column", err)
	}
	if strings.Contains(err.Error(), "goxfragment") {
		t.Fatalf("LoadFragment() error exposed synthetic identifier: %q", err)
	}
}

func TestLoadFragmentKeepsStatementParsingSyntaxOnly(t *testing.T) {
	t.Parallel()

	fragment, err := source.LoadFragment(
		"stdin.go",
		source.FragmentStatement,
		[]byte("goto missing\nvalue = unresolved"),
	)
	if err != nil {
		t.Fatalf("LoadFragment() required semantic resolution: %v", err)
	}
	if !fragment.CanFormat() {
		t.Fatal("syntactically valid unresolved statements must remain formatable")
	}
}

func TestLoadFragmentRejectsFilePlacementDirectives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		kind  source.FragmentKind
		input string
	}{
		{name: "build constraint", kind: source.FragmentDeclaration, input: "//go:build linux\nvar value int"},
		{name: "generated marker", kind: source.FragmentDeclaration, input: "// Code generated by fixture. DO NOT EDIT.\nvar value int"},
		{name: "cgo preamble", kind: source.FragmentDeclaration, input: "/*\n#cgo CFLAGS: -DVALUE=1\n*/\nimport \"C\""},
		{name: "line directive", kind: source.FragmentStatement, input: "//line generated.go:100\nvalue++"},
		{name: "compiler directive", kind: source.FragmentDeclaration, input: "//go:linkname local remote\nfunc local()"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fragment, err := source.LoadFragment("stdin.go", test.kind, []byte(test.input))
			if err == nil {
				t.Fatal("LoadFragment() must reject file-placement directives")
			}
			if fragment == nil || fragment.CanFormat() {
				t.Fatalf("LoadFragment() fragment = %#v, want diagnostic-only state", fragment)
			}
		})
	}
}

func TestLoadRejectsMalformedBuildConstraints(t *testing.T) {
	t.Parallel()

	input := []byte("// +build " + strings.Repeat("tag ", 102) + "\n\npackage invalid_constraint\n")
	file, err := source.Load("constraint.go", input)
	if err == nil {
		t.Fatal("Load() must reject a malformed build constraint")
	}
	if file == nil || file.CanFormat() {
		t.Fatalf("Load() file = %#v, want malformed constraint to be diagnostic-only", file)
	}
}

func TestSourceBytesAreReturnedByValue(t *testing.T) {
	t.Parallel()

	input := []byte("package immutable\n")
	file, err := source.Load("immutable.go", input)
	if err != nil {
		t.Fatal(err)
	}

	first := file.Bytes()
	first[0] = 'X'
	if got := file.Bytes(); !bytes.Equal(got, input) {
		t.Fatalf("Bytes() exposed mutable source storage: got %q, want %q", got, input)
	}
}

func TestLoadIndexesTriviaCommentsAndAnchoredDirectives(t *testing.T) {
	t.Parallel()

	input := []byte("\xef\xbb\xbf// Code generated by fixture. DO NOT EDIT.\npackage cgo_fixture\n\n/*\n#cgo CFLAGS: -DVALUE=1\n*/\nimport \"C\"\n")
	file, err := source.Load("cgo.go", input)
	if err != nil {
		t.Fatal(err)
	}

	trivia := file.Trivia()
	if len(trivia) == 0 || trivia[0].Kind != source.TriviaBOM || trivia[0].Raw != "\xef\xbb\xbf" {
		t.Fatalf("Trivia() = %#v, want a distinct leading BOM", trivia)
	}
	comments := file.Comments()
	if len(comments) != 2 || comments[0].Raw != "// Code generated by fixture. DO NOT EDIT." || comments[1].Raw != "/*\n#cgo CFLAGS: -DVALUE=1\n*/" {
		t.Fatalf("Comments() = %#v, want exact generated marker and cgo preamble", comments)
	}

	want := []source.DirectiveKind{source.DirectiveGenerated, source.DirectiveCgoPreamble}
	got := make([]source.DirectiveKind, 0, len(file.Directives()))
	for _, directive := range file.Directives() {
		got = append(got, directive.Kind)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Directives() kinds = %v, want %v", got, want)
	}
	if !file.Metadata().Generated {
		t.Fatal("Metadata().Generated must follow the generated-file marker")
	}
}

func TestDirectiveCorpusCoversEveryPrototypeDirectiveClass(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/corpus/hostile/directives.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("directives.go", input)
	if err != nil {
		t.Fatal(err)
	}
	want := []source.DirectiveKind{
		source.DirectiveBuildConstraint,
		source.DirectiveBuildConstraint,
		source.DirectiveGenerated,
		source.DirectiveCgoPreamble,
		source.DirectiveGoEmbed,
		source.DirectiveCompiler,
		source.DirectiveGoGenerate,
		source.DirectiveCompiler,
		source.DirectiveLine,
		source.DirectiveGoxSuppression,
	}
	got := make([]source.DirectiveKind, 0, len(file.Directives()))
	for _, directive := range file.Directives() {
		got = append(got, directive.Kind)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("directive corpus kinds = %v, want %v", got, want)
	}
}

func TestValidateEquivalentRejectsCommentMovementAcrossSignificantTokens(t *testing.T) {
	t.Parallel()

	before, err := source.Load("ownership.go", []byte("package ownership\nvar _ = combine(first /* keep */, second)\n"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := source.Load("ownership.go", []byte("package ownership\nvar _ = combine(first, second /* keep */)\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(before, after); err == nil {
		t.Fatal("ValidateEquivalent() must reject a comment moved across an argument")
	}
}

func TestValidateEquivalentAllowsCommentMovementAcrossFormatterPunctuation(t *testing.T) {
	t.Parallel()

	before, err := source.Load("punctuation.go", []byte("package ownership\nvar _ = combine(first /* keep */, second)\n"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := source.Load("punctuation.go", []byte("package ownership\nvar _ = combine(first, /* keep */ second)\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(before, after); err != nil {
		t.Fatalf("ValidateEquivalent() rejected formatter punctuation movement: %v", err)
	}
}

func TestValidateFragmentEquivalentRejectsSyntaxAndCommentOwnershipChanges(t *testing.T) {
	t.Parallel()

	before, err := source.LoadFragment(
		"fragment.go",
		source.FragmentExpression,
		[]byte("combine(first /* keep */, second)"),
	)
	if err != nil {
		t.Fatal(err)
	}
	commentMoved, err := source.LoadFragment(
		"fragment.go",
		source.FragmentExpression,
		[]byte("combine(first, second /* keep */)"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateFragmentEquivalent(before, commentMoved); err == nil {
		t.Fatal("ValidateFragmentEquivalent() must reject comment ownership movement")
	}

	syntaxChanged, err := source.LoadFragment(
		"fragment.go",
		source.FragmentExpression,
		[]byte("combine(first /* keep */, third)"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateFragmentEquivalent(before, syntaxChanged); err == nil {
		t.Fatal("ValidateFragmentEquivalent() must reject syntax changes")
	}
}

func TestEquivalenceRejectsDirectiveLineAnchorMovement(t *testing.T) {
	t.Parallel()

	beforeFile, err := source.Load(
		"directive.go",
		[]byte("package directive\nfunc run(){ //gox:ignore example because ownership matters\nwork()}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	afterFile, err := source.Load(
		"directive.go",
		[]byte("package directive\nfunc run(){\n//gox:ignore example because ownership matters\nwork()}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(beforeFile, afterFile); err == nil {
		t.Fatal("ValidateEquivalent() must reject directive line-anchor movement")
	}

	beforeFragment, err := source.LoadFragment(
		"directive.go",
		source.FragmentStatement,
		[]byte("if ready { //gox:ignore example because ownership matters\nwork()}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	afterFragment, err := source.LoadFragment(
		"directive.go",
		source.FragmentStatement,
		[]byte("if ready {\n//gox:ignore example because ownership matters\nwork()}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateFragmentEquivalent(beforeFragment, afterFragment); err == nil {
		t.Fatal("ValidateFragmentEquivalent() must reject directive line-anchor movement")
	}

	adjacentFile, err := source.Load(
		"directive.go",
		[]byte("package directive\nfunc run(){\n//gox:ignore example because ownership matters\nwork()}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	blankLineFile, err := source.Load(
		"directive.go",
		[]byte("package directive\nfunc run(){\n//gox:ignore example because ownership matters\n\nwork()}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(adjacentFile, blankLineFile); err == nil {
		t.Fatal("ValidateEquivalent() must reject a new blank line at a directive anchor")
	}

	indentedFile, err := source.Load(
		"directive.go",
		[]byte("package directive\nfunc run(){\n\t//gox:ignore example because ownership matters\nwork()}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(adjacentFile, indentedFile); err != nil {
		t.Fatalf("ValidateEquivalent() rejected indentation-only directive placement: %v", err)
	}

	generateAdjacent, err := source.Load(
		"directive.go",
		[]byte("package directive\n//go:generate go run example.invalid/generator\nfunc run(){}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	generateSeparated, err := source.Load(
		"directive.go",
		[]byte("package directive\n\n//go:generate go run example.invalid/generator\nfunc run(){}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(generateAdjacent, generateSeparated); err != nil {
		t.Fatalf("ValidateEquivalent() rejected canonical spacing before a declaration directive: %v", err)
	}

	generateTrailing, err := source.Load(
		"directive.go",
		[]byte("package directive\nvar value = 1 //go:generate go run example.invalid/generator\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	generateActivated, err := source.Load(
		"directive.go",
		[]byte("package directive\nvar value = 1\n//go:generate go run example.invalid/generator\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateEquivalent(generateTrailing, generateActivated); err == nil {
		t.Fatal("ValidateEquivalent() must reject a directive moved to line start")
	}
}

func reconstruct(pieces []source.Piece) []byte {
	var result []byte
	for _, piece := range pieces {
		result = append(result, piece.Raw...)
	}
	return result
}
