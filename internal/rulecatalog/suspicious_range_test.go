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

func TestSuspiciousRangeReportsMutationOfCopiedValues(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct {
	enabled bool
	index int
	slots [2]bool
}

func badSlice(values []item) {
	for _, value := range values {
		value.enabled = true
	}
}

func badMap(values map[string]item) {
	for _, value := range values {
		value.enabled = true
	}
}

func goodIndex(values []item) {
	for index := range values {
		values[index].enabled = true
	}
}

func goodPointers(values []*item) {
	for _, value := range values {
		value.enabled = true
	}
}

func goodRead(values []item) bool {
	for _, value := range values {
		if value.enabled { return true }
	}
	return false
}

func goodWriteBack(values []item) {
	for index, value := range values {
		value.enabled = true
		values[index] = value
	}
}

func goodAppend(values []item) []item {
	var copied []item
	for _, value := range values {
		value.enabled = true
		copied = append(copied, value)
	}
	return copied
}

func goodProjection(values []item) []bool {
	var enabled []bool
	for _, value := range values {
		value.enabled = true
		enabled = append(enabled, value.enabled)
	}
	return enabled
}

func goodCapturedUse(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		if inspect() { return true }
	}
	return false
}

func goodCapturedAlias(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		alias := inspect
		value.enabled = true
		if alias() { return true }
	}
	return false
}

func goodConditionalReassignment(values []item, reset bool) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		if reset {
			inspect = func() bool { return false }
		}
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrange\n\ngo 1.25.0\n",
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
				"suspicious-range": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("suspicious-range result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], "value.enabled = true")
		if relative < 0 {
			t.Fatal("missing copied-value mutation")
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "suspicious-range" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("value.enabled") ||
			!strings.Contains(diagnostic.Message, "copy") {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len("value.enabled = true")
	}
}

func TestSuspiciousRangeReportsWhenCapturedValueIsUsedOnlyBeforeMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

	type item struct {
		enabled bool
		index int
		slots [2]bool
	}

func bad(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		_ = inspect()
		value.enabled = true
		inspect = func() bool { return false }
		_ = inspect()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangecapture\n\ngo 1.25.0\n",
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
				"suspicious-range": rules.SeverityWarn,
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
	want := "value.enabled"
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("captured-before-mutation result = %#v", result.Files)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	if content := input[diagnostic.Range.Start:diagnostic.Range.End];
		diagnostic.RuleID != "suspicious-range" || content != want {
		t.Fatalf("captured-before-mutation diagnostic = %#v for %q", diagnostic, content)
	}
}

func TestSuspiciousRangeTracksSimultaneousClosureAliasAssignment(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item) {
	for _, value := range values {
		capturing := func() bool { return value.enabled }
		noncapturing := func() bool { return false }
		inspect := capturing
		inspect, alias := noncapturing, inspect
		value.enabled = true
		_ = alias()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangesimultaneous\n\ngo 1.25.0\n",
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
				"suspicious-range": rules.SeverityWarn,
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
		t.Fatalf("simultaneous closure alias result = %#v", result.Files)
	}
}

func TestSuspiciousRangeIgnoresFunctionValueReferenceAfterMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func bad(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		_ = inspect
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangereference\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeTracksEscapedCapturingClosures(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }
type holder struct { inspect func() bool }

func consume(func() bool) {}

func passed(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		consume(inspect)
	}
}

func returned(values []item) func() bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		return inspect
	}
	return nil
}

func storedField(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		var saved holder
		saved.inspect = inspect
	}
}

func storedContainer(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		saved := make([]func() bool, 1)
		saved[0] = inspect
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeescape\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("escaped capturing closures result = %#v", result.Files)
	}
}

func TestSuspiciousRangeIgnoresConditionalReassignmentWithoutLaterInvocation(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func bad(values []item, reset bool) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		_ = inspect()
		value.enabled = true
		if reset {
			inspect = func() bool { return false }
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeconditional\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeTracksCapturingClosureWrappers(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		wrapper := func() bool { return inspect() }
		value.enabled = true
		if wrapper() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangewrapper\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("capturing wrapper result = %#v", result.Files)
	}
}

func TestSuspiciousRangeTracksPostMutationClosureAliases(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		alias := inspect
		if alias() { return true }
	}
	return false
}

