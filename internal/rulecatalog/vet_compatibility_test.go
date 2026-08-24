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

func TestInitialVetCompatibilityPackReportsDefects(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
)

type payload struct {
	Value string ` +
		"`json:\"value`" +
		`
}

type writer struct{}

func (writer) WriteTo(io.Writer) error { return nil }

func decode(data []byte) {
	var value payload
	json.Unmarshal(data, value)
}

func printValue() {
	fmt.Printf("%d", "text")
}

func launch() {
	var group sync.WaitGroup
	go func() {
		group.Add(1)
		defer group.Done()
	}()
	group.Wait()
}

func failFromWorker(t *testing.T) {
	go func() {
		t.Fatal("worker failed")
	}()
}
`
	result := runVetCompatibilityRules(
		t,
		input,
		[]string{
			"printf-arguments",
			"invalid-struct-tag",
			"invalid-unmarshal-target",
			"waitgroup-misuse",
			"testing-goroutine-call",
			"standard-method-signature",
		},
	)
	if len(result.Files) != 1 {
		t.Fatalf("vet compatibility files = %#v", result.Files)
	}
	type expectedDiagnostic struct {
		start int
		text string
	}
	want := map[string]expectedDiagnostic{
		"invalid-struct-tag": {start: strings.Index(input, "Value string"), text: "Value"},
		"invalid-unmarshal-target": {
			start: strings.Index(input, "json.Unmarshal(data, value)") +
				len("json.Unmarshal"),
		},
		"printf-arguments": {start: strings.Index(input, "%d"), text: "%d"},
		"standard-method-signature": {
			start: strings.Index(input, "WriteTo"),
			text: "WriteTo",
		},
		"testing-goroutine-call": {
			start: strings.Index(input, `t.Fatal("worker failed")`),
			text: `t.Fatal("worker failed")`,
		},
		"waitgroup-misuse": {
			start: strings.Index(input, "group.Add(1)") + len("group.Add"),
		},
	}
	got := make(map[string]struct{}, len(result.Files[0].Diagnostics))
	for _, diagnostic := range result.Files[0].Diagnostics {
		expected, found := want[diagnostic.RuleID]
		if !found {
			t.Fatalf("unexpected vet compatibility diagnostic = %#v", diagnostic)
		}
		if diagnostic.Range.Start != expected.start ||
			diagnostic.Range.End != expected.start + len(expected.text) {
			t.Fatalf(
				"%s range = %#v, want start %d text %q",
				diagnostic.RuleID,
				diagnostic.Range,
				expected.start,
				expected.text,
			)
		}
		got[diagnostic.RuleID] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("vet compatibility rules = %#v, want %#v", got, want)
	}
}

func TestInitialVetCompatibilityPackExcludesNearbyValidCode(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
)

type payload struct {
	Value string ` +
		"`json:\"value\"`" +
		`
}

type writer struct{}

func (writer) WriteTo(io.Writer) (int64, error) { return 0, nil }

func decode(data []byte) {
	var value payload
	_ = json.Unmarshal(data, &value)
}

func printValue() {
	fmt.Printf("%s", "text")
}

func launch() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
	}()
	group.Wait()
}

func failInTest(t *testing.T) {
	t.Fatal("test failed")
}
`
	result := runVetCompatibilityRules(
		t,
		input,
		[]string{
			"printf-arguments",
			"invalid-struct-tag",
			"invalid-unmarshal-target",
			"waitgroup-misuse",
			"testing-goroutine-call",
			"standard-method-signature",
		},
	)
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("valid vet compatibility result = %#v", result)
	}
}

