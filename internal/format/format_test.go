package format_test

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/source"
)

func TestFormatExpandsMotivatingHostileGo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		width int
		fixture string
	}{
		{name: "compressed if block", width: 100, fixture: "compressed-if"},
		{
			name: "ordinary statement semicolons",
			width: 100,
			fixture: "statement-semicolons",
		},
		{name: "boolean chain", width: 24, fixture: "boolean-chain"},
		{name: "long call", width: 30, fixture: "long-call"},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				input, err := os.ReadFile(
					"../../testdata/format/motivating/" +
						test.fixture +
						".input",
				)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(
					"../../testdata/format/motivating/" +
						test.fixture +
						".golden",
				)
				if err != nil {
					t.Fatal(err)
				}
				file, err := source.Load(test.fixture + ".go", input)
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
				}
				reparsed, err := source.Load("formatted.go", got)
				if err != nil {
					t.Fatalf("formatted output does not parse: %v", err)
				}
				again, err := goxformat.File(
					reparsed,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(again, got) {
					t.Fatalf(
						"formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s",
						got,
						again,
					)
				}
			},
		)
	}
}

func TestFormatFragmentsAtTheirSelectedUserBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind source.FragmentKind
		width int
		input string
		want string
	}{
		{
			name: "declarations",
			kind: source.FragmentDeclaration,
			width: 100,
			input: "var answer=42\nfunc run(){}",
			want: "var answer = 42\n\nfunc run() {}\n",
		},
		{
			name: "statements",
			kind: source.FragmentStatement,
			width: 100,
			input: "ctx,cancel:=context.WithCancel(t.Context());cancel();result:=work(ctx)",
			want: "ctx, cancel := context.WithCancel(t.Context())\ncancel()\nresult := work(ctx)\n",
		},
		{
			name: "statement groups",
			kind: source.FragmentStatement,
			width: 100,
			input: "first()\n\n\nsecond();\n\nthird()",
			want: "first()\n\nsecond()\nthird()\n",
		},
		{
			name: "expression",
			kind: source.FragmentExpression,
			width: 20,
			input: "foo && bar && baz && somethingReallyLong\n",
			want: "foo &&\n\tbar &&\n\tbaz &&\n\tsomethingReallyLong\n",
		},
		{
			name: "empty declarations",
			kind: source.FragmentDeclaration,
			width: 100,
			input: " \n",
			want: "\n",
		},
		{
			name: "empty statements",
			kind: source.FragmentStatement,
			width: 100,
			input: " \n",
			want: "\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				fragment, err := source.LoadFragment(
					"stdin.go",
					test.kind,
					[]byte(test.input),
				)
				if err != nil {
					t.Fatal(err)
				}
				options := goxformat.Options{
					Width: test.width,
					TabWidth: 8,
					FitBudget: 1_000,
				}
				got, err := goxformat.Fragment(fragment, options)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != test.want {
					t.Fatalf(
						"formatted fragment =\n%s\nwant:\n%s",
						got,
						test.want,
					)
				}
				reparsed, err := source.LoadFragment("stdin.go", test.kind, got)
				if err != nil {
					t.Fatalf("formatted fragment did not reparse: %v", err)
				}
				again, err := goxformat.Fragment(reparsed, options)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, again) {
					t.Fatalf(
						"fragment is not byte-idempotent:\nfirst:\n%s\nsecond:\n%s",
						got,
						again,
					)
				}
			},
		)
	}
}

func TestFormatFragmentsPreserveCommentsOwnedInsideTheSelectedBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind source.FragmentKind
		input string
		want string
	}{
		{
			name: "declaration comments",
			kind: source.FragmentDeclaration,
			input: "// Value documents the declaration.\nvar Value=1 // keep trailing\n",
			want: "// Value documents the declaration.\nvar Value = 1 // keep trailing\n",
		},
		{
			name: "statement comments",
			kind: source.FragmentStatement,
			input: "//gox:ignore example because this is a fixture\nvalue:=call(/* keep argument */ first,second) // keep trailing\n",
			want: "//gox:ignore example because this is a fixture\nvalue := call(\n\t/* keep argument */\n\tfirst,\n\tsecond,\n) // keep trailing\n",
		},
		{
			name: "expression comments",
			kind: source.FragmentExpression,
			input: "/* leading */ foo+/* middle */bar /* trailing */\n",
			want: "/* leading */\nfoo +\n\t/* middle */ bar /* trailing */\n",
		},
		{
			name: "expression leading line comment",
			kind: source.FragmentExpression,
			input: "// keep leading\nfoo+bar\n",
			want: "// keep leading\nfoo + bar\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				fragment, err := source.LoadFragment(
					"stdin.go",
					test.kind,
					[]byte(test.input),
				)
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.Fragment(
					fragment,
					goxformat.Options{
						Width: 100,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != test.want {
					t.Fatalf(
						"formatted fragment =\n%s\nwant:\n%s",
						got,
						test.want,
					)
				}
			},
		)
	}
}

func TestFormatPreservesFieldTypeBoundaryComments(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		"comment.go",
		[]byte("package comments\nfunc run(value /* keep me */ int){_=value}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run(value /* keep me */ int) {\n\t_ = value\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want field type-boundary comment preserved", got)
	}
}

func TestFormatPreservesFieldListBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc run(/* first parameter */ value int,other string /* trailing parameter */)(/* first result */ string,error /* trailing result */){return \"\",nil}\nfunc Generic[/* first type parameter */ T any,U comparable /* trailing type parameter */](value T){}\nfunc empty(/* empty parameter list */){}\n",
	)
	file, err := source.Load("field_lists.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run(\n\t/* first parameter */\n\tvalue int,\n\tother string, /* trailing parameter */\n) (\n\t/* first result */\n\tstring,\n\terror, /* trailing result */\n) {\n\treturn \"\", nil\n}\n\nfunc Generic[\n\t/* first type parameter */\n\tT any,\n\tU comparable, /* trailing type parameter */\n](value T) {}\n\nfunc empty(\n\t/* empty parameter list */\n) {}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesFieldListLineCommentsAfterCommas(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc run(value int, // first parameter\nother string, // second parameter\n){}\n",
	)
	file, err := source.Load("field_line_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run(\n\tvalue int, // first parameter\n\tother string, // second parameter\n) {}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesExpressionListBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc use(){Generic[/* first type argument */ string,int /* trailing type argument */](/* first argument */ first,second /* trailing argument */);Single[/* single type argument */ string](value);empty(/* empty argument list */);values:=[]int{/* empty composite literal */};_=values}\n",
	)
	file, err := source.Load("expression_lists.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc use() {\n\tGeneric[\n\t\t/* first type argument */\n\t\tstring,\n\t\tint, /* trailing type argument */\n\t](\n\t\t/* first argument */\n\t\tfirst,\n\t\tsecond, /* trailing argument */\n\t)\n\tSingle[\n\t\t/* single type argument */\n\t\tstring](value)\n\tempty(\n\t\t/* empty argument list */\n\t)\n\tvalues := []int{\n\t\t/* empty composite literal */\n\t}\n\t_ = values\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesNonListDelimiterComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/non-list-delimiters.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/non-list-delimiters.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("non_list_delimiters.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesIfHeaderComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/if-header.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/if-header.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("if_header.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesForHeaderComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/for-header.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/for-header.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("for_header.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesRangeBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/range-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/range-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("range_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesSwitchHeaderComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/switch-headers.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/switch-headers.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("switch_headers.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesCaseHeaderComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/case-headers.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/case-headers.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("case_headers.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesCommunicationHeaderComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/communication-headers.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/communication-headers.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("communication_headers.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesCommunicationOperatorComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/communication-operators.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/communication-operators.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("communication_operators.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesValueSpecBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/value-spec-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/value-spec-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("value_spec_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesIncrementAndDecrementOperatorComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/inc-dec-operators.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/inc-dec-operators.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("inc_dec_operators.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesStatementKeywordBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/statement-keywords.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/statement-keywords.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("statement_keywords.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesGeneralDeclarationBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/declaration-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/declaration-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("declaration_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesDotBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/dot-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/dot-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("dot_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesKeyValueColonComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/key-value-colons.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/key-value-colons.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("key_value_colons.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesSliceColonComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/slice-colons.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/slice-colons.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("slice_colons.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesTypeSpecBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/type-spec-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/type-spec-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("type_spec_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesFieldBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/field-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/field-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("field_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesFunctionDeclarationBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile(
		"../../testdata/format/comments/function-declaration-boundaries.input",
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(
		"../../testdata/format/comments/function-declaration-boundaries.golden",
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("function_declaration_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesFunctionTypeAndLiteralBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/function-type-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/function-type-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("function_type_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesTypeConstructorBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile(
		"../../testdata/format/comments/type-constructor-boundaries.input",
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(
		"../../testdata/format/comments/type-constructor-boundaries.golden",
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("type_constructor_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
	reparsed, err := source.Load("formatted_type_constructor_boundaries.go", got)
	if err != nil {
		t.Fatalf("formatted output does not parse: %v", err)
	}
	again, err := goxformat.File(
		reparsed,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, got) {
		t.Fatalf("formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestFormatPreservesPostfixBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/postfix-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/postfix-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("postfix_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
	reparsed, err := source.Load("formatted_postfix_boundaries.go", got)
	if err != nil {
		t.Fatalf("formatted output does not parse: %v", err)
	}
	again, err := goxformat.File(
		reparsed,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, got) {
		t.Fatalf("formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestFormatPreservesFilePrefixCommentsAndDirectives(t *testing.T) {
	t.Parallel()

	input := []byte(
		"//go:build linux\n\n// Package prefix documents the package.\npackage prefix\nfunc run(){}\n",
	)
	file, err := source.Load("prefix.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "//go:build linux\n\n// Package prefix documents the package.\npackage prefix\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want exact anchored prefix comments", got)
	}
}

func TestFormatCanonicalizesStructuralLineEndings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		want string
	}{
		{
			name: "crlf",
			input: "package crlf\r\nfunc run(){}\r\n",
			want: "package crlf\n\nfunc run() {}\n",
		},
		{
			name: "mixed without final newline",
			input: "package mixed\r\nfunc run(){}",
			want: "package mixed\n\nfunc run() {}\n",
		},
		{
			name: "bom and prefix line directive",
			input: "\xef\xbb\xbf//line generated.go:100\r\npackage physical\r\nfunc run(){}",
			want: "\xef\xbb\xbf//line generated.go:100\npackage physical\n\nfunc run() {}\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				file, err := source.Load("line_endings.go", []byte(test.input))
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: 100,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != test.want {
					t.Fatalf(
						"File() = %q, want canonical structural line endings",
						got,
					)
				}
			},
		)
	}
}

func TestFormatPreservesDeclarationDocumentationAndTrailingComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\n//go:generate go run example.invalid/generator\n// run performs work.\nfunc run(){} // keep trailing\n",
	)
	file, err := source.Load("comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\n//go:generate go run example.invalid/generator\n// run performs work.\nfunc run() {} // keep trailing\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want declaration comments and directive anchored", got)
	}
}

func TestFormatPreservesNolintLineOwnershipWhenLayoutWouldBreak(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc run(){\n_,err=veryLongCall(nil,firstArgument,secondArgument,thirdArgument) //nolint:staticcheck // contract test\nif err:=veryLongCall(nil,firstArgument,secondArgument,thirdArgument);err!=nil { //nolint:staticcheck // contract test\npanic(err)\n}\nreturn 16 + 36*uint8((uint16(red)*5+127)/255) + //nolint:gosec\n\t6*uint8((uint16(green)*5+127)/255) + uint8((uint16(blue)*5+127)/255) //nolint:gosec\n}\n",
	)
	file, err := source.Load("nolint.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 60, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run() {\n\t_,err=veryLongCall(nil,firstArgument,secondArgument,thirdArgument) //nolint:staticcheck // contract test\n\tif err:=veryLongCall(nil,firstArgument,secondArgument,thirdArgument);err!=nil { //nolint:staticcheck // contract test\n\t\tpanic(err)\n\t}\n\treturn 16 + 36*uint8((uint16(red)*5+127)/255) + //nolint:gosec\n\t6*uint8((uint16(green)*5+127)/255) + uint8((uint16(blue)*5+127)/255) //nolint:gosec\n}\n"
	if string(got) != want {
		t.Fatalf(
			"File() =\n%s\nwant nolint comments on their original physical lines:\n%s",
			got,
			want,
		)
	}
}

func TestFormatPreservesStandaloneTopLevelDirectives(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\n\n//go:generate go run example.invalid/generator\n\nfunc run(){}\n",
	)
	file, err := source.Load("directive.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\n//go:generate go run example.invalid/generator\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want standalone directive boundary preserved", got)
	}
}

func TestFormatPreservesFieldDocumentationAndTrailingComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\ntype Config struct{// Value documents the field.\nValue string `json:\"value\"` // keep field\n}\n",
	)
	file, err := source.Load("fields.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\ntype Config struct {\n\t// Value documents the field.\n\tValue string `json:\"value\"` // keep field\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want field comments anchored", got)
	}
}

func TestFormatPreservesCommentsBeforeAggregateClosers(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\ntype Config struct{Value string\n// after last field\n}\ntype Empty struct{/* inside empty aggregate */}\n",
	)
	file, err := source.Load("aggregate_closers.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\ntype Config struct {\n\tValue string\n\t// after last field\n}\n\ntype Empty struct {\n\t/* inside empty aggregate */\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want aggregate closing-boundary comments anchored", got)
	}
}

func TestFormatPreservesStatementBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc run(){// before first\nfirst() // after first\n// between statements\nsecond()\n// before close\n}\n",
	)
	file, err := source.Load("statements.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run() {\n\t// before first\n\tfirst() // after first\n\t// between statements\n\tsecond()\n\t// before close\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want statement boundary comments anchored", got)
	}
}

func TestFormatPreservesBlankLinesBetweenStatementGroups(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package grouping\nfunc run(){first()\n\n\nsecond();\n\nthird()\n\n// grouped\nfourth()\n// attached\n\nfifth()\nsixth() /* trailing\n\ncomment */\nseventh()\n\n// before close\n\n}\n",
	)
	file, err := source.Load("statement_groups.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package grouping\n\nfunc run() {\n\tfirst()\n\n\tsecond()\n\tthird()\n\n\t// grouped\n\tfourth()\n\t// attached\n\n\tfifth()\n\tsixth() /* trailing\n\ncomment */\n\tseventh()\n\n\t// before close\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesBlankLinesAfterBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\ntype Config struct{// standalone field\n\nValue string\n}\nfunc run(){// standalone statement\n\nwork()}\n",
	)
	file, err := source.Load("blank_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\ntype Config struct {\n\t// standalone field\n\n\tValue string\n}\n\nfunc run() {\n\t// standalone statement\n\n\twork()\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesOperandAndListElementComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc add(a,b int)int{return a /* left operand */ + /* right operand */ b}\nfunc values()[]int{return []int{1 /* first */,2 /* second */,3}}\n",
	)
	file, err := source.Load("expressions.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc add(a, b int) int {\n\treturn a /* left operand */ + /* right operand */ b\n}\n\nfunc values() []int {\n\treturn []int{\n\t\t1, /* first */\n\t\t2, /* second */\n\t\t3,\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesLineCommentsAfterBinaryOperators(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package comments\nfunc condition(first,second bool)bool{return first || // keep logical\nsecond}\nfunc total(first,second int)int{return 1+first+ // keep arithmetic\nsecond}\nfunc mixed(first,second int)int{return first+ // first line\n/* middle */ // second line\nsecond}\n",
	)
	file, err := source.Load("binary_line_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc condition(first, second bool) bool {\n\treturn first || // keep logical\n\t\tsecond\n}\n\nfunc total(first, second int) int {\n\treturn 1 +\n\t\tfirst + // keep arithmetic\n\t\tsecond\n}\n\nfunc mixed(first, second int) int {\n\treturn first + // first line\n\t\t/* middle */ // second line\n\t\tsecond\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesCallArgumentComments(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		"arguments.go",
		[]byte(
			"package comments\nfunc run(){use(first /* first */,second /* second */)}\n",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run() {\n\tuse(\n\t\tfirst, /* first */\n\t\tsecond, /* second */\n\t)\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want call argument comments anchored", got)
	}
}

func TestFormatInitialCorpus(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("../../testdata/corpus/hostile/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("initial corpus is empty")
	}
	gofmtDivergences := map[string]string{
		"blocks": "intentional width-aware if-header break",
		"compatibility": "preserved import order, literal spelling, parentheses, and unaligned layout",
		"empty-statements": "preserved explicit and implicit empty-statement spelling",
	}
	for name := range gofmtDivergences {
		path := "../../testdata/corpus/hostile/" + name + ".go"
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("gofmt divergence names missing corpus input %s: %v", path, err)
		}
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()

				input, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(
					"../../testdata/corpus/hostile/" + name + ".golden",
				)
				if err != nil {
					t.Fatal(err)
				}
				file, err := source.Load(name + ".go", input)
				if err != nil {
					t.Fatal(err)
				}
				options := goxformat.Options{
					Width: 60,
					TabWidth: 8,
					FitBudget: 1_000,
				}
				got, err := goxformat.File(file, options)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
				}
				reparsed, err := source.Load("formatted_" + name + ".go", got)
				if err != nil {
					t.Fatalf("formatted output does not parse: %v", err)
				}
				again, err := goxformat.File(reparsed, options)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(again, got) {
					t.Fatalf(
						"formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s",
						got,
						again,
					)
				}
				gofmtOutput, err := format.Source(got)
				if err != nil {
					t.Fatalf(
						"gofmt rejected output under %s: %v",
						runtime.Version(),
						err,
					)
				}
				gofmtFixedPoint := bytes.Equal(gofmtOutput, got)
				gofmtDivergence := gofmtDivergences[name]
				gofmtGoldenPath := "../../testdata/corpus/hostile/" +
					name +
					".gofmt.golden"
				if gofmtDivergence == "" {
					if _, err := os.Stat(gofmtGoldenPath); !os.IsNotExist(err) {
						t.Fatalf(
							"fixed-point corpus input has unexpected gofmt golden %s",
							gofmtGoldenPath,
						)
					}
				}
				if gofmtDivergence == "" && !gofmtFixedPoint {
					t.Fatalf(
						"output is not a gofmt fixed point under %s:\ngofmt:\n%s\ngox:\n%s",
						runtime.Version(),
						gofmtOutput,
						got,
					)
				}
				if gofmtDivergence != "" {
					if gofmtFixedPoint {
						t.Fatalf(
							"recorded gofmt divergence %q no longer occurs under %s",
							gofmtDivergence,
							runtime.Version(),
						)
					}
					wantGofmt, err := os.ReadFile(gofmtGoldenPath)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(gofmtOutput, wantGofmt) {
						t.Fatalf(
							"recorded gofmt divergence %q changed under %s:\ngofmt:\n%s\nwant:\n%s",
							gofmtDivergence,
							runtime.Version(),
							gofmtOutput,
							wantGofmt,
						)
					}
					t.Logf("gofmt divergence: %s", gofmtDivergence)
				}
			},
		)
	}
}

func TestFormatCorpusAcrossRepresentativeWidths(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("../../testdata/corpus/hostile/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("initial corpus is empty")
	}
	responsiveFiles := 0
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(
			name,
			func(t *testing.T) {
				input, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				file, err := source.Load(name + ".go", input)
				if err != nil {
					t.Fatal(err)
				}
				outputs := make(map[string]struct{}, 4)
				for _, width := range []int{20, 60, 100, 120} {
					options := goxformat.Options{
						Width: width,
						TabWidth: 8,
						FitBudget: 1_000,
					}
					formatted, err := goxformat.File(file, options)
					if err != nil {
						t.Fatalf("width %d: %v", width, err)
					}
					reparsed, err := source.Load(
						"formatted_" + name + ".go",
						formatted,
					)
					if err != nil {
						t.Fatalf(
							"width %d output does not parse: %v",
							width,
							err,
						)
					}
					again, err := goxformat.File(reparsed, options)
					if err != nil {
						t.Fatalf(
							"width %d repeat formatting failed: %v",
							width,
							err,
						)
					}
					if !bytes.Equal(formatted, again) {
						t.Fatalf(
							"width %d formatting is not idempotent",
							width,
						)
					}
					outputs[string(formatted)] = struct{}{}
				}
				if len(outputs) > 1 {
					responsiveFiles++
				}
			},
		)
	}
	if responsiveFiles == 0 {
		t.Fatal("corpus output did not respond to configured width")
	}
}

func TestFormatLowersControlFlowAndStatementSurface(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc run(values []int,ch chan int){var total int;for i:=0;i<len(values);i++{total+=values[i];if total>10{break}else{continue}};for total<20{total++};for{break};for index,value:=range values{_=index;total+=value};go work();defer close(ch);ch<-total;Block:{goto Block}}\n",
	)
	file, err := source.Load("control.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc run(values []int, ch chan int) {\n\tvar total int\n\tfor i := 0; i < len(values); i++ {\n\t\ttotal += values[i]\n\t\tif total > 10 {\n\t\t\tbreak\n\t\t} else {\n\t\t\tcontinue\n\t\t}\n\t}\n\tfor total < 20 {\n\t\ttotal++\n\t}\n\tfor {\n\t\tbreak\n\t}\n\tfor index, value := range values {\n\t\t_ = index\n\t\ttotal += value\n\t}\n\tgo work()\n\tdefer close(ch)\n\tch <- total\nBlock:\n\t{\n\t\tgoto Block\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesExplicitClassicForClauses(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc run(ready bool){for ;ready;{work()};for ;;{break}}\n",
	)
	file, err := source.Load("classic_for.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc run(ready bool) {\n\tfor ; ready; {\n\t\twork()\n\t}\n\tfor ; ; {\n\t\tbreak\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesExplicitAndImplicitEmptyStatements(t *testing.T) {
	t.Parallel()

	input := []byte("package empty\nfunc run(){;;work();label:;goto label;done:}\n")
	file, err := source.Load("empty.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package empty\n\nfunc run() {\n\t;\n\t;\n\twork()\nlabel:\n\t;\n\tgoto label\ndone:\n}\n"
	if string(got) != want {
		t.Fatalf(
			"File() = %q, want explicit empty statements and implicit closing label",
			got,
		)
	}
}

func TestFormatDoesNotMistakeNestedSemicolonsForClassicForClause(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc run(ready bool){for func()bool{work();return ready}(){break}}\n",
	)
	file, err := source.Load("nested_for.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc run(ready bool) {\n\tfor func() bool {\n\t\twork()\n\t\treturn ready\n\t}() {\n\t\tbreak\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLowersSwitchTypeSwitchAndSelectClauses(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc classify(value any,ready <-chan int,done <-chan struct{})string{switch size:=len([]int{1});size{case 0:return \"empty\";case 1,2:// small\nreturn \"small\";default:return \"many\"};switch current:=value.(type){case string:return current;case nil:return \"nil\";default:return \"other\"};select{case item:=<-ready:_=item;case <-done:return \"done\";case ready<-1:return \"sent\";default:return \"waiting\"}}\n",
	)
	file, err := source.Load("clauses.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc classify(value any, ready <-chan int, done <-chan struct{}) string {\n\tswitch size := len([]int{1}); size {\n\tcase 0:\n\t\treturn \"empty\"\n\tcase 1, 2:\n\t\t// small\n\t\treturn \"small\"\n\tdefault:\n\t\treturn \"many\"\n\t}\n\tswitch current := value.(type) {\n\tcase string:\n\t\treturn current\n\tcase nil:\n\t\treturn \"nil\"\n\tdefault:\n\t\treturn \"other\"\n\t}\n\tselect {\n\tcase item := <-ready:\n\t\t_ = item\n\tcase <-done:\n\t\treturn \"done\"\n\tcase ready <- 1:\n\t\treturn \"sent\"\n\tdefault:\n\t\treturn \"waiting\"\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatBreaksControlFlowHeaders(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc loop(){for index:=startingIndex;index<collectionLength;index++{work()}}\nfunc iterate(){for index,value:=range valuesWithAnExtremelyLongName{_,_=index,value}}\nfunc choose(inputValue int){switch currentValue:=inputValue;currentValue{case FirstVeryLongValue,SecondVeryLongValue,ThirdVeryLongValue:work()}}\nfunc classify(inputValue any){switch prepared:=inputValue;current:=prepared.(type){case string:_=current}}\nfunc communicate(){select{case receivedValue:=<-incomingValuesChannel:_=receivedValue;case outgoingValuesChannelWithLongName<-value:return}}\n",
	)
	file, err := source.Load("broken_control.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 44, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc loop() {\n\tfor index := startingIndex;\n\t\tindex < collectionLength;\n\t\tindex++ {\n\t\twork()\n\t}\n}\n\nfunc iterate() {\n\tfor index, value := range\n\t\tvaluesWithAnExtremelyLongName {\n\t\t_, _ = index, value\n\t}\n}\n\nfunc choose(inputValue int) {\n\tswitch currentValue := inputValue;\n\t\tcurrentValue {\n\tcase FirstVeryLongValue,\n\t\tSecondVeryLongValue,\n\t\tThirdVeryLongValue:\n\t\twork()\n\t}\n}\n\nfunc classify(inputValue any) {\n\tswitch prepared := inputValue;\n\t\tcurrent := prepared.(type) {\n\tcase string:\n\t\t_ = current\n\t}\n}\n\nfunc communicate() {\n\tselect {\n\tcase receivedValue :=\n\t\t<-incomingValuesChannel:\n\t\t_ = receivedValue\n\tcase outgoingValuesChannelWithLongName <-\n\t\tvalue:\n\t\treturn\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatUsesDeterministicCaseListWidthBoundary(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc choose(value int){switch value{case FirstVeryLongValue,SecondVeryLongValue,ThirdVeryLongValue:return}}\n",
	)
	flat := "package control\n\nfunc choose(value int) {\n\tswitch value {\n\tcase FirstVeryLongValue, SecondVeryLongValue, ThirdVeryLongValue:\n\t\treturn\n\t}\n}\n"
	broken := "package control\n\nfunc choose(value int) {\n\tswitch value {\n\tcase FirstVeryLongValue,\n\t\tSecondVeryLongValue,\n\t\tThirdVeryLongValue:\n\t\treturn\n\t}\n}\n"
	for _, test := range
		[]struct {
			name string
			width int
			want string
		}{
			{name: "exact fit", width: 73, want: flat},
			{name: "one column under", width: 72, want: broken},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				file, err := source.Load("case_boundary.go", input)
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != test.want {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
				}
			},
		)
	}
}

func TestFormatUsesDeterministicIfInitializerWidthBoundary(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc choose(){if current:=initialValue;current!=expectedVal{work()}}\n",
	)
	flat := "package control\n\nfunc choose() {\n\tif current := initialValue; current != expectedVal {\n\t\twork()\n\t}\n}\n"
	broken := "package control\n\nfunc choose() {\n\tif current := initialValue;\n\t\tcurrent != expectedVal {\n\t\twork()\n\t}\n}\n"
	for _, test := range
		[]struct {
			name string
			width int
			want string
		}{
			{name: "exact fit", width: 60, want: flat},
			{name: "one column under", width: 59, want: broken},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				file, err := source.Load("if_boundary.go", input)
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != test.want {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
				}
			},
		)
	}
}

