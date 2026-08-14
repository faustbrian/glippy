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

const semanticDefectFixture = `package sample

import (
	"io"
	"math"
	"net/url"
	"sync"
)

type counter struct { value int }
func (counter counter) mutate() { counter.value = 1 }
func acquire() (io.ReadCloser, error) { return nil, nil }
func open() error {
	resource, err := acquire()
	defer resource.Close()
	if err != nil { return err }
	return nil
}
func recurse() { recurse() }
func defects(items []int, numerator, denominator int, parsed *url.URL, lock *sync.Mutex, value float64) {
	_ = append(items, 1)
	var entries map[string]int
	entries["key"] = 1
	_ = value == math.NaN()
	_ = float64(numerator / denominator)
	parsed.Query().Set("key", "value")
	defer lock.Lock()
	_ = any(42) == nil
}
`

func TestSemanticCorrectnessPackReportsHighConfidenceDefects(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"math"
	"net/url"
	"sync"
)

type counter struct { value int }

func (counter counter) mutate() {
	counter.value = 1
}

func defects(items []int, numerator, denominator int, parsed *url.URL, lock *sync.Mutex, value float64) {
	_ = append(items, 1)
	var entries map[string]int
	entries["key"] = 1
	_ = value == math.NaN()
	_ = float64(numerator / denominator)
	parsed.Query().Set("key", "value")
	defer lock.Lock()
}
`
	ruleIDs := []string{
		"deferred-lock",
		"ineffective-url-query-mutation",
		"ineffective-value-receiver-assignment",
		"integer-division-before-conversion",
		"ignored-append-result",
		"nan-comparison",
		"nil-map-write",
	}
	result := runSemanticCorrectnessRules(t, input, ruleIDs)
	got := make(map[string]int)
	for _, file := range result.Files {
		for _, diagnostic := range file.Diagnostics {
			got[diagnostic.RuleID]++
		}
	}
	for _, ruleID := range ruleIDs {
		if got[ruleID] != 1 {
			t.Fatalf("%s diagnostic count = %d; result = %#v", ruleID, got[ruleID], result)
		}
	}
	if len(got) != len(ruleIDs) {
		t.Fatalf("semantic correctness diagnostics = %#v", got)
	}
}

func TestSemanticCorrectnessPackExcludesIntentionalNearbyCode(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"math"
	"net/url"
	"sync"
)

type counter struct { value int }
type inner struct { value int }
type outer struct { *inner }

func initialize(entries *map[string]int) {
	*entries = make(map[string]int)
}

func (counter *counter) mutate() {
	counter.value = 1
}

func (counter counter) incremented() int {
	counter.value++
	return counter.value
}

func (counter counter) withValue(value int) counter {
	counter.value = value
	return counter
}

func (counter counter) useValue() {
	counter.value = 1
	println(counter.value)
}

func (counter counter) observeThroughPointer() {
	pointer := &counter
	defer func() { println(pointer.value) }()
	counter.value = 1
}

func (outer outer) mutateEmbeddedPointer() {
	outer.value = 1
}

func valid(items []int, numerator, denominator int, parsed *url.URL, lock *sync.Mutex, value float64) []int {
	items = append(items, 1)
	entries := make(map[string]int)
	entries["key"] = 1
	_ = math.IsNaN(value)
	_ = float64(numerator) / float64(denominator)
	_ = float64(4 / 2)
	var conditional map[string]int
	if numerator > 0 {
		conditional = make(map[string]int)
	}
	conditional["key"] = 1
	var blockInitialized map[string]int
	{
		blockInitialized = make(map[string]int)
	}
	blockInitialized["key"] = 1
	var switchInitialized map[string]int
	switch {
	default:
		switchInitialized = make(map[string]int)
	}
	switchInitialized["key"] = 1
	var rangeInitialized map[string]int
	for _, rangeInitialized = range []map[string]int{{}} {
		break
	}
	rangeInitialized["key"] = 1
	var helperInitialized map[string]int
	initialize(&helperInitialized)
	helperInitialized["key"] = 1
	query := parsed.Query()
	query.Set("key", "value")
	parsed.RawQuery = query.Encode()
	lock.Lock()
	defer lock.Unlock()
	return items
}
`
	ruleIDs := []string{
		"deferred-lock",
		"ineffective-url-query-mutation",
		"ineffective-value-receiver-assignment",
		"integer-division-before-conversion",
		"ignored-append-result",
		"nan-comparison",
		"nil-map-write",
	}
	result := runSemanticCorrectnessRules(t, input, ruleIDs)
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("valid semantic correctness result = %#v", result)
	}
}

