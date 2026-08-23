package rulecatalog_test

import (
	"context"
	"fmt"
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

func TestImpossibleComparisonExcludesArchitectureSizedConstantConversions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisonportable\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "bounds.go"),
		"package sample\n\nimport . \"unsafe\"\n\ntype widthSized [Sizeof(uintptr(0))*32 - 1]byte\ntype offsetLayout struct { prefix [Sizeof(uintptr(0))*32 - 1]byte; value byte }\ntype genericLayout[T any] struct { prefix T; value byte }\n\nvar fixedLayout struct { prefix [127]byte; value byte }\nvar portableLayout offsetLayout\nvar portableWidth widthSized\nvar portableGenericLayout genericLayout[widthSized]\nvar portableAnonymousGenericLayout genericLayout[[parameterLength]byte]\n\nconst maxInt = int(^uint(0) >> 1)\nconst parameterLength = 1 - int(^uint(0)>>63)\nconst portableMax = int64(maxInt)\nconst shiftedPortableMax = uint64(uint(1)<<63 | (uint(1)<<63)-1)\nconst representabilityMax = int(1<<40)>>33 - 1\nconst dotPortableMax = uint64(1<<(Sizeof(uintptr(0))*8) - 1)\nconst portableLayoutSize = uint8(Sizeof(widthSized{}))\nconst portableValueSize = uint8(Sizeof(portableWidth))\nconst portableLength = uint8(len(widthSized{}))\nconst portableCapacity = uint8(cap(widthSized{}))\nconst portableLayoutOffset = uint8(Offsetof(portableLayout.value))\nconst portableGenericOffset = uint8(Offsetof(portableGenericLayout.value))\nconst portableAnonymousGenericOffset = uint8(Offsetof(portableAnonymousGenericLayout.value))\nconst fixedSizeMax = int8(Sizeof([127]byte{}))\nconst fixedAlignMax = int8(Alignof(int32(0))*32 - 1)\nconst fixedOffsetMax = int8(Offsetof(fixedLayout.value))\nconst fixedMax int64 = 9223372036854775807\nconst fixedInt int = 127\nconst fixedConverted = int(127)\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "dependency", "layout.go"),
		`package dependency

import "unsafe"

type SizeLayout [unsafe.Sizeof(uintptr(0))*32 - 1]byte

type OffsetLayout struct {
	prefix [unsafe.Sizeof(uintptr(0))*32 - 1]byte
	Value byte
}
`,
	)
	input := `package sample

import (
	"example.com/impossiblecomparisonportable/dependency"
	"unsafe"
)

var importedOffsetLayout dependency.OffsetLayout

const importedLayoutSize = uint8(unsafe.Sizeof(dependency.SizeLayout{}))
const importedLayoutOffset = uint8(unsafe.Offsetof(importedOffsetLayout.Value))

func portableDirect(value int64) bool {
	return value > int64(maxInt)
}

func portableNamed(value int64) bool {
	return value > portableMax
}

func portableDotImportedUnsafe(value uint64) bool {
	return value > dotPortableMax
}

func portableShift(value uint64) bool {
	return value > shiftedPortableMax
}

func portableNamedLayoutSize(value uint8) bool {
	return value > portableLayoutSize
}

func portableNamedValueSize(value uint8) bool {
	return value > portableValueSize
}

func portableNamedLength(value uint8) bool {
	return value > portableLength
}

func portableNamedCapacity(value uint8) bool {
	return value > portableCapacity
}

func portableNamedLayoutOffset(value uint8) bool {
	return value > portableLayoutOffset
}

func portableImportedLayoutSize(value uint8) bool {
	return value > importedLayoutSize
}

func portableImportedLayoutOffset(value uint8) bool {
	return value > importedLayoutOffset
}

func portableGenericLayoutOffset(value uint8) bool {
	return value > portableGenericOffset
}

func portableAnonymousGenericLayoutOffset(value uint8) bool {
	return value < portableAnonymousGenericOffset
}

func portableParameter(value uint8, values [parameterLength]byte) bool {
	return value < uint8(len(values))
}

func portableShortDeclaration(value uint8) bool {
	values := [parameterLength]byte{}
	return value < uint8(cap(values))
}

func portableInferredVariable(value uint8) bool {
	var values = [parameterLength]byte{}
	return value < uint8(unsafe.Sizeof(values))
}

func portableRepresentability(value int8) bool {
	return value > int8(representabilityMax)
}

func fixed(value int64) bool {
	return value > int64(9223372036854775807)
}

func fixedNamed(value int64) bool {
	return value > fixedMax
}

func fixedLocalNamed(value int64) bool {
	const localFixedMax int64 = 9223372036854775807
	return value > localFixedMax
}

func fixedArchitectureSized(value int8) bool {
	return value > int8(fixedInt)
}

func fixedArchitectureConversion(value int8) bool {
	return value > int8(fixedConverted)
}

func fixedLength(value int8) bool {
	return value > int8(len([127]byte{}))
}

func fixedUnsafeSize(value int8) bool {
	return value > fixedSizeMax
}

func fixedUnsafeAlign(value int8) bool {
	return value > fixedAlignMax
}

func fixedUnsafeOffset(value int8) bool {
	return value > fixedOffsetMax
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
		"value > int64(9223372036854775807)",
		"value > fixedMax",
		"value > localFixedMax",
		"value > int8(fixedInt)",
		"value > int8(fixedConverted)",
		"value > int8(len([127]byte{}))",
		"value > fixedSizeMax",
		"value > fixedAlignMax",
		"value > fixedOffsetMax",
	}
	var diagnostics []rules.Diagnostic
	for _, file := range result.Files {
		diagnostics = append(diagnostics, file.Diagnostics...)
	}
	if len(diagnostics) != len(want) {
		got := make([]string, 0, len(diagnostics))
		for _, diagnostic := range diagnostics {
			got = append(got, input[diagnostic.Range.Start:diagnostic.Range.End])
		}
		t.Fatalf("portable integer comparisons = %q, want %q", got, want)
	}
	for index, diagnostic := range diagnostics {
		if content := input[diagnostic.Range.Start:diagnostic.Range.End];
			diagnostic.RuleID != "impossible-comparison" || content != want[index] {
			t.Fatalf(
				"portable integer comparison diagnostic[%d] = %#v for %q",
				index,
				diagnostic,
				content,
			)
		}
	}
}

func TestImpossibleComparisonExcludesInheritedIotaAndSelectorLayouts(t *testing.T) {
	t.Parallel()

	input := `package sample

