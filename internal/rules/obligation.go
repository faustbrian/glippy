package rules

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type obligationEffect uint8

const (
	obligationOpen obligationEffect = iota
	obligationCompleted
	obligationTransferred
	obligationLost
)

type obligationStart struct {
	block *cfg.Block
	offset int
}

type obligationWork struct {
	block *cfg.Block
	offset int
}

func obligationStartAt(block *cfg.Block) obligationStart {
	return obligationStart{block: block}
}

func obligationStartAfter(graph *cfg.CFG, node ast.Node) (obligationStart, bool) {
	if graph == nil || node == nil {
		return obligationStart{}, false
	}
	for _, block := range graph.Blocks {
		for index, candidate := range block.Nodes {
			if candidate == node {
				return obligationStart{block: block, offset: index + 1}, true
			}
		}
	}
	return obligationStart{}, false
}

func obligationReachesOpenReturn(
	start obligationStart,
	classify func(ast.Node) obligationEffect,
) bool {
	if start.block == nil || classify == nil {
		return false
	}
	work := []obligationWork{{block: start.block, offset: start.offset}}
	seen := make(map[obligationWork]bool)
	summaries := make(map[obligationWork]obligationEffect)
	for len(work) > 0 {
		current := work[len(work) - 1]
		work = work[:len(work) - 1]
		if current.block == nil || !current.block.Live || seen[current] {
			continue
		}
		seen[current] = true
		effect, found := summaries[current]
		if !found {
			effect = summarizeObligationNodes(
				current.block.Nodes,
				current.offset,
				classify,
			)
			summaries[current] = effect
		}
		switch effect {
		case obligationCompleted, obligationTransferred:
			continue
		case obligationLost:
			return true
		}
		if current.block.Return() != nil {
			return true
		}
		for _, successor := range current.block.Succs {
			work = append(work, obligationWork{block: successor})
		}
	}
	return false
}

func summarizeObligationNodes(
	nodes []ast.Node,
	offset int,
	classify func(ast.Node) obligationEffect,
) obligationEffect {
	if offset < 0 {
		offset = 0
	}
	if offset > len(nodes) {
		offset = len(nodes)
	}
	for _, node := range nodes[offset:] {
		if effect := classify(node); effect != obligationOpen {
			return effect
		}
	}
	return obligationOpen
}

func objectObligationEffect(
	info *types.Info,
	node ast.Node,
	object types.Object,
	complete func(*ast.CallExpr) bool,
	parameterEffect func(*ast.CallExpr, int) ParameterEffectSummary,
	acceptedEffects ParameterEffectKind,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) obligationEffect {
	effect := obligationOpen
	assignment, _ := node.(*ast.AssignStmt)
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if effect != obligationOpen {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				if expressionUsesObject(info, literal.Body, object) {
					effect = obligationTransferred
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				if complete != nil && complete(current) {
					effect = obligationCompleted
					return false
				}
				for index, argument := range current.Args {
					if directObject(info, argument) == object {
						aliasReturned := assignmentReturnsObjectAlias(
							info,
							assignment,
							object,
							current,
							index,
							returnsAlias,
						)
						if parameterEffect != nil {
							summary := parameterEffect(current, index)
							if summary.Known {
								if summary.GuaranteesAny(
									acceptedEffects &
										^ParameterEffectTransfer,
								) {
									effect = obligationCompleted
									return false
								}
								if aliasReturned {
									continue
								}
								if summary.GuaranteesAny(
									acceptedEffects,
								) {
									effect = obligationCompleted
									return false
								}
								continue
							}
						}
						if aliasReturned {
							continue
						}
						effect = obligationTransferred
						return false
					}
					if methodValueUsesObject(info, argument, object) {
						effect = obligationTransferred
						return false
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directObject(info, expression) == object {
						effect = obligationTransferred
						return false
					}
				}
			case *ast.SendStmt:
				if directObject(info, current.Value) == object {
					effect = obligationTransferred
					return false
				}
			case *ast.AssignStmt:
				for index, expression := range current.Rhs {
					if directObject(info, expression) != object &&
						!methodValueUsesObject(info, expression, object) {
						continue
					}
					if len(current.Rhs) != len(current.Lhs) {
						effect = obligationTransferred
						return false
					}
					target := current.Lhs[index]
					identifier, blank := target.(*ast.Ident)
					if blank && identifier.Name == "_" {
						continue
					}
					if directObject(info, target) != object {
						effect = obligationTransferred
						return false
					}
				}
			case *ast.CompositeLit:
				for _, element := range current.Elts {
					if expressionUsesObject(info, element, object) {
						effect = obligationTransferred
						return false
					}
				}
			}
			return true
		},
	)
	if effect != obligationOpen {
		return effect
	}
	if assignment == nil {
		return obligationOpen
	}
	for _, target := range assignment.Lhs {
		if directObject(info, target) == object {
			if !assignmentReplacesObject(info, assignment, object, returnsAlias) {
				return obligationOpen
			}
			return obligationLost
		}
	}
	return obligationOpen
}

