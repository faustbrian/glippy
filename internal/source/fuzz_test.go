package source_test

import (
	"bytes"
	"testing"

	"github.com/faustbrian/gox/internal/source"
)

func FuzzSourceLedgerReconstruction(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("package valid\nfunc run(){first();second()}\n"),
		[]byte("\xef\xbb\xbf//go:build linux\r\n\r\npackage directives\r\n\r\n//go:generate echo generated\r\n"),
		[]byte("package cgo\n/*\n#cgo CFLAGS: -DVALUE=1\n*/\nimport \"C\"\n"),
		[]byte("package invalid\nfunc broken( {\n"),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 256<<10 {
			t.Skip()
		}
		file, _ := source.Load("fuzz.go", input)
		if file == nil {
			t.Fatal("Load() returned no diagnostic source unit")
		}
		if got := reconstruct(file.Pieces()); !bytes.Equal(got, input) {
			t.Fatalf("physical ledger did not reconstruct the input")
		}
		for index, comment := range file.Comments() {
			if int(comment.ID) != index {
				t.Fatalf("comment ID = %d, want %d", comment.ID, index)
			}
			raw, ok := file.Slice(comment.Range)
			if !ok || raw != comment.Raw {
				t.Fatalf("comment %d does not match its physical range", comment.ID)
			}
		}
		for index, directive := range file.Directives() {
			raw, ok := file.Slice(directive.Range)
			if !ok || raw != directive.Raw {
				t.Fatalf("directive %d does not match its physical range", index)
			}
		}
	})
}

func FuzzSourceFragmentLedgerReconstruction(f *testing.F) {
	for _, seed := range []struct {
		kind  uint8
		input []byte
	}{
		{kind: 0, input: []byte("var answer=42\nfunc run(){}")},
		{kind: 1, input: []byte("value:=1;value++")},
		{kind: 2, input: []byte("foo && bar && somethingLong\n")},
		{kind: 1, input: []byte("}\nvar escaped=1\nfunc reopened(){")},
	} {
		f.Add(seed.kind, seed.input)
	}

	f.Fuzz(func(t *testing.T, rawKind uint8, input []byte) {
		if len(input) > 256<<10 {
			t.Skip()
		}
		kind := source.FragmentKind(1 + rawKind%3)
		fragment, _ := source.LoadFragment("fuzz.go", kind, input)
		if fragment == nil {
			t.Fatal("LoadFragment() returned no diagnostic source unit")
		}
		if got := reconstruct(fragment.Pieces()); !bytes.Equal(got, input) {
			t.Fatal("fragment physical ledger did not reconstruct the input")
		}
		for index, comment := range fragment.Comments() {
			if int(comment.ID) != index {
				t.Fatalf("comment ID = %d, want %d", comment.ID, index)
			}
			raw, ok := fragment.Slice(comment.Range)
			if !ok || raw != comment.Raw {
				t.Fatalf("comment %d does not match its physical range", comment.ID)
			}
		}
	})
}
