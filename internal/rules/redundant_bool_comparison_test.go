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

func TestRedundantBoolComparisonReportsTypedBooleanConstantsWithSafeFixes(t *testing.T) {
	t.Parallel()

	input := `package sample
type namedBool bool
type alias = bool
const enabled = true
func run(ready bool, named namedBool, aliased alias) {
	_ = ready == true
	_ = false != ready
	_ = ready != true
	_ = false == ready
	_ = named == true
	_ = aliased == true
	_ = ready == enabled
}
`
	result := runRedundantBoolComparison(t, input, nil)
	want := []struct {
		expression  string
		replacement string
	}{
		{expression: "ready == true", replacement: "ready"},
		{expression: "false != ready", replacement: "ready"},
		{expression: "ready != true", replacement: "!ready"},
		{expression: "false == ready", replacement: "!ready"},
		{expression: "aliased == true", replacement: "aliased"},
		{expression: "ready == enabled", replacement: "ready"},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("result = %#v, want %d diagnostics", result, len(want))
	}
	searchStart := 0
	for index, expected := range want {
		offset := strings.Index(input[searchStart:], expected.expression)
		if offset < 0 {
			t.Fatalf("input does not contain %q after %d", expected.expression, searchStart)
		}
		offset += searchStart
		diagnostic := result.Files[0].Diagnostics[index]
		wantRange := source.Range{Start: offset, End: offset + len(expected.expression)}
		if diagnostic.RuleID != "redundant-bool-comparison" ||
			diagnostic.MessageKey != "omit-comparison" || diagnostic.Range != wantRange ||
			len(diagnostic.Fixes) != 1 || diagnostic.Fixes[0].Name != "simplify-comparison" ||
			diagnostic.Fixes[0].Safety != rules.FixSafe ||
			len(diagnostic.Fixes[0].Edits) != 1 ||
			diagnostic.Fixes[0].Edits[0] != (rules.Edit{Range: wantRange, NewText: expected.replacement}) {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
		searchStart = offset + len(expected.expression)
	}
}

func TestRedundantBoolComparisonReportsWithoutFixWhenRemovedTriviaContainsComments(t *testing.T) {
	t.Parallel()

	result := runRedundantBoolComparison(t, `package sample
func run(ready bool) {
	_ = ready /* keep operator context */ == true
	_ = ready == /* keep constant context */ true
}
`, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("result = %#v, want two diagnostics", result)
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "redundant-bool-comparison" || len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic = %#v, want comment-preserving no-fix finding", diagnostic)
		}
	}
}

