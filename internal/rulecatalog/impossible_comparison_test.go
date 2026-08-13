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

func TestImpossibleComparisonReportsIntegerExtremeComparisons(t *testing.T) {
	t.Parallel()

	input := `package sample

func unsigned(value uint) (bool, bool) {
	return value < 0, 0 <= value
}

func fixedUnsigned(value uint8) (bool, bool) {
	return value > 255, value <= 255
}

func fixedSigned(value int8) (bool, bool, bool, bool) {
	return value < -128, value >= -128, 127 < value, 127 >= value
}

func possible(value int8) (bool, bool, bool, bool) {
	return value <= -128, value > -128, value < 127, value >= 127
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparison\n\ngo 1.25.0\n",
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
				"impossible-comparison": rules.SeverityWarn,
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
	want := []string{
		"value < 0",
		"0 <= value",
		"value > 255",
		"value <= 255",
		"value < -128",
		"value >= -128",
		"127 < value",
		"127 >= value",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("impossible-comparison result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing expression %q", want[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "impossible-comparison" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			!strings.Contains(diagnostic.Message, "always") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}

func TestImpossibleComparisonMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("impossible-comparison")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(
			metadata.NodeInterests,
			[]rules.NodeKind{rules.NodeBinaryExpr},
		) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("impossible-comparison metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkImpossibleComparisonPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisonbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nfunc f(value uint8) bool { return value > 255 }\n",
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
				"impossible-comparison": rules.SeverityWarn,
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
