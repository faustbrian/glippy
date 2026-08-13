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

func TestSubsumedConditionReportsUnreachableElseIfBranches(t *testing.T) {
	t.Parallel()

	input := `package sample

func upper(value int) int {
	if value > 0 { return 1 } else if value > 10 { return 2 }
	return 0
}

func lower(value int) int {
	if value <= 100 { return 1 } else if value < 10 { return 2 }
	return 0
}

func inclusiveUpper(value int) int {
	if value >= 0 { return 1 } else if value >= 10 { return 2 }
	return 0
}

func strictLower(value int) int {
	if value < 100 { return 1 } else if value <= 10 { return 2 }
	return 0
}

func distinct(value int) int {
	if value > 10 { return 1 } else if value > 0 { return 2 }
	return 0
}

func initialized(value int) int {
	if current := value; current > 0 { return 1 } else if current > 10 { return 2 }
	return 0
}
`
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/subsumed\n\ngo 1.25.0\n")
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
				"subsumed-condition": rules.SeverityWarn,
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
	want := []string{"value > 10", "value < 10", "value >= 10", "value <= 10"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("subsumed-condition result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing expression %q", want[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "subsumed-condition" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			!strings.Contains(diagnostic.Message, "subsumed") ||
			len(diagnostic.Related) != 1 ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}

func TestSubsumedConditionMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("subsumed-condition")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeIfStmt}) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("subsumed-condition metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkSubsumedConditionPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/subsumedbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nfunc f(value int) int { if value > 0 { return 1 } else if value > 10 { return 2 }; return 0 }\n",
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
				"subsumed-condition": rules.SeverityWarn,
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
