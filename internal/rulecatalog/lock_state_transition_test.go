package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestLockStateRulesReportPathProvenDefects(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"sync"
	"time"
)

func missingRelease(mu *sync.Mutex, fail bool) {
	mu.Lock()
	if fail {
		return
	}
	mu.Unlock()
}

type guarded struct {
	mu sync.RWMutex
}

func deferredDoubleUnlock(value *guarded, fail bool) {
	value.mu.RLock()
	defer value.mu.RUnlock()
	if fail {
		value.mu.RUnlock()
		return
	}
}

func unmatchedLocalUnlock() {
	var mu sync.Mutex
	mu.Unlock()
}

func wrongUnlockMode(mu *sync.RWMutex) {
	mu.RLock()
	mu.Unlock()
}

func doubleUnlock(mu *sync.Mutex) {
	mu.Lock()
	mu.Unlock()
	mu.Unlock()
}

func blockingAcrossBranch(mu *sync.Mutex, ready bool) {
	mu.Lock()
	if ready {
		_ = ready
	}
	time.Sleep(time.Second)
	mu.Unlock()
}
`
	result := runLockStateRules(
		t,
		input,
		"lock-held-across-blocking-call",
		"lock-not-released",
		"unlock-without-lock",
	)
	want := []struct {
		ruleID string
		occurrence string
		index int
	}{
		{"lock-not-released", "return", 0},
		{"unlock-without-lock", "mu.Unlock()", 1},
		{"unlock-without-lock", "mu.Unlock()", 2},
		{"unlock-without-lock", "mu.Unlock()", 4},
		{"lock-held-across-blocking-call", "time.Sleep(time.Second)", 0},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("lock state diagnostics = %#v", result)
	}
	for diagnosticIndex, expected := range want {
		diagnostic := result.Files[0].Diagnostics[diagnosticIndex]
		start := nthIndex(input, expected.occurrence, expected.index)
		if start < 0 {
			t.Fatalf("missing occurrence %q[%d]", expected.occurrence, expected.index)
		}
		if diagnostic.RuleID != expected.ruleID ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expected.occurrence) {
			t.Fatalf("diagnostic[%d] = %#v", diagnosticIndex, diagnostic)
		}
	}
}

func TestLockStateRulesRemainConservativeForValidAndUnknownState(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"sync"
	"time"
)

func completeRelease(mu *sync.Mutex, fail bool) {
	mu.Lock()
	if fail {
		mu.Unlock()
		return
	}
	mu.Unlock()
}

func deferredRelease(mu *sync.RWMutex) {
	mu.RLock()
	defer mu.RUnlock()
}

func reassignedDeferredReceiver(mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()
	mu = new(sync.Mutex)
}

func balancedNestedReadLocks(mu *sync.RWMutex) {
	mu.RLock()
	mu.RLock()
	mu.RUnlock()
	mu.RUnlock()
}

func boundedDeepReadLocks(mu *sync.RWMutex) {
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RLock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
	mu.RUnlock()
}

func unknownParameterState(mu *sync.Mutex) {
	mu.Unlock()
}

func unknownReadDepth(mu *sync.RWMutex) {
	mu.RLock()
	mu.RUnlock()
	mu.RUnlock()
}

type guarded struct {
	mu sync.RWMutex
}

func (value *guarded) readWithEarlyReturn(closed bool) {
	value.mu.RLock()
	defer value.mu.RUnlock()
	if closed {
		return
	}
	value.inspect()
}

func (*guarded) inspect() {}

func unknownFieldState(value *guarded) {
	value.mu.RUnlock()
}

func handoff(*sync.Mutex) {}

func transferred(mu *sync.Mutex) {
	mu.Lock()
	handoff(mu)
}

func deferredAfterEscape(mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()
	handoff(mu)
	mu.Unlock()
}

func ambiguousAfterLongEscape(ready, other bool) {
	var mu sync.Mutex
	if !ready {
		if other {
			_ = other
		}
		handoff(&mu)
	}
	mu.Unlock()
}

func loop(mu *sync.RWMutex, count int) {
	for index := 0; index < count; index++ {
		mu.RLock()
		if index == 0 {
			mu.RUnlock()
			continue
		}
		mu.RUnlock()
	}
}

func noReturn(mu *sync.Mutex) {
	mu.Lock()
	panic("stop")
}

func conditionWait(mu *sync.Mutex, condition *sync.Cond) {
	mu.Lock()
	condition.Wait()
	mu.Unlock()
}

func indexedReceiver(locks []sync.Mutex) {
	locks[0].Lock()
}

func computedReceiver() {
	new(sync.Mutex).Lock()
}

func deferredBlocking(mu *sync.Mutex) {
	mu.Lock()
	defer time.Sleep(time.Second)
	mu.Unlock()
}

func asynchronousBlocking(mu *sync.Mutex) {
	mu.Lock()
	go time.Sleep(time.Second)
	mu.Unlock()
}

type lookalike struct{}

func (*lookalike) Lock() {}
func (*lookalike) Unlock() {}

func ignored(value *lookalike) {
	value.Lock()
	value.Unlock()
}
`
	result := runLockStateRules(
		t,
		input,
		"lock-held-across-blocking-call",
		"lock-not-released",
		"unlock-without-lock",
	)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("lock state conservative result = %#v", result)
	}
}

func TestLockStateRuleMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id string
		presets []rules.Preset
		requiresEffects bool
	}{
		{
			id: "lock-held-across-blocking-call",
			presets: []rules.Preset{rules.PresetSuspicious},
			requiresEffects: true,
		},
		{id: "lock-not-released", presets: []rules.Preset{rules.PresetSuspicious}},
		{id: "unlock-without-lock", presets: []rules.Preset{rules.PresetCorrectness}},
	}
	for _, test := range tests {
		metadata, found := registry.Metadata(test.id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(metadata.Presets, test.presets) ||
			metadata.Requirement != rules.RequireControlFlow ||
			metadata.RequiresEffectFacts != test.requiresEffects ||
			len(metadata.NodeInterests) != 0 ||
			len(metadata.Fixes) != 0 {
			t.Fatalf("%s metadata = %#v, found = %v", test.id, metadata, found)
		}
	}
}

func TestLockHeldAcrossBlockingCallUsesProjectContract(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func waitForProject() {}

func run(mu *sync.Mutex) {
	mu.Lock()
	waitForProject()
	mu.Unlock()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/lockcontract\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/lockcontract.waitForProject"
blocking = true
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
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
				"lock-held-across-blocking-call": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
			Contracts: set,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("contract blocking result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	wantStart := strings.LastIndex(input, "waitForProject()")
	if diagnostic.RuleID != "lock-held-across-blocking-call" ||
		diagnostic.Range.Start != wantStart ||
		diagnostic.Range.End != wantStart + len("waitForProject()") {
		t.Fatalf("contract blocking diagnostic = %#v", diagnostic)
	}
}

func TestLockStateRulesHonorGeneratedTypeErrorAndSuppressionPolicy(t *testing.T) {
	t.Parallel()

	t.Run(
		"generated",
		func(t *testing.T) {
			t.Parallel()
			result := runLockStateRules(
				t,
				`// Code generated by test. DO NOT EDIT.
package sample
import "sync"
func invalid() { var mu sync.Mutex; mu.Unlock() }
`,
				"unlock-without-lock",
			)
			if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
				t.Fatalf("generated lock result = %#v", result)
			}
		},
	)
	t.Run(
		"type-error",
		func(t *testing.T) {
			t.Parallel()
			result := runLockStateRules(
				t,
				`package sample
import "sync"
func invalid() { missing(); var mu sync.Mutex; mu.Unlock() }
`,
				"unlock-without-lock",
			)
			if len(result.LoadDiagnostics) == 0 ||
				len(result.Files) != 1 ||
				len(result.Files[0].Diagnostics) != 0 {
				t.Fatalf("type-error lock result = %#v", result)
			}
		},
	)
	t.Run(
		"suppressed",
		func(t *testing.T) {
			t.Parallel()
			result := runLockStateRules(
				t,
				`package sample
import "sync"
func invalid() {
	var mu sync.Mutex
	//glippy:ignore unlock-without-lock -- external lock protocol
	mu.Unlock()
}
`,
				"unlock-without-lock",
			)
			if len(result.Files) != 1 ||
				len(result.Files[0].Diagnostics) != 0 ||
				len(result.Files[0].Suppressed) != 1 {
				t.Fatalf("suppressed lock result = %#v", result)
			}
		},
	)
}

func runLockStateRules(t *testing.T, input string, ruleIDs ...string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/lockstate\n\ngo 1.25.0\n",
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
			Presets: []rules.Preset{},
			Overrides: overrides,
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

func BenchmarkLockStateTransitionPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/lockstatebenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport (\"sync\"; \"time\")\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(mu *sync.Mutex, fail bool) { mu.Lock(); if fail { return }; time.Sleep(time.Second); mu.Unlock(); var local sync.Mutex; local.Unlock() }\n",
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
				"lock-held-across-blocking-call": rules.SeverityWarn,
				"lock-not-released": rules.SeverityWarn,
				"unlock-without-lock": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		300,
	)
}

func nthIndex(input, text string, occurrence int) int {
	start := 0
	for index := 0; index <= occurrence; index++ {
		relative := strings.Index(input[start:], text)
		if relative < 0 {
			return -1
		}
		start += relative
		if index == occurrence {
			return start
		}
		start += len(text)
	}
	return -1
}
