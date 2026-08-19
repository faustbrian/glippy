package analysis

import (
	"context"
	"errors"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/faustbrian/glippy/internal/rules"
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

func TestPackageSessionRejectsGeneratedCompiledTestRoots(t *testing.T) {
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
	cgoRoot := testVariant
	cgoRoot.CompiledGoFiles = append(
		cgoRoot.CompiledGoFiles,
		filepath.Join(root, "_cgo_gotypes.go"),
	)
	loaded.Packages = []*packages.Package{&cgoRoot}
	if _, retained := newPackageSessionEntry(loaded, PackageLoadOptions{Dir: root}); retained {
		t.Fatal("cgo-generated compiled root was retained for incremental type checking")
	}
}

func TestPackageSessionRootFamilyRejectsMalformedAndAmbiguousVariants(t *testing.T) {
	t.Parallel()

	base := &packages.Package{
		ID: "example.com/sample",
		PkgPath: "example.com/sample",
		Name: "sample",
	}
	internal := &packages.Package{
		ID: "example.com/sample [example.com/sample.test]",
		PkgPath: "example.com/sample",
		Name: "sample",
		ForTest: "example.com/sample",
	}
	external := &packages.Package{
		ID: "example.com/sample_test [example.com/sample.test]",
		PkgPath: "example.com/sample_test",
		Name: "sample_test",
		ForTest: "example.com/sample",
	}
	tests := []struct {
		name string
		roots []*packages.Package
		wantIDs []string
	}{
		{name: "empty"},
		{
			name: "internal only",
			roots: []*packages.Package{internal},
			wantIDs: []string{internal.ID},
		},
		{
			name: "external only",
			roots: []*packages.Package{external},
			wantIDs: []string{external.ID},
		},
		{
			name: "complete family is ordered",
			roots: []*packages.Package{external, internal, base},
			wantIDs: []string{base.ID, internal.ID, external.ID},
		},
		{
			name: "too many roots",
			roots: []*packages.Package{base, internal, external, base},
		},
		{name: "duplicate rank", roots: []*packages.Package{base, base}},
		{
			name: "duplicate identity across ranks",
			roots: []*packages.Package{
				base,
				{
					ID: base.ID,
					PkgPath: internal.PkgPath,
					Name: internal.Name,
					ForTest: internal.ForTest,
				},
			},
		},
		{
			name: "mismatched test owner",
			roots: []*packages.Package{
				base,
				{
					ID: "example.com/sample [example.com/other.test]",
					PkgPath: "example.com/sample",
					Name: "sample",
					ForTest: "example.com/other",
				},
			},
		},
		{
			name: "mismatched external name",
			roots: []*packages.Package{
				base,
				{
					ID: "example.com/sample_test [example.com/sample.test]",
					PkgPath: "example.com/sample_test",
					Name: "other_test",
					ForTest: "example.com/sample",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				family, ok := packageSessionRootFamily(test.roots)
				if len(test.wantIDs) == 0 {
					if ok {
						t.Fatalf(
							"root family = %#v, want rejection",
							family,
						)
					}
					return
				}
				if !ok || len(family) != len(test.wantIDs) {
					t.Fatalf(
						"root family = %#v, want IDs %#v",
						family,
						test.wantIDs,
					)
				}
				for index, id := range test.wantIDs {
					if family[index].ID != id {
						t.Fatalf(
							"root family[%d].ID = %q, want %q",
							index,
							family[index].ID,
							id,
						)
					}
				}
			},
		)
	}
}

func TestPackageSessionVariantImportsRequireSelectedFreshRoot(t *testing.T) {
	t.Parallel()

	base := &packages.Package{
		ID: "example.com/sample",
		PkgPath: "example.com/sample",
		Name: "sample",
	}
	internal := &packages.Package{
		ID: "example.com/sample [example.com/sample.test]",
		PkgPath: "example.com/sample",
		Name: "sample",
		ForTest: "example.com/sample",
	}
	external := &packages.Package{
		ID: "example.com/sample_test [example.com/sample.test]",
		PkgPath: "example.com/sample_test",
		Name: "sample_test",
		ForTest: "example.com/sample",
		Imports: map[string]*packages.Package{"example.com/sample": internal},
	}
	if !validPackageSessionVariantImports([]*packages.Package{base, internal, external}) {
		t.Fatal("complete test family rejected its selected internal import")
	}
	if validPackageSessionVariantImports([]*packages.Package{base, external}) {
		t.Fatal("partial test family accepted an unselected internal import")
	}
	if !validPackageSessionVariantImports([]*packages.Package{external}) {
		t.Fatal("isolated external test root rejected its closed dependency")
	}
}

func TestPackageSessionRebindsExternalTestImportToFreshInternalVariant(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modulePath := filepath.Join(root, "go.mod")
	productionPath := filepath.Join(root, "sample.go")
	internalTestPath := filepath.Join(root, "sample_test.go")
	externalTestPath := filepath.Join(root, "external_test.go")
	writePackageSessionFixture(t, modulePath, "module example.com/sample\n\ngo 1.26.0\n")
	originalProduction := "package sample\n\nfunc Value() int { return 1 }\n"
	changedProduction := "package sample\n\nfunc Value() string { return \"value\" }\n"
	writePackageSessionFixture(t, productionPath, originalProduction)
	writePackageSessionFixture(
		t,
		internalTestPath,
		"package sample\n\nfunc TestInternal() { _ = Value() }\n",
	)
	originalExternal := `package sample_test

import sample "example.com/sample"

var _ int = sample.Value()
`
	changedExternal := `package sample_test

import sample "example.com/sample"

var _ string = sample.Value()
`
	writePackageSessionFixture(t, externalTestPath, originalExternal)
	session := NewPackageSession()
	options := PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		Requirement: rules.RequireTypes,
		Tests: true,
		ModuleMode: ModuleReadonly,
	}
	if _, err := session.load(context.Background(), "1.26", options); err != nil {
		t.Fatal(err)
	}
	options.Overlay = map[string][]byte{
		productionPath: []byte(changedProduction),
		externalTestPath: []byte(changedExternal),
	}
	loaded, err := session.load(context.Background(), "1.26", options)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Diagnostics) != 0 {
		t.Fatalf("incremental external-test diagnostics = %#v", loaded.Diagnostics)
	}
	statistics := session.Statistics()
	if statistics.FullLoads != 1 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want fresh test-family recheck",
			statistics,
		)
	}
}