func observedSimultaneous(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		noncapturing := func() bool { return false }
		value.enabled = true
		inspect, alias := noncapturing, inspect
		if alias() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangepostalias\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("post-mutation alias result = %#v", result.Files)
	}
}

func TestSuspiciousRangePropagatesCapturingWrappersToFixpoint(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item) bool {
	for _, value := range values {
		var inspect func() bool
		wrapper := func() bool { return inspect() }
		inspect = func() bool { return value.enabled }
		value.enabled = true
		if wrapper() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangefixpoint\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("capturing wrapper fixpoint result = %#v", result.Files)
	}
}

func TestSuspiciousRangeIgnoresUninvokedPostMutationClosure(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func bad(values []item) {
	for _, value := range values {
		value.enabled = true
		inspect := func() bool { return value.enabled }
		_ = inspect
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeuninvoked\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeHonorsUnconditionalNakedBlockReassignment(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func bad(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		{
			inspect = func() bool { return false }
		}
		_ = inspect()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeblock\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeTracksImmediatelyInvokedWrappers(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		(func() { _ = inspect() })()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeiife\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("immediately invoked wrapper result = %#v", result.Files)
	}
}

func TestSuspiciousRangeTracksDeferredAndLateCapturingWrappers(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func deferred(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		defer inspect()
		value.enabled = true
	}
}

func capturedAfterMutation(values []item) bool {
	for _, value := range values {
		var inspect func() bool
		wrapper := func() bool { return inspect() }
		value.enabled = true
		inspect = func() bool { return value.enabled }
		if wrapper() { return true }
	}
	return false
}

func inlineDeferred(values []item) {
	for _, value := range values {
		defer func() { _ = value.enabled }()
		value.enabled = true
	}
}

func deferredLateCapture(values []item) {
	for _, value := range values {
		var inspect func()
		wrapper := func() { inspect() }
		defer wrapper()
		value.enabled = true
		inspect = func() { _ = value.enabled }
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangelatecapture\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("late captured range use result = %#v", result.Files)
	}
}

func TestSuspiciousRangeEvaluatesDeferredArgumentsBeforeMutation(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func consume(bool) {}

func bad(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		defer consume(inspect())
		value.enabled = true
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangedeferargs\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeSnapshotsDeferredFunctionValues(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		defer inspect()
		inspect = func() {}
		value.enabled = true
	}
}

func bad(values []item) {
	for _, value := range values {
		inspect := func() {}
		defer inspect()
		inspect = func() { _ = value.enabled }
		value.enabled = true
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangedeferstate\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
	diagnostic := result.Files[0].Diagnostics[0]
	if want := strings.LastIndex(input, "value.enabled"); diagnostic.Range.Start != want {
		t.Fatalf("deferred snapshot diagnostic = %#v, want start %d", diagnostic, want)
	}
}

func TestSuspiciousRangeRetractsReassignedWrapperCaptures(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func bad(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		wrapper := func() bool { return inspect() }
		inspect = func() bool { return false }
		value.enabled = true
		_ = wrapper()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangewrapperstate\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeBoundsAdversarialClosureAnalysis(t *testing.T) {
	t.Parallel()

	var input strings.Builder
	input.WriteString("package sample\n\ntype item struct { enabled bool }\n")
	input.WriteString("func bounded(values []item) {\nfor _, value := range values {\n")
	for index := range 80 {
		fmt.Fprintf(
			&input,
			"inspect%d := func() bool { return value.enabled }; _ = inspect%d\n",
			index,
			index,
		)
	}
	for range 80 {
		input.WriteString("value.enabled = true\n")
	}
	input.WriteString("}\n}\n")
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangebounded\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input.String())
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("bounded suspicious-range result = %#v", result.Files)
	}
}

func TestSuspiciousRangeModelsLoopAggregateAndNestedClosures(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }
type holder struct { inspect func() bool }

func keep(callback func() bool) (func() bool, bool) { return callback, true }

func loopBackedge(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return false }
		value.enabled = true
		for index := 0; index < 2; index++ {
			if inspect() { return true }
			inspect = func() bool { return value.enabled }
		}
	}
	return false
}

func selectorStorage(values []item) bool {
	for _, value := range values {
		var saved holder
		saved.inspect = func() bool { return value.enabled }
		value.enabled = true
		if saved.inspect() { return true }
	}
	return false
}

func multiResult(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		inspect, _ = keep(inspect)
		value.enabled = true
		if inspect() { return true }
	}
	return false
}

func nestedUninvoked(values []item) {
	for _, value := range values {
		wrapper := func() { _ = func() bool { return value.enabled } }
		value.enabled = true
		wrapper()
	}
}

func selectorIdentity(values []item) bool {
	for _, value := range values {
		var observed, ignored holder
		observed.inspect = func() bool { return value.enabled }
		ignored.inspect = func() bool { return false }
		value.enabled = true
		if observed.inspect() { return true }
	}
	return false
}

func tupleIntroducesCapture(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return false }
		inspect, _ = keep(func() bool { return value.enabled })
		value.enabled = true
		if inspect() { return true }
	}
	return false
}

func invokedNested(values []item) bool {
	for _, value := range values {
		wrapper := func() bool {
			nested := func() bool { return value.enabled }
			return nested()
		}
		value.enabled = true
		if wrapper() { return true }
	}
	return false
}

func earlierBackedge(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return false }
		for index := 0; index < 2; index++ {
			if inspect() { return true }
			value.enabled = true
			inspect = func() bool { return value.enabled }
		}
	}
	return false
}

