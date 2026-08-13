package diff_test

import (
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/diff"
)

func TestUnifiedSeparatesDistantChangesWithDeterministicContext(t *testing.T) {
	t.Parallel()

	before := []byte("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\n")
	after := []byte("one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\nnine\nTEN\neleven\n")

	got := diff.Unified("before.go", "after.go", before, after)

	want := "--- before.go\n" +
		"+++ after.go\n" +
		"@@ -1,5 +1,5 @@\n" +
		" one\n" +
		"-two\n" +
		"+TWO\n" +
		" three\n" +
		" four\n" +
		" five\n" +
		"@@ -7,5 +7,5 @@\n" +
		" seven\n" +
		" eight\n" +
		" nine\n" +
		"-ten\n" +
		"+TEN\n" +
		" eleven\n"
	if got != want {
		t.Fatalf("Unified() = %q, want %q", got, want)
	}
}

func TestUnifiedMarksMissingFinalNewlinesAndQuotesUnsafeLabels(t *testing.T) {
	t.Parallel()

	got := diff.Unified("before\nname", "after\tname", []byte("old"), []byte("new"))

	want := "--- \"before\\nname\"\n" +
		"+++ \"after\\tname\"\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"\\ No newline at end of file\n" +
		"+new\n" +
		"\\ No newline at end of file\n"
	if got != want {
		t.Fatalf("Unified() = %q, want %q", got, want)
	}
}

func TestUnifiedBoundsRepeatedLineSearch(t *testing.T) {
	t.Parallel()

	before := []byte(strings.Repeat("before\n", 1_100))
	after := []byte(strings.Repeat("after\n", 1_100))

	first := diff.Unified("before.go", "after.go", before, after)
	second := diff.Unified("before.go", "after.go", before, after)

	if first != second {
		t.Fatal("Unified() output is nondeterministic")
	}
	if !strings.HasPrefix(first, "--- before.go\n+++ after.go\n@@ -1,1100 +1,1100 @@\n") {
		t.Fatalf("Unified() header = %q", first[:min(len(first), 100)])
	}
	if strings.Count(first, "-before\n") != 1_100 || strings.Count(first, "+after\n") != 1_100 {
		t.Fatal("Unified() fallback did not account for every line")
	}
}
