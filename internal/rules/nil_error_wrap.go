package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"unicode/utf8"

	"golang.org/x/tools/go/ssa"
)

type nilErrorWrapRule struct{}

// NewNilErrorWrapRule constructs the proven-nil fmt.Errorf wrapping rule.
func NewNilErrorWrapRule() Rule {
	return nilErrorWrapRule{}
}

func (nilErrorWrapRule) Metadata() Metadata {
	return Metadata{
		ID: "nil-error-wrap",
		Summary: "detects fmt.Errorf calls that wrap an error proven nil",
		Documentation: "Wrapping nil with fmt.Errorf's %w directive returns a non-nil formatting error that does not wrap the intended failure. The rule reports literal nil and built-in error values proven nil by direct control flow, an exact helper result state, or an exact selected-helper sibling result relationship.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only exact fmt.Errorf calls with a compile-time format string and sequential non-star directives are analyzed; explicit argument indexes and star width or precision remain conservative.",
			"Path proof covers literal nil, direct nil SSA values, exact nil comparisons whose nil outcome edge dominates the call, exact static helper results proven nil on every explicit normal return, and exact sibling-result states that contradict every non-nil-error return from a selected local helper.",
			"Unconditional result proof requires a direct SSA call or tuple extraction; exact static delegation may cross selected local-source packages through a bounded recursion-rejecting summary. Phis, dynamic calls, recursive helper cycles, typed nil errors, captured or address-taken named results, conflicting returns, and unavailable dependency facts remain conservative. Relational return-state proof requires an exact sibling nil comparison; the relationship may cross the same bounded static delegation boundary.",
			"Only values with the exact built-in error interface type are tracked; typed nil pointers and application-specific error interfaces are excluded because converting them to an interface can produce a non-nil interface value.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Do not wrap an error in its nil branch",
				Incorrect: "if err == nil { return fmt.Errorf(\"operation: %w\", err) }",
				Correct: "if err != nil { return fmt.Errorf(\"operation: %w\", err) }",
			},
		},
	}
}

func (nilErrorWrapRule) RequiresSSADebug() {}

func (nilErrorWrapRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("nil-error-wrap requires a complete SSA context")
	}
	expressionValues := overwrittenErrorExpressionValues(ctx.Function())
	findings := make([]Finding, 0)
	var runErr error
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			if runErr != nil {
				return
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || !exactFmtErrorfCall(ctx.Info(), call) || len(call.Args) < 2 {
				return
			}
			format, ok := constantStringValue(ctx.Info(), call.Args[0])
			if !ok {
				return
			}
			arguments, ok := sequentialWrapArguments(format)
			if !ok || len(arguments) == 0 {
				return
			}
			callValue := expressionValues[ast.Unparen(call)]
			if callValue == nil {
				callValue, _ = ctx.Function().ValueForExpr(call)
			}
			instruction, _ := callValue.(ssa.Instruction)
			if instruction == nil || instruction.Block() == nil {
				return
			}
			for _, argumentIndex := range arguments {
				if argumentIndex < 0 || argumentIndex >= len(call.Args) - 1 {
					continue
				}
				argument := ast.Unparen(call.Args[argumentIndex + 1])
				if !nilErrorWrapArgumentType(ctx.Info(), argument) {
					continue
				}
				value := expressionValues[argument]
				if value == nil {
					value, _ = ctx.Function().ValueForExpr(argument)
				}
				if !literalNilExpression(ctx.Info(), argument) &&
					!ssaValueDefinitelyNilAt(
						ctx.Function(),
						value,
						instruction.Block(),
					) &&
					!returnStateProvesErrorNilAt(
						ctx,
						value,
						instruction.Block(),
					) &&
					!resultStateProvesErrorNil(ctx, value) {
					continue
				}
				range_, err := ctx.Range(argument)
				if err != nil {
					runErr = err
					return
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "nil-error-wrap",
						Message: "fmt.Errorf wraps an error that is proven nil on this path",
						Range: range_,
						Help: "return or handle the nil case without a %w wrapping directive",
					},
				)
			}
		},
	)
	if runErr != nil {
		return nil, runErr
	}
	return findings, nil
}

