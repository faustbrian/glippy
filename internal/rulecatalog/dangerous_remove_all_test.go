package rulecatalog_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestDangerousRemoveAllReportsProvenSystemDirectoryOrigins(t *testing.T) {
	t.Parallel()

	input := `package sample

import system "os"

var startupCleanup = system.RemoveAll(system.TempDir())

func direct() error {
	return system.RemoveAll(system.TempDir())
}

func variables() {
	temporary := system.TempDir()
	_ = system.RemoveAll(temporary)
	remove := system.RemoveAll
	tempDir := system.TempDir
	_ = remove(tempDir())
	cache, _ := system.UserCacheDir()
	_ = system.RemoveAll(cache)
	config, _ := system.UserConfigDir()
	defer system.RemoveAll(config)
	config = "owned-child"
	home, _ := system.UserHomeDir()
	go system.RemoveAll(home)
}

func sameValueAcrossBranches(flag bool) {
	directory := system.TempDir()
	selected := directory
	if flag {
		selected = directory
	}
	_ = system.RemoveAll(selected)
}
`
	result := runDangerousRemoveAll(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 8 {
		t.Fatalf("dangerous-remove-all result = %#v", result)
	}
	wants := []struct {
		call string
		origin string
		kind string
		provider string
	}{
		{"system.RemoveAll(system.TempDir())", "system.TempDir()", "temporary", "TempDir"},
		{"system.RemoveAll(system.TempDir())", "system.TempDir()", "temporary", "TempDir"},
		{"system.RemoveAll(temporary)", "system.TempDir()", "temporary", "TempDir"},
		{"remove(tempDir())", "tempDir()", "temporary", "TempDir"},
		{"system.RemoveAll(cache)", "system.UserCacheDir()", "cache", "UserCacheDir"},
		{"system.RemoveAll(config)", "system.UserConfigDir()", "config", "UserConfigDir"},
		{"system.RemoveAll(home)", "system.UserHomeDir()", "home", "UserHomeDir"},
		{"system.RemoveAll(selected)", "system.TempDir()", "temporary", "TempDir"},
	}
	callSearch := 0
	originSearch := 0
	for index, want := range wants {
		callOffset := strings.Index(input[callSearch:], want.call)
		if callOffset < 0 {
			t.Fatalf("missing call fixture %q", want.call)
		}
		callStart := callSearch + callOffset
		originOffset := strings.Index(input[originSearch:], want.origin)
		if originOffset < 0 {
			t.Fatalf("missing origin fixture %q", want.origin)
		}
		originStart := originSearch + originOffset
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "dangerous-remove-all" ||
			diagnostic.Severity != rules.SeverityWarn ||
			diagnostic.MessageKey != "dangerous-remove-all-" + want.kind ||
			diagnostic.Message !=
				"os.RemoveAll deletes the complete " + want.kind + " directory" ||
			diagnostic.Range.Start != callStart ||
			diagnostic.Range.End != callStart + len(want.call) ||
			len(diagnostic.Related) != 1 ||
			diagnostic.Related[0].Range.Start != originStart ||
			diagnostic.Related[0].Range.End != originStart + len(want.origin) ||
			diagnostic.Related[0].Message !=
				"os." +
					want.provider +
					" returns this " +
					want.kind +
					" directory" ||
			diagnostic.Help !=
				"create or append an application-owned child directory before deleting it" ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("dangerous-remove-all diagnostic[%d] = %#v", index, diagnostic)
		}
		callSearch = callStart + len(want.call)
		originSearch = originStart + len(want.origin)
	}
}

func TestDangerousRemoveAllExcludesUnprovenOrNarrowedPaths(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"os"
	"path/filepath"
)

type filesystem struct{}

func (filesystem) TempDir() string { return "/tmp" }
func (filesystem) RemoveAll(string) error { return nil }
func helper() string { return os.TempDir() }

func valid(
	fake filesystem,
	pointer *string,
	remove func(string) error,
	temporary func() string,
	flag bool,
) {
	_ = os.RemoveAll(filepath.Join(os.TempDir(), "child"))
	_ = os.RemoveAll(os.TempDir() + "/child")
	_ = os.RemoveAll(helper())
	_ = os.RemoveAll(*pointer)
	_ = os.RemoveAll("/tmp")
	_ = fake.RemoveAll(fake.TempDir())
	remove(os.TempDir())
	_ = os.RemoveAll(temporary())
	maybeTemporary := os.TempDir()
	if flag {
		maybeTemporary = "owned-child"
	}
	_ = os.RemoveAll(maybeTemporary)
}
`
	result := runDangerousRemoveAll(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("valid dangerous-remove-all result = %#v", result)
	}
}

func TestDangerousRemoveAllMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("dangerous-remove-all")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSSA ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!reflect.DeepEqual(
			metadata.Categories,
			[]rules.Category{rules.CategorySafety, rules.CategorySuspicious},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("dangerous-remove-all metadata = %#v, found = %v", metadata, found)
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"dangerous-remove-all": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil || len(selection) != 0 {
		t.Fatalf("pre-minimum dangerous-remove-all selection = %#v, %v", selection, err)
	}
}

func TestDangerousRemoveAllHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/dangerousremoveallpolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import "os"

func suppressed() {
	//glippy:ignore dangerous-remove-all -- isolated destructive-system test fixture
	_ = os.RemoveAll(os.TempDir())
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import "os"

func generated() { _ = os.RemoveAll(os.TempDir()) }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import "os"

func invalid() {
	var broken string = 1
	_ = broken
	_ = os.RemoveAll(os.TempDir())
}
`,
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"dangerous-remove-all": rules.SeverityError,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 || len(result.LoadDiagnostics) == 0 {
		t.Fatalf("dangerous-remove-all policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "dangerous-remove-all" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed dangerous-remove-all result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded dangerous-remove-all result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
}

func runDangerousRemoveAll(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/dangerousremoveall\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"dangerous-remove-all": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || !strings.HasSuffix(result.Files[0].Path, "sample.go") {
		t.Fatalf("dangerous-remove-all result = %#v", result)
	}
	return result
}
