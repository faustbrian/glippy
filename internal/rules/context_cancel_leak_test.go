package rules_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestContextCancelLeakReportsDiscardedAndPathLeakedCancellation(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"context"
	"time"
)

func discarded(parent context.Context) context.Context {
	child, _ := context.WithCancel(parent)
	return child
}

func partial(parent context.Context, stop bool) context.Context {
	child, cancel := context.WithTimeout(parent, 1)
	if stop {
		cancel()
	}
	return child
}

func safe(parent context.Context) context.Context {
	child, cancel := context.WithDeadline(parent, deadline())
	defer cancel()
	return child
}

func transferred(parent context.Context) (context.Context, context.CancelFunc) {
	child, cancel := context.WithCancel(parent)
	return child, cancel
}

func captured(parent context.Context) context.Context {
	child, cancel := context.WithCancelCause(parent)
	go func() { cancel(nil) }()
	return child
}

func deadline() time.Time { return time.Time{} }
`
	result := runContextCancelLeak(t, "sample", input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("context-cancel-leak result = %#v", result)
	}
	want := []struct {
		needle string
		length int
		message string
	}{
		{
			needle: "_ := context.WithCancel",
			length: 1,
			message: "should be called, not discarded",
		},
		{
			needle: "child, cancel := context.WithTimeout",
			length: len("child, cancel := context.WithTimeout(parent, 1)"),
			message: "not used on all paths",
		},
	}
	for index, expected := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, expected.needle)
		if start < 0 {
			t.Fatalf("fixture does not contain %q", expected.needle)
		}
		if diagnostic.RuleID != "context-cancel-leak" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + expected.length ||
			!strings.Contains(diagnostic.Message, expected.message) {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestContextCancelLeakExcludesMainAndLookalikeFunctions(t *testing.T) {
	t.Parallel()

	result := runContextCancelLeak(
		t,
		"main",
		`package main

import "context"

type factory struct{}

func (factory) WithCancel(context.Context) (context.Context, context.CancelFunc) {
	return nil, nil
}

func main() {
	_, _ = context.WithCancel(context.Background())
}

func lookalike(parent context.Context) context.Context {
	child, _ := (factory{}).WithCancel(parent)
	return child
}
`,
	)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("context-cancel-leak exclusions = %#v", result)
	}
}

func TestContextCancelLeakDocumentsControlFlowPolicyWithoutAFix(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(rules.NewContextCancelLeakRule())
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("context-cancel-leak")
	if !found ||
		metadata.Requirement != rules.RequireControlFlow ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.NodeInterests) != 0 ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Presets) != 1 ||
		metadata.Presets[0] != rules.PresetCorrectness ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("metadata = %#v, found = %v", metadata, found)
	}
}

func TestContextCancelLeakMatchesLostCancelFunctionAndScopeBoundaries(t *testing.T) {
	t.Parallel()

	input := `package sample

import "context"

var packageCancel context.CancelFunc

func declarations(parent context.Context) {
	var _, declared = context.WithCancel(parent)
	_ = declared
	return
}

func assignments(parent context.Context) {
	var assigned context.CancelFunc
	_, assigned = context.WithCancel(parent)
	if parent == nil {
		assigned()
	}
}

func namedResult(parent context.Context) (child context.Context, cancel context.CancelFunc) {
	child, cancel = context.WithCancel(parent)
	return
}

var literal = func(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	if parent == nil {
		cancel()
	}
}

func outerAssignment(parent context.Context) {
	var cancel context.CancelFunc
	func() {
		_, cancel = context.WithCancel(parent)
	}()
	cancel()
}

func packageAssignment(parent context.Context) {
	_, packageCancel = context.WithCancel(parent)
}

func captured(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	callback := func() { cancel() }
	_ = callback
}

func panics(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	panic(cancel)
}
`
	result := runContextCancelLeak(t, "sample", input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("context-cancel-leak boundary result = %#v", result)
	}
	for index, needle := range
		[]string{
			"_, assigned = context.WithCancel(parent)",
			"_, cancel := context.WithCancel(parent)",
		} {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, needle)
		if start < 0 {
			t.Fatalf("fixture does not contain %q", needle)
		}
		if diagnostic.RuleID != "context-cancel-leak" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(needle) ||
			!strings.Contains(diagnostic.Message, "not used on all paths") {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func runContextCancelLeak(t *testing.T, packageName string, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/contextcancel\n\ngo 1.26.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, packageName + ".go")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(rules.NewContextCancelLeakRule())
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"context-cancel-leak": rules.SeverityWarn,
			},
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
