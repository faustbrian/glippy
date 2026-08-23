package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
)

func TestAutomaticPackageModuleModeSelectsCompatibleModuleVendorManifest(t *testing.T) {
	root := t.TempDir()
	writePackageModuleModeFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writePackageModuleModeFile(
		t,
		filepath.Join(root, "vendor", "modules.txt"),
		"# example.com/dependency v1.0.0\n",
	)

	mode, err := automaticPackageModuleMode(root, []string{"GOWORK=off"})
	if err != nil {
		t.Fatal(err)
	}
	if mode != analysis.ModuleVendor {
		t.Fatalf("automaticPackageModuleMode() = %q, want vendor", mode)
	}
}

func TestAutomaticPackageModuleModeKeepsReadonlyWithoutCompatibleManifest(t *testing.T) {
	for _, test := range
		[]struct {
			name string
			goMod string
			manifest string
		}{
			{
				name: "absent manifest",
				goMod: "module example.com/project\n\ngo 1.26.0\n",
			},
			{
				name: "missing go directive",
				goMod: "module example.com/project\n",
				manifest: "# example.com/dependency v1.0.0\n",
			},
			{
				name: "workspace manifest at module root",
				goMod: "module example.com/project\n\ngo 1.26.0\n",
				manifest: "## workspace\n# example.com/dependency v1.0.0\n",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				writePackageModuleModeFile(
					t,
					filepath.Join(root, "go.mod"),
					test.goMod,
				)
				if test.manifest != "" {
					writePackageModuleModeFile(
						t,
						filepath.Join(root, "vendor", "modules.txt"),
						test.manifest,
					)
				}
				mode, err := automaticPackageModuleMode(
					root,
					[]string{"GOWORK=off"},
				)
				if err != nil {
					t.Fatal(err)
				}
				if mode != analysis.ModuleReadonly {
					t.Fatalf(
						"automaticPackageModuleMode() = %q, want readonly",
						mode,
					)
				}
			},
		)
	}
}

func TestAutomaticPackageModuleModeSelectsWorkspaceVendorManifest(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "module")
	workspace := filepath.Join(root, "go.work")
	writePackageModuleModeFile(t, workspace, "go 1.26.0\n\nuse ./module\n")
	writePackageModuleModeFile(
		t,
		filepath.Join(module, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writePackageModuleModeFile(
		t,
		filepath.Join(root, "vendor", "modules.txt"),
		"## workspace\n# example.com/dependency v1.0.0\n",
	)

	for _, environment := range [][]string{nil, {"GOWORK=" + workspace}} {
		mode, err := automaticPackageModuleMode(module, environment)
		if err != nil {
			t.Fatal(err)
		}
		if mode != analysis.ModuleVendor {
			t.Fatalf(
				"automaticPackageModuleMode(%q) = %q, want vendor",
				environment,
				mode,
			)
		}
	}
}

func TestAutomaticPackageModuleModeRejectsMismatchedWorkspaceManifest(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "module")
	workspace := filepath.Join(root, "go.work")
	writePackageModuleModeFile(t, workspace, "go 1.26.0\n\nuse ./module\n")
	writePackageModuleModeFile(
		t,
		filepath.Join(module, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writePackageModuleModeFile(
		t,
		filepath.Join(root, "vendor", "modules.txt"),
		"# example.com/dependency v1.0.0\n",
	)

	mode, err := automaticPackageModuleMode(module, []string{"GOWORK=" + workspace})
	if err != nil {
		t.Fatal(err)
	}
	if mode != analysis.ModuleReadonly {
		t.Fatalf("automaticPackageModuleMode() = %q, want readonly", mode)
	}
}

func TestAutomaticPackageModuleModeKeepsReadonlyForUnreadableManifest(t *testing.T) {
	root := t.TempDir()
	writePackageModuleModeFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	if err := os.MkdirAll(filepath.Join(root, "vendor", "modules.txt"), 0o700); err != nil {
		t.Fatal(err)
	}

	mode, err := automaticPackageModuleMode(root, []string{"GOWORK=off"})
	if err != nil {
		t.Fatalf("automaticPackageModuleMode() error = %v, want readonly fallback", err)
	}
	if mode != analysis.ModuleReadonly {
		t.Fatalf("automaticPackageModuleMode() = %q, want readonly", mode)
	}
}

func TestAutomaticPackageModuleModeReadsSymlinkedManifestWithoutWriting(t *testing.T) {
	root := t.TempDir()
	writePackageModuleModeFile(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	manifest := filepath.Join(t.TempDir(), "modules.txt")
	writePackageModuleModeFile(t, manifest, "# example.com/dependency v1.0.0\n")
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(manifest, filepath.Join(root, "vendor", "modules.txt")); err != nil {
		t.Skipf("create manifest symlink: %v", err)
	}

	mode, err := automaticPackageModuleMode(root, []string{"GOWORK=off"})
	if err != nil {
		t.Fatal(err)
	}
	if mode != analysis.ModuleVendor {
		t.Fatalf("automaticPackageModuleMode() = %q, want vendor", mode)
	}
	contents, err := os.ReadFile(manifest)
	if err != nil || string(contents) != "# example.com/dependency v1.0.0\n" {
		t.Fatalf("manifest changed to %q, error = %v", contents, err)
	}
}

func writePackageModuleModeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
