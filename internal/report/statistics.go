package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
)

// LintStatisticsMetric is one process-local cost measurement. Allocation
// values cover this process and exclude allocations in Go tool subprocesses.
type LintStatisticsMetric struct {
	Calls uint64 `json:"calls"`
	DurationNanoseconds int64 `json:"duration_ns"`
	Allocations uint64 `json:"allocations"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
}

// LintStatisticsPhase compares package loading with in-process analysis.
type LintStatisticsPhase struct {
	Name string `json:"name"`
	Metric LintStatisticsMetric `json:"metric"`
}

// LintStatisticsTier explains why a representation was constructed.
type LintStatisticsTier struct {
	Tier string `json:"tier"`
	Reasons []string `json:"reasons"`
	Metric LintStatisticsMetric `json:"metric"`
}

// LintStatisticsRule combines one rule's cost and final disposition counts.
type LintStatisticsRule struct {
	ID string `json:"id"`
	Tier string `json:"tier"`
	Calls uint64 `json:"calls"`
	DurationNanoseconds int64 `json:"duration_ns"`
	Allocations uint64 `json:"allocations"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
	Findings int `json:"findings"`
	Diagnostics int `json:"diagnostics"`
	Preexisting int `json:"preexisting"`
	Suppressed int `json:"suppressed"`
	Baselined int `json:"baselined"`
}

// LintStatisticsLoad explains an optional package-loading expansion.
type LintStatisticsLoad struct {
	Loaded bool `json:"loaded"`
	Reasons []string `json:"reasons"`
}

// LintStatisticsCache reports observable persistent cache outcomes. An
// invalidation is a found entry rejected by semantic validation; ordinary key
// changes appear as misses.
type LintStatisticsCache struct {
	Lookups uint64 `json:"lookups"`
	Hits uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
	Invalidations uint64 `json:"invalidations"`
	Writes uint64 `json:"writes"`
}

// LintStatistics is the schema-versioned opt-in lint profiling document.
type LintStatistics struct {
	SchemaVersion int `json:"schema_version"`
	Command string `json:"command"`
	Outcome Outcome `json:"outcome"`
	Complete bool `json:"complete"`
	MaximumTier string `json:"maximum_tier"`
	Packages int `json:"packages"`
	Files int `json:"files"`
	LoadedFiles int `json:"loaded_files"`
	Total LintStatisticsMetric `json:"total"`
	Phases []LintStatisticsPhase `json:"phases"`
	Tiers []LintStatisticsTier `json:"tiers"`
	Rules []LintStatisticsRule `json:"rules"`
	Cache LintStatisticsCache `json:"cache"`
	DependencySyntax LintStatisticsLoad `json:"dependency_syntax"`
	EffectFacts LintStatisticsLoad `json:"effect_facts"`
}

