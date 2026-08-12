package rules_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type benchmarkDeferControlFlowRule struct{}

func (benchmarkDeferControlFlowRule) Metadata() rules.Metadata {
	return rules.Metadata{
		ID: "benchmark-defer-cfg", Summary: "provides a control-flow callback baseline",
		Documentation:    "Provides a no-op control-flow callback for benchmark comparison.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetSuspicious},
		MinimumGoVersion: "1.26",
		Requirement:      rules.RequireControlFlow,
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Examples:         []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}
}

func (benchmarkDeferControlFlowRule) RunControlFlow(
	*rules.ControlFlowContext,
) ([]rules.Finding, error) {
	return nil, nil
}

func TestDeferInInfiniteLoopReportsOnlyWhenNoFunctionExitIsReachable(t *testing.T) {
	t.Parallel()

	input := `package sample
func cleanup() {}
func direct() {
	for {
		defer cleanup()
	}
}
func nestedBreak() {
	for {
		defer cleanup()
		switch {
		default:
			break
		}
	}
}
func nestedSelectBreak() {
	for {
		defer cleanup()
		select {
		default:
			break
		}
	}
}
func loopBreak(ready bool) {
	for {
		defer cleanup()
		if ready {
			break
		}
	}
}
func returnExit(ready bool) {
	for {
		defer cleanup()
		if ready {
			return
		}
	}
}
func panicExit(ready bool) {
	for {
		defer cleanup()
		if ready {
			panic("stop")
		}
	}
}
func gotoExit(ready bool) {
	for {
		defer cleanup()
		if ready {
			goto done
		}
	}
done:
}
`
	result := runDeferInInfiniteLoop(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 3 {
		t.Fatalf("result = %#v, want three defer diagnostics", result)
	}
	searchStart := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		offset := strings.Index(input[searchStart:], "defer")
		if offset < 0 {
			t.Fatalf("input does not contain defer after %d", searchStart)
		}
		offset += searchStart
		if diagnostic.RuleID != "defer-in-infinite-loop" ||
			diagnostic.MessageKey != "defer-never-runs" ||
			diagnostic.Range != (source.Range{Start: offset, End: offset + len("defer")}) ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic %d = %#v, want defer at %d", index, diagnostic, offset)
		}
		searchStart = offset + len("defer")
	}
}

func TestDeferInInfiniteLoopAnalyzesNestedFunctionLiteralsInTheirOwnCFG(t *testing.T) {
	t.Parallel()

	input := `package sample
func cleanup() {}
func outer() {
	_ = func() {
		for {
			defer cleanup()
		}
	}
}
`
	result := runDeferInInfiniteLoop(t, input, nil)
	offset := strings.Index(input, "defer")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].Range != (source.Range{
			Start: offset,
			End:   offset + len("defer"),
		}) {
		t.Fatalf("nested function result = %#v", result)
	}
}

func TestDeferInInfiniteLoopRecognizesDotImportedGoexit(t *testing.T) {
	t.Parallel()

	result := runDeferInInfiniteLoop(t, `package sample
import . "runtime"
func cleanup() {}
func run() {
	for {
		defer cleanup()
		Goexit()
	}
}
`, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("dot-imported Goexit result = %#v", result)
	}
}

func TestDeferInInfiniteLoopHandlesNestedUnreachableAndUnwindBoundaries(t *testing.T) {
	t.Parallel()

	input := `package sample
import "runtime"
func cleanup() {}
func nested() {
	for {
		for {
			defer cleanup()
		}
	}
}
func shadowedPanic(panic func(string)) {
	for {
		defer cleanup()
		panic("continue")
	}
}
func unreachable() {
	for {
		continue
		defer cleanup()
	}
}
func conditional() {
	for false {
		defer cleanup()
	}
}
func explicitTrue() {
	for true {
		defer cleanup()
	}
}
func nestedFunction() {
	for {
		_ = func() { defer cleanup() }
		break
	}
}
func goexit() {
	for {
		defer cleanup()
		runtime.Goexit()
	}
}
func parenthesizedPanic() {
	for {
		defer cleanup()
		(panic)("stop")
	}
}
`
	result := runDeferInInfiniteLoop(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("result = %#v, want nested and shadowed-panic diagnostics", result)
	}
	searchStart := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		offset := strings.Index(input[searchStart:], "defer")
		if offset < 0 {
			t.Fatalf("input does not contain defer after %d", searchStart)
		}
		offset += searchStart
		if diagnostic.Range != (source.Range{Start: offset, End: offset + len("defer")}) {
			t.Fatalf("diagnostic %d = %#v, want defer at %d", index, diagnostic, offset)
		}
		searchStart = offset + len("defer")
	}
}

