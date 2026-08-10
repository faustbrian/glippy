package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faustbrian/gox/internal/config"
)

func TestDiscoverSelectsConfigurationAtNearestProjectBoundary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"))
	configurationPath := filepath.Join(root, config.Filename)
	writeTestFile(t, configurationPath)
	inputPath := filepath.Join(root, "pkg", "source.go")
	writeTestFile(t, inputPath)

	selection, err := config.Discover(inputPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Root != root || selection.Path != configurationPath || selection.Explicit {
		t.Fatalf("Discover() = %#v, want root and configuration at %q", selection, root)
	}
}

func TestDiscoverSearchesForConfigurationOnlyAtOrAboveBoundary(t *testing.T) {
	t.Parallel()

	repositoryRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repositoryRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	repositoryConfiguration := filepath.Join(repositoryRoot, config.Filename)
	writeTestFile(t, repositoryConfiguration)
	moduleRoot := filepath.Join(repositoryRoot, "module")
	writeTestFile(t, filepath.Join(moduleRoot, "go.mod"))
	inputPath := filepath.Join(moduleRoot, "nested", "source.go")
	writeTestFile(t, inputPath)
	writeTestFile(t, filepath.Join(filepath.Dir(inputPath), config.Filename))

	selection, err := config.Discover(inputPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Root != moduleRoot || selection.Path != repositoryConfiguration {
		t.Fatalf("Discover() = %#v, want module root with repository configuration", selection)
	}
}

func TestDiscoverUsesExplicitConfigurationWithoutAutomaticSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"))
	writeTestFile(t, filepath.Join(root, config.Filename))
	inputPath := filepath.Join(root, "pkg", "source.go")
	writeTestFile(t, inputPath)
	explicitPath := filepath.Join(root, "policy", "selected.toml")
	writeTestFile(t, explicitPath)

	selection, err := config.Discover(inputPath, explicitPath)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Root != root || selection.Path != explicitPath || !selection.Explicit {
		t.Fatalf("Discover() = %#v, want exact explicit configuration", selection)
	}
}

func TestDiscoverUsesExplicitConfigurationWithoutProjectBoundary(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	inputPath := filepath.Join(directory, "source.go")
	writeTestFile(t, inputPath)
	explicitPath := filepath.Join(directory, "selected.toml")
	writeTestFile(t, explicitPath)

	selection, err := config.Discover(inputPath, explicitPath)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Root != "" || selection.Path != explicitPath || !selection.Explicit {
		t.Fatalf("Discover() = %#v, want boundary-free explicit configuration", selection)
	}
}

func TestDiscoverDoesNotSelectConfigurationWithoutProjectBoundary(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, config.Filename))
	inputPath := filepath.Join(directory, "nested", "source.go")
	writeTestFile(t, inputPath)

	selection, err := config.Discover(inputPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if selection != (config.Selection{}) {
		t.Fatalf("Discover() = %#v, want no project selection", selection)
	}
}

func TestDiscoverRecognizesModuleWorkspaceAndRepositoryBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		marker    string
		directory bool
	}{
		{name: "module", marker: "go.mod"},
		{name: "workspace", marker: "go.work"},
		{name: "repository directory", marker: ".git", directory: true},
		{name: "worktree file", marker: ".git"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			markerPath := filepath.Join(root, test.marker)
			if test.directory {
				if err := os.Mkdir(markerPath, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				writeTestFile(t, markerPath)
			}
			configurationPath := filepath.Join(root, config.Filename)
			writeTestFile(t, configurationPath)
			inputPath := filepath.Join(root, "nested", "source.go")
			writeTestFile(t, inputPath)

			selection, err := config.Discover(inputPath, "")
			if err != nil {
				t.Fatal(err)
			}
			if selection.Root != root || selection.Path != configurationPath {
				t.Fatalf("Discover() = %#v, want boundary %q", selection, root)
			}
		})
	}
}

func TestDiscoverRejectsUnusableInputAndConfigurationPaths(t *testing.T) {
	t.Parallel()

	t.Run("missing input", func(t *testing.T) {
		t.Parallel()
		if _, err := config.Discover(filepath.Join(t.TempDir(), "missing.go"), ""); err == nil {
			t.Fatal("Discover() error = nil, want missing-input failure")
		}
	})

	t.Run("missing explicit configuration", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "go.mod"))
		inputPath := filepath.Join(root, "source.go")
		writeTestFile(t, inputPath)
		if _, err := config.Discover(inputPath, filepath.Join(root, "missing.toml")); err == nil {
			t.Fatal("Discover() error = nil, want missing-configuration failure")
		}
	})

	t.Run("configuration directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "go.mod"))
		if err := os.Mkdir(filepath.Join(root, config.Filename), 0o755); err != nil {
			t.Fatal(err)
		}
		inputPath := filepath.Join(root, "source.go")
		writeTestFile(t, inputPath)
		if _, err := config.Discover(inputPath, ""); err == nil {
			t.Fatal("Discover() error = nil, want non-file configuration failure")
		}
	})
}

func TestDiscoverFileContextUsesNonexistentEditorPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"))
	configurationPath := filepath.Join(root, config.Filename)
	writeTestFile(t, configurationPath)
	inputPath := filepath.Join(root, "new", "source.go")

	selection, err := config.DiscoverFileContext(inputPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Root != root || selection.Path != configurationPath {
		t.Fatalf("DiscoverFileContext() = %#v, want editor path context", selection)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