func TestWaitGroupMisuseReportsIndirectAddOrdering(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func addInside(group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
}

func launchInside(group *sync.WaitGroup) {
	addInside(group)
}

func launch() {
	var group sync.WaitGroup
	go launchInside(&group)
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("indirect WaitGroup result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	wantStart := strings.Index(input, "group.Add(1)") + len("group.Add")
	if diagnostic.RuleID != "waitgroup-misuse" ||
		diagnostic.Range.Start != wantStart ||
		diagnostic.Range.End != wantStart {
		t.Fatalf("indirect WaitGroup diagnostic = %#v", diagnostic)
	}
}

func TestWaitGroupMisuseReportsMethodExpressionAddOrdering(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func addInside(group *sync.WaitGroup) {
	(*sync.WaitGroup).Add(group, 1)
}

func launch() {
	var group sync.WaitGroup
	go (*sync.WaitGroup).Add(&group, 1)
	go addInside(&group)
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 2 {
		t.Fatalf("method-expression WaitGroup result = %#v", result)
	}
}

func TestWaitGroupMisuseAllowsNegativeAddInsideGoroutine(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func finish(group *sync.WaitGroup) {
	group.Add(-1)
}

func noChange(group *sync.WaitGroup) {
	group.Add(0)
}

func change(group *sync.WaitGroup, delta int) {
	group.Add(delta)
}

func launch() {
	var group sync.WaitGroup
	group.Add(1)
	go finish(&group)
	go noChange(&group)
	go change(&group, -1)
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("negative WaitGroup.Add result = %#v", result)
	}
}

func TestWaitGroupMisuseAllowsNestedRegistrationWithEstablishedCounter(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		group.Add(1)
		go func() { group.Done() }()
		group.Done()
	}()
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("established WaitGroup counter result = %#v", result)
	}
}

func TestWaitGroupMisuseAllowsLabeledEstablishedCounter(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	if false { goto registered }
registered:
	group.Add(1)
	go func() {
		group.Add(1)
		defer group.Done()
	}()
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("labeled established WaitGroup counter result = %#v", result)
	}
}

func TestWaitGroupMisuseReportsAfterEstablishedCounterIsBalanced(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	group.Add(1)
	group.Done()
	go func() {
		group.Add(1)
		defer group.Done()
	}()
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 1 {
		t.Fatalf("balanced WaitGroup counter result = %#v", result)
	}
}

func TestWaitGroupMisuseIgnoresUnreachableWait(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	go func() { group.Add(1) }()
	return
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("unreachable WaitGroup wait result = %#v", result)
	}
}

func TestWaitGroupMisuseIgnoresUnreachableLaunch(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	return
	go func() { group.Add(1) }()
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("unreachable WaitGroup launch result = %#v", result)
	}
}

func TestWaitGroupMisuseIgnoresLaunchAfterTerminalControlFlow(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func returnBeforeLaunch() {
	var group sync.WaitGroup
	if true { return }
	go func() { group.Add(1) }()
	group.Wait()
}

func branchBeforeLaunch() {
	var group sync.WaitGroup
	if true { goto done }
	go func() { group.Add(1) }()
done:
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("terminal control-flow WaitGroup launch result = %#v", result)
	}
}

func TestWaitGroupMisuseIgnoresLaunchAfterUnprovenNegativeCounter(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	group.Done()
	go func() { group.Add(1) }()
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("negative pre-launch WaitGroup result = %#v", result)
	}
}

func TestWaitGroupMisuseRequiresDirectReachableLaunchAndWait(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func unreachableNestedLaunch() {
	var group sync.WaitGroup
	if false {
		go func() { group.Add(1) }()
		group.Wait()
	}
}

func unreachableWait() {
	var group sync.WaitGroup
	go func() { group.Add(1) }()
	if true { return }
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("non-direct WaitGroup launch or wait result = %#v", result)
	}
}

func TestWaitGroupMisuseIgnoresWaitAfterLabeledReturn(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	if false { goto finished }
	go func() { group.Add(1) }()
finished:
	return
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("WaitGroup wait after labeled return result = %#v", result)
	}
}

func TestWaitGroupMisuseReportsParameterizedFunctionLiteral(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	go func(current *sync.WaitGroup) {
		current.Add(1)
		defer current.Done()
	}(&group)
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 1 {
		t.Fatalf("parameterized function literal result = %#v", result)
	}
}