func TestFormatUsesDeterministicControlFlowHeaderWidthBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		width int
		flat string
		broken string
	}{
		{
			name: "if condition",
			input: "package boundary\nfunc choose(){if conditionWithLongName{work()}}\n",
			width: 34,
			flat: "package boundary\n\nfunc choose() {\n\tif conditionWithLongName {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc choose() {\n\tif conditionWithLongName {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "for condition",
			input: "package boundary\nfunc loop(){for conditionWithLongName{work()}}\n",
			width: 35,
			flat: "package boundary\n\nfunc loop() {\n\tfor conditionWithLongName {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc loop() {\n\tfor conditionWithLongName {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "switch tag",
			input: "package boundary\nfunc choose(){switch subjectWithLongName{default:work()}}\n",
			width: 36,
			flat: "package boundary\n\nfunc choose() {\n\tswitch subjectWithLongName {\n\tdefault:\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc choose() {\n\tswitch subjectWithLongName {\n\tdefault:\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "type switch guard",
			input: "package boundary\nfunc classify(inputValue any){switch current:=inputValue.(type){case string:_=current}}\n",
			width: 45,
			flat: "package boundary\n\nfunc classify(inputValue any) {\n\tswitch current := inputValue.(type) {\n\tcase string:\n\t\t_ = current\n\t}\n}\n",
			broken: "package boundary\n\nfunc classify(inputValue any) {\n\tswitch current := inputValue.(type) {\n\tcase string:\n\t\t_ = current\n\t}\n}\n",
		},
		{
			name: "classic for",
			input: "package boundary\nfunc loop(){for index:=startingIndex;index<collectionLength;index++{work()}}\n",
			width: 71,
			flat: "package boundary\n\nfunc loop() {\n\tfor index := startingIndex; index < collectionLength; index++ {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc loop() {\n\tfor index := startingIndex;\n\t\tindex < collectionLength;\n\t\tindex++ {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "classic for without initializer",
			input: "package boundary\nfunc loop(){for ;conditionWithLongName;indexWithLongName++{work()}}\n",
			width: 58,
			flat: "package boundary\n\nfunc loop() {\n\tfor ; conditionWithLongName; indexWithLongName++ {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc loop() {\n\tfor ;\n\t\tconditionWithLongName;\n\t\tindexWithLongName++ {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "classic for without condition",
			input: "package boundary\nfunc loop(){for indexWithLongName:=startingValue;;indexWithLongName++{work()}}\n",
			width: 71,
			flat: "package boundary\n\nfunc loop() {\n\tfor indexWithLongName := startingValue; ; indexWithLongName++ {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc loop() {\n\tfor indexWithLongName := startingValue;\n\t\t;\n\t\tindexWithLongName++ {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "classic for without post",
			input: "package boundary\nfunc loop(){for indexWithLongName:=startingValue;conditionWithLongName;{work()}}\n",
			width: 72,
			flat: "package boundary\n\nfunc loop() {\n\tfor indexWithLongName := startingValue; conditionWithLongName; {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc loop() {\n\tfor indexWithLongName := startingValue;\n\t\tconditionWithLongName; {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "range for",
			input: "package boundary\nfunc iterate(){for index,value:=range valuesWithLongName{_,_=index,value}}\n",
			width: 54,
			flat: "package boundary\n\nfunc iterate() {\n\tfor index, value := range valuesWithLongName {\n\t\t_, _ = index, value\n\t}\n}\n",
			broken: "package boundary\n\nfunc iterate() {\n\tfor index, value := range\n\t\tvaluesWithLongName {\n\t\t_, _ = index, value\n\t}\n}\n",
		},
		{
			name: "bare range for",
			input: "package boundary\nfunc iterate(){for range valuesWithLongName{work()}}\n",
			width: 38,
			flat: "package boundary\n\nfunc iterate() {\n\tfor range valuesWithLongName {\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc iterate() {\n\tfor range\n\t\tvaluesWithLongName {\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "single declaration range for",
			input: "package boundary\nfunc iterate(){for indexWithLongName:=range valuesWithLongName{_=indexWithLongName}}\n",
			width: 59,
			flat: "package boundary\n\nfunc iterate() {\n\tfor indexWithLongName := range valuesWithLongName {\n\t\t_ = indexWithLongName\n\t}\n}\n",
			broken: "package boundary\n\nfunc iterate() {\n\tfor indexWithLongName := range\n\t\tvaluesWithLongName {\n\t\t_ = indexWithLongName\n\t}\n}\n",
		},
		{
			name: "single assignment range for",
			input: "package boundary\nfunc iterate(){for indexWithLongName=range valuesWithLongName{_=indexWithLongName}}\n",
			width: 58,
			flat: "package boundary\n\nfunc iterate() {\n\tfor indexWithLongName = range valuesWithLongName {\n\t\t_ = indexWithLongName\n\t}\n}\n",
			broken: "package boundary\n\nfunc iterate() {\n\tfor indexWithLongName = range\n\t\tvaluesWithLongName {\n\t\t_ = indexWithLongName\n\t}\n}\n",
		},
		{
			name: "switch initializer",
			input: "package boundary\nfunc choose(inputValue int){switch currentValue:=inputValue;currentValue{default:work()}}\n",
			width: 57,
			flat: "package boundary\n\nfunc choose(inputValue int) {\n\tswitch currentValue := inputValue; currentValue {\n\tdefault:\n\t\twork()\n\t}\n}\n",
			broken: "package boundary\n\nfunc choose(inputValue int) {\n\tswitch currentValue := inputValue;\n\t\tcurrentValue {\n\tdefault:\n\t\twork()\n\t}\n}\n",
		},
		{
			name: "type switch initializer",
			input: "package boundary\nfunc classify(inputValue any){switch prepared:=inputValue;current:=prepared.(type){case string:_=current}}\n",
			width: 67,
			flat: "package boundary\n\nfunc classify(inputValue any) {\n\tswitch prepared := inputValue; current := prepared.(type) {\n\tcase string:\n\t\t_ = current\n\t}\n}\n",
			broken: "package boundary\n\nfunc classify(inputValue any) {\n\tswitch prepared := inputValue;\n\t\tcurrent := prepared.(type) {\n\tcase string:\n\t\t_ = current\n\t}\n}\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				for _, mode := range
					[]struct {
						name string
						width int
						want string
					}{
						{
							name: "exact fit",
							width: test.width,
							want: test.flat,
						},
						{
							name: "one column under",
							width: test.width - 1,
							want: test.broken,
						},
					} {
					t.Run(
						mode.name,
						func(t *testing.T) {
							t.Parallel()

							file, err := source.Load(
								"control_flow_boundary.go",
								[]byte(test.input),
							)
							if err != nil {
								t.Fatal(err)
							}
							got, err := goxformat.File(
								file,
								goxformat.Options{
									Width: mode.width,
									TabWidth: 8,
									FitBudget: 1_000,
								},
							)
							if err != nil {
								t.Fatal(err)
							}
							if string(got) != mode.want {
								t.Fatalf(
									"File() =\n%s\nwant:\n%s",
									got,
									mode.want,
								)
							}
						},
					)
				}
			},
		)
	}
}

