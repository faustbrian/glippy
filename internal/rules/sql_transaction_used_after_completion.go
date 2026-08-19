package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

const maxTrackedFunctionTransactions = 4096

type sqlTransactionUsedAfterCompletionRule struct{}

type sqlTransactionState uint8

const (
	sqlTransactionOpen sqlTransactionState = 1 << iota
	sqlTransactionCompleted
	sqlTransactionUnknown
)

type sqlTransactionFlowState struct {
	value *sqlTransactionState
}

type sqlTransactionStateIssue struct {
	object types.Object
	call *ast.CallExpr
	completion *ast.CallExpr
}

type sqlTransactionStateAnalysis struct {
	complete bool
	issues []sqlTransactionStateIssue
}

type sqlTransactionStateBuilder struct {
	ctx *ControlFlowContext
	candidate sqlTransactionCandidate
	completionCalls []*ast.CallExpr
	issues map[token.Pos]sqlTransactionStateIssue
	record bool
}

type sqlTransactionCallKind uint8

const (
	sqlTransactionCallCompletion sqlTransactionCallKind = iota
	sqlTransactionCallOperation
	sqlTransactionCallUnknown
)

type sqlTransactionCallEffect struct {
	kind sqlTransactionCallKind
	call *ast.CallExpr
}

// NewSQLTransactionUsedAfterCompletionRule constructs the completed
// transaction use rule for product registry composition.
func NewSQLTransactionUsedAfterCompletionRule() Rule {
	return sqlTransactionUsedAfterCompletionRule{}
}

func (sqlTransactionUsedAfterCompletionRule) Metadata() Metadata {
	return Metadata{
		ID: "sql-transaction-used-after-completion",
		Summary: "detects operations on definitely completed database transactions",
		Documentation: "After database/sql Tx.Commit or Tx.Rollback, every transaction operation fails with ErrTxDone. The rule follows directly acquired transactions through bounded control flow, consumes proven project transaction-completion effects, and reports only when every reaching state is completed.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The initial contract recognizes direct database/sql DB.Begin, DB.BeginTx, and Conn.BeginTx assignments followed immediately by a returning acquisition-error guard.",
			"A finding requires every reaching path to have completed the transaction; conditional completion, aliases, ownership transfer, reassignment, asynchronous use, and unknown helpers become conservative unknown state.",
			"Deferred completion is not applied at registration time, and a CFG node containing multiple transaction calls becomes unknown instead of assuming nested evaluation order.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Do not execute after committing a transaction",
				Incorrect: "tx, err := db.Begin()\nif err != nil { return err }\ntx.Commit()\n_, err = tx.Exec(query)",
				Correct: "tx, err := db.Begin()\nif err != nil { return err }\n_, err = tx.Exec(query)\nif err != nil { return err }\nreturn tx.Commit()",
			},
		},
	}
}