func asynchronousObservation(values []item) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		go inspect()
		value.enabled = true
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeflow\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
	nestedStart := strings.Index(input, "func nestedUninvoked")
	if nestedStart < 0 {
		t.Fatal("nestedUninvoked fixture is missing")
	}
	wantStart := nestedStart + strings.Index(input[nestedStart:], "value.enabled = true")
	if got, want := result.Files[0].Diagnostics[0].Range.Start, wantStart; got != want {
		t.Fatalf("nested closure diagnostic start = %d, want %d", got, want)
	}
}

func TestSuspiciousRangeModelsDirectLoopBackedgeReads(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct {
	enabled bool
	index int
	slots [2]bool
}

func observed(values []item) bool {
	for _, value := range values {
		for range 2 {
			if value.enabled { return true }
			value.enabled = true
		}
	}
	return false
}

func observedInCondition(values []item) {
	for _, value := range values {
		for ; !value.enabled; {
			value.enabled = true
		}
	}
}

func observedByPostClosure(values []item) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		for done := false; !done; inspect() {
			value.enabled = true
			done = true
		}
	}
}

func observedByIndex(values []item) {
	for _, value := range values {
		for range 2 {
			value.slots[value.index] = true
			value.index = 1
		}
	}
}

func writesOnly(values []item) {
	for _, value := range values {
		for range 2 {
			value.enabled = false
			value.enabled = true
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangedirectbackedge\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
	if got, want := result.
		Files[0].
		Diagnostics[0].
		Range.
		Start, strings.LastIndex(input, "value.enabled = true");
		got != want {
		t.Fatalf("write-only backedge diagnostic start = %d, want %d", got, want)
	}
}

func TestSuspiciousRangeIgnoresUnreachableLoopBackedges(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func completed(values []item) bool {
	for _, value := range values {
		for {
			if value.enabled { return true }
			value.enabled = true
			break
		}
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeunreachablebackedge\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeIgnoresNestedBreakLoopBackedges(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func completed(values []item, condition bool) {
	for _, value := range values {
		for {
			if value.enabled {
				return
			}
			if condition {
				value.enabled = true
				break
			}
		}
	}
}

func observedAfterSwitch(values []item, condition bool) {
	for _, value := range values {
		for {
			if value.enabled {
				return
			}
			switch {
			case condition:
				value.enabled = true
				break
			}
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangenestedbreak\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeAppliesClosureReplacementBeforeLoopBackedge(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func unobserved(values []item) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		for range 2 {
			inspect()
			value.enabled = true
			inspect = func() {}
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangebackedgereplacement\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeRetainsClosureAcrossConditionalContinue(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item, skip bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		for range 2 {
			inspect()
			value.enabled = true
			if skip {
				continue
			}
			inspect = func() {}
		}
	}
}

func observedLabeled(values []item, skip bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
	outer: for range 2 {
			inspect()
			value.enabled = true
			if skip {
				continue outer
			}
			inspect = func() {}
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeconditionalcontinue\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conditional continue closure result = %#v", result.Files)
	}
}

func TestSuspiciousRangeRetainsClosureAcrossConditionalBreak(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item, stop bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		for range 2 {
			value.enabled = true
			if stop {
				break
			}
			inspect = func() {}
		}
		inspect()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeconditionalbreak\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conditional break closure result = %#v", result.Files)
	}
}

func TestSuspiciousRangeRetainsClosureAcrossClauseBreaks(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observedSwitch(values []item, chosen, stop bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		switch {
		case chosen:
			value.enabled = true
			if stop {
				break
			}
			inspect = func() {}
		}
		inspect()
	}
}

func observedTypeSwitch(values []item, candidate any, stop bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		switch candidate.(type) {
		case int:
			value.enabled = true
			if stop {
				break
			}
			inspect = func() {}
		}
		inspect()
	}
}

func observedSelect(values []item, ready <-chan struct{}, stop bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		select {
		case <-ready:
			value.enabled = true
			if stop {
				break
			}
			inspect = func() {}
		default:
		}
		inspect()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeclausebreaks\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("clause break closure result = %#v", result.Files)
	}
}

func TestSuspiciousRangeRetainsClosureAcrossForwardGoto(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func observed(values []item, skip bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		value.enabled = true
		if skip {
			goto observe
		}
		inspect = func() {}
	observe:
		inspect()
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeforwardgoto\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("forward goto closure result = %#v", result.Files)
	}
}

func TestSuspiciousRangeAppliesClosureReplacementAfterConditionalReturn(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func unobserved(values []item, stop bool) {
	for _, value := range values {
		inspect := func() { _ = value.enabled }
		for range 2 {
			inspect()
			value.enabled = true
			if stop {
				return
			}
			inspect = func() {}
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeconditionalreturn\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeRejectsUnprovenTupleClosureFlow(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func discard(func() bool) (func() bool, bool) {
	return func() bool { return false }, true
}

func unobserved(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		inspect, _ = discard(inspect)
		value.enabled = true
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangetupleflow\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeRetainsClosureAcrossConditionalTupleAssignment(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func discard(callback func() bool) (func() bool, bool) {
	return func() bool { return false }, true
}

func observed(values []item, replace bool) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		if replace {
			inspect, _ = discard(inspect)
		}
		value.enabled = true
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeconditionaltuple\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conditional tuple closure result = %#v", result.Files)
	}
}

func TestSuspiciousRangeAppliesTupleClosureAssignmentsSimultaneously(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func swap(first, second func() bool) (func() bool, func() bool) {
	return second, first
}

func observed(values []item) bool {
	for _, value := range values {
		first := func() bool { return value.enabled }
		second := func() bool { return false }
		first, second = swap(first, second)
		value.enabled = true
		if second() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangetupleswap\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("simultaneous tuple closure result = %#v", result.Files)
	}
}

func TestSuspiciousRangeTreatsTupleCallArgumentsAsEscaped(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

var retained func() bool

func executeAndReturn(callback func() bool) (func() bool, bool) {
	_ = callback()
	return func() bool { return false }, true
}

func retainAndReturn(callback func() bool) (func() bool, bool) {
	retained = callback
	return func() bool { return false }, true
}

func observed(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		_, _ = executeAndReturn(inspect)
	}
}

func escaped(values []item) {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		value.enabled = true
		_, _ = retainAndReturn(inspect)
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangetupleescape\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("tuple call argument result = %#v", result.Files)
	}
}

func TestSuspiciousRangeTracksNestedSelectorAndReceiverAliases(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }
type nested struct { inspect func() bool }
type holder struct { inner nested }

func nestedSelector(values []item) bool {
	for _, value := range values {
		var saved holder
		saved.inner.inspect = func() bool { return value.enabled }
		value.enabled = true
		if saved.inner.inspect() { return true }
	}
	return false
}

func receiverAlias(values []item) bool {
	for _, value := range values {
		saved := &nested{}
		alias := saved
		alias.inspect = func() bool { return value.enabled }
		value.enabled = true
		if saved.inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeselectoralias\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("nested selector and receiver alias result = %#v", result.Files)
	}
}

func TestSuspiciousRangeMapsMethodExpressionTupleArguments(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }
type keeper struct{}

func (keeper) keep(callback func() bool) (func() bool, bool) {
	return callback, true
}

func observed(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		inspect, _ = keeper.keep(keeper{}, inspect)
		value.enabled = true
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangemethodtuple\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("method expression tuple result = %#v", result.Files)
	}
}

func TestSuspiciousRangeModelsSameStatementBackedgeReads(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct {
	count int
	slots [2]int
}

func increment(values []item) {
	for _, value := range values {
		for range 2 {
			value.count++
		}
	}
}

func indexed(values []item) {
	for _, value := range values {
		for range 2 {
			value.slots[value.slots[0]] = 1
		}
	}
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangesamestatement\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("same-statement backedge result = %#v", result.Files)
	}
}

func TestSuspiciousRangeTracksPointerMethodValues(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func (value *item) Enabled() bool { return value.enabled }
func (value item) Snapshot() bool { return value.enabled }

func observed(values []item) bool {
	for _, value := range values {
		inspect := value.Enabled
		value.enabled = true
		if inspect() { return true }
	}
	return false
}

func snapshot(values []item) bool {
	for _, value := range values {
		inspect := value.Snapshot
		value.enabled = true
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangemethodvalue\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
	wantStart := strings.LastIndex(input, "value.enabled = true")
	if got := result.Files[0].Diagnostics[0].Range.Start; got != wantStart {
		t.Fatalf("value-method snapshot diagnostic start = %d, want %d", got, wantStart)
	}
}

func TestSuspiciousRangeRejectsReassignedTupleParameters(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func replace(callback func() bool) (func() bool, bool) {
	callback = func() bool { return false }
	return callback, true
}

func unobserved(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		inspect, _ = replace(inspect)
		value.enabled = true
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangereassignedtuple\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func TestSuspiciousRangeRejectsTupleParameterMutationInIIFE(t *testing.T) {
	t.Parallel()

	input := `package sample

type item struct { enabled bool }

func replace(callback func() bool) (func() bool, bool) {
	func() {
		callback = func() bool { return false }
	}()
	return callback, true
}

func unobserved(values []item) bool {
	for _, value := range values {
		inspect := func() bool { return value.enabled }
		inspect, _ = replace(inspect)
		value.enabled = true
		if inspect() { return true }
	}
	return false
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangeiifetuple\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	result := runSuspiciousRange(t, root)
	assertSuspiciousRangeDiagnostic(t, input, result, "value.enabled")
}

func runSuspiciousRange(t *testing.T, root string) analysis.PackageResult {
	t.Helper()
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
				"suspicious-range": rules.SeverityWarn,
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

func assertSuspiciousRangeDiagnostic(
	t *testing.T,
	input string,
	result analysis.PackageResult,
	want string,
) {
	t.Helper()
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("suspicious-range result = %#v", result.Files)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	if content := input[diagnostic.Range.Start:diagnostic.Range.End];
		diagnostic.RuleID != "suspicious-range" || content != want {
		t.Fatalf("suspicious-range diagnostic = %#v for %q", diagnostic, content)
	}
}

func TestSuspiciousRangeMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("suspicious-range")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeRangeStmt}) ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("suspicious-range metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkSuspiciousRangePackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangebenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\ntype item struct{ ready bool }\nfunc run(values []item) { for _, value := range values { value.ready = true } }\n",
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
				"suspicious-range": rules.SeverityWarn,
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

func BenchmarkSuspiciousRangeTupleFlowScaling(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/suspiciousrangetuplescaling\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\ntype item struct{ ready bool }\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func keep%d(callback func() bool) (func() bool, bool) { return callback, true }\n",
			index,
		)
	}
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(values []item) { for _, value := range values { inspect := func() bool { return value.ready }; inspect, _ = keep%d(inspect); value.ready = true; _ = inspect() } }\n",
			index,
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
				"suspicious-range": rules.SeverityWarn,
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