func TestWaitGroupMisuseAllowsHelperOwnedWaitGroup(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

var group sync.WaitGroup

func work() {
	group.Add(1)
	go func() { group.Done() }()
	group.Wait()
}

func launch() {
	go work()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("helper-owned WaitGroup result = %#v", result)
	}
}

func TestWaitGroupMisuseAllowsFunctionLiteralOwnedWaitGroup(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	go func() {
		group.Add(1)
		go func() { group.Done() }()
		group.Wait()
	}()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("function-literal-owned WaitGroup result = %#v", result)
	}
}

func TestWaitGroupMisuseRejectsReassignedWaitReceiverIdentity(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func work(group *sync.WaitGroup) {
	group.Add(1)
}

func launch() {
	var first sync.WaitGroup
	var second sync.WaitGroup
	group := &first
	go work(group)
	group = &second
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("reassigned WaitGroup receiver result = %#v", result)
	}
}

func TestWaitGroupMisuseRemainsConservativeAcrossSynchronization(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func addAfterReady(group *sync.WaitGroup, ready <-chan struct{}) {
	<-ready
	group.Add(1)
}

func launch() {
	var group sync.WaitGroup
	ready := make(chan struct{})
	go addAfterReady(&group, ready)
	group.Add(1)
	close(ready)
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("synchronized WaitGroup result = %#v", result)
	}
}

func TestWaitGroupMisuseAllowsCallerSynchronizationBeforeWait(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func launch() {
	var group sync.WaitGroup
	added := make(chan struct{})
	go func() {
		group.Add(1)
		close(added)
	}()
	<-added
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("caller-synchronized WaitGroup result = %#v", result)
	}
}

func TestWaitGroupMisuseRemainsConservativeWhenLaunchArgumentsSynchronize(t *testing.T) {
	t.Parallel()

	input := `package sample

import "sync"

func addInside(ready struct{}, group *sync.WaitGroup) {
	group.Add(1)
}

func launch(ready <-chan struct{}) {
	var group sync.WaitGroup
	go addInside(<-ready, &group)
	group.Wait()
}
`
	result := runVetCompatibilityRules(t, input, []string{"waitgroup-misuse"})
	if countPackageDiagnostics(result) != 0 {
		t.Fatalf("synchronized launch argument result = %#v", result)
	}
}

func TestInitialVetCompatibilityPackMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	wantCorrectness := []string{
		"invalid-struct-tag",
		"invalid-unmarshal-target",
		"printf-arguments",
		"standard-method-signature",
		"testing-goroutine-call",
		"waitgroup-misuse",
	}
	for _, id := range wantCorrectness {
		metadata, found := registry.Metadata(id)
		if !found {
			t.Fatalf("product registry does not include %s", id)
		}
		if metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(
				metadata.Presets,
				[]rules.Preset{rules.PresetCorrectness},
			) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != rules.RequireTypes ||
			metadata.RunOnGenerated {
			t.Fatalf("%s metadata = %#v", id, metadata)
		}
		if id == "waitgroup-misuse" {
			if len(metadata.NodeInterests) != 0 {
				t.Fatalf("%s metadata = %#v", id, metadata)
			}
		} else if !reflect.DeepEqual(
			metadata.NodeInterests,
			[]rules.NodeKind{rules.NodeFile},
		) {
			t.Fatalf("%s metadata = %#v", id, metadata)
		}
	}
	metadata, _ := registry.Metadata("printf-arguments")
	if len(metadata.Fixes) != 1 ||
		metadata.Fixes[0].Name != "insert-string-format" ||
		metadata.Fixes[0].Safety != rules.FixSuggestion {
		t.Fatalf("printf-arguments fixes = %#v", metadata.Fixes)
	}
}

func TestPrintfArgumentsUsesDependencyWrapperFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/printfwrapper\n\ngo 1.25.0\n",
	)
	dependencyPath := filepath.Join(root, "wrapped", "wrapped.go")
	writeFixture(
		t,
		dependencyPath,
		"package wrapped\n\nimport \"fmt\"\n\nfunc Warnf(format string, arguments ...any) { fmt.Printf(format, arguments...) }\n",
	)
	input := "package app\n\nimport \"example.com/printfwrapper/wrapped\"\n\nfunc run() { wrapped.Warnf(\"%d\", \"text\") }\n"
	path := filepath.Join(root, "app", "app.go")
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
				"printf-arguments": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./app"},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "printf-arguments" {
		t.Fatalf("printf wrapper fact result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	if input[diagnostic.Range.Start:diagnostic.Range.End] != "%d" {
		t.Fatalf("printf wrapper range = %#v", diagnostic.Range)
	}
	rootSource, found := result.Sources.Lookup(path)
	if !found || len(rootSource.Tokens()) == 0 {
		t.Fatalf("root source is not fully indexed")
	}
	if _, found := result.Sources.Lookup(dependencyPath); found {
		t.Fatalf("dependency source %q was retained", dependencyPath)
	}
	if paths := result.Sources.Paths(); !reflect.DeepEqual(paths, []string{path}) {
		t.Fatalf("retained reporter sources = %q, want only %q", paths, path)
	}
}

func TestUnreachableCodeUsesImportedTestingSkipWrapperFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/skipwrapper\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "wrapped", "wrapped.go"),
		`package wrapped

import "testing"

func Skip(t *testing.T) {
	t.Skip("disabled test")
	panic("unreachable")
}
`,
	)
	input := `package app

import (
	"testing"

	"example.com/skipwrapper/wrapped"
)

func skipped(t *testing.T) {
	wrapped.Skip(t)
	println("intentionally retained test body")
}
`
	path := filepath.Join(root, "app", "app.go")
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
				"unreachable-code": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./app"},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("imported testing skip wrapper result = %#v", result)
	}
}

func TestRemainingVetCompatibilityPackReportsDefects(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"fmt"
	"log/slog"
	"net"
	"time"
)

func target() {}

func defects(value uint8, number int, items []int, host string, start time.Time) {
	_ = value << 8
	_ = string(number)
	if target == nil {}
	_ = append(items)
	slog.Info("message", "key")
	fmt.Sprintf("%s", "discarded")
	_, _ = net.Dial("tcp", fmt.Sprintf("%s:%d", host, 80))
	defer fmt.Println(time.Since(start))
	return
	fmt.Println("unreachable")
}
`
	ruleIDs := []string{
		"oversized-shift",
		"suspicious-string-conversion",
		"nil-function-comparison",
		"append-no-values",
		"invalid-slog-arguments",
		"unused-result",
		"unreachable-code",
		"unsafe-host-port",
		"deferred-time-since",
	}
	result := runVetCompatibilityRules(t, input, ruleIDs)
	if len(result.Files) != 1 {
		t.Fatalf("remaining vet compatibility files = %#v", result.Files)
	}
	got := make(map[string]int)
	for _, diagnostic := range result.Files[0].Diagnostics {
		got[diagnostic.RuleID]++
		wantFixes := map[string][]string{
			"suspicious-string-conversion": {
				"convert-single-rune",
				"format-number-decimal",
			},
			"unreachable-code": {"remove-unreachable-code"},
			"unsafe-host-port": {"use-net-join-host-port"},
		}[diagnostic.RuleID]
		if len(diagnostic.Fixes) != len(wantFixes) {
			t.Fatalf("%s diagnostic fixes = %#v", diagnostic.RuleID, diagnostic.Fixes)
		}
		for index, name := range wantFixes {
			if diagnostic.Fixes[index].Name != name ||
				diagnostic.Fixes[index].Safety != rules.FixSuggestion ||
				len(diagnostic.Fixes[index].Edits) == 0 {
				t.Fatalf(
					"%s diagnostic fix[%d] = %#v",
					diagnostic.RuleID,
					index,
					diagnostic.Fixes[index],
				)
			}
		}
	}
	for _, ruleID := range ruleIDs {
		if got[ruleID] != 1 {
			t.Fatalf(
				"%s diagnostic count = %d; result = %#v",
				ruleID,
				got[ruleID],
				result,
			)
		}
	}
	if len(got) != len(ruleIDs) {
		t.Fatalf(
			"remaining vet compatibility diagnostics = %#v",
			result.Files[0].Diagnostics,
		)
	}
}

func TestUnreachableCodePreservesSharedCFGControlFlowBoundaries(t *testing.T) {
	t.Parallel()

	input := `package sample

