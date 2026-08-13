package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

type almostSwappedRule struct{}

// NewAlmostSwappedRule constructs the sequential almost-swap rule for product
// registry composition.
func NewAlmostSwappedRule() Rule {
	return almostSwappedRule{}
}

func (almostSwappedRule) Metadata() Metadata {
	return Metadata{
		ID: "almost-swapped",
		Summary: "detects sequential assignments that fail to swap values",
		Documentation: "Two consecutive assignments such as left = right followed by right = left do not exchange values: the first assignment overwrites the original left value before the second reads it. Go supports an explicit simultaneous assignment for swaps.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBlockStmt},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only consecutive simple identifier assignments are reported; selectors, indexing, dereferences, compound assignments, and intervening statements are excluded because evaluation may have effects.",
			"The rule does not claim the intended repair is a swap; assigning both variables the same value can be deliberate and may require suppression.",
		},
		Examples: []Example{
			{
				Title: "Use Go's simultaneous assignment for a swap",
				Incorrect: "left = right\nright = left",
				Correct: "left, right = right, left",
			},
		},
	}
}

func (almostSwappedRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	block, ok := node.(*ast.BlockStmt)
	if !ok {
		return nil, fmt.Errorf("almost-swapped requires a block statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("almost-swapped requires complete type information")
	}
	findings := make([]Finding, 0)
	for index := 0; index + 1 < len(block.List); index++ {
		first, firstLeft, firstRight := simpleIdentifierAssignment(
			ctx.Info(),
			block.List[index],
		)
		second, secondLeft, secondRight := simpleIdentifierAssignment(
			ctx.Info(),
			block.List[index + 1],
		)
		if first == nil ||
			second == nil ||
			firstLeft == firstRight ||
			firstLeft != secondRight ||
			firstRight != secondLeft {
			continue
		}
		range_, err := ctx.PositionRange(first.Pos(), second.End())
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "sequential-swap",
				Message: "these sequential assignments do not swap the values",
				Range: range_,
				Help: "use simultaneous assignment if the values should be swapped",
			},
		)
	}
	return findings, nil
}

func simpleIdentifierAssignment(
	info *types.Info,
	statement ast.Stmt,
) (*ast.AssignStmt, types.Object, types.Object) {
	assignment, _ := statement.(*ast.AssignStmt)
	if assignment == nil ||
		assignment.Tok != token.ASSIGN ||
		len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return nil, nil, nil
	}
	left, _ := assignment.Lhs[0].(*ast.Ident)
	right, _ := assignment.Rhs[0].(*ast.Ident)
	if left == nil || right == nil {
		return nil, nil, nil
	}
	leftObject := info.ObjectOf(left)
	rightObject := info.ObjectOf(right)
	if leftObject == nil || rightObject == nil {
		return nil, nil, nil
	}
	return assignment, leftObject, rightObject
}