func TestDeferInInfiniteLoopHonorsSuppressionGeneratedTypeErrorAndSeverityPolicies(t *testing.T) {
	t.Parallel()

	suppressed := runDeferInInfiniteLoop(t, `package sample
func cleanup() {}
func run() {
	for {
		//gox:ignore defer-in-infinite-loop -- process lifetime cleanup
		defer cleanup()
	}
}
`, nil)
	if len(suppressed.Files) != 1 || len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 ||
		suppressed.Files[0].Suppressed[0].Diagnostic.RuleID != "defer-in-infinite-loop" {
		t.Fatalf("suppressed result = %#v", suppressed)
	}

	generated := runDeferInInfiniteLoop(t, `// Code generated by fixture. DO NOT EDIT.
package sample
func cleanup() {}
func run() { for { defer cleanup() } }
`, nil)
	if len(generated.Files) != 1 || len(generated.Files[0].Diagnostics) != 0 {
		t.Fatalf("generated result = %#v", generated)
	}

	illTyped := runDeferInInfiniteLoop(t, `package sample
func cleanup() {}
func run() { missing(); for { defer cleanup() } }
`, nil)
	if len(illTyped.LoadDiagnostics) == 0 || len(illTyped.Files) != 1 ||
		len(illTyped.Files[0].Diagnostics) != 0 {
		t.Fatalf("ill-typed result = %#v", illTyped)
	}

	severity := runDeferInInfiniteLoop(t, `package sample
func cleanup() {}
func run() { for { defer cleanup() } }
`, map[string]rules.Severity{"defer-in-infinite-loop": rules.SeverityError})
	if len(severity.Files) != 1 || len(severity.Files[0].Diagnostics) != 1 ||
		severity.Files[0].Diagnostics[0].Severity != rules.SeverityError {
		t.Fatalf("severity result = %#v", severity)
	}
}

func TestDefaultRegistryDocumentsDeferInInfiniteLoopWithoutAFix(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("defer-in-infinite-loop")
	if !found || metadata.Requirement != rules.RequireControlFlow ||
		metadata.DefaultSeverity != rules.SeverityWarn || metadata.MinimumGoVersion != "1.25" ||
		metadata.RunOnGenerated || metadata.RunDespiteTypeErrors || len(metadata.Fixes) != 0 ||
		len(metadata.Presets) != 1 || metadata.Presets[0] != rules.PresetSuspicious ||
		len(metadata.Examples) == 0 || len(metadata.KnownLimitations) == 0 {
		t.Fatalf("metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkDeferInInfiniteLoopSharedCFG(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/deferbenchmark\n\ngo 1.26.0\n"),
		0o600,
	); err != nil {
		b.Fatal(err)
	}
	var input strings.Builder
	input.WriteString("package sample\nfunc cleanup() {}\n")
	for index := range 100 {
		fmt.Fprintf(&input, "func run%d() { for { defer cleanup() } }\n", index)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(input.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	loaded, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir: root, Patterns: []string{"."}, Requirement: rules.RequireControlFlow,
	})
	if err != nil {
		b.Fatal(err)
	}
	deferRegistry, err := rules.NewDefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	deferSelection, err := deferRegistry.Resolve(rules.PresetSuspicious, map[string]rules.Severity{
		"context-key":         rules.SeverityOff,
		"errors-is-arguments": rules.SeverityOff,
		"nilness":             rules.SeverityOff,
	})
	if err != nil {
		b.Fatal(err)
	}
	baselineRegistry, err := rules.NewRegistry(benchmarkDeferControlFlowRule{})
	if err != nil {
		b.Fatal(err)
	}
	baselineSelection, err := baselineRegistry.Resolve(rules.PresetSuspicious, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("baseline", func(b *testing.B) {
		benchmarkControlFlowExecution(b, loaded, baselineRegistry, baselineSelection, 0)
	})
	b.Run("defer-in-infinite-loop", func(b *testing.B) {
		benchmarkControlFlowExecution(b, loaded, deferRegistry, deferSelection, 100)
	})
}

func benchmarkControlFlowExecution(
	b *testing.B,
	loaded analysis.PackageLoadResult,
	registry *rules.Registry,
	selection []rules.Selection,
	wantDiagnostics int,
) {
	b.Helper()
	b.ReportAllocs()
	for range b.N {
		diagnostics, err := analysis.RunControlFlow(context.Background(), loaded, registry, selection)
		if err != nil {
			b.Fatal(err)
		}
		if len(diagnostics) != wantDiagnostics {
			b.Fatalf("diagnostics = %d, want %d", len(diagnostics), wantDiagnostics)
		}
	}
}

func runDeferInInfiniteLoop(
	t *testing.T,
	input string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/deferloop\n\ngo 1.26.0\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if overrides == nil {
		overrides = make(map[string]rules.Severity)
	}
	overrides["nilness"] = rules.SeverityOff
	overrides["errors-is-arguments"] = rules.SeverityOff
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetSuspicious, Overrides: overrides},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
