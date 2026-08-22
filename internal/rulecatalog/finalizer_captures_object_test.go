package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestFinalizerCapturesObjectReportsRetainingClosures(t *testing.T) {
	t.Parallel()

	input := `package sample

import "runtime"

type resource struct{}

func (*resource) Close() {}

func directCapture() {
	value := &resource{}
	runtime.SetFinalizer(value, func(*resource) {
		value.Close()
	})
}

func assignedCapture() {
	value := &resource{}
	finalizer := func(*resource) {
		value.Close()
	}
	runtime.SetFinalizer(value, finalizer)
}

func parameterCapture(value *resource) {
	runtime.SetFinalizer(value, func(*resource) {
		value.Close()
	})
}

func usesArgument() {
	value := &resource{}
	runtime.SetFinalizer(value, func(current *resource) {
		current.Close()
	})
}

func namedFinalizer(value *resource) {
	value.Close()
}

func usesNamedFinalizer() {
	value := &resource{}
	runtime.SetFinalizer(value, namedFinalizer)
	runtime.SetFinalizer(value, nil)
}

func capturesAnotherObject() {
	value := &resource{}
	other := &resource{}
	runtime.SetFinalizer(value, func(*resource) {
		other.Close()
	})
}

func capturesAlias() {
	value := &resource{}
	alias := value
	runtime.SetFinalizer(value, func(*resource) {
		alias.Close()
	})
}

func staticallyResolvedAliasCall() {
	value := &resource{}
	setFinalizer := runtime.SetFinalizer
	setFinalizer(value, func(*resource) {
		value.Close()
	})
}

func unresolvedCall(setFinalizer func(any, any)) {
	value := &resource{}
	setFinalizer(value, func(*resource) {
		value.Close()
	})
}

func ambiguousFinalizer(condition bool) {
	value := &resource{}
	finalizer := func(*resource) {
		value.Close()
	}
	if condition {
		finalizer = func(*resource) {}
	}
	runtime.SetFinalizer(value, finalizer)
}

func SetFinalizer(any, any) {}

func userDefinedFunction() {
	value := &resource{}
	SetFinalizer(value, func(*resource) {
		value.Close()
	})
}
`
	result := runFinalizerCapturesObject(t, input)
	if len(result.Files[0].Diagnostics) != 4 {
		t.Fatalf("finalizer-captures-object result = %#v", result)
	}
	const closureText = "func(*resource) {\n\t\tvalue.Close()\n\t}"
	closureRanges := make([][2]int, 0, 4)
	closureRemaining := input
	closureOffset := 0
	for index := range 4 {
		start := strings.Index(closureRemaining, closureText)
		if start < 0 {
			t.Fatalf("fixture is missing finalizer closure %d", index)
		}
		start += closureOffset
		closureRanges = append(closureRanges, [2]int{start, start + len(closureText)})
		closureOffset = start + len(closureText)
		closureRemaining = input[closureOffset:]
	}
	primaryTexts := []string{
		"runtime.SetFinalizer(value, " + closureText + ")",
		"runtime.SetFinalizer(value, finalizer)",
		"runtime.SetFinalizer(value, " + closureText + ")",
		"setFinalizer(value, " + closureText + ")",
	}
	remaining := input
	offset := 0
	for index, primaryText := range primaryTexts {
		start := strings.Index(remaining, primaryText)
		if start < 0 {
			t.Fatalf("fixture is missing finalizer call %d", index)
		}
		start += offset
		end := start + len(primaryText)
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "finalizer-captures-object" ||
			diagnostic.MessageKey != "finalizer-captures-object" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != end ||
			len(diagnostic.Related) != 1 ||
			diagnostic.Related[0].Range.Start != closureRanges[index][0] ||
			diagnostic.Related[0].Range.End != closureRanges[index][1] ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf(
				"finalizer-captures-object diagnostic[%d] = %#v",
				index,
				diagnostic,
			)
		}
		offset = end
		remaining = input[offset:]
	}
}

func TestFinalizerCapturesObjectMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("finalizer-captures-object")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!slices.Equal(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSSA ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!slices.Equal(
			metadata.Categories,
			[]rules.Category{rules.CategoryCorrectness, rules.CategorySafety},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("finalizer-captures-object metadata = %#v, found = %v", metadata, found)
	}
}

func TestFinalizerCapturesObjectHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/finalizercapturesobjectpolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import "runtime"

type suppressedResource struct{}

func (*suppressedResource) Close() {}

func suppressed() {
	value := &suppressedResource{}
	//glippy:ignore finalizer-captures-object -- legacy finalizer retained during migration
	runtime.SetFinalizer(value, func(*suppressedResource) {
		value.Close()
	})
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import "runtime"

type generatedResource struct{}

func (*generatedResource) Close() {}

func generated() {
	value := &generatedResource{}
	runtime.SetFinalizer(value, func(*generatedResource) {
		value.Close()
	})
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import "runtime"

type resource struct{}

func (*resource) Close() {}

func invalid() {
	value := &resource{}
	var broken string = 1
	_ = broken
	runtime.SetFinalizer(value, func(*resource) {
		value.Close()
	})
}
`,
	)
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"finalizer-captures-object": rules.SeverityError,
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
		t.Fatalf("finalizer-captures-object policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID !=
					"finalizer-captures-object" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed finalizer-captures-object result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded finalizer-captures-object result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range selection {
		if selected.ID == "finalizer-captures-object" {
			t.Fatalf("pre-minimum finalizer-captures-object selection = %#v", selection)
		}
	}
}

func runFinalizerCapturesObject(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/finalizercapturesobject\n\ngo 1.25.0\n",
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
				"finalizer-captures-object": rules.SeverityWarn,
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
		t.Fatalf("finalizer-captures-object files = %#v", result)
	}
	return result
}

func BenchmarkFinalizerCapturesObjectPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/finalizercapturesobjectbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString(
		"package sample\nimport \"runtime\"\ntype resource struct{}\nfunc (*resource) Close() {}\n",
	)
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() { value := &resource{}; runtime.SetFinalizer(value, func(*resource) { value.Close() }) }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
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
				"finalizer-captures-object": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		100,
	)
}
