package analysis_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"golang.org/x/tools/go/packages"
)

func TestLoadPackagesProvidesOneCanonicalTypedModuleSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "z", "z.go"), "package z\nconst Z = 1\n")
	writeLoadFixture(t, filepath.Join(root, "a", "a.go"), "package a\nconst A = 1\n")

	result, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"./..."},
		Requirement: rules.RequireTypes,
	})

	if err != nil {
		t.Fatal(err)
	}
	if result.Requirement != rules.RequireTypes || len(result.Diagnostics) != 0 ||
		len(result.Packages) != 2 {
		t.Fatalf("LoadPackages() = %#v", result)
	}
	if result.Packages[0].ID != "example.com/project/a" ||
		result.Packages[1].ID != "example.com/project/z" {
		t.Fatalf("LoadPackages() package order = %q, %q", result.Packages[0].ID, result.Packages[1].ID)
	}
	for _, loaded := range result.Packages {
		if loaded.Fset == nil || loaded.Types == nil || loaded.TypesInfo == nil ||
			len(loaded.Syntax) == 0 || len(loaded.CompiledGoFiles) == 0 {
			t.Fatalf("LoadPackages() incomplete package %q", loaded.ID)
		}
	}
}

func TestLoadPackagesRejectsCheapTiersAndPreservesCancellation(t *testing.T) {
	t.Parallel()

	if _, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         t.TempDir(),
		Patterns:    []string{"./..."},
		Requirement: rules.RequireSyntax,
	}); err == nil {
		t.Fatal("LoadPackages() accepted syntax-only requirement")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := analysis.LoadPackages(ctx, analysis.PackageLoadOptions{
		Dir:         t.TempDir(),
		Patterns:    []string{"./..."},
		Requirement: rules.RequireTypes,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadPackages() cancellation error = %v", err)
	}
}

func TestLoadPackagesIncludesInternalAndExternalTestVariants(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "project.go"), "package project\nconst Value = 1\n")
	writeLoadFixture(t, filepath.Join(root, "project_test.go"), "package project\nfunc TestInternal() {}\n")
	writeLoadFixture(t, filepath.Join(root, "external_test.go"), "package project_test\nfunc TestExternal() {}\n")

	result, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"."},
		Requirement: rules.RequireTypes,
		Tests:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var foundInternal, foundExternal bool
	for _, loaded := range result.Packages {
		if loaded.ForTest != "example.com/project" {
			continue
		}
		switch loaded.Name {
		case "project":
			foundInternal = true
		case "project_test":
			foundExternal = true
		}
	}
	if !foundInternal || !foundExternal {
		t.Fatalf("LoadPackages() test variants = internal:%t external:%t", foundInternal, foundExternal)
	}
}

func TestLoadPackagesDoesNotExecuteAmbientExternalDriver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX executable")
	}

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "project.go"), "package project\n")
	marker := filepath.Join(root, "driver-ran")
	driver := filepath.Join(root, "driver.sh")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\n: > \"$GOX_DRIVER_MARKER\"\nprintf '{\"NotHandled\":true}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"."},
		Requirement: rules.RequireTypes,
		Env: append(os.Environ(),
			"GOPACKAGESDRIVER="+driver,
			"GOX_DRIVER_MARKER="+marker,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ambient package driver marker error = %v", err)
	}
}

func TestLoadPackagesRetainsCanonicalDependencyDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "dep", "dep.go"), "package dep\nconst Value = missing\n")
	writeLoadFixture(t, filepath.Join(root, "root", "root.go"), "package root\nimport \"example.com/project/dep\"\nconst Value = dep.Value\n")

	options := analysis.PackageLoadOptions{
		Dir:                  root,
		Patterns:             []string{"./root", "./root"},
		Requirement:          rules.RequireTypes,
		LoadDependencySyntax: true,
	}
	first, err := analysis.LoadPackages(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := analysis.LoadPackages(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Packages) != 1 || first.Packages[0].ID != "example.com/project/root" {
		t.Fatalf("LoadPackages() roots = %#v", first.Packages)
	}
	if len(first.Diagnostics) == 0 || !slices.Equal(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("LoadPackages() diagnostics = %#v then %#v", first.Diagnostics, second.Diagnostics)
	}
	if !slices.IsSortedFunc(first.Diagnostics, comparePackageDiagnostics) {
		t.Fatalf("LoadPackages() diagnostics are not canonical: %#v", first.Diagnostics)
	}
	foundDependency := false
	for _, diagnostic := range first.Diagnostics {
		if diagnostic.PackageID == "example.com/project/dep" && strings.Contains(diagnostic.Message, "undefined: missing") {
			foundDependency = true
		}
	}
	if !foundDependency {
		t.Fatalf("LoadPackages() omitted dependency type error: %#v", first.Diagnostics)
	}
}

