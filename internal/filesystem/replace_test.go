package filesystem_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/faustbrian/gox/internal/filesystem"
	"github.com/faustbrian/gox/internal/source"
)

func TestReadRejectsOversizedSourceBeforeSnapshotAllocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.go")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(source.MaxFileSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := filesystem.Read(path)
	if snapshot != nil || !errors.Is(err, source.ErrTooLarge) {
		t.Fatalf(
			"Read() returned snapshot=%t, error=%v, want ErrTooLarge",
			snapshot != nil,
			err,
		)
	}
}

func TestSnapshotReplaceAtomicallyPreservesPermissions(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := snapshot.Replace([]byte("after\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("Replace() content = %q, want after", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("Replace() permissions = %o, want 640", info.Mode().Perm())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "source.go" {
		t.Fatalf("Replace() left temporary files: %#v", entries)
	}
}

func TestSnapshotReplaceUnchangedBytesDoesNotTouchFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	input := []byte("package sample\n")
	if err := os.WriteFile(path, input, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := snapshot.Replace(input); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("Replace() replaced an unchanged file")
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf(
			"Replace() modification time = %v, want %v",
			after.ModTime(),
			before.ModTime(),
		)
	}
}

func TestCreateWithinCreatesRegularFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".gox-baseline.json")
	if err := filesystem.CreateWithin(root, path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("created baseline mode = %v", info.Mode())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first\n" {
		t.Fatalf("CreateWithin() content = %q", got)
	}
}

func TestCreateWithinRejectsSymlinkTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte("preserve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".gox-baseline.json")
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.CreateWithin(root, path, []byte("replace\n"), 0o600); err == nil {
		t.Fatal("CreateWithin() error = nil, want symlink refusal")
	}
	got, err := os.ReadFile(external)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "preserve\n" {
		t.Fatalf("external content = %q", got)
	}
}

func TestCreateWithinDoesNotReplaceExistingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".gox-baseline.json")
	if err := os.WriteFile(path, []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.CreateWithin(root, path, []byte("new\n"), 0o600);
		!errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("CreateWithin() error = %v, want ErrStale", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing\n" {
		t.Fatalf("CreateWithin() replaced existing file: %q", got)
	}
}

func TestSnapshotReplaceRejectsChangedSourceBytes(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte("newer source\n")
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("formatted old source\n"))
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("Replace() error = %v, want ErrStale", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(changed) {
		t.Fatalf("Replace() content = %q, want newer source preserved", got)
	}
}

func TestSnapshotReplaceTreatsOversizedSourceGrowthAsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(source.MaxFileSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("package changed\n"))
	if !errors.Is(err, filesystem.ErrStale) || errors.Is(err, source.ErrTooLarge) {
		t.Fatalf("Replace() error = %v, want stale-source conflict", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Size() != source.MaxFileSize + 1 {
		t.Fatalf("Replace() changed grown source size to %d", info.Size())
	}
}

func TestSnapshotReplaceRejectsChangedSourcePermissions(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	input := []byte("before\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("formatted old source\n"))
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("Replace() error = %v, want ErrStale", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Fatalf("Replace() content = %q, want source preserved", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf(
			"Replace() permissions = %o, want changed permissions preserved",
			info.Mode().Perm(),
		)
	}
}

func TestSnapshotReplaceRejectsReplacedSourcePath(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "replacement.go")
	newer := []byte("new inode\n")
	if err := os.WriteFile(replacement, newer, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("formatted old source\n"))
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("Replace() error = %v, want ErrStale", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newer) {
		t.Fatalf("Replace() content = %q, want replacement preserved", got)
	}
}

func TestSnapshotReplaceReportsRemovedSourceAsStale(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("formatted old source\n"))
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("Replace() error = %v, want ErrStale", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Replace() recreated removed source: %v", err)
	}
}

func TestReadRejectsSymlink(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	target := filepath.Join(directory, "target.go")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "source.go")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if _, err := filesystem.Read(path); err == nil {
		t.Fatal("Read() error = nil, want symlink refusal")
	}
}

func TestReadWithinRejectsSourceOutsideAuthorizedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := filesystem.ReadWithin(root, path); err == nil {
		t.Fatal("ReadWithin() error = nil, want outside-root refusal")
	}
}

func TestReadWithinRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(external, "source.go"),
		[]byte("package sample\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}

	if _, err := filesystem.ReadWithin(root, filepath.Join(link, "source.go")); err == nil {
		t.Fatal("ReadWithin() error = nil, want symlink-escape refusal")
	}
}

func TestSnapshotReplaceRejectsChangedAuthorizedRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.go")
	input := []byte("before\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.ReadWithin(root, path)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(parent, "external")
	if err := os.Mkdir(external, 0o755); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(external, "source.go")
	if err := os.Link(path, externalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, root); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("after\n"))
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("Replace() error = %v, want ErrStale", err)
	}
	got, err := os.ReadFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("Replace() changed file through replaced root: %q", got)
	}
}

func TestSnapshotReplaceRejectsSourceChangedToSymlink(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "source.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target.go")
	targetBytes := []byte("target remains unchanged\n")
	if err := os.WriteFile(target, targetBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	err = snapshot.Replace([]byte("formatted old source\n"))
	if !errors.Is(err, filesystem.ErrStale) {
		t.Fatalf("Replace() error = %v, want ErrStale", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(targetBytes) {
		t.Fatalf("Replace() target content = %q, want target preserved", got)
	}
}