func TestPackageSessionFallsBackWhenExternalTestDependencyOverlayChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modulePath := filepath.Join(root, "go.mod")
	productionPath := filepath.Join(root, "sample.go")
	externalTestPath := filepath.Join(root, "external_test.go")
	writePackageSessionFixture(t, modulePath, "module example.com/sample\n\ngo 1.26.0\n")
	originalProduction := "package sample\n\nfunc Value() int { return 1 }\n"
	changedProduction := "package sample\n\nfunc Value() string { return \"value\" }\n"
	writePackageSessionFixture(t, productionPath, originalProduction)
	originalExternal := `package sample_test

import sample "example.com/sample"

var _ int = sample.Value()
`
	changedExternal := `package sample_test

import sample "example.com/sample"

// Retained external test variant.
var _ int = sample.Value()
`
	dependencyChangedExternal := `package sample_test

import sample "example.com/sample"

// Retained external test variant.
var _ string = sample.Value()
`
	writePackageSessionFixture(t, externalTestPath, originalExternal)
	session := NewPackageSession()
	options := PackageLoadOptions{
		Dir: root,
		Patterns: []string{"file=" + externalTestPath},
		Requirement: rules.RequireTypes,
		Tests: true,
		ModuleMode: ModuleReadonly,
		Overlay: map[string][]byte{externalTestPath: []byte(originalExternal)},
	}
	if _, err := session.load(context.Background(), "1.26", options); err != nil {
		t.Fatal(err)
	}
	options.Overlay = map[string][]byte{externalTestPath: []byte(changedExternal)}
	if _, err := session.load(context.Background(), "1.26", options); err != nil {
		t.Fatal(err)
	}
	options.Overlay = map[string][]byte{
		productionPath: []byte(changedProduction),
		externalTestPath: []byte(dependencyChangedExternal),
	}
	loaded, err := session.load(context.Background(), "1.26", options)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Diagnostics) != 0 {
		t.Fatalf("full external-test diagnostics = %#v", loaded.Diagnostics)
	}
	statistics := session.Statistics()
	if statistics.FullLoads != 2 || statistics.IncrementalLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want dependency-overlay fallback",
			statistics,
		)
	}
}

