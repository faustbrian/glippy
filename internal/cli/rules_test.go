package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	glippyreport "github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestRunRulesListsAndFiltersCanonicalMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		explainMetadataRule{
			metadata: rules.Metadata{
				ID: "delta-rule",
				Summary: "delta",
				Documentation: "Delta.",
				DefaultSeverity: rules.SeverityOff,
				Presets: []rules.Preset{rules.PresetNursery},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireTypes,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				Categories: []rules.Category{rules.CategorySuspicious},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
		explainMetadataRule{
			metadata: rules.Metadata{
				ID: "alpha-rule",
				Summary: "alpha",
				Documentation: "Alpha.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSyntax,
				NodeInterests: []rules.NodeKind{rules.NodeCallExpr},
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
		explainMetadataRule{
			metadata: rules.Metadata{
				ID: "beta-rule",
				Summary: "beta",
				Documentation: "Beta.",
				DefaultSeverity: rules.SeverityOff,
				Presets: []rules.Preset{rules.PresetPedantic},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSSA,
				Categories: []rules.Category{rules.CategoryMaintainability},
				Fixes: []rules.FixMetadata{
					{
						Name: "rewrite",
						Description: "rewrite beta",
						Safety: rules.FixSuggestion,
					},
				},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
		explainMetadataRule{
			metadata: rules.Metadata{
				ID: "gamma-rule",
				Summary: "gamma",
				Documentation: "Gamma.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireSSA,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		arguments []string
		want string
	}{
		{
			arguments: []string{"rules"},
			want: "alpha-rule\tdefault=warn\tpresets=correctness\ttier=syntax\tfixes=none\n" +
				"beta-rule\tdefault=off\tpresets=pedantic\ttier=ssa\tfixes=suggestion\n" +
				"delta-rule\tdefault=off\tpresets=nursery\ttier=types\tfixes=none\n" +
				"gamma-rule\tdefault=warn\tpresets=correctness\ttier=ssa\tfixes=none\n",
		},
		{
			arguments: []string{"rules", "--preset=nursery"},
			want: "delta-rule\tdefault=off\tpresets=nursery\ttier=types\tfixes=none\n",
		},
		{
			arguments: []string{"rules", "--preset", "correctness", "--tier=ssa"},
			want: "gamma-rule\tdefault=warn\tpresets=correctness\ttier=ssa\tfixes=none\n",
		},
		{
			arguments: []string{"rules", "--fixable"},
			want: "beta-rule\tdefault=off\tpresets=pedantic\ttier=ssa\tfixes=suggestion\n",
		},
	}

	for _, test := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runRules(
			context.Background(),
			test.arguments,
			&stdout,
			&stderr,
			registry,
		)
		if exitCode != ExitSuccess || stdout.String() != test.want || stderr.Len() != 0 {
			t.Fatalf(
				"runRules(%q) = exit %d, stdout %q, stderr %q",
				test.arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunRulesRejectsInvalidFilters(t *testing.T) {
	t.Parallel()

	for _, arguments := range
		[][]string{
			{"rules", "--preset=unknown"},
			{"rules", "--tier=control-flow"},
			{"rules", "--fixable", "--fixable"},
			{"rules", "path"},
		} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(arguments, failingReader{}, &stdout, &stderr)
		if exitCode != ExitInvalidInvocation ||
			stdout.Len() != 0 ||
			stderr.String() != rulesUsage {
			t.Fatalf(
				"Run(%q) = exit %d, stdout %q, stderr %q",
				arguments,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestRunExplainJSONRendersVersionedCanonicalMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rules.NewRegistry(
		explainMetadataRule{
			metadata: rules.Metadata{
				ID: "json-rule",
				Summary: "json summary",
				Documentation: "JSON documentation.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{
					rules.PresetCorrectness,
					rules.PresetPedantic,
				},
				MinimumGoVersion: "1.22",
				Requirement: rules.RequireControlFlow,
				RunDespiteTypeErrors: true,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Fixes: []rules.FixMetadata{
					{
						Name: "rewrite",
						Description: "rewrite safely",
						Safety: rules.FixSafe,
					},
				},
				Options: []rules.OptionMetadata{
					{
						Name: "limit",
						Summary: "maximum limit",
						Kind: rules.OptionInteger,
						Default: ruleOptionValue(rules.IntegerOption(3)),
						Minimum: ruleInt64Pointer(1),
						Maximum: ruleInt64Pointer(10),
					},
				},
				KnownLimitations: []string{"one limitation"},
				Examples: []rules.Example{
					{Title: "sample", Incorrect: "bad()", Correct: "good()"},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, arguments := range
		[][]string{{"explain", "json-rule", "--json"}, {"explain", "--json", "json-rule"}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := runExplain(context.Background(), arguments, &stdout, &stderr, registry)
		if exitCode != ExitSuccess || stderr.Len() != 0 {
			t.Fatalf(
				"runExplain(%q) = exit %d, stderr %q",
				arguments,
				exitCode,
				stderr.String(),
			)
		}
		var result glippyreport.RuleExplanation
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("decode explain JSON: %v\n%s", err, stdout.Bytes())
		}
		if result.SchemaVersion != glippyreport.SchemaVersion ||
			result.Command != "explain" ||
			result.Rule.ID != "json-rule" ||
			result.Rule.AnalysisTier != "control-flow" ||
			len(result.Rule.Options) != 1 ||
			result.Rule.Options[0].Default != "3" ||
			result.Rule.Options[0].Minimum == nil ||
			*result.Rule.Options[0].Minimum != 1 ||
			result.Rule.Options[0].Maximum == nil ||
			*result.Rule.Options[0].Maximum != 10 ||
			!strings.HasSuffix(stdout.String(), "\n") {
			t.Fatalf("explain JSON = %#v\n%s", result, stdout.Bytes())
		}
	}
}

func ruleOptionValue(value rules.OptionValue) *rules.OptionValue {
	return &value
}

func ruleInt64Pointer(value int64) *int64 {
	return &value
}
