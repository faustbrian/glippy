package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

type badBitMaskRule struct{}

// NewBadBitMaskRule constructs the masked-comparison rule for product registry
// composition.
func NewBadBitMaskRule() Rule {
	return badBitMaskRule{}
}

func (badBitMaskRule) Metadata() Metadata {
	return Metadata{
		ID: "bad-bit-mask",
		Summary: "detects masked comparisons that cannot change result",
		Documentation: "A bitwise AND cannot produce bits absent from its mask, while a bitwise OR cannot remove bits required by its mask. Comparing either result with an incompatible constant is therefore always true or false and usually indicates a mistaken mask or comparison value.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBinaryExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"The rule currently covers equality and inequality checks around integer AND and OR expressions with compile-time masks and comparison values.",
			"Ordered masked comparisons and ineffective masks that do not force a constant result are outside this rule's current scope.",
		},
		Examples: []Example{
			{
				Title: "Compare only bits allowed by the mask",
				Incorrect: "value&0b0010 == 0b0001",
				Correct: "value&0b0010 == 0b0010",
			},
		},
	}
}

func (badBitMaskRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("bad-bit-mask requires a binary expression")
	}
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, nil
	}
	masked, compared, _, found := normalizedIntegerConstantComparison(ctx.Info(), comparison)
	if !found {
		return nil, nil
	}
	bitwise, _ := ast.Unparen(masked).(*ast.BinaryExpr)
	if bitwise == nil || bitwise.Op != token.AND && bitwise.Op != token.OR {
		return nil, nil
	}
	variable, mask, found := constantBitwiseOperand(ctx.Info(), bitwise)
	if !found || !isIntegerType(ctx.Info().TypeOf(variable)) {
		return nil, nil
	}
	equalityResult, constantResult := maskedEqualityResult(bitwise.Op, mask, compared)
	if !constantResult {
		return nil, nil
	}
	always := equalityResult
	if comparison.Op == token.NEQ {
		always = !always
	}
	range_, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "bad-bit-mask",
			Message: fmt.Sprintf("masked comparison is always %t", always),
			Range: range_,
			Help: "make the mask and comparison constant describe compatible bits",
		},
	}, nil
}

func constantBitwiseOperand(
	info *types.Info,
	expression *ast.BinaryExpr,
) (ast.Expr, constant.Value, bool) {
	if info == nil || expression == nil {
		return nil, nil, false
	}
	leftValue := info.Types[expression.X].Value
	rightValue := info.Types[expression.Y].Value
	if (leftValue == nil) == (rightValue == nil) {
		return nil, nil, false
	}
	if rightValue != nil && rightValue.Kind() == constant.Int {
		return expression.X, rightValue, true
	}
	if leftValue == nil || leftValue.Kind() != constant.Int {
		return nil, nil, false
	}
	return expression.Y, leftValue, true
}

func isIntegerType(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	basic, ok := types.Unalias(type_).Underlying().(*types.Basic)
	return ok && basic.Info() & types.IsInteger != 0
}

func maskedEqualityResult(
	operator token.Token,
	mask constant.Value,
	compared constant.Value,
) (bool, bool) {
	switch operator {
	case token.AND:
		if constant.Sign(mask) == 0 {
			return constant.Sign(compared) == 0, true
		}
		available := constant.BinaryOp(mask, token.AND, compared)
		if !constant.Compare(available, token.EQL, compared) {
			return false, true
		}
	case token.OR:
		required := constant.BinaryOp(mask, token.OR, compared)
		if !constant.Compare(required, token.EQL, compared) {
			return false, true
		}
	}
	return false, false
}