func assignmentReturnsFinalObjectAlias(
	info *types.Info,
	assignment *ast.AssignStmt,
	object types.Object,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) bool {
	if info == nil || assignment == nil || object == nil || returnsAlias == nil {
		return false
	}
	target := -1
	for index, expression := range assignment.Lhs {
		if directObject(info, expression) == object {
			target = index
		}
	}
	if target < 0 {
		return false
	}
	var call *ast.CallExpr
	result := 0
	if len(assignment.Rhs) == 1 {
		call, _ = ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
		if call == nil || callResultCount(info, call) != len(assignment.Lhs) {
			return false
		}
		result = target
	} else {
		if len(assignment.Rhs) != len(assignment.Lhs) {
			return false
		}
		call, _ = ast.Unparen(assignment.Rhs[target]).(*ast.CallExpr)
		if call == nil || callResultCount(info, call) != 1 {
			return false
		}
	}
	for argument, expression := range call.Args {
		if directObject(info, expression) == object &&
			returnsAlias(call, result, argument) {
			_, aliasesOther := assignmentCallAliasTargets(
				info,
				assignment,
				object,
				call,
				argument,
				returnsAlias,
			)
			return !aliasesOther
		}
	}
	return false
}

func assignmentReplacesObject(
	info *types.Info,
	node ast.Node,
	object types.Object,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) bool {
	assignment, _ := node.(*ast.AssignStmt)
	if info == nil || assignment == nil || object == nil {
		return false
	}
	target := -1
	for index, expression := range assignment.Lhs {
		if directObject(info, expression) == object {
			target = index
		}
	}
	if target < 0 {
		return false
	}
	if len(assignment.Rhs) == len(assignment.Lhs) &&
		directObject(info, assignment.Rhs[target]) == object {
		return false
	}
	return !assignmentReturnsFinalObjectAlias(info, assignment, object, returnsAlias)
}

func assignmentReturnsObjectAlias(
	info *types.Info,
	assignment *ast.AssignStmt,
	object types.Object,
	call *ast.CallExpr,
	argument int,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) bool {
	if info == nil ||
		assignment == nil ||
		object == nil ||
		call == nil ||
		returnsAlias == nil ||
		argument < 0 ||
		argument >= len(call.Args) ||
		directObject(info, call.Args[argument]) != object {
		return false
	}
	aliasesObject, aliasesOther := assignmentCallAliasTargets(
		info,
		assignment,
		object,
		call,
		argument,
		returnsAlias,
	)
	return aliasesObject && !aliasesOther
}

func assignmentCallAliasTargets(
	info *types.Info,
	assignment *ast.AssignStmt,
	object types.Object,
	call *ast.CallExpr,
	argument int,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) (bool, bool) {
	if info == nil || assignment == nil || object == nil || call == nil || returnsAlias == nil {
		return false, false
	}
	classify := func(target ast.Expr) (bool, bool) {
		identifier, blank := ast.Unparen(target).(*ast.Ident)
		if blank && identifier.Name == "_" {
			return false, false
		}
		if directObject(info, target) == object {
			return true, false
		}
		return false, true
	}
	aliasesObject := false
	aliasesOther := false
	if len(assignment.Rhs) == 1 && ast.Unparen(assignment.Rhs[0]) == call {
		if callResultCount(info, call) != len(assignment.Lhs) {
			return false, false
		}
		for result, target := range assignment.Lhs {
			if !returnsAlias(call, result, argument) {
				continue
			}
			objectTarget, otherTarget := classify(target)
			aliasesObject = aliasesObject || objectTarget
			aliasesOther = aliasesOther || otherTarget
		}
		return aliasesObject, aliasesOther
	}
	if len(assignment.Rhs) != len(assignment.Lhs) || callResultCount(info, call) != 1 {
		return false, false
	}
	for result, expression := range assignment.Rhs {
		if ast.Unparen(expression) != call || !returnsAlias(call, 0, argument) {
			continue
		}
		objectTarget, otherTarget := classify(assignment.Lhs[result])
		aliasesObject = aliasesObject || objectTarget
		aliasesOther = aliasesOther || otherTarget
	}
	return aliasesObject, aliasesOther
}

func returningNonNilErrorGuard(info *types.Info, guard *ast.IfStmt, errorObject types.Object) bool {
	if guard == nil ||
		guard.Init != nil ||
		guard.Else != nil ||
		guard.Body == nil ||
		len(guard.Body.List) == 0 {
		return false
	}
	if _, returns := guard.Body.List[len(guard.Body.List) - 1].(*ast.ReturnStmt); !returns {
		return false
	}
	comparison, _ := ast.Unparen(guard.Cond).(*ast.BinaryExpr)
	if comparison == nil || comparison.Op != token.NEQ {
		return false
	}
	nilObject := types.Universe.Lookup("nil")
	return directObject(info, comparison.X) == errorObject &&
		directObject(info, comparison.Y) == nilObject ||
		directObject(info, comparison.Y) == errorObject &&
			directObject(info, comparison.X) == nilObject
}
