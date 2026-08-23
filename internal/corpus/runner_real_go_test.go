package corpus

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/mod/module"
)

func TestPrefetchRepositoryModulesCompletesGraphWithoutChangingMetadata(t *testing.T) {
	t.Parallel()

	temporaryRoot := t.TempDir()
	root, err := filepath.EvalSymlinks(temporaryRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := removeReadOnlyTree(root); err != nil {
				t.Errorf("remove fixture: %v", err)
			}
		},
	)
	proxyRoot := filepath.Join(root, "proxy")
	writeProxyModule(
		t,
		proxyRoot,
		"example.com/transitive",
		"v1.0.0",
		"module example.com/transitive\n\ngo 1.26\n",
		map[string]string{"transitive.go": "package transitive\n\nconst Value = 1\n"},
	)
	writeProxyModule(
		t,
		proxyRoot,
		"example.com/direct",
		"v1.0.0",
		"module example.com/direct\n\ngo 1.26\n\nrequire example.com/transitive v1.0.0\n",
		map[string]string{"direct.go": "package direct\n\nconst Value = 1\n"},
	)

	checkout := filepath.Join(root, "checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(
		t,
		filepath.Join(checkout, "go.mod"),
		"module example.com/main\n\ngo 1.26\n\nrequire example.com/direct v1.0.0\n",
	)
	writeTestFile(
		t,
		filepath.Join(checkout, "main.go"),
		"package main\n\nimport _ \"example.com/direct\"\n\nfunc main() {}\n",
	)

	setupCache := filepath.Join(root, "setup-gomodcache")
	setupEnvironment := realGoFixtureEnvironment(
		t,
		proxyRoot,
		setupCache,
		filepath.Join(root, "setup-gocache"),
		filepath.Join(root, "setup-tmp"),
		"",
	)
	direct := exactModuleSums(t, setupEnvironment, root, "example.com/direct@v1.0.0")
	transitive := exactModuleSums(t, setupEnvironment, root, "example.com/transitive@v1.0.0")
	goSumPath := filepath.Join(checkout, "go.sum")
	goSum := []byte(
		fmt.Sprintf(
			"example.com/direct v1.0.0 %s\n" +
				"example.com/direct v1.0.0/go.mod %s\n" +
				"example.com/transitive v1.0.0/go.mod %s\n",
			direct.Sum,
			direct.GoModSum,
			transitive.GoModSum,
		),
	)
	if err := os.WriteFile(goSumPath, goSum, 0o600); err != nil {
		t.Fatal(err)
	}
	goModBefore, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goSumBefore := slices.Clone(goSum)

	ambientRoot := filepath.Join(root, "ambient")
	if err := os.MkdirAll(ambientRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ambientModulePath := filepath.Join(ambientRoot, "go.mod")
	ambientModule := []byte("module example.com/ambient\n\ngo 1.26\n")
	if err := os.WriteFile(ambientModulePath, ambientModule, 0o600); err != nil {
		t.Fatal(err)
	}
	ambientSumPath := filepath.Join(ambientRoot, "go.sum")
	cacheRoot := filepath.Join(ambientRoot, "corpus-cache")
	executor := &moduleContextExecutor{
		localProxyExecutor: localProxyExecutor{proxyRoot: proxyRoot},
	}
	err = prefetchRepositoryModules(
		context.Background(),
		RunOptions{
			CacheRoot: cacheRoot,
			Executor: executor,
			downloadEnvironment: os.Environ(),
		},
		Repository{ID: "fixture"},
		checkout,
	)
	if err != nil {
		t.Fatal(err)
	}

	goModAfter, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goSumAfter, err := os.ReadFile(goSumPath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(goModAfter, goModBefore) || !slices.Equal(goSumAfter, goSumBefore) {
		t.Fatal("module prefetch changed source metadata")
	}
	ambientModuleAfter, err := os.ReadFile(ambientModulePath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ambientModuleAfter, ambientModule) {
		t.Fatal("module prefetch changed ambient go.mod")
	}
	if _, err := os.Stat(ambientSumPath); !os.IsNotExist(err) {
		t.Fatalf("module prefetch created ambient go.sum: %v", err)
	}
	if executor.downloadModuleFile == "" ||
		!pathAtOrWithin(cacheRoot, executor.downloadModuleFile) {
		t.Fatalf(
			"exact module download context = %q, want task-owned module metadata",
			executor.downloadModuleFile,
		)
	}

	offlineEnvironment := realGoFixtureEnvironment(
		t,
		proxyRoot,
		filepath.Join(cacheRoot, "gomodcache"),
		filepath.Join(root, "offline-gocache"),
		filepath.Join(root, "offline-tmp"),
		"-mod=readonly",
	)
	offlineEnvironment = replaceEnvironment(
		offlineEnvironment,
		map[string]string{"GOPROXY": "off", "GOVCS": "*:off"},
	)
	result, err := (osExecutor{}).Run(
		context.Background(),
		Command{
			Path: "go",
			Args: []string{"list", "-deps", "./..."},
			Dir: checkout,
			Env: offlineEnvironment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("offline package loading failed: %s", result.Stderr)
	}
}

func TestPrefetchRepositoryModulesHonorsWorkspaceVersionReplacements(t *testing.T) {
	t.Parallel()

	temporaryRoot := t.TempDir()
	root, err := filepath.EvalSymlinks(temporaryRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := removeReadOnlyTree(root); err != nil {
				t.Errorf("remove fixture: %v", err)
			}
		},
	)
	proxyRoot := filepath.Join(root, "proxy")
	writeProxyModule(
		t,
		proxyRoot,
		"example.com/replacement",
		"v1.0.0",
		"module example.com/replacement\n\ngo 1.26\n",
		map[string]string{"replacement.go": "package replacement\n\nconst Value = 1\n"},
	)
	setupEnvironment := realGoFixtureEnvironment(
		t,
		proxyRoot,
		filepath.Join(root, "setup-gomodcache"),
		filepath.Join(root, "setup-gocache"),
		filepath.Join(root, "setup-tmp"),
		"",
	)
	replacement := exactModuleSums(t, setupEnvironment, root, "example.com/replacement@v1.0.0")
	checkout := filepath.Join(root, "checkout")
	moduleRoot := filepath.Join(checkout, "app")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	modulePath := filepath.Join(moduleRoot, "go.mod")
	moduleInput := []byte(
		"module example.com/app\n\ngo 1.26\n\nrequire example.com/original v1.0.0\n",
	)
	if err := os.WriteFile(modulePath, moduleInput, 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(
		t,
		filepath.Join(moduleRoot, "app.go"),
		"package app\n\nimport _ \"example.com/original\"\n",
	)
	workspacePath := filepath.Join(checkout, "go.work")
	workspaceInput := []byte(
		"go 1.26\n\nuse ./app\n\n" +
			"replace example.com/original => example.com/replacement v1.0.0\n",
	)
	if err := os.WriteFile(workspacePath, workspaceInput, 0o600); err != nil {
		t.Fatal(err)
	}
	workspaceSumPath := filepath.Join(checkout, "go.work.sum")
	workspaceSumInput := []byte(
		fmt.Sprintf(
			"example.com/replacement v1.0.0 %s\n" +
				"example.com/replacement v1.0.0/go.mod %s\n",
			replacement.Sum,
			replacement.GoModSum,
		),
	)
	if err := os.WriteFile(workspaceSumPath, workspaceSumInput, 0o600); err != nil {
		t.Fatal(err)
	}

	cacheRoot := filepath.Join(root, "corpus-cache")
	err = prefetchRepositoryModules(
		context.Background(),
		RunOptions{
			CacheRoot: cacheRoot,
			Executor: localProxyExecutor{proxyRoot: proxyRoot},
			downloadEnvironment: os.Environ(),
		},
		Repository{ID: "workspace-fixture"},
		checkout,
	)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range
		map[string][]byte{
			modulePath: moduleInput,
			workspacePath: workspaceInput,
			workspaceSumPath: workspaceSumInput,
		} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("module prefetch changed %s", filepath.Base(path))
		}
	}

	offlineEnvironment := realGoFixtureEnvironment(
		t,
		proxyRoot,
		filepath.Join(cacheRoot, "gomodcache"),
		filepath.Join(root, "offline-gocache"),
		filepath.Join(root, "offline-tmp"),
		"-mod=readonly",
	)
	offlineEnvironment = replaceEnvironment(
		offlineEnvironment,
		map[string]string{"GOPROXY": "off", "GOVCS": "*:off", "GOWORK": workspacePath},
	)
	result, err := (osExecutor{}).Run(
		context.Background(),
		Command{
			Path: "go",
			Args: []string{"list", "-deps", "./..."},
			Dir: moduleRoot,
			Env: offlineEnvironment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("offline workspace loading failed: %s", result.Stderr)
	}
}

func TestResolvedModuleDownloadsUsesExactVersionedTargets(t *testing.T) {
	t.Parallel()

	resolved, err := resolvedModuleDownloads(
		[]byte(
			"{\"Path\":\"example.com/main\",\"Main\":true}\n" +
				"{\"Path\":\"example.com/direct\",\"Version\":\"v1.2.0\"}\n" +
				"{\"Path\":\"example.com/replaced\",\"Version\":\"v1.0.0\",\"Replace\":{\"Path\":\"example.com/replacement\",\"Version\":\"v1.3.0\"}}\n" +
				"{\"Path\":\"example.com/local\",\"Version\":\"v1.0.0\",\"Replace\":{\"Path\":\"../local\"}}\n" +
				"{\"Path\":\"example.com/direct\",\"Version\":\"v1.2.0\"}\n",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"example.com/direct@v1.2.0", "example.com/replacement@v1.3.0"}
	if !slices.Equal(resolved, want) {
		t.Fatalf("resolved module downloads = %v, want %v", resolved, want)
	}
}

func TestModuleDownloadBatchesAreDeterministicAndBounded(t *testing.T) {
	t.Parallel()

	downloads := make(map[string]struct{}, maximumModuleDownloadArguments + 1)
	for index := maximumModuleDownloadArguments; index >= 0; index-- {
		downloads[fmt.Sprintf("example.com/module-%03d@v1.0.0", index)] = struct{}{}
	}
	batches := moduleDownloadBatches(downloads)
	if len(batches) != 2 ||
		len(batches[0]) != maximumModuleDownloadArguments ||
		len(batches[1]) != 1 {
		t.Fatalf("module download batches = %v, want 128 and 1 arguments", batches)
	}
	flattened := slices.Concat(batches...)
	if !slices.IsSorted(flattened) {
		t.Fatalf("module download batches are not sorted: %v", flattened)
	}
}

type moduleSums struct {
	Sum string
	GoModSum string
}

func exactModuleSums(t *testing.T, environment []string, directory, version string) moduleSums {
	t.Helper()
	result, err := (osExecutor{}).Run(
		context.Background(),
		Command{
			Path: "go",
			Args: []string{"mod", "download", "-json", version},
			Dir: directory,
			Env: environment,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("prepare module metadata for %s: %s", version, result.Stderr)
	}
	var sums moduleSums
	if err := json.Unmarshal(result.Stdout, &sums); err != nil {
		t.Fatal(err)
	}
	if sums.Sum == "" || sums.GoModSum == "" {
		t.Fatalf("module metadata for %s lacks checksums: %s", version, result.Stdout)
	}
	return sums
}

type localProxyExecutor struct {
	proxyRoot string
}

type moduleContextExecutor struct {
	localProxyExecutor
	downloadModuleFile string
}

func (e *moduleContextExecutor) Run(ctx context.Context, command Command) (CommandResult, error) {
	if command.Path == "go" &&
		len(command.Args) > 2 &&
		slices.Equal(command.Args[:2], []string{"mod", "download"}) {
		probe := command
		probe.Args = []string{"env", "GOMOD"}
		result, err := (osExecutor{}).Run(ctx, probe)
		if err != nil {
			return CommandResult{}, err
		}
		if result.ExitCode != 0 {
			return result, nil
		}
		e.downloadModuleFile = string(bytes.TrimSpace(result.Stdout))
	}
	return e.localProxyExecutor.Run(ctx, command)
}

func (e localProxyExecutor) Run(ctx context.Context, command Command) (CommandResult, error) {
	command.Env = replaceEnvironment(
		command.Env,
		map[string]string{
			"GONOSUMDB": "*",
			"GOPRIVATE": "",
			"GOPROXY": "file://" + filepath.ToSlash(e.proxyRoot),
			"GOSUMDB": "off",
			"GOVCS": "*:off",
		},
	)
	return (osExecutor{}).Run(ctx, command)
}

func realGoFixtureEnvironment(
	t *testing.T,
	proxyRoot, moduleCache, buildCache, temporary, flags string,
) []string {
	t.Helper()
	for _, directory := range []string{moduleCache, buildCache, temporary} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return replaceEnvironment(
		os.Environ(),
		map[string]string{
			"GOCACHE": buildCache,
			"GOCACHEPROG": "",
			"GOENV": "off",
			"GOFLAGS": flags,
			"GOMODCACHE": moduleCache,
			"GONOSUMDB": "*",
			"GOPRIVATE": "",
			"GOPROXY": "file://" + filepath.ToSlash(proxyRoot),
			"GOSUMDB": "off",
			"GOTOOLCHAIN": "local",
			"GOVCS": "*:off",
			"GOTMPDIR": temporary,
			"GOWORK": "off",
			"TMPDIR": temporary,
		},
	)
}

func writeProxyModule(
	t *testing.T,
	proxyRoot, modulePath, version, goMod string,
	files map[string]string,
) {
	t.Helper()
	escapedPath, err := module.EscapePath(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	versionRoot := filepath.Join(proxyRoot, filepath.FromSlash(escapedPath), "@v")
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(versionRoot, escapedVersion + ".mod"), goMod)
	writeTestFile(
		t,
		filepath.Join(versionRoot, escapedVersion + ".info"),
		fmt.Sprintf("{\"Version\":%q,\"Time\":\"2026-01-01T00:00:00Z\"}\n", version),
	)

	archivePath := filepath.Join(versionRoot, escapedVersion + ".zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archive)
	contents := map[string]string{"go.mod": goMod}
	for path, input := range files {
		contents[path] = input
	}
	for path, input := range contents {
		entry, createErr := zipWriter.Create(modulePath + "@" + version + "/" + path)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write([]byte(input)); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path, input string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
}