const (
	base = int((1 << (iota + 31)) - 1)
	portableMax
)

const width = 1 - int(^uint(0)>>63)

type holder struct { values [width]byte }

var layout holder

const selectorLength = uint8(len(layout.values))

func inherited(value uint32) bool { return value > uint32(portableMax) }
func selected(value uint8) bool { return value < selectorLength }
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisoniota\n\ngo 1.25.0\n",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("architecture-dependent declarations = %#v", result.Files)
	}
}

func TestImpossibleComparisonExcludesImportedArchitectureSizedConstants(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisonimported\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "dependency", "bounds.go"),
		"package dependency\n\nconst Width = 1 - int(^uint(0)>>63)\n",
	)
	input := `package sample

import "example.com/impossiblecomparisonimported/dependency"

func portable(value uint8) bool {
	return value < uint8(dependency.Width)
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("imported architecture constant result = %#v", result.Files)
	}
}

func TestImpossibleComparisonExcludesAnonymousGenericLayouts(t *testing.T) {
	t.Parallel()

	input := `package sample

import "unsafe"

const width = 1 - int(^uint(0)>>63)

type box[T any] struct {
	prefix T
	value byte
}

func portable(candidate uint8, layout box[[width]byte]) bool {
	return candidate < uint8(unsafe.Offsetof(layout.value))
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisongeneric\n\ngo 1.25.0\n",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("anonymous generic layout result = %#v", result.Files)
	}
}

