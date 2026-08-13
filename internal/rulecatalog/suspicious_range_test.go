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

func TestSuspiciousRangeReportsMutationOfCopiedValues(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func badSlice(values []item) {
	for _, value := range values {
		value.enabled = true
	}
}

func badMap(values map[string]item) {
	for _, value := range values {
		value.enabled = true
	}
}

func goodIndex(values []item) {
	for index := range values {
		values[index].enabled = true
	}
}

func goodPointers(values []*item) {
	for _, value := range values {
		value.enabled = true
	}
}

func goodRead(values []item) bool {
	for _, value := range values {
		if value.enabled { return true }
	}
	return false
}

func goodWriteBack(values []item) {
	for index, value := range values {
		value.enabled = true
		values[index] = value
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrange\n\ngo 1.25.0\n",
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
				"suspicious-range": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("suspicious-range result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], "value.enabled = true")
		if relative < 0 {
			t.Fatal("missing copied-value mutation")
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "suspicious-range" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("value.enabled") ||
			!strings.Contains(diagnostic.Message, "copy") {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len("value.enabled = true")
	}
}

func TestSuspiciousRangeMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("suspicious-range")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeRangeStmt}) ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("suspicious-range metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkSuspiciousRangePackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangebenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\ntype item struct{ ready bool }\nfunc run(values []item) { for _, value := range values { value.ready = true } }\n",
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
				"suspicious-range": rules.SeverityWarn,
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
