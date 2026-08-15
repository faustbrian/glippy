package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/faustbrian/glippy/internal/source"
)

type redundantBoolComparisonRule struct{}

const redundantBoolComparisonFix = "simplify-comparison"

func (redundantBoolComparisonRule) Metadata() Metadata {
	return Metadata{
		ID: "redundant-bool-comparison",
		Summary: "detects comparisons with boolean constants",
		Documentation: "Comparing a boolean expression with a compile-time true or false value adds no information. Use the boolean expression directly, negating it when the comparison reverses its truth value.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetStyle},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBinaryExpr},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		Fixes: []FixMetadata{
			{
				Name: redundantBoolComparisonFix,
				Description: "replace the comparison with an equivalent boolean expression",
				Safety: FixSafe,
			},
		},
		KnownLimitations: []string{
			"Boolean type parameters are not reported until their complete type sets can be proven boolean.",
			"Comparisons are not reported when the retained operand has a defined boolean type because the comparison may intentionally normalize the result to predeclared bool.",
			"A diagnostic has no automatic fix when removing the comparison would discard a comment outside the retained operand.",
		},
		Examples: []Example{
			{
				Title: "Use the boolean condition directly",
				Incorrect: "if ready == true {\n\trun()\n}",
				Correct: "if ready {\n\trun()\n}",
			},
		},
	}
}

func (redundantBoolComparisonRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	comparison, ok := node.(*ast.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("redundant-bool-comparison requires a binary expression")
	}
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, nil
	}
	constantValue, other, found := comparisonBooleanConstant(ctx.Info(), comparison)
	if !found || !isFixSafeBoolean(ctx.Info().TypeOf(other)) {
		return nil, nil
	}
	comparisonRange, err := ctx.Range(comparison)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "omit-comparison",
		Message: "comparison with a boolean constant is redundant",
		Range: comparisonRange,
		Help: "use the boolean expression directly",
	}
	otherRange, err := ctx.Range(other)
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRange(ctx.File().Comments(), comparisonRange, otherRange) {
		finding.WithheldFixes = commentWithheldFix(
			redundantBoolComparisonFix,
			"simplifying this comparison would remove comments",
		)
		return []Finding{finding}, nil
	}
	replacement, found := ctx.File().Slice(otherRange)
	if !found {
		return nil, fmt.Errorf(
			"redundant-bool-comparison operand has an invalid source range",
		)
	}
	negate := comparison.Op == token.EQL && !constantValue ||
		comparison.Op == token.NEQ && constantValue
	if negate {
		replacement = negateBooleanSource(other, replacement)
	}
	finding.Fixes = []Fix{
		{
			Name: redundantBoolComparisonFix,
			Safety: FixSafe,
			Edits: []Edit{{Range: comparisonRange, NewText: replacement}},
		},
	}
	return []Finding{finding}, nil
}

func comparisonBooleanConstant(
	info *types.Info,
	comparison *ast.BinaryExpr,
) (value bool, other ast.Expr, found bool) {
	if info == nil || comparison == nil {
		return false, nil, false
	}
	if value, found := typedBooleanConstant(info, comparison.X); found {
		return value, comparison.Y, true
	}
	if value, found := typedBooleanConstant(info, comparison.Y); found {
		return value, comparison.X, true
	}
	return false, nil, false
}

func typedBooleanConstant(info *types.Info, expression ast.Expr) (bool, bool) {
	typeAndValue, found := info.Types[expression]
	if !found || typeAndValue.Value == nil || typeAndValue.Value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(typeAndValue.Value), true
}

func isFixSafeBoolean(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	basic, ok := types.Unalias(type_).(*types.Basic)
	return ok && (basic.Kind() == types.Bool || basic.Kind() == types.UntypedBool)
}

func commentsOutsideRetainedRange(
	comments []source.Comment,
	comparison source.Range,
	retained source.Range,
) bool {
	for _, comment := range comments {
		if comment.Range.Start < comparison.Start || comment.Range.End > comparison.End {
			continue
		}
		if comment.Range.Start < retained.Start || comment.Range.End > retained.End {
			return true
		}
	}
	return false
}

func negateBooleanSource(expression ast.Expr, text string) string {
	if unary, ok := expression.(*ast.UnaryExpr);
		ok && unary.Op == token.NOT && strings.HasPrefix(text, "!") {
		return strings.TrimPrefix(text, "!")
	}
	switch expression.(type) {
	case *ast.ParenExpr,
		*ast.Ident,
		*ast.SelectorExpr,
		*ast.CallExpr,
		*ast.IndexExpr,
		*ast.IndexListExpr,
		*ast.TypeAssertExpr,
		*ast.UnaryExpr:
		return "!" + text
	default:
		return "!(" + text + ")"
	}
}