func TestFormatBreaksInsideControlFlowOperandBeforeKeywordBoundary(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc choose(){if firstCondition&&secondCondition&&thirdCondition{work()}}\n",
	)
	file, err := source.Load("control_flow_operand.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 36, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc choose() {\n\tif firstCondition &&\n\t\tsecondCondition &&\n\t\tthirdCondition {\n\t\twork()\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatUsesDeterministicCommunicationWidthBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		width int
		flat string
		broken string
	}{
		{
			name: "send statement",
			input: "package boundary\nfunc send(){outgoingValuesChannel<-valueWithLongName}\n",
			width: 50,
			flat: "package boundary\n\nfunc send() {\n\toutgoingValuesChannel <- valueWithLongName\n}\n",
			broken: "package boundary\n\nfunc send() {\n\toutgoingValuesChannel <-\n\t\tvalueWithLongName\n}\n",
		},
		{
			name: "select receive assignment",
			input: "package boundary\nfunc receive(){select{case receivedValue:=<-incomingValuesChannel:_=receivedValue}}\n",
			width: 54,
			flat: "package boundary\n\nfunc receive() {\n\tselect {\n\tcase receivedValue := <-incomingValuesChannel:\n\t\t_ = receivedValue\n\t}\n}\n",
			broken: "package boundary\n\nfunc receive() {\n\tselect {\n\tcase receivedValue :=\n\t\t<-incomingValuesChannel:\n\t\t_ = receivedValue\n\t}\n}\n",
		},
		{
			name: "select send",
			input: "package boundary\nfunc sendCase(){select{case outgoingValuesChannel<-valueWithLongName:return}}\n",
			width: 56,
			flat: "package boundary\n\nfunc sendCase() {\n\tselect {\n\tcase outgoingValuesChannel <- valueWithLongName:\n\t\treturn\n\t}\n}\n",
			broken: "package boundary\n\nfunc sendCase() {\n\tselect {\n\tcase outgoingValuesChannel <-\n\t\tvalueWithLongName:\n\t\treturn\n\t}\n}\n",
		},
		{
			name: "select receive expression",
			input: "package boundary\nfunc wait(){select{case <-incomingValuesChannel:return}}\n",
			width: 37,
			flat: "package boundary\n\nfunc wait() {\n\tselect {\n\tcase <-incomingValuesChannel:\n\t\treturn\n\t}\n}\n",
			broken: "package boundary\n\nfunc wait() {\n\tselect {\n\tcase <-incomingValuesChannel:\n\t\treturn\n\t}\n}\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				for _, mode := range
					[]struct {
						name string
						width int
						want string
					}{
						{
							name: "exact fit",
							width: test.width,
							want: test.flat,
						},
						{
							name: "one column under",
							width: test.width - 1,
							want: test.broken,
						},
					} {
					t.Run(
						mode.name,
						func(t *testing.T) {
							t.Parallel()

							file, err := source.Load(
								"communication_boundary.go",
								[]byte(test.input),
							)
							if err != nil {
								t.Fatal(err)
							}
							got, err := goxformat.File(
								file,
								goxformat.Options{
									Width: mode.width,
									TabWidth: 8,
									FitBudget: 1_000,
								},
							)
							if err != nil {
								t.Fatal(err)
							}
							if string(got) != mode.want {
								t.Fatalf(
									"File() =\n%s\nwant:\n%s",
									got,
									mode.want,
								)
							}
						},
					)
				}
			},
		)
	}
}

