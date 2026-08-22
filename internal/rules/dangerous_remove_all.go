package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type dangerousRemoveAllRule struct{}

type dangerousDirectorySpec struct {
	function string
	kind string
}

// NewDangerousRemoveAllRule constructs the system-directory deletion rule.
func NewDangerousRemoveAllRule() Rule {
	return dangerousRemoveAllRule{}
}

func (dangerousRemoveAllRule) Metadata() Metadata {
	return Metadata{
		ID: "dangerous-remove-all",
		Summary: "detects deletion of complete user or system directories",
		Documentation: "Passing the direct result of os.TempDir, os.UserCacheDir, os.UserConfigDir, or os.UserHomeDir to os.RemoveAll deletes the complete directory rather than a child created within it. This is commonly caused by confusing os.TempDir with os.MkdirTemp or by forgetting to append a project-specific path.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"Only exact static calls to os.RemoveAll and the four exact standard-library directory functions are recognized.",
			"Direct SSA value flow and equivalent phi values are followed; helper returns, pointer loads, dynamic function calls, and transformed paths remain conservative.",
			"The rule intentionally reports no fix because choosing a safe child directory requires application intent.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Delete only an owned temporary directory",
				Incorrect: "directory := os.TempDir()\ndefer os.RemoveAll(directory)",
				Correct: "directory, err := os.MkdirTemp(\"\", \"project-*\")\nif err != nil { return err }\ndefer os.RemoveAll(directory)",
			},
		},
	}
}

func (dangerousRemoveAllRule) RunsOnSSAInitializers() {}

func (dangerousRemoveAllRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil {
		return nil, fmt.Errorf("dangerous-remove-all requires a complete SSA context")
	}
	calls := dangerousRemoveAllSyntaxCalls(ctx.Syntax())
	findings := make([]Finding, 0)
	for _, block := range ctx.Function().Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(ssa.CallInstruction)
			if !ok ||
				!dangerousRemoveAllStaticCall(call.Common()) ||
				len(call.Common().Args) != 1 {
				continue
			}
			callExpression := calls[call.Common().Pos()]
			if callExpression == nil {
				if _, initializer := ctx.Syntax().(*ast.File); initializer {
					continue
				}
				return nil, fmt.Errorf(
					"cannot map proven os.RemoveAll call to source",
				)
			}
			origin, spec, found := dangerousDirectoryOrigin(call.Common().Args[0])
			if !found {
				continue
			}
			originExpression := calls[origin.Call.Pos()]
			if originExpression == nil {
				if _, initializer := ctx.Syntax().(*ast.File); initializer {
					continue
				}
				return nil, fmt.Errorf(
					"cannot map proven os directory origin to source",
				)
			}
			callRange, err := ctx.Range(callExpression)
			if err != nil {
				return nil, err
			}
			originRange, err := ctx.Range(originExpression)
			if err != nil {
				return nil, err
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "dangerous-remove-all-" + spec.kind,
					Message: "os.RemoveAll deletes the complete " +
						spec.kind +
						" directory",
					Range: callRange,
					Related: []Related{
						{
							Range: originRange,
							Message: "os." +
								spec.function +
								" returns this " +
								spec.kind +
								" directory",
						},
					},
					Help: "create or append an application-owned child directory before deleting it",
				},
			)
		}
	}
	return findings, nil
}

func dangerousRemoveAllSyntaxCalls(root ast.Node) map[token.Pos]*ast.CallExpr {
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

func dangerousRemoveAllStaticCall(call *ssa.CallCommon) bool {
	return dangerousStaticFunction(call, "RemoveAll") != nil
}

func dangerousDirectoryOrigin(value ssa.Value) (*ssa.Call, dangerousDirectorySpec, bool) {
	value = flattenEquivalentSSAValue(value)
	if call, ok := value.(*ssa.Call);
		ok && dangerousStaticFunction(&call.Call, "TempDir") != nil {
		return call, dangerousDirectorySpec{function: "TempDir", kind: "temporary"}, true
	}
	extract, ok := value.(*ssa.Extract)
	if !ok || extract.Index != 0 {
		return nil, dangerousDirectorySpec{}, false
	}
	call, ok := flattenEquivalentSSAValue(extract.Tuple).(*ssa.Call)
	if !ok {
		return nil, dangerousDirectorySpec{}, false
	}
	for _, spec := range
		[]dangerousDirectorySpec{
			{function: "UserCacheDir", kind: "cache"},
			{function: "UserConfigDir", kind: "config"},
			{function: "UserHomeDir", kind: "home"},
		} {
		if dangerousStaticFunction(&call.Call, spec.function) != nil {
			return call, spec, true
		}
	}
	return nil, dangerousDirectorySpec{}, false
}

func flattenEquivalentSSAValue(value ssa.Value) ssa.Value {
	seen := make(map[ssa.Value]struct{})
	var result ssa.Value
	var visit func(ssa.Value) bool
	visit = func(current ssa.Value) bool {
		if current == nil {
			return false
		}
		if _, found := seen[current]; found {
			return true
		}
		seen[current] = struct{}{}
		phi, ok := current.(*ssa.Phi)
		if !ok {
			if result == nil {
				result = current
				return true
			}
			return result == current
		}
		for _, edge := range phi.Edges {
			if !visit(edge) {
				return false
			}
		}
		return true
	}
	if !visit(value) {
		return nil
	}
	return result
}

func dangerousStaticFunction(call *ssa.CallCommon, name string) *types.Func {
	if call == nil {
		return nil
	}
	callee := call.StaticCallee()
	if callee == nil {
		return nil
	}
	function, _ := callee.Object().(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "os" ||
		function.Name() != name {
		return nil
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() != nil {
		return nil
	}
	return function
}
