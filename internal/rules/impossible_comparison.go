package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
)

type impossibleComparisonRule struct{}

// NewImpossibleComparisonRule constructs the integer-boundary rule for product
// registry composition.
func NewImpossibleComparisonRule() Rule {
	return impossibleComparisonRule{}
}

func (impossibleComparisonRule) Metadata() Metadata {
	return Metadata{
		ID: "impossible-comparison",
		Summary: "detects comparisons outside an integer type's value range",
		Documentation: "Ordered comparisons against an integer type's minimum or maximum value can be constant regardless of the runtime operand. Such conditions commonly leave dead branches or accidentally invert a boundary check.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBinaryExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Architecture-sized int and uint maximum and minimum comparisons are excluded because source selection may target an architecture different from the running Glippy binary.",
			"The rule reports only comparisons with a compile-time integer constant and does not infer ranges from preceding control flow.",
		},
		Examples: []Example{
			{
				Title: "Use a reachable unsigned boundary",
				Incorrect: "value < 0",
				Correct: "value == 0",
			},
		},
	}
}

func (impossibleComparisonRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("impossible-comparison requires a binary expression")
	}
	if comparison.Op != token.LSS &&
		comparison.Op != token.LEQ &&
		comparison.Op != token.GTR &&
		comparison.Op != token.GEQ {
		return nil, nil
	}
	variable, value, operator, found := normalizedIntegerConstantComparison(
		ctx.Info(),
		comparison,
	)
	if !found {
		return nil, nil
	}
	minimum, maximum, found := integerTypeExtremes(ctx.Info().TypeOf(variable))
	if !found {
		return nil, nil
	}
	always, found := extremeComparisonResult(operator, value, minimum, maximum)
	if !found {
		return nil, nil
	}
	range_, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "impossible-comparison",
			Message: fmt.Sprintf(
				"comparison is always %t for type %s",
				always,
				ctx.Info().TypeOf(variable),
			),
			Range: range_,
			Help: "use a boundary comparison that can change with the runtime value",
		},
	}, nil
}

func normalizedIntegerConstantComparison(
	info *types.Info,
	comparison *ast.BinaryExpr,
) (ast.Expr, constant.Value, token.Token, bool) {
	if info == nil || comparison == nil {
		return nil, nil, token.ILLEGAL, false
	}
	leftValue := info.Types[comparison.X].Value
	rightValue := info.Types[comparison.Y].Value
	if (leftValue == nil) == (rightValue == nil) {
		return nil, nil, token.ILLEGAL, false
	}
	if rightValue != nil && rightValue.Kind() == constant.Int {
		return comparison.X, rightValue, comparison.Op, true
	}
	if leftValue == nil || leftValue.Kind() != constant.Int {
		return nil, nil, token.ILLEGAL, false
	}
	return comparison.Y, leftValue, reverseOrderedComparison(comparison.Op), true
}

func reverseOrderedComparison(operator token.Token) token.Token {
	switch operator {
	case token.EQL, token.NEQ:
		return operator
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	default:
		return token.ILLEGAL
	}
}

func integerTypeExtremes(type_ types.Type) (constant.Value, constant.Value, bool) {
	if type_ == nil {
		return nil, nil, false
	}
	basic, ok := types.Unalias(type_).Underlying().(*types.Basic)
	if !ok {
		return nil, nil, false
	}
	switch basic.Kind() {
	case types.Uint, types.Uintptr:
		return constant.MakeUint64(0), nil, true
	case types.Uint8:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint8), true
	case types.Uint16:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint16), true
	case types.Uint32:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint32), true
	case types.Uint64:
		return constant.MakeUint64(0), constant.MakeUint64(math.MaxUint64), true
	case types.Int8:
		return constant.MakeInt64(math.MinInt8), constant.MakeInt64(math.MaxInt8), true
	case types.Int16:
		return constant.MakeInt64(math.MinInt16), constant.MakeInt64(math.MaxInt16), true
	case types.Int32:
		return constant.MakeInt64(math.MinInt32), constant.MakeInt64(math.MaxInt32), true
	case types.Int64:
		return constant.MakeInt64(math.MinInt64), constant.MakeInt64(math.MaxInt64), true
	default:
		return nil, nil, false
	}
}

func extremeComparisonResult(
	operator token.Token,
	value constant.Value,
	minimum constant.Value,
	maximum constant.Value,
) (bool, bool) {
	if minimum != nil && constant.Compare(value, token.EQL, minimum) {
		switch operator {
		case token.LSS:
			return false, true
		case token.GEQ:
			return true, true
		}
	}
	if maximum != nil && constant.Compare(value, token.EQL, maximum) {
		switch operator {
		case token.GTR:
			return false, true
		case token.LEQ:
			return true, true
		}
	}
	return false, false
}
