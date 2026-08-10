package source_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/source"
)

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

func reconstruct(pieces []source.Piece) []byte {
	var result []byte
	for _, piece := range pieces {
		result = append(result, piece.Raw...)
	}
	return result
}
