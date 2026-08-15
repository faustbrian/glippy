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
) obligationEffect {
	effect := obligationOpen
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
				for _, argument := range current.Args {
					if directObject(info, argument) == object ||
						methodValueUsesObject(info, argument, object) {
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
				for _, expression := range current.Rhs {
					if directObject(info, expression) != object &&
						!methodValueUsesObject(info, expression, object) {
						continue
					}
					for _, target := range current.Lhs {
						identifier, blank := target.(*ast.Ident)
						if blank && identifier.Name == "_" {
							continue
						}
						if directObject(info, target) != object {
							effect = obligationTransferred
							return false
						}
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
	assignment, ok := node.(*ast.AssignStmt)
	if !ok {
		return obligationOpen
	}
	for _, target := range assignment.Lhs {
		if directObject(info, target) == object {
			return obligationLost
		}
	}
	return obligationOpen
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
