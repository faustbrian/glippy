package goversion_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/goversion"
)

func TestResolveUsesNearestModuleBeforeWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module := filepath.Join(root, "module")
	path := filepath.Join(module, "nested", "source.go")
	writeFile(t, filepath.Join(root, "go.work"), "go 1.26.5\n")
	writeFile(t, filepath.Join(module, "go.mod"), "module example.com/module\n\ngo 1.25.4\n")

	selection, err := goversion.Resolve(path, root)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Language != "go1.25" || selection.Path != filepath.Join(module, "go.mod") {
		t.Fatalf("Resolve() = %#v, want nearest module Go 1.25", selection)
	}
}

func TestResolveUsesWorkspaceWhenNoModuleOwnsPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "tools", "source.go")
	work := filepath.Join(root, "go.work")
	writeFile(t, work, "go 1.26.5\n")

	selection, err := goversion.Resolve(path, root)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Language != "go1.26" || selection.Path != work {
		t.Fatalf("Resolve() = %#v, want workspace Go 1.26", selection)
	}
}

func TestResolveDefaultsWithoutDirective(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")

	selection, err := goversion.Resolve(filepath.Join(root, "source.go"), root)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Language != goversion.Default || selection.Path != "" {
		t.Fatalf(
			"Resolve() = %#v, want default %q without a directive",
			selection,
			goversion.Default,
		)
	}
}

func TestResolveRejectsUnsupportedSourceVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mod := filepath.Join(root, "go.mod")
	writeFile(t, mod, "module example.com/project\n\ngo 1.24\n")

	_, err := goversion.Resolve(filepath.Join(root, "source.go"), root)
	if err == nil ||
		!strings.Contains(err.Error(), "supports Go 1.25 through Go 1.26") ||
		!strings.Contains(err.Error(), mod) {
		t.Fatalf("Resolve() error = %v, want located unsupported-version error", err)
	}
}

func TestResolveRejectsMalformedVersionFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mod := filepath.Join(root, "go.mod")
	writeFile(t, mod, "module example.com/project\n\ngo nope\n")

	_, err := goversion.Resolve(filepath.Join(root, "source.go"), root)
	if err == nil || !strings.Contains(err.Error(), mod) {
		t.Fatalf("Resolve() error = %v, want located parse error", err)
	}
}

func TestResolveClassifiesVersionFileReadFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mod := filepath.Join(root, "go.mod")
	if err := os.Mkdir(mod, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := goversion.Resolve(filepath.Join(root, "source.go"), root)
	if err == nil || !goversion.IsFilesystemError(err) {
		t.Fatalf("Resolve() error = %v, want filesystem classification", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