func (sqlTransactionUsedAfterCompletionRule) RunControlFlow(
	ctx *ControlFlowContext,
) ([]Finding, error) {
	if ctx == nil ||
		ctx.Body() == nil ||
		ctx.Graph() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf(
			"sql-transaction-used-after-completion requires a complete control-flow context",
		)
	}
	if !packageImports(ctx.Package(), databaseSQLPackagePath) {
		return nil, nil
	}
	analysis := sqlTransactionStateAnalysisFor(ctx)
	if analysis == nil || !analysis.complete {
		return nil, nil
	}
	findings := make([]Finding, 0, len(analysis.issues))
	for _, issue := range analysis.issues {
		range_, err := ctx.Range(issue.call)
		if err != nil {
			return nil, err
		}
		finding := Finding{
			MessageKey: "transaction-used-after-completion",
			Message: fmt.Sprintf(
				"transaction %q is used after it is committed or rolled back",
				issue.object.Name(),
			),
			Range: range_,
			Help: "move the operation before Commit or Rollback, or begin a new transaction",
		}
		if issue.completion != nil {
			completionRange, rangeErr := ctx.Range(issue.completion)
			if rangeErr != nil {
				return nil, rangeErr
			}
			finding.Related = []Related{
				{Range: completionRange, Message: "transaction completed here"},
			}
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func sqlTransactionStateAnalysisFor(ctx *ControlFlowContext) *sqlTransactionStateAnalysis {
	if ctx.shared == nil {
		return buildSQLTransactionStateAnalysis(ctx)
	}
	ctx.shared.transactionStateOnce.Do(
		func() {
			ctx.shared.transactionState = buildSQLTransactionStateAnalysis(ctx)
		},
	)
	return ctx.shared.transactionState
}

func buildSQLTransactionStateAnalysis(ctx *ControlFlowContext) *sqlTransactionStateAnalysis {
	candidates := sqlTransactionCandidates(ctx.Body(), ctx.Graph(), ctx.Info())
	if len(candidates) == 0 {
		return &sqlTransactionStateAnalysis{complete: true}
	}
	if len(candidates) > maxTrackedFunctionTransactions {
		return &sqlTransactionStateAnalysis{}
	}
	issues := make(map[token.Pos]sqlTransactionStateIssue)
	for _, candidate := range candidates {
		builder := &sqlTransactionStateBuilder{
			ctx: ctx,
			candidate: candidate,
			completionCalls: collectSQLTransactionCompletionCalls(
				ctx,
				candidate.object,
			),
			issues: issues,
		}
		changeBound := len(ctx.Graph().Blocks) * 8
		if changeBound <= 0 || changeBound > maxStateTransitionChanges {
			changeBound = maxStateTransitionChanges
		}
		initial := sqlTransactionOpen
		snapshot, complete := runStateTransitions(
			ctx.Graph(),
			stateTransitionModel[sqlTransactionFlowState]{
				Initial: sqlTransactionFlowState{value: &initial},
				Entry: candidate.start.block,
				Clone: func(state sqlTransactionFlowState) sqlTransactionFlowState {
					value := *state.value
					return sqlTransactionFlowState{value: &value}
				},
				Merge: func(
					existing *sqlTransactionFlowState,
					incoming sqlTransactionFlowState,
				) bool {
					merged := *existing.value | *incoming.value
					if merged == *existing.value {
						return false
					}
					*existing.value = merged
					return true
				},
				Transfer: builder.transfer,
				MaxChanges: changeBound,
			},
		)
		if !complete {
			return &sqlTransactionStateAnalysis{}
		}
		builder.record = true
		for _, block := range ctx.Graph().Blocks {
			if block == nil ||
				!block.Live ||
				block.Index < 0 ||
				int(block.Index) >= len(snapshot.entries) ||
				!snapshot.present[block.Index] {
				continue
			}
			state := snapshot.entries[block.Index]
			for _, node := range block.Nodes {
				builder.transfer(state, node)
			}
		}
	}
	result := make([]sqlTransactionStateIssue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, issue)
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			return result[left].call.Pos() < result[right].call.Pos()
		},
	)
	return &sqlTransactionStateAnalysis{complete: true, issues: result}
}

func (b *sqlTransactionStateBuilder) transfer(flow sqlTransactionFlowState, node ast.Node) bool {
	state := *flow.value
	if transactionAddressTaken(b.ctx.Info(), node, b.candidate.object) {
		*flow.value = sqlTransactionUnknown
		return true
	}
	if asynchronous, ok := node.(*ast.GoStmt);
		ok &&
			sqlTransactionMethodName(
				b.ctx.Info(),
				asynchronous.Call,
				b.candidate.object,
			) !=
				"" {
		*flow.value = sqlTransactionUnknown
		return true
	}
	effects := b.callEffects(node)
	if len(effects) > 1 {
		*flow.value = sqlTransactionUnknown
		return true
	}
	if len(effects) == 1 {
		effect := effects[0]
		if _, deferred := node.(*ast.DeferStmt); deferred {
			return true
		}
		if _, asynchronous := node.(*ast.GoStmt); asynchronous {
			*flow.value = sqlTransactionUnknown
			return true
		}
		switch effect.kind {
		case sqlTransactionCallCompletion:
			if state == sqlTransactionCompleted {
				b.addIssue(effect.call)
			}
			state = sqlTransactionCompleted
		case sqlTransactionCallOperation:
			if state == sqlTransactionCompleted {
				b.addIssue(effect.call)
			}
		case sqlTransactionCallUnknown:
			state = sqlTransactionUnknown
		}
	}
	effect := objectObligationEffect(
		b.ctx.Info(),
		node,
		b.candidate.object,
		nil,
		b.ctx.ParameterEffect,
		ParameterEffectTransactionComplete,
	)
	switch effect {
	case obligationCompleted:
		state = sqlTransactionCompleted
	case obligationTransferred, obligationLost:
		state = sqlTransactionUnknown
	}
	*flow.value = state
	return true
}

