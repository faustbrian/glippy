package format_test

import (
	"bytes"
	"testing"

	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/source"
)

func TestFormatExpandsMotivatingHostileGo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		width int
		input string
		want  string
	}{
		{
			name:  "compressed if block",
			width: 100,
			input: "package hostile\nfunc check(){if _,err:=client.Discover(nil);!errors.Is(err,ErrContextRequired){t.Fatal(err)}}\n",
			want:  "package hostile\n\nfunc check() {\n\tif _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {\n\t\tt.Fatal(err)\n\t}\n}\n",
		},
		{
			name:  "ordinary statement semicolons",
			width: 100,
			input: "package hostile\nfunc run(){ctx,cancel:=context.WithCancel(t.Context());cancel();result:=work(ctx);_ = result}\n",
			want:  "package hostile\n\nfunc run() {\n\tctx, cancel := context.WithCancel(t.Context())\n\tcancel()\n\tresult := work(ctx)\n\t_ = result\n}\n",
		},
		{
			name:  "boolean chain",
			width: 24,
			input: "package hostile\nfunc condition() bool{return foo&&bar&&baz&&somethingReallyLong}\n",
			want:  "package hostile\n\nfunc condition() bool {\n\treturn foo &&\n\t\tbar &&\n\t\tbaz &&\n\t\tsomethingReallyLong\n}\n",
		},
		{
			name:  "long call",
			width: 30,
			input: "package hostile\nfunc call(){result,err:=client.executeContent(ctx,OperationInfo,http.MethodGet,\"/\",nil,\"application/json\",200);_,_=result,err}\n",
			want:  "package hostile\n\nfunc call() {\n\tresult, err := client.executeContent(\n\t\tctx,\n\t\tOperationInfo,\n\t\thttp.MethodGet,\n\t\t\"/\",\n\t\tnil,\n\t\t\"application/json\",\n\t\t200,\n\t)\n\t_, _ = result, err\n}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			file, err := source.Load(test.name+".go", []byte(test.input))
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

	file, err := source.Load("comment.go", []byte("package comments\n// keep me\nfunc run() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goxformat.File(file, goxformat.Options{Width: 100, TabWidth: 8, FitBudget: 1_000})
	if err == nil || len(got) != 0 {
		t.Fatalf("File() = (%q, %v), want refusal without partial output", got, err)
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