func TestRedundantBoolComparisonPreservesPrecedenceAndRetainedComments(t *testing.T) {
	t.Parallel()

	input := `package sample
func run(left, right bool) {
	_ = (left && right) != true
	_ = (left /* keep operand context */ && right) == false
	_ = !left == false
}
`
	result := runRedundantBoolComparison(t, input, nil)
	want := []struct {
		expression  string
		replacement string
	}{
		{expression: "(left && right) != true", replacement: "!(left && right)"},
		{
			expression:  "(left /* keep operand context */ && right) == false",
			replacement: "!(left /* keep operand context */ && right)",
		},
		{expression: "!left == false", replacement: "left"},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("result = %#v, want %d diagnostics", result, len(want))
	}
	searchStart := 0
	for index, expected := range want {
		offset := strings.Index(input[searchStart:], expected.expression)
		if offset < 0 {
			t.Fatalf("input does not contain %q after %d", expected.expression, searchStart)
		}
		offset += searchStart
		diagnostic := result.Files[0].Diagnostics[index]
		if len(diagnostic.Fixes) != 1 || len(diagnostic.Fixes[0].Edits) != 1 ||
			diagnostic.Fixes[0].Edits[0].NewText != expected.replacement {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
		searchStart = offset + len(expected.expression)
	}
}

func TestRedundantBoolComparisonAcceptsNonConstantsAndNonBooleanOperands(t *testing.T) {
	t.Parallel()

	result := runRedundantBoolComparison(t, `package sample
func run(ready, other bool, dynamic any, number int) {
	_ = ready == other
	_ = dynamic == true
	_ = number == 1
}
func shadowed(true bool, ready bool) {
	_ = ready == true
}
`, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestRedundantBoolComparisonHonorsSuppressionsGeneratedFilesTypeErrorsAndSeverity(t *testing.T) {
	t.Parallel()

	suppressed := runRedundantBoolComparison(t, `package sample
func run(ready bool) {
	//gox:ignore redundant-bool-comparison -- explicit comparison
	_ = ready == true
}
`, nil)
	if len(suppressed.Files) != 1 || len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed result = %#v", suppressed)
	}

	generated := runRedundantBoolComparison(t, `// Code generated by fixture. DO NOT EDIT.
package sample
func run(ready bool) { _ = ready == true }
`, nil)
	if len(generated.Files) != 1 || len(generated.Files[0].Diagnostics) != 0 {
		t.Fatalf("generated result = %#v", generated)
	}

	illTyped := runRedundantBoolComparison(t, `package sample
func run(ready bool) { missing(); _ = ready == true }
`, nil)
	if len(illTyped.Files) != 1 || len(illTyped.Files[0].Diagnostics) != 0 ||
		len(illTyped.LoadDiagnostics) == 0 {
		t.Fatalf("ill-typed result = %#v", illTyped)
	}

	errorSeverity := runRedundantBoolComparison(t, `package sample
func run(ready bool) { _ = ready == true }
`, map[string]rules.Severity{"redundant-bool-comparison": rules.SeverityError})
	if len(errorSeverity.Files) != 1 || len(errorSeverity.Files[0].Diagnostics) != 1 ||
		errorSeverity.Files[0].Diagnostics[0].Severity != rules.SeverityError {
		t.Fatalf("severity result = %#v", errorSeverity)
	}

	disabled := runRedundantBoolComparison(t, `package sample
func run(ready bool) { _ = ready == true }
`, map[string]rules.Severity{
		"nilness":                   rules.SeverityWarn,
		"redundant-bool-comparison": rules.SeverityOff,
	})
	if len(disabled.Files) != 1 || len(disabled.Files[0].Diagnostics) != 0 {
		t.Fatalf("disabled result = %#v", disabled)
	}
}

func TestDefaultRegistryDocumentsRedundantBoolComparisonSafeFix(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("redundant-bool-comparison")
	if !found || metadata.Requirement != rules.RequireTypes ||
		metadata.DefaultSeverity != rules.SeverityWarn || metadata.MinimumGoVersion != "1.26" ||
		metadata.RunOnGenerated || metadata.RunDespiteTypeErrors ||
		len(metadata.Presets) != 1 || metadata.Presets[0] != rules.PresetStyle ||
		len(metadata.NodeInterests) != 1 || metadata.NodeInterests[0] != rules.NodeBinaryExpr ||
		len(metadata.Fixes) != 1 || metadata.Fixes[0] != (rules.FixMetadata{
		Name:        "simplify-comparison",
		Description: "replace the comparison with an equivalent boolean expression",
		Safety:      rules.FixSafe,
	}) {
		t.Fatalf("metadata = %#v, found = %v", metadata, found)
	}
}

func BenchmarkRedundantBoolComparisonSharedTypes(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/redundantboolbenchmark\n\ngo 1.26.0\n"),
		0o600,
	); err != nil {
		b.Fatal(err)
	}
	var input strings.Builder
	input.WriteString("package sample\nfunc inspect(values [100]bool) {\n")
	for index := range 100 {
		fmt.Fprintf(&input, "_ = values[%d] == true\n", index)
	}
	input.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(input.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	loaded, err := analysis.LoadPackages(context.Background(), analysis.PackageLoadOptions{
		Dir: root, Patterns: []string{"."}, Requirement: rules.RequireTypes,
	})
	if err != nil {
		b.Fatal(err)
	}
	registry, err := rules.NewDefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetStyle, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		diagnostics, err := analysis.RunTypes(context.Background(), loaded, registry, selection)
		if err != nil {
			b.Fatal(err)
		}
		if len(diagnostics) != 100 {
			b.Fatalf("diagnostics = %d, want 100", len(diagnostics))
		}
	}
}

func runRedundantBoolComparison(
	t *testing.T,
	input string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/redundantbool\n\ngo 1.26.0\n"),
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
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetStyle, Overrides: overrides},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
