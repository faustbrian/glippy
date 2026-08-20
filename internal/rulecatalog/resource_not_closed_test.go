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

func nilResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file == nil { return nil }
	return file.Close()
}

func reversedNilResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if nil == file { return nil }
	return file.Close()
}

func nilResultUnreleased() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file == nil { return nil }
	_ = file.Name()
	return nil
}

func nonNilResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file != nil { return file.Close() }
	return nil
}

func explicitElseResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file != nil { return file.Close() } else { return nil }
}

func nonNilResultUnreleased() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file != nil { _ = file.Name() }
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 9 {
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
		{function: "func nilResultUnreleased()", acquisition: "file, err := os.Open"},
		{function: "func nonNilResultUnreleased()", acquisition: "file, err := os.Open"},
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

func TestResourceNotClosedUsesGuaranteedCleanupManagedResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/cleanupmanaged\n\ngo 1.26.0\n",
	)
	input := `package sample

import (
	"os"
	"testing"
)

func directManaged(t *testing.T) *os.File {
	file, _ := os.Open("input")
	_ = file.Name()
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func helperManaged(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { closeFile(file) })
	return file
}

func closeFile(file *os.File) { _ = file.Close() }

func observationOnly(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Name() })
	return file
}

func conditionallyManaged(t *testing.T, closeFile bool) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() {
		if closeFile { _ = file.Close() }
	})
	return file
}

func conditionallyRegistered(t *testing.T, register bool) *os.File {
	file, _ := os.Open("input")
	if register { t.Cleanup(func() { _ = file.Close() }) }
	return file
}

func asynchronouslyManaged(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { go file.Close() })
	return file
}

func nestedOnly(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = func() error { return file.Close() } })
	return file
}

func replacedAfterRegistration(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Close() })
	file, _ = os.Open("replacement")
	return file
}

func aliasedBeforeCleanup(t *testing.T) *os.File {
	file, _ := os.Open("input")
	alias := file
	_ = alias
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func replacedDuringCleanup(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() {
		file, _ = os.Open("replacement")
		_ = file.Close()
	})
	return file
}

func replaceFile(file **os.File) { *file, _ = os.Open("replacement") }

func escapedDuringCleanup(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() {
		replaceFile(&file)
		_ = file.Close()
	})
	return file
}

func copiedTestHandle(t testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func use(t *testing.T) {
	direct := directManaged(t)
	_ = direct.Name()
	helper := helperManaged(t)
	_ = helper.Name()
	observed := observationOnly(t)
	_ = observed.Name()
	conditional := conditionallyManaged(t, false)
	_ = conditional.Name()
	registered := conditionallyRegistered(t, false)
	_ = registered.Name()
	asynchronous := asynchronouslyManaged(t)
	_ = asynchronous.Name()
	nested := nestedOnly(t)
	_ = nested.Name()
	replaced := replacedAfterRegistration(t)
	_ = replaced.Name()
	aliased := aliasedBeforeCleanup(t)
	_ = aliased.Name()
	callbackReplaced := replacedDuringCleanup(t)
	_ = callbackReplaced.Name()
	callbackEscaped := escapedDuringCleanup(t)
	_ = callbackEscaped.Name()
	copied := copiedTestHandle(*t)
	_ = copied.Name()
}
`
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 10 {
		t.Fatalf("cleanup-managed result diagnostics = %#v", result)
	}
	want := map[int]string{
		strings.Index(input, "observed := observationOnly"): "observed",
		strings.Index(input, "conditional := conditionallyManaged"): "conditional",
		strings.Index(input, "registered := conditionallyRegistered"): "registered",
		strings.Index(input, "asynchronous := asynchronouslyManaged"): "asynchronous",
		strings.Index(input, "nested := nestedOnly"): "nested",
		strings.Index(input, "replaced := replacedAfterRegistration"): "replaced",
		strings.Index(input, "aliased := aliasedBeforeCleanup"): "aliased",
		strings.Index(
			input,
			"callbackReplaced := replacedDuringCleanup",
		): "callbackReplaced",
		strings.Index(input, "callbackEscaped := escapedDuringCleanup"): "callbackEscaped",
		strings.Index(input, "copied := copiedTestHandle"): "copied",
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		name, found := want[diagnostic.Range.Start]
		if !found || diagnostic.Range.End != diagnostic.Range.Start + len(name) {
			t.Fatalf("cleanup-managed result diagnostic = %#v", diagnostic)
		}
		delete(want, diagnostic.Range.Start)
	}
	if len(want) != 0 {
		t.Fatalf("missing cleanup-managed result diagnostics = %#v", want)
	}
}

func TestResourceNotClosedUsesImportedCleanupManagedResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/importedcleanup\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import (
	"os"
	"testing"
)

func Open(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { closeFile(file) })
	return file
}

func closeFile(file *os.File) { _ = file.Close() }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"testing"

	"example.com/importedcleanup/helper"
)

func use(t *testing.T) {
	file := helper.Open(t)
	_ = file.Name()
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
		t.Fatalf("imported cleanup-managed result diagnostics = %#v", result)
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