func TestPackageSessionFallsBackWhenIncrementalImportLoadFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modulePath := filepath.Join(root, "go.mod")
	path := filepath.Join(root, "sample.go")
	writePackageSessionFixture(t, modulePath, "module example.com/sample\n\ngo 1.26.0\n")
	original := "package sample\n\nfunc Value() int { return 1 }\n"
	changed := "package sample\n\nimport \"strings\"\n\nfunc Value() string { return strings.TrimSpace(\" value \") }\n"
	writePackageSessionFixture(t, path, original)
	session := NewPackageSession()
	loadError := errors.New("incremental import metadata unavailable")
	session.loadImports = func(
		_ context.Context,
		options PackageLoadOptions,
		paths []string,
	) (PackageLoadResult, error) {
		if options.Requirement != rules.RequireTypes ||
			options.Tests ||
			options.LoadDependencySyntax ||
			options.LoadEffectFacts {
			t.Fatalf("incremental import options = %#v", options)
		}
		if !slices.Equal(options.Patterns, []string{"pattern=strings"}) {
			t.Fatalf("incremental import patterns = %#v", options.Patterns)
		}
		if len(paths) != 1 || paths[0] != "strings" {
			t.Fatalf("incremental import paths = %#v", paths)
		}
		return PackageLoadResult{}, loadError
	}
	options := PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		Requirement: rules.RequireTypes,
		ModuleMode: ModuleReadonly,
	}
	if _, err := session.load(context.Background(), "1.26", options); err != nil {
		t.Fatal(err)
	}
	options.Overlay = map[string][]byte{path: []byte(changed)}
	loaded, err := session.load(context.Background(), "1.26", options)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Diagnostics) != 0 {
		t.Fatalf("fallback package diagnostics = %#v", loaded.Diagnostics)
	}
	statistics := session.Statistics()
	if statistics.FullLoads != 2 ||
		statistics.IncrementalLoads != 0 ||
		statistics.ImportLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want import-load fallback",
			statistics,
		)
	}
}

func TestPackageSessionEscapesExactImportLoadPatterns(t *testing.T) {
	t.Parallel()

	selected := packageSessionImportLoadOptions(
		PackageLoadOptions{Patterns: []string{"./..."}},
		[]string{"file=example.com/project/value", "all", "example.com/project/...", "all"},
	)
	want := []string{
		"pattern=all",
		"pattern=example.com/project/...",
		"pattern=file=example.com/project/value",
	}
	if !slices.Equal(selected.Patterns, want) {
		t.Fatalf("incremental import patterns = %#v, want %#v", selected.Patterns, want)
	}
}

func TestPackageSessionCancelsIncrementalImportLoad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modulePath := filepath.Join(root, "go.mod")
	path := filepath.Join(root, "sample.go")
	writePackageSessionFixture(t, modulePath, "module example.com/sample\n\ngo 1.26.0\n")
	original := "package sample\n\nfunc Value() int { return 1 }\n"
	changed := "package sample\n\nimport \"strings\"\n\nfunc Value() string { return strings.TrimSpace(\" value \") }\n"
	writePackageSessionFixture(t, path, original)
	session := NewPackageSession()
	options := PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		Requirement: rules.RequireTypes,
		ModuleMode: ModuleReadonly,
	}
	if _, err := session.load(context.Background(), "1.26", options); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	session.loadImports = func(
		ctx context.Context,
		_ PackageLoadOptions,
		_ []string,
	) (PackageLoadResult, error) {
		cancel()
		return PackageLoadResult{}, ctx.Err()
	}
	options.Overlay = map[string][]byte{path: []byte(changed)}
	if _, err := session.load(ctx, "1.26", options); !errors.Is(err, context.Canceled) {
		t.Fatalf("incremental import cancellation error = %v", err)
	}
	statistics := session.Statistics()
	if statistics.FullLoads != 1 ||
		statistics.IncrementalLoads != 0 ||
		statistics.ImportLoads != 1 {
		t.Fatalf(
			"typed package session statistics = %#v, want canceled import load",
			statistics,
		)
	}
}

func TestPackageSessionImportAdmissionPreservesGoVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		root string
		importPath string
		allowed bool
	}{
		{
			name: "ordinary dependency",
			root: "example.com/project/app",
			importPath: "example.com/dependency",
			allowed: true,
		},
		{
			name: "owned internal dependency",
			root: "example.com/project/app",
			importPath: "example.com/project/internal/hidden",
			allowed: true,
		},
		{
			name: "foreign internal dependency",
			root: "example.com/project/app",
			importPath: "example.com/dependency/internal/hidden",
		},
		{
			name: "toolchain internal dependency",
			root: "example.com/project/app",
			importPath: "internal/abi",
		},
		{
			name: "vendor path",
			root: "example.com/project/app",
			importPath: "example.com/project/vendor/example.com/dependency",
		},
		{
			name: "self import",
			root: "example.com/project/app",
			importPath: "example.com/project/app",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				if got := packageSessionImportAllowed(test.root, test.importPath);
					got != test.allowed {
					t.Fatalf(
						"import admission = %t, want %t",
						got,
						test.allowed,
					)
				}
			},
		)
	}
}

func writePackageSessionFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
