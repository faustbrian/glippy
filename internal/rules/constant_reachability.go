package rules

import (
	"go/ast"
	"go/types"
)

func nodeInsideConstantUnreachableBranch(
	body *ast.BlockStmt,
	info *types.Info,
	target ast.Node,
) bool {
	if body == nil || info == nil || target == nil {
		return false
	}
	unreachable := false
	ast.PreorderStack(
		body,
		nil,
		func(current ast.Node, stack []ast.Node) bool {
			if current != target {
				return astNodeContains(current, target)
			}
			for index := len(stack) - 1; index >= 0; index-- {
				switch statement := stack[index].(type) {
				case *ast.IfStmt:
					condition, constant := typedBooleanConstant(
						info,
						statement.Cond,
					)
					if !constant {
						continue
					}
					if astNodeContains(statement.Body, target) {
						unreachable = !condition
					} else if statement.Else != nil &&
						astNodeContains(statement.Else, target) {
						unreachable = condition
					}
				case *ast.ForStmt:
					condition, constant := typedBooleanConstant(
						info,
						statement.Cond,
					)
					if constant &&
						!condition &&
						astNodeContains(statement.Body, target) {
						unreachable = true
					}
				}
				if unreachable {
					break
				}
			}
			return false
		},
	)
	return unreachable
}

func astNodeContains(container ast.Node, node ast.Node) bool {
	return container != nil &&
		node != nil &&
		container.Pos() <= node.Pos() &&
		node.End() <= container.End()
}
