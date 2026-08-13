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
			Range: source.Range{Start: 10, End: 20},
			Fixes: []rules.Fix{
				{Name: "suggest", Safety: rules.FixSuggestion},
				{Name: "rewrite", Safety: rules.FixSafe},
				{Name: "unsafe", Safety: rules.FixUnsafe},
			},
		},
		{
			RuleID: "review-only",
			Range: source.Range{Start: 30, End: 40},
			Fixes: []rules.Fix{{Name: "suggest", Safety: rules.FixSuggestion}},
		},
	}

	selections, err := fixengine.SelectSafe(diagnostics)

	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 1 ||
		selections[0].Diagnostic.RuleID != "safe-rule" ||
		selections[0].FixName != "rewrite" {
		t.Fatalf("SelectSafe() = %#v", selections)
	}
}

func TestSelectSafeFixesRejectsAmbiguousAutomaticChoices(t *testing.T) {
	t.Parallel()

	diagnostics := []rules.Diagnostic{
		{
			RuleID: "ambiguous-rule",
			Range: source.Range{Start: 10, End: 20},
			Fixes: []rules.Fix{
				{Name: "first", Safety: rules.FixSafe},
				{Name: "second", Safety: rules.FixSafe},
			},
		},
	}

	selections, err := fixengine.SelectSafe(diagnostics)

	if err == nil || !strings.Contains(err.Error(), "ambiguous-rule") || len(selections) != 0 {
		t.Fatalf("SelectSafe() = %#v, %v", selections, err)
	}
}

func TestSelectFixesChoosesOnlyExplicitlyAuthorizedSafetyClasses(t *testing.T) {
	t.Parallel()

	diagnostics := []rules.Diagnostic{
		diagnosticWithFix("safe-rule", rules.FixSafe, "safe"),
		diagnosticWithFix("suggestion-rule", rules.FixSuggestion, "suggestion"),
		diagnosticWithFix("unsafe-rule", rules.FixUnsafe, "unsafe"),
	}

	tests := []struct {
		name string
		options fixengine.SelectionOptions
		want []string
	}{
		{
			name: "safe",
			options: fixengine.SelectionOptions{AllowSafe: true},
			want: []string{"safe-rule"},
		},
		{
			name: "suggestion",
			options: fixengine.SelectionOptions{AllowSuggestion: true},
			want: []string{"suggestion-rule"},
		},
		{
			name: "unsafe",
			options: fixengine.SelectionOptions{AllowUnsafe: true},
			want: []string{"unsafe-rule"},
		},
		{
			name: "all",
			options: fixengine.SelectionOptions{
				AllowSafe: true,
				AllowSuggestion: true,
				AllowUnsafe: true,
			},
			want: []string{"safe-rule", "suggestion-rule", "unsafe-rule"},
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				selections, err := fixengine.Select(diagnostics, test.options)
				if err != nil {
					t.Fatal(err)
				}
				if len(selections) != len(test.want) {
					t.Fatalf("Select() = %#v, want %v", selections, test.want)
				}
				for index, want := range test.want {
					if selections[index].Diagnostic.RuleID != want {
						t.Fatalf(
							"Select()[%d] = %#v, want %q",
							index,
							selections[index],
							want,
						)
					}
				}
			},
		)
	}
}

func TestSelectFixesRejectsAmbiguousAuthorizedAlternatives(t *testing.T) {
	t.Parallel()

	diagnostic := diagnosticWithFix("ambiguous-rule", rules.FixSafe, "safe")
	diagnostic.Fixes = append(
		diagnostic.Fixes,
		rules.Fix{Name: "suggestion", Safety: rules.FixSuggestion},
	)

	selections, err := fixengine.Select(
		[]rules.Diagnostic{diagnostic},
		fixengine.SelectionOptions{AllowSafe: true, AllowSuggestion: true},
	)
	if err == nil || !strings.Contains(err.Error(), "ambiguous-rule") || len(selections) != 0 {
		t.Fatalf("Select() = %#v, %v", selections, err)
	}
}

func diagnosticWithFix(ruleID string, safety rules.FixSafety, name string) rules.Diagnostic {
	return rules.Diagnostic{RuleID: ruleID, Fixes: []rules.Fix{{Name: name, Safety: safety}}}
}