func (b *sqlTransactionStateBuilder) callEffects(node ast.Node) []sqlTransactionCallEffect {
	result := make([]sqlTransactionCallEffect, 0, 1)
	for _, call := range callsInLockNode(node) {
		if method := sqlTransactionMethodName(b.ctx.Info(), call, b.candidate.object);
			method != "" {
			kind := sqlTransactionCallOperation
			if method == "Commit" || method == "Rollback" {
				kind = sqlTransactionCallCompletion
			}
			result = append(result, sqlTransactionCallEffect{kind: kind, call: call})
			continue
		}
		for index, argument := range call.Args {
			if directObject(b.ctx.Info(), argument) != b.candidate.object {
				if expressionUsesObject(
					b.ctx.Info(),
					argument,
					b.candidate.object,
				) {
					result = append(
						result,
						sqlTransactionCallEffect{
							kind: sqlTransactionCallUnknown,
							call: call,
						},
					)
				}
				continue
			}
			summary := b.ctx.ParameterEffect(call, index)
			kind := sqlTransactionCallUnknown
			if summary.GuaranteesAny(ParameterEffectTransactionComplete) {
				kind = sqlTransactionCallCompletion
			} else if summary.Known && summary.Kinds == 0 {
				continue
			}
			result = append(result, sqlTransactionCallEffect{kind: kind, call: call})
		}
	}
	return result
}

func sqlTransactionMethodName(info *types.Info, call *ast.CallExpr, object types.Object) string {
	if info == nil || call == nil || object == nil {
		return ""
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || directObject(info, selector.X) != object {
		return ""
	}
	selection := info.Selections[selector]
	function, _ := selectionObject(selection).(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != databaseSQLPackagePath ||
		!namedReceiver(selection.Recv(), databaseSQLPackagePath, "Tx") {
		return ""
	}
	return function.Name()
}

func transactionAddressTaken(info *types.Info, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			unary, ok := current.(*ast.UnaryExpr)
			if ok && unary.Op == token.AND && directObject(info, unary.X) == object {
				found = true
				return false
			}
			return !found
		},
	)
	return found
}

func collectSQLTransactionCompletionCalls(
	ctx *ControlFlowContext,
	object types.Object,
) []*ast.CallExpr {
	result := make([]*ast.CallExpr, 0)
	root := ctx.Function()
	ast.PreorderStack(
		root,
		nil,
		func(node ast.Node, stack []ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested && literal != root {
				return false
			}
			call, _ := node.(*ast.CallExpr)
			if call == nil || deferredOrAsynchronousCall(call, stack) {
				return true
			}
			method := sqlTransactionMethodName(ctx.Info(), call, object)
			if method == "Commit" ||
				method == "Rollback" ||
				helperGuaranteesTransactionCompletion(ctx, call, object) {
				result = append(result, call)
			}
			return true
		},
	)
	return result
}

func helperGuaranteesTransactionCompletion(
	ctx *ControlFlowContext,
	call *ast.CallExpr,
	object types.Object,
) bool {
	for index, argument := range call.Args {
		if directObject(ctx.Info(), argument) == object &&
			ctx.ParameterEffect(call, index).GuaranteesAny(
				ParameterEffectTransactionComplete,
			) {
			return true
		}
	}
	return false
}

func (b *sqlTransactionStateBuilder) unambiguousCompletion(before token.Pos) *ast.CallExpr {
	var result *ast.CallExpr
	for _, call := range b.completionCalls {
		if call.Pos() >= before {
			break
		}
		if result != nil {
			return nil
		}
		result = call
	}
	return result
}

func (b *sqlTransactionStateBuilder) addIssue(call *ast.CallExpr) {
	if !b.record || call == nil || !call.Pos().IsValid() {
		return
	}
	if _, found := b.issues[call.Pos()]; found {
		return
	}
	b.issues[call.Pos()] = sqlTransactionStateIssue{
		object: b.candidate.object,
		call: call,
		completion: b.unambiguousCompletion(call.Pos()),
	}
}