import "testing"

func sideEffect() int { return 1 }

func afterReturn() {
	return
	println("after return")
	println("same dead region")
}

func deadControl(condition bool) {
	return
	if condition {
		println("dead parent")
	}
}

func nested(condition bool) {
	if condition {
		return
		println("dead branch")
	}
	println("reachable after if")
}

func loops() {
	for {
	}
	println("after loop")
}

func breakable() {
	for {
		break
	}
	println("after break")
}

func labels() {
	goto live
	println("between goto and label")
live:
	println("label target")
}

func declaration() {
	return
	var _ = sideEffect()
}

func literal() {
	callback := func() {
		return
		println("dead literal")
	}
	_ = callback
}

func shadowedPanic() {
	panic := func(any) {}
	panic("returns")
	println("after shadowed panic")
}

func requiredReturn(t *testing.T) int {
	t.Fatal("terminal")
	return 0
}

func redundantReturn(t *testing.T) {
	t.Fatal("terminal")
	return
}

func skipped(t *testing.T) {
	t.Skip("disabled test")
	println("intentionally retained test body")
}

func skipInternalf(t *testing.T, format string, args ...any) {
	t.Skipf(format, args...)
}

func skipWrapper(t *testing.T, format string, args ...any) {
	skipInternalf(t, format, args...)
	panic("unreachable")
}

func skippedThroughWrapper(t *testing.T) {
	skipWrapper(t, "disabled test")
	println("intentionally retained wrapped test body")
}

func fatalWrapper(t *testing.T) {
	t.Fatal("terminal")
}

func stoppedByFatalWrapper(t *testing.T) {
	fatalWrapper(t)
	println("after fatal wrapper")
}

func fatalThenSkipWrapper(t *testing.T) {
	t.Fatal("terminal")
	t.Skip("unreachable skip")
}

func stoppedByFatalThenSkipWrapper(t *testing.T) {
	fatalThenSkipWrapper(t)
	println("after fatal-then-skip wrapper")
}

func stop() {
	panic("stop")
}

func stoppedWithSentinel() {
	stop()
	panic("unreachable")
}

func returnedBeforeSentinel() {
	return
	panic("unreachable")
}

func panickedBeforeSentinel() {
	panic("stop")
	panic("unreachable")
}

const branchUnreachable = "unreachable"

func mixedBranchBeforeSentinel(condition bool) {
	if condition {
		stop()
	} else {
		return
	}
	panic(branchUnreachable)
}

func allNoReturnBranchesBeforeSentinel(condition bool) {
	if condition {
		stop()
	} else {
		stop()
	}
	panic("unreachable")
}

const switchUnreachable = "unreachable"

func mixedSwitchBeforeSentinel(value int) {
	switch value {
	case 0:
		stop()
	default:
		return
	}
	panic(switchUnreachable)
}

func requiredPanic(t *testing.T, ready <-chan int) int {
	select {
	case value := <-ready:
		return value
	default:
		t.Fatal("terminal")
	}
	panic("syntactically required")
}
`
	result := runVetCompatibilityRules(t, input, []string{"unreachable-code"})
	want := []string{
		`println("after return")`,
		"if condition {\n\t\tprintln(\"dead parent\")\n\t}",
		`println("dead branch")`,
		`println("after loop")`,
		`println("between goto and label")`,
		"var _ = sideEffect()",
		`println("dead literal")`,
		"return",
		`println("after fatal wrapper")`,
		`t.Skip("unreachable skip")`,
		`println("after fatal-then-skip wrapper")`,
		`panic("unreachable")`,
		`panic("unreachable")`,
		"panic(branchUnreachable)",
		"panic(switchUnreachable)",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unreachable-code CFG result = %#v", result)
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "unreachable-code" ||
			diagnostic.Range.Start < 0 ||
			diagnostic.Range.End > len(input) ||
			string(input[diagnostic.Range.Start:diagnostic.Range.End]) != want[index] ||
			len(diagnostic.Fixes) != 1 ||
			diagnostic.Fixes[0].Safety != rules.FixSuggestion {
			t.Fatalf(
				"unreachable-code diagnostic[%d] = %#v, want %q",
				index,
				diagnostic,
				want[index],
			)
		}
	}
}

func TestUnreachableCodeAcceptsTestingZeroReturnShims(t *testing.T) {
	t.Parallel()

	input := `package sample

