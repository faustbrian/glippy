package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

type unreachableCodeRule struct{}

const removeUnreachableCodeFix = "remove-unreachable-code"

// NewUnreachableCodeRule constructs the shared-CFG unreachable statement rule.
func NewUnreachableCodeRule() Rule {
	return unreachableCodeRule{}
}

func (unreachableCodeRule) Metadata() Metadata {
	return Metadata{
		ID: "unreachable-code",
		Summary: "detects statements that execution cannot reach",
		Documentation: "Statements following an unconditional return, panic, terminating branch, infinite loop, or proven no-return call cannot execute. They often preserve stale work after a refactor or conceal control-flow mistakes. Glippy uses the shared no-return analysis behind its control-flow graph so selected local-source module facts participate without retaining dependency source as a lint target.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		RunDespiteTypeErrors: true,
		Categories: []Category{CategoryCorrectness},
		Fixes: []FixMetadata{
			{
				Name: removeUnreachableCodeFix,
				Description: "remove the unreachable statement",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"The control-flow walk reports the first unreachable statement in each contiguous lexical region.",
			"Same-module imported no-return helpers are recognized; dynamic calls, recursion without a proven terminal path, and helpers outside selected modules remain conservatively returning.",
			"A direct return or built-in panic required for a value-returning function to satisfy Go's syntactic termination check after a proven helper call is not reported.",
			"A direct proven no-return helper call may be followed by a final unlabeled break or continue without being reported; labeled branches, compound terminal flow, and retained work remain diagnostics.",
			"An exact testing FailNow, Fatal, or Fatalf call, or a proven selected local-source wrapper whose terminal paths are testing failures, may also be followed by a final bare return without being reported. Exact testing calls in value-returning functions may additionally use one zero-value variable declaration and a return of only those variables; empty or initialized declarations, retained work, mixed terminal wrappers, and lookalikes remain diagnostics.",
			"Source retained after an exact testing Skip, Skipf, or SkipNow call, an exact Ginkgo Skip call, or a proven selected local-source skip wrapper is treated as an intentional disabled-test body and is not reported.",
			"A built-in panic with the constant message \"unreachable\" after a proven no-return call is treated as an intentional sentinel and is not reported.",
			"Removal remains suggestion-only because comments and intentionally retained examples require review.",
		},
		Examples: []Example{
			{
				Title: "Remove statements after a terminal return",
				Incorrect: "return\nwork()",
				Correct: "work()\nreturn",
			},
		},
	}
}

func (unreachableCodeRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil {
		return nil, fmt.Errorf("unreachable-code requires a complete control-flow context")
	}
	walker := newUnreachableWalker(ctx)
	walker.discover(ctx.Body())
	walker.reachable = true
	walker.walk(ctx.Body())
	findings := make([]Finding, 0, len(walker.unreachable))
	for _, statement := range walker.unreachable {
		range_, err := ctx.Range(statement)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "unreachable-code",
				Message: "unreachable code",
				Range: range_,
				Help: "remove the unreachable statement or restore a reachable path",
				Fixes: []Fix{
					{
						Name: removeUnreachableCodeFix,
						Safety: FixSuggestion,
						Edits: []Edit{{Range: range_, NewText: ""}},
					},
				},
			},
		)
	}
	return findings, nil
}

type unreachableWalker struct {
	ctx *ControlFlowContext
	unreachable []ast.Stmt
	breaks map[ast.Stmt]int
	breakTargets map[*ast.BranchStmt]ast.Stmt
	gotos map[string]bool
	labels map[string]ast.Stmt
	breakTarget ast.Stmt
	reachable bool
	requiresReturn bool
	sentinelAfterNoReturn bool
	hasResults bool
}

func newUnreachableWalker(ctx *ControlFlowContext) *unreachableWalker {
	return &unreachableWalker{
		ctx: ctx,
		unreachable: make([]ast.Stmt, 0),
		breaks: make(map[ast.Stmt]int),
		breakTargets: make(map[*ast.BranchStmt]ast.Stmt),
		gotos: make(map[string]bool),
		labels: make(map[string]ast.Stmt),
		hasResults: functionHasResults(ctx.Function()),
	}
}

