package format_test

import (
	"bytes"
	"testing"

	goxformat "github.com/faustbrian/gox/internal/format"
	"github.com/faustbrian/gox/internal/source"
)

func FuzzFormatValidSource(f *testing.F) {
	for _, seed := range
		[]string{
			"package hostile\nfunc check(){if _,err:=client.Discover(nil);!errors.Is(err,ErrContextRequired){t.Fatal(err)}}\n",
			"package comments\nfunc use(){Generic[/* type */ string](/* value */ input)}\n",
			"package comments\nfunc allowed(first,second bool)bool{return first || // keep\nsecond}\n",
			"//go:build linux\n\npackage directives\n\n//go:generate go run example.invalid/generator\nfunc run(){}\n",
			"\xef\xbb\xbf//line generated.go:100\r\npackage physical\r\nfunc run(){}",
		} {
		f.Add([]byte(seed), uint8(100))
	}

	f.Fuzz(
		func(t *testing.T, input []byte, rawWidth uint8) {
			if len(input) > 256 << 10 {
				t.Skip()
			}
			file, err := source.Load("fuzz.go", input)
			if err != nil {
				return
			}
			options := goxformat.Options{
				Width: 20 + int(rawWidth % 101),
				TabWidth: 8,
				FitBudget: 1_000,
			}
			formatted, err := goxformat.File(file, options)
			if err != nil {
				if len(formatted) != 0 {
					t.Fatalf(
						"File() returned partial output with an error: %v",
						err,
					)
				}
				return
			}
			reparsed, err := source.Load("formatted.go", formatted)
			if err != nil {
				t.Fatalf("formatted output does not parse: %v", err)
			}
			again, err := goxformat.File(reparsed, options)
			if err != nil {
				t.Fatalf("repeat formatting failed: %v", err)
			}
			if !bytes.Equal(formatted, again) {
				t.Fatalf("formatting is not byte-idempotent")
			}
		},
	)
}

func FuzzFormatValidFragment(f *testing.F) {
	for _, seed := range
		[]struct {
			kind uint8
			input []byte
			width uint8
		}{
			{kind: 0, input: []byte("var answer=42\nfunc run(){}"), width: 100},
			{kind: 1, input: []byte("value:=1;value++"), width: 100},
			{kind: 2, input: []byte("foo && bar && somethingLong\n"), width: 20},
			{kind: 2, input: []byte("/* leading */ foo+/* middle */bar"), width: 100},
		} {
		f.Add(seed.kind, seed.input, seed.width)
	}

	f.Fuzz(
		func(t *testing.T, rawKind uint8, input []byte, rawWidth uint8) {
			if len(input) > 256 << 10 {
				t.Skip()
			}
			kind := source.FragmentKind(1 + rawKind % 3)
			fragment, err := source.LoadFragment("fuzz.go", kind, input)
			if err != nil {
				return
			}
			options := goxformat.Options{
				Width: 20 + int(rawWidth % 101),
				TabWidth: 8,
				FitBudget: 1_000,
			}
			formatted, err := goxformat.Fragment(fragment, options)
			if err != nil {
				if len(formatted) != 0 {
					t.Fatalf(
						"Fragment() returned partial output with an error: %v",
						err,
					)
				}
				return
			}
			reparsed, err := source.LoadFragment("formatted.go", kind, formatted)
			if err != nil {
				t.Fatalf("formatted fragment does not parse: %v", err)
			}
			again, err := goxformat.Fragment(reparsed, options)
			if err != nil {
				t.Fatalf("repeat fragment formatting failed: %v", err)
			}
			if !bytes.Equal(formatted, again) {
				t.Fatal("fragment formatting is not byte-idempotent")
			}
		},
	)
}