import "testing"

func sideEffect() int { return 1 }
func stop() { panic("stop") }

type fatalLookalike struct{}

func (fatalLookalike) Fatal(string) { panic("stop") }

func fromT[Value any](t *testing.T, values <-chan Value) Value {
	select {
	case value := <-values:
		return value
	default:
		t.Fatal("terminal")
		var zero Value
		return zero
	}
}

func fromTB[Value any](t testing.TB) Value {
	t.FailNow()
	var zero Value
	return zero
}

func fromB[Value any](b *testing.B) Value {
	b.Fatalf("terminal")
	var zero Value
	return zero
}

func fromF[Value any](f *testing.F) Value {
	f.Fatal("terminal")
	var zero Value
	return zero
}

func localHelper[Value any]() Value {
	stop()
	var zero Value
	return zero
}

func initialized(t *testing.T) int {
	t.Fatal("terminal")
	var zero = sideEffect()
	return zero
}

func retainedWork(t *testing.T) int {
	t.Fatal("terminal")
	var zero int
	println("not a return shim")
	return zero
}

func noResult(t *testing.T) {
	t.Fatal("terminal")
	var zero int
	_ = zero
}

func lookalike[Value any](fatal fatalLookalike) Value {
	fatal.Fatal("terminal")
	var zero Value
	return zero
}

func workAfterShim[Value any](t *testing.T) Value {
	t.Fatal("terminal")
	var zero Value
	return zero
	println("still dead")
	return zero
}

