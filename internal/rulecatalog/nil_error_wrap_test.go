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

func TestNilErrorWrapReportsOnlyPathProvenNilErrors(t *testing.T) {
	t.Parallel()

	input := `package sample

import "fmt"

type concreteError struct{}

func (*concreteError) Error() string { return "failure" }

func literalNil() error {
	return fmt.Errorf("literal: %w", nil)
}

func zeroValue() error {
	var err error
	return fmt.Errorf("zero value: %w", err)
}

func nilBranch(err error) error {
	if err == nil {
		return fmt.Errorf("nil branch: %w", err)
	}
	return err
}

func nilFallthrough(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("fallthrough %s: %w", "failure", err)
}

func nonNilBranch(err error) error {
	if err != nil {
		return fmt.Errorf("non-nil: %w", err)
	}
	return nil
}

func initializerNonNilBranch(operation func() error) error {
	if err := operation(); err != nil {
		return fmt.Errorf("initializer non-nil: %w", err)
	}
	return nil
}

func loopNonNilBranch(operations []func() error) error {
	for _, operation := range operations {
		if err := operation(); err != nil {
			return fmt.Errorf("loop non-nil: %w", err)
		}
	}
	return nil
}

func unknown(err error) error {
	return fmt.Errorf("unknown: %w", err)
}

func dynamic(format string, err error) error {
	if err == nil {
		return fmt.Errorf(format, err)
	}
	return err
}

func indexed(err error) error {
	if err == nil {
		return fmt.Errorf("indexed: %[1]w", err)
	}
	return err
}

func starred(err error) error {
	if err == nil {
		return fmt.Errorf("starred: %*w", 1, err)
	}
	return err
}

func indirect(err error) error {
	if err == nil {
		formatError := fmt.Errorf
		return formatError("indirect: %w", err)
	}
	return err
}

func typedNil() error {
	var err *concreteError
	return fmt.Errorf("typed nil: %w", err)
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/nilerrorwrap\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeFixture(t, path, input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("nil-error-wrap")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		metadata.Requirement != rules.RequireSSA ||
		metadata.RequiresEffectFacts ||
		!slices.Equal(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!slices.Equal(metadata.Categories, []rules.Category{rules.CategoryCorrectness}) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("nil-error-wrap metadata = %#v, %t", metadata, found)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"nil-error-wrap": rules.SeverityWarn},
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
	if len(result.Files) != 1 || result.Files[0].Path != path {
		t.Fatalf("nil-error-wrap result = %#v", result)
	}
	snippets := []struct {
		text string
		argument string
	}{
		{text: `fmt.Errorf("literal: %w", nil)`, argument: "nil"},
		{text: `fmt.Errorf("zero value: %w", err)`, argument: "err"},
		{text: `fmt.Errorf("nil branch: %w", err)`, argument: "err"},
		{text: `fmt.Errorf("fallthrough %s: %w", "failure", err)`, argument: "err"},
	}
	if len(result.Files[0].Diagnostics) != len(snippets) {
		t.Fatalf("nil-error-wrap diagnostics = %#v", result.Files[0].Diagnostics)
	}
	for index, snippet := range snippets {
		start := strings.Index(input, snippet.text)
		if start < 0 {
			t.Fatalf("fixture does not contain %q", snippet.text)
		}
		start += strings.LastIndex(snippet.text, snippet.argument)
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "nil-error-wrap" ||
			diagnostic.Severity != rules.SeverityWarn ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(snippet.argument) ||
			diagnostic.MessageKey != "nil-error-wrap" ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("nil-error-wrap diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestNilErrorWrapHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/nilerrorwrappolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import "fmt"

func suppressed(err error) error {
	if err == nil {
		//glippy:ignore nil-error-wrap -- compatibility requires this placeholder
		return fmt.Errorf("suppressed: %w", err)
	}
	return err
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import "fmt"

func generated() error {
	return fmt.Errorf("generated: %w", nil)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import "fmt"

func invalid() error {
	var broken string = 1
	_ = broken
	return fmt.Errorf("invalid: %w", nil)
}
`,
	)
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"nil-error-wrap": rules.SeverityError},
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
		t.Fatalf("nil-error-wrap policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "nil-error-wrap" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed nil-error-wrap result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded nil-error-wrap result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetSuspicious},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range selection {
		if selected.ID == "nil-error-wrap" {
			t.Fatalf("pre-minimum nil-error-wrap selection = %#v", selection)
		}
	}
}

func BenchmarkNilErrorWrapPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/nilerrorwrapbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"fmt\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(err error) error { if err == nil { return fmt.Errorf(\"operation: %%w\", err) }; return err }\n",
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
			Overrides: map[string]rules.Severity{"nil-error-wrap": rules.SeverityWarn},
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
