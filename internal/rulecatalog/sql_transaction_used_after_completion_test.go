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

func TestSQLTransactionUsedAfterCompletionReportsProvenCompletedPaths(t *testing.T) {
	t.Parallel()

	input := `package sample

import "database/sql"

func afterCommit(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if err := tx.Commit(); err != nil { return err }
	_, err = tx.Exec("update")
	return err
}
`
	result := runSQLTransactionUsedAfterCompletion(t, input, "go1.25")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("sql transaction use-after-completion result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	want := `tx.Exec("update")`
	start := strings.Index(input, want)
	if start < 0 {
		t.Fatal("missing transaction operation")
	}
	commit := strings.Index(input, "tx.Commit()")
	if diagnostic.RuleID != "sql-transaction-used-after-completion" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len(want) ||
		!strings.Contains(diagnostic.Message, "after it is committed or rolled back") ||
		len(diagnostic.Related) != 1 ||
		diagnostic.Related[0].Range.Start != commit ||
		len(diagnostic.Fixes) != 0 {
		t.Fatalf("sql transaction use-after-completion diagnostic = %#v", diagnostic)
	}
}

func TestSQLTransactionUsedAfterCompletionTracksBranchesAndFailsClosed(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"database/sql"
	"fmt"
)

func afterRollback(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Rollback()
	_, err = tx.Query("select 1")
	return err
}

func completedBranches(db *sql.DB, commit bool) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if commit { _ = tx.Commit() } else { _ = tx.Rollback() }
	_, err = tx.Exec("update")
	return err
}

func repeatedCompletion(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	return tx.Rollback()
}

func conditionalCompletion(db *sql.DB, commit bool) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if commit { _ = tx.Commit() }
	_, err = tx.Exec("update")
	return err
}

func deferredCompletion(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec("update")
	return err
}

func asynchronousUse(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	go tx.Exec("asynchronous")
	_, err = tx.Exec("after asynchronous use")
	return err
}

func unknownHelper(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	consume(tx)
	_, err = tx.Exec("update")
	return err
}

func multipleCallsInNode(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_, _ = fmt.Println(tx.Commit(), tx.Rollback())
	_, err = tx.Exec("update")
	return err
}

func consume(*sql.Tx) {}
`
	result := runSQLTransactionUsedAfterCompletion(t, input, "go1.25")
	wants := []string{`tx.Query("select 1")`, `tx.Exec("update")`, "tx.Rollback()"}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(wants) {
		t.Fatalf("sql transaction state result = %#v", result)
	}
	for index, want := range wants {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, want)
		if index == 1 {
			functionStart := strings.Index(input, "func completedBranches(")
			start = functionStart + strings.Index(input[functionStart:], want)
		}
		if index == 2 {
			functionStart := strings.Index(input, "func repeatedCompletion(")
			start = functionStart + strings.Index(input[functionStart:], want)
		}
		if diagnostic.RuleID != "sql-transaction-used-after-completion" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want) {
			t.Fatalf("sql transaction state diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestSQLTransactionUsedAfterCompletionUsesEffectsAndExactCompletion(t *testing.T) {
	t.Parallel()

	input := `package sample

import "database/sql"

func finish(tx *sql.Tx) { _ = tx.Rollback() }
func borrow(*sql.Tx) {}
func handoff(tx *sql.Tx) { go tx.Rollback() }
func alias(**sql.Tx) {}

func helperCompletion(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	finish(tx)
	_, err = tx.Exec("helper")
	return err
}

func borrowedCompletion(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	borrow(tx)
	_, err = tx.Exec("borrow")
	return err
}

func exactCompletionAfterUnknown(db *sql.DB, consume func(*sql.Tx)) error {
	tx, err := db.Begin()
	if err != nil { return err }
	consume(tx)
	_ = tx.Commit()
	_, err = tx.Exec("exact")
	return err
}

func transferredOwnership(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	handoff(tx)
	_, err = tx.Exec("transferred")
	return err
}

func aliasedTransaction(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	alias(&tx)
	_, err = tx.Exec("aliased")
	return err
}

func localAlias(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	alias := &tx
	_ = alias
	_, err = tx.Exec("locally aliased")
	return err
}
`
	result := runSQLTransactionUsedAfterCompletion(t, input, "go1.25")
	wants := []string{`tx.Exec("helper")`, `tx.Exec("borrow")`, `tx.Exec("exact")`}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(wants) {
		t.Fatalf("transaction completion effects result = %#v", result)
	}
	for index, want := range wants {
		diagnostic := result.Files[0].Diagnostics[index]
		start := strings.Index(input, want)
		if diagnostic.RuleID != "sql-transaction-used-after-completion" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want) {
			t.Fatalf(
				"transaction completion effect diagnostic %d = %#v",
				index,
				diagnostic,
			)
		}
	}
}

func TestSQLTransactionUsedAfterCompletionMetadataAndEligibility(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("sql-transaction-used-after-completion")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		!metadata.RequiresEffectFacts ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf(
			"sql-transaction-used-after-completion metadata = %#v, found = %v",
			metadata,
			found,
		)
	}

	suppressed := runSQLTransactionUsedAfterCompletion(
		t,
		`package sample
import "database/sql"
func run(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	//glippy:ignore sql-transaction-used-after-completion -- caller records the expected error
	_, err = tx.Exec("update")
	return err
}
`,
		"go1.25",
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed transaction state result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
import "database/sql"
func run(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	_, err = tx.Exec("update")
	return err
}
`,
			"type-error": `package sample
import "database/sql"
func run(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	undefined()
	_, err = tx.Exec("update")
	return err
}
`,
		} {
		result := runSQLTransactionUsedAfterCompletion(t, input, "go1.25")
		if len(result.Files) != 1 ||
			len(result.Files[0].Diagnostics) != 0 ||
			len(result.Files[0].Suppressed) != 0 {
			t.Fatalf("%s transaction state result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf(
				"type-error transaction state result has no load diagnostics: %#v",
				result,
			)
		}
	}

	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"sql-transaction-used-after-completion": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 transaction state selection = %#v", older)
	}
}

func BenchmarkSQLTransactionUsedAfterCompletionPackageAnalysis(b *testing.B) {
	var input strings.Builder
	input.WriteString("package sample\nimport \"database/sql\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(db *sql.DB) error { tx, err := db.Begin(); if err != nil { return err }; _ = tx.Commit(); _, err = tx.Exec(\"update\"); return err }\n",
			index,
		)
	}
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/sqltransactionstatebenchmark\n\ngo 1.25.0\n",
	)
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
				"sql-transaction-used-after-completion": rules.SeverityWarn,
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

func runSQLTransactionUsedAfterCompletion(
	t *testing.T,
	input string,
	goVersion string,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/sqltransactionstate\n\ngo 1.25.0\n",
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
				"sql-transaction-used-after-completion": rules.SeverityWarn,
			},
			SourceGoVersion: goVersion,
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

func TestSQLTransactionUsedAfterCompletionTracksInitializedLocalDeclarations(t *testing.T) {
	t.Parallel()

	input := `package sample

import "database/sql"

func use(db *sql.DB) error {
	var tx, err = db.Begin()
	if err != nil { return err }
	_ = tx.Commit()
	_, err = tx.Exec("update")
	return err
}
`
	result := runSQLTransactionUsedAfterCompletion(t, input, "go1.25")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("initialized declaration result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	operation := `tx.Exec("update")`
	start := strings.Index(input, operation)
	if diagnostic.RuleID != "sql-transaction-used-after-completion" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len(operation) {
		t.Fatalf("initialized declaration diagnostic = %#v", diagnostic)
	}
}
