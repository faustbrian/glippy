package analysis

import (
	"context"
	"errors"
	"go/token"
	"go/types"
	"path/filepath"
	"testing"
	"time"

	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

func TestPackageSessionInvalidationDoesNotWaitForFullLoad(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	loadError := errors.New("stopped test package load")
	session := NewPackageSession()
	session.loadPackages = func(
		context.Context,
		PackageLoadOptions,
	) (PackageLoadResult, error) {
		close(started)
		<-release
		return PackageLoadResult{}, loadError
	}
	loadDone := make(chan error, 1)
	go func() {
		_, err := session.load(context.Background(), "1.26", PackageLoadOptions{})
		loadDone <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("timed out waiting for full package load")
	}
	invalidationDone := make(chan struct{})
	go func() {
		session.InvalidateAll()
		close(invalidationDone)
	}()
	returnedPromptly := false
	select {
	case <-invalidationDone:
		returnedPromptly = true
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-loadDone; !errors.Is(err, loadError) {
		t.Fatalf("full package load error = %v, want %v", err, loadError)
	}
	if !returnedPromptly {
		<-invalidationDone
		t.Fatal("package session invalidation waited for a full package load")
	}
}

func TestPackageSessionBoundsRetainedGraphsByRecencyAndBytes(t *testing.T) {
	t.Parallel()

	session := NewPackageSession()
	for index := 0; index < maximumPackageSessionEntries + 2; index++ {
		var key packageSessionKey
		key[0] = byte(index)
		session.entries[key] = packageSessionEntry{
			accountedBytes: 1,
			used: uint64(index + 1),
		}
	}
	session.bound()
	if len(session.entries) != maximumPackageSessionEntries {
		t.Fatalf(
			"bounded package session entries = %d, want %d",
			len(session.entries),
			maximumPackageSessionEntries,
		)
	}
	for index := 0; index < 2; index++ {
		var key packageSessionKey
		key[0] = byte(index)
		if _, found := session.entries[key]; found {
			t.Fatalf("least-recently-used package session entry %d was retained", index)
		}
	}

	var olderKey, newerKey packageSessionKey
	olderKey[0] = 1
	newerKey[0] = 2
	session.entries = map[packageSessionKey]packageSessionEntry{
		olderKey: {accountedBytes: maximumPackageSessionBytes * 3 / 4, used: 1},
		newerKey: {accountedBytes: maximumPackageSessionBytes * 3 / 4, used: 2},
	}
	session.bound()
	if len(session.entries) != 1 {
		t.Fatalf("byte-bounded package session entries = %d, want 1", len(session.entries))
	}
	if _, found := session.entries[newerKey]; !found {
		t.Fatal("newer package session entry was evicted before the older entry")
	}
}

func TestPackageSessionRejectsTestVariantsAndGeneratedCompiledRoots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	file, err := source.Load(path, []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	base := &packages.Package{
		ID: "example.com/sample",
		Name: "sample",
		PkgPath: "example.com/sample",
		Dir: root,
		GoFiles: []string{path},
		CompiledGoFiles: []string{path},
		Fset: token.NewFileSet(),
		Types: types.NewPackage("example.com/sample", "sample"),
		TypesInfo: &types.Info{},
		TypesSizes: types.SizesFor("gc", "amd64"),
		Imports: map[string]*packages.Package{},
	}
	loaded := PackageLoadResult{
		Packages: []*packages.Package{base},
		Sources: PackageSourceSet{
			paths: []string{path},
			files: map[string]*source.File{path: file},
		},
	}
	testVariant := *base
	testVariant.ForTest = "example.com/sample"
	loaded.Packages = []*packages.Package{&testVariant}
	if _, retained := newPackageSessionEntry(loaded, PackageLoadOptions{Dir: root}); retained {
		t.Fatal("test package variant was retained for incremental type checking")
	}
	cgoRoot := *base
	cgoRoot.CompiledGoFiles = append(
		cgoRoot.CompiledGoFiles,
		filepath.Join(root, "_cgo_gotypes.go"),
	)
	loaded.Packages = []*packages.Package{&cgoRoot}
	if _, retained := newPackageSessionEntry(loaded, PackageLoadOptions{Dir: root}); retained {
		t.Fatal("cgo-generated compiled root was retained for incremental type checking")
	}
}
