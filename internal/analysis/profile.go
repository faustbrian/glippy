package analysis

import (
	"context"
	"fmt"
)

// PhaseProfiler observes explicit retained-analysis boundaries. Implementations
// may force garbage collection or write heap profiles, so profiling is opt-in
// and is never enabled by ordinary CLI or editor analysis.
type PhaseProfiler interface {
	Capture(phase string) error
}

const (
	ProfilePhasePackages = "packages"
	ProfilePhaseSourceModel = "source-model"
	ProfilePhaseEffectFacts = "effect-facts"
	ProfilePhaseTypes = "types"
	ProfilePhaseControlFlow = "control-flow"
	ProfilePhaseSSA = "ssa"
	ProfilePhasePackageAnalyzers = "package-analyzers"
	ProfilePhaseResult = "result"
)

type phaseProfilerContextKey struct{}

func withPhaseProfiler(ctx context.Context, profiler PhaseProfiler) context.Context {
	if profiler == nil {
		return ctx
	}
	return context.WithValue(ctx, phaseProfilerContextKey{}, profiler)
}

func captureProfilePhase(ctx context.Context, phase string) error {
	if ctx == nil {
		return nil
	}
	profiler, _ := ctx.Value(phaseProfilerContextKey{}).(PhaseProfiler)
	if profiler == nil {
		return nil
	}
	if err := profiler.Capture(phase); err != nil {
		return fmt.Errorf("profile analysis phase %q: %w", phase, err)
	}
	return nil
}