func resultStateProvesErrorNil(ctx *SSAContext, value ssa.Value) bool {
	if ctx == nil || value == nil {
		return false
	}
	resultIndex := 0
	var call *ssa.Call
	switch value := value.(type) {
	case *ssa.Call:
		call = value
	case *ssa.Extract:
		call, _ = value.Tuple.(*ssa.Call)
		resultIndex = value.Index
	default:
		return false
	}
	if call == nil || call.Call.StaticCallee() == nil {
		return false
	}
	function, _ := call.Call.StaticCallee().Object().(*types.Func)
	if function == nil {
		return false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil ||
		signature.Results() == nil ||
		resultIndex < 0 ||
		resultIndex >= signature.Results().Len() ||
		!isBuiltinErrorType(signature.Results().At(resultIndex).Type()) {
		return false
	}
	return ctx.ResultState(function, resultIndex) == NilStateNil
}

func returnStateProvesErrorNilAt(
	ctx *SSAContext,
	errorValue ssa.Value,
	target *ssa.BasicBlock,
) bool {
	if ctx == nil || errorValue == nil || target == nil {
		return false
	}
	errorExtract, _ := errorValue.(*ssa.Extract)
	if errorExtract == nil {
		return false
	}
	call, _ := errorExtract.Tuple.(*ssa.Call)
	if call == nil {
		return false
	}
	callee := call.Call.StaticCallee()
	if callee == nil {
		return false
	}
	function, _ := callee.Object().(*types.Func)
	if function == nil {
		return false
	}
	for valueIndex, valueExtract := range callResultExtracts(call) {
		if valueIndex == errorExtract.Index {
			continue
		}
		summary := ctx.ReturnState(function, valueIndex, errorExtract.Index)
		switch summary.WhenErrorNonNil {
		case NilStateNil:
			if ssaValueDefinitelyNonNilAt(ctx.Function(), valueExtract, target) {
				return true
			}
		case NilStateNonNil:
			if ssaValueDefinitelyNilAt(ctx.Function(), valueExtract, target) {
				return true
			}
		}
	}
	return false
}

func exactFmtErrorfCall(info *types.Info, call *ast.CallExpr) bool {
	if info == nil || call == nil {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "fmt" &&
		function.Name() == "Errorf"
}

func constantStringValue(info *types.Info, expression ast.Expr) (string, bool) {
	if info == nil || expression == nil {
		return "", false
	}
	value := info.Types[ast.Unparen(expression)].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

func sequentialWrapArguments(format string) ([]int, bool) {
	arguments := make([]int, 0)
	argumentIndex := 0
	for offset := 0; offset < len(format); {
		if format[offset] != '%' {
			_, size := utf8.DecodeRuneInString(format[offset:])
			if size == 0 {
				return nil, false
			}
			offset += size
			continue
		}
		offset++
		if offset >= len(format) {
			return nil, false
		}
		if format[offset] == '%' {
			offset++
			continue
		}
		if format[offset] == '[' {
			return nil, false
		}
		for offset < len(format) && isFormatFlag(format[offset]) {
			offset++
		}
		if offset < len(format) && (format[offset] == '*' || format[offset] == '[') {
			return nil, false
		}
		for offset < len(format) && format[offset] >= '0' && format[offset] <= '9' {
			offset++
		}
		if offset < len(format) && format[offset] == '.' {
			offset++
			if offset < len(format) &&
				(format[offset] == '*' || format[offset] == '[') {
				return nil, false
			}
			for offset < len(format) && format[offset] >= '0' && format[offset] <= '9' {
				offset++
			}
		}
		if offset >= len(format) || format[offset] == '[' {
			return nil, false
		}
		verb, size := utf8.DecodeRuneInString(format[offset:])
		if verb == utf8.RuneError && size == 1 {
			return nil, false
		}
		if verb == 'w' {
			arguments = append(arguments, argumentIndex)
		}
		argumentIndex++
		offset += size
	}
	return arguments, true
}

func isFormatFlag(value byte) bool {
	switch value {
	case '#', '0', '+', '-', ' ':
		return true
	default:
		return false
	}
}

func nilErrorWrapArgumentType(info *types.Info, expression ast.Expr) bool {
	if literalNilExpression(info, expression) {
		return true
	}
	errorObject := types.Universe.Lookup("error")
	return errorObject != nil && types.Identical(info.TypeOf(expression), errorObject.Type())
}

func literalNilExpression(info *types.Info, expression ast.Expr) bool {
	identifier, _ := ast.Unparen(expression).(*ast.Ident)
	return info != nil &&
		identifier != nil &&
		info.ObjectOf(identifier) == types.Universe.Lookup("nil")
}

func ssaValueDefinitelyNilAt(function *ssa.Function, value ssa.Value, target *ssa.BasicBlock) bool {
	return ssaValueDefinitelyStateAt(function, value, target, NilStateNil)
}

func ssaValueDefinitelyNonNilAt(
	function *ssa.Function,
	value ssa.Value,
	target *ssa.BasicBlock,
) bool {
	return ssaValueDefinitelyStateAt(function, value, target, NilStateNonNil)
}

func ssaValueDefinitelyStateAt(
	function *ssa.Function,
	value ssa.Value,
	target *ssa.BasicBlock,
	state NilState,
) bool {
	if function == nil || value == nil || target == nil {
		return false
	}
	if constant, ok := value.(*ssa.Const); ok {
		return state == NilStateNil && constant.IsNil()
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			branch, ok := instruction.(*ssa.If)
			if !ok || branch.Block() == nil || len(branch.Block().Succs) != 2 {
				continue
			}
			comparison, _ := branch.Cond.(*ssa.BinOp)
			if comparison == nil ||
				(comparison.Op != token.EQL && comparison.Op != token.NEQ) ||
				!comparisonWithNil(comparison, value) {
				continue
			}
			nilSuccessor := branch.Block().Succs[0]
			if comparison.Op == token.NEQ {
				nilSuccessor = branch.Block().Succs[1]
			}
			provenSuccessor := nilSuccessor
			if state == NilStateNonNil {
				provenSuccessor = branch.Block().Succs[0]
				if provenSuccessor == nilSuccessor {
					provenSuccessor = branch.Block().Succs[1]
				}
			}
			if ssaControlFlowEdgeDominates(
				function,
				branch.Block(),
				provenSuccessor,
				target,
			) {
				return true
			}
		}
	}
	return false
}

func ssaControlFlowEdgeDominates(
	function *ssa.Function,
	from *ssa.BasicBlock,
	to *ssa.BasicBlock,
	target *ssa.BasicBlock,
) bool {
	if function == nil ||
		len(function.Blocks) == 0 ||
		function.Blocks[0] == nil ||
		from == nil ||
		to == nil ||
		target == nil ||
		!from.Dominates(target) ||
		!to.Dominates(target) {
		return false
	}
	visited := make(map[*ssa.BasicBlock]struct{}, len(function.Blocks))
	pending := []*ssa.BasicBlock{function.Blocks[0]}
	for len(pending) > 0 {
		last := len(pending) - 1
		block := pending[last]
		pending = pending[:last]
		if block == target {
			return false
		}
		if _, found := visited[block]; found {
			continue
		}
		visited[block] = struct{}{}
		for _, successor := range block.Succs {
			if block == from && successor == to {
				continue
			}
			pending = append(pending, successor)
		}
	}
	return true
}
