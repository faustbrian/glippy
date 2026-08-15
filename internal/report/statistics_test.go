package report_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/report"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/suppressions"
)

func TestLintStatisticsOrdersRulesAndCountsFinalDispositions(t *testing.T) {
	t.Parallel()

	statistics := report.NewLintStatistics(
		"lint",
		"findings",
		1,
		analysis.StatisticsSnapshot{
			MaximumRequirement: rules.RequireControlFlow,
			Total: analysis.StatisticsMetric{
				Calls: 1,
				Duration: 15 * time.Millisecond,
				Allocations: 40,
				AllocatedBytes: 400,
			},
			PackageLoads: analysis.StatisticsMetric{
				Calls: 1,
				Duration: 10 * time.Millisecond,
				Allocations: 20,
				AllocatedBytes: 200,
			},
			Tiers: []analysis.StatisticsTier{
				{
					Requirement: rules.RequireSyntax,
					Reasons: []string{"alpha"},
					Metric: analysis.StatisticsMetric{Calls: 1},
				},
				{
					Requirement: rules.RequireControlFlow,
					Reasons: []string{"beta"},
					Metric: analysis.StatisticsMetric{Calls: 1},
				},
			},
			Rules: []analysis.StatisticsRule{
				{
					ID: "beta",
					Requirement: rules.RequireControlFlow,
					Metric: analysis.StatisticsMetric{Calls: 2},
				},
				{
					ID: "alpha",
					Requirement: rules.RequireSyntax,
					Metric: analysis.StatisticsMetric{Calls: 3},
				},
			},
			Cache: analysis.StatisticsCache{
				Lookups: 2,
				Hits: 1,
				Invalidations: 1,
				Writes: 1,
			},
			DependencySyntaxReasons: []string{"alpha"},
			EffectFactReasons: []string{"beta"},
			Packages: 2,
			Files: 3,
		},
		[]analysis.Result{
			{
				Diagnostics: []rules.Diagnostic{{RuleID: "beta"}},
				PreexistingDiagnostics: []rules.Diagnostic{{RuleID: "alpha"}},
				Suppressed: []suppressions.SuppressedDiagnostic{
					{Diagnostic: rules.Diagnostic{RuleID: "alpha"}},
				},
				Baselined: []rules.Diagnostic{{RuleID: "beta"}},
			},
		},
	)
	if statistics.SchemaVersion != 1 ||
		statistics.Command != "lint" ||
		!statistics.Complete ||
		statistics.Outcome.ExitCode != 1 ||
		statistics.Outcome.Category != "findings" ||
		statistics.MaximumTier != "control-flow" ||
		statistics.Packages != 2 ||
		statistics.Files != 1 ||
		statistics.LoadedFiles != 3 ||
		len(statistics.Rules) != 2 {
		t.Fatalf("statistics = %#v", statistics)
	}
	if ids := []string{statistics.Rules[0].ID, statistics.Rules[1].ID};
		!reflect.DeepEqual(ids, []string{"alpha", "beta"}) {
		t.Fatalf("rule order = %q", ids)
	}
	if alpha := statistics.Rules[0];
		alpha.Findings != 2 ||
			alpha.Preexisting != 1 ||
			alpha.Suppressed != 1 ||
			alpha.Calls != 3 {
		t.Fatalf("alpha statistics = %#v", alpha)
	}
	if beta := statistics.Rules[1];
		beta.Findings != 2 ||
			beta.Diagnostics != 1 ||
			beta.Baselined != 1 ||
			beta.Calls != 2 {
		t.Fatalf("beta statistics = %#v", beta)
	}
	if statistics.Cache.Lookups != 2 ||
		statistics.Cache.Hits != 1 ||
		statistics.Cache.Invalidations != 1 ||
		statistics.Cache.Writes != 1 ||
		!statistics.DependencySyntax.Loaded ||
		!statistics.EffectFacts.Loaded {
		t.Fatalf("load and cache statistics = %#v", statistics)
	}

	encoded, err := report.MarshalLintStatisticsJSON(statistics)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema_version"] != float64(1) || decoded["command"] != "lint" {
		t.Fatalf("encoded statistics = %s", encoded)
	}
	text := string(report.RenderLintStatisticsText(statistics))
	for _, expected := range
		[]string{
			"outcome: findings (exit 1), complete: true",
			"maximum tier: control-flow",
			"phase package-loading:",
			"rule alpha (syntax): calls=3, time=0s, allocations=0, bytes=0, findings=2, diagnostics=0, preexisting=1, suppressed=1, baselined=0",
			"cache: lookups=2, hits=1, misses=0, invalidations=1, writes=1",
			"effect facts: loaded=true; reasons=beta",
		} {
		if !strings.Contains(text, expected) {
			t.Fatalf("text statistics missing %q:\n%s", expected, text)
		}
	}
}
