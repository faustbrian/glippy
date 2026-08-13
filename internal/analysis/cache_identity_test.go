package analysis

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestBuildPackageCacheKeyCapturesLoadedGraphDeterministically(t *testing.T) {
	t.Parallel()

	fixture := newPackageCacheIdentityFixture(t)
	canonical, err := buildPackageCacheKey(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	reordered := fixture.input
	reordered.LoadOptions.Patterns = slices.Clone(reordered.LoadOptions.Patterns)
	slices.Reverse(reordered.LoadOptions.Patterns)
	reordered.LoadOptions.BuildTags = slices.Clone(reordered.LoadOptions.BuildTags)
	slices.Reverse(reordered.LoadOptions.BuildTags)
	reordered.LoadOptions.Env = slices.Clone(reordered.LoadOptions.Env)
	slices.Reverse(reordered.LoadOptions.Env)
	reordered.Loaded.Packages = slices.Clone(reordered.Loaded.Packages)
	slices.Reverse(reordered.Loaded.Packages)
	reorderedKey, err := buildPackageCacheKey(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != reorderedKey {
		t.Fatalf("reordered package cache keys = %q and %q", canonical, reorderedKey)
	}
	equivalent := fixture.input
	equivalent.LoadOptions.Patterns = append(
		slices.Clone(equivalent.LoadOptions.Patterns),
		equivalent.LoadOptions.Patterns[0],
	)
	equivalent.LoadOptions.BuildTags = append(
		slices.Clone(equivalent.LoadOptions.BuildTags),
		equivalent.LoadOptions.BuildTags[0],
	)
	equivalentKey, err := buildPackageCacheKey(equivalent)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != equivalentKey {
		t.Fatalf("equivalent package cache keys = %q and %q", canonical, equivalentKey)
	}
	locationOnly := fixture.input
	locationOnly.LoadOptions.Env = append(
		slices.Clone(locationOnly.LoadOptions.Env),
		"GOCACHE=/another/disposable/cache",
		"UNRELATED_SECRET=changed",
	)
	dependency := locationOnly.Loaded.Packages[0].Imports["example.com/dependency"]
	relocatedExport := writePackageCacheFile(t, t.TempDir(), "dependency.a", "export data")
	dependency.ExportFile = relocatedExport
	locationKey, err := buildPackageCacheKey(locationOnly)
	if err != nil {
		t.Fatal(err)
	}
	if locationKey != canonical {
		t.Fatalf("location-only package cache keys = %q and %q", canonical, locationKey)
	}
	dependency.ExportFile = fixture.exportFile

	assertPackageCacheKeyChanges(
		t,
		fixture.input,
		canonical,
		func(input *packageCacheKeyInput) {
			file, err := source.Load(
				fixture.rootSource,
				[]byte("package root\nvar Changed int\n"),
			)
			if err != nil {
				t.Fatal(err)
			}
			input.Loaded.Sources.files = clonePackageSourceFiles(
				input.Loaded.Sources.files,
			)
			input.Loaded.Sources.files[fixture.rootSource] = file
		},
	)
	assertPackageCacheKeyChanges(
		t,
		fixture.input,
		canonical,
		func(input *packageCacheKeyInput) {
			input.LoadOptions.Tests = true
		},
	)
	assertPackageCacheKeyChanges(
		t,
		fixture.input,
		canonical,
		func(input *packageCacheKeyInput) {
			input.LoadOptions.Env = replacePackageCacheEnvironment(
				input.LoadOptions.Env,
				"GOAMD64=v3",
			)
		},
	)
	assertPackageCacheKeyChanges(
		t,
		fixture.input,
		canonical,
		func(input *packageCacheKeyInput) {
			input.LoadOptions.Overlay = map[string][]byte{
				fixture.rootSource: []byte("overlay changed"),
			}
		},
	)
	assertPackageCacheKeyChanges(
		t,
		fixture.input,
		canonical,
		func(input *packageCacheKeyInput) {
			input.Facts = map[string]cache.Digest{
				"dependency:fact": cache.DigestOf([]byte("changed")),
			}
		},
	)

	for _, path := range
		[]string{
			fixture.rootModule,
			fixture.rootSum,
			fixture.workspace,
			fixture.workspaceSum,
			fixture.vendorModules,
			fixture.exportFile,
		} {
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(slices.Clone(original), 'x'), 0o600);
			err != nil {
			t.Fatal(err)
		}
		changed, err := buildPackageCacheKey(fixture.input)
		if err != nil {
			t.Fatal(err)
		}
		if changed == canonical {
			t.Fatalf("package cache key did not change for %s", filepath.Base(path))
		}
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildPackageCacheKeyRejectsIncompleteGraphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mutate func(*packageCacheKeyInput)
	}{
		{
			name: "ambient environment",
			mutate: func(input *packageCacheKeyInput) {
				input.LoadOptions.Env = nil
			},
		},
		{
			name: "ambient Go environment",
			mutate: func(input *packageCacheKeyInput) {
				input.LoadOptions.Env = replacePackageCacheEnvironment(
					input.LoadOptions.Env,
					"GOENV=default",
				)
			},
		},
		{
			name: "ambient CGO",
			mutate: func(input *packageCacheKeyInput) {
				input.LoadOptions.Env = replacePackageCacheEnvironment(
					input.LoadOptions.Env,
					"CGO_ENABLED=",
				)
			},
		},
		{
			name: "GOOS",
			mutate: func(input *packageCacheKeyInput) {
				input.LoadOptions.GOOS = ""
			},
		},
		{
			name: "GOARCH",
			mutate: func(input *packageCacheKeyInput) {
				input.LoadOptions.GOARCH = ""
			},
		},
		{
			name: "load diagnostics",
			mutate: func(input *packageCacheKeyInput) {
				input.Loaded.Diagnostics = []PackageDiagnostic{{Message: "broken"}}
			},
		},
		{
			name: "ill typed package",
			mutate: func(input *packageCacheKeyInput) {
				input.Loaded.Packages[0].IllTyped = true
			},
		},
		{
			name: "missing root source",
			mutate: func(input *packageCacheKeyInput) {
				input.Loaded.Sources.files = clonePackageSourceFiles(
					input.Loaded.Sources.files,
				)
				delete(
					input.Loaded.Sources.files,
					input.Loaded.Packages[0].CompiledGoFiles[0],
				)
			},
		},
		{
			name: "root replaced by dependency export",
			mutate: func(input *packageCacheKeyInput) {
				rootPath := input.Loaded.Packages[0].CompiledGoFiles[0]
				dependency := input.
					Loaded.
					Packages[0].
					Imports["example.com/dependency"]
				input.Loaded.Sources.paths = []string{dependency.CompiledGoFiles[0]}
				input.Loaded.Sources.files = map[string]*source.File{
					dependency.CompiledGoFiles[0]: input.
						Loaded.
						Sources.
						files[rootPath],
				}
				input.Loaded.Packages[0].ExportFile = dependency.ExportFile
			},
		},
		{
			name: "missing dependency evidence",
			mutate: func(input *packageCacheKeyInput) {
				dependency := input.
					Loaded.
					Packages[0].
					Imports["example.com/dependency"]
				dependency.ExportFile = ""
			},
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				fixture := newPackageCacheIdentityFixture(t)
				input := fixture.input
				test.mutate(&input)
				if _, err := buildPackageCacheKey(input); err == nil {
					t.Fatalf("buildPackageCacheKey() accepted %s", test.name)
				}
			},
		)
	}
}

