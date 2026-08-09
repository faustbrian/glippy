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
