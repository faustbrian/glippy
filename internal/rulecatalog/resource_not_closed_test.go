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

type customCommand struct{}

func (*customCommand) StdoutPipe() (*os.File, error) {
	return os.Open("input")
}

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

func badCustomPipe(command *customCommand) error {
	file, err := command.StdoutPipe()
	if err != nil { return err }
	_ = file.Name()
	return nil
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 7 {
		t.Fatalf("resource-not-closed result = %#v", result)
	}
	expected := []struct {
		function string
		acquisition string
	}{
		{function: "func bad()", acquisition: "file, err := os.Open"},
		{function: "func badFallthrough()", acquisition: "file, _ := os.Open"},
		{function: "func badCustomPipe(", acquisition: "file, err := command.StdoutPipe"},
		{function: "func partialClose(", acquisition: "file, err := os.Open"},
		{function: "func completedBranches(", acquisition: "file, err := os.Open"},
		{function: "func overwritten()", acquisition: "file, err := os.Open"},
		{function: "func pass()", acquisition: "file, err := os.Open"},
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

func TestResourceNotClosedAcceptsDogfoodOwnershipPatterns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourceownership\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"io"
	"os"
	"os/exec"
	"testing"
)

func methodDefer(root *os.Root) error {
	file, err := root.Open("input")
	if os.IsNotExist(err) { return err }
	if err != nil { return err }
	defer file.Close()
	_, err = file.Stat()
	return err
}

func cleanupCapture(t *testing.T) {
	store, err := os.OpenRoot(".")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = store.Close() })
}

func rejectedConstruction(t *testing.T) {
	store, err := os.OpenRoot("missing")
	if store != nil || err == nil { t.Fatal("constructor unexpectedly succeeded") }
}

func returnSuccessfulLoop(root *os.Root) (string, *os.File, error) {
	for range 10 {
		file, err := root.Open("input")
		if err == nil { return "input", file, nil }
		if !os.IsNotExist(err) { return "", nil, err }
	}
	return "", nil, os.ErrNotExist
}

func pipeTransfer() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	if err := consume(output); err != nil { _ = output.Close(); return err }
	return command.Wait()
}

func commandOwnedPipe() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	if err := command.Start(); err != nil { return err }
	extractErr := consume(output)
	if extractErr != nil { _ = output.Close() }
	waitErr := command.Wait()
	if extractErr != nil { return extractErr }
	return waitErr
}

func consume(io.Reader) error { return nil }
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("resource-not-closed dogfood patterns = %#v", result)
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