func TestBuildPackageCacheKeyUsesWorkspaceVendorRootFromSubdirectory(t *testing.T) {
	t.Parallel()

	fixture := newPackageCacheIdentityFixture(t)
	subdirectory := filepath.Join(filepath.Dir(fixture.rootModule), "nested", "package")
	if err := os.MkdirAll(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.input.LoadOptions.Dir = subdirectory
	if _, err := buildPackageCacheKey(fixture.input); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPackageCacheKeyAcceptsCompletePackageLoad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePackageCacheFile(t, root, "go.mod", "module example.com/root\n\ngo 1.26.0\n")
	writePackageCacheFile(
		t,
		root,
		"dependency/dependency.go",
		"package dependency\nimport _ \"fmt\"\nconst Value = 1\n",
	)
	writePackageCacheFile(
		t,
		root,
		"root.go",
		"package root\nimport _ \"example.com/root/dependency\"\n",
	)
	loadOptions := PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		Requirement: rules.RequireTypes,
		LoadDependencySyntax: true,
		ModuleMode: ModuleReadonly,
		Env: append(os.Environ(), "CGO_ENABLED=0", "GOENV=off"),
		GOOS: runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
	loaded, err := LoadPackages(context.Background(), loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildPackageCacheKey(
		packageCacheKeyInput{
			Namespace: "typed-analysis:test",
			ToolVersion: "v0.1.0",
			BuildGoVersion: runtime.Version(),
			SourceGoVersion: "1.26",
			Configuration: cache.DigestOf([]byte("configuration")),
			Rules: []cache.RuleInput{
				{ID: "test", Severity: "warn", Options: cache.DigestOf(nil)},
			},
			FormatterMode: "glippy-v1",
			LoadOptions: loadOptions,
			Loaded: loaded,
			Facts: map[string]cache.Digest{},
		},
	);
		err != nil {
		t.Fatal(err)
	}
}

type packageCacheIdentityFixture struct {
	input packageCacheKeyInput
	rootSource string
	rootModule string
	rootSum string
	workspace string
	workspaceSum string
	vendorModules string
	exportFile string
}

func newPackageCacheIdentityFixture(t *testing.T) packageCacheIdentityFixture {
	t.Helper()
	root := t.TempDir()
	dependencyRoot := t.TempDir()
	rootSource := writePackageCacheFile(t, root, "root.go", "package root\n")
	dependencySource := writePackageCacheFile(
		t,
		dependencyRoot,
		"dependency.go",
		"package dependency\n",
	)
	rootModule := writePackageCacheFile(
		t,
		root,
		"go.mod",
		"module example.com/root\n\ngo 1.26\n",
	)
	rootSum := writePackageCacheFile(
		t,
		root,
		"go.sum",
		"example.com/dependency v1.0.0 h1:sum\n",
	)
	workspace := writePackageCacheFile(t, root, "go.work", "go 1.26\n\nuse .\n")
	workspaceSum := writePackageCacheFile(
		t,
		root,
		"go.work.sum",
		"example.com/dependency v1.0.0 h1:sum\n",
	)
	vendorModules := writePackageCacheFile(
		t,
		root,
		"vendor/modules.txt",
		"# example.com/dependency v1.0.0\n",
	)
	dependencyModule := writePackageCacheFile(
		t,
		dependencyRoot,
		"go.mod",
		"module example.com/dependency\n\ngo 1.26\n",
	)
	writePackageCacheFile(t, dependencyRoot, "go.sum", "")
	exportFile := writePackageCacheFile(t, dependencyRoot, "dependency.a", "export data")
	rootFile, err := source.Load(rootSource, []byte("package root\n"))
	if err != nil {
		t.Fatal(err)
	}
	dependency := &packages.Package{
		ID: "example.com/dependency",
		Name: "dependency",
		PkgPath: "example.com/dependency",
		GoFiles: []string{dependencySource},
		CompiledGoFiles: []string{dependencySource},
		ExportFile: exportFile,
		Module: &packages.Module{
			Path: "example.com/dependency",
			Version: "v1.0.0",
			GoMod: dependencyModule,
			GoVersion: "1.26",
		},
		Imports: map[string]*packages.Package{},
	}
	rootPackage := &packages.Package{
		ID: "example.com/root",
		Name: "root",
		PkgPath: "example.com/root",
		GoFiles: []string{rootSource},
		CompiledGoFiles: []string{rootSource},
		Module: &packages.Module{
			Path: "example.com/root",
			GoMod: rootModule,
			GoVersion: "1.26",
			Main: true,
		},
		Imports: map[string]*packages.Package{"example.com/dependency": dependency},
	}
	input := packageCacheKeyInput{
		Namespace: "typed-analysis:example-rule",
		ToolVersion: "v0.1.0",
		BuildGoVersion: "go1.26.5",
		SourceGoVersion: "1.26",
		Configuration: cache.DigestOf([]byte("configuration")),
		Rules: []cache.RuleInput{
			{ID: "example-rule", Severity: "warn", Options: cache.DigestOf(nil)},
		},
		CGOEnabled: false,
		FormatterMode: "glippy-v1",
		LoadOptions: PackageLoadOptions{
			Dir: root,
			Patterns: []string{".", "./..."},
			Requirement: rules.RequireTypes,
			Tests: false,
			LoadDependencySyntax: false,
			BuildTags: []string{"integration", "linux"},
			ModuleMode: ModuleVendor,
			Env: []string{
				"GOWORK=" + workspace,
				"GOAMD64=v1",
				"GOENV=off",
				"CGO_ENABLED=0",
			},
			Overlay: map[string][]byte{rootSource: []byte("overlay")},
			GOOS: "linux",
			GOARCH: "amd64",
		},
		Loaded: PackageLoadResult{
			Requirement: rules.RequireTypes,
			Packages: []*packages.Package{rootPackage},
			Sources: PackageSourceSet{
				paths: []string{rootSource},
				files: map[string]*source.File{rootSource: rootFile},
			},
		},
		Facts: map[string]cache.Digest{"dependency:fact": cache.DigestOf([]byte("fact"))},
	}
	return packageCacheIdentityFixture{
		input: input,
		rootSource: rootSource,
		rootModule: rootModule,
		rootSum: rootSum,
		workspace: workspace,
		workspaceSum: workspaceSum,
		vendorModules: vendorModules,
		exportFile: exportFile,
	}
}

func assertPackageCacheKeyChanges(
	t *testing.T,
	input packageCacheKeyInput,
	base cache.Key,
	mutate func(*packageCacheKeyInput),
) {
	t.Helper()
	mutate(&input)
	changed, err := buildPackageCacheKey(input)
	if err != nil {
		t.Fatal(err)
	}
	if changed == base {
		t.Fatal("package cache key did not change")
	}
}

func clonePackageSourceFiles(input map[string]*source.File) map[string]*source.File {
	result := make(map[string]*source.File, len(input))
	for path, file := range input {
		result[path] = file
	}
	return result
}

func replacePackageCacheEnvironment(environment []string, replacement string) []string {
	result := slices.Clone(environment)
	name, _, _ := strings.Cut(replacement, "=")
	for index, entry := range result {
		if strings.HasPrefix(entry, name + "=") {
			result[index] = replacement
			return result
		}
	}
	return append(result, replacement)
}

func writePackageCacheFile(t *testing.T, root string, relative string, contents string) string {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
