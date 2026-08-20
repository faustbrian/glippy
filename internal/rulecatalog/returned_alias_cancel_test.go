package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestContextCancelObligationsConsumeReturnedAliasContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/cancelaliasobligation\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	input := `package sample

import (
	"context"
	"example.com/cancelaliasobligation/helper"
)

func retainCancel(cancel context.CancelFunc) context.CancelFunc { return cancel }
func retainCancelWithError(cancel context.CancelFunc) (context.CancelFunc, error) {
	return cancel, nil
}
func retainCancelAfterError(cancel context.CancelFunc) (error, context.CancelFunc) {
	return nil, cancel
}
func retainCancelTwice(cancel context.CancelFunc) (context.CancelFunc, context.CancelFunc) {
	return cancel, cancel
}

func replaceCancel(cancel context.CancelFunc) context.CancelFunc { return cancel }
func cancelAndRetain(cancel context.CancelFunc) context.CancelFunc {
	cancel()
	return cancel
}

type CancelRetainer struct{}

func (CancelRetainer) Retain(cancel context.CancelFunc) context.CancelFunc { return cancel }

func retainedAlias(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel = retainCancel(cancel)
}

func retainedTupleAlias(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	var err error
	cancel, err = retainCancelWithError(cancel)
	_ = err
}

func retainedNonzeroTupleAlias(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	var err error
	err, cancel = retainCancelAfterError(cancel)
	_ = err
}

func retainedOrdinaryPositionalAlias(parent context.Context, replacement context.CancelFunc) {
	_, cancel := context.WithCancel(parent)
	var other context.CancelFunc
	cancel, other = retainCancel(cancel), replacement
	_ = other
}

func retainedImportedAlias(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel = helper.Retain(cancel)
}

func retainedMethodExpressionAlias(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel = CancelRetainer.Retain(CancelRetainer{}, cancel)
}

func retainedAfterFinalSelfWrite(parent context.Context, replacement context.CancelFunc) {
	_, cancel := context.WithCancel(parent)
	cancel, cancel = replacement, cancel
}

func retainedAfterFinalAliasWrite(parent context.Context, replacement context.CancelFunc) {
	_, cancel := context.WithCancel(parent)
	cancel, cancel = replacement, retainCancel(cancel)
}

func uncontractedReplacement(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel = replaceCancel(cancel)
}

func invokedAndReturned(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel = cancelAndRetain(cancel)
}

func overwrittenReturnedAlias(parent context.Context, replacement context.CancelFunc) {
	_, cancel := context.WithCancel(parent)
	cancel, cancel = retainCancel(cancel), replacement
	cancel()
}

func returnedIntoDifferentBinding(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	replacement := retainCancel(cancel)
	_ = replacement
}

func returnedIntoSameAndDifferentBindings(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel, other := retainCancelTwice(cancel)
	other()
}

func methodValueReplacement(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	retain := CancelRetainer{}.Retain
	cancel = retain(cancel)
}

func preservedWithBlankUse(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel, _ = cancel, cancel
}

func preservedWithAddressUse(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	var pointer *context.CancelFunc
	cancel, pointer = cancel, &cancel
	_ = pointer
}

func returnedWithBlankUse(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	cancel, _ = retainCancel(cancel), cancel
}
`
	writeFixture(t, path, input)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import "context"

func Retain(cancel context.CancelFunc) context.CancelFunc { return cancel }
`,
	)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/cancelaliasobligation.retainCancel"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/cancelaliasobligation.retainCancelWithError"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/cancelaliasobligation.retainCancelAfterError"
returns-alias = [{ result = 1, argument = 0 }]

[[functions]]
symbol = "example.com/cancelaliasobligation.retainCancelTwice"
returns-alias = [{ result = 0, argument = 0 }, { result = 1, argument = 0 }]

[[functions]]
symbol = "example.com/cancelaliasobligation.CancelRetainer.Retain"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/cancelaliasobligation/helper.Retain"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/cancelaliasobligation.cancelAndRetain"
invokes-cancellation = [0]
takes-ownership = [0]
returns-alias = [{ result = 0, argument = 0 }]
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
				"context-cancel-leak": rules.SeverityWarn,
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
	if len(result.Files) != 1 || result.Files[0].Path != path {
		t.Fatalf("returned-alias cancel source ownership = %#v", result.Files)
	}
	if len(result.Files[0].Diagnostics) != 8 {
		t.Fatalf("returned-alias cancel diagnostics = %#v", result.Files[0].Diagnostics)
	}
	wantStarts := make(map[int]bool)
	for _, function := range
		[]string{
			"retainedAlias",
			"retainedTupleAlias",
			"retainedNonzeroTupleAlias",
			"retainedOrdinaryPositionalAlias",
			"retainedImportedAlias",
			"retainedMethodExpressionAlias",
			"retainedAfterFinalSelfWrite",
			"retainedAfterFinalAliasWrite",
		} {
		functionStart := strings.Index(input, "func " + function)
		acquisitionStart := strings.Index(
			input[functionStart:],
			"_, cancel := context.WithCancel(parent)",
		)
		wantStarts[functionStart + acquisitionStart] = true
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "context-cancel-leak" ||
			diagnostic.MessageKey != "cancel-not-used-on-all-paths" ||
			!wantStarts[diagnostic.Range.Start] {
			t.Fatalf("returned-alias cancel diagnostic = %#v", diagnostic)
		}
		delete(wantStarts, diagnostic.Range.Start)
	}
	if len(wantStarts) != 0 {
		t.Fatalf("missing returned-alias cancel diagnostics at %#v", wantStarts)
	}
}

func BenchmarkReturnedAliasCancelObligations(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/cancelaliasbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString(
		"package sample\nimport \"context\"\nfunc retain(cancel context.CancelFunc) context.CancelFunc { return cancel }\n",
	)
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(parent context.Context) { _, cancel := context.WithCancel(parent); cancel = retain(cancel) }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					"version = 1\n[[functions]]\nsymbol = \"example.com/cancelaliasbenchmark.retain\"\nreturns-alias = [{ result = 0, argument = 0 }]\n",
				),
			},
		},
	)
	if err != nil {
		b.Fatal(err)
	}
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
				"context-cancel-leak": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
			Contracts: set,
		},
		100,
	)
}