func TestFormatUsesDeterministicSelectorChainWidthBoundary(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/chains/selector-call.input")
	if err != nil {
		t.Fatal(err)
	}
	flat, err := os.ReadFile("../../testdata/format/chains/selector-call.flat.golden")
	if err != nil {
		t.Fatal(err)
	}
	broken, err := os.ReadFile("../../testdata/format/chains/selector-call.broken.golden")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range
		[]struct {
			name string
			width int
			want []byte
		}{
			{name: "exact fit", width: 49, want: flat},
			{name: "one column under", width: 48, want: broken},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				file, err := source.Load("selector_call.go", input)
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, test.want) {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
				}
			},
		)
	}
}

func TestFormatKeepsSelectorCalleeFlatWhenArgumentsBreak(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package selector\nfunc run(){result:=l.arena.Concat(firstArgument,secondArgument);_=result}\n",
	)
	file, err := source.Load("selector_callee.go", input)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range
		[]struct {
			name string
			width int
			want string
		}{
			{
				name: "callee fits",
				width: 48,
				want: "package selector\n\nfunc run() {\n\tresult := l.arena.Concat(\n\t\tfirstArgument,\n\t\tsecondArgument,\n\t)\n\t_ = result\n}\n",
			},
			{
				name: "callee and opening delimiter exceed width",
				width: 32,
				want: "package selector\n\nfunc run() {\n\tresult := l.\n\t\tarena.\n\t\tConcat(\n\t\t\tfirstArgument,\n\t\t\tsecondArgument,\n\t\t)\n\t_ = result\n}\n",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != test.want {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
				}
			},
		)
	}
}