func emptyDeclaration(t *testing.T) (zero int) {
	t.Fatal("terminal")
	var ()
	return
}
`
	result := runVetCompatibilityRules(t, input, []string{"unreachable-code"})
	if len(result.LoadDiagnostics) != 0 {
		t.Fatalf(
			"compiler-required return shim failed to load: %#v",
			result.LoadDiagnostics,
		)
	}
	want := []string{
		"var zero Value",
		"var zero = sideEffect()",
		"var zero int",
		"var zero int",
		"var zero Value",
		"var zero Value",
		`println("still dead")`,
		"var ()",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("unreachable-code zero return shims = %#v", result)
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.Range.Start < 0 ||
			diagnostic.Range.End > len(input) ||
			string(input[diagnostic.Range.Start:diagnostic.Range.End]) != want[index] {
			t.Fatalf(
				"unreachable-code diagnostic[%d] = %#v, want %q",
				index,
				diagnostic,
				want[index],
			)
		}
	}
}

func TestRemainingVetCompatibilityPackMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	wantPresets := map[string][]rules.Preset{
		"append-no-values": {rules.PresetCorrectness},
		"deferred-time-since": {rules.PresetCorrectness},
		"invalid-slog-arguments": {rules.PresetCorrectness},
		"nil-function-comparison": {rules.PresetCorrectness},
		"oversized-shift": {rules.PresetCorrectness},
		"suspicious-string-conversion": {rules.PresetSuspicious},
		"unreachable-code": {rules.PresetCorrectness},
		"unsafe-host-port": {rules.PresetCorrectness},
		"unused-result": {rules.PresetCorrectness},
	}
	for id, presets := range wantPresets {
		metadata, found := registry.Metadata(id)
		if !found {
			t.Fatalf("product registry does not include %s", id)
		}
		wantRequirement := rules.RequireTypes
		wantInterests := []rules.NodeKind{rules.NodeFile}
		if id == "unreachable-code" {
			wantRequirement = rules.RequireControlFlow
			wantInterests = nil
		}
		if metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(metadata.Presets, presets) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != wantRequirement ||
			!reflect.DeepEqual(metadata.NodeInterests, wantInterests) ||
			metadata.RunOnGenerated ||
			metadata.RequiresEffectFacts != (id == "unreachable-code") {
			t.Fatalf("%s metadata = %#v", id, metadata)
		}
	}
	wantFixes := map[string][]string{
		"suspicious-string-conversion": {"format-number-decimal", "convert-single-rune"},
		"unreachable-code": {"remove-unreachable-code"},
		"unsafe-host-port": {"use-net-join-host-port"},
	}
	for ruleID, names := range wantFixes {
		metadata, _ := registry.Metadata(ruleID)
		if len(metadata.Fixes) != len(names) {
			t.Fatalf("%s fixes = %#v", ruleID, metadata.Fixes)
		}
		for index, name := range names {
			if metadata.Fixes[index].Name != name ||
				metadata.Fixes[index].Safety != rules.FixSuggestion {
				t.Fatalf("%s fix[%d] = %#v", ruleID, index, metadata.Fixes[index])
			}
		}
	}
}

func TestVetCompatibilityPackHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ruleID string
		input string
		runsDespiteTypeError bool
	}{
		{
			"printf-arguments",
			"package sample\nimport \"fmt\"\nfunc run() { fmt.Printf(\"%d\", \"text\") }\n",
			false,
		},
		{
			"invalid-struct-tag",
			"package sample\ntype value struct { Field string `json:\"field` }\n",
			true,
		},
		{
			"invalid-unmarshal-target",
			"package sample\nimport \"encoding/json\"\nfunc run(data []byte) { var value int; json.Unmarshal(data, value) }\n",
			false,
		},
		{
			"waitgroup-misuse",
			"package sample\nimport \"sync\"\nfunc run() { var group sync.WaitGroup; go func() { group.Add(1) }(); group.Wait() }\n",
			false,
		},
		{
			"testing-goroutine-call",
			"package sample\nimport \"testing\"\nfunc run(t *testing.T) { go func() { t.Fatal(\"failed\") }() }\n",
			false,
		},
		{
			"standard-method-signature",
			"package sample\nimport \"io\"\ntype value struct{}\nfunc (value) WriteTo(io.Writer) error { return nil }\n",
			false,
		},
		{
			"oversized-shift",
			"package sample\nfunc run(value uint8) { _ = value << 8 }\n",
			false,
		},
		{
			"suspicious-string-conversion",
			"package sample\nfunc run(value int) { _ = string(value) }\n",
			false,
		},
		{
			"nil-function-comparison",
			"package sample\nfunc target() {}\nfunc run() { _ = target == nil }\n",
			false,
		},
		{
			"append-no-values",
			"package sample\nfunc run(values []int) { _ = append(values) }\n",
			false,
		},
		{
			"invalid-slog-arguments",
			"package sample\nimport \"log/slog\"\nfunc run() { slog.Info(\"message\", \"key\") }\n",
			false,
		},
		{
			"unused-result",
			"package sample\nimport \"fmt\"\nfunc run() { fmt.Sprintf(\"value\") }\n",
			false,
		},
		{
			"unreachable-code",
			"package sample\nfunc run() { return; println(\"unreachable\") }\n",
			true,
		},
		{
			"unsafe-host-port",
			"package sample\nimport (\"fmt\"; \"net\")\nfunc run(host string) { _, _ = net.Dial(\"tcp\", fmt.Sprintf(\"%s:%d\", host, 80)) }\n",
			false,
		},
		{
			"deferred-time-since",
			"package sample\nimport (\"fmt\"; \"time\")\nfunc run(start time.Time) { defer fmt.Println(time.Since(start)) }\n",
			false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(
			test.ruleID,
			func(t *testing.T) {
				t.Parallel()
				suppressed := "//glippy:ignore-file " +
					test.ruleID +
					" -- reviewed fixture\n" +
					test.input
				suppressedResult := runVetCompatibilityRules(
					t,
					suppressed,
					[]string{test.ruleID},
				)
				if len(suppressedResult.Files) != 1 ||
					len(suppressedResult.Files[0].Diagnostics) != 0 ||
					len(suppressedResult.Files[0].Suppressed) != 1 ||
					suppressedResult.Files[0].Suppressed[0].Diagnostic.RuleID !=
						test.ruleID {
					t.Fatalf(
						"%s suppressed result = %#v",
						test.ruleID,
						suppressedResult,
					)
				}

				generated := "// Code generated by fixture. DO NOT EDIT.\n" +
					test.input
				generatedResult := runVetCompatibilityRules(
					t,
					generated,
					[]string{test.ruleID},
				)
				if countPackageDiagnostics(generatedResult) != 0 {
					t.Fatalf(
						"%s generated result = %#v",
						test.ruleID,
						generatedResult,
					)
				}

				illTyped := test.input +
					"\nfunc glippyTypeError() { var invalid string = 1; _ = invalid }\n"
				illTypedResult := runVetCompatibilityRules(
					t,
					illTyped,
					[]string{test.ruleID},
				)
				want := 0
				if test.runsDespiteTypeError {
					want = 1
				}
				if countPackageDiagnostics(illTypedResult) != want ||
					len(illTypedResult.LoadDiagnostics) == 0 {
					t.Fatalf(
						"%s ill-typed result = %#v, want %d diagnostics",
						test.ruleID,
						illTypedResult,
						want,
					)
				}
			},
		)
	}

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	overrides := make(map[string]rules.Severity, len(tests))
	for _, test := range tests {
		overrides[test.ruleID] = rules.SeverityWarn
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("pre-minimum vet compatibility selection = %#v", selection)
	}
}

