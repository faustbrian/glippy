package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

type subsumedConditionRule struct{}

// NewSubsumedConditionRule constructs the ordered-branch rule for product
// registry composition.
func NewSubsumedConditionRule() Rule {
	return subsumedConditionRule{}
}

func (subsumedConditionRule) Metadata() Metadata {
	return Metadata{
		ID: "subsumed-condition",
		Summary: "detects else-if conditions covered by an earlier branch",
		Documentation: "An else-if branch is unreachable when its ordered comparison can only be true in cases already accepted by the preceding branch. The later condition commonly contains a reversed bound or branches ordered from broadest to narrowest.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeIfStmt},
		Categories: []Category{CategorySuspicious, CategoryCorrectness},
		KnownLimitations: []string{
			"The rule currently compares adjacent ordered integer conditions over the same plain identifier and a compile-time constant.",
			"Chains with an initializer or compound conditions are excluded because their value and evaluation relationships require broader control-flow reasoning.",
		},
		Examples: []Example{
			{
				Title: "Order branches from narrowest to broadest",
				Incorrect: "if value > 0 { use() } else if value > 10 { specialize() }",
				Correct: "if value > 10 { specialize() } else if value > 0 { use() }",
			},
		},
	}
}

func (subsumedConditionRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	current, ok := node.(*ast.IfStmt)
	if !ok {
		return nil, fmt.Errorf("subsumed-condition requires an if statement")
	}
	next, _ := current.Else.(*ast.IfStmt)
	if next == nil || current.Init != nil || next.Init != nil {
		return nil, nil
	}
	prior, found := orderedIdentifierPredicate(ctx.Info(), current.Cond)
	if !found {
		return nil, nil
	}
	later, found := orderedIdentifierPredicate(ctx.Info(), next.Cond)
	if !found || prior.object != later.object || !predicateSubsumes(prior, later) {
		return nil, nil
	}
	priorRange, err := ctx.Range(current.Cond)
	if err != nil {
		return nil, err
	}
	laterRange, err := ctx.Range(next.Cond)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "subsumed-condition",
			Message: "condition is subsumed by the preceding branch",
			Range: laterRange,
			Related: []Related{
				{
					Range: priorRange,
					Message: "preceding condition already covers these values",
				},
			},
			Help: "order narrower branches first or correct the comparison bound",
		},
	}, nil
}

type orderedPredicate struct {
	object types.Object
	operator token.Token
	value constant.Value
}

func orderedIdentifierPredicate(info *types.Info, expression ast.Expr) (orderedPredicate, bool) {
	comparison, _ := ast.Unparen(expression).(*ast.BinaryExpr)
	if comparison == nil ||
		comparison.Op != token.LSS &&
			comparison.Op != token.LEQ &&
			comparison.Op != token.GTR &&
			comparison.Op != token.GEQ {
		return orderedPredicate{}, false
	}
	variable, value, operator, found := normalizedIntegerConstantComparison(info, comparison)
	if !found {
		return orderedPredicate{}, false
	}
	identifier, _ := ast.Unparen(variable).(*ast.Ident)
	if identifier == nil {
		return orderedPredicate{}, false
	}
	object := info.ObjectOf(identifier)
	if object == nil || !isIntegerType(info.TypeOf(identifier)) {
		return orderedPredicate{}, false
	}
	return orderedPredicate{object: object, operator: operator, value: value}, true
}

func predicateSubsumes(prior, later orderedPredicate) bool {
	comparison := constant.Compare(later.value, token.EQL, prior.value)
	less := constant.Compare(later.value, token.LSS, prior.value)
	greater := !less && !comparison
	switch prior.operator {
	case token.GTR:
		switch later.operator {
		case token.GTR:
			return comparison || greater
		case token.GEQ:
			return greater
		}
	case token.GEQ:
		if later.operator == token.GTR || later.operator == token.GEQ {
			return comparison || greater
		}
	case token.LSS:
		switch later.operator {
		case token.LSS:
			return comparison || less
		case token.LEQ:
			return less
		}
	case token.LEQ:
		if later.operator == token.LSS || later.operator == token.LEQ {
			return comparison || less
		}
	}
	return false
}