func TestImpossibleComparisonBoundsConstantProvenance(t *testing.T) {
	t.Parallel()

	var input strings.Builder
	input.WriteString("package sample\n\nconst short0 = 255\n")
	for index := 1; index <= 8; index++ {
		fmt.Fprintf(&input, "const short%d = short%d\n", index, index - 1)
	}
	input.WriteString("\nconst long0 = 255\n")
	for index := 1; index <= 5_000; index++ {
		fmt.Fprintf(&input, "const long%d = long%d\n", index, index - 1)
	}
	input.WriteString(
		`
func fixed(value uint8) bool { return value > uint8(short8) }
func bounded(value uint8) bool { return value > uint8(long5000) }
`,
	)
	source := input.String()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisonbound\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), source)
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("bounded constant provenance result = %#v", result.Files)
	}
	want := "value > uint8(short8)"
	diagnostic := result.Files[0].Diagnostics[0]
	if content := source[diagnostic.Range.Start:diagnostic.Range.End]; content != want {
		t.Fatalf("bounded constant provenance diagnostic = %q, want %q", content, want)
	}
}

func TestImpossibleComparisonArchitectureBudgetDoesNotPoisonTypeCache(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, fieldCount int, cheapFirst bool) []string {
		t.Helper()
		var input strings.Builder
		input.WriteString(
			"package sample\n\nimport \"unsafe\"\n\ntype Cheap [255]byte\ntype Huge struct {\n",
		)
		for index := range fieldCount {
			fmt.Fprintf(&input, "field%d byte\n", index)
		}
		input.WriteString("tail Cheap\n}\n\n")
		cheap := "func cheap(value uint8, layout Cheap) bool { return value > 255 * uint8(unsafe.Alignof(layout)) }\n"
		large := "func large(value uint8, layout Huge) bool { return value > uint8(unsafe.Alignof(layout)) * 255 }\n"
		if cheapFirst {
			input.WriteString(cheap)
			input.WriteString(large)
		} else {
			input.WriteString(large)
			input.WriteString(cheap)
		}
		source := input.String()
		root := t.TempDir()
		writeFixture(
			t,
			filepath.Join(root, "go.mod"),
			"module example.com/impossiblecomparisontypecache\n\ngo 1.25.0\n",
		)
		writeFixture(t, filepath.Join(root, "sample.go"), source)
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
		var findings []string
		for _, file := range result.Files {
			for _, diagnostic := range file.Diagnostics {
				findings = append(
					findings,
					source[diagnostic.Range.Start:diagnostic.Range.End],
				)
			}
		}
		return findings
	}

	largeFirst := run(t, 3_274, false)
	cheapFirst := run(t, 3_274, true)
	want := []string{"value > 255 * uint8(unsafe.Alignof(layout))"}
	if !reflect.DeepEqual(largeFirst, want) || !reflect.DeepEqual(cheapFirst, want) {
		t.Fatalf(
			"architecture budget findings = large-first %q, cheap-first %q, want %q",
			largeFirst,
			cheapFirst,
			want,
		)
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

func BenchmarkImpossibleComparisonArchitectureTypeDAG(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/impossiblecomparisontypedag\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\n\nimport \"unsafe\"\n\ntype T0 int\n")
	for depth := 1; depth <= 30; depth++ {
		fmt.Fprintf(&input, "type T%d struct { left, right T%d }\n", depth, depth - 1)
	}
	input.WriteString(
		"\nfunc bounded(value uint8, layout T30) bool { " +
			"return value > uint8(unsafe.Alignof(layout)) }\n",
	)
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
				"impossible-comparison": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		0,
	)
}
