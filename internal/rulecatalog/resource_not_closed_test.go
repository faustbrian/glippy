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

func TestResourceNotClosedReportsUnreleasedLocalClosers(t *testing.T) {
	t.Parallel()

	input := `package sample

import "os"

func bad() error {
	file, err := os.Open("input")
	if err != nil { return err }
	_ = file.Name()
	return nil
}

func badFallthrough() {
	file, _ := os.Open("input")
	_ = file.Name()
}

func goodDefer() error {
	file, err := os.Open("input")
	if err != nil { return err }
	defer file.Close()
	return nil
}

func goodExplicit() error {
	file, err := os.Open("input")
	if err != nil { return err }
	return file.Close()
}

func partialClose(closeFile bool) error {
	file, err := os.Open("input")
	if err != nil { return err }
	if closeFile { return file.Close() }
	return nil
}

func completedBranches(closeFile bool) error {
	file, err := os.Open("input")
	if err != nil { return err }
	if closeFile { return file.Close() }
	consume(file)
	return nil
}

func overwritten() error {
	file, err := os.Open("input")
	if err != nil { return err }
	file, err = os.Open("replacement")
	if err != nil { return err }
	defer file.Close()
	return nil
}

func transfer() (*os.File, error) {
	file, err := os.Open("input")
	if err != nil { return nil, err }
	return file, nil
}

func pass() error {
	file, err := os.Open("input")
	if err != nil { return err }
	consume(file)
	return nil
}

func consume(*os.File) {}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourcenotclosed\n\ngo 1.25.0\n",
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
				"resource-not-closed": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 4 {
		t.Fatalf("resource-not-closed result = %#v", result)
	}
	expected := []struct {
		function string
		acquisition string
	}{
		{function: "func bad()", acquisition: "file, err := os.Open"},
		{function: "func badFallthrough()", acquisition: "file, _ := os.Open"},
		{function: "func partialClose(", acquisition: "file, err := os.Open"},
		{function: "func overwritten()", acquisition: "file, err := os.Open"},
	}
	expectedStarts := make(map[int]bool, len(expected))
	for index, location := range expected {
		functionStart := strings.Index(input, location.function)
		if functionStart < 0 {
			t.Fatalf("missing function %d", index)
		}
		relative := strings.Index(input[functionStart:], location.acquisition)
		if relative < 0 {
			t.Fatalf("missing acquisition %d", index)
		}
		expectedStarts[functionStart + relative] = true
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		start := diagnostic.Range.Start
		if diagnostic.RuleID != "resource-not-closed" ||
			diagnostic.Range.End != start + len("file") ||
			!expectedStarts[start] ||
			!strings.Contains(diagnostic.Message, "not closed") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("resource-not-closed diagnostic %d = %#v", index, diagnostic)
		}
		delete(expectedStarts, start)
	}
	if len(expectedStarts) != 0 {
		t.Fatalf("missing resource-not-closed ranges = %#v", expectedStarts)
	}
}

func TestResourceNotClosedMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("resource-not-closed")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		len(metadata.NodeInterests) != 0 ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("resource-not-closed metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkResourceNotClosedPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/resourcenotclosedbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nimport \"os\"\nfunc run() error { file, err := os.Open(\"input\"); if err != nil { return err }; _ = file.Name(); return nil }\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	benchmarkPackageRuns(
		b,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		1,
	)
}
