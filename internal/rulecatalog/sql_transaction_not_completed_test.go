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

func TestSQLTransactionNotCompletedReportsNormallyReturningOpenTransactions(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"context"
	"database/sql"
	"errors"
)

func missing(db *sql.DB, fail bool) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_, _ = tx.Exec("update")
	if fail { return errors.New("failed") }
	return nil
}

func partial(db *sql.DB, finish bool) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if finish { return tx.Commit() }
	return nil
}

func conditionalDefer(db *sql.DB, cleanup bool) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if cleanup { defer tx.Rollback() }
	return nil
}

func overwritten(db, replacement *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	tx, err = replacement.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	return nil
}

func missingConn(conn *sql.Conn, ctx context.Context) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil { return err }
	_, _ = tx.Exec("update")
	return nil
}

func deferredMethodValue(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	rollback := tx.Rollback
	defer rollback()
	return nil
}

func asynchronousTransfer(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	go tx.Rollback()
	return nil
}

func deferred(db *sql.DB, ctx context.Context) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	return tx.Commit()
}

func completedBranches(db *sql.DB, commit bool) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if commit { return tx.Commit() }
	return tx.Rollback()
}

func transferred(db *sql.DB) (*sql.Tx, error) {
	tx, err := db.Begin()
	if err != nil { return nil, err }
	return tx, nil
}

type ownedTransaction struct { transaction *sql.Tx }

func wrapped(db *sql.DB) (ownedTransaction, error) {
	tx, err := db.Begin()
	if err != nil { return ownedTransaction{}, err }
	return ownedTransaction{transaction: tx}, nil
}

func passed(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	consume(tx)
	return nil
}

func consume(*sql.Tx) {}

type wrapper struct{}

func (*wrapper) Begin() (*sql.Tx, error) { return nil, nil }

func lookalike(db *wrapper) error {
	tx, err := db.Begin()
	if err != nil { return err }
	return tx.Rollback()
}

func noncanonicalGuard(db *sql.DB) error {
	tx, err := db.Begin()
	if err == nil { return tx.Commit() }
	return err
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/sqltransaction\n\ngo 1.25.0\n",
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
				"sql-transaction-not-completed": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 6 {
		t.Fatalf("sql-transaction-not-completed result = %#v", result)
	}
	expected := []struct {
		function string
		acquisition string
	}{
		{function: "func missing(", acquisition: "tx, err := db.Begin()"},
		{function: "func partial(", acquisition: "tx, err := db.Begin()"},
		{function: "func conditionalDefer(", acquisition: "tx, err := db.Begin()"},
		{function: "func overwritten(", acquisition: "tx, err := db.Begin()"},
		{function: "func missingConn(", acquisition: "tx, err := conn.BeginTx(ctx, nil)"},
		{function: "func passed(", acquisition: "tx, err := db.Begin()"},
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		functionStart := strings.Index(input, expected[index].function)
		if functionStart < 0 {
			t.Fatalf("missing function %d", index)
		}
		relative := strings.Index(input[functionStart:], expected[index].acquisition)
		if relative < 0 {
			t.Fatalf("missing acquisition %d", index)
		}
		start := functionStart + relative
		if diagnostic.RuleID != "sql-transaction-not-completed" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("tx") ||
			!strings.Contains(diagnostic.Message, "not committed or rolled back") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestSQLTransactionNotCompletedMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("sql-transaction-not-completed")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf(
			"sql-transaction-not-completed metadata = %#v, found = %v",
			metadata,
			found,
		)
	}
}

func TestSQLTransactionNotCompletedHonorsSharedEligibilityPolicies(t *testing.T) {
	t.Parallel()

	suppressed := runSQLTransactionNotCompleted(
		t,
		`package sample
import "database/sql"
func run(db *sql.DB) error {
	//glippy:ignore sql-transaction-not-completed -- finalized by the caller
	tx, err := db.Begin()
	if err != nil { return err }
	_, _ = tx.Exec("update")
	return nil
}
`,
		"go1.25",
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 ||
		suppressed.Files[0].Suppressed[0].Diagnostic.RuleID !=
			"sql-transaction-not-completed" ||
		suppressed.Files[0].Suppressed[0].Diagnostic.Severity != rules.SeverityError {
		t.Fatalf("suppressed result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
import "database/sql"
func run(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	_, _ = tx.Exec("update")
	return nil
}
`,
			"type-error": `package sample
import "database/sql"
func run(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil { return err }
	undefined()
	_, _ = tx.Exec("update")
	return nil
}
`,
		} {
		result := runSQLTransactionNotCompleted(t, input, "go1.25")
		if len(result.Files) != 1 ||
			len(result.Files[0].Diagnostics) != 0 ||
			len(result.Files[0].Suppressed) != 0 {
			t.Fatalf("%s result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf("type-error result has no load diagnostics: %#v", result)
		}
	}

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"sql-transaction-not-completed": rules.SeverityError,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 result = %#v", older)
	}
}

func BenchmarkSQLTransactionNotCompletedPackageAnalysis(b *testing.B) {
	var input strings.Builder
	input.WriteString("package sample\nimport \"database/sql\"\n")
	for index := 0; index < 100; index++ {
		fmt.Fprintf(
			&input,
			"func run%d(db *sql.DB) error { tx, err := db.Begin(); if err != nil { return err }; _, _ = tx.Exec(\"update\"); return nil }\n",
			index,
		)
	}
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/sqltransactionbenchmark\n\ngo 1.25.0\n",
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
				"sql-transaction-not-completed": rules.SeverityWarn,
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

func runSQLTransactionNotCompleted(
	t *testing.T,
	input string,
	goVersion string,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/sqltransactionpolicy\n\ngo 1.25.0\n",
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
				"sql-transaction-not-completed": rules.SeverityError,
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
