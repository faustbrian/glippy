package fix_test

import (
	"strings"
	"testing"

	fixengine "github.com/faustbrian/gox/internal/fix"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

func TestSelectSafeFixesChoosesOneUnambiguousSafeFixPerDiagnostic(t *testing.T) {
	t.Parallel()

	diagnostics := []rules.Diagnostic{
		{
			RuleID: "safe-rule",
			Range:  source.Range{Start: 10, End: 20},
			Fixes: []rules.Fix{
				{Name: "suggest", Safety: rules.FixSuggestion},
				{Name: "rewrite", Safety: rules.FixSafe},
				{Name: "unsafe", Safety: rules.FixUnsafe},
			},
		},
		{
			RuleID: "review-only",
			Range:  source.Range{Start: 30, End: 40},
			Fixes:  []rules.Fix{{Name: "suggest", Safety: rules.FixSuggestion}},
		},
	}

	selections, err := fixengine.SelectSafe(diagnostics)

	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 1 || selections[0].Diagnostic.RuleID != "safe-rule" ||
		selections[0].FixName != "rewrite" {
		t.Fatalf("SelectSafe() = %#v", selections)
	}
}

func TestSelectSafeFixesRejectsAmbiguousAutomaticChoices(t *testing.T) {
	t.Parallel()

	diagnostics := []rules.Diagnostic{{
		RuleID: "ambiguous-rule",
		Range:  source.Range{Start: 10, End: 20},
		Fixes: []rules.Fix{
			{Name: "first", Safety: rules.FixSafe},
			{Name: "second", Safety: rules.FixSafe},
		},
	}}

	selections, err := fixengine.SelectSafe(diagnostics)

	if err == nil || !strings.Contains(err.Error(), "ambiguous-rule") || len(selections) != 0 {
		t.Fatalf("SelectSafe() = %#v, %v", selections, err)
	}
}
