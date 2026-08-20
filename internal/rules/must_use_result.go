package rules

import (
	"fmt"
	"go/ast"
	"go/types"
	"strconv"
	"strings"
)

type mustUseResultRule struct{}

type ignoredMustUseCall struct {
	call *ast.CallExpr
	results []int
}

// NewMustUseResultRule constructs the configured required-result rule for
// product registry composition.
func NewMustUseResultRule() Rule {
	return mustUseResultRule{}
}

func (mustUseResultRule) Metadata() Metadata {
	return Metadata{
		ID: "must-use-result",
		Summary: "detects results discarded contrary to a project contract",
		Documentation: "A project semantic contract can mark exact function or method results as required when discarding them loses an application-specific failure, resource, token, or state transition that Go's type system cannot identify. Glippy reports a statically resolved call when any configured must-use result is discarded by a call statement, go or defer statement, or blank assignment.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only exact statically resolved calls with a validated project semantic contract are covered; dynamic calls, function values, and interface dispatch remain outside the contract.",
			"A result assigned to a non-blank destination, returned, or passed to another call counts as consumed; this rule does not prove that the later use is meaningful.",
			"Generated files and packages with type errors are excluded.",
			"Contract authors are responsible for the runtime truth of must-use declarations; Glippy validates identities and result indexes but cannot prove application policy.",
		},
		Examples: []Example{
			{
				Title: "Consume every result required by the configured API contract",
				Incorrect: "value, _ := openResource()",
				Correct: "value, err := openResource()\nif err != nil { return err }",
			},
		},
	}
}

func (mustUseResultRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("must-use-result requires a complete control-flow context")
	}
	ignored := collectIgnoredMustUseCalls(ctx)
	findings := make([]Finding, 0, len(ignored))
	for _, candidate := range ignored {
		range_, err := ctx.Range(candidate.call)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "must-use-result",
				Message: fmt.Sprintf(
					"project contract requires %s of this call to be used",
					formatResultIndexes(candidate.results),
				),
				Range: range_,
				Help: "bind and consume the required result or remove the incorrect must-use contract",
			},
		)
	}
	return findings, nil
}

func collectIgnoredMustUseCalls(ctx *ControlFlowContext) []ignoredMustUseCall {
	result := make([]ignoredMustUseCall, 0)
	ast.Inspect(
		ctx.Body(),
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			switch node := node.(type) {
			case *ast.ExprStmt:
				if call, _ := ast.Unparen(node.X).(*ast.CallExpr); call != nil {
					result = appendIgnoredMustUseCall(ctx, result, call, nil)
				}
			case *ast.GoStmt:
				result = appendIgnoredMustUseCall(ctx, result, node.Call, nil)
			case *ast.DeferStmt:
				result = appendIgnoredMustUseCall(ctx, result, node.Call, nil)
			case *ast.AssignStmt:
				result = appendIgnoredMustUseAssignments(
					ctx,
					result,
					node.Lhs,
					node.Rhs,
				)
			case *ast.ValueSpec:
				left := make([]ast.Expr, len(node.Names))
				for index, name := range node.Names {
					left[index] = name
				}
				result = appendIgnoredMustUseAssignments(
					ctx,
					result,
					left,
					node.Values,
				)
			}
			return true
		},
	)
	return result
}

func appendIgnoredMustUseAssignments(
	ctx *ControlFlowContext,
	result []ignoredMustUseCall,
	left []ast.Expr,
	right []ast.Expr,
) []ignoredMustUseCall {
	if len(left) == 0 || len(right) == 0 {
		return result
	}
	if len(right) == 1 {
		call, _ := ast.Unparen(right[0]).(*ast.CallExpr)
		if call == nil || callResultCount(ctx.Info(), call) != len(left) {
			return result
		}
		ignored := make([]int, 0, len(left))
		for index, destination := range left {
			if mustUseBlankIdentifier(destination) {
				ignored = append(ignored, index)
			}
		}
		return appendIgnoredMustUseCall(ctx, result, call, ignored)
	}
	if len(right) != len(left) {
		return result
	}
	for index, expression := range right {
		if !mustUseBlankIdentifier(left[index]) {
			continue
		}
		call, _ := ast.Unparen(expression).(*ast.CallExpr)
		if call == nil || callResultCount(ctx.Info(), call) != 1 {
			continue
		}
		result = appendIgnoredMustUseCall(ctx, result, call, []int{0})
	}
	return result
}

func appendIgnoredMustUseCall(
	ctx *ControlFlowContext,
	result []ignoredMustUseCall,
	call *ast.CallExpr,
	candidates []int,
) []ignoredMustUseCall {
	count := callResultCount(ctx.Info(), call)
	if count == 0 {
		return result
	}
	if candidates == nil {
		candidates = make([]int, count)
		for index := range count {
			candidates[index] = index
		}
	}
	ignored := make([]int, 0, len(candidates))
	for _, index := range candidates {
		if index >= 0 && index < count && ctx.MustUse(call, index) {
			ignored = append(ignored, index)
		}
	}
	if len(ignored) == 0 {
		return result
	}
	return append(result, ignoredMustUseCall{call: call, results: ignored})
}

func callResultCount(info *types.Info, call *ast.CallExpr) int {
	if info == nil || call == nil {
		return 0
	}
	signature, _ := types.Unalias(info.TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || signature.Results() == nil {
		return 0
	}
	return signature.Results().Len()
}

func mustUseBlankIdentifier(expression ast.Expr) bool {
	identifier, _ := ast.Unparen(expression).(*ast.Ident)
	return identifier != nil && identifier.Name == "_"
}

func formatResultIndexes(indexes []int) string {
	if len(indexes) == 1 {
		return "result " + strconv.Itoa(indexes[0])
	}
	parts := make([]string, len(indexes))
	for index, result := range indexes {
		parts[index] = strconv.Itoa(result)
	}
	if len(parts) == 2 {
		return "results " + parts[0] + " and " + parts[1]
	}
	return "results " +
		strings.Join(parts[:len(parts) - 1], ", ") +
		", and " +
		parts[len(parts) - 1]
}
