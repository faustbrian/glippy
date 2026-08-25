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

func TestIneffectiveAssignmentReportsUnreadAssignedValues(t *testing.T) {
	t.Parallel()

	input := `package sample

func next() int { return 1 }
func observe(int) {}

func assigned() {
	value := 0
	observe(value)
	value = next()
}

func received(ch <-chan error) {
	var err error
	if err != nil { panic(err) }
	err = <-ch
}

func incremented() {
	offset := 0
	observe(offset)
	offset++
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/ineffectiveassignment\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeFixture(t, path, input)
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
				"ineffective-assignment": rules.SeverityWarn,
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
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 3 {
		t.Fatalf("ineffective-assignment result = %#v", result)
	}
	for index, target := range []string{"value = next()", "err = <-ch", "offset++"} {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, target)
		if start < 0 {
			t.Fatalf("fixture does not contain %q", target)
		}
		identifier := strings.TrimRight(target, "+")
		if assignment := strings.IndexByte(identifier, ' '); assignment >= 0 {
			identifier = identifier[:assignment]
		}
		if diagnostic.RuleID != "ineffective-assignment" ||
			diagnostic.Severity != rules.SeverityWarn ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(identifier) ||
			diagnostic.MessageKey != "ineffective-assignment" ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("ineffective-assignment diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestIneffectiveAssignmentReportsUnreadCompoundAssignmentResult(t *testing.T) {
	t.Parallel()

	input := `package sample

func next() int { return 1 }
func observe(int) {}

func compound() {
	count := 0
	observe(count)
	count += next()
}
`
	result := runIneffectiveAssignment(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("compound ineffective-assignment result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	start := strings.Index(input, "count += next()")
	if start < 0 ||
		diagnostic.RuleID != "ineffective-assignment" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len("count") {
		t.Fatalf("compound ineffective-assignment diagnostic = %#v", diagnostic)
	}
}

func TestIneffectiveAssignmentPreservesObservedAndExplicitlyIgnoredValues(t *testing.T) {
	t.Parallel()

	input := `package sample

func next() int { return 1 }
func pair() (int, error) { return 1, nil }
func observe(int) {}

func observed() {
	value := 0
	observe(value)
	value = next()
	observe(value)
}

func explicitlyIgnored() {
	value := 0
	observe(value)
	value = next()
	_ = value
}

func switchUse() {
	value := 0
	observe(value)
	value = next()
	switch value { case 1: }
}

func joined(condition bool) {
	value := 0
	observe(value)
	if condition { value = next() }
	observe(value)
}

func observedUpdates() {
	value := 0
	observe(value)
	value++
	observe(value)
	value += next()
	observe(value)
}

func constantAssignment() {
	value := next()
	observe(value)
	value = 0
}

func tupleAssignment() {
	value, err := pair()
	observe(value)
	if err != nil { panic(err) }
	value, err = pair()
	if err != nil { panic(err) }
}
`
	result := runIneffectiveAssignment(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("ineffective-assignment boundary result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	target := "value, err = pair()"
	start := strings.LastIndex(input, target)
	if start < 0 ||
		diagnostic.RuleID != "ineffective-assignment" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len("value") {
		t.Fatalf("ineffective-assignment tuple diagnostic = %#v", diagnostic)
	}
}

func TestIneffectiveAssignmentMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("ineffective-assignment")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetNursery}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSSA ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!reflect.DeepEqual(
			metadata.Categories,
			[]rules.Category{rules.CategoryCorrectness, rules.CategorySuspicious},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("ineffective-assignment metadata = %#v, found = %v", metadata, found)
	}
}

func TestOverwrittenErrorSupersedesGeneralIneffectiveAssignment(t *testing.T) {
	t.Parallel()

	input := `package sample

func first() error { return nil }
func second() error { return nil }

func overwritten() {
	err := first()
	err = second()
	if err != nil { panic(err) }
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/ineffectiveassignmentinteraction\n\ngo 1.25.0\n",
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
				"ineffective-assignment": rules.SeverityWarn,
				"overwritten-error": rules.SeverityWarn,
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
	if len(result.Files) != 1 ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "overwritten-error" {
		t.Fatalf("overwritten-error interaction result = %#v", result)
	}
}

func TestIneffectiveAssignmentHonorsSharedPoliciesAndSourceVersions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/ineffectiveassignmentpolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

func suppressedNext() int { return 1 }
func suppressedObserve(int) {}

func suppressed() {
	value := 0
	suppressedObserve(value)
	//glippy:ignore ineffective-assignment -- the result is intentionally discarded
	value = suppressedNext()
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

func generatedNext() int { return 1 }
func generatedObserve(int) {}

func generated() {
	value := 0
	generatedObserve(value)
	value = generatedNext()
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

func next() int { return 1 }
func observe(int) {}

func run() {
	value := 0
	observe(value)
	value = next()
	var broken string = 1
	_ = broken
}
`,
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	overrides := map[string]rules.Severity{"ineffective-assignment": rules.SeverityError}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
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
		t.Fatalf("ineffective-assignment policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "ineffective-assignment" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed ineffective-assignment result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded ineffective-assignment result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("pre-minimum ineffective-assignment selection = %#v", selection)
	}
}

func runIneffectiveAssignment(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/ineffectiveassignmentfixture\n\ngo 1.25.0\n",
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
				"ineffective-assignment": rules.SeverityWarn,
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
