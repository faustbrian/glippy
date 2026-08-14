package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

const databaseSQLPackagePath = "database/sql"

type uncheckedRowsErrorRule struct{}

type rowsIterationCandidate struct {
	call *ast.CallExpr
	receiver types.Object
	done *cfg.Block
}

type rowsPathState struct {
	block *cfg.Block
	invalidated bool
}

// NewUncheckedRowsErrorRule constructs the database row-iteration lifecycle
// rule for product registry composition.
func NewUncheckedRowsErrorRule() Rule {
	return uncheckedRowsErrorRule{}
}

func (uncheckedRowsErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "unchecked-rows-error",
		Summary: "detects database row iteration without a checked terminal error",
		Documentation: "database/sql.Rows.Next returns false both when iteration is complete and when iteration stops because of an error. Code that returns normally after the loop must observe Rows.Err or it can silently accept a partial result set. This rule follows the shared control-flow graph from each direct Rows.Next loop and reports when any normally returning path can bypass observation of the matching Rows.Err result.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract recognizes direct identifier-backed database/sql.Rows.Next and Rows.Err calls; aliases stored in fields, containers, or other variables are not tracked.",
			"A direct assignment to the rows variable invalidates later checks against a replacement value; writes through range targets and indirect aliases are not modeled.",
			"Passing Rows.Err to another call counts as observing the result; the rule does not inspect the callee's behavior.",
			"The shared CFG recognizes predeclared panic as non-returning; imported and project-local termination helpers are treated as returning.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Check the terminal iteration error",
				Incorrect: "for rows.Next() { scan(rows) }\nreturn nil",
				Correct: "for rows.Next() { scan(rows) }\nreturn rows.Err()",
			},
		},
	}
}

func (uncheckedRowsErrorRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Graph() == nil || ctx.Info() == nil || ctx.Package() == nil {
		return nil, fmt.Errorf(
			"unchecked-rows-error requires a complete control-flow context",
		)
	}
	findings := make([]Finding, 0)
	for _, candidate := range rowsIterationCandidates(ctx.Graph(), ctx.Info()) {
		if !rowsPathReturnsUnchecked(candidate, ctx.Info()) {
			continue
		}
		range_, err := ctx.Range(candidate.call)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "rows-error-not-checked",
				Message: "rows.Err is not checked on every normally returning path after iteration",
				Range: range_,
				Help: "check rows.Err after the loop before returning the accumulated results",
			},
		)
	}
	return findings, nil
}

func rowsIterationCandidates(graph *cfg.CFG, info *types.Info) []rowsIterationCandidate {
	doneBlocks := make(map[*ast.ForStmt]*cfg.Block)
	for _, block := range graph.Blocks {
		loop, ok := block.Stmt.(*ast.ForStmt)
		if block.Live && ok && block.Kind == cfg.KindForDone {
			doneBlocks[loop] = block
		}
	}

	result := make([]rowsIterationCandidate, 0)
	for _, block := range graph.Blocks {
		loop, ok := block.Stmt.(*ast.ForStmt)
		if !block.Live || !ok || block.Kind != cfg.KindForLoop || loop.Cond == nil {
			continue
		}
		call, _ := ast.Unparen(loop.Cond).(*ast.CallExpr)
		receiver, matched := databaseRowsMethodReceiver(info, call, "Next")
		done := doneBlocks[loop]
		if !matched || done == nil || !done.Live {
			continue
		}
		result = append(
			result,
			rowsIterationCandidate{call: call, receiver: receiver, done: done},
		)
	}
	return result
}

func databaseRowsMethodReceiver(
	info *types.Info,
	call *ast.CallExpr,
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
	if selection == nil || !namedReceiver(selection.Recv(), databaseSQLPackagePath, "Rows") {
		return nil, false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != databaseSQLPackagePath ||
		function.Name() != methodName {
		return nil, false
	}
	receiver := directObject(info, selector.X)
	return receiver, receiver != nil
}

func rowsPathReturnsUnchecked(candidate rowsIterationCandidate, info *types.Info) bool {
	work := []rowsPathState{{block: candidate.done}}
	seen := make(map[rowsPathState]bool)
	for len(work) > 0 {
		state := work[len(work) - 1]
		work = work[:len(work) - 1]
		if state.block == nil || !state.block.Live || seen[state] {
			continue
		}
		seen[state] = true

		checked := false
		for _, node := range state.block.Nodes {
			if !state.invalidated && nodeUsesRowsErr(info, node, candidate.receiver) {
				checked = true
				break
			}
			if nodeReassignsObject(info, node, candidate.receiver) {
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
				rowsPathState{block: successor, invalidated: state.invalidated},
			)
		}
	}
	return false
}

func nodeUsesRowsErr(info *types.Info, node ast.Node, receiver types.Object) bool {
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
			callReceiver, matched := databaseRowsMethodReceiver(info, call, "Err")
			if matched && callReceiver == receiver && callResultIsUsed(call, stack) {
				used = true
				return false
			}
			return !used
		},
	)
	return used
}

func callResultIsUsed(call *ast.CallExpr, stack []ast.Node) bool {
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
			return assignmentCallResultIsUsed(parent, call)
		case *ast.ValueSpec:
			return valueSpecCallResultIsUsed(parent, call)
		default:
			return true
		}
	}
	return true
}

func assignmentCallResultIsUsed(assignment *ast.AssignStmt, call *ast.CallExpr) bool {
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

func valueSpecCallResultIsUsed(specification *ast.ValueSpec, call *ast.CallExpr) bool {
	for index, expression := range specification.Values {
		if ast.Unparen(expression) != call {
			continue
		}
		return len(specification.Names) != len(specification.Values) ||
			specification.Names[index].Name != "_"
	}
	return true
}

func nodeReassignsObject(info *types.Info, node ast.Node, object types.Object) bool {
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
