package rulecatalog_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestEmptyBranchReportsUncommentedIfBranches(t *testing.T) {
	t.Parallel()

	input := `package sample

func run(value int) {
	if value == 0 {}
	if value == 1 { use() } else {}
	if value == 2 { /* intentional */ }
	if value == 3 { use() } else { /* intentional */ }
	if value == 4 { use() }
	if value == 5 {} else if value == 6 { use() }
	if value == 7 {;}
}

func use() {}
`
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"empty-branch": rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 4 {
		t.Fatalf("empty-branch result = %#v", result)
	}
	wantBlocks := []string{"{}", "{}", "{}", "{;}"}
	searchFrom := 0
	for index, diagnostic := range result.Diagnostics {
		wantBlock := wantBlocks[index]
		relative := strings.Index(input[searchFrom:], wantBlock)
		if relative < 0 {
			t.Fatal("missing expected empty block")
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "empty-branch" ||
			diagnostic.MessageKey != "empty-if-branch" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(wantBlock) ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("empty-branch diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(wantBlock)
	}
}

func TestManualMinMaxReportsIntegerAssignments(t *testing.T) {
	t.Parallel()

	input := `package sample

type Count int

func adjust(left, right Count, first, second float64) {
	if left < right { left = right }
	if right > left { left = right }
	if left > right { left = right }
	if right < left { left = right }
	if left < right { right = left }
	if first < second { first = second }
	if current := left; current < right { current = right }
	if left < right { left = right } else { right = left }
	if left < right { left = right; use(left) }
	if left < right { left += right }
}

func use(Count) {}
`
	result := runOnePedanticRule(t, "manual-min-max", input)
	assertRuleRanges(
		t,
		input,
		result,
		"manual-min-max",
		"manual-min-max",
		[]string{
			"left < right",
			"right > left",
			"left > right",
			"right < left",
			"left < right",
		},
	)
	wantOperations := []string{"max", "max", "min", "min", "min"}
	for index, diagnostic := range result.Files[0].Diagnostics {
		operation := wantOperations[index]
		if diagnostic.Message !=
			"conditional assignment manually implements " + operation ||
			diagnostic.Help !=
				"assign the result of the " + operation + " built-in directly" ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("manual-min-max diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestRedundantTypeDeclarationReportsIdenticalInferredTypes(t *testing.T) {
	t.Parallel()

	input := `package sample

type Count int

func source() int { return 1 }

const untypedConstant = 1
const typedConstant int = 1
const namedConstant Count = 1

var packageValue int = 1
var fromUntypedConstant int = untypedConstant
var fromTypedConstant int = typedConstant
var fromNamedConstant Count = namedConstant
var converted Count = Count(1)
var named Count = 1
var widened int64 = 1
var boxed any = 1
var absent []int = nil

func run() {
	var local int = source()
	var inferred int = 1
	var commented int /* keep */ = 1
	_, _, _, _, _, _, _, _, _, _, _, _ = packageValue, fromUntypedConstant, fromTypedConstant, fromNamedConstant, converted, named, widened, boxed, absent, local, inferred, commented
}
`
	result := runOnePedanticRule(t, "redundant-type-declaration", input)
	want := []string{
		"packageValue int = 1",
		"fromUntypedConstant int = untypedConstant",
		"fromTypedConstant int = typedConstant",
		"fromNamedConstant Count = namedConstant",
		"converted Count = Count(1)",
		"local int = source()",
		"inferred int = 1",
		"commented int /* keep */ = 1",
	}
	assertRuleRanges(t, input, result, "redundant-type-declaration", "redundant-type", want)
	for index, diagnostic := range result.Files[0].Diagnostics {
		if index == len(result.Files[0].Diagnostics) - 1 {
			if len(diagnostic.Fixes) != 0 {
				t.Fatalf("commented declaration fixes = %#v", diagnostic.Fixes)
			}
			continue
		}
		if len(diagnostic.Fixes) != 1 ||
			diagnostic.Fixes[0].Name != "remove-redundant-type" ||
			diagnostic.Fixes[0].Safety != rules.FixSafe ||
			len(diagnostic.Fixes[0].Edits) != 1 ||
			diagnostic.Fixes[0].Edits[0].NewText != " = " {
			t.Fatalf(
				"redundant type diagnostic[%d] fixes = %#v",
				index,
				diagnostic.Fixes,
			)
		}
	}
}

func TestAdditionalPedanticMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		requirement rules.Requirement
		interests []rules.NodeKind
	}{
		"empty-branch": {rules.RequireSyntax, []rules.NodeKind{rules.NodeIfStmt}},
		"manual-min-max": {rules.RequireTypes, []rules.NodeKind{rules.NodeIfStmt}},
		"redundant-type-declaration": {
			rules.RequireTypes,
			[]rules.NodeKind{rules.NodeValueSpec},
		},
	}
	for id, expected := range want {
		metadata, found := registry.Metadata(id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(
				metadata.Presets,
				[]rules.Preset{rules.PresetPedantic},
			) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != expected.requirement ||
			!reflect.DeepEqual(metadata.NodeInterests, expected.interests) ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors {
			t.Fatalf("%s metadata = %#v, found = %v", id, metadata, found)
		}
	}
	redundant, _ := registry.Metadata("redundant-type-declaration")
	if !reflect.DeepEqual(
		redundant.Fixes,
		[]rules.FixMetadata{
			{
				Name: "remove-redundant-type",
				Description: "remove the explicit type already inferred from the initializer",
				Safety: rules.FixSafe,
			},
		},
	) {
		t.Fatalf("redundant-type-declaration fixes = %#v", redundant.Fixes)
	}
	for _, id := range []string{"empty-branch", "manual-min-max"} {
		metadata, _ := registry.Metadata(id)
		if len(metadata.Fixes) != 0 {
			t.Fatalf("%s fixes = %#v", id, metadata.Fixes)
		}
	}
}

func BenchmarkEmptyBranchSyntaxAnalysis(b *testing.B) {
	file, err := source.Load(
		"sample.go",
		[]byte("package sample\nfunc run(value int) { if value > 0 {} }\n"),
	)
	if err != nil {
		b.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, runErr := analysis.Run(
			context.Background(),
			file,
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{
					"empty-branch": rules.SeverityWarn,
				},
				SourceGoVersion: "go1.25",
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Diagnostics) != 1 {
			b.Fatalf("benchmark result = %#v", result)
		}
	}
}

func BenchmarkManualMinMaxPackageAnalysis(b *testing.B) {
	benchmarkPedanticExpressionRule(
		b,
		"manual-min-max",
		"package sample\nfunc run(left, right int) int { if left < right { left = right }; return left }\n",
	)
}

func BenchmarkRedundantTypeDeclarationPackageAnalysis(b *testing.B) {
	benchmarkPedanticExpressionRule(
		b,
		"redundant-type-declaration",
		"package sample\nfunc run() int { var value int = source(); return value }\nfunc source() int { return 1 }\n",
	)
}
