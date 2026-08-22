package corpus_test

import (
	"bytes"
	"context"
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
		"GOWORK=/users/real/go.work",
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
				"GOWORK=/users/real/go.work",
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
				"GOFLAGS=",
				"GOWORK=off",
				"GOCACHEPROG=",
				"GONOPROXY=none",
				"GOVCS=*:off",
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
	commands []corpus.Command
	probeReadOnly bool
	snapshotWriteError error
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
	case command.Path == "/tools/glippy" && len(command.Args) > 5 && command.Args[0] == "lint":
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
		statistics := `{"schema_version":1,"command":"lint","outcome":{"category":"findings","exit_code":1},"complete":true,"duration_ns":1}`
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
		return corpus.CommandResult{}, nil
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
