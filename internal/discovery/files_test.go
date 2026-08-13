package discovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/faustbrian/glippy/internal/discovery"
)

func TestGoFilesRecursesDeterministicallyWithinProjectPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, relativePath := range
		[]string{
			"z.go",
			"a.go",
			"nested/b.go",
			"README.md",
			"vendor/dependency.go",
			"testdata/fixture.go",
			"fixtures/example.go",
			".git/hooks/hook.go",
		} {
		writeDiscoveryFile(t, filepath.Join(root, relativePath))
	}
	external := filepath.Join(t.TempDir(), "external.go")
	writeDiscoveryFile(t, external)
	if err := os.Symlink(external, filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}

	files, err := discovery.GoFiles(
		context.Background(),
		[]string{root},
		discovery.Options{Root: root},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []discovery.File{
		{Path: filepath.Join(root, "a.go")},
		{Path: filepath.Join(root, "nested", "b.go")},
		{Path: filepath.Join(root, "z.go")},
	}
	if len(files) != len(want) {
		t.Fatalf("GoFiles() = %#v, want %#v", files, want)
	}
	for index := range want {
		if files[index] != want[index] {
			t.Fatalf("GoFiles()[%d] = %#v, want %#v", index, files[index], want[index])
		}
	}
}

func TestGoFilesRejectsExplicitTraversalOfExcludedTrees(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	vendor := filepath.Join(root, "vendor")
	vendorFile := filepath.Join(vendor, "dependency.go")
	writeDiscoveryFile(t, vendorFile)

	for _, input := range []string{vendor, vendorFile} {
		if _, err := discovery.GoFiles(
			context.Background(),
			[]string{input},
			discovery.Options{Root: root},
		);
			err == nil {
			t.Fatalf("GoFiles(%q) error = nil, want excluded-tree failure", input)
		}
	}
}

func TestGoFilesIncludesFixtureTreesOnlyWhenExplicitlySelected(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	testdata := filepath.Join(root, "testdata")
	selectedDirectory := filepath.Join(testdata, "cases")
	fixturePath := filepath.Join(selectedDirectory, "fixtures", "nested", "fixture.go")
	writeDiscoveryFile(t, fixturePath)

	files, err := discovery.GoFiles(
		context.Background(),
		[]string{selectedDirectory},
		discovery.Options{Root: root},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != (discovery.File{Path: fixturePath}) {
		t.Fatalf("GoFiles() = %#v, want explicitly selected fixture", files)
	}
}

func TestGoFilesPreservesExplicitFileAndSymlinkIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectFile := filepath.Join(root, "project.go")
	writeDiscoveryFile(t, projectFile)
	externalRoot := t.TempDir()
	externalFile := filepath.Join(externalRoot, "external.go")
	writeDiscoveryFile(t, externalFile)
	linkedFile := filepath.Join(root, "linked.go")
	if err := os.Symlink(externalFile, linkedFile); err != nil {
		t.Fatal(err)
	}
	linkedDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(externalRoot, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	linkedDirectoryFile := filepath.Join(linkedDirectory, "external.go")

	want := map[string]discovery.File{
		externalFile: {Path: externalFile, Explicit: true},
		linkedDirectoryFile: {
			Path: linkedDirectoryFile,
			Explicit: true,
			TraversesSymlink: true,
		},
		linkedFile: {Path: linkedFile, Explicit: true, TraversesSymlink: true},
		projectFile: {Path: projectFile, Explicit: true},
	}
	for _, inputs := range
		[][]string{
			{root, projectFile, externalFile, linkedFile, linkedDirectoryFile},
			{projectFile, externalFile, linkedFile, linkedDirectoryFile, root},
		} {
		files, err := discovery.GoFiles(
			context.Background(),
			inputs,
			discovery.Options{Root: root},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != len(want) {
			t.Fatalf("GoFiles() = %#v, want %#v", files, want)
		}
		for index, file := range files {
			if index > 0 && files[index - 1].Path >= file.Path {
				t.Fatalf("GoFiles() paths are not strictly sorted: %#v", files)
			}
			if file != want[file.Path] {
				t.Fatalf(
					"GoFiles()[%d] = %#v, want %#v",
					index,
					file,
					want[file.Path],
				)
			}
		}
	}
}

func TestGoFilesStopsWithoutResultsWhenCanceled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(root, "source.go"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	files, err := discovery.GoFiles(ctx, []string{root}, discovery.Options{Root: root})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GoFiles() error = %v, want context cancellation", err)
	}
	if files != nil {
		t.Fatalf("GoFiles() files = %#v, want no partial results", files)
	}
}

func TestGoFilesRejectsRecursiveInputThroughSymlinkAncestor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	externalDirectory := filepath.Join(external, "nested")
	writeDiscoveryFile(t, filepath.Join(externalDirectory, "outside.go"))
	linkedDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(external, linkedDirectory); err != nil {
		t.Fatal(err)
	}

	files, err := discovery.GoFiles(
		context.Background(),
		[]string{filepath.Join(linkedDirectory, "nested")},
		discovery.Options{Root: root},
	)
	if err == nil {
		t.Fatalf("GoFiles() = %#v, want symlink-boundary failure", files)
	}
	if files != nil {
		t.Fatalf("GoFiles() files = %#v, want no outside-root traversal", files)
	}
}

func TestGoFilesRejectsRecursiveTraversalOutsideProjectRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(external, "outside.go"))

	files, err := discovery.GoFiles(
		context.Background(),
		[]string{external},
		discovery.Options{Root: root},
	)
	if err == nil {
		t.Fatalf("GoFiles() = %#v, want outside-root failure", files)
	}
	if files != nil {
		t.Fatalf("GoFiles() files = %#v, want no outside-root traversal", files)
	}
}

func writeDiscoveryFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