func functionHasResults(function ast.Node) bool {
	var results *ast.FieldList
	switch function := function.(type) {
	case *ast.FuncDecl:
		results = function.Type.Results
	case *ast.FuncLit:
		results = function.Type.Results
	}
	return results != nil && len(results.List) != 0
}

func (w *unreachableWalker) discover(statement ast.Stmt) {
	if statement == nil {
		return
	}
	switch statement := statement.(type) {
	case *ast.BlockStmt:
		w.discoverList(statement.List)
	case *ast.BranchStmt:
		w.discoverBranch(statement)
	case *ast.IfStmt:
		w.discover(statement.Body)
		w.discover(statement.Else)
	case *ast.LabeledStmt:
		w.labels[statement.Label.Name] = statement.Stmt
		w.discover(statement.Stmt)
	case *ast.ForStmt:
		w.discoverBreakable(statement, statement.Body)
	case *ast.RangeStmt:
		w.discoverBreakable(statement, statement.Body)
	case *ast.SelectStmt:
		w.discoverBreakable(statement, statement.Body)
	case *ast.SwitchStmt:
		w.discoverBreakable(statement, statement.Body)
	case *ast.TypeSwitchStmt:
		w.discoverBreakable(statement, statement.Body)
	case *ast.CaseClause:
		w.discoverList(statement.Body)
	case *ast.CommClause:
		w.discoverList(statement.Body)
	}
}

func (w *unreachableWalker) discoverList(statements []ast.Stmt) {
	for _, statement := range statements {
		w.discover(statement)
	}
}

func (w *unreachableWalker) discoverBranch(statement *ast.BranchStmt) {
	switch statement.Tok {
	case token.GOTO:
		if statement.Label != nil {
			w.gotos[statement.Label.Name] = true
		}
	case token.BREAK:
		target := w.breakTarget
		if statement.Label != nil {
			target = w.labels[statement.Label.Name]
		}
		if target != nil {
			w.breaks[target]++
			w.breakTargets[statement] = target
		}
	}
}

func (w *unreachableWalker) discoverBreakable(owner ast.Stmt, body *ast.BlockStmt) {
	outer := w.breakTarget
	w.breakTarget = owner
	w.discover(body)
	w.breakTarget = outer
}

func (w *unreachableWalker) walk(statement ast.Stmt) {
	if statement == nil {
		return
	}
	if labeled, ok := statement.(*ast.LabeledStmt); ok && w.gotos[labeled.Label.Name] {
		w.reachable = true
		w.requiresReturn = false
		w.sentinelAfterNoReturn = false
	}
	if !w.reachable {
		if _, empty := statement.(*ast.EmptyStmt); !empty {
			if !w.requiresReturn ||
				!w.hasResults ||
				!isSyntacticReturnTerminator(w.ctx.Info(), statement) {
				if !w.sentinelAfterNoReturn ||
					!isUnreachableSentinelPanic(w.ctx.Info(), statement) {
					w.unreachable = append(w.unreachable, statement)
				}
			}
			w.reachable = true
			w.requiresReturn = false
			w.sentinelAfterNoReturn = false
		}
	}
	switch statement := statement.(type) {
	case *ast.BlockStmt:
		w.walkList(statement.List)
	case *ast.BranchStmt:
		switch statement.Tok {
		case token.BREAK, token.CONTINUE, token.FALLTHROUGH, token.GOTO:
			w.reachable = false
			w.requiresReturn = false
			w.sentinelAfterNoReturn = false
		}
	case *ast.ExprStmt:
		call, _ := statement.X.(*ast.CallExpr)
		if call != nil &&
			!w.ctx.CallMayReturn(call) &&
			!isTestingSkip(w.ctx.Info(), call) &&
			!w.ctx.CallIsTestingSkip(call) {
			w.reachable = false
			w.requiresReturn = !isBuiltinPanic(w.ctx.Info(), call)
			w.sentinelAfterNoReturn = w.requiresReturn
		}
	case *ast.ForStmt:
		w.walk(statement.Body)
		w.reachable = statement.Cond != nil || w.breaks[statement] != 0
		w.requiresReturn = false
		w.sentinelAfterNoReturn = false
	case *ast.IfStmt:
		w.walk(statement.Body)
		if statement.Else == nil {
			w.reachable = true
			w.requiresReturn = false
			w.sentinelAfterNoReturn = false
			return
		}
		thenReaches := w.reachable
		thenRequiresReturn := w.requiresReturn
		thenSentinelAfterNoReturn := w.sentinelAfterNoReturn
		w.reachable = true
		w.requiresReturn = false
		w.sentinelAfterNoReturn = false
		w.walk(statement.Else)
		elseReaches := w.reachable
		elseSentinelAfterNoReturn := w.sentinelAfterNoReturn
		w.reachable = elseReaches || thenReaches
		w.requiresReturn = !w.reachable && (w.requiresReturn || thenRequiresReturn)
		w.sentinelAfterNoReturn = !w.reachable &&
			thenSentinelAfterNoReturn &&
			elseSentinelAfterNoReturn
	case *ast.LabeledStmt:
		w.walk(statement.Stmt)
	case *ast.RangeStmt:
		w.walk(statement.Body)
		w.reachable = true
		w.requiresReturn = false
		w.sentinelAfterNoReturn = false
	case *ast.SelectStmt:
		w.walkClauses(statement.Body.List, false, statement)
	case *ast.SwitchStmt:
		w.walkClauses(statement.Body.List, true, statement)
	case *ast.TypeSwitchStmt:
		w.walkClauses(statement.Body.List, true, statement)
	case *ast.ReturnStmt:
		w.reachable = false
		w.requiresReturn = false
		w.sentinelAfterNoReturn = false
	}
}

