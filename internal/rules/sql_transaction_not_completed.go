package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type sqlTransactionNotCompletedRule struct{}

type sqlTransactionCandidate struct {
	identifier *ast.Ident
	object types.Object
	start obligationStart
}

// NewSQLTransactionNotCompletedRule constructs the database transaction
// lifecycle rule for product registry composition.
func NewSQLTransactionNotCompletedRule() Rule {
	return sqlTransactionNotCompletedRule{}
}

func (sqlTransactionNotCompletedRule) Metadata() Metadata {
	return Metadata{
		ID: "sql-transaction-not-completed",
		Summary: "detects database transactions left without commit or rollback",
		Documentation: "database/sql requires every successful transaction to end with Tx.Commit or Tx.Rollback. A locally owned transaction that reaches a normal return without either call can retain a connection, locks, and transaction state. This rule follows direct DB.Begin, DB.BeginTx, and Conn.BeginTx results through the shared control-flow graph after a conventional acquisition-error guard and consumes versioned helper effects and returned-alias contracts.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The initial contract recognizes direct database/sql DB.Begin, DB.BeginTx, and Conn.BeginTx assignments or one-spec initialized local var declarations followed immediately by an err != nil guard whose body returns; declarations containing multiple specifications and parallel multi-expression declarations remain conservative.",
			"A statically resolved helper in a selected local-source module that provably borrows the transaction leaves the obligation open; guaranteed Commit, Rollback, or transfer must cover every normally returning helper path.",
			"An exact returned-alias contract preserves the obligation when the result is assigned back to the same transaction variable; new alias bindings remain outside the tracked ownership identity.",
			"Dynamic calls, interface dispatch, recursion, wrapper finalizers, and helpers outside selected modules retain the conservative ownership-transfer behavior when no summary is available.",
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
		if !sqlTransactionReturnsOpen(candidate, ctx) {
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
				acquisition, found := localCallAcquisitionAtStatement(
					block.List[index],
				)
				if !found {
					continue
				}
				identifier, object, errorObject, matched := sqlBeginAcquisition(
					info,
					acquisition,
				)
				guard, _ := block.List[index + 1].(*ast.IfStmt)
				start := doneBlocks[guard]
				if !matched ||
					!returningNonNilErrorGuard(info, guard, errorObject) ||
					start == nil ||
					!start.Live {
					continue
				}
				result = append(
					result,
					sqlTransactionCandidate{
						identifier: identifier,
						object: object,
						start: obligationStartAt(start),
					},
				)
			}
			return true
		},
	)
	return result
}

func sqlBeginAcquisition(
	info *types.Info,
	acquisition localCallAcquisition,
) (*ast.Ident, types.Object, types.Object, bool) {
	if info == nil || acquisition.call == nil || len(acquisition.identifiers) != 2 {
		return nil, nil, nil, false
	}
	transaction := acquisition.identifiers[0]
	errorIdentifier := acquisition.identifiers[1]
	call := acquisition.call
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

func sqlTransactionReturnsOpen(candidate sqlTransactionCandidate, ctx *ControlFlowContext) bool {
	info := ctx.Info()
	return obligationReachesOpenReturn(
		candidate.start,
		func(node ast.Node) obligationEffect {
			return objectObligationEffect(
				info,
				node,
				candidate.object,
				func(call *ast.CallExpr) bool {
					return sqlTransactionCompletionCall(
						info,
						call,
						candidate.object,
					)
				},
				nil,
				ctx.ParameterEffect,
				ParameterEffectTransactionComplete | ParameterEffectTransfer,
				ctx.ReturnAliasesArgument,
			)
		},
	)
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
