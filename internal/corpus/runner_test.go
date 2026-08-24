package corpus_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/corpus"
)

func TestRunAuditsProfilesAndComparatorsWithoutMutatingCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	checkoutRoot := filepath.Join(root, "checkouts")
	outputRoot := filepath.Join(root, "results")
	cacheRoot := filepath.Join(root, "cache")
	checkout := filepath.Join(checkoutRoot, "alpha")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, input := range
		map[string]string{
			filepath.Join(checkout, "LICENSE"): "license\n",
			filepath.Join(checkout, "go.mod"): "module example.com/alpha\n\ngo 1.26\n",
		} {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	executor := &corpusExecutor{
		revision: strings.Repeat("a", 40),
		repository: "https://github.com/example/alpha.git",
		checkout: checkout,
	}
	manifest, err := corpus.ParseManifest(
		[]byte(
			strings.ReplaceAll(
				validManifestJSON,
				`,
    {
      "id": "beta",
      "repository": "https://github.com/example/beta.git",
      "revision": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "license": "MIT",
      "license_path": "COPYING",
      "roles": ["library"],
      "go_directive": "1.24",
      "source_version_policy": "unsupported",
      "cgo": true,
      "generated": false,
      "patterns": ["."]
    }`,
				"",
			),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = corpus.Run(
		context.Background(),
		manifest,
		corpus.RunOptions{
			RunID: "source-aaaaaaaa-run-1",
			CheckoutRoot: checkoutRoot,
			OutputRoot: outputRoot,
			CacheRoot: cacheRoot,
			GlippyPath: "/tools/glippy",
			StaticcheckPath: "/tools/staticcheck",
			Executor: executor,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	wantProfiles := []string{"default", "recommended", "strict", "pedantic"}
	if !slices.Equal(executor.profiles, wantProfiles) {
		t.Fatalf("profile execution order = %v, want %v", executor.profiles, wantProfiles)
	}
	if executor.vetRuns != 1 || executor.staticcheckRuns != 1 || executor.statusRuns != 2 {
		t.Fatalf(
			"comparator/status runs = vet %d, staticcheck %d, status %d",
			executor.vetRuns,
			executor.staticcheckRuns,
			executor.statusRuns,
		)
	}
	if !slices.ContainsFunc(
		executor.commands,
		func(command corpus.Command) bool {
			return command.Path == "go" &&
				len(command.Args) >= 4 &&
				slices.Equal(
					command.Args[:4],
					[]string{"list", "-deps", "-test", "-export"},
				)
		},
	) {
		t.Fatal("analysis package preflight did not run")
	}

	findings, err := os.ReadFile(filepath.Join(outputRoot, "alpha", "default", "findings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(findings), checkout) ||
		!strings.Contains(string(findings), `"path": "sample.go"`) ||
		!strings.Contains(string(findings), `"rule_id": "sample-rule"`) ||
		!strings.Contains(string(findings), `"fingerprint": "`) {
		t.Fatalf("normalized findings = %s", findings)
	}
	result, err := os.ReadFile(filepath.Join(outputRoot, "alpha", "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), checkout) ||
		!strings.Contains(string(result), `"run_id": "source-aaaaaaaa-run-1"`) ||
		!strings.Contains(string(result), `"staticcheck_version": "v0.8.1"`) ||
		!strings.Contains(string(result), `"exit_code": 1`) {
		t.Fatalf("result artifact = %s", result)
	}
}

func TestRunRejectsDirtyOrMismatchedCheckoutsBeforeAnalysis(t *testing.T) {
	t.Parallel()

	for _, test := range
		[]struct {
			name string
			revision string
			status string
			want string
		}{
			{
				name: "revision mismatch",
				revision: strings.Repeat("b", 40),
				want: "revision",
			},
			{
				name: "dirty checkout",
				revision: strings.Repeat("a", 40),
				status: "?? generated.txt\n",
				want: "not clean",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				checkoutRoot := filepath.Join(root, "checkouts")
				checkout := filepath.Join(checkoutRoot, "alpha")
				if err := os.MkdirAll(checkout, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(checkout, "LICENSE"),
					[]byte("license\n"),
					0o600,
				);
					err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(checkout, "go.mod"),
					[]byte("module example.com/alpha\n\ngo 1.26\n"),
					0o600,
				);
					err != nil {
					t.Fatal(err)
				}
				manifest := corpus.Manifest{
					SchemaVersion: 1,
					StaticcheckVersion: "v0.8.1",
					Repositories: []corpus.Repository{
						{
							ID: "alpha",
							Repository: "https://github.com/example/alpha.git",
							Revision: strings.Repeat("a", 40),
							License: "Apache-2.0",
							LicensePath: "LICENSE",
							Roles: []string{"cli"},
							GoDirective: "1.26",
							SourceVersionPolicy: corpus.SourceVersionSupported,
							Patterns: []string{"./..."},
						},
					},
				}
				executor := &corpusExecutor{
					revision: test.revision,
					repository: manifest.Repositories[0].Repository,
					status: test.status,
					checkout: checkout,
				}
				err := corpus.Run(
					context.Background(),
					manifest,
					corpus.RunOptions{
						RunID: "source-aaaaaaaa-run-1",
						CheckoutRoot: checkoutRoot,
						OutputRoot: filepath.Join(root, "results"),
						CacheRoot: filepath.Join(root, "cache"),
						GlippyPath: "/tools/glippy",
						StaticcheckPath: "/tools/staticcheck",
						Executor: executor,
					},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"Run() error = %v, want containing %q",
						err,
						test.want,
					)
				}
				if len(executor.profiles) != 0 {
					t.Fatalf(
						"analysis ran after invalid checkout: %v",
						executor.profiles,
					)
				}
			},
		)
	}
}

func TestRunRejectsStaticcheckPatchVersionPrefixMatches(t *testing.T) {
	t.Parallel()

	manifest, options, executor, _ := newRunFixture(t)
	executor.staticcheckVersion = "staticcheck v0.8.10"
	err := corpus.Run(context.Background(), manifest, options)
	if err == nil || !strings.Contains(err.Error(), "does not identify v0.8.1") {
		t.Fatalf("Run() error = %v, want exact Staticcheck version rejection", err)
	}
}

func TestRunRejectsOutputAndCacheSymlinkAliasesIntoCheckouts(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"output", "cache"} {
		t.Run(
			target,
			func(t *testing.T) {
				t.Parallel()
				manifest, options, _, checkout := newRunFixture(t)
				alias := filepath.Join(
					filepath.Dir(options.CheckoutRoot),
					target + "-alias",
				)
				if err := os.Symlink(checkout, alias); err != nil {
					t.Fatal(err)
				}
				if target == "output" {
					options.OutputRoot = alias
				} else {
					options.CacheRoot = alias
				}
				err := corpus.Run(context.Background(), manifest, options)
				if err == nil ||
					!strings.Contains(err.Error(), "outside checkouts") {
					t.Fatalf(
						"Run() error = %v, want symlink boundary rejection",
						err,
					)
				}
			},
		)
	}
}

func TestRunUsesIsolatedEnvironmentAndReadOnlyCheckoutSnapshot(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	executor.probeReadOnly = true
	options.Environment = []string{
		"PATH=/usr/bin",
		"HOME=/users/real",
		"GOFLAGS=-mod=vendor",
		"GOMEMLIMIT=off",
		"GOWORK=/users/real/go.work",
		"GOPATH=/users/real/go",
		"GOCACHEPROG=remote-cache",
		"GIT_DIR=/users/real/other.git",
		"SECRET_TOKEN=do-not-inherit",
		"XDG_CONFIG_HOME=/users/real/config",
	}
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	canonicalCheckoutRoot, err := filepath.EvalSymlinks(options.CheckoutRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalCheckout, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		t.Fatal(err)
	}
	canonicalCacheRoot, err := filepath.EvalSymlinks(options.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, command := range executor.commands {
		if command.Path != "git" &&
			(command.Dir == canonicalCheckoutRoot || command.Dir == canonicalCheckout) {
			t.Fatalf(
				"%s inspected from writable checkout path %q",
				command.Path,
				command.Dir,
			)
		}
		for _, forbidden := range
			[]string{
				"HOME=/users/real",
				"GOFLAGS=-mod=vendor",
				"GOMEMLIMIT=off",
				"GOWORK=/users/real/go.work",
				"GOPATH=/users/real/go",
				"GOCACHEPROG=remote-cache",
				"GIT_DIR=/users/real/other.git",
				"SECRET_TOKEN=do-not-inherit",
				"XDG_CONFIG_HOME=/users/real/config",
			} {
			if slices.Contains(command.Env, forbidden) {
				t.Fatalf(
					"%s inherited forbidden environment variable %q",
					command.Path,
					forbidden,
				)
			}
		}
		for _, required := range
			[]string{
				"GIT_CONFIG_GLOBAL=/dev/null",
				"GIT_CONFIG_NOSYSTEM=1",
				"GIT_OPTIONAL_LOCKS=0",
				"GIT_TERMINAL_PROMPT=0",
				"GOMEMLIMIT=4GiB",
				"GOWORK=off",
				"GOCACHEPROG=",
				"GONOPROXY=none",
				"GOPATH=" + filepath.Join(canonicalCacheRoot, "gopath"),
			} {
			if !slices.Contains(command.Env, required) {
				t.Fatalf(
					"%s environment is missing %q: %v",
					command.Path,
					required,
					corpusEnvironment(command.Env),
				)
			}
		}
		if command.Path == "go" &&
			slices.Equal(
				command.Args,
				[]string{"list", "-mod=readonly", "-m", "-json", "all"},
			) {
			for _, required := range
				[]string{
					"GOFLAGS=-mod=readonly",
					"GOPROXY=https://proxy.golang.org,direct",
					"GOVCS=public:git|hg,private:off",
				} {
				if !slices.Contains(command.Env, required) {
					t.Fatalf(
						"module download environment is missing %q: %v",
						required,
						corpusEnvironment(command.Env),
					)
				}
			}
			continue
		}
		if command.Path == "go" &&
			len(command.Args) > 2 &&
			slices.Equal(command.Args[:2], []string{"mod", "download"}) {
			for _, required := range
				[]string{
					"GOFLAGS=",
					"GOPROXY=https://proxy.golang.org,direct",
					"GOVCS=public:git|hg,private:off",
				} {
				if !slices.Contains(command.Env, required) {
					t.Fatalf(
						"exact module download environment is missing %q: %v",
						required,
						corpusEnvironment(command.Env),
					)
				}
			}
			continue
		}
		for _, required := range []string{"GOFLAGS=", "GOVCS=*:off"} {
			if !slices.Contains(command.Env, required) {
				t.Fatalf(
					"%s environment is missing %q: %v",
					command.Path,
					required,
					corpusEnvironment(command.Env),
				)
			}
		}
	}
	for _, command := range executor.commands {
		if command.Path == "/tools/glippy" &&
			len(command.Args) > 0 &&
			command.Args[0] == "lint" {
			canonicalCache, err := filepath.EvalSymlinks(options.CacheRoot)
			if err != nil {
				t.Fatal(err)
			}
			if command.Dir == checkout ||
				!strings.HasPrefix(command.Dir, canonicalCache) {
				t.Fatalf(
					"analysis directory = %q, want task-owned snapshot",
					command.Dir,
				)
			}
			if executor.snapshotWriteError == nil {
				t.Fatal("analysis snapshot unexpectedly permits source writes")
			}
			break
		}
	}
}

func TestRunMaterializesWorkspaceSumInsideSnapshotBeforeLockdown(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse .\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	executor.materializeWorkspaceSum = true
	executor.probeReadOnly = true

	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	if executor.moduleGraphWriteError != nil {
		t.Fatalf("materialize snapshot go.work.sum: %v", executor.moduleGraphWriteError)
	}
	if !executor.workspaceSumObserved {
		t.Fatal("analysis did not observe the materialized snapshot go.work.sum")
	}
	if _, err := os.Stat(filepath.Join(checkout, "go.work.sum")); !os.IsNotExist(err) {
		t.Fatalf("source checkout go.work.sum changed: %v", err)
	}
	if executor.snapshotWriteError == nil {
		t.Fatal("analysis snapshot unexpectedly permits source writes")
	}
}

func TestRunAllowsOnlyWorkspaceSumUpdateDuringOfflinePreflight(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse .\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	executor.materializeWorkspaceSum = true
	executor.updateWorkspaceSumDuringPreflight = true
	executor.createPreflightSourceFile = true
	executor.probeReadOnly = true

	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	if executor.preflightWorkspaceSumWriteError != nil {
		t.Fatalf(
			"update snapshot go.work.sum during preflight: %v",
			executor.preflightWorkspaceSumWriteError,
		)
	}
	if executor.preflightSourceWriteError == nil {
		t.Fatal("offline preflight unexpectedly permits other source writes")
	}
	if executor.workspaceSumWriteAfterPreflightError == nil {
		t.Fatal("analysis snapshot unexpectedly permits go.work.sum writes")
	}
	if _, err := os.Stat(filepath.Join(checkout, "go.work.sum")); !os.IsNotExist(err) {
		t.Fatalf("source checkout go.work.sum changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "preflight-mutation.txt"));
		!os.IsNotExist(err) {
		t.Fatalf("source checkout preflight-mutation.txt changed: %v", err)
	}
	if len(executor.profiles) != 4 {
		t.Fatalf("profiles after preflight = %v, want all profiles", executor.profiles)
	}
}

func TestRunRejectsModuleGraphChangesOutsideWorkspaceSum(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse .\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	executor.materializeWorkspaceSum = true
	executor.createModuleGraphSourceFile = true

	err := corpus.Run(context.Background(), manifest, options)
	if err == nil || !strings.Contains(err.Error(), "outside go.work.sum") {
		t.Fatalf("Run() error = %v, want snapshot delta rejection", err)
	}
	if executor.moduleGraphSourceWriteError != nil {
		t.Fatalf(
			"create task-owned snapshot mutation: %v",
			executor.moduleGraphSourceWriteError,
		)
	}
	if _, err := os.Stat(filepath.Join(checkout, "mutation.txt")); !os.IsNotExist(err) {
		t.Fatalf("source checkout mutation.txt changed: %v", err)
	}
}

func TestRunBindsTheWorkspaceInsideTheReadOnlySnapshot(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse .\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	for _, command := range executor.commands {
		if command.Path == "/tools/glippy" &&
			len(command.Args) > 0 &&
			command.Args[0] == "lint" {
			want := "GOWORK=" + filepath.Join(command.Dir, "go.work")
			if !slices.Contains(command.Env, want) {
				t.Fatalf(
					"analysis environment = %v, want %q",
					corpusEnvironment(command.Env),
					want,
				)
			}
			return
		}
	}
	t.Fatal("Glippy lint command did not run")
}

func TestRunPreflightsTheOfflineWorkspaceBeforeGlippyProfiles(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse .\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	preflightIndex := -1
	profileIndex := -1
	var preflight, profile corpus.Command
	for index, command := range executor.commands {
		switch {
		case command.Path == "go" &&
			len(command.Args) >= 4 &&
			slices.Equal(
				command.Args[:4],
				[]string{"list", "-deps", "-test", "-export"},
			):
			preflightIndex = index
			preflight = command
		case command.Path == "/tools/glippy" &&
			len(command.Args) > 0 &&
			command.Args[0] == "lint" &&
			profileIndex == -1:
			profileIndex = index
			profile = command
		}
	}
	if preflightIndex == -1 || profileIndex == -1 || preflightIndex >= profileIndex {
		t.Fatalf(
			"command order has preflight %d and first profile %d, want preflight first",
			preflightIndex,
			profileIndex,
		)
	}
	if environmentValue(preflight.Env, "GOPROXY") != "off" ||
		environmentValue(preflight.Env, "GOWORK") !=
			filepath.Join(preflight.Dir, "go.work") {
		t.Fatalf(
			"preflight environment = %v, want offline snapshot workspace",
			corpusEnvironment(preflight.Env),
		)
	}
	if environmentValue(preflight.Env, "GOMODCACHE") == "" ||
		environmentValue(preflight.Env, "GOMODCACHE") !=
			environmentValue(profile.Env, "GOMODCACHE") {
		t.Fatalf(
			"preflight and profile module caches differ: %v, %v",
			corpusEnvironment(preflight.Env),
			corpusEnvironment(profile.Env),
		)
	}
	if environmentValue(preflight.Env, "GOCACHE") == "" ||
		environmentValue(preflight.Env, "GOCACHE") ==
			environmentValue(profile.Env, "GOCACHE") {
		t.Fatalf(
			"preflight reused profile build cache: %v, %v",
			corpusEnvironment(preflight.Env),
			corpusEnvironment(profile.Env),
		)
	}
	canonicalCacheRoot, err := filepath.EvalSymlinks(options.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	wantPreflightCache := filepath.Join(
		canonicalCacheRoot,
		"repositories",
		"alpha",
		"preflight",
		"gocache",
	)
	wantProfileCache := filepath.Join(
		canonicalCacheRoot,
		"repositories",
		"alpha",
		"analysis",
		"gocache",
	)
	if environmentValue(preflight.Env, "GOCACHE") != wantPreflightCache ||
		environmentValue(profile.Env, "GOCACHE") != wantProfileCache {
		t.Fatalf(
			"preflight and profile caches = %q, %q; want %q, %q",
			environmentValue(preflight.Env, "GOCACHE"),
			environmentValue(profile.Env, "GOCACHE"),
			wantPreflightCache,
			wantProfileCache,
		)
	}
}

func TestRunRecordsIncompleteEvidenceWhenOfflinePreflightFails(t *testing.T) {
	t.Parallel()

	manifest, options, executor, _ := newRunFixture(t)
	executor.preflightExitCode = 1
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatalf("Run() error = %v, want recorded incomplete evidence", err)
	}
	if len(executor.profiles) != 0 {
		t.Fatalf("profiles ran after failed preflight: %v", executor.profiles)
	}
	if executor.vetRuns != 0 || executor.staticcheckRuns != 0 {
		t.Fatalf(
			"comparators ran after failed preflight: vet %d, staticcheck %d",
			executor.vetRuns,
			executor.staticcheckRuns,
		)
	}
	manifestInput, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	template, err := corpus.BuildAdjudicationTemplate(
		manifest,
		manifestInput,
		options.OutputRoot,
	)
	if err != nil {
		t.Fatalf("BuildAdjudicationTemplate() error = %v", err)
	}
	var adjudication struct {
		Repositories []struct {
			IncompleteProfiles []string `json:"incomplete_profiles"`
			IncompleteComparators []string `json:"incomplete_comparators"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(template, &adjudication); err != nil {
		t.Fatal(err)
	}
	if len(adjudication.Repositories) != 1 ||
		!slices.Equal(
			adjudication.Repositories[0].IncompleteProfiles,
			[]string{"default", "recommended"},
		) ||
		!slices.Equal(
			adjudication.Repositories[0].IncompleteComparators,
			[]string{"go-vet", "staticcheck"},
		) {
		t.Fatalf("incomplete adjudication = %+v", adjudication.Repositories)
	}
}

func TestRunPrefetchesEveryWorkspaceModuleBeforeOfflineAnalysis(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	nested := filepath.Join(checkout, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(nested, "go.mod"),
		[]byte("module example.com/alpha/nested\n\ngo 1.26\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse (\n\t./nested\n\t.\n)\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	if len(executor.moduleResolutions) != 1 {
		t.Fatalf(
			"workspace module resolutions = %v, want one aggregate workspace graph",
			executor.moduleResolutions,
		)
	}
	rootResolution := executor.moduleResolutions[0]
	if filepath.Base(rootResolution.Dir) != "source" {
		t.Fatalf(
			"workspace module resolution directory = %q, want snapshot root",
			rootResolution.Dir,
		)
	}
	for _, command := range executor.moduleResolutions {
		if pathAtOrWithinForTest(checkout, command.Dir) {
			t.Fatalf("module resolution used writable checkout %q", command.Dir)
		}
		for _, required := range
			[]string{
				"GOFLAGS=-mod=readonly",
				"GOWORK=" + filepath.Join(rootResolution.Dir, "go.work"),
				"GOCACHEPROG=",
				"GONOPROXY=none",
			} {
			if !slices.Contains(command.Env, required) {
				t.Fatalf(
					"module download environment is missing %q: %v",
					required,
					corpusEnvironment(command.Env),
				)
			}
		}
		if environmentValue(command.Env, "GOPROXY") == "off" {
			t.Fatalf(
				"module resolution disabled its network source: %v",
				corpusEnvironment(command.Env),
			)
		}
	}
	if len(executor.moduleDownloads) != 1 {
		t.Fatalf(
			"workspace module downloads = %v, want one batch",
			executor.moduleDownloads,
		)
	}
	download := executor.moduleDownloads[0]
	if !slices.Equal(
		download.Args,
		[]string{
			"mod",
			"download",
			"example.com/nested-dependency@v1.0.0",
			"example.com/root-dependency@v1.0.0",
		},
	) {
		t.Fatalf(
			"workspace module download = %v, want exact resolved versions",
			download.Args,
		)
	}
	if pathAtOrWithinForTest(rootResolution.Dir, download.Dir) {
		t.Fatalf("exact module download ran inside source module %q", download.Dir)
	}
}

func TestRunRejectsWorkspaceModulesOutsideTheReadOnlySnapshot(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse ../outside\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	err := corpus.Run(context.Background(), manifest, options)
	if err == nil || !strings.Contains(err.Error(), "outside the checkout") {
		t.Fatalf("Run() error = %v, want workspace boundary refusal", err)
	}
	if len(executor.moduleResolutions) != 0 ||
		len(executor.moduleDownloads) != 0 ||
		len(executor.profiles) != 0 {
		t.Fatalf(
			"commands ran after workspace boundary refusal: downloads %v, profiles %v",
			executor.moduleDownloads,
			executor.profiles,
		)
	}
}

func TestRunRejectsWorkspaceReplacementsOutsideTheReadOnlySnapshot(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte("go 1.26\n\nuse .\n\n" + "replace example.com/outside => ../outside\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	err := corpus.Run(context.Background(), manifest, options)
	if err == nil ||
		!strings.Contains(err.Error(), "replacement") ||
		!strings.Contains(err.Error(), "outside the checkout") {
		t.Fatalf("Run() error = %v, want workspace replacement boundary refusal", err)
	}
	if len(executor.moduleResolutions) != 0 ||
		len(executor.moduleDownloads) != 0 ||
		len(executor.profiles) != 0 {
		t.Fatalf(
			"commands ran after workspace replacement refusal: downloads %v, profiles %v",
			executor.moduleDownloads,
			executor.profiles,
		)
	}
}

func TestRunRejectsLocalModuleReplacementsOutsideTheReadOnlySnapshot(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	if err := os.WriteFile(
		filepath.Join(checkout, "go.mod"),
		[]byte(
			"module example.com/alpha\n\ngo 1.26\n\n" +
				"replace example.com/outside => ../outside\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	err := corpus.Run(context.Background(), manifest, options)
	if err == nil ||
		!strings.Contains(err.Error(), "replacement") ||
		!strings.Contains(err.Error(), "outside the checkout") {
		t.Fatalf("Run() error = %v, want module replacement boundary refusal", err)
	}
	if len(executor.moduleResolutions) != 0 ||
		len(executor.moduleDownloads) != 0 ||
		len(executor.profiles) != 0 {
		t.Fatalf(
			"commands ran after module replacement refusal: downloads %v, profiles %v",
			executor.moduleDownloads,
			executor.profiles,
		)
	}
}

func TestRunAllowsLocalReplacementsInsideTheReadOnlySnapshot(t *testing.T) {
	t.Parallel()

	manifest, options, executor, checkout := newRunFixture(t)
	nested := filepath.Join(checkout, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(nested, "go.mod"),
		[]byte("module example.com/nested\n\ngo 1.26\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(checkout, "go.mod"),
		[]byte(
			"module example.com/alpha\n\ngo 1.26\n\n" +
				"replace example.com/module-dependency => ./nested\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(checkout, "go.work"),
		[]byte(
			"go 1.26\n\nuse .\n\n" +
				"replace example.com/workspace-dependency => ./nested\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := corpus.Run(context.Background(), manifest, options); err != nil {
		t.Fatal(err)
	}
	if len(executor.moduleResolutions) != 1 ||
		len(executor.moduleDownloads) != 1 ||
		len(executor.profiles) != 4 {
		t.Fatalf(
			"commands after valid local replacements: downloads %v, profiles %v",
			executor.moduleDownloads,
			executor.profiles,
		)
	}
}

func TestRunAlwaysPerformsPostRunCheckoutVerification(t *testing.T) {
	t.Parallel()

	manifest, options, executor, _ := newRunFixture(t)
	executor.analysisError = errors.New("analysis failed")
	executor.statuses = []string{"", "!! ignored-output\n"}
	err := corpus.Run(context.Background(), manifest, options)
	if err == nil ||
		!strings.Contains(err.Error(), "analysis failed") ||
		!strings.Contains(err.Error(), "post-run checkout verification") {
		t.Fatalf("Run() error = %v, want analysis and post-run verification failures", err)
	}
	if executor.statusRuns != 2 {
		t.Fatalf("checkout status runs = %d, want 2", executor.statusRuns)
	}
	for _, command := range executor.commands {
		if command.Path == "git" &&
			len(command.Args) > 0 &&
			command.Args[0] == "status" &&
			!slices.Contains(command.Args, "--ignored=matching") {
			t.Fatalf(
				"checkout status arguments = %v, want ignored files included",
				command.Args,
			)
		}
	}
}

func TestRunResultDigestExcludesVolatileStatistics(t *testing.T) {
	t.Parallel()

	manifest, firstOptions, firstExecutor, _ := newRunFixture(t)
	firstExecutor.statistics = []string{
		`{"duration_ns":1,"allocations":2,"allocated_bytes":3}`,
		`{"duration_ns":4,"allocations":5,"allocated_bytes":6}`,
		`{"duration_ns":7,"allocations":8,"allocated_bytes":9}`,
		`{"duration_ns":10,"allocations":11,"allocated_bytes":12}`,
	}
	firstExecutor.includeTaskPath = true
	if err := corpus.Run(context.Background(), manifest, firstOptions); err != nil {
		t.Fatal(err)
	}
	firstResult, err := os.ReadFile(
		filepath.Join(firstOptions.OutputRoot, "alpha", "result.json"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, secondOptions, secondExecutor, _ := newRunFixture(t)
	secondExecutor.statistics = []string{
		`{"duration_ns":101,"allocations":102,"allocated_bytes":103}`,
		`{"duration_ns":104,"allocations":105,"allocated_bytes":106}`,
		`{"duration_ns":107,"allocations":108,"allocated_bytes":109}`,
		`{"duration_ns":110,"allocations":111,"allocated_bytes":112}`,
	}
	secondExecutor.includeTaskPath = true
	if err := corpus.Run(context.Background(), manifest, secondOptions); err != nil {
		t.Fatal(err)
	}
	secondResult, err := os.ReadFile(
		filepath.Join(secondOptions.OutputRoot, "alpha", "result.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstResult, secondResult) {
		t.Fatalf(
			"result.json changed with volatile statistics:\nfirst: %s\nsecond: %s",
			firstResult,
			secondResult,
		)
	}
}

func newRunFixture(t *testing.T) (corpus.Manifest, corpus.RunOptions, *corpusExecutor, string) {
	t.Helper()

	root := t.TempDir()
	checkoutRoot := filepath.Join(root, "checkouts")
	checkout := filepath.Join(checkoutRoot, "alpha")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, input := range
		map[string]string{
			filepath.Join(checkout, "LICENSE"): "license\n",
			filepath.Join(checkout, "go.mod"): "module example.com/alpha\n\ngo 1.26\n",
		} {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := corpus.Manifest{
		SchemaVersion: 1,
		StaticcheckVersion: "v0.8.1",
		Repositories: []corpus.Repository{
			{
				ID: "alpha",
				Repository: "https://github.com/example/alpha.git",
				Revision: strings.Repeat("a", 40),
				License: "Apache-2.0",
				LicensePath: "LICENSE",
				Roles: []string{"cli"},
				GoDirective: "1.26",
				SourceVersionPolicy: corpus.SourceVersionSupported,
				Patterns: []string{"./..."},
			},
		},
	}
	executor := &corpusExecutor{
		revision: manifest.Repositories[0].Revision,
		repository: manifest.Repositories[0].Repository,
		checkout: checkout,
	}
	return manifest, corpus.RunOptions{
		RunID: "source-aaaaaaaa-run-1",
		CheckoutRoot: checkoutRoot,
		OutputRoot: filepath.Join(root, "results"),
		CacheRoot: filepath.Join(root, "cache"),
		GlippyPath: "/tools/glippy",
		StaticcheckPath: "/tools/staticcheck",
		Executor: executor,
	}, executor, checkout
}

type corpusExecutor struct {
	revision string
	repository string
	status string
	statuses []string
	checkout string
	profiles []string
	vetRuns int
	staticcheckRuns int
	statusRuns int
	staticcheckVersion string
	statistics []string
	analysisError error
	preflightExitCode int
	commands []corpus.Command
	moduleResolutions []corpus.Command
	moduleDownloads []corpus.Command
	probeReadOnly bool
	snapshotWriteError error
	materializeWorkspaceSum bool
	createModuleGraphSourceFile bool
	updateWorkspaceSumDuringPreflight bool
	createPreflightSourceFile bool
	moduleGraphWriteError error
	moduleGraphSourceWriteError error
	preflightWorkspaceSumWriteError error
	preflightSourceWriteError error
	workspaceSumWriteAfterPreflightError error
	workspaceSumObserved bool
	includeTaskPath bool
}

func (e *corpusExecutor) Run(
	_ context.Context,
	command corpus.Command,
) (corpus.CommandResult, error) {
	e.commands = append(e.commands, command)
	switch {
	case command.Path == "git" && slices.Equal(command.Args, []string{"rev-parse", "HEAD"}):
		return corpus.CommandResult{Stdout: []byte(e.revision + "\n")}, nil
	case command.Path == "git" &&
		slices.Equal(
			command.Args,
			[]string{
				"status",
				"--porcelain=v1",
				"--untracked-files=all",
				"--ignored=matching",
			},
		):
		index := e.statusRuns
		e.statusRuns++
		if index < len(e.statuses) {
			return corpus.CommandResult{Stdout: []byte(e.statuses[index])}, nil
		}
		return corpus.CommandResult{Stdout: []byte(e.status)}, nil
	case command.Path == "git" &&
		slices.Equal(command.Args, []string{"config", "--get", "remote.origin.url"}):
		return corpus.CommandResult{Stdout: []byte(e.repository + "\n")}, nil
	case command.Path == "/tools/glippy" && slices.Equal(command.Args, []string{"version"}):
		return corpus.CommandResult{Stdout: []byte("glippy v0.6-dev\n")}, nil
	case command.Path == "/tools/staticcheck" &&
		slices.Equal(command.Args, []string{"-version"}):
		if e.staticcheckVersion != "" {
			return corpus.CommandResult{
				Stdout: []byte(e.staticcheckVersion + "\n"),
			}, nil
		}
		return corpus.CommandResult{Stdout: []byte("staticcheck 2026.1.1 (0.8.1)\n")}, nil
	case command.Path == "go" && slices.Equal(command.Args, []string{"version"}):
		return corpus.CommandResult{Stdout: []byte("go version go1.26.0 test/arch\n")}, nil
	case command.Path == "go" &&
		slices.Equal(command.Args, []string{"list", "-mod=readonly", "-m", "-json", "all"}):
		e.moduleResolutions = append(e.moduleResolutions, command)
		if environmentValue(command.Env, "GOWORK") != "off" {
			if e.materializeWorkspaceSum {
				e.moduleGraphWriteError = os.WriteFile(
					filepath.Join(command.Dir, "go.work.sum"),
					[]byte("example.com/dependency v1.0.0 h1:fixture\n"),
					0o600,
				)
				if e.moduleGraphWriteError != nil {
					return corpus.CommandResult{
						Stderr: []byte(e.moduleGraphWriteError.Error()),
						ExitCode: 1,
					}, nil
				}
			}
			if e.createModuleGraphSourceFile {
				e.moduleGraphSourceWriteError = os.WriteFile(
					filepath.Join(command.Dir, "mutation.txt"),
					[]byte("mutation\n"),
					0o600,
				)
			}
			return corpus.CommandResult{
				Stdout: []byte(
					"{\"Path\":\"example.com/main\",\"Main\":true}\n" +
						"{\"Path\":\"example.com/nested-dependency\",\"Version\":\"v1.0.0\"}\n" +
						"{\"Path\":\"example.com/root-dependency\",\"Version\":\"v1.0.0\"}\n",
				),
			}, nil
		}
		dependency := "example.com/root-dependency"
		if filepath.Base(command.Dir) == "nested" {
			dependency = "example.com/nested-dependency"
		}
		return corpus.CommandResult{
			Stdout: []byte(
				"{\"Path\":\"example.com/main\",\"Main\":true}\n" +
					fmt.Sprintf(
						"{\"Path\":%q,\"Version\":\"v1.0.0\"}\n",
						dependency,
					),
			),
		}, nil
	case command.Path == "go" &&
		len(command.Args) > 2 &&
		slices.Equal(command.Args[:2], []string{"mod", "download"}):
		e.moduleDownloads = append(e.moduleDownloads, command)
		return corpus.CommandResult{}, nil
	case command.Path == "/tools/glippy" && len(command.Args) > 5 && command.Args[0] == "lint":
		if e.materializeWorkspaceSum {
			_, err := os.Stat(filepath.Join(command.Dir, "go.work.sum"))
			e.workspaceSumObserved = err == nil
			if e.probeReadOnly && e.workspaceSumWriteAfterPreflightError == nil {
				workspaceSum, err := os.OpenFile(
					filepath.Join(command.Dir, "go.work.sum"),
					os.O_WRONLY,
					0,
				)
				if err == nil {
					err = workspaceSum.Close()
				}
				e.workspaceSumWriteAfterPreflightError = err
			}
		}
		if e.probeReadOnly && e.snapshotWriteError == nil {
			e.snapshotWriteError = os.WriteFile(
				filepath.Join(command.Dir, "mutation.txt"),
				[]byte("x"),
				0o600,
			)
		}
		if e.analysisError != nil {
			return corpus.CommandResult{}, e.analysisError
		}
		configPath := command.Args[4]
		configured, err := os.ReadFile(configPath)
		if err != nil {
			return corpus.CommandResult{}, err
		}
		profile := ""
		for _, candidate := range []string{"default", "recommended", "strict", "pedantic"} {
			if strings.Contains(string(configured), `profile = "` + candidate + `"`) {
				profile = candidate
				break
			}
		}
		if profile == "" {
			return corpus.CommandResult{}, fmt.Errorf(
				"config %q has no profile",
				configPath,
			)
		}
		e.profiles = append(e.profiles, profile)
		diagnostic := fmt.Sprintf(
			`{"schema_version":1,"command":"lint","mode":"check","outcome":{"category":"findings","exit_code":1},"summary":{"files":1,"diagnostics":1,"suppressed":0,"baselined":0,"baseline_problems":0,"suppression_problems":0,"unused_suppressions":0,"complete":true},"files":[],"diagnostics":[{"rule_id":"sample-rule","severity":"warn","message_key":"sample","message":"sample finding","path":%q,"source_digest":"digest","range":{"start":1,"end":2},"related":[],"notes":[],"help":"","fixes":[]}],"suppression_problems":[],"unused_suppressions":[],"baseline_problems":[],"errors":[]}`,
			filepath.Join(command.Dir, "sample.go"),
		)
		if e.includeTaskPath {
			diagnostic = strings.Replace(
				diagnostic,
				`"message":"sample finding"`,
				fmt.Sprintf(
					`"message":%q`,
					environmentValue(command.Env, "GOTMPDIR"),
				),
				1,
			)
		}
		statistics := `{"schema_version":1,"command":"lint","outcome":{"category":"findings","exit_code":1},"complete":true,"maximum_tier":"syntax","packages":1,"files":1,"loaded_files":1,"total":{"calls":1,"duration_ns":1,"allocations":1,"allocated_bytes":1},"phases":[],"tiers":[],"rules":[],"cache":{"lookups":0,"hits":0,"misses":0,"invalidations":0,"writes":0},"dependency_syntax":{"loaded":false,"reasons":[]},"effect_facts":{"loaded":false,"reasons":[]}}`
		if len(e.statistics) != 0 {
			statistics = e.statistics[0]
			e.statistics = e.statistics[1:]
		}
		return corpus.CommandResult{
			Stdout: []byte(diagnostic),
			Stderr: []byte(statistics),
			ExitCode: 1,
		}, nil
	case command.Path == "go" &&
		len(command.Args) >= 4 &&
		slices.Equal(command.Args[:4], []string{"list", "-deps", "-test", "-export"}):
		if e.updateWorkspaceSumDuringPreflight {
			workspaceSum, err := os.OpenFile(
				filepath.Join(command.Dir, "go.work.sum"),
				os.O_APPEND | os.O_WRONLY,
				0,
			)
			if err == nil {
				_, writeErr := workspaceSum.WriteString(
					"example.com/preflight v1.0.0 h1:fixture\n",
				)
				err = errors.Join(writeErr, workspaceSum.Close())
			}
			e.preflightWorkspaceSumWriteError = err
			if err != nil {
				return corpus.CommandResult{
					Stderr: []byte(err.Error()),
					ExitCode: 1,
				}, nil
			}
		}
		if e.createPreflightSourceFile {
			e.preflightSourceWriteError = os.WriteFile(
				filepath.Join(command.Dir, "preflight-mutation.txt"),
				[]byte("mutation\n"),
				0o600,
			)
		}
		return corpus.CommandResult{ExitCode: e.preflightExitCode}, nil
	case command.Path == "go" && len(command.Args) > 0 && command.Args[0] == "vet":
		e.vetRuns++
		if e.includeTaskPath {
			return corpus.CommandResult{
				Stdout: []byte(environmentValue(command.Env, "GOTMPDIR")),
			}, nil
		}
		return corpus.CommandResult{}, nil
	case command.Path == "/tools/staticcheck" && len(command.Args) > 0:
		e.staticcheckRuns++
		return corpus.CommandResult{
			Stdout: []byte(filepath.Join(command.Dir, "sample.go") + ":1:1: sample\n"),
			ExitCode: 1,
		}, nil
	default:
		return corpus.CommandResult{}, fmt.Errorf(
			"unexpected command: %s %v",
			command.Path,
			command.Args,
		)
	}
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func pathAtOrWithinForTest(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".." + string(filepath.Separator))
}

func corpusEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if found &&
			(strings.HasPrefix(name, "GIT_") ||
				strings.HasPrefix(name, "GO") ||
				strings.HasPrefix(name, "XDG_") ||
				name == "CGO_ENABLED" ||
				name == "TMPDIR") {
			result = append(result, entry)
		}
	}
	return result
}
