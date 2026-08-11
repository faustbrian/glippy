package report_test

import (
	"strings"
	"testing"

	"github.com/faustbrian/gox/internal/report"
	"github.com/faustbrian/gox/internal/rules"
)

type documentedRule struct {
	metadata rules.Metadata
}

func TestRenderRuleTextKeepsEmptyContractsAndDeprecationVisible(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(documentedRule{metadata: rules.Metadata{
		ID:               "deprecated-rule",
		Summary:          "reports a deprecated pattern",
		Documentation:    "Use the replacement rule for new code.",
		DefaultSeverity:  rules.SeverityOff,
		Presets:          []rules.Preset{rules.PresetMigration},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireLexical,
		Categories:       []rules.Category{rules.CategoryMigration},
		Deprecation: &rules.Deprecation{
			Since:       "1.3",
			Replacement: "replacement-rule",
			Message:     "use the replacement rule",
		},
		Examples: []rules.Example{{Incorrect: "old()", Correct: "new()"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	output, found := report.RenderRuleText(registry, "deprecated-rule")
	if !found {
		t.Fatal("RenderRuleText() did not find the deprecated rule")
	}
	for _, contract := range []string{
		"node interests: none\n",
		"deprecated since 1.3: use the replacement rule\n",
		"replacement: replacement-rule\n",
		"fixes:\n  none\n",
		"configuration:\n  none\n",
		"known limitations:\n  none documented\n",
		"examples:\n  example 1\n",
	} {
		if !strings.Contains(string(output), contract) {
			t.Fatalf("RenderRuleText() output does not contain %q:\n%s", contract, output)
		}
	}
}

func (r documentedRule) Metadata() rules.Metadata { return r.metadata }

func TestRenderRuleTextIncludesTypeErrorPolicy(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(documentedRule{metadata: rules.Metadata{
		ID:                   "typed-rule",
		Summary:              "reports a typed defect",
		Documentation:        "Runs when partial type information is sufficient.",
		DefaultSeverity:      rules.SeverityWarn,
		Presets:              []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion:     "1.22",
		Requirement:          rules.RequireTypes,
		NodeInterests:        []rules.NodeKind{rules.NodeCallExpr},
		RunDespiteTypeErrors: true,
		Categories:           []rules.Category{rules.CategoryCorrectness},
		Examples:             []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	output, found := report.RenderRuleText(registry, "typed-rule")
	if !found || !strings.Contains(string(output), "type-error packages: included\n") {
		t.Fatalf("RenderRuleText() = %q, %t", output, found)
	}
}

func TestRenderRuleTextUsesCanonicalMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(documentedRule{metadata: rules.Metadata{
		ID:               "example-rule",
		Summary:          "reports one observable defect",
		Documentation:    "Reports calls whose result is ignored.",
		DefaultSeverity:  rules.SeverityWarn,
		Presets:          []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement:      rules.RequireSyntax,
		NodeInterests:    []rules.NodeKind{rules.NodeCallExpr},
		RunOnGenerated:   false,
		Categories:       []rules.Category{rules.CategoryCorrectness},
		Fixes: []rules.FixMetadata{{
			Name:        "rewrite",
			Description: "replace the ignored call",
			Safety:      rules.FixSafe,
		}},
		Options: []rules.OptionMetadata{{
			Name:     "allow-comment",
			Summary:  "allow an explanatory comment",
			Kind:     rules.OptionBoolean,
			Required: false,
			Default:  reportOptionValue(rules.BooleanOption(false)),
		}},
		KnownLimitations: []string{"does not inspect generated files"},
		Examples: []rules.Example{{
			Title:     "ignored result",
			Incorrect: "target()\n",
			Correct:   "_ = target()\n",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	got, found := report.RenderRuleText(registry, "example-rule")
	if !found {
		t.Fatal("RenderRuleText() did not find the registered rule")
	}
	want := "example-rule\n" +
		"reports one observable defect\n\n" +
		"Reports calls whose result is ignored.\n\n" +
		"default severity: warn\n" +
		"presets: correctness\n" +
		"minimum Go: 1.22\n" +
		"analysis tier: syntax\n" +
		"node interests: call-expr\n" +
		"generated files: excluded\n" +
		"type-error packages: not applicable\n" +
		"categories: correctness\n\n" +
		"fixes:\n" +
		"  rewrite [safe]: replace the ignored call\n\n" +
		"configuration:\n" +
		"  allow-comment (boolean, optional, default false): allow an explanatory comment\n\n" +
		"known limitations:\n" +
		"  - does not inspect generated files\n\n" +
		"examples:\n" +
		"  ignored result\n" +
		"    incorrect:\n" +
		"      target()\n" +
		"    correct:\n" +
		"      _ = target()\n"
	if string(got) != want {
		t.Fatalf("RenderRuleText() =\n%s\nwant:\n%s", got, want)
	}
	if _, found := report.RenderRuleText(registry, "missing-rule"); found {
		t.Fatal("RenderRuleText() found an unknown rule")
	}
}

func reportOptionValue(value rules.OptionValue) *rules.OptionValue { return &value }