func TestFormatUsesDeterministicDelimitedListWidthBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		width int
		flat string
		broken string
	}{
		{
			name: "call arguments",
			input: "package boundary\nfunc run(){use(firstArgument,secondArgument)}\n",
			width: 42,
			flat: "package boundary\n\nfunc run() {\n\tuse(firstArgument, secondArgument)\n}\n",
			broken: "package boundary\n\nfunc run() {\n\tuse(\n\t\tfirstArgument,\n\t\tsecondArgument,\n\t)\n}\n",
		},
		{
			name: "composite literal elements",
			input: "package boundary\nfunc run(){items:=[]Item{firstValue,secondValue};_=items}\n",
			width: 48,
			flat: "package boundary\n\nfunc run() {\n\titems := []Item{firstValue, secondValue}\n\t_ = items\n}\n",
			broken: "package boundary\n\nfunc run() {\n\titems := []Item{\n\t\tfirstValue,\n\t\tsecondValue,\n\t}\n\t_ = items\n}\n",
		},
		{
			name: "type arguments",
			input: "package boundary\nfunc run(){value:=NewPair[string,int];_=value}\n",
			width: 37,
			flat: "package boundary\n\nfunc run() {\n\tvalue := NewPair[string, int]\n\t_ = value\n}\n",
			broken: "package boundary\n\nfunc run() {\n\tvalue := NewPair[\n\t\tstring,\n\t\tint,\n\t]\n\t_ = value\n}\n",
		},
		{
			name: "function type parameters",
			input: "package boundary\ntype Pair[Key comparable,Value any] struct{}\n",
			width: 45,
			flat: "package boundary\n\ntype Pair[Key comparable, Value any] struct{}\n",
			broken: "package boundary\n\ntype Pair[\n\tKey comparable,\n\tValue any,\n] struct{}\n",
		},
		{
			name: "function parameters",
			input: "package boundary\nfunc transform(firstArgument int,secondArgument string){}\n",
			width: 59,
			flat: "package boundary\n\nfunc transform(firstArgument int, secondArgument string) {}\n",
			broken: "package boundary\n\nfunc transform(\n\tfirstArgument int,\n\tsecondArgument string,\n) {}\n",
		},
		{
			name: "function results",
			input: "package boundary\nfunc transform()(firstResult int,secondResult error){}\n",
			width: 57,
			flat: "package boundary\n\nfunc transform() (firstResult int, secondResult error) {}\n",
			broken: "package boundary\n\nfunc transform() (\n\tfirstResult int,\n\tsecondResult error,\n) {}\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				for _, mode := range
					[]struct {
						name string
						width int
						want string
					}{
						{
							name: "exact fit",
							width: test.width,
							want: test.flat,
						},
						{
							name: "one column under",
							width: test.width - 1,
							want: test.broken,
						},
					} {
					t.Run(
						mode.name,
						func(t *testing.T) {
							t.Parallel()

							file, err := source.Load(
								"delimited_boundary.go",
								[]byte(test.input),
							)
							if err != nil {
								t.Fatal(err)
							}
							got, err := goxformat.File(
								file,
								goxformat.Options{
									Width: mode.width,
									TabWidth: 8,
									FitBudget: 1_000,
								},
							)
							if err != nil {
								t.Fatal(err)
							}
							if string(got) != mode.want {
								t.Fatalf(
									"File() =\n%s\nwant:\n%s",
									got,
									mode.want,
								)
							}
						},
					)
				}
			},
		)
	}
}

func TestFormatUsesDeterministicBinaryChainWidthBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		input string
		width int
		flat string
		broken string
	}{
		{
			name: "boolean expression",
			input: "package boundary\nfunc condition()bool{return firstCondition&&secondCondition&&thirdCondition}\n",
			width: 66,
			flat: "package boundary\n\nfunc condition() bool {\n\treturn firstCondition && secondCondition && thirdCondition\n}\n",
			broken: "package boundary\n\nfunc condition() bool {\n\treturn firstCondition &&\n\t\tsecondCondition &&\n\t\tthirdCondition\n}\n",
		},
		{
			name: "type union",
			input: "package boundary\ntype Number interface{~int|~int64|~float64}\n",
			width: 32,
			flat: "package boundary\n\ntype Number interface {\n\t~int | ~int64 | ~float64\n}\n",
			broken: "package boundary\n\ntype Number interface {\n\t~int |\n\t\t~int64 |\n\t\t~float64\n}\n",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				for _, mode := range
					[]struct {
						name string
						width int
						want string
					}{
						{
							name: "exact fit",
							width: test.width,
							want: test.flat,
						},
						{
							name: "one column under",
							width: test.width - 1,
							want: test.broken,
						},
					} {
					t.Run(
						mode.name,
						func(t *testing.T) {
							t.Parallel()

							file, err := source.Load(
								"binary_boundary.go",
								[]byte(test.input),
							)
							if err != nil {
								t.Fatal(err)
							}
							got, err := goxformat.File(
								file,
								goxformat.Options{
									Width: mode.width,
									TabWidth: 8,
									FitBudget: 1_000,
								},
							)
							if err != nil {
								t.Fatal(err)
							}
							if string(got) != mode.want {
								t.Fatalf(
									"File() =\n%s\nwant:\n%s",
									got,
									mode.want,
								)
							}
						},
					)
				}
			},
		)
	}
}

