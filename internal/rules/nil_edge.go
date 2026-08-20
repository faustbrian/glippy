package rules

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

func exactObjectNilStateOnEdge(
	info *types.Info,
	from *cfg.Block,
	to *cfg.Block,
	object types.Object,
) (NilState, bool) {
	if info == nil || from == nil || to == nil || object == nil || len(from.Nodes) == 0 {
		return NilStateUnknown, false
	}
	guard, _ := to.Stmt.(*ast.IfStmt)
	if guard == nil || from.Nodes[len(from.Nodes) - 1] != guard.Cond {
		return NilStateUnknown, false
	}
	comparison, _ := ast.Unparen(guard.Cond).(*ast.BinaryExpr)
	if comparison == nil || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return NilStateUnknown, false
	}
	nilObject := types.Universe.Lookup("nil")
	comparesObjectToNil := directObject(info, comparison.X) == object &&
		directObject(info, comparison.Y) == nilObject ||
		directObject(info, comparison.Y) == object &&
			directObject(info, comparison.X) == nilObject
	if !comparesObjectToNil {
		return NilStateUnknown, false
	}
	trueEdge := to.Kind == cfg.KindIfThen
	falseEdge := to.Kind == cfg.KindIfElse || to.Kind == cfg.KindIfDone
	if !trueEdge && !falseEdge {
		return NilStateUnknown, false
	}
	nilOnTrue := comparison.Op == token.EQL
	if trueEdge == nilOnTrue {
		return NilStateNil, true
	}
	return NilStateNonNil, true
}
