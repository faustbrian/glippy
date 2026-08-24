package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type iterationErrorSpec struct {
	packagePath string
	typeName string
	iterationMethod string
	errorMethod string
}

type iterationErrorCandidate struct {
	call *ast.CallExpr
	receiver types.Object
	done *cfg.Block
}

type iterationErrorPathState struct {
	block *cfg.Block
	invalidated bool
}

func runUncheckedIterationError(
	ctx *ControlFlowContext,
	ruleID string,
	spec iterationErrorSpec,
	messageKey string,
	message string,
	help string,
) ([]Finding, error) {
	if ctx == nil ||
		ctx.Body() == nil ||
		ctx.Graph() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf("%s requires a complete control-flow context", ruleID)
	}
	findings := make([]Finding, 0)
	for _, candidate := range
		iterationErrorCandidates(ctx.Body(), ctx.Graph(), ctx.Info(), spec) {
		if !iterationErrorPathReturnsUnchecked(candidate, ctx.Info(), spec) {
			continue
		}
		range_, err := ctx.Range(candidate.call)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: messageKey,
				Message: message,
				Range: range_,
				Help: help,
			},
		)
	}
	return findings, nil
}

func iterationErrorCandidates(
	body *ast.BlockStmt,
	graph *cfg.CFG,
	info *types.Info,
	spec iterationErrorSpec,
) []iterationErrorCandidate {
	doneBlocks := make(map[*ast.ForStmt]*cfg.Block)
	for _, block := range graph.Blocks {
		loop, ok := block.Stmt.(*ast.ForStmt)
		if block.Live && ok && block.Kind == cfg.KindForDone {
			doneBlocks[loop] = block
		}
	}

	result := make([]iterationErrorCandidate, 0)
	for _, block := range graph.Blocks {
		loop, ok := block.Stmt.(*ast.ForStmt)
		if !block.Live || !ok || block.Kind != cfg.KindForLoop || loop.Cond == nil {
			continue
		}
		call, _ := ast.Unparen(loop.Cond).(*ast.CallExpr)
		receiver, matched := iterationMethodReceiver(info, call, spec, spec.iterationMethod)
		done := doneBlocks[loop]
		if !matched ||
			done == nil ||
			!done.Live ||
			nodeInsideConstantUnreachableBranch(body, info, call) {
			continue
		}
		result = append(
			result,
			iterationErrorCandidate{call: call, receiver: receiver, done: done},
		)
	}
	return result
}

func iterationMethodReceiver(
	info *types.Info,
	call *ast.CallExpr,
	spec iterationErrorSpec,
	methodName string,
) (types.Object, bool) {
	if info == nil || call == nil || len(call.Args) != 0 {
		return nil, false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil, false
	}
	selection := info.Selections[selector]
	if selection == nil || !namedReceiver(selection.Recv(), spec.packagePath, spec.typeName) {
		return nil, false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != spec.packagePath ||
		function.Name() != methodName {
		return nil, false
	}
	receiver := directObject(info, selector.X)
	return receiver, receiver != nil
}

func iterationErrorPathReturnsUnchecked(
	candidate iterationErrorCandidate,
	info *types.Info,
	spec iterationErrorSpec,
) bool {
	work := []iterationErrorPathState{{block: candidate.done}}
	seen := make(map[iterationErrorPathState]bool)
	for len(work) > 0 {
		state := work[len(work) - 1]
		work = work[:len(work) - 1]
		if state.block == nil || !state.block.Live || seen[state] {
			continue
		}
		seen[state] = true

		checked := false
		for _, node := range state.block.Nodes {
			if !state.invalidated &&
				nodeUsesIterationError(info, node, candidate.receiver, spec) {
				checked = true
				break
			}
			if nodeReassignsIterationObject(info, node, candidate.receiver) {
				state.invalidated = true
			}
		}
		if checked {
			continue
		}
		if state.block.Return() != nil {
			return true
		}
		for _, successor := range state.block.Succs {
			work = append(
				work,
				iterationErrorPathState{
					block: successor,
					invalidated: state.invalidated,
				},
			)
		}
	}
	return false
}

func nodeUsesIterationError(
	info *types.Info,
	node ast.Node,
	receiver types.Object,
	spec iterationErrorSpec,
) bool {
	used := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, stack []ast.Node) bool {
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			call, ok := current.(*ast.CallExpr)
			if !ok {
				return !used
			}
			callReceiver, matched := iterationMethodReceiver(
				info,
				call,
				spec,
				spec.errorMethod,
			)
			if matched &&
				callReceiver == receiver &&
				iterationCallResultIsUsed(call, stack) {
				used = true
				return false
			}
			return !used
		},
	)
	return used
}

func iterationCallResultIsUsed(call *ast.CallExpr, stack []ast.Node) bool {
	if len(stack) == 0 {
		return true
	}
	for index := len(stack) - 1; index >= 0; index-- {
		switch parent := stack[index].(type) {
		case *ast.ParenExpr:
			continue
		case *ast.ExprStmt:
			return ast.Unparen(parent.X) != call
		case *ast.DeferStmt:
			return parent.Call != call
		case *ast.GoStmt:
			return parent.Call != call
		case *ast.AssignStmt:
			return iterationAssignmentCallResultIsUsed(parent, call)
		case *ast.ValueSpec:
			return iterationValueSpecCallResultIsUsed(parent, call)
		default:
			return true
		}
	}
	return true
}

func iterationAssignmentCallResultIsUsed(assignment *ast.AssignStmt, call *ast.CallExpr) bool {
	for index, expression := range assignment.Rhs {
		if ast.Unparen(expression) != call {
			continue
		}
		if len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		identifier, _ := assignment.Lhs[index].(*ast.Ident)
		return identifier == nil || identifier.Name != "_"
	}
	return true
}

func iterationValueSpecCallResultIsUsed(specification *ast.ValueSpec, call *ast.CallExpr) bool {
	for index, expression := range specification.Values {
		if ast.Unparen(expression) != call {
			continue
		}
		return len(specification.Names) != len(specification.Values) ||
			specification.Names[index].Name != "_"
	}
	return true
}

func nodeReassignsIterationObject(info *types.Info, node ast.Node, object types.Object) bool {
	assignment, ok := node.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, expression := range assignment.Lhs {
		if directObject(info, expression) == object {
			return true
		}
	}
	return false
}
