package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type sqlTransactionNotCompletedRule struct{}

type sqlTransactionCandidate struct {
	identifier *ast.Ident
	object types.Object
	start *cfg.Block
}

type sqlTransactionDisposition uint8

const (
	sqlTransactionOpen sqlTransactionDisposition = iota
	sqlTransactionDischarged
	sqlTransactionLost
)

// NewSQLTransactionNotCompletedRule constructs the database transaction
// lifecycle rule for product registry composition.
func NewSQLTransactionNotCompletedRule() Rule {
	return sqlTransactionNotCompletedRule{}
}

func (sqlTransactionNotCompletedRule) Metadata() Metadata {
	return Metadata{
		ID: "sql-transaction-not-completed",
		Summary: "detects database transactions left without commit or rollback",
		Documentation: "database/sql requires every successful transaction to end with Tx.Commit or Tx.Rollback. A locally owned transaction that reaches a normal return without either call can retain a connection, locks, and transaction state. This rule follows direct DB.Begin, DB.BeginTx, and Conn.BeginTx results through the shared control-flow graph after a conventional acquisition-error guard.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The initial contract recognizes direct database/sql DB.Begin, DB.BeginTx, and Conn.BeginTx assignments followed immediately by an err != nil guard whose body returns.",
			"Passing, returning, sending, storing, or capturing the transaction counts as an ownership transfer; the rule does not inspect the receiving code.",
			"Only standard Tx.Commit and Tx.Rollback calls or an explicit ownership transfer discharge the obligation; wrapper finalizers are not inferred.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Rollback an uncommitted transaction",
				Incorrect: "tx, err := db.Begin()\nif err != nil { return err }\n_, err = tx.Exec(query)\nreturn err",
				Correct: "tx, err := db.Begin()\nif err != nil { return err }\ndefer tx.Rollback()\n_, err = tx.Exec(query)\nreturn err",
			},
		},
	}
}

func (sqlTransactionNotCompletedRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil ||
		ctx.Body() == nil ||
		ctx.Graph() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf(
			"sql-transaction-not-completed requires a complete control-flow context",
		)
	}
	if !packageImports(ctx.Package(), databaseSQLPackagePath) {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for _, candidate := range sqlTransactionCandidates(ctx.Body(), ctx.Graph(), ctx.Info()) {
		if !sqlTransactionReturnsOpen(candidate, ctx.Info()) {
			continue
		}
		range_, err := ctx.Range(candidate.identifier)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "transaction-not-completed",
				Message: fmt.Sprintf(
					"transaction %q is not committed or rolled back on every normally returning path",
					candidate.identifier.Name,
				),
				Range: range_,
				Help: "defer tx.Rollback after the acquisition error check and commit on the successful path",
			},
		)
	}
	return findings, nil
}

func sqlTransactionCandidates(
	body *ast.BlockStmt,
	graph *cfg.CFG,
	info *types.Info,
) []sqlTransactionCandidate {
	doneBlocks := make(map[*ast.IfStmt]*cfg.Block)
	for _, block := range graph.Blocks {
		statement, ok := block.Stmt.(*ast.IfStmt)
		if block.Live && ok && block.Kind == cfg.KindIfDone {
			doneBlocks[statement] = block
		}
	}
	result := make([]sqlTransactionCandidate, 0)
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			block, ok := node.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for index := 0; index + 1 < len(block.List); index++ {
				assignment, ok := block.List[index].(*ast.AssignStmt)
				if !ok {
					continue
				}
				identifier, object, errorObject, matched := sqlBeginAssignment(
					info,
					assignment,
				)
				guard, ok := block.List[index + 1].(*ast.IfStmt)
				start := doneBlocks[guard]
				if !matched ||
					!sqlAcquisitionErrorGuard(info, guard, errorObject) ||
					start == nil ||
					!start.Live {
					continue
				}
				result = append(
					result,
					sqlTransactionCandidate{
						identifier: identifier,
						object: object,
						start: start,
					},
				)
			}
			return true
		},
	)
	return result
}