// NewLintStatistics combines analysis telemetry with final reporter-visible
// diagnostic dispositions in canonical order.
func NewLintStatistics(
	command string,
	category string,
	exitCode int,
	snapshot analysis.StatisticsSnapshot,
	results []analysis.Result,
) LintStatistics {
	tierMetrics := make([]LintStatisticsTier, 0, len(snapshot.Tiers))
	analysisMetric := analysis.StatisticsMetric{}
	selectedTiers := make(map[string]rules.Requirement)
	for _, tier := range snapshot.Tiers {
		tierMetrics = append(
			tierMetrics,
			LintStatisticsTier{
				Tier: statisticsRequirement(tier.Requirement),
				Reasons: append([]string{}, tier.Reasons...),
				Metric: lintStatisticsMetric(tier.Metric),
			},
		)
		addAnalysisStatisticsMetric(&analysisMetric, tier.Metric)
		for _, id := range tier.Reasons {
			selectedTiers[id] = tier.Requirement
		}
	}
	ruleMetrics := make(map[string]analysis.StatisticsRule, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		ruleMetrics[rule.ID] = rule
	}
	type disposition struct {
		diagnostics int
		preexisting int
		suppressed int
		baselined int
	}
	dispositions := make(map[string]disposition)
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			entry := dispositions[diagnostic.RuleID]
			entry.diagnostics++
			dispositions[diagnostic.RuleID] = entry
		}
		for _, diagnostic := range result.PreexistingDiagnostics {
			entry := dispositions[diagnostic.RuleID]
			entry.preexisting++
			dispositions[diagnostic.RuleID] = entry
		}
		for _, suppressed := range result.Suppressed {
			entry := dispositions[suppressed.Diagnostic.RuleID]
			entry.suppressed++
			dispositions[suppressed.Diagnostic.RuleID] = entry
		}
		for _, diagnostic := range result.Baselined {
			entry := dispositions[diagnostic.RuleID]
			entry.baselined++
			dispositions[diagnostic.RuleID] = entry
		}
	}
	ruleIDs := make([]string, 0, len(selectedTiers) + len(dispositions))
	seen := make(map[string]struct{}, len(selectedTiers) + len(dispositions))
	for id := range selectedTiers {
		seen[id] = struct{}{}
		ruleIDs = append(ruleIDs, id)
	}
	for id := range dispositions {
		if _, found := seen[id]; found {
			continue
		}
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	ruleStatistics := make([]LintStatisticsRule, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		metric := ruleMetrics[id]
		requirement := selectedTiers[id]
		if metric.ID != "" {
			requirement = metric.Requirement
		}
		counts := dispositions[id]
		ruleStatistics = append(
			ruleStatistics,
			LintStatisticsRule{
				ID: id,
				Tier: statisticsRequirement(requirement),
				Calls: metric.Metric.Calls,
				DurationNanoseconds: metric.Metric.Duration.Nanoseconds(),
				Allocations: metric.Metric.Allocations,
				AllocatedBytes: metric.Metric.AllocatedBytes,
				Findings: counts.diagnostics +
					counts.preexisting +
					counts.suppressed +
					counts.baselined,
				Diagnostics: counts.diagnostics,
				Preexisting: counts.preexisting,
				Suppressed: counts.suppressed,
				Baselined: counts.baselined,
			},
		)
	}
	return LintStatistics{
		SchemaVersion: SchemaVersion,
		Command: command,
		Outcome: Outcome{Category: category, ExitCode: exitCode},
		Complete: true,
		MaximumTier: statisticsRequirement(snapshot.MaximumRequirement),
		Packages: snapshot.Packages,
		Files: len(results),
		LoadedFiles: snapshot.Files,
		Total: lintStatisticsMetric(snapshot.Total),
		Phases: []LintStatisticsPhase{
			{
				Name: "package-loading",
				Metric: lintStatisticsMetric(snapshot.PackageLoads),
			},
			{Name: "analysis", Metric: lintStatisticsMetric(analysisMetric)},
		},
		Tiers: tierMetrics,
		Rules: ruleStatistics,
		Cache: LintStatisticsCache{
			Lookups: snapshot.Cache.Lookups,
			Hits: snapshot.Cache.Hits,
			Misses: snapshot.Cache.Misses,
			Invalidations: snapshot.Cache.Invalidations,
			Writes: snapshot.Cache.Writes,
		},
		DependencySyntax: LintStatisticsLoad{
			Loaded: snapshot.PackageLoads.Calls > 0 &&
				len(snapshot.DependencySyntaxReasons) > 0,
			Reasons: append([]string{}, snapshot.DependencySyntaxReasons...),
		},
		EffectFacts: LintStatisticsLoad{
			Loaded: snapshot.PackageLoads.Calls > 0 &&
				len(snapshot.EffectFactReasons) > 0,
			Reasons: append([]string{}, snapshot.EffectFactReasons...),
		},
	}
}

func addAnalysisStatisticsMetric(
	target *analysis.StatisticsMetric,
	metric analysis.StatisticsMetric,
) {
	target.Calls += metric.Calls
	target.Duration += metric.Duration
	target.Allocations += metric.Allocations
	target.AllocatedBytes += metric.AllocatedBytes
}

