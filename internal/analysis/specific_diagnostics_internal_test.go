package analysis

import (
	"testing"

	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestPreferSpecificDiagnosticsLetsMustUseOwnDeferredFunctionDiscard(t *testing.T) {
	t.Parallel()

	range_ := source.Range{Start: 10, End: 17}
	diagnostics := []rules.Diagnostic{
		{RuleID: "deferred-function-not-called", Path: "sample.go", Range: range_},
		{RuleID: "must-use-result", Path: "sample.go", Range: range_},
		{RuleID: "discarded-error", Path: "sample.go", Range: range_},
		{
			RuleID: "deferred-function-not-called",
			Path: "sample.go",
			Range: source.Range{Start: 20, End: 27},
		},
	}
	result := preferSpecificDiagnostics(diagnostics)
	if len(result) != 2 ||
		result[0].RuleID != "must-use-result" ||
		result[1].RuleID != "deferred-function-not-called" ||
		result[1].Range.Start != 20 {
		t.Fatalf("specific diagnostics = %#v", result)
	}
}