func TestSemanticControlFlowPackReportsHighConfidenceDefects(t *testing.T) {
	t.Parallel()

	input := `package sample

import "io"

func acquire() (io.ReadCloser, error) { return nil, nil }

func open() error {
	resource, err := acquire()
	defer resource.Close()
	if err != nil { return err }
	return nil
}

func recurse(value int) int {
	return recurse(value)
}

func compare() bool {
	return any(42) == nil
}
`
	ruleIDs := []string{
		"defer-before-error-check",
		"impossible-interface-nil-comparison",
		"infinite-recursion",
	}
	result := runSemanticCorrectnessRules(t, input, ruleIDs)
	got := make(map[string]int)
	for _, file := range result.Files {
		for _, diagnostic := range file.Diagnostics {
			got[diagnostic.RuleID]++
		}
	}
	for _, ruleID := range ruleIDs {
		if got[ruleID] != 1 {
			t.Fatalf("%s diagnostic count = %d; result = %#v", ruleID, got[ruleID], result)
		}
	}
}

func TestSemanticCorrectnessPackReportsConcreteTypedNilAndTemporaryQueryMutations(t *testing.T) {
	t.Parallel()

	input := `package sample

import "net/url"

func defects(parsed *url.URL, pointer *int) {
	_ = any(pointer) == nil
	parsed.Query()["key"] = []string{"value"}
	delete(parsed.Query(), "key")
}
`
	ruleIDs := []string{
		"impossible-interface-nil-comparison",
		"ineffective-url-query-mutation",
	}
	result := runSemanticCorrectnessRules(t, input, ruleIDs)
	want := map[string]int{
		"impossible-interface-nil-comparison": 1,
		"ineffective-url-query-mutation":      2,
	}
	got := make(map[string]int)
	for _, file := range result.Files {
		for _, diagnostic := range file.Diagnostics {
			got[diagnostic.RuleID]++
		}
	}
	for ruleID, count := range want {
		if got[ruleID] != count {
			t.Fatalf("%s diagnostic count = %d, want %d; result = %#v", ruleID, got[ruleID], count, result)
		}
	}
}

func TestSemanticControlFlowPackExcludesSafeNearbyCode(t *testing.T) {
	t.Parallel()

	input := `package sample

import "io"

func acquire() (io.ReadCloser, error) { return nil, nil }
func fallback() io.ReadCloser { return nil }

func open() error {
	resource, err := acquire()
	if err != nil { return err }
	defer resource.Close()
	return nil
}

func openWhenSuccessful() error {
	resource, err := acquire()
	defer resource.Close()
	if err == nil { return nil }
	return err
}

func replaceBeforeDefer() error {
	resource, err := acquire()
	resource = fallback()
	defer resource.Close()
	if err != nil { return err }
	return nil
}

func recurse(value int) int {
	if value == 0 { return 0 }
	return recurse(value - 1)
}

func compare(value any) bool {
	return value == nil
}
`
	ruleIDs := []string{
		"defer-before-error-check",
		"impossible-interface-nil-comparison",
		"infinite-recursion",
	}
	result := runSemanticCorrectnessRules(t, input, ruleIDs)
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("valid semantic control-flow result = %#v", result)
	}
}

func TestInfiniteRecursionReportsVoidSelfCall(t *testing.T) {
	t.Parallel()

	result := runSemanticCorrectnessRules(
		t,
		"package sample\n\nfunc recurse() { recurse() }\n",
		[]string{"infinite-recursion"},
	)
	if countPackageDiagnostics(result) != 1 {
		t.Fatalf("void recursion result = %#v", result)
	}
}

func TestInfiniteRecursionReportsMethodSelfCall(t *testing.T) {
	t.Parallel()

	result := runSemanticCorrectnessRules(
		t,
		"package sample\n\ntype worker struct{}\nfunc (worker worker) recurse() { worker.recurse() }\n",
		[]string{"infinite-recursion"},
	)
	if countPackageDiagnostics(result) != 1 {
		t.Fatalf("method recursion result = %#v", result)
	}
}

func TestSemanticCorrectnessPackReportsExactRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id    string
		input string
		want  string
	}{
		{
			id:    "ignored-append-result",
			input: "package sample\nfunc run(items []int) { _ = append(items, 1) }\n",
			want:  "append(items, 1)",
		},
		{
			id:    "nil-map-write",
			input: "package sample\nfunc run() { var entries map[string]int; entries[\"key\"] = 1 }\n",
			want:  "entries[\"key\"]",
		},
		{
			id:    "ineffective-value-receiver-assignment",
			input: "package sample\ntype counter struct{ value int }; func (counter counter) run() { counter.value = 1 }\n",
			want:  "counter.value",
		},
		{
			id:    "nan-comparison",
			input: "package sample\nimport \"math\"\nfunc run(value float64) { _ = value == math.NaN() }\n",
			want:  "value == math.NaN()",
		},
		{
			id:    "integer-division-before-conversion",
			input: "package sample\nfunc run(left, right int) { _ = float64(left / right) }\n",
			want:  "float64(left / right)",
		},
		{
			id:    "ineffective-url-query-mutation",
			input: "package sample\nimport \"net/url\"\nfunc run(parsed *url.URL) { parsed.Query().Set(\"key\", \"value\") }\n",
			want:  "parsed.Query().Set(\"key\", \"value\")",
		},
		{
			id:    "deferred-lock",
			input: "package sample\nimport \"sync\"\nfunc run(lock *sync.Mutex) { defer lock.Lock() }\n",
			want:  "lock.Lock()",
		},
		{
			id: "defer-before-error-check",
			input: "package sample\nimport \"io\"\nfunc acquire() (io.ReadCloser, string, error) { return nil, \"\", nil }; " +
				"func run() error { resource, _, err := acquire(); defer resource.Close(); if err != nil { return err }; return nil }\n",
			want: "resource.Close()",
		},
		{
			id:    "infinite-recursion",
			input: "package sample\nfunc recurse/**/() { recurse() }\n",
			want:  "recurse()",
		},
		{
			id:    "impossible-interface-nil-comparison",
			input: "package sample\nfunc run() { _ = any(42) == nil }\n",
			want:  "any(42) == nil",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()
			result := runSemanticCorrectnessRules(t, test.input, []string{test.id})
			assertRuleRanges(t, test.input, result, test.id, test.id, []string{test.want})
			if len(result.Files[0].Diagnostics[0].Fixes) != 0 {
				t.Fatalf("%s unexpectedly offered a fix", test.id)
			}
		})
	}
}

func TestSemanticCorrectnessPackHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/semanticpolicy\n\ngo 1.26.0\n")
	suppressionHeader := ""
	for _, ruleID := range semanticCorrectnessRuleIDs() {
		suppressionHeader += "//glippy:ignore-file " + ruleID + " -- policy fixture\n"
	}
	suppressedPath := filepath.Join(root, "suppressed.go")
	generatedPath := filepath.Join(root, "generated", "generated.go")
	invalidPath := filepath.Join(root, "invalid", "invalid.go")
	writeFixture(t, suppressedPath, suppressionHeader+semanticDefectFixture)
	writeFixture(t, generatedPath, "// Code generated by fixture. DO NOT EDIT.\n"+semanticDefectFixture)
	writeFixture(t, invalidPath, semanticDefectFixture+"\nvar invalid string = 1\n")

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	overrides := make(map[string]rules.Severity)
	for _, ruleID := range semanticCorrectnessRuleIDs() {
		overrides[ruleID] = rules.SeverityWarn
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets:         []rules.Preset{},
			Overrides:       overrides,
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir:        root,
			Patterns:   []string{"./..."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 || len(result.LoadDiagnostics) == 0 {
		t.Fatalf("semantic policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch file.Path {
		case suppressedPath:
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != len(overrides) {
				t.Fatalf("suppressed semantic result = %#v", file)
			}
			got := make([]string, len(file.Suppressed))
			for index, suppressed := range file.Suppressed {
				got[index] = suppressed.Diagnostic.RuleID
			}
			slices.Sort(got)
			if !slices.Equal(got, semanticCorrectnessRuleIDs()) {
				t.Fatalf("suppressed semantic rules = %q", got)
			}
		case generatedPath, invalidPath:
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded semantic result = %#v", file)
			}
		default:
			t.Fatalf("unexpected semantic policy path %q", file.Path)
		}
	}

	selection, err := registry.ResolveOptions(rules.ResolveOptions{
		Presets:         []rules.Preset{},
		Overrides:       overrides,
		SourceGoVersion: "go1.24",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("pre-minimum semantic selection = %#v", selection)
	}
	selection, err = registry.ResolveOptions(rules.ResolveOptions{
		Presets:         []rules.Preset{},
		Overrides:       overrides,
		SourceGoVersion: "go1.25",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != len(overrides) {
		t.Fatalf("minimum-version semantic selection = %#v", selection)
	}
}

func BenchmarkSemanticCorrectnessPackPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(b, filepath.Join(root, "go.mod"), "module example.com/semanticbenchmark\n\ngo 1.26.0\n")
	writeFixture(b, filepath.Join(root, "sample.go"), semanticDefectFixture)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	overrides := make(map[string]rules.Severity)
	for _, ruleID := range semanticCorrectnessRuleIDs() {
		overrides[ruleID] = rules.SeverityWarn
	}
	benchmarkPackageRuns(
		b,
		registry,
		analysis.RunOptions{
			Presets:         []rules.Preset{},
			Overrides:       overrides,
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir:        root,
			Patterns:   []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		len(overrides),
	)
}

func semanticCorrectnessRuleIDs() []string {
	return []string{
		"defer-before-error-check",
		"deferred-lock",
		"ignored-append-result",
		"impossible-interface-nil-comparison",
		"ineffective-url-query-mutation",
		"ineffective-value-receiver-assignment",
		"infinite-recursion",
		"integer-division-before-conversion",
		"nan-comparison",
		"nil-map-write",
	}
}

func runSemanticCorrectnessRules(
	t *testing.T,
	input string,
	ruleIDs []string,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/semanticcorrectness\n\ngo 1.26.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	overrides := make(map[string]rules.Severity, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		overrides[ruleID] = rules.SeverityWarn
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets:         []rules.Preset{},
			Overrides:       overrides,
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir:        root,
			Patterns:   []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