func lintStatisticsMetric(metric analysis.StatisticsMetric) LintStatisticsMetric {
	return LintStatisticsMetric{
		Calls: metric.Calls,
		DurationNanoseconds: metric.Duration.Nanoseconds(),
		Allocations: metric.Allocations,
		AllocatedBytes: metric.AllocatedBytes,
	}
}

func statisticsRequirement(requirement rules.Requirement) string {
	switch requirement {
	case rules.RequireLexical:
		return "lexical"
	case rules.RequireSyntax:
		return "syntax"
	case rules.RequireTypes:
		return "types"
	case rules.RequireControlFlow:
		return "control-flow"
	case rules.RequireSSA:
		return "ssa"
	default:
		return "unknown"
	}
}

// MarshalLintStatisticsJSON encodes one canonical stats document.
func MarshalLintStatisticsJSON(statistics LintStatistics) ([]byte, error) {
	return marshalJSON(statistics)
}

// RenderLintStatisticsText renders a concise, deterministic stats report.
func RenderLintStatisticsText(statistics LintStatistics) []byte {
	var output strings.Builder
	fmt.Fprintf(
		&output,
		"glippy %s statistics (schema %d)\noutcome: %s (exit %d), complete: %t\nmaximum tier: %s\npackages: %d, files: %d, loaded files: %d\n",
		statistics.Command,
		statistics.SchemaVersion,
		statistics.Outcome.Category,
		statistics.Outcome.ExitCode,
		statistics.Complete,
		statistics.MaximumTier,
		statistics.Packages,
		statistics.Files,
		statistics.LoadedFiles,
	)
	for _, phase := range statistics.Phases {
		fmt.Fprintf(
			&output,
			"phase %s: %s\n",
			phase.Name,
			formatStatisticsMetric(phase.Metric),
		)
	}
	for _, tier := range statistics.Tiers {
		fmt.Fprintf(
			&output,
			"tier %s: %s; reasons=%s\n",
			tier.Tier,
			formatStatisticsMetric(tier.Metric),
			strings.Join(tier.Reasons, ","),
		)
	}
	for _, rule := range statistics.Rules {
		fmt.Fprintf(
			&output,
			"rule %s (%s): calls=%d, time=%s, allocations=%d, bytes=%d, findings=%d, diagnostics=%d, preexisting=%d, suppressed=%d, baselined=%d\n",
			rule.ID,
			rule.Tier,
			rule.Calls,
			time.Duration(rule.DurationNanoseconds),
			rule.Allocations,
			rule.AllocatedBytes,
			rule.Findings,
			rule.Diagnostics,
			rule.Preexisting,
			rule.Suppressed,
			rule.Baselined,
		)
	}
	fmt.Fprintf(
		&output,
		"cache: lookups=%d, hits=%d, misses=%d, invalidations=%d, writes=%d\n",
		statistics.Cache.Lookups,
		statistics.Cache.Hits,
		statistics.Cache.Misses,
		statistics.Cache.Invalidations,
		statistics.Cache.Writes,
	)
	fmt.Fprintf(
		&output,
		"dependency syntax: loaded=%t; reasons=%s\neffect facts: loaded=%t; reasons=%s\n",
		statistics.DependencySyntax.Loaded,
		strings.Join(statistics.DependencySyntax.Reasons, ","),
		statistics.EffectFacts.Loaded,
		strings.Join(statistics.EffectFacts.Reasons, ","),
	)
	return []byte(output.String())
}

func formatStatisticsMetric(metric LintStatisticsMetric) string {
	return fmt.Sprintf(
		"calls=%d, time=%s, allocations=%d, bytes=%d",
		metric.Calls,
		time.Duration(metric.DurationNanoseconds),
		metric.Allocations,
		metric.AllocatedBytes,
	)
}
