package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestRunInitCreatesConfigurationExclusively(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"init", root}, failingReader{}, &stdout, &stderr)

	configurationPath := filepath.Join(root, config.Filename)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(init) = exit %d, stderr %q", exitCode, stderr.String())
	}
	if stdout.String() != "glippy init: created " + configurationPath + "\n" {
		t.Fatalf("Run(init) stdout = %q", stdout.String())
	}
	contents, err := os.ReadFile(configurationPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "version = 1\n\n[format]\nline-width = 100\ntab-width = 8\n\n" +
		"[lint]\npresets = [\"correctness\"]\nwarnings-as-errors = false\n"
	if string(contents) != want {
		t.Fatalf("initialized configuration = %q, want %q", contents, want)
	}
	info, err := os.Stat(configurationPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("initialized configuration mode = %o, want 600", info.Mode().Perm())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"init", root}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitConflict ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "already exists") {
		t.Fatalf(
			"second Run(init) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	unchanged, err := os.ReadFile(configurationPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, contents) {
		t.Fatal("second Run(init) changed existing configuration")
	}
}

func TestRunInitRefusesSymlinkConfigurationTarget(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires separate Windows privileges")
	}

	root := t.TempDir()
	target := filepath.Join(root, "policy.toml")
	if err := os.WriteFile(target, []byte("owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, config.Filename)
	if err := os.Symlink(target, configurationPath); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"init", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitConflict ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "already exists") {
		t.Fatalf(
			"Run(init symlink) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "owned\n" {
		t.Fatalf("Run(init symlink) changed target to %q", contents)
	}
}

func TestRunInitDisclosesCreatedPathWhenStatusOutputFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configurationPath := filepath.Join(root, config.Filename)
	var stderr bytes.Buffer

	exitCode := Run([]string{"init", root}, failingReader{}, failingWriter{}, &stderr)

	if exitCode != ExitFilesystemError ||
		!strings.Contains(stderr.String(), configurationPath) ||
		!strings.Contains(stderr.String(), "write standard output") {
		t.Fatalf("Run(init output failure) = exit %d, stderr %q", exitCode, stderr.String())
	}
	if _, err := os.Stat(configurationPath); err != nil {
		t.Fatalf("Run(init output failure) did not retain created configuration: %v", err)
	}
}

