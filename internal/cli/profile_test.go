package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/config"
)

func TestRunInitCreatesSelectedProfileConfiguration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"init", "--profile=pedantic", root},
		failingReader{},
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(init profile) = exit %d, stderr %q", exitCode, stderr.String())
	}
	contents, err := os.ReadFile(filepath.Join(root, config.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "[lint]\nprofile = \"pedantic\"\n") {
		t.Fatalf("initialized configuration = %q", contents)
	}
}

func TestRunInitRejectsUnknownProfile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"init", "--profile", "maximum", root},
		failingReader{},
		&stdout,
		&stderr,
	)
	if exitCode != ExitInvalidInvocation ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "unknown lint profile \"maximum\"") {
		t.Fatalf(
			"Run(init unknown profile) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	if _, err := os.Stat(filepath.Join(root, config.Filename)); !os.IsNotExist(err) {
		t.Fatalf("Run(init unknown profile) created configuration: %v", err)
	}
}

func TestRunConfigShowExplainsProfileAndExplicitPrecedence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/profile\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configuration := `version = 1
[lint]
profile = "recommended"

[lint.rules]
identical-branches = "error"
`
	if err := os.WriteFile(filepath.Join(root, config.Filename), []byte(configuration), 0o600);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"config", "show", root}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf("Run(config show profile) = exit %d, stderr %q", exitCode, stderr.String())
	}
	for _, expected := range
		[]string{
			"profile: recommended",
			"presets: correctness",
			"rule almost-swapped: warn (profile recommended)",
			"rule identical-branches: error (explicit override)",
		} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("config show missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunConfigCheckAcceptsEveryProfile(t *testing.T) {
	t.Parallel()

	for _, profile := range config.Profiles() {
		profile := profile
		t.Run(
			string(profile),
			func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				if err := os.WriteFile(
					filepath.Join(root, "go.mod"),
					[]byte("module example.com/profilecheck\n\ngo 1.26.0\n"),
					0o600,
				);
					err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(root, config.Filename),
					[]byte(
						"version = 1\n[lint]\nprofile = \"" +
							string(profile) +
							"\"\n",
					),
					0o600,
				);
					err != nil {
					t.Fatal(err)
				}
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				exitCode := Run(
					[]string{"config", "check", root},
					failingReader{},
					&stdout,
					&stderr,
				)
				if exitCode != ExitSuccess ||
					stderr.Len() != 0 ||
					!strings.Contains(stdout.String(), "configuration valid:") {
					t.Fatalf(
						"Run(config check %s) = exit %d, stdout %q, stderr %q",
						profile,
						exitCode,
						stdout.String(),
						stderr.String(),
					)
				}
			},
		)
	}
}

func TestRunLintAppliesRecommendedProfileAndExplicitDisable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/profilelint\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		sourcePath,
		[]byte(
			"package sample\nfunc run(v bool) { if v { println(1) } else { println(1) } }\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(root, config.Filename)
	if err := os.WriteFile(
		configurationPath,
		[]byte("version = 1\n[lint]\nprofile = \"recommended\"\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"lint", sourcePath}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitFindings ||
		stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "identical-branches") {
		t.Fatalf(
			"Run(lint recommended) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	if err := os.WriteFile(
		configurationPath,
		[]byte(
			"version = 1\n[lint]\nprofile = \"recommended\"\n" +
				"[lint.rules]\nidentical-branches = \"off\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"lint", sourcePath}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint disabled profile rule) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	if err := os.WriteFile(
		configurationPath,
		[]byte(
			"version = 1\n[lint]\nprofile = \"recommended\"\n" +
				"[[lint.overrides]]\npaths = [\"sample.go\"]\n" +
				"[lint.overrides.rules]\nidentical-branches = \"off\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exitCode = Run([]string{"lint", sourcePath}, failingReader{}, &stdout, &stderr)
	if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint path-disabled profile rule) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}