func BenchmarkInitialVetCompatibilityPack(b *testing.B) {
	for _, ruleID := range
		[]string{
			"printf-arguments",
			"invalid-struct-tag",
			"invalid-unmarshal-target",
			"waitgroup-misuse",
			"testing-goroutine-call",
			"standard-method-signature",
			"oversized-shift",
			"suspicious-string-conversion",
			"nil-function-comparison",
			"append-no-values",
			"invalid-slog-arguments",
			"unused-result",
			"unreachable-code",
			"unsafe-host-port",
			"deferred-time-since",
		} {
		ruleID := ruleID
		b.Run(
			ruleID,
			func(b *testing.B) {
				root := b.TempDir()
				writeFixture(
					b,
					filepath.Join(root, "go.mod"),
					"module example.com/vetpackbench\n\ngo 1.25.0\n",
				)
				writeFixture(
					b,
					filepath.Join(root, "sample.go"),
					vetCompatibilityBenchmarkInput,
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
							ruleID: rules.SeverityWarn,
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
			},
		)
	}
}

const vetCompatibilityBenchmarkInput = `package sample

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

type payload struct { Value string ` +
	"`json:\"value`" +
	` }
type writer struct{}
func (writer) WriteTo(io.Writer) error { return nil }
func target() {}

func defects(data []byte, value uint8, number int, items []int, host string, start time.Time, t *testing.T) {
	var decoded payload
	json.Unmarshal(data, decoded)
	fmt.Printf("%d", "text")
	var group sync.WaitGroup
	go func() { group.Add(1); defer group.Done() }()
	group.Wait()
	go func() { t.Fatal("worker failed") }()
	_ = value << 8
	_ = string(number)
	if target == nil {}
	_ = append(items)
	slog.Info("message", "key")
	fmt.Sprintf("%s", "discarded")
	_, _ = net.Dial("tcp", fmt.Sprintf("%s:%d", host, 80))
	defer fmt.Println(time.Since(start))
	return
	fmt.Println("unreachable")
}
`

func runVetCompatibilityRules(t *testing.T, input string, ruleIDs []string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/vetpack\n\ngo 1.25.0\n")
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