func TestLoadPackagesLoadsDependencySyntaxOnlyWhenRequested(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "dep", "dep.go"), "package dep\nconst Value = 1\n")
	writeLoadFixture(t, filepath.Join(root, "root", "root.go"), "package root\nimport \"example.com/project/dep\"\nconst Value = dep.Value\n")

	load := func(dependencies bool) analysis.PackageLoadResult {
		t.Helper()
		result, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
			Dir:                  root,
			Patterns:             []string{"./root"},
			Requirement:          rules.RequireTypes,
			LoadDependencySyntax: dependencies,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	shallow := load(false).Packages[0].Imports["example.com/project/dep"]
	if shallow == nil || shallow.Types == nil || shallow.TypesInfo != nil || len(shallow.Syntax) != 0 {
		t.Fatalf("default dependency = %#v", shallow)
	}
	deep := load(true).Packages[0].Imports["example.com/project/dep"]
	if deep == nil || deep.Types == nil || len(deep.Syntax) == 0 {
		t.Fatalf("requested dependency = %#v", deep)
	}
}

func TestLoadPackagesHonorsBuildSelectionAndOverlay(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	basePath := filepath.Join(root, "project.go")
	writeLoadFixture(t, basePath, "package project\nconst Base = 1\n")
	writeLoadFixture(t, filepath.Join(root, "feature.go"), "//go:build feature\n\npackage project\nconst Feature = 1\n")
	writeLoadFixture(t, filepath.Join(root, "target_linux.go"), "package project\nconst Target = \"linux\"\n")
	writeLoadFixture(t, filepath.Join(root, "target_darwin.go"), "package project\nconst Target = \"darwin\"\n")

	result, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"."},
		Requirement: rules.RequireTypes,
		BuildTags:   []string{"feature"},
		GOOS:        "linux",
		GOARCH:      "amd64",
		Overlay: map[string][]byte{
			basePath: []byte("package project\nconst Base = 1\nconst OverlayOnly = 1\n"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Packages) != 1 || len(result.Diagnostics) != 0 {
		t.Fatalf("LoadPackages() = %#v", result)
	}
	loaded := result.Packages[0]
	if loaded.Types.Scope().Lookup("Feature") == nil || loaded.Types.Scope().Lookup("OverlayOnly") == nil ||
		loaded.Types.Scope().Lookup("Target") == nil {
		t.Fatalf("LoadPackages() scope = %s", loaded.Types.Scope())
	}
	compiled := make([]string, len(loaded.CompiledGoFiles))
	for index, path := range loaded.CompiledGoFiles {
		compiled[index] = filepath.Base(path)
	}
	if !slices.Contains(compiled, "feature.go") || !slices.Contains(compiled, "target_linux.go") ||
		slices.Contains(compiled, "target_darwin.go") {
		t.Fatalf("LoadPackages() compiled files = %q", compiled)
	}
}

func TestLoadPackagesLoadsMultipleWorkspaceModules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.work"), "go 1.26.0\n\nuse (\n\t./one\n\t./two\n)\n")
	writeLoadFixture(t, filepath.Join(root, "one", "go.mod"), "module example.com/one\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "one", "one.go"), "package one\n")
	writeLoadFixture(t, filepath.Join(root, "two", "go.mod"), "module example.com/two\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "two", "two.go"), "package two\n")

	result, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"./one/...", "./two/..."},
		Requirement: rules.RequireTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := packageIDs(result.Packages)
	if !slices.Equal(ids, []string{"example.com/one", "example.com/two"}) {
		t.Fatalf("LoadPackages() workspace packages = %q", ids)
	}
}

func TestLoadPackagesUsesExplicitVendorMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n\nrequire example.com/dependency v1.0.0\n")
	writeLoadFixture(t, filepath.Join(root, "project.go"), "package project\nimport \"example.com/dependency\"\nconst Value = dependency.Value\n")
	writeLoadFixture(t, filepath.Join(root, "vendor", "modules.txt"), "# example.com/dependency v1.0.0\n## explicit; go 1.26.0\nexample.com/dependency\n")
	writeLoadFixture(t, filepath.Join(root, "vendor", "example.com", "dependency", "dependency.go"), "package dependency\nconst Value = 1\n")

	result, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"."},
		Requirement: rules.RequireTypes,
		ModuleMode:  analysis.ModuleVendor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Packages) != 1 || len(result.Diagnostics) != 0 ||
		result.Packages[0].Types.Scope().Lookup("Value") == nil {
		t.Fatalf("LoadPackages(vendor) = %#v", result)
	}
}

func TestLoadPackagesDefaultsOfflineAndRequiresNetworkOptIn(t *testing.T) {
	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n\nrequire example.com/dependency v1.0.0\n")
	writeLoadFixture(t, filepath.Join(root, "go.sum"), "example.com/dependency v1.0.0 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\nexample.com/dependency v1.0.0/go.mod h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n")
	writeLoadFixture(t, filepath.Join(root, "project.go"), "package project\nimport _ \"example.com/dependency\"\n")

	var requests atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.NotFound(writer, request)
	}))
	t.Cleanup(proxy.Close)

	offline, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:         root,
		Patterns:    []string{"."},
		Requirement: rules.RequireTypes,
		Env:         packageLoadTestEnvironment(t, proxy.URL),
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 || len(offline.Diagnostics) == 0 {
		t.Fatalf("offline load requests = %d, diagnostics = %#v", requests.Load(), offline.Diagnostics)
	}

	_, err = analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir:          root,
		Patterns:     []string{"."},
		Requirement:  rules.RequireTypes,
		Env:          packageLoadTestEnvironment(t, proxy.URL),
		AllowNetwork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() == 0 {
		t.Fatal("network-enabled package load did not reach configured proxy")
	}
}

func TestLoadPackagesValidatesRequestIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeLoadFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.26.0\n")
	writeLoadFixture(t, filepath.Join(root, "project.go"), "package project\n")
	cases := []struct {
		name    string
		options analysis.PackageLoadOptions
	}{
		{name: "relative directory", options: analysis.PackageLoadOptions{Dir: ".", Patterns: []string{"."}, Requirement: rules.RequireTypes}},
		{name: "missing directory", options: analysis.PackageLoadOptions{Dir: filepath.Join(root, "missing"), Patterns: []string{"."}, Requirement: rules.RequireTypes}},
		{name: "no patterns", options: analysis.PackageLoadOptions{Dir: root, Requirement: rules.RequireTypes}},
		{name: "empty pattern", options: analysis.PackageLoadOptions{Dir: root, Patterns: []string{" "}, Requirement: rules.RequireTypes}},
		{name: "relative overlay", options: analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}, Requirement: rules.RequireTypes, Overlay: map[string][]byte{"project.go": []byte("package project\n")}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := analysis.LoadPackages(context.Background(), test.options); err == nil {
				t.Fatal("LoadPackages() accepted invalid request")
			}
		})
	}
}

func comparePackageDiagnostics(left, right analysis.PackageDiagnostic) int {
	if left.PackageID != right.PackageID {
		return strings.Compare(left.PackageID, right.PackageID)
	}
	if left.Position != right.Position {
		return strings.Compare(left.Position, right.Position)
	}
	if left.Message != right.Message {
		return strings.Compare(left.Message, right.Message)
	}
	return int(left.Kind - right.Kind)
}

func packageIDs(packages []*packages.Package) []string {
	ids := make([]string, len(packages))
	for index, loaded := range packages {
		ids[index] = loaded.ID
	}
	return ids
}

func packageLoadTestEnvironment(t *testing.T, proxy string) []string {
	t.Helper()
	return append(os.Environ(),
		"GOMODCACHE="+t.TempDir(),
		"GOPROXY="+proxy,
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
	)
}

func writeLoadFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