func isBuiltinPanic(info *types.Info, call *ast.CallExpr) bool {
	if info == nil || call == nil {
		return false
	}
	identifier, _ := ast.Unparen(call.Fun).(*ast.Ident)
	return identifier != nil && info.Uses[identifier] == types.Universe.Lookup("panic")
}

func isSyntacticReturnTerminator(info *types.Info, statement ast.Stmt) bool {
	if _, ok := statement.(*ast.ReturnStmt); ok {
		return true
	}
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return false
	}
	call, _ := expression.X.(*ast.CallExpr)
	return isBuiltinPanic(info, call)
}

func isUnreachableSentinelPanic(info *types.Info, statement ast.Stmt) bool {
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return false
	}
	call, _ := expression.X.(*ast.CallExpr)
	if !isBuiltinPanic(info, call) || len(call.Args) != 1 || info == nil {
		return false
	}
	value := info.Types[call.Args[0]].Value
	return value != nil &&
		value.Kind() == constant.String &&
		constant.StringVal(value) == "unreachable"
}

func isTestingSkip(info *types.Info, call *ast.CallExpr) bool {
	function := staticTestingMethod(info, call)
	if function == nil {
		return false
	}
	switch function.Name() {
	case "Skip", "Skipf", "SkipNow":
		return true
	default:
		return false
	}
}

func (w *unreachableWalker) walkList(statements []ast.Stmt) {
	for index := 0; index < len(statements); index++ {
		statement := statements[index]
		w.walk(statement)
		if w.reachable || !w.requiresReturn {
			continue
		}
		remaining := statements[index + 1:]
		if isDirectProvenNoReturnStatement(w.ctx, statement) &&
			isNoReturnLoopControlShim(remaining) {
			w.discardNoReturnShimBreak(remaining[0])
			index++
			w.requiresReturn = false
			w.sentinelAfterNoReturn = false
			continue
		}
		if !isTestingFailureStatement(w.ctx, statement) {
			continue
		}
		if isTestingReturnShim(remaining) {
			index++
			w.requiresReturn = false
			w.sentinelAfterNoReturn = false
			continue
		}
		if w.hasResults &&
			isDirectTestingFailureStatement(w.ctx.Info(), statement) &&
			isZeroReturnShim(w.ctx.Info(), remaining) {
			index += 2
			w.requiresReturn = false
			w.sentinelAfterNoReturn = false
		}
	}
}

func isDirectProvenNoReturnStatement(ctx *ControlFlowContext, statement ast.Stmt) bool {
	if ctx == nil {
		return false
	}
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return false
	}
	call, _ := expression.X.(*ast.CallExpr)
	return call != nil && !isBuiltinPanic(ctx.Info(), call) && !ctx.CallMayReturn(call)
}

