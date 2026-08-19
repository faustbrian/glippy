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

func TestWaitGroupNegativeCounterReportsDefiniteUnderflow(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func direct() {
	var group sync.WaitGroup
	group.Done()
}

func repeated() {
	var group sync.WaitGroup
	group.Add(1)
	group.Done()
	group.Done()
}

func delta() {
	group := new(sync.WaitGroup)
	group.Add(2)
	group.Add(-3)
}
`
	result := runWaitGroupNegativeCounter(t, input)
	want := []string{"group.Done()", "group.Done()", "group.Add(-3)"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("waitgroup-negative-counter result = %#v", result)
	}
	for index, expression := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		occurrence := 0
		if index == 1 {
			occurrence = 2
		}
		start := nthIndex(input, expression, occurrence)
		if diagnostic.RuleID != "waitgroup-negative-counter" ||
			diagnostic.MessageKey != "negative-counter" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expression) ||
			!strings.Contains(diagnostic.Message, "negative") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestWaitGroupNegativeCounterDoesNotReportAfterUnfulfillableWait(t *testing.T) {
	t.Parallel()

	result := runWaitGroupNegativeCounter(
		t,
		`package sample

import "sync"

func blocked() {
	var group sync.WaitGroup
	group.Add(1)
	group.Wait()
	group.Done()
}
`,
	)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("blocked WaitGroup result = %#v", result)
	}
}

func TestWaitGroupNegativeCounterPropagatesAcrossControlFlow(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func branches(flag bool) {
	var group sync.WaitGroup
	if flag {
		group.Add(1)
	} else {
		group.Add(2)
	}
	group.Add(-3)
}

func waitContinuation(flag bool) {
	group := &sync.WaitGroup{}
	if flag {
		group.Add(1)
	}
	group.Wait()
	group.Done()
}

func reinitialized() {
	group := sync.WaitGroup{}
	group.Add(4)
	group = sync.WaitGroup{}
	group.Add(-1)
}
`
	result := runWaitGroupNegativeCounter(t, input)
	want := []string{"group.Add(-3)", "group.Done()", "group.Add(-1)"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("control-flow WaitGroup result = %#v", result)
	}
	for index, expression := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, expression)
		if diagnostic.RuleID != "waitgroup-negative-counter" ||
			diagnostic.MessageKey != "negative-counter" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expression) {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestWaitGroupNegativeCounterRemainsConservative(t *testing.T) {
	t.Parallel()

	result := runWaitGroupNegativeCounter(
		t,
		`package sample

import "sync"

var global sync.WaitGroup

func use(*sync.WaitGroup) {}

func partial(flag bool) {
	var group sync.WaitGroup
	if flag {
		group.Add(1)
	}
	group.Done()
}

func alias() {
	var group sync.WaitGroup
	other := &group
	_ = other
	group.Done()
}

func escapedReinitialization() {
	var group sync.WaitGroup
	other := &group
	group = sync.WaitGroup{}
	other.Add(1)
	group.Done()
}

func escapedDuringReinitialization() {
	var group sync.WaitGroup
	var other *sync.WaitGroup
	group, other = sync.WaitGroup{}, &group
	other.Add(1)
	group.Done()
}

func escapedAfterWait() {
	var group sync.WaitGroup
	other := &group
	group.Wait()
	other.Add(1)
	group.Done()
}

func helper() {
	var group sync.WaitGroup
	use(&group)
	group.Done()
}

func sent(out chan<- *sync.WaitGroup) {
	var group sync.WaitGroup
	out <- &group
	group.Done()
}

func methodValue() {
	var group sync.WaitGroup
	add := group.Add
	add(1)
	group.Done()
}

func asynchronous() {
	var group sync.WaitGroup
	go group.Add(1)
	group.Done()
}

func closure() {
	var group sync.WaitGroup
	work := func() { group.Add(1) }
	_ = work
	group.Done()
}

func deferred() {
	var group sync.WaitGroup
	group.Add(1)
	defer group.Done()
	group.Done()
}

func dynamic(delta int) {
	var group sync.WaitGroup
	group.Add(delta)
	group.Done()
}

func large() {
	var group sync.WaitGroup
	group.Add(63)
	group.Add(-64)
}

func parameter(group *sync.WaitGroup) {
	group.Done()
}

func nonlocal() {
	global.Done()
}
`,
	)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conservative WaitGroup result = %#v", result)
	}
}

func TestWaitGroupNegativeCounterMetadataAndEligibility(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("waitgroup-negative-counter")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		metadata.RequiresEffectFacts ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf("waitgroup-negative-counter metadata = %#v, found = %v", metadata, found)
	}

	suppressed := runWaitGroupNegativeCounter(
		t,
		`package sample
import "sync"
func run() {
	var group sync.WaitGroup
	//glippy:ignore waitgroup-negative-counter -- compatibility probe
	group.Done()
}
`,
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed WaitGroup result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
import "sync"
func run() { var group sync.WaitGroup; group.Done() }
`,
			"type-error": `package sample
import "sync"
func run() { var group sync.WaitGroup; missing(); group.Done() }
`,
		} {
		result := runWaitGroupNegativeCounter(t, input)
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
			t.Fatalf("%s WaitGroup result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf("type-error result has no load diagnostics: %#v", result)
		}
	}

	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"waitgroup-negative-counter": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 WaitGroup selection = %#v", older)
	}
}

func BenchmarkWaitGroupNegativeCounterPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/waitgroupbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"sync\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() { var group sync.WaitGroup; group.Done() }\n",
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
			Overrides: map[string]rules.Severity{
				"waitgroup-negative-counter": rules.SeverityWarn,
			},
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

func runWaitGroupNegativeCounter(t testing.TB, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/waitgroupstate\n\ngo 1.25.0\n",
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
				"waitgroup-negative-counter": rules.SeverityWarn,
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
	return result
}
