package rulecatalog_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestUncheckedRowsErrorReportsOnlyNormallyReturningUncheckedPaths(t *testing.T) {
	t.Parallel()

	input := `package sample

import "database/sql"

func unchecked(rows *sql.Rows) {
	for rows.Next() {}
}

func partial(rows *sql.Rows, report bool) error {
	for rows.Next() {}
	if report {
		return rows.Err()
	}
	return nil
}

func ignored(rows *sql.Rows) {
	for rows.Next() {}
	_ = rows.Err()
}

func reassigned(rows, other *sql.Rows) error {
	for rows.Next() {}
	rows = other
	return rows.Err()
}

func checked(rows *sql.Rows) error {
	for rows.Next() {}
	return rows.Err()
}

func checkedAfterOuterBranch(rows *sql.Rows, iterate bool) error {
	if iterate {
		for rows.Next() {}
	}
	return rows.Err()
}

func checkedCondition(rows *sql.Rows) error {
	for rows.Next() {}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

type localRows struct{}
func (*localRows) Next() bool { return false }
func (*localRows) Err() error { return nil }
func unrelated(rows *localRows) {
	for rows.Next() {}
}

func expressionStatement(rows *sql.Rows) {
	for rows.Next() {}
	rows.Err()
}

func checkedBeforeReassignment(rows, other *sql.Rows) error {
	for rows.Next() {}
	err := rows.Err()
	rows = other
	return err
}

func passedToHandler(rows *sql.Rows) {
	for rows.Next() {}
	handle(rows.Err())
}

func handle(error) {}
`
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/rows\n\ngo 1.26.0\n")
	path := filepath.Join(root, "rows.go")
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
				"unchecked-rows-error": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 5 {
		t.Fatalf("unchecked rows diagnostics = %#v", result.Files)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "unchecked-rows-error" ||
			string(content[diagnostic.Range.Start:diagnostic.Range.End]) !=
				"rows.Next()" ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("unchecked rows diagnostic = %#v", diagnostic)
		}
	}
}

func TestUncheckedRowsErrorHandlesNestedFunctionsAndNoReturnPaths(t *testing.T) {
	t.Parallel()

	result := runUncheckedRowsError(
		t,
		`package sample

import "database/sql"

func nested(rows *sql.Rows, skip bool) error {
	if skip {
		return nil
	}
	for rows.Next() {
		if skip {
			break
		}
	}
	return nil
}

func branch(rows *sql.Rows, fail bool) error {
	for rows.Next() {}
	if fail {
		panic("failed")
	}
	return rows.Err()
}

func literal(rows *sql.Rows) func() {
	return func() {
		for rows.Next() {}
	}
}
`,
		"go1.26",
		nil,
	)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("nested diagnostics = %#v", result.Files)
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "unchecked-rows-error" {
			t.Fatalf("nested diagnostic = %#v", diagnostic)
		}
	}
}

func TestUncheckedRowsErrorRecognizesRowsReturnedByImportedHelpers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/rows\n\ngo 1.26.0\n")
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper
import "database/sql"
func Open() *sql.Rows { return nil }
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "rows.go"),
		`package sample
import "example.com/rows/helper"
func run() {
	rows := helper.Open()
	for rows.Next() {}
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
				"unchecked-rows-error": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("helper-returned rows diagnostics = %#v", result.Files)
	}
}

func TestUncheckedRowsErrorHonorsSharedEligibilityPolicies(t *testing.T) {
	t.Parallel()

	suppressed := runUncheckedRowsError(
		t,
		`package sample
import "database/sql"
func run(rows *sql.Rows) {
	//glippy:ignore unchecked-rows-error -- caller owns iteration error policy
	for rows.Next() {}
}
`,
		"go1.26",
		nil,
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed result = %#v", suppressed)
	}

	generated := runUncheckedRowsError(
		t,
		`// Code generated by fixture. DO NOT EDIT.
package sample
import "database/sql"
func run(rows *sql.Rows) { for rows.Next() {} }
`,
		"go1.26",
		nil,
	)
	if len(generated.Files) != 1 || len(generated.Files[0].Diagnostics) != 0 {
		t.Fatalf("generated result = %#v", generated)
	}

	illTyped := runUncheckedRowsError(
		t,
		`package sample
import "database/sql"
func run(rows *sql.Rows) { missing(); for rows.Next() {} }
`,
		"go1.26",
		nil,
	)
	if len(illTyped.LoadDiagnostics) == 0 ||
		len(illTyped.Files) != 1 ||
		len(illTyped.Files[0].Diagnostics) != 0 {
		t.Fatalf("ill-typed result = %#v", illTyped)
	}

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"unchecked-rows-error": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("old-version selection = %#v", selection)
	}

	severity := runUncheckedRowsError(
		t,
		`package sample
import "database/sql"
func run(rows *sql.Rows) { for rows.Next() {} }
`,
		"go1.26",
		map[string]rules.Severity{"unchecked-rows-error": rules.SeverityError},
	)
	if len(severity.Files) != 1 ||
		len(severity.Files[0].Diagnostics) != 1 ||
		severity.Files[0].Diagnostics[0].Severity != rules.SeverityError {
		t.Fatalf("severity result = %#v", severity)
	}
}

func TestCatalogDocumentsUncheckedRowsErrorWithoutAFix(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("unchecked-rows-error")
	if !found ||
		metadata.Requirement != rules.RequireControlFlow ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.NodeInterests) != 0 ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Presets) != 1 ||
		metadata.Presets[0] != rules.PresetSuspicious ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkUncheckedRowsErrorSharedCFG(b *testing.B) {
	var input strings.Builder
	input.WriteString("package sample\nimport \"database/sql\"\n")
	for index := range 100 {
		input.WriteString("func run")
		input.WriteString(strings.Repeat("x", index + 1))
		input.WriteString(
			"(rows *sql.Rows) error { for rows.Next() {} ; return rows.Err() }\n",
		)
	}
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/rowsbench\n\ngo 1.26.0\n",
	)
	writeFixture(b, filepath.Join(root, "rows.go"), input.String())
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		result, runErr := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{
					"unchecked-rows-error": rules.SeverityWarn,
				},
				SourceGoVersion: "go1.26",
			},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				ModuleMode: analysis.ModuleReadonly,
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
			b.Fatalf("benchmark result = %#v", result)
		}
	}
}

func runUncheckedRowsError(
	t testing.TB,
	input string,
	goVersion string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "go.mod"), "module example.com/rows\n\ngo 1.26.0\n")
	writeFixture(t, filepath.Join(root, "rows.go"), input)
	if overrides == nil {
		overrides = map[string]rules.Severity{"unchecked-rows-error": rules.SeverityWarn}
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
			Overrides: overrides,
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