func isNoReturnLoopControlShim(statements []ast.Stmt) bool {
	if len(statements) != 1 {
		return false
	}
	branch, _ := statements[0].(*ast.BranchStmt)
	return branch != nil &&
		branch.Label == nil &&
		(branch.Tok == token.BREAK || branch.Tok == token.CONTINUE)
}

func isTestingReturnShim(statements []ast.Stmt) bool {
	if len(statements) != 1 {
		return false
	}
	return_, _ := statements[0].(*ast.ReturnStmt)
	return return_ != nil && len(return_.Results) == 0
}

func (w *unreachableWalker) discardNoReturnShimBreak(statement ast.Stmt) {
	branch, _ := statement.(*ast.BranchStmt)
	if branch == nil || branch.Tok != token.BREAK {
		return
	}
	target := w.breakTargets[branch]
	if target == nil {
		return
	}
	if w.breaks[target] <= 1 {
		delete(w.breaks, target)
		return
	}
	w.breaks[target]--
}

func isTestingFailureStatement(ctx *ControlFlowContext, statement ast.Stmt) bool {
	if ctx == nil {
		return false
	}
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return false
	}
	call, _ := expression.X.(*ast.CallExpr)
	return call != nil && ctx.CallIsTestingFailure(call)
}

func isDirectTestingFailureStatement(info *types.Info, statement ast.Stmt) bool {
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return false
	}
	call, _ := expression.X.(*ast.CallExpr)
	function := staticTestingMethod(info, call)
	if function == nil {
		return false
	}
	switch function.Name() {
	case "FailNow", "Fatal", "Fatalf":
		return true
	default:
		return false
	}
}

func isZeroReturnShim(info *types.Info, statements []ast.Stmt) bool {
	if info == nil || len(statements) != 2 {
		return false
	}
	declaration, _ := statements[0].(*ast.DeclStmt)
	return_, _ := statements[1].(*ast.ReturnStmt)
	if declaration == nil || return_ == nil {
		return false
	}
	general, _ := declaration.Decl.(*ast.GenDecl)
	if general == nil || general.Tok != token.VAR {
		return false
	}
	objects := make(map[types.Object]bool)
	for _, specification := range general.Specs {
		values, _ := specification.(*ast.ValueSpec)
		if values == nil || len(values.Names) == 0 || len(values.Values) != 0 {
			return false
		}
		for _, name := range values.Names {
			object := info.Defs[name]
			if object == nil || name.Name == "_" {
				return false
			}
			objects[object] = false
		}
	}
	if len(objects) == 0 || len(return_.Results) != len(objects) {
		return false
	}
	for _, result := range return_.Results {
		identifier, _ := ast.Unparen(result).(*ast.Ident)
		if identifier == nil {
			return false
		}
		object := info.Uses[identifier]
		used, exists := objects[object]
		if !exists || used {
			return false
		}
		objects[object] = true
	}
	return true
}

func staticTestingMethod(info *types.Info, call *ast.CallExpr) *types.Func {
	if info == nil || call == nil {
		return nil
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "testing" {
		return nil
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return nil
	}
	return function
}

func (w *unreachableWalker) walkClauses(
	clauses []ast.Stmt,
	missingDefaultReturns bool,
	owner ast.Stmt,
) {
	anyReaches := false
	anyRequiresReturn := false
	allSentinelAfterNoReturn := len(clauses) != 0
	hasDefault := false
	for _, clause := range clauses {
		w.reachable = true
		w.requiresReturn = false
		w.sentinelAfterNoReturn = false
		switch clause := clause.(type) {
		case *ast.CaseClause:
			hasDefault = hasDefault || clause.List == nil
			w.walkList(clause.Body)
		case *ast.CommClause:
			hasDefault = hasDefault || clause.Comm == nil
			w.walkList(clause.Body)
		}
		anyReaches = anyReaches || w.reachable
		anyRequiresReturn = anyRequiresReturn || !w.reachable && w.requiresReturn
		allSentinelAfterNoReturn = allSentinelAfterNoReturn &&
			!w.reachable &&
			w.sentinelAfterNoReturn
	}
	w.reachable = anyReaches || w.breaks[owner] != 0 || missingDefaultReturns && !hasDefault
	w.requiresReturn = !w.reachable && anyRequiresReturn
	w.sentinelAfterNoReturn = !w.reachable && allSentinelAfterNoReturn
}
