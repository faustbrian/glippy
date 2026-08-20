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

func TestLifecycleObligationsConsumeReturnedAliasContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/aliasobligation\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	input := `package sample

import "database/sql"

func retainTransaction(transaction *sql.Tx) *sql.Tx { return transaction }
func retainTransactionWithError(transaction *sql.Tx) (*sql.Tx, error) { return transaction, nil }
func completeAndRetainTransaction(transaction *sql.Tx) *sql.Tx { return transaction }
func replaceTransaction(transaction *sql.Tx) *sql.Tx { return transaction }
func borrowTransactionPositionally(transaction, replacement *sql.Tx) {
	var other *sql.Tx
	transaction, other = transaction, replacement
	_ = other
}

type TransactionRetainer struct{}

func (TransactionRetainer) Retain(transaction *sql.Tx) *sql.Tx { return transaction }

func retainedAlias(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction = retainTransaction(transaction)
	return nil
}

func uncontractedReplacement(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction = replaceTransaction(transaction)
	return nil
}

func retainedTupleAlias(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction, err = retainTransactionWithError(transaction)
	if err != nil { return err }
	return nil
}

func completedAndReturned(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction = completeAndRetainTransaction(transaction)
	return nil
}

func overwrittenReturnedAlias(database *sql.DB, replacement *sql.Tx) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction, transaction = retainTransaction(transaction), replacement
	return transaction.Commit()
}

func returnedIntoDifferentBinding(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	replacement := retainTransaction(transaction)
	_ = replacement
	return nil
}

func retainedMethodExpressionAlias(database *sql.DB) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction = TransactionRetainer.Retain(TransactionRetainer{}, transaction)
	return nil
}

func retainedAfterPositionalSelfAssignment(database *sql.DB, replacement *sql.Tx) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	var other *sql.Tx
	transaction, other, transaction = transaction, replacement, retainTransaction(transaction)
	_ = other
	return nil
}

func retainedAfterInferredPositionalBorrow(database *sql.DB, replacement *sql.Tx) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	borrowTransactionPositionally(transaction, replacement)
	return nil
}

func finalSelfWriteThenComplete(database *sql.DB, replacement *sql.Tx) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction, transaction = replacement, transaction
	return transaction.Commit()
}
`
	writeFixture(t, path, input)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/aliasobligation.retainTransaction"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/aliasobligation.retainTransactionWithError"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/aliasobligation.completeAndRetainTransaction"
completes-transaction = [0]
takes-ownership = [0]
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/aliasobligation.TransactionRetainer.Retain"
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
				"sql-transaction-not-completed": rules.SeverityWarn,
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
		t.Fatalf("returned-alias source ownership = %#v", result.Files)
	}
	if len(result.Files[0].Diagnostics) != 6 {
		t.Fatalf("returned-alias lifecycle diagnostics = %#v", result.Files[0].Diagnostics)
	}
	wantStarts := make(map[int]bool)
	for _, function := range
		[]string{
			"retainedAlias",
			"retainedTupleAlias",
			"overwrittenReturnedAlias",
			"retainedMethodExpressionAlias",
			"retainedAfterPositionalSelfAssignment",
			"retainedAfterInferredPositionalBorrow",
		} {
		functionStart := strings.Index(input, "func " + function)
		acquisitionStart := strings.Index(
			input[functionStart:],
			"transaction, err := database.Begin()",
		)
		wantStarts[functionStart + acquisitionStart] = true
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "sql-transaction-not-completed" ||
			!wantStarts[diagnostic.Range.Start] ||
			diagnostic.Range.End != diagnostic.Range.Start + len("transaction") {
			t.Fatalf("returned-alias lifecycle diagnostic = %#v", diagnostic)
		}
		delete(wantStarts, diagnostic.Range.Start)
	}
	if len(wantStarts) != 0 {
		t.Fatalf("missing returned-alias lifecycle diagnostics at %#v", wantStarts)
	}
}

func TestResourceObligationsConsumeReturnedAliasContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourcealiasobligation\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	input := `package sample

import "os"

func retainFile(file *os.File) *os.File { return file }
func closeAndRetainFile(file *os.File) *os.File { return file }

type FileRetainer struct{}

func (FileRetainer) Retain(file *os.File) *os.File { return file }

type CancelCloser func()

func (CancelCloser) Close() error { return nil }
func acquireCancelCloser() CancelCloser { return func() {} }
func closeAndInvokeCancelCloser(closer CancelCloser) CancelCloser { return closer }

func retainedAlias(path string) error {
	file, err := os.Open(path)
	if err != nil { return err }
	file = retainFile(file)
	return nil
}

func completedAndReturned(path string) error {
	file, err := os.Open(path)
	if err != nil { return err }
	file = closeAndRetainFile(file)
	return nil
}

func retainedMethodExpressionAlias(path string) error {
	file, err := os.Open(path)
	if err != nil { return err }
	file = FileRetainer.Retain(FileRetainer{}, file)
	return nil
}

func completedWithMultipleEffects() {
	closer := acquireCancelCloser()
	closer = closeAndInvokeCancelCloser(closer)
}
`
	writeFixture(t, path, input)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/resourcealiasobligation.retainFile"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/resourcealiasobligation.closeAndRetainFile"
closes = [0]
takes-ownership = [0]
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/resourcealiasobligation.FileRetainer.Retain"
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/resourcealiasobligation.closeAndInvokeCancelCloser"
closes = [0]
invokes-cancellation = [0]
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
				"resource-not-closed": rules.SeverityWarn,
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
		t.Fatalf("returned-alias source ownership = %#v", result.Files)
	}
	if len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("returned-alias resource diagnostics = %#v", result.Files[0].Diagnostics)
	}
	wantStarts := make(map[int]bool)
	for _, function := range []string{"retainedAlias", "retainedMethodExpressionAlias"} {
		functionStart := strings.Index(input, "func " + function)
		acquisitionStart := strings.Index(
			input[functionStart:],
			"file, err := os.Open(path)",
		)
		wantStarts[functionStart + acquisitionStart] = true
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "resource-not-closed" ||
			!wantStarts[diagnostic.Range.Start] ||
			diagnostic.Range.End != diagnostic.Range.Start + len("file") {
			t.Fatalf("returned-alias resource diagnostic = %#v", diagnostic)
		}
		delete(wantStarts, diagnostic.Range.Start)
	}
	if len(wantStarts) != 0 {
		t.Fatalf("missing returned-alias resource diagnostics at %#v", wantStarts)
	}
}

func TestReturnedAliasStateUsesTheFinalAssignmentWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/aliasstate\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeFixture(
		t,
		path,
		`package sample

import (
	"database/sql"
	"os"
)

func completeAndRetainTransaction(transaction *sql.Tx) *sql.Tx { return transaction }
func closeAndRetainFile(file *os.File) *os.File { return file }

func replacedTransaction(database *sql.DB, replacement *sql.Tx) error {
	transaction, err := database.Begin()
	if err != nil { return err }
	transaction, transaction = completeAndRetainTransaction(transaction), replacement
	_, err = transaction.Exec("SELECT 1")
	return err
}

func replacedFile(path string, replacement *os.File) error {
	file, err := os.Open(path)
	if err != nil { return err }
	file, file = closeAndRetainFile(file), replacement
	_, err = file.Read(nil)
	return err
}
`,
	)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/aliasstate.completeAndRetainTransaction"
completes-transaction = [0]
takes-ownership = [0]
returns-alias = [{ result = 0, argument = 0 }]

[[functions]]
symbol = "example.com/aliasstate.closeAndRetainFile"
closes = [0]
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
				"resource-used-after-close": rules.SeverityWarn,
				"sql-transaction-used-after-completion": rules.SeverityWarn,
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
		t.Fatalf("returned-alias state source ownership = %#v", result.Files)
	}
	if len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf(
			"returned-alias replacement state diagnostics = %#v",
			result.Files[0].Diagnostics,
		)
	}
}

func BenchmarkReturnedAliasObligations(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/aliasobligationbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString(
		"package sample\nimport \"database/sql\"\nfunc retain(transaction *sql.Tx) *sql.Tx { return transaction }\n",
	)
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(database *sql.DB) error { transaction, err := database.Begin(); if err != nil { return err }; transaction = retain(transaction); return nil }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					"version = 1\n[[functions]]\nsymbol = \"example.com/aliasobligationbenchmark.retain\"\nreturns-alias = [{ result = 0, argument = 0 }]\n",
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
				"sql-transaction-not-completed": rules.SeverityWarn,
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
