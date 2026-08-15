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

func TestLifecycleRulesUseImportedParameterEffects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/effects\n\ngo 1.26.0\n")
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import (
	"context"
	"database/sql"
	"fmt"
	"io"
)

var retainedCloser io.Closer
var retainedCancel context.CancelFunc
var retainedTransaction *sql.Tx

func BorrowCloser(io.Closer) {}
func CloseCloser(closer io.Closer) { _ = closer.Close() }
func TransferCloser(closer io.Closer) { retainedCloser = closer }
func BorrowThenTransfer(first, second io.Closer) { retainedCloser = second }
func MaybeCloseCloser(closer io.Closer, close bool) {
	if close { _ = closer.Close() }
}

func OpaqueCloser(closer io.Closer) { _, _ = fmt.Fprint(io.Discard, closer) }
func RecursiveCloser(closer io.Closer) { RecursiveCloser(closer) }

func BorrowCancel(context.CancelFunc) {}
func InvokeCancel(cancel context.CancelFunc) { cancel() }
func TransferCancel(cancel context.CancelFunc) { retainedCancel = cancel }
func BorrowThenTransferCancel(first, second context.CancelFunc) { retainedCancel = second }
func MaybeInvokeCancel(cancel context.CancelFunc, invoke bool) {
	if invoke { cancel() }
}

func BorrowTransaction(*sql.Tx) {}
func CompleteTransaction(transaction *sql.Tx) { _ = transaction.Rollback() }
func TransferTransaction(transaction *sql.Tx) { retainedTransaction = transaction }
func MaybeCompleteTransaction(transaction *sql.Tx, complete bool) {
	if complete { _ = transaction.Rollback() }
}
`,
	)
	path := filepath.Join(root, "sample.go")
	writeFixture(
		t,
		path,
		`package sample

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"example.com/effects/helper"
)

func leakedCloser() error {
	file, err := os.Open("input")
	if err != nil { return err }
	helper.BorrowCloser(file)
	return nil
}

func closedCloser() error {
	file, err := os.Open("input")
	if err != nil { return err }
	helper.CloseCloser(file)
	return nil
}

func transferredCloser() error {
	file, err := os.Open("input")
	if err != nil { return err }
	helper.TransferCloser(file)
	return nil
}

func maybeClosedCloser(close bool) error {
	file, err := os.Open("input")
	if err != nil { return err }
	helper.MaybeCloseCloser(file, close)
	return nil
}

func opaqueCloser() error {
	file, err := os.Open("input")
	if err != nil { return err }
	helper.OpaqueCloser(file)
	return nil
}

func recursiveCloser() error {
	file, err := os.Open("input")
	if err != nil { return err }
	helper.RecursiveCloser(file)
	return nil
}

func leakedCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	helper.BorrowCancel(cancel)
}

func invokedCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	helper.InvokeCancel(cancel)
}

func transferredCancel(parent context.Context) {
	_, cancel := context.WithCancel(parent)
	helper.BorrowThenTransferCancel(cancel, cancel)
}

func maybeInvokedCancel(parent context.Context, invoke bool) {
	_, cancel := context.WithCancel(parent)
	helper.MaybeInvokeCancel(cancel, invoke)
}

func leakedTransaction(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	helper.BorrowTransaction(transaction)
	return nil
}

func completedTransaction(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	helper.CompleteTransaction(transaction)
	return nil
}

func transferredTransaction(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	helper.TransferTransaction(transaction)
	return nil
}

func maybeCompletedTransaction(database *sql.DB, complete bool) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	helper.MaybeCompleteTransaction(transaction, complete)
	return nil
}

func leakedResponse() error {
	response, err := http.Get("https://example.test")
	if err != nil { return err }
	helper.BorrowCloser(response.Body)
	return nil
}

func closedResponse() error {
	response, err := http.Get("https://example.test")
	if err != nil { return err }
	helper.CloseCloser(response.Body)
	return nil
}

func transferredResponse() error {
	response, err := http.Get("https://example.test")
	if err != nil { return err }
	helper.BorrowThenTransfer(response.Body, response.Body)
	return nil
}

func maybeClosedResponse(close bool) error {
	response, err := http.Get("https://example.test")
	if err != nil { return err }
	helper.MaybeCloseCloser(response.Body, close)
	return nil
}

`,
	)

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
				"http-response-body-not-closed": rules.SeverityWarn,
				"resource-not-closed": rules.SeverityWarn,
				"sql-transaction-not-completed": rules.SeverityWarn,
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
	if len(result.Files) != 1 || result.Files[0].Path != path {
		t.Fatalf("effect source ownership = %#v", result.Files)
	}
	physical, found := result.Sources.Lookup(path)
	if !found {
		t.Fatal("analyzed source is missing")
	}
	got := make(map[string][]int)
	for _, diagnostic := range result.Files[0].Diagnostics {
		position, valid := physical.Position(diagnostic.Range.Start)
		if !valid {
			t.Fatalf("diagnostic has invalid range: %#v", diagnostic)
		}
		got[diagnostic.RuleID] = append(got[diagnostic.RuleID], position.Line)
	}
	want := map[string][]int{
		"resource-not-closed": {13, 34},
		"context-cancel-leak": {55, 70},
		"sql-transaction-not-completed": {75, 96},
		"http-response-body-not-closed": {103, 124},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("imported parameter effect diagnostics = %#v, want %#v", got, want)
	}
}

func BenchmarkImportedParameterEffects(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/effectbenchmark\n\ngo 1.26.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import "io"

func Borrow(io.Closer) {}
func Close(closer io.Closer) { _ = closer.Close() }
`,
	)
	var source strings.Builder
	source.WriteString(
		"package sample\n\nimport (\n\t\"os\"\n\t\"example.com/effectbenchmark/helper\"\n)\n",
	)
	for index := range 100 {
		helper := "Close"
		if index % 2 == 0 {
			helper = "Borrow"
		}
		_, _ = fmt.Fprintf(
			&source,
			"func inspect%d() error { file, err := os.Open(\"input\"); if err != nil { return err }; helper.%s(file); return nil }\n",
			index,
			helper,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), source.String())
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	options := analysis.RunOptions{
		Presets: []rules.Preset{},
		Overrides: map[string]rules.Severity{"resource-not-closed": rules.SeverityWarn},
		SourceGoVersion: "go1.26",
	}
	load := analysis.PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		ModuleMode: analysis.ModuleReadonly,
	}
	b.ResetTimer()
	for b.Loop() {
		result, err := analysis.RunPackages(context.Background(), registry, options, load)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 50 {
			b.Fatalf("imported parameter effect benchmark result = %#v", result)
		}
	}
}
