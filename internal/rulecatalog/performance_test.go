package rulecatalog_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

const performanceDefectFixture = `package sample

import (
	"io"
	"regexp"
	"sync"
)

func defects(text string, writer io.Writer, data []byte, pool *sync.Pool) {
	for { regexp.MatchString("constant", text); break }
	pool.Put(data)
	for _, runeValue := range []rune(text) { _ = runeValue }
	io.WriteString(writer, string(data))
}
`

func TestPerformancePresetReportsCredibleCosts(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io"
	"regexp"
	"sync"
)

const constantPattern = "[a-z]+"

func regex(text string, reader io.RuneReader) {
	regexp.MatchString("outside", text)
	for index := 0; index < 2; index++ {
		regexp.MatchString("inside", text)
		regexp.Match(constantPattern, []byte(text))
	}
	for range []string{text} {
		regexp.MatchReader("inside", reader)
	}
}

func pooling(pool *sync.Pool, bytes []byte) {
	pool.Put(42)
	pool.Put(bytes)
}

func ranges(text string) {
	for range []rune(text) {}
	for _, value := range []rune(text) { _ = value }
}

func writes(writer io.Writer, bytes []byte) {
	io.WriteString(writer, string(bytes))
}
`
	result := runPerformanceRules(t, input, nil)
	want := map[string]int{
		"inefficient-io-string-write": 1,
		"regexp-compile-in-loop": 3,
		"string-range-rune-conversion": 2,
		"sync-pool-non-pointer": 2,
	}
	got := make(map[string]int)
	for _, file := range result.Files {
		for _, diagnostic := range file.Diagnostics {
			got[diagnostic.RuleID]++
		}
	}
	if len(got) != len(want) {
		t.Fatalf("performance diagnostics = %#v, want %#v; result = %#v", got, want, result)
	}
	for ruleID, count := range want {
		if got[ruleID] != count {
			t.Fatalf(
				"%s diagnostic count = %d, want %d; result = %#v",
				ruleID,
				got[ruleID],
				count,
				result,
			)
		}
	}
}

func TestRegexpCompileInLoopIncludesImmediatelyInvokedFunctionLiterals(t *testing.T) {
	t.Parallel()

	input := `package sample

import "regexp"

func run(values []string) {
	for _, value := range values {
		func() { regexp.MatchString("inside", value) }()
	}
}
`
	result := runPerformanceRules(t, input, nil)
	if countPackageDiagnostics(result) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "regexp-compile-in-loop" {
		t.Fatalf("immediate function literal result = %#v", result)
	}
}

func TestPerformancePresetExcludesNearbyCodeWithoutTheCostContract(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io"
	"regexp"
	"sync"
)

type customPool struct{}
func (*customPool) Put(any) {}

func writeString(io.Writer, string) {}

func valid(
	pattern string,
	text string,
	writer io.Writer,
	bytes []byte,
	runes []rune,
	pool *sync.Pool,
	custom *customPool,
	value any,
) {
	regexp.MatchString("outside", text)
	for range []string{text} {
		regexp.MatchString(pattern, text)
	}
	pointer := new(int)
	pool.Put(pointer)
	pool.Put(map[string]int{})
	pool.Put(make(chan int))
	pool.Put(func() {})
	pool.Put(value)
	pool.Put(nil)
	custom.Put(42)
	for index, runeValue := range []rune(text) { _, _ = index, runeValue }
	for _, runeValue := range runes { _ = runeValue }
	io.WriteString(writer, text)
	io.WriteString(writer, string(runes))
	writeString(writer, string(bytes))
}
`
	result := runPerformanceRules(t, input, nil)
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("nearby performance result = %#v", result)
	}
}

func TestPerformanceRulesExposeStableMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, ruleID := range performanceRuleIDs() {
		metadata, found := registry.Metadata(ruleID)
		if !found {
			t.Fatalf("missing performance rule %q", ruleID)
		}
		if metadata.DefaultSeverity != rules.SeverityWarn ||
			!slices.Equal(metadata.Presets, []rules.Preset{rules.PresetPerformance}) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != rules.RequireTypes ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors ||
			!slices.Equal(
				metadata.Categories,
				[]rules.Category{rules.CategoryPerformance},
			) ||
			len(metadata.Fixes) != 0 ||
			len(metadata.Examples) == 0 ||
			len(metadata.KnownLimitations) == 0 {
			t.Fatalf("%s metadata = %#v", ruleID, metadata)
		}
	}
}