func sqlBeginAssignment(
	info *types.Info,
	assignment *ast.AssignStmt,
) (*ast.Ident, types.Object, types.Object, bool) {
	if info == nil ||
		assignment == nil ||
		len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 {
		return nil, nil, nil, false
	}
	transaction, _ := assignment.Lhs[0].(*ast.Ident)
	errorIdentifier, _ := assignment.Lhs[1].(*ast.Ident)
	call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if transaction == nil ||
		transaction.Name == "_" ||
		errorIdentifier == nil ||
		errorIdentifier.Name == "_" ||
		call == nil ||
		!sqlBeginCall(info, call) ||
		!namedReceiver(info.TypeOf(transaction), databaseSQLPackagePath, "Tx") {
		return nil, nil, nil, false
	}
	transactionObject := info.ObjectOf(transaction)
	errorObject := info.ObjectOf(errorIdentifier)
	if transactionObject == nil || errorObject == nil {
		return nil, nil, nil, false
	}
	return transaction, transactionObject, errorObject, true
}

func sqlBeginCall(info *types.Info, call *ast.CallExpr) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	selection := info.Selections[selector]
	function, _ := selectionObject(selection).(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != databaseSQLPackagePath {
		return false
	}
	databaseReceiver := namedReceiver(selection.Recv(), databaseSQLPackagePath, "DB")
	connectionReceiver := namedReceiver(selection.Recv(), databaseSQLPackagePath, "Conn")
	switch function.Name() {
	case "Begin":
		return databaseReceiver && len(call.Args) == 0
	case "BeginTx":
		return (databaseReceiver || connectionReceiver) && len(call.Args) == 2
	default:
		return false
	}
}

func sqlAcquisitionErrorGuard(info *types.Info, guard *ast.IfStmt, errorObject types.Object) bool {
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

func sqlTransactionReturnsOpen(candidate sqlTransactionCandidate, info *types.Info) bool {
	work := []*cfg.Block{candidate.start}
	seen := make(map[*cfg.Block]bool)
	for len(work) > 0 {
		block := work[len(work) - 1]
		work = work[:len(work) - 1]
		if block == nil || !block.Live || seen[block] {
			continue
		}
		seen[block] = true
		discharged := false
		for _, node := range block.Nodes {
			switch sqlTransactionNodeDisposition(info, node, candidate.object) {
			case sqlTransactionDischarged:
				discharged = true
			case sqlTransactionLost:
				return true
			}
			if discharged {
				break
			}
		}
		if discharged {
			continue
		}
		if block.Return() != nil {
			return true
		}
		work = append(work, block.Succs...)
	}
	return false
}

func sqlTransactionNodeDisposition(
	info *types.Info,
	node ast.Node,
	object types.Object,
) sqlTransactionDisposition {
	disposition := sqlTransactionOpen
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if disposition != sqlTransactionOpen {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				if expressionUsesObject(info, literal.Body, object) {
					disposition = sqlTransactionDischarged
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				if sqlTransactionCompletionCall(info, current, object) {
					disposition = sqlTransactionDischarged
					return false
				}
				for _, argument := range current.Args {
					if directObject(info, argument) == object ||
						methodValueUsesObject(info, argument, object) {
						disposition = sqlTransactionDischarged
						return false
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directObject(info, expression) == object {
						disposition = sqlTransactionDischarged
						return false
					}
				}
			case *ast.SendStmt:
				if directObject(info, current.Value) == object {
					disposition = sqlTransactionDischarged
					return false
				}
			case *ast.AssignStmt:
				for _, expression := range current.Rhs {
					if methodValueUsesObject(info, expression, object) {
						disposition = sqlTransactionDischarged
						return false
					}
					if directObject(info, expression) != object {
						continue
					}
					for _, target := range current.Lhs {
						identifier, _ := target.(*ast.Ident)
						if identifier == nil || identifier.Name != "_" {
							disposition = sqlTransactionDischarged
							return false
						}
					}
				}
			case *ast.CompositeLit:
				for _, element := range current.Elts {
					if expressionUsesObject(info, element, object) {
						disposition = sqlTransactionDischarged
						return false
					}
				}
			}
			return true
		},
	)
	if disposition != sqlTransactionOpen {
		return disposition
	}
	assignment, ok := node.(*ast.AssignStmt)
	if !ok {
		return sqlTransactionOpen
	}
	for _, target := range assignment.Lhs {
		if directObject(info, target) == object {
			return sqlTransactionLost
		}
	}
	return sqlTransactionOpen
}

func sqlTransactionCompletionCall(info *types.Info, call *ast.CallExpr, object types.Object) bool {
	if call == nil || len(call.Args) != 0 {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || directObject(info, selector.X) != object {
		return false
	}
	selection := info.Selections[selector]
	function, _ := selectionObject(selection).(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != databaseSQLPackagePath ||
		!namedReceiver(selection.Recv(), databaseSQLPackagePath, "Tx") {
		return false
	}
	return function.Name() == "Commit" || function.Name() == "Rollback"
}