func TestRunConfigCheckDiscoversAndValidatesConfiguration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, config.Filename)
	if err := os.WriteFile(configurationPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"config", "check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess ||
		stderr.Len() != 0 ||
		stdout.String() != "configuration valid: " + configurationPath + " (discovered)\n" {
		t.Fatalf(
			"Run(config check) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunConfigCheckSupportsExactConfigurationSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(t.TempDir(), "policy.toml")
	if err := os.WriteFile(configurationPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		[]string{"config", "check", "--config", configurationPath, root},
		failingReader{},
		&stdout,
		&stderr,
	)

	if exitCode != ExitSuccess ||
		stderr.Len() != 0 ||
		stdout.String() != "configuration valid: " + configurationPath + " (explicit)\n" {
		t.Fatalf(
			"Run(config check explicit) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunConfigCheckReportsInvalidConfigurationWithoutSuccessOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, config.Filename)
	if err := os.WriteFile(configurationPath, []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"config", "check", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitInvalidInvocation ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "unsupported configuration version 2") {
		t.Fatalf(
			"Run(config check invalid) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunConfigCheckRejectsIncompleteEnabledRulePolicy(t *testing.T) {
	t.Parallel()

	registry := requiredPolicyRegistry(t, "1.25")
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, config.Filename),
		[]byte("version = 1\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"check", "show"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := runConfig(
			context.Background(),
			[]string{"config", action, root},
			&stdout,
			&stderr,
			registry,
		)

		if exitCode != ExitInvalidInvocation ||
			stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), `missing required option "policy"`) {
			t.Fatalf(
				"runConfig(%s incomplete policy) = exit %d, stdout %q, stderr %q",
				action,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunConfigCheckValidatesRulesEnabledOnlyByPathPolicy(t *testing.T) {
	t.Parallel()

	registry := requiredPolicyRegistry(t, "1.25")
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, config.Filename),
		[]byte(
			`version = 1
[lint]
presets = []

[[lint.overrides]]
paths = ["**/*_test.go"]

[lint.overrides.rules]
required-policy = "warn"
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runConfig(
		context.Background(),
		[]string{"config", "check", root},
		&stdout,
		&stderr,
		registry,
	)
	if exitCode != ExitInvalidInvocation ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), `missing required option "policy"`) {
		t.Fatalf(
			"runConfig(path-only required policy) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func requiredPolicyRegistry(t *testing.T, minimumGoVersion string) *rules.Registry {
	t.Helper()
	registry, err := rules.NewRegistry(
		explainMetadataRule{
			metadata: rules.Metadata{
				ID: "required-policy",
				Summary: "requires explicit policy",
				Documentation: "Reports a configured project policy.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: minimumGoVersion,
				Requirement: rules.RequireSyntax,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Options: []rules.OptionMetadata{
					{
						Name: "policy",
						Summary: "required project policy",
						Kind: rules.OptionString,
						Required: true,
					},
				},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestRunConfigCheckIgnoresRequiredPolicyForUnavailableRule(t *testing.T) {
	t.Parallel()

	registry := requiredPolicyRegistry(t, "1.26")
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, config.Filename),
		[]byte("version = 1\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runConfig(
		context.Background(),
		[]string{"config", "check", root},
		&stdout,
		&stderr,
		registry,
	)

	if exitCode != ExitSuccess || stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf(
			"runConfig(unavailable policy) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunConfigShowExplainsEffectivePolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modulePath := filepath.Join(root, "go.mod")
	if err := os.WriteFile(
		modulePath,
		[]byte("module example.com/project\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, config.Filename)
	configuration := `version = 1

[format]
line-width = 88
tab-width = 4

[analysis]
build-tags = ["integration", "linux"]
goos = "linux"
goarch = "arm64"
cgo-enabled = false

[[analysis.targets]]
goos = "linux"
goarch = "amd64"
tags = ["linux", "integration"]

[[analysis.targets]]
goos = "darwin"
goarch = "arm64"
cgo-enabled = true

[lint]
presets = ["correctness"]
warnings-as-errors = true

[lint.rules]
duplicate-condition = "off"
nilness = "error"

[lint.suppressions]
require-reason = true
expiry-cutoff = "2027-01-01"

[lint.baseline]
path = ".glippy-baseline.json"
report-stale = false
expiry-cutoff = "2027-02-01"

[cache]
enabled = true
max-entries = 12
max-bytes = 4096
`
	if err := os.WriteFile(configurationPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".glippy-baseline.json"), []byte("{}\n"), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"config", "show", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(config show) = exit %d, stderr %q", exitCode, stderr.String())
	}
	output := stdout.String()
	for _, expected := range
		[]string{
			"project root: " + root,
			"configuration: " + configurationPath + " (discovered)",
			"source language: go1.25 (" + modulePath + ")",
			"migration target: unset",
			"format: line-width=88 tab-width=4",
			"presets: correctness",
			"warnings-as-errors: true",
			"rule nilness: error (explicit override)",
			"  option include-tests=false",
			"maximum analysis tier: SSA",
			"generated files: readable; writes refused; enabled rules eligible=",
			"type errors: enabled rules eligible=",
			"test files: included through package selection",
			"testdata and fixtures: excluded unless explicitly selected",
			"vendor: excluded from recursive discovery",
			"analysis: goos=linux goarch=arm64 cgo=false build-tags=integration,linux",
			"analysis targets: 2",
			"  target darwin/arm64+cgo",
			"  target linux/amd64+tags=integration,linux",
			"baseline: .glippy-baseline.json (present; report-stale=false; expiry-cutoff=2027-02-01)",
			"suppressions: require-reason=true expiry-cutoff=2027-01-01",
			"cache: enabled=true max-entries=12 max-bytes=4096",
		} {
		if !strings.Contains(output, expected) {
			t.Fatalf("Run(config show) output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "rule duplicate-condition:") {
		t.Fatalf("Run(config show) listed disabled rule:\n%s", output)
	}
}

func TestRunConfigShowReportsBuiltInDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/project\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"config", "show", root}, failingReader{}, &stdout, &stderr)

	if exitCode != ExitSuccess ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "configuration: built-in defaults\n") ||
		!strings.Contains(stdout.String(), "presets: correctness\n") {
		t.Fatalf(
			"Run(config show defaults) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunConfigShowExplainsPathScopedRulePolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/pathpolicy\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "internal", "client_test.go")
	if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package internal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, config.Filename),
		[]byte(
			`version = 1
[lint]
presets = []

[[lint.overrides]]
paths = ["**/*_test.go"]

[lint.overrides.rules]
duplicate-condition = "error"
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"config", "show", path}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"Run(config show path policy) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	for _, expected := range
		[]string{
			"path overrides: configured=1 matched=1 path=internal/client_test.go",
			"rule duplicate-condition: error (path override 1)",
		} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("config show missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunConfigurationCommandsRejectInvalidInvocationAndObserveCancellation(t *testing.T) {
	t.Parallel()

	for _, arguments := range
		[][]string{
			{"init", "one", "two"},
			{"config"},
			{"config", "unknown"},
			{"config", "check", "one", "two"},
			{"config", "show", "--config"},
		} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
		if exitCode != ExitInvalidInvocation || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf(
				"Run(%q) = exit %d, stdout %q, stderr %q",
				arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunContext(
		canceled,
		[]string{"init", t.TempDir()},
		failingReader{},
		&stdout,
		&stderr,
	)
	if exitCode != ExitCanceled ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf(
			"RunContext(canceled init) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}