func TestPerformanceRulesReportExactRangesWithoutFixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id string
		input string
		want string
	}{
		{
			id: "regexp-compile-in-loop",
			input: "package sample\nimport \"regexp\"\nfunc run(text string) { for { regexp.MatchString(\"x\", text) } }\n",
			want: "regexp.MatchString(\"x\", text)",
		},
		{
			id: "sync-pool-non-pointer",
			input: "package sample\nimport \"sync\"\nfunc run(pool *sync.Pool) { pool.Put(42) }\n",
			want: "42",
		},
		{
			id: "string-range-rune-conversion",
			input: "package sample\nfunc run(text string) { for _, value := range []rune(text) { _ = value } }\n",
			want: "[]rune(text)",
		},
		{
			id: "inefficient-io-string-write",
			input: "package sample\nimport \"io\"\nfunc run(writer io.Writer, data []byte) { io.WriteString(writer, string(data)) }\n",
			want: "string(data)",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.id,
			func(t *testing.T) {
				t.Parallel()
				result := runPerformanceRules(t, test.input, nil)
				assertRuleRanges(
					t,
					test.input,
					result,
					test.id,
					test.id,
					[]string{test.want},
				)
				if len(result.Files[0].Diagnostics[0].Fixes) != 0 {
					t.Fatalf("%s unexpectedly offered a fix", test.id)
				}
			},
		)
	}
}

func TestPerformanceRulesHonorSharedPoliciesAndSourceVersions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/performancepolicy\n\ngo 1.26.0\n",
	)
	suppressionHeader := ""
	for _, ruleID := range performanceRuleIDs() {
		suppressionHeader += "//glippy:ignore-file " +
			ruleID +
			" -- performance policy fixture\n"
	}
	suppressedPath := filepath.Join(root, "suppressed", "sample.go")
	generatedPath := filepath.Join(root, "generated", "sample.go")
	invalidPath := filepath.Join(root, "invalid", "sample.go")
	writeFixture(t, suppressedPath, suppressionHeader + performanceDefectFixture)
	writeFixture(
		t,
		generatedPath,
		"// Code generated by fixture. DO NOT EDIT.\n" + performanceDefectFixture,
	)
	writeFixture(t, invalidPath, performanceDefectFixture + "\nvar invalid string = 1\n")

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{rules.PresetPerformance},
			SourceGoVersion: "go1.26",
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
		t.Fatalf("performance policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch file.Path {
		case suppressedPath:
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != len(performanceRuleIDs()) {
				t.Fatalf("suppressed performance result = %#v", file)
			}
		case generatedPath, invalidPath:
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded performance result = %#v", file)
			}
		default:
			t.Fatalf("unexpected performance policy path %q", file.Path)
		}
	}

	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetPerformance},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("pre-minimum performance selection = %#v", selection)
	}
	selection, err = registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetPerformance},
			SourceGoVersion: "go1.25",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != len(performanceRuleIDs()) {
		t.Fatalf("minimum-version performance selection = %#v", selection)
	}
}

func BenchmarkPerformanceRulesPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/performancebenchmark\n\ngo 1.26.0\n",
	)
	writeFixture(b, filepath.Join(root, "sample.go"), performanceDefectFixture)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, ruleID := range performanceRuleIDs() {
		ruleID := ruleID
		b.Run(
			ruleID,
			func(b *testing.B) {
				benchmarkPackageRuns(
					b,
					registry,
					analysis.RunOptions{
						Presets: []rules.Preset{},
						Overrides: map[string]rules.Severity{
							ruleID: rules.SeverityWarn,
						},
						SourceGoVersion: "go1.26",
					},
					analysis.PackageLoadOptions{
						Dir: root,
						Patterns: []string{"."},
						ModuleMode: analysis.ModuleReadonly,
					},
					1,
				)
			},
		)
	}
}

func performanceRuleIDs() []string {
	return []string{
		"inefficient-io-string-write",
		"regexp-compile-in-loop",
		"string-range-rune-conversion",
		"sync-pool-non-pointer",
	}
}

func runPerformanceRules(
	t *testing.T,
	input string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/performance\n\ngo 1.26.0\n",
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
			Presets: []rules.Preset{rules.PresetPerformance},
			Overrides: overrides,
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
	return result
}
