package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestNonOctalFileModeReportsExactStandardModeArguments(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io/fs"
	operating "os"
)

type fileMode = fs.FileMode

func consume(fileMode) {}

func invalid(path string) {
	_ = operating.Mkdir(path, 755)
	_ = operating.MkdirAll(path, 700)
	_, _ = operating.OpenFile(path, operating.O_CREATE, 644)
	_ = operating.Chmod(path, 600)
	_ = operating.WriteFile(path, nil, 666)
	consume(777)
	consume(operating.FileMode(711))
}
`
	result := runNonOctalFileMode(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 7 {
		t.Fatalf("non-octal-file-mode result = %#v", result)
	}
	wantText := []string{"755", "700", "644", "600", "666", "777", "711"}
	wantEvaluated := []string{
		"0o1363",
		"0o1274",
		"0o1204",
		"0o1130",
		"0o1232",
		"0o1411",
		"0o1307",
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], wantText[index])
		if relative < 0 {
			t.Fatalf("missing literal %q", wantText[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "non-octal-file-mode" ||
			diagnostic.MessageKey != "decimal-file-mode" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(wantText[index]) ||
			!strings.Contains(diagnostic.Message, wantEvaluated[index]) ||
			len(diagnostic.Fixes) != 1 ||
			diagnostic.Fixes[0].Name != "use-octal-file-mode" ||
			diagnostic.Fixes[0].Safety != rules.FixSuggestion ||
			!reflect.DeepEqual(
				diagnostic.Fixes[0].Edits,
				[]rules.Edit{
					{Range: diagnostic.Range, NewText: "0o" + wantText[index]},
				},
			) {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(wantText[index])
	}
}

func TestNonOctalFileModeExcludesDeliberateAndUnprovenModes(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io/fs"
	"os"
)

type localMode fs.FileMode

const decimalConstant fs.FileMode = 644

func consumeLocal(localMode) {}

func inspect(path string, dynamic fs.FileMode) {
	_ = os.Chmod(path, 0o644)
	_ = os.Chmod(path, 0644)
	_ = os.Chmod(path, 0x1a4)
	_ = os.Chmod(path, 0b110100100)
	_ = os.Chmod(path, 6_44)
	_ = os.Chmod(path, 888)
	_ = os.Chmod(path, dynamic)
	_ = os.Chmod(path, decimalConstant)
	consumeLocal(644)
}
`
	result := runNonOctalFileMode(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conservative non-octal-file-mode result = %#v", result)
	}
}

func TestNonOctalFileModeSuggestionRequiresOptInAndIsIdempotent(t *testing.T) {
	t.Parallel()

	input := "package sample\nimport \"os\"\nfunc run(){_=os.Chmod(\"file\",644)}\n"
	result := runNonOctalFileMode(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("non-octal-file-mode suggestion result = %#v", result)
	}
	path := result.Files[0].Path
	file, found := result.Sources.Lookup(path)
	if !found {
		t.Fatal("non-octal-file-mode source is missing")
	}
	selection := fixengine.Selection{
		Diagnostic: result.Files[0].Diagnostics[0],
		FixName: "use-octal-file-mode",
	}
	options := fixengine.Options{
		Format: glippyformat.Options{Width: 100, TabWidth: 8, FitBudget: 10_000},
	}
	rejected, err := fixengine.Coordinate(file, []fixengine.Selection{selection}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected.Applied) != 0 ||
		len(rejected.Rejected) != 1 ||
		rejected.Rejected[0].Reason != fixengine.RejectionSuggestion {
		t.Fatalf("default suggestion result = %#v", rejected)
	}
	options.AllowSuggestion = true
	applied, err := fixengine.Coordinate(file, []fixengine.Selection{selection}, options)
	if err != nil {
		t.Fatal(err)
	}
	want := `package sample

import "os"

func run() {
	_ = os.Chmod("file", 0o644)
}
`
	if string(applied.Bytes) != want ||
		len(applied.Applied) != 1 ||
		len(applied.Rejected) != 0 {
		t.Fatalf("authorized suggestion result = %#v, bytes = %q", applied, applied.Bytes)
	}
	second := runNonOctalFileMode(t, string(applied.Bytes))
	if len(second.Files) != 1 || len(second.Files[0].Diagnostics) != 0 {
		t.Fatalf("fixed non-octal-file-mode result = %#v", second)
	}
}

func TestNonOctalFileModeMetadataAndGoVersion(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("non-octal-file-mode")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeCallExpr}) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!reflect.DeepEqual(
			metadata.Fixes,
			[]rules.FixMetadata{
				{
					Name: "use-octal-file-mode",
					Description: "spell the file mode as an octal literal",
					Safety: rules.FixSuggestion,
				},
			},
		) {
		t.Fatalf("non-octal-file-mode metadata = %#v, found = %v", metadata, found)
	}
	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"non-octal-file-mode": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil || len(older) != 0 {
		t.Fatalf("go1.24 non-octal-file-mode selection = %#v, %v", older, err)
	}
}

func TestNonOctalFileModeHonorsSharedSourcePolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/non-octal-mode-policy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample
import "os"
func suppressed(path string) error {
	//glippy:ignore non-octal-file-mode -- decimal mode is protocol-defined
	return os.Chmod(path, 644)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample
import "os"
func generated(path string) error { return os.Chmod(path, 644) }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid
import "os"
func invalid(path string) error {
	var text string = 1
	_ = text
	return os.Chmod(path, 644)
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
				"non-octal-file-mode": rules.SeverityWarn,
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
		t.Fatalf("non-octal-file-mode policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "non-octal-file-mode" {
				t.Fatalf("suppressed result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy path %q", file.Path)
		}
	}
}

func runNonOctalFileMode(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/non-octal-mode\n\ngo 1.25.0\n",
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
				"non-octal-file-mode": rules.SeverityWarn,
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
	return result
}

func BenchmarkNonOctalFileMode(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/non-octal-mode-benchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"os\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func mode%d(path string) error { return os.Chmod(path, 644) }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, runErr := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{
					"non-octal-file-mode": rules.SeverityWarn,
				},
				SourceGoVersion: "go1.25",
			},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				ModuleMode: analysis.ModuleReadonly,
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 100 {
			b.Fatalf("non-octal-file-mode benchmark result = %#v", result)
		}
	}
}
