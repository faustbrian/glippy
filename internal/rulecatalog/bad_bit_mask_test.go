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

func TestBadBitMaskReportsImpossibleMaskedComparisons(t *testing.T) {
	t.Parallel()

	input := `package sample

func bad(value uint8) (bool, bool, bool, bool, bool) {
	return value&0b0010 == 0b0001,
		value&0b0010 != 0b0001,
		value|0b0010 == 0b0001,
		value&0 == 0,
		value&(0b0100|0b0010) == (0b0111^0b1000)
}

func swapped(value uint8) bool {
	return 0b0001 == 0b0010&value
}

func possible(value uint8) (bool, bool) {
	return value&0b0011 == 0b0001, value|0b0001 == 0b0011
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/badbitmask\n\ngo 1.25.0\n",
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
			Overrides: map[string]rules.Severity{"bad-bit-mask": rules.SeverityWarn},
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
		"value&0b0010 == 0b0001",
		"value&0b0010 != 0b0001",
		"value|0b0010 == 0b0001",
		"value&0 == 0",
		"value&(0b0100|0b0010) == (0b0111^0b1000)",
		"0b0001 == 0b0010&value",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("bad-bit-mask result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing expression %q", want[index])
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "bad-bit-mask" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			!strings.Contains(diagnostic.Message, "always") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}

func TestBadBitMaskMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("bad-bit-mask")
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
		t.Fatalf("bad-bit-mask metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkBadBitMaskPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/badbitmaskbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nfunc f(value uint8) bool { return value&2 == 1 }\n",
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
			Overrides: map[string]rules.Severity{"bad-bit-mask": rules.SeverityWarn},
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