func TestFormatKeepsDocumentedAtomicConstructsIntactWhenOverWidth(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package atomic\nfunc unary(){_=!conditionWithAnExtremelyLongName}\nfunc index(){_=values[indexWithAnExtremelyLongName]}\nfunc slice(){_=values[lowerBoundWithLongName:upperBoundWithLongName]}\nfunc assertion(){_=value.(TypeWithAnExtremelyLongName)}\nfunc increment(){counterWithAnExtremelyLongName++}\n",
	)
	file, err := source.Load("atomic.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 30, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package atomic\n\nfunc unary() {\n\t_ = !conditionWithAnExtremelyLongName\n}\n\nfunc index() {\n\t_ = values[indexWithAnExtremelyLongName]\n}\n\nfunc slice() {\n\t_ = values[lowerBoundWithLongName:upperBoundWithLongName]\n}\n\nfunc assertion() {\n\t_ = value.(TypeWithAnExtremelyLongName)\n}\n\nfunc increment() {\n\tcounterWithAnExtremelyLongName++\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatKeepsOrdinaryAssignmentOperatorWithRightHandSide(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package assignment\nfunc run(){result,err:=client.executeContent(ctx,OperationInfo,http.MethodGet,\"/\",nil,\"application/json\",200);value:=identifierWithAnExtremelyLongName;_,_,_=result,err,value}\nfunc comment(){value:= // keep\notherValue;_=value}\n",
	)
	file, err := source.Load("assignment.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 48, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package assignment\n\nfunc run() {\n\tresult, err := client.executeContent(\n\t\tctx,\n\t\tOperationInfo,\n\t\thttp.MethodGet,\n\t\t\"/\",\n\t\tnil,\n\t\t\"application/json\",\n\t\t200,\n\t)\n\tvalue := identifierWithAnExtremelyLongName\n\t_, _, _ = result, err, value\n}\n\nfunc comment() {\n\tvalue :=\n\t\t// keep\n\t\totherValue\n\t_ = value\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatKeepsSelectorAssignmentTargetFlatWhenValueBreaks(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package selector\nfunc run(){execution.outcome.Rejected=append([]fixengine.Rejection(nil),transaction.Result.Rejected...)}\n",
	)
	file, err := source.Load("selector_assignment.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package selector\n\nfunc run() {\n\texecution.outcome.Rejected = append(\n\t\t[]fixengine.Rejection(nil),\n\t\ttransaction.Result.Rejected...,\n\t)\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatBreaksSelectorAssignmentTargetThatDoesNotFit(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package selector\nfunc run(){executionWithAnExtremelyLongName.outcomeWithAnExtremelyLongName.Rejected=value}\n",
	)
	file, err := source.Load("selector_assignment.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 60, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package selector\n\nfunc run() {\n\texecutionWithAnExtremelyLongName.\n\t\toutcomeWithAnExtremelyLongName.\n\t\tRejected = value\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatUsesDeterministicGenericSelectorChainWidthBoundary(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/chains/selector-generic.input")
	if err != nil {
		t.Fatal(err)
	}
	flat, err := os.ReadFile("../../testdata/format/chains/selector-generic.flat.golden")
	if err != nil {
		t.Fatal(err)
	}
	broken, err := os.ReadFile("../../testdata/format/chains/selector-generic.golden")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range
		[]struct {
			name string
			width int
			want []byte
		}{
			{name: "exact fit", width: 55, want: flat},
			{name: "one column under", width: 54, want: broken},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				file, err := source.Load("selector_generic.go", input)
				if err != nil {
					t.Fatal(err)
				}
				got, err := goxformat.File(
					file,
					goxformat.Options{
						Width: test.width,
						TabWidth: 8,
						FitBudget: 1_000,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, test.want) {
					t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
				}
			},
		)
	}
}

func TestFormatPreservesClauseBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package control\nfunc run(value int,ready <-chan int){switch value{case 1:first() // trailing\n// between cases\ncase 2:second()\n// before switch close\n};select{case <-ready:use() // trailing\n// between communications\ndefault:wait()\n// before select close\n}}\n",
	)
	file, err := source.Load("clause_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc run(value int, ready <-chan int) {\n\tswitch value {\n\tcase 1:\n\t\tfirst() // trailing\n\t// between cases\n\tcase 2:\n\t\tsecond()\n\t// before switch close\n\t}\n\tselect {\n\tcase <-ready:\n\t\tuse() // trailing\n\t// between communications\n\tdefault:\n\t\twait()\n\t// before select close\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesAcceptedByteOrderMark(t *testing.T) {
	t.Parallel()

	file, err := source.Load("bom.go", []byte("\xef\xbb\xbfpackage bom\nfunc run(){}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbfpackage bom\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want preserved byte-order mark", got)
	}
}

func TestFormatPreservesVariadicCallEllipsis(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		"variadic.go",
		[]byte("package variadic\nfunc run(){use(values...)}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package variadic\n\nfunc run() {\n\tuse(values...)\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want call ellipsis preserved", got)
	}
}

