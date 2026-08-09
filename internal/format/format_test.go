package format_test

import (
	"bytes"
	"os"
	"testing"

	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/source"
)

func TestFormatExpandsMotivatingHostileGo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		width   int
		fixture string
	}{
		{
			name:    "compressed if block",
			width:   100,
			fixture: "compressed-if",
		},
		{
			name:    "ordinary statement semicolons",
			width:   100,
			fixture: "statement-semicolons",
		},
		{
			name:    "boolean chain",
			width:   24,
			fixture: "boolean-chain",
		},
		{
			name:    "long call",
			width:   30,
			fixture: "long-call",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input, err := os.ReadFile("../../testdata/format/motivating/" + test.fixture + ".input")
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile("../../testdata/format/motivating/" + test.fixture + ".golden")
			if err != nil {
				t.Fatal(err)
			}
			file, err := source.Load(test.fixture+".go", input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := goxformat.File(file, goxformat.Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
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
			again, err := goxformat.File(reparsed, goxformat.Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again, got) {
				t.Fatalf("formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s", got, again)
			}
		})
	}
}

func TestFormatRefusesCommentsUntilOwnershipCanBePreserved(t *testing.T) {
	t.Parallel()

	file, err := source.Load("comment.go", []byte("package comments\nfunc run(value /* keep me */ int){_=value}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err == nil || len(got) != 0 {
		t.Fatalf("File() = (%q, %v), want refusal without partial output", got, err)
	}
}

func TestFormatPreservesFieldListBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte("package comments\nfunc run(/* first parameter */ value int,other string /* trailing parameter */)(/* first result */ string,error /* trailing result */){return \"\",nil}\nfunc Generic[/* first type parameter */ T any,U comparable /* trailing type parameter */](value T){}\nfunc empty(/* empty parameter list */){}\n")
	file, err := source.Load("field_lists.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package comments\nfunc run(value int, // first parameter\nother string, // second parameter\n){}\n")
	file, err := source.Load("field_line_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package comments\nfunc use(){Generic[/* first type argument */ string,int /* trailing type argument */](/* first argument */ first,second /* trailing argument */);Single[/* single type argument */ string](value);empty(/* empty argument list */);values:=[]int{/* empty composite literal */};_=values}\n")
	file, err := source.Load("expression_lists.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesFilePrefixCommentsAndDirectives(t *testing.T) {
	t.Parallel()

	input := []byte("//go:build linux\n\n// Package prefix documents the package.\npackage prefix\nfunc run(){}\n")
	file, err := source.Load("prefix.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "//go:build linux\n\n// Package prefix documents the package.\npackage prefix\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want exact anchored prefix comments", got)
	}
}

func TestFormatPreservesDeclarationDocumentationAndTrailingComments(t *testing.T) {
	t.Parallel()

	input := []byte("package comments\n//go:generate go run example.invalid/generator\n// run performs work.\nfunc run(){} // keep trailing\n")
	file, err := source.Load("comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\n//go:generate go run example.invalid/generator\n// run performs work.\nfunc run() {} // keep trailing\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want declaration comments and directive anchored", got)
	}
}

func TestFormatPreservesStandaloneTopLevelDirectives(t *testing.T) {
	t.Parallel()

	input := []byte("package comments\n\n//go:generate go run example.invalid/generator\n\nfunc run(){}\n")
	file, err := source.Load("directive.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package comments\ntype Config struct{// Value documents the field.\nValue string `json:\"value\"` // keep field\n}\n")
	file, err := source.Load("fields.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package comments\ntype Config struct{Value string\n// after last field\n}\ntype Empty struct{/* inside empty aggregate */}\n")
	file, err := source.Load("aggregate_closers.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package comments\nfunc run(){// before first\nfirst() // after first\n// between statements\nsecond()\n// before close\n}\n")
	file, err := source.Load("statements.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run() {\n\t// before first\n\tfirst() // after first\n\t// between statements\n\tsecond()\n\t// before close\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want statement boundary comments anchored", got)
	}
}

func TestFormatPreservesBlankLinesAfterBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte("package comments\ntype Config struct{// standalone field\n\nValue string\n}\nfunc run(){// standalone statement\n\nwork()}\n")
	file, err := source.Load("blank_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package comments\nfunc add(a,b int)int{return a /* left operand */ + /* right operand */ b}\nfunc values()[]int{return []int{1 /* first */,2 /* second */,3}}\n")
	file, err := source.Load("expressions.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc add(a, b int) int {\n\treturn a /* left operand */ + /* right operand */ b\n}\n\nfunc values() []int {\n\treturn []int{\n\t\t1, /* first */\n\t\t2, /* second */\n\t\t3,\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesCallArgumentComments(t *testing.T) {
	t.Parallel()

	file, err := source.Load("arguments.go", []byte("package comments\nfunc run(){use(first /* first */,second /* second */)}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package comments\n\nfunc run() {\n\tuse(\n\t\tfirst, /* first */\n\t\tsecond, /* second */\n\t)\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want call argument comments anchored", got)
	}
}

func TestFormatHostileCommentCorpus(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/corpus/hostile/comments.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "// Package hostile contains owned hostile-valid-Go fixtures.\npackage hostile\n\n//go:generate go run example.invalid/generator\n\nfunc comments(a, b int) int {\n\treturn a /* left operand */ + /* right operand */ b\n}\n\nfunc literal() []int {\n\treturn []int{\n\t\t1, /* first */\n\t\t2, /* second */\n\t\t3,\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatHostileGenericRangeCorpus(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/corpus/hostile/generics.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("generics.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package hostile\n\ntype Number interface {\n\t~int | ~int64 | ~float64\n}\n\nfunc sum[T Number](values []T) T {\n\tvar result T\n\tfor _, value := range values {\n\t\tresult += value\n\t}\n\treturn result\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLowersControlFlowAndStatementSurface(t *testing.T) {
	t.Parallel()

	input := []byte("package control\nfunc run(values []int,ch chan int){var total int;for i:=0;i<len(values);i++{total+=values[i];if total>10{break}else{continue}};for total<20{total++};for{break};for index,value:=range values{_=index;total+=value};go work();defer close(ch);ch<-total;Block:{goto Block}}\n")
	file, err := source.Load("control.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package control\nfunc run(ready bool){for ;ready;{work()};for ;;{break}}\n")
	file, err := source.Load("classic_for.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc run(ready bool) {\n\tfor ; ready; {\n\t\twork()\n\t}\n\tfor ; ; {\n\t\tbreak\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatDoesNotMistakeNestedSemicolonsForClassicForClause(t *testing.T) {
	t.Parallel()

	input := []byte("package control\nfunc run(ready bool){for func()bool{work();return ready}(){break}}\n")
	file, err := source.Load("nested_for.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc run(ready bool) {\n\tfor\n\t\tfunc() bool {\n\t\t\twork()\n\t\t\treturn ready\n\t\t}() {\n\t\tbreak\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLowersSwitchTypeSwitchAndSelectClauses(t *testing.T) {
	t.Parallel()

	input := []byte("package control\nfunc classify(value any,ready <-chan int,done <-chan struct{})string{switch size:=len([]int{1});size{case 0:return \"empty\";case 1,2:// small\nreturn \"small\";default:return \"many\"};switch current:=value.(type){case string:return current;case nil:return \"nil\";default:return \"other\"};select{case item:=<-ready:_=item;case <-done:return \"done\";case ready<-1:return \"sent\";default:return \"waiting\"}}\n")
	file, err := source.Load("clauses.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package control\nfunc loop(){for index:=startingIndex;index<collectionLength;index++{work()}}\nfunc iterate(){for index,value:=range valuesWithAnExtremelyLongName{_,_=index,value}}\nfunc choose(inputValue int){switch currentValue:=inputValue;currentValue{case FirstVeryLongValue,SecondVeryLongValue,ThirdVeryLongValue:work()}}\nfunc classify(inputValue any){switch prepared:=inputValue;current:=prepared.(type){case string:_=current}}\nfunc communicate(){select{case receivedValue:=<-incomingValuesChannel:_=receivedValue;case outgoingValuesChannelWithLongName<-value:return}}\n")
	file, err := source.Load("broken_control.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 44, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package control\n\nfunc loop() {\n\tfor index := startingIndex;\n\t\tindex < collectionLength;\n\t\tindex++ {\n\t\twork()\n\t}\n}\n\nfunc iterate() {\n\tfor\n\t\tindex, value := range\n\t\tvaluesWithAnExtremelyLongName {\n\t\t_, _ = index, value\n\t}\n}\n\nfunc choose(inputValue int) {\n\tswitch currentValue := inputValue;\n\t\tcurrentValue {\n\tcase FirstVeryLongValue,\n\t\tSecondVeryLongValue,\n\t\tThirdVeryLongValue:\n\t\twork()\n\t}\n}\n\nfunc classify(inputValue any) {\n\tswitch prepared := inputValue;\n\t\tcurrent := prepared.(type) {\n\tcase string:\n\t\t_ = current\n\t}\n}\n\nfunc communicate() {\n\tselect {\n\tcase\n\t\treceivedValue :=\n\t\t\t<-incomingValuesChannel:\n\t\t_ = receivedValue\n\tcase\n\t\toutgoingValuesChannelWithLongName <-\n\t\t\tvalue:\n\t\treturn\n\t}\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatUsesDeterministicCaseListWidthBoundary(t *testing.T) {
	t.Parallel()

	input := []byte("package control\nfunc choose(value int){switch value{case FirstVeryLongValue,SecondVeryLongValue,ThirdVeryLongValue:return}}\n")
	flat := "package control\n\nfunc choose(value int) {\n\tswitch value {\n\tcase FirstVeryLongValue, SecondVeryLongValue, ThirdVeryLongValue:\n\t\treturn\n\t}\n}\n"
	broken := "package control\n\nfunc choose(value int) {\n\tswitch value {\n\tcase FirstVeryLongValue,\n\t\tSecondVeryLongValue,\n\t\tThirdVeryLongValue:\n\t\treturn\n\t}\n}\n"
	for _, test := range []struct {
		name  string
		width int
		want  string
	}{
		{name: "exact fit", width: 73, want: flat},
		{name: "one column under", width: 72, want: broken},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			file, err := source.Load("case_boundary.go", input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := goxformat.File(file, goxformat.Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}

func TestFormatUsesDeterministicIfInitializerWidthBoundary(t *testing.T) {
	t.Parallel()

	input := []byte("package control\nfunc choose(){if current:=initialValue;current!=expectedVal{work()}}\n")
	flat := "package control\n\nfunc choose() {\n\tif current := initialValue; current != expectedVal {\n\t\twork()\n\t}\n}\n"
	broken := "package control\n\nfunc choose() {\n\tif current := initialValue;\n\t\tcurrent != expectedVal {\n\t\twork()\n\t}\n}\n"
	for _, test := range []struct {
		name  string
		width int
		want  string
	}{
		{name: "exact fit", width: 60, want: flat},
		{name: "one column under", width: 59, want: broken},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			file, err := source.Load("if_boundary.go", input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := goxformat.File(file, goxformat.Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
			}
		})
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
	for _, test := range []struct {
		name  string
		width int
		want  []byte
	}{
		{name: "exact fit", width: 49, want: flat},
		{name: "one column under", width: 48, want: broken},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			file, err := source.Load("selector_call.go", input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := goxformat.File(file, goxformat.Options{Width: test.width, TabWidth: 8, FitBudget: 1_000})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, test.want) {
				t.Fatalf("File() =\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}

func TestFormatBreaksGenericSelectorChains(t *testing.T) {
	t.Parallel()

	input, err := os.ReadFile("../../testdata/format/chains/selector-generic.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/format/chains/selector-generic.golden")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("selector_generic.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 54, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesClauseBoundaryComments(t *testing.T) {
	t.Parallel()

	input := []byte("package control\nfunc run(value int,ready <-chan int){switch value{case 1:first() // trailing\n// between cases\ncase 2:second()\n// before switch close\n};select{case <-ready:use() // trailing\n// between communications\ndefault:wait()\n// before select close\n}}\n")
	file, err := source.Load("clause_comments.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	file, err := source.Load("variadic.go", []byte("package variadic\nfunc run(){use(values...)}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package variadic\n\nfunc run() {\n\tuse(values...)\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want call ellipsis preserved", got)
	}
}

func TestFormatRejectsUnmodeledSyntaxDifferences(t *testing.T) {
	t.Parallel()

	file, err := source.Load("empty.go", []byte("package empty\nfunc run(){value:=1;;value++;_=value}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err == nil || len(got) != 0 {
		t.Fatalf("File() = (%q, %v), want syntax-equivalence refusal", got, err)
	}
}

func TestFormatPreservesImportGroupsOrderAliasesAndLiterals(t *testing.T) {
	t.Parallel()

	input := []byte("package imports\nimport(z \"z.example/pkg\";_ `a.example/side`\n\n. \"dot.example/pkg\")\nfunc run(){}\n")
	file, err := source.Load("imports.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package imports\n\nimport (\n\tz \"z.example/pkg\"\n\t_ `a.example/side`\n\n\t. \"dot.example/pkg\"\n)\n\nfunc run() {}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want import identity and order preserved", got)
	}
}

func TestFormatLowersDeclarationsGenericSignaturesAndGoTypes(t *testing.T) {
	t.Parallel()

	input := []byte("package declarations\nconst answer=42\nvar cache map[string][]*Entry\nvar sink chan<- map[string][3]byte\ntype Pair[K comparable,V any] struct{Left K;Right V}\ntype Tagged struct{Value string `json:\"value\"`}\ntype Reader interface{Read([]byte)(int,error);Close()error}\nfunc Convert[K comparable,V any](input func(K)(V,error),values ...K)(map[K]V,error){return nil,nil}\nfunc(p *Pair[K,V])Reset(){}\n")
	file, err := source.Load("declarations.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package expressions\nfunc use(){values:=[]Item{{Name:\"first\",Score:1},{Name:\"second\",Score:2}};selected:=values[1:len(values):cap(values)];current:=anyValue.(Widget);pair:=NewPair[string,int](\"x\",1);transform:=func(value int)int{return value+1};_,_,_,_=selected,current,pair,transform}\n")
	file, err := source.Load("expressions.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 60, TabWidth: 8, FitBudget: 1_000})
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

	input := []byte("package wrapping\nfunc Transform[InputType comparable,OutputType any](primary InputType,secondary InputType,convert func(InputType)(OutputType,error))(map[InputType]OutputType,error){return nil,nil}\n")
	file, err := source.Load("wrapping.go", input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 52, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package wrapping\n\nfunc Transform[\n\tInputType comparable,\n\tOutputType any,\n](\n\tprimary InputType,\n\tsecondary InputType,\n\tconvert func(InputType) (OutputType, error),\n) (map[InputType]OutputType, error) {\n\treturn nil, nil\n}\n"
	if string(got) != want {
		t.Fatalf("File() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPreservesInferredArrayLength(t *testing.T) {
	t.Parallel()

	file, err := source.Load("array.go", []byte("package array\nfunc values(){items:=[...]int{1,2,3};_=items}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
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

	file, err := source.Load("result.go", []byte("package result\nfunc load()(error){return nil}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err != nil {
		t.Fatal(err)
	}
	want := "package result\n\nfunc load() (error) {\n\treturn nil\n}\n"
	if string(got) != want {
		t.Fatalf("File() = %q, want explicit single result list preserved", got)
	}
}
