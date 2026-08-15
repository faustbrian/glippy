package analysis

import (
	"context"
	"runtime"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/faustbrian/glippy/internal/rules"
)

// Statistics collects opt-in process-local analysis measurements. Callers
// share one collector across every file and package in one CLI invocation.
type Statistics struct {
	mu sync.Mutex
	started statisticsMeasurement
	maximumRequirement rules.Requirement
	tierReasons map[rules.Requirement]map[string]struct{}
	dependencySyntaxReasons map[string]struct{}
	effectFactReasons map[string]struct{}
	packageLoads StatisticsMetric
	tiers map[rules.Requirement]StatisticsMetric
	rules map[string]StatisticsRule
	cache StatisticsCache
	packageIDs map[string]struct{}
	filePaths map[string]struct{}
}

// StatisticsMetric is one aggregate process-local measurement.
type StatisticsMetric struct {
	Calls uint64
	Duration time.Duration
	Allocations uint64
	AllocatedBytes uint64
}

// StatisticsRule is one rule's aggregate execution cost.
type StatisticsRule struct {
	ID string
	Requirement rules.Requirement
	Metric StatisticsMetric
}

// StatisticsTier explains and measures one analysis representation.
type StatisticsTier struct {
	Requirement rules.Requirement
	Reasons []string
	Metric StatisticsMetric
}

// StatisticsCache records observable persistent analysis-cache outcomes.
type StatisticsCache struct {
	Lookups uint64
	Hits uint64
	Misses uint64
	Invalidations uint64
	Writes uint64
}

// StatisticsSnapshot is one immutable, canonically ordered collector view.
type StatisticsSnapshot struct {
	Total StatisticsMetric
	MaximumRequirement rules.Requirement
	DependencySyntaxReasons []string
	EffectFactReasons []string
	PackageLoads StatisticsMetric
	Tiers []StatisticsTier
	Rules []StatisticsRule
	Cache StatisticsCache
	Packages int
	Files int
}

type statisticsContextKey struct{}

type statisticsMeasurement struct {
	started time.Time
	mallocs uint64
	totalAlloc uint64
}

// NewStatistics creates an empty opt-in analysis statistics collector.
func NewStatistics() *Statistics {
	statistics := &Statistics{
		tierReasons: make(map[rules.Requirement]map[string]struct{}),
		dependencySyntaxReasons: make(map[string]struct{}),
		effectFactReasons: make(map[string]struct{}),
		tiers: make(map[rules.Requirement]StatisticsMetric),
		rules: make(map[string]StatisticsRule),
		packageIDs: make(map[string]struct{}),
		filePaths: make(map[string]struct{}),
	}
	statistics.started = beginStatisticsMeasurement(statistics)
	return statistics
}

func withStatistics(ctx context.Context, statistics *Statistics) context.Context {
	if statistics == nil {
		return ctx
	}
	return context.WithValue(ctx, statisticsContextKey{}, statistics)
}

func statisticsFromContext(ctx context.Context) *Statistics {
	if ctx == nil {
		return nil
	}
	statistics, _ := ctx.Value(statisticsContextKey{}).(*Statistics)
	return statistics
}

