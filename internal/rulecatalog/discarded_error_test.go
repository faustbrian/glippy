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

func TestDiscardedErrorReportsIgnoredErrorResults(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"bytes"
	"errors"
	"fmt"
)

func fail() error { return errors.New("failed") }
func pair() (int, error) { return 0, nil }

func bad(buffer *bytes.Buffer) {
	fail()
	pair()
	fmt.Errorf("wrapped")
	buffer.WriteString("safe")
}

func good() error {
	if err := fail(); err != nil { return err }
	_, _ = pair()
	return fail()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/discardederror\n\ngo 1.25.0\n",
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
			Overrides: map[string]rules.Severity{"discarded-error": rules.SeverityWarn},
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
	want := []string{"fail()", "pair()", "fmt.Errorf(\"wrapped\")"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("discarded-error result = %#v", result)
	}
	searchFrom := strings.Index(input, "func bad")
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing expression %q", want[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "discarded-error" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			!strings.Contains(diagnostic.Message, "error result") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}

func TestDiscardedErrorMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("discarded-error")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeExprStmt}) ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("discarded-error metadata = %#v, found = %v", metadata, found)
	}
}

func TestDiscardedErrorCanIncludeTestFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/discardederrortests\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		"package sample\nfunc fail() error { return nil }\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample_test.go"),
		"package sample\nimport \"testing\"\nfunc TestFailure(t *testing.T) { fail() }\n",
	)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	run := func(includeTests bool) analysis.PackageResult {
		t.Helper()
		result, runErr := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{
					"discarded-error": rules.SeverityWarn,
				},
				RuleOptions: map[string]rules.OptionSet{
					"discarded-error": rules.NewOptionSet(
						map[string]rules.OptionValue{
							"include-tests": rules.BooleanOption(
								includeTests,
							),
						},
					),
				},
				SourceGoVersion: "go1.25",
			},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				Tests: true,
				ModuleMode: analysis.ModuleReadonly,
			},
		)
		if runErr != nil {
			t.Fatal(runErr)
		}
		return result
	}
	withoutTests := run(false)
	if countPackageDiagnostics(withoutTests) != 0 {
		t.Fatalf("default test-file result = %#v", withoutTests)
	}
	withTests := run(true)
	if countPackageDiagnostics(withTests) != 1 {
		t.Fatalf("included test-file result = %#v", withTests)
	}
}

func countPackageDiagnostics(result analysis.PackageResult) int {
	total := 0
	for _, file := range result.Files {
		total += len(file.Diagnostics)
	}
	return total
}

func BenchmarkDiscardedErrorPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/discardederrorbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nfunc fail() error { return nil }\nfunc run() { fail() }\n",
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
			Overrides: map[string]rules.Severity{"discarded-error": rules.SeverityWarn},
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
