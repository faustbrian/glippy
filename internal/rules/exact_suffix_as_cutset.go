package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

const useTrimSuffixFix = "use-trim-suffix"

type exactSuffixAsCutsetRule struct{}

// NewExactSuffixAsCutsetRule constructs the guarded strings.TrimRight misuse
// rule for product registry composition.
func NewExactSuffixAsCutsetRule() Rule {
	return exactSuffixAsCutsetRule{}
}

func (exactSuffixAsCutsetRule) Metadata() Metadata {
	return Metadata{
		ID:               "exact-suffix-as-cutset",
		Summary:          "detects exact suffixes passed to strings.TrimRight as cutsets",
		Documentation:    "Strings.TrimRight treats its second argument as an unordered set of characters, not an exact suffix. When the same value and suffix are first checked with strings.HasSuffix, TrimRight can remove additional trailing characters beyond the recognized suffix; TrimSuffix expresses the proven intent exactly.",
		DefaultSeverity:  SeverityWarn,
		Presets:          []Preset{PresetNursery},
		MinimumGoVersion: "1.25",
		Requirement:      RequireTypes,
		NodeInterests:    []NodeKind{NodeIfStmt},
		Categories:       []Category{CategoryCorrectness, CategorySuspicious},
		Fixes: []FixMetadata{
			{
				Name:        useTrimSuffixFix,
				Description: "replace strings.TrimRight with strings.TrimSuffix",
				Safety:      FixUnsafe,
			},
		},
		KnownLimitations: []string{
			"Only a direct strings.HasSuffix condition followed immediately by a matching TrimRight call as the sole assignment right-hand side, return result, or expression statement is considered.",
			"Value and suffix identity is limited to the same typed identifier or selector, or equal compile-time string constants.",
			"Equivalent guards expressed through helpers, boolean combinations, early returns, or broader control-flow dominance are excluded.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title:     "Remove the exact suffix that was recognized",
				Incorrect: "if strings.HasSuffix(value, suffix) {\n\tvalue = strings.TrimRight(value, suffix)\n}",
				Correct:   "if strings.HasSuffix(value, suffix) {\n\tvalue = strings.TrimSuffix(value, suffix)\n}",
			},
		},
	}
}

func (exactSuffixAsCutsetRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	statement, ok := node.(*ast.IfStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"exact-suffix-as-cutset requires an if statement and type information",
		)
	}
	guard, _ := ast.Unparen(statement.Cond).(*ast.CallExpr)
	if guard == nil ||
		len(guard.Args) != 2 ||
		!isDirectStringsFunction(ctx.Info(), guard, "HasSuffix") ||
		len(statement.Body.List) == 0 {
		return nil, nil
	}
	call := immediateTrimRightCall(statement.Body.List[0])
	if call == nil ||
		len(call.Args) != 2 ||
		!isDirectStringsFunction(ctx.Info(), call, "TrimRight") ||
		!sameExactStringExpression(ctx.Info(), guard.Args[0], call.Args[0]) ||
		!sameExactStringExpression(ctx.Info(), guard.Args[1], call.Args[1]) {
		return nil, nil
	}
	identifier := directFunctionIdentifier(call.Fun)
	if identifier == nil {
		return nil, nil
	}
	range_, err := ctx.Range(identifier)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "exact-suffix-as-cutset",
			Message:    "strings.TrimRight treats the recognized suffix as a character cutset",
			Range:      range_,
			Help:       "use strings.TrimSuffix to remove only the suffix proven by HasSuffix",
			Fixes: []Fix{
				{
					Name:   useTrimSuffixFix,
					Safety: FixUnsafe,
					Edits:  []Edit{{Range: range_, NewText: "TrimSuffix"}},
				},
			},
		},
	}, nil
}

func immediateTrimRightCall(statement ast.Stmt) *ast.CallExpr {
	var expression ast.Expr
	switch statement := statement.(type) {
	case *ast.AssignStmt:
		if len(statement.Lhs) != 1 ||
			len(statement.Rhs) != 1 ||
			!sideEffectFreeAssignmentTarget(statement.Lhs[0]) {
			return nil
		}
		expression = statement.Rhs[0]
	case *ast.ExprStmt:
		expression = statement.X
	case *ast.ReturnStmt:
		if len(statement.Results) != 1 {
			return nil
		}
		expression = statement.Results[0]
	default:
		return nil
	}
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	return call
}

func sideEffectFreeAssignmentTarget(expression ast.Expr) bool {
	switch expression := ast.Unparen(expression).(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return sideEffectFreeAssignmentTarget(expression.X)
	default:
		return false
	}
}

func isDirectStringsFunction(info *types.Info, call *ast.CallExpr, name string) bool {
	function := directStandardFunction(info, call.Fun, "strings")
	return function != nil && function.Name() == name
}

func directFunctionIdentifier(expression ast.Expr) *ast.Ident {
	switch expression := ast.Unparen(expression).(type) {
	case *ast.Ident:
		return expression
	case *ast.SelectorExpr:
		return expression.Sel
	default:
		return nil
	}
}

func sameExactStringExpression(info *types.Info, first, second ast.Expr) bool {
	if sameSimpleExpression(info, first, second) {
		return true
	}
	firstValue := info.Types[ast.Unparen(first)].Value
	secondValue := info.Types[ast.Unparen(second)].Value
	return firstValue != nil &&
		secondValue != nil &&
		firstValue.Kind() == constant.String &&
		secondValue.Kind() == constant.String &&
		constant.Compare(firstValue, token.EQL, secondValue)
}