func beginStatisticsMeasurement(statistics *Statistics) statisticsMeasurement {
	if statistics == nil {
		return statisticsMeasurement{}
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return statisticsMeasurement{
		started: time.Now(),
		mallocs: memory.Mallocs,
		totalAlloc: memory.TotalAlloc,
	}
}

func finishStatisticsMeasurement(started statisticsMeasurement) StatisticsMetric {
	if started.started.IsZero() {
		return StatisticsMetric{}
	}
	finished := time.Now()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return StatisticsMetric{
		Calls: 1,
		Duration: finished.Sub(started.started),
		Allocations: positiveDelta(memory.Mallocs, started.mallocs),
		AllocatedBytes: positiveDelta(memory.TotalAlloc, started.totalAlloc),
	}
}

func positiveDelta(after, before uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func addStatisticsMetric(target *StatisticsMetric, measured StatisticsMetric) {
	target.Calls += measured.Calls
	target.Duration += measured.Duration
	target.Allocations += measured.Allocations
	target.AllocatedBytes += measured.AllocatedBytes
}

func (s *Statistics) recordTier(requirement rules.Requirement, started statisticsMeasurement) {
	if s == nil {
		return
	}
	measured := finishStatisticsMeasurement(started)
	s.mu.Lock()
	metric := s.tiers[requirement]
	addStatisticsMetric(&metric, measured)
	s.tiers[requirement] = metric
	s.mu.Unlock()
}

func (s *Statistics) recordRule(
	id string,
	requirement rules.Requirement,
	started statisticsMeasurement,
) {
	if s == nil {
		return
	}
	measured := finishStatisticsMeasurement(started)
	s.mu.Lock()
	entry := s.rules[id]
	entry.ID = id
	entry.Requirement = requirement
	addStatisticsMetric(&entry.Metric, measured)
	s.rules[id] = entry
	s.mu.Unlock()
}

func (s *Statistics) recordPackageLoad(started statisticsMeasurement, result PackageLoadResult) {
	if s == nil {
		return
	}
	measured := finishStatisticsMeasurement(started)
	s.mu.Lock()
	addStatisticsMetric(&s.packageLoads, measured)
	for _, package_ := range result.Packages {
		if package_ != nil {
			s.packageIDs[package_.ID] = struct{}{}
		}
	}
	for _, path := range result.Sources.paths {
		s.filePaths[path] = struct{}{}
	}
	s.mu.Unlock()
}

func (s *Statistics) recordSourceFile(path string) {
	if s == nil || path == "" {
		return
	}
	s.mu.Lock()
	s.filePaths[path] = struct{}{}
	s.mu.Unlock()
}

func (s *Statistics) recordPlan(registry *rules.Registry, selection []rules.Selection) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, selected := range selection {
		if selected.Requirement > s.maximumRequirement {
			s.maximumRequirement = selected.Requirement
		}
		reasons := s.tierReasons[selected.Requirement]
		if reasons == nil {
			reasons = make(map[string]struct{})
			s.tierReasons[selected.Requirement] = reasons
		}
		reasons[selected.ID] = struct{}{}
		if selected.Requirement > rules.RequireTypes {
			typeReasons := s.tierReasons[rules.RequireTypes]
			if typeReasons == nil {
				typeReasons = make(map[string]struct{})
				s.tierReasons[rules.RequireTypes] = typeReasons
			}
			typeReasons[selected.ID] = struct{}{}
		}
		metadata, found := registry.Metadata(selected.ID)
		if !found {
			continue
		}
		if metadata.RequiresDependencySyntax {
			s.dependencySyntaxReasons[selected.ID] = struct{}{}
		}
		if metadata.RequiresEffectFacts {
			s.effectFactReasons[selected.ID] = struct{}{}
		}
		if adapted, found := registry.Lookup(selected.ID); found {
			if analyzer, ok := adapted.(*packageAnalyzerRule);
				ok && analyzer.usesFacts() {
				s.dependencySyntaxReasons[selected.ID] = struct{}{}
			}
		}
	}
}

func (s *Statistics) recordCacheLookup(found, invalid bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache.Lookups++
	switch {
	case invalid:
		s.cache.Invalidations++
	case found:
		s.cache.Hits++
	default:
		s.cache.Misses++
	}
	s.mu.Unlock()
}

func (s *Statistics) recordCacheWrite() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache.Writes++
	s.mu.Unlock()
}

// Snapshot returns a deterministic, independently owned statistics view.
func (s *Statistics) Snapshot() StatisticsSnapshot {
	if s == nil {
		return StatisticsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirements := make([]rules.Requirement, 0, len(s.tierReasons))
	for requirement := range s.tierReasons {
		requirements = append(requirements, requirement)
	}
	slices.Sort(requirements)
	tiers := make([]StatisticsTier, 0, len(requirements))
	for _, requirement := range requirements {
		tiers = append(
			tiers,
			StatisticsTier{
				Requirement: requirement,
				Reasons: sortedStatisticsKeys(s.tierReasons[requirement]),
				Metric: s.tiers[requirement],
			},
		)
	}
	ruleIDs := make([]string, 0, len(s.rules))
	for id := range s.rules {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	ruleStatistics := make([]StatisticsRule, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		ruleStatistics = append(ruleStatistics, s.rules[id])
	}
	return StatisticsSnapshot{
		Total: finishStatisticsMeasurement(s.started),
		MaximumRequirement: s.maximumRequirement,
		DependencySyntaxReasons: sortedStatisticsKeys(s.dependencySyntaxReasons),
		EffectFactReasons: sortedStatisticsKeys(s.effectFactReasons),
		PackageLoads: s.packageLoads,
		Tiers: tiers,
		Rules: ruleStatistics,
		Cache: s.cache,
		Packages: len(s.packageIDs),
		Files: len(s.filePaths),
	}
}

func sortedStatisticsKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
