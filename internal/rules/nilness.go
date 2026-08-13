package rules

import (
	"fmt"

	"github.com/faustbrian/glippy/internal/source"
	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/ssa"
)

type nilnessRule struct{}

func (nilnessRule) Metadata() Metadata {
	return Metadata{
		ID: "nilness",
		Summary: "detects operations on values proven to be nil",
		Documentation: "Reports nil dereferences, degenerate nil comparisons, nil channel and map operations, nil panics, and invalid nil-slice conversions when SSA dominance proves the value's nilness. The implementation reuses the current x/tools nilness analyzer over Glippy's shared SSA function instead of constructing a second SSA program.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Control-flow joins may lose nilness facts, so the rule intentionally misses some defects rather than guessing.",
			"The shared SSA program does not yet import interprocedural no-return facts, so findings that depend on a callee terminating may be missed.",
			"Functions marked with //go:cgo_unsafe_args are excluded because their runtime behavior is not represented faithfully in SSA.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Dereference in a nil branch",
				Incorrect: `if pointer == nil {
	return *pointer
}`,
				Correct: `if pointer != nil {
	return *pointer
}`,
			},
			{
				Title: "Impossible nil comparison",
				Incorrect: "channel := make(chan int)\nif channel == nil { use(channel) }",
				Correct: "channel := make(chan int)\nuse(channel)",
			},
		},
	}
}

func (nilnessRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil ||
		ctx.Function() == nil ||
		ctx.SSAPackage() == nil ||
		ctx.Package() == nil ||
		ctx.Info() == nil ||
		ctx.FileSet() == nil {
		return nil, fmt.Errorf("nilness requires a complete SSA context")
	}
	if len(nilness.Analyzer.Requires) != 1 ||
		nilness.Analyzer.Requires[0] != buildssa.Analyzer {
		return nil, fmt.Errorf("x/tools nilness prerequisite contract changed")
	}

	diagnostics := make([]goanalysis.Diagnostic, 0)
	pass := &goanalysis.Pass{
		Analyzer: nilness.Analyzer,
		Fset: ctx.FileSet(),
		Pkg: ctx.Package(),
		TypesInfo: ctx.Info(),
		Report: func(diagnostic goanalysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
		ResultOf: map[*goanalysis.Analyzer]any{
			buildssa.Analyzer: &buildssa.SSA{
				Pkg: ctx.SSAPackage(),
				SrcFuncs: []*ssa.Function{ctx.Function()},
			},
		},
	}
	result, err := runNilnessAnalyzer(pass)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return nil, fmt.Errorf("x/tools nilness returned an unexpected result")
	}

	findings := make([]Finding, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if len(diagnostic.Related) != 0 || len(diagnostic.SuggestedFixes) != 0 {
			return nil, fmt.Errorf("x/tools nilness diagnostic contract changed")
		}
		range_, err := nilnessDiagnosticRange(ctx, diagnostic)
		if err != nil {
			return nil, err
		}
		if !validNilnessCategory(diagnostic.Category) {
			return nil, fmt.Errorf(
				"x/tools nilness returned unknown category %q",
				diagnostic.Category,
			)
		}
		findings = append(
			findings,
			Finding{
				MessageKey: diagnostic.Category,
				Message: diagnostic.Message,
				Range: range_,
				Help: "run `glippy explain nilness` for the rule contract and limitations",
			},
		)
	}
	return findings, nil
}

func validNilnessCategory(category string) bool {
	switch category {
	case "cond", "conversionpanic", "nilderef", "nilpanic":
		return true
	default:
		return false
	}
}

func nilnessDiagnosticRange(
	ctx *SSAContext,
	diagnostic goanalysis.Diagnostic,
) (source.Range, error) {
	if !diagnostic.Pos.IsValid() {
		return source.Range{}, fmt.Errorf(
			"x/tools nilness returned an invalid diagnostic position",
		)
	}
	if diagnostic.End.IsValid() {
		return ctx.PositionRange(diagnostic.Pos, diagnostic.End)
	}
	return ctx.TokenRange(diagnostic.Pos)
}

func runNilnessAnalyzer(pass *goanalysis.Pass) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("x/tools nilness panicked: %v", recovered)
		}
	}()
	return nilness.Analyzer.Run(pass)
}