func TestFormatRejectsDirectiveAnchorMovementWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	options := goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000}
	file, err := source.Load(
		"directive.go",
		[]byte(
			"package directive\nfunc run(){ //gox:ignore example because ownership matters\nwork()}\n",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := goxformat.File(file, options)
	if err == nil || !strings.Contains(err.Error(), "directive source anchor changed") {
		t.Fatalf("File() error = %v, want directive anchor rejection", err)
	}
	if len(formatted) != 0 {
		t.Fatalf("File() returned partial output %q", formatted)
	}

	fragment, err := source.LoadFragment(
		"directive.go",
		source.FragmentStatement,
		[]byte("if ready { //gox:ignore example because ownership matters\nwork()}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err = goxformat.Fragment(fragment, options)
	if err == nil || !strings.Contains(err.Error(), "directive source anchor changed") {
		t.Fatalf("Fragment() error = %v, want directive anchor rejection", err)
	}
	if len(formatted) != 0 {
		t.Fatalf("Fragment() returned partial output %q", formatted)
	}
}

func TestFormatRejectsSuppressionTargetDriftWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		"suppression.go",
		[]byte(
			"package sample\nfunc run(ready bool) {\n//gox:ignore duplicate-condition -- legacy branch\nif ready { use() } else if ready { retry() }\n}\nfunc use(){}\nfunc retry(){}\n",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err == nil || !strings.Contains(err.Error(), "suppression ownership changed") {
		t.Fatalf("File() error = %v, want suppression ownership rejection", err)
	}
	if len(formatted) != 0 {
		t.Fatalf("File() returned partial output %q", formatted)
	}

	fragment, err := source.LoadFragment(
		"suppression.go",
		source.FragmentStatement,
		[]byte(
			"//gox:ignore duplicate-condition -- legacy branch\nif ready { use() } else if ready { retry() }",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err = goxformat.Fragment(
		fragment,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err == nil || !strings.Contains(err.Error(), "suppression ownership changed") {
		t.Fatalf("Fragment() error = %v, want suppression ownership rejection", err)
	}
	if len(formatted) != 0 {
		t.Fatalf("Fragment() returned partial output %q", formatted)
	}
}

func TestFormatRejectsDiagnosticOnlyFileWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	file, loadErr := source.Load("invalid.go", []byte("package invalid\nfunc broken( {\n"))
	if loadErr == nil || file == nil || file.CanFormat() {
		t.Fatalf("Load() = %#v, %v; want diagnostic-only file", file, loadErr)
	}
	formatted, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err == nil {
		t.Fatal("File() must reject diagnostic-only source")
	}
	if len(formatted) != 0 {
		t.Fatalf("File() returned partial output %q", formatted)
	}
}

func TestFormatRejectsDiagnosticOnlyFragmentWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	fragment, loadErr := source.LoadFragment(
		"invalid_fragment.go",
		source.FragmentStatement,
		[]byte("if ready {"),
	)
	if loadErr == nil || fragment == nil || fragment.CanFormat() {
		t.Fatalf(
			"LoadFragment() = %#v, %v; want diagnostic-only fragment",
			fragment,
			loadErr,
		)
	}
	formatted, err := goxformat.Fragment(
		fragment,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err == nil {
		t.Fatal("Fragment() must reject diagnostic-only source")
	}
	if len(formatted) != 0 {
		t.Fatalf("Fragment() returned partial output %q", formatted)
	}
}

func TestFormatPreservesImportGroupsOrderAliasesAndLiterals(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package imports\nimport(z \"z.example/pkg\";_ `a.example/side`\n\n. \"dot.example/pkg\")\nfunc run(){}\n",
	)
	file, err := source.Load("imports.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package imports\n\nimport (\n\tz \"z.example/pkg\"\n\t_ `a.example/side`\n\n\t. \"dot.example/pkg\"\n)\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want import identity and order preserved", got)
	}
}

func TestFormatPreservesImportBoundaryComments(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/comments/import-boundaries.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/comments/import-boundaries.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("import_boundaries.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesOmittedConstExpressionsAndGroupedDeclarationOrder(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package declarations\nconst(Zero=iota;One;PairA,PairB=1,2;PairC,PairD)\nvar(first,second int;third=3)\ntype(Alias=Existing;Defined Existing;Existing int;Generic[T any] struct{Value T})\n",
	)
	file, err := source.Load("grouped_declarations.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package declarations\n\nconst (\n\tZero = iota\n\tOne\n\tPairA, PairB = 1, 2\n\tPairC, PairD\n)\n\nvar (\n\tfirst, second int\n\tthird = 3\n)\n\ntype (\n\tAlias = Existing\n\tDefined Existing\n\tExisting int\n\tGeneric[T any] struct {\n\t\tValue T\n\t}\n)\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesAcceptedEmptyDeclarationGroups(t *testing.T) {
	t.Parallel()

	input := []byte("package empty\nimport()\nconst()\nvar()\ntype()\n")
	file, err := source.Load("empty_declarations.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package empty\n\nimport ()\n\nconst ()\n\nvar ()\n\ntype ()\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLowersDeclarationsGenericSignaturesAndGoTypes(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package declarations\nconst answer=42\nvar cache map[string][]*Entry\nvar sink chan<- map[string][3]byte\ntype Pair[K comparable,V any] struct{Left K;Right V}\ntype Tagged struct{Value string `json:\"value\"`}\ntype Reader interface{Read([]byte)(int,error);Close()error}\nfunc Convert[K comparable,V any](input func(K)(V,error),values ...K)(map[K]V,error){return nil,nil}\nfunc(p *Pair[K,V])Reset(){}\n",
	)
	file, err := source.Load("declarations.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package declarations\n\nconst answer = 42\n\nvar cache map[string][]*Entry\n\nvar sink chan<- map[string][3]byte\n\ntype Pair[K comparable, V any] struct {\n\tLeft K\n\tRight V\n}\n\ntype Tagged struct {\n\tValue string `json:\"value\"`\n}\n\ntype Reader interface {\n\tRead([]byte) (int, error)\n\tClose() error\n}\n\nfunc Convert[K comparable, V any](input func(K) (V, error), values ...K) (map[K]V, error) {\n\treturn nil, nil\n}\n\nfunc (p *Pair[K, V]) Reset() {}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLowersCompositeAndPostfixExpressions(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package expressions\nfunc use(){values:=[]Item{{Name:\"first\",Score:1},{Name:\"second\",Score:2}};selected:=values[1:len(values):cap(values)];current:=anyValue.(Widget);pair:=NewPair[string,int](\"x\",1);transform:=func(value int)int{return value+1};_,_,_,_=selected,current,pair,transform}\n",
	)
	file, err := source.Load("expressions.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 60, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package expressions\n\nfunc use() {\n\tvalues := []Item{\n\t\t{Name: \"first\", Score: 1},\n\t\t{Name: \"second\", Score: 2},\n\t}\n\tselected := values[1:len(values):cap(values)]\n\tcurrent := anyValue.(Widget)\n\tpair := NewPair[string, int](\"x\", 1)\n\ttransform := func(value int) int {\n\t\treturn value + 1\n\t}\n\t_, _, _, _ = selected, current, pair, transform\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatWrapsGenericFunctionSignatures(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package wrapping\nfunc Transform[InputType comparable,OutputType any](primary InputType,secondary InputType,convert func(InputType)(OutputType,error))(map[InputType]OutputType,error){return nil,nil}\n",
	)
	file, err := source.Load("wrapping.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 52, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package wrapping\n\nfunc Transform[\n\tInputType comparable,\n\tOutputType any,\n](\n\tprimary InputType,\n\tsecondary InputType,\n\tconvert func(InputType) (OutputType, error),\n) (map[InputType]OutputType, error) {\n\treturn nil, nil\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatKeepsSingleTypeParameterFlatWhenParametersBreak(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package wrapping\nfunc runInteractive[T any](ctx context.Context,prompt Prompt[T],execution Execution)(result T,resultErr error){}\n",
	)
	file, err := source.Load("single_type_parameter.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package wrapping\n\nfunc runInteractive[T any](\n\tctx context.Context,\n\tprompt Prompt[T],\n\texecution Execution,\n) (result T, resultErr error) {}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatBreaksSingleTypeParameterWhenItMakesUnderlyingTypeFit(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package wrapping\ntype Container[T any] map[string]map[string]map[string]map[string]map[string]map[string]map[string]map[string]T\n",
	)
	file, err := source.Load("single_type_parameter_type.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package wrapping\n\ntype Container[\n\tT any,\n] map[string]map[string]map[string]map[string]map[string]map[string]map[string]map[string]T\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatKeepsMethodReceiverFlatWhenParametersBreak(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package receiver\ntype Tool struct{}\nfunc (t *Tool) Execute(firstParameter string,secondParameter string){}\n",
	)
	file, err := source.Load("receiver.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 42, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package receiver\n\ntype Tool struct{}\n\nfunc (t *Tool) Execute(\n\tfirstParameter string,\n\tsecondParameter string,\n) {}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatKeepsNonCanonicalReceiverListsBreakable(t *testing.T) {
	t.Parallel()

	input := []byte(
		"package receiver\ntype Tool struct{}\nfunc (left,right *Tool) MultiName(firstParameter string,secondParameter string){}\nfunc (left *Tool,right *Tool) MultiField(firstParameter string,secondParameter string){}\n",
	)
	file, err := source.Load("receiver_lists.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 42, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package receiver\n\ntype Tool struct{}\n\nfunc (\n\tleft, right *Tool,\n) MultiName(\n\tfirstParameter string,\n\tsecondParameter string,\n) {}\n\nfunc (\n\tleft *Tool,\n\tright *Tool,\n) MultiField(\n\tfirstParameter string,\n\tsecondParameter string,\n) {}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesInferredArrayLength(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		"array.go",
		[]byte("package array\nfunc values(){items:=[...]int{1,2,3};_=items}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package array\n\nfunc values() {\n\titems := [...]int{1, 2, 3}\n\t_ = items\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want inferred array length preserved", got)
	}
}

func TestFormatPreservesExplicitSingleResultList(t *testing.T) {
	t.Parallel()

	file, err := source.Load(
		"result.go",
		[]byte("package result\nfunc load()(error){return nil}\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(
		file,
		goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "package result\n\nfunc load() (error) {\n\treturn nil\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want explicit single result list preserved", got)
	}
}
