package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type finalizerCapturesObjectRule struct{}

// NewFinalizerCapturesObjectRule constructs the retaining-finalizer rule for
// product registry composition.
func NewFinalizerCapturesObjectRule() Rule {
	return finalizerCapturesObjectRule{}
}

func (finalizerCapturesObjectRule) Metadata() Metadata {
	return Metadata{
		ID: "finalizer-captures-object",
		Summary: "detects finalizers that retain the object they finalize",
		Documentation: "A runtime finalizer runs only after its object becomes unreachable. A finalizer closure that captures the same object keeps it reachable through the closure registered with the runtime, so the finalizer never runs and the object cannot be collected. The closure must use the object passed to its finalizer parameter instead of capturing the outer variable.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only exact static calls to runtime.SetFinalizer with a directly proven closure are considered.",
			"The rule reports only when SSA identifies the exact captured variable cell used to load the finalized object; object aliases, helper returns, unresolved function calls, and ambiguous value flow remain conservative.",
			"No automatic fix is offered because rewriting the closure to use its parameter requires application-specific naming and behavior decisions.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Use the finalizer parameter",
				Incorrect: "value := &resource{}\nruntime.SetFinalizer(value, func(*resource) { value.Close() })",
				Correct: "value := &resource{}\nruntime.SetFinalizer(value, func(current *resource) { current.Close() })",
			},
		},
	}
}

func (finalizerCapturesObjectRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil {
		return nil, fmt.Errorf("finalizer-captures-object requires a complete SSA context")
	}
	calls := finalizerSyntaxCalls(ctx.Syntax())
	findings := make([]Finding, 0)
	for _, block := range ctx.Function().Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(ssa.CallInstruction)
			if !ok || !runtimeSetFinalizer(call.Common()) {
				continue
			}
			common := call.Common()
			if len(common.Args) != 2 {
				continue
			}
			objectCell := finalizerObjectCell(common.Args[0])
			closure := finalizerClosure(common.Args[1])
			if objectCell == nil ||
				closure == nil ||
				!closureBindsValue(closure, objectCell) {
				continue
			}
			callExpression := calls[common.Pos()]
			if callExpression == nil {
				return nil, fmt.Errorf(
					"cannot map proven runtime.SetFinalizer call to source",
				)
			}
			closureFunction, _ := closure.Fn.(*ssa.Function)
			var closureSyntax *ast.FuncLit
			if closureFunction != nil {
				closureSyntax, _ = closureFunction.Syntax().(*ast.FuncLit)
			}
			if closureSyntax == nil {
				return nil, fmt.Errorf(
					"cannot map proven finalizer closure to source",
				)
			}
			callRange, err := ctx.Range(callExpression)
			if err != nil {
				return nil, err
			}
			closureRange, err := ctx.Range(closureSyntax)
			if err != nil {
				return nil, err
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "finalizer-captures-object",
					Message: "this finalizer captures its object, so the object remains reachable and the finalizer cannot run",
					Range: callRange,
					Related: []Related{
						{
							Range: closureRange,
							Message: "this closure retains the finalized object",
						},
					},
					Help: "use the object passed to the finalizer parameter instead of capturing the outer variable",
				},
			)
		}
	}
	return findings, nil
}

func finalizerSyntaxCalls(root ast.Node) map[token.Pos]*ast.CallExpr {
	result := make(map[token.Pos]*ast.CallExpr)
	inspectOwnedFunction(
		root,
		func(node ast.Node) {
			call, ok := node.(*ast.CallExpr)
			if ok {
				result[call.Lparen] = call
			}
		},
	)
	return result
}

func runtimeSetFinalizer(call *ssa.CallCommon) bool {
	if call == nil || call.StaticCallee() == nil {
		return false
	}
	function, _ := call.StaticCallee().Object().(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "runtime" ||
		function.Name() != "SetFinalizer" {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil && signature.Recv() == nil
}

func finalizerObjectCell(value ssa.Value) *ssa.Alloc {
	value = unwrapFinalizerInterface(value)
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	allocation, _ := load.X.(*ssa.Alloc)
	return allocation
}

func finalizerClosure(value ssa.Value) *ssa.MakeClosure {
	value = unwrapFinalizerInterface(value)
	closure, _ := flattenEquivalentSSAValue(value).(*ssa.MakeClosure)
	return closure
}

func unwrapFinalizerInterface(value ssa.Value) ssa.Value {
	for {
		switch current := value.(type) {
		case *ssa.MakeInterface:
			value = current.X
		case *ssa.ChangeInterface:
			value = current.X
		default:
			return value
		}
	}
}

func closureBindsValue(closure *ssa.MakeClosure, value ssa.Value) bool {
	if closure == nil || value == nil {
		return false
	}
	for _, binding := range closure.Bindings {
		if binding == value {
			return true
		}
	}
	return false
}
