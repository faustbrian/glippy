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
		!metadata.RequiresEffectFacts ||
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

func TestNilErrorWrapConsumesImportedReturnStateFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/nilwrapfacts\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import "errors"

type Value struct{}

func Lookup(found bool) (*Value, error) {
	if found { return &Value{}, nil }
	return nil, errors.New("missing")
}

func Reverse(found bool) (*Value, error) {
	if found { return nil, nil }
	return &Value{}, errors.New("unexpected")
}

func Ambiguous(found bool) (*Value, error) {
	if found { return &Value{}, nil }
	return &Value{}, errors.New("missing")
}

func Conflicting(found bool) (*Value, error) {
	if found { return nil, errors.New("missing") }
	return &Value{}, errors.New("also missing")
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "delegate", "delegate.go"),
		`package delegate

import "example.com/nilwrapfacts/helper"

func Lookup(found bool) (*helper.Value, error) { return helper.Lookup(found) }
func Reverse(found bool) (*helper.Value, error) { return helper.Reverse(found) }
func Ambiguous(found bool) (*helper.Value, error) { return helper.Ambiguous(found) }
func Conflicting(found bool) (*helper.Value, error) { return helper.Conflicting(found) }
`,
	)
	input := `package sample

import (
	"fmt"

	"example.com/nilwrapfacts/delegate"
	"example.com/nilwrapfacts/helper"
)

func nonNilSibling(found bool) error {
	value, err := helper.Lookup(found)
	if value != nil {
		return fmt.Errorf("lookup succeeded: %w", err)
	}
	return err
}

func nilSibling(found bool) error {
	value, err := helper.Reverse(found)
	if value == nil {
		return fmt.Errorf("reverse succeeded: %w", err)
	}
	return err
}

func ambiguous(found bool) error {
	value, err := helper.Ambiguous(found)
	if value != nil {
		return fmt.Errorf("ambiguous result: %w", err)
	}
	return err
}

func conflicting(found bool) error {
	value, err := helper.Conflicting(found)
	if value != nil {
		return fmt.Errorf("conflicting result: %w", err)
	}
	return err
}

func delegatedNonNilSibling(found bool) error {
	value, err := delegate.Lookup(found)
	if value != nil {
		return fmt.Errorf("delegated lookup succeeded: %w", err)
	}
	return err
}

func delegatedNilSibling(found bool) error {
	value, err := delegate.Reverse(found)
	if value == nil {
		return fmt.Errorf("delegated reverse succeeded: %w", err)
	}
	return err
}

func delegatedAmbiguous(found bool) error {
	value, err := delegate.Ambiguous(found)
	if value != nil {
		return fmt.Errorf("delegated ambiguous result: %w", err)
	}
	return err
}

func delegatedConflicting(found bool) error {
	value, err := delegate.Conflicting(found)
	if value != nil {
		return fmt.Errorf("delegated conflicting result: %w", err)
	}
	return err
}
`
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
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 4 {
		t.Fatalf("return-state nil-error-wrap result = %#v", result)
	}
	want := []string{
		`fmt.Errorf("lookup succeeded: %w", err)`,
		`fmt.Errorf("reverse succeeded: %w", err)`,
		`fmt.Errorf("delegated lookup succeeded: %w", err)`,
		`fmt.Errorf("delegated reverse succeeded: %w", err)`,
	}
	for index, snippet := range want {
		start := strings.Index(input, snippet) + strings.LastIndex(snippet, "err")
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "nil-error-wrap" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("err") {
			t.Fatalf("return-state diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestNilErrorWrapConsumesExactUnconditionalResultStates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/nilwrapresultfacts\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import "errors"

func Nil() error { return nil }
func TupleNil() (int, error) { return 1, nil }
func Failure() error { return errors.New("failed") }
func Unknown(err error) error { return err }
func Deferred() (err error) {
	defer func() { err = errors.New("deferred") }()
	return nil
}
func DeferredPanic() error {
	defer func() { panic("deferred") }()
	return nil
}
func DeferredHelperPanic() error {
	defer stopDeferredReturn()
	return nil
}
func stopDeferredReturn() { panic("deferred") }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "delegate", "delegate.go"),
		`package delegate

import "example.com/nilwrapresultfacts/helper"

func Nil() error { return helper.Nil() }
func TupleNil() (int, error) { return helper.TupleNil() }
`,
	)
	input := `package sample

import (
	"fmt"

	"example.com/nilwrapresultfacts/delegate"
	"example.com/nilwrapresultfacts/helper"
)

type typedError struct{}
func (*typedError) Error() string { return "typed" }

func localNil() error { return nil }
func localRecursive() error { return localRecursive() }
func localTypedNil() error {
	var err *typedError
	return err
}

func directLocal() error {
	return fmt.Errorf("local: %w", localNil())
}

func assignedImported() error {
	err := helper.Nil()
	return fmt.Errorf("imported: %w", err)
}

func directImported() error {
	return fmt.Errorf("direct imported: %w", helper.Nil())
}

func tupleImported() error {
	_, err := helper.TupleNil()
	return fmt.Errorf("tuple: %w", err)
}

func impossibleNonNilBranch() error {
	err := helper.Nil()
	if err != nil {
		return fmt.Errorf("impossible non-nil branch: %w", err)
	}
	return nil
}

func delegatedImported() error {
	return fmt.Errorf("delegated imported: %w", delegate.Nil())
}

func delegatedTupleImported() error {
	_, err := delegate.TupleNil()
	return fmt.Errorf("delegated tuple: %w", err)
}

func failure() error {
	err := helper.Failure()
	return fmt.Errorf("failure: %w", err)
}

func unknown(err error) error {
	err = helper.Unknown(err)
	return fmt.Errorf("unknown: %w", err)
}

func deferred() error {
	err := helper.Deferred()
	return fmt.Errorf("deferred: %w", err)
}

func deferredPanic() error {
	return fmt.Errorf("deferred panic: %w", helper.DeferredPanic())
}

func deferredHelperPanic() error {
	return fmt.Errorf("deferred helper panic: %w", helper.DeferredHelperPanic())
}

func dynamic(operation func() error) error {
	err := operation()
	return fmt.Errorf("dynamic: %w", err)
}

func recursive() error {
	err := localRecursive()
	return fmt.Errorf("recursive: %w", err)
}

func typedNil() error {
	err := localTypedNil()
	return fmt.Errorf("typed nil: %w", err)
}

func merged(selectLocal bool) error {
	var err error
	if selectLocal {
		err = localNil()
	} else {
		err = helper.Nil()
	}
	return fmt.Errorf("merged: %w", err)
}
`
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
	want := []struct {
		call string
		argument string
	}{
		{call: `fmt.Errorf("local: %w", localNil())`, argument: "localNil()"},
		{call: `fmt.Errorf("imported: %w", err)`, argument: "err"},
		{call: `fmt.Errorf("direct imported: %w", helper.Nil())`, argument: "helper.Nil()"},
		{call: `fmt.Errorf("tuple: %w", err)`, argument: "err"},
		{
			call: `fmt.Errorf("delegated imported: %w", delegate.Nil())`,
			argument: "delegate.Nil()",
		},
		{call: `fmt.Errorf("delegated tuple: %w", err)`, argument: "err"},
	}
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("result-state nil-error-wrap result = %#v", result)
	}
	for index, expected := range want {
		start := strings.Index(input, expected.call) +
			strings.LastIndex(expected.call, expected.argument)
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "nil-error-wrap" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expected.argument) {
			t.Fatalf("result-state diagnostic[%d] = %#v", index, diagnostic)
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

func BenchmarkDelegatedResultStatePackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/delegatedresultbenchmark\n\ngo 1.26.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"fmt\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func base%d() error { return nil }; func delegate%d() error { return base%d() }; func run%d() error { return fmt.Errorf(\"operation: %%w\", delegate%d()) }\n",
			index,
			index,
			index,
			index,
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
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		100,
	)
}

func BenchmarkDelegatedReturnRelationshipPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/delegatedreturnbenchmark\n\ngo 1.26.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport (\"errors\"; \"fmt\")\ntype Value struct{}\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func base%d(found bool) (*Value, error) { if found { return &Value{}, nil }; return nil, errors.New(\"missing\") }; func delegate%d(found bool) (*Value, error) { return base%d(found) }; func run%d(found bool) error { value, err := delegate%d(found); if value != nil { return fmt.Errorf(\"operation: %%w\", err) }; return err }\n",
			index,
			index,
			index,
			index,
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
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		100,
	)
}
