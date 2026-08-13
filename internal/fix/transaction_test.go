package fix_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/faustbrian/gox/internal/filesystem"
	fixengine "github.com/faustbrian/gox/internal/fix"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

func TestCoordinateAndReplaceWritesOneValidatedAtomicResult(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	path := writeSource(t, input)
	snapshot, file := readSnapshotSource(t, path)
	transaction, err := fixengine.CoordinateAndReplace(
		snapshot,
		[]fixengine.Selection{
			selection(
				file,
				"rename",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "primary"),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Status != fixengine.WriteCompleted || len(transaction.Result.Applied) != 1 {
		t.Fatalf("CoordinateAndReplace() = %#v", transaction)
	}
	want := "package sample\n\nfunc run() {\n\tprimary()\n}\n"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("written source = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("written permissions = %o, want 640", info.Mode().Perm())
	}
}

func TestCoordinateAndReplaceDoesNotWriteRejectedFixes(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	path := writeSource(t, input)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, file := readSnapshotSource(t, path)
	transaction, err := fixengine.CoordinateAndReplace(
		snapshot,
		[]fixengine.Selection{
			selection(
				file,
				"first",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "primary"),
			),
			selection(
				file,
				"second",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "secondary"),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Status != fixengine.WriteNotPerformed ||
		len(transaction.Result.Rejected) != 2 {
		t.Fatalf("CoordinateAndReplace() = %#v", transaction)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("rejected fixes changed the source file")
	}
}

func TestCoordinateAndReplaceRejectsAnOnDiskSourceChange(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	path := writeSource(t, input)
	snapshot, file := readSnapshotSource(t, path)
	newer := []byte("package sample\nfunc newer(){}\n")
	if err := os.WriteFile(path, newer, 0o640); err != nil {
		t.Fatal(err)
	}
	transaction, err := fixengine.CoordinateAndReplace(
		snapshot,
		[]fixengine.Selection{
			selection(
				file,
				"rename",
				"rewrite",
				rules.FixSafe,
				edit(input, "target", "primary"),
			),
		},
		fixOptions(),
	)
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("CoordinateAndReplace() error = %v, want ErrStale", err)
	}
	if transaction.Status != fixengine.WriteNotPerformed {
		t.Fatalf("CoordinateAndReplace() status = %q", transaction.Status)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, newer) {
		t.Fatalf("stale transaction wrote %q, want newer source", got)
	}
}

func TestCoordinateAndReplacePreservesDiskWhenValidationRejectsFix(t *testing.T) {
	t.Parallel()

	input := "package sample\nfunc run(){target()}\n"
	path := writeSource(t, input)
	snapshot, file := readSnapshotSource(t, path)
	transaction, err := fixengine.CoordinateAndReplace(
		snapshot,
		[]fixengine.Selection{
			selection(
				file,
				"invalid",
				"rewrite",
				rules.FixSafe,
				edit(input, "target()", "("),
			),
		},
		fixOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Status != fixengine.WriteNotPerformed ||
		len(transaction.Result.Rejected) != 1 ||
		transaction.Result.Rejected[0].Reason != fixengine.RejectionValidation {
		t.Fatalf("CoordinateAndReplace() = %#v", transaction)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != input {
		t.Fatalf("validation rejection wrote %q", got)
	}
}

func writeSource(t *testing.T, input string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(path, []byte(input), 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSnapshotSource(t *testing.T, path string) (*filesystem.Snapshot, *source.File) {
	t.Helper()
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load(snapshot.Path(), snapshot.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, file
}
