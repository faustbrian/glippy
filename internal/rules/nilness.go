package rules

import (
	"fmt"
	"go/token"
	"go/types"

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
		Documentation: "Reports nil dereferences, degenerate nil comparisons, nil channel and map operations, nil panics, and invalid nil-slice conversions when SSA dominance proves the value's nilness. The implementation reuses the current x/tools nilness analyzer over Glippy's shared SSA function and augments it with conservative nil/error return relationships from selected local-source modules.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Control-flow joins may lose nilness facts, so the rule intentionally misses some defects rather than guessing.",
			"The shared SSA program propagates no-return behavior and explicit nil/error return relationships through selected local-source modules. Third-party helpers outside those modules remain conservative unless they match an exact standard-library terminal API.",
			"Return relationships require explicit results with exact nil, definitely non-nil allocation forms, errors.New, or fmt.Errorf. Bare returns, delegated or recursive results, &*x expressions, unknown error construction, aliases, and conflicting returns remain unknown.",
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
	return appendReturnStateFindings(ctx, findings)
}

func appendReturnStateFindings(ctx *SSAContext, findings []Finding) ([]Finding, error) {
	deduplicated := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		deduplicated[nilnessFindingIdentity(finding)] = struct{}{}
	}
	for _, block := range ctx.Function().Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			callee := call.Call.StaticCallee()
			if callee == nil {
				continue
			}
			function, _ := callee.Object().(*types.Func)
			signature := call.Call.Signature().Results()
			if function == nil || signature == nil {
				continue
			}
			extracts := callResultExtracts(call)
			for errorIndex, errorExtract := range extracts {
				if !isBuiltinErrorType(signature.At(errorIndex).Type()) {
					continue
				}
				for _, condition := range nilBranchConditions(errorExtract) {
					if condition.branch.Block() == nil ||
						len(condition.branch.Block().Succs) != 2 {
						continue
					}
					for valueIndex, valueExtract := range extracts {
						if valueIndex == errorIndex {
							continue
						}
						summary := ctx.ReturnState(
							function,
							valueIndex,
							errorIndex,
						)
						states := []NilState{
							summary.WhenErrorNil,
							summary.WhenErrorNonNil,
						}
						successors := condition.branch.Block().Succs
						if !condition.nilOnTrue {
							successors = []*ssa.BasicBlock{
								successors[1],
								successors[0],
							}
						}
						for index, state := range states {
							if state == NilStateUnknown {
								continue
							}
							var err error
							findings, err = addDominatedReturnStateFindings(
								ctx,
								findings,
								deduplicated,
								valueExtract,
								successors[index],
								state,
							)
							if err != nil {
								return nil, err
							}
						}
					}
				}
			}
		}
	}
	return findings, nil
}

func callResultExtracts(call *ssa.Call) map[int]*ssa.Extract {
	result := make(map[int]*ssa.Extract)
	if call == nil || call.Referrers() == nil {
		return result
	}
	for _, reference := range *call.Referrers() {
		if extract, ok := reference.(*ssa.Extract); ok {
			result[extract.Index] = extract
		}
	}
	return result
}

type nilBranch struct {
	branch *ssa.If
	nilOnTrue bool
}

func nilBranchConditions(value ssa.Value) []nilBranch {
	if value == nil || value.Referrers() == nil {
		return nil
	}
	conditions := make([]nilBranch, 0)
	for _, reference := range *value.Referrers() {
		comparison, ok := reference.(*ssa.BinOp)
		if !ok ||
			(comparison.Op != token.EQL && comparison.Op != token.NEQ) ||
			!comparisonWithNil(comparison, value) {
			continue
		}
		if comparison.Referrers() == nil {
			continue
		}
		for _, comparisonReference := range *comparison.Referrers() {
			if branch, ok := comparisonReference.(*ssa.If); ok {
				conditions = append(
					conditions,
					nilBranch{
						branch: branch,
						nilOnTrue: comparison.Op == token.EQL,
					},
				)
			}
		}
	}
	return conditions
}

func comparisonWithNil(comparison *ssa.BinOp, value ssa.Value) bool {
	if comparison == nil {
		return false
	}
	if comparison.X == value {
		constant, ok := comparison.Y.(*ssa.Const)
		return ok && constant.IsNil()
	}
	if comparison.Y == value {
		constant, ok := comparison.X.(*ssa.Const)
		return ok && constant.IsNil()
	}
	return false
}

func addDominatedReturnStateFindings(
	ctx *SSAContext,
	findings []Finding,
	deduplicated map[string]struct{},
	value ssa.Value,
	dominator *ssa.BasicBlock,
	state NilState,
) ([]Finding, error) {
	if value == nil || value.Referrers() == nil || dominator == nil {
		return findings, nil
	}
	for _, reference := range *value.Referrers() {
		if reference.Block() == nil || !dominator.Dominates(reference.Block()) {
			continue
		}
		var finding Finding
		switch instruction := reference.(type) {
		case *ssa.UnOp:
			if state != NilStateNil || instruction.Op != token.MUL {
				continue
			}
			range_, err := ctx.TokenRange(instruction.Pos())
			if err != nil {
				return nil, err
			}
			finding = Finding{
				MessageKey: "nilderef",
				Message: "nil dereference in load",
				Range: range_,
				Help: "run `glippy explain nilness` for the rule contract and limitations",
			}
		case *ssa.BinOp:
			if (instruction.Op != token.EQL && instruction.Op != token.NEQ) ||
				!comparisonWithNil(instruction, value) {
				continue
			}
			range_, err := ctx.TokenRange(instruction.Pos())
			if err != nil {
				return nil, err
			}
			message := "tautological condition: nil == nil"
			if state == NilStateNil {
				if instruction.Op == token.NEQ {
					message = "impossible condition: nil != nil"
				}
			} else if instruction.Op == token.EQL {
				message = "impossible condition: non-nil == nil"
			} else {
				message = "tautological condition: non-nil != nil"
			}
			finding = Finding{
				MessageKey: "cond",
				Message: message,
				Range: range_,
				Help: "run `glippy explain nilness` for the rule contract and limitations",
			}
		default:
			continue
		}
		identity := nilnessFindingIdentity(finding)
		if _, found := deduplicated[identity]; found {
			continue
		}
		deduplicated[identity] = struct{}{}
		findings = append(findings, finding)
	}
	return findings, nil
}

func nilnessFindingIdentity(finding Finding) string {
	return fmt.Sprintf("%s:%d:%d", finding.MessageKey, finding.Range.Start, finding.Range.End)
}

func isBuiltinErrorType(type_ types.Type) bool {
	errorObject := types.Universe.Lookup("error")
	return errorObject != nil && types.Identical(type_, errorObject.Type())
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
