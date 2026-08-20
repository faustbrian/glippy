package analysis

import (
	"context"
	"go/ast"
	"go/types"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const maxResultStateBuildDepth = 4096

type returnStateAnalysis struct {
	ctx context.Context
	effects *nativeEffectFacts
	noReturns *noReturnAnalysis
	summaries map[*types.Func]map[returnStateKey]rules.ReturnStateSummary
	resultStates map[*types.Func]map[int]rules.NilState
	definitions []returnStateDefinition
	definitionsByFunction map[*types.Func]*returnStateDefinition
	resultStatesBuilt map[*types.Func]bool
	resultStatesStarted map[*types.Func]bool
	buildDepth int
}

type returnStateDefinition struct {
	function *types.Func
	declaration *ast.FuncDecl
	signature *types.Signature
	info *types.Info
}

func newReturnStateAnalysis(
	ctx context.Context,
	packages_ []*packages.Package,
	effects *nativeEffectFacts,
	noReturns *noReturnAnalysis,
) *returnStateAnalysis {
	analysis := &returnStateAnalysis{
		ctx: ctx,
		effects: effects,
		noReturns: noReturns,
		summaries: make(map[*types.Func]map[returnStateKey]rules.ReturnStateSummary),
		resultStates: make(map[*types.Func]map[int]rules.NilState),
		definitions: make([]returnStateDefinition, 0),
		definitionsByFunction: make(map[*types.Func]*returnStateDefinition),
		resultStatesBuilt: make(map[*types.Func]bool),
		resultStatesStarted: make(map[*types.Func]bool),
	}
	for _, pkg := range packages_ {
		if ctx == nil || ctx.Err() != nil {
			break
		}
		if pkg == nil || pkg.TypesInfo == nil || pkg.IllTyped {
			continue
		}
		for _, file := range pkg.Syntax {
			if ctx.Err() != nil {
				break
			}
			for _, declaration := range file.Decls {
				if ctx.Err() != nil {
					break
				}
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				object, _ := pkg.TypesInfo.Defs[function.Name].(*types.Func)
				if object == nil {
					continue
				}
				signature, _ := object.Type().(*types.Signature)
				if signature == nil {
					continue
				}
				definition := returnStateDefinition{
					function: object,
					declaration: function,
					signature: signature,
					info: pkg.TypesInfo,
				}
				analysis.definitions = append(analysis.definitions, definition)
			}
		}
	}
	for index := range analysis.definitions {
		definition := &analysis.definitions[index]
		analysis.definitionsByFunction[definition.function] = definition
	}
	return analysis
}

func (a *returnStateAnalysis) buildAll() {
	if a == nil || a.ctx == nil {
		return
	}
	for index := range a.definitions {
		if a.ctx.Err() != nil {
			return
		}
		definition := &a.definitions[index]
		summaries := summarizeReturnStates(
			definition.declaration,
			definition.signature,
			definition.info,
		)
		if len(summaries) != 0 {
			a.summaries[definition.function] = summaries
		}
		a.buildDefinitionResultStates(definition)
	}
}

func (a *returnStateAnalysis) buildResultStates() {
	if a == nil || a.ctx == nil {
		return
	}
	for index := range a.definitions {
		if a.ctx.Err() != nil {
			return
		}
		a.buildDefinitionResultStates(&a.definitions[index])
	}
}

func (a *returnStateAnalysis) buildDefinitionResultStates(
	definition *returnStateDefinition,
) map[int]rules.NilState {
	if a == nil ||
		definition == nil ||
		definition.function == nil ||
		a.ctx == nil ||
		a.ctx.Err() != nil ||
		a.buildDepth >= maxResultStateBuildDepth {
		return nil
	}
	if a.resultStatesBuilt[definition.function] {
		return a.resultStates[definition.function]
	}
	if a.noReturns != nil && a.noReturns.noReturn(definition.function) {
		a.resultStatesBuilt[definition.function] = true
		return nil
	}
	if a.resultStatesStarted[definition.function] {
		return nil
	}
	a.resultStatesStarted[definition.function] = true
	a.buildDepth++
	defer func() {
		a.buildDepth--
		delete(a.resultStatesStarted, definition.function)
	}()
	states := a.summarizeResultStates(
		definition.declaration,
		definition.signature,
		definition.info,
	)
	if len(states) != 0 {
		a.resultStates[definition.function] = states
	}
	a.resultStatesBuilt[definition.function] = true
	return states
}

func (a *returnStateAnalysis) summarizeResultStates(
	function *ast.FuncDecl,
	signature *types.Signature,
	info *types.Info,
) map[int]rules.NilState {
	if function == nil || function.Body == nil || signature == nil || info == nil {
		return nil
	}
	results := signature.Results()
	if results == nil || results.Len() == 0 {
		return nil
	}
	if a.hasDeferredNoReturn(function.Body, info) {
		return nil
	}
	returns := explicitFunctionReturns(function.Body)
	if len(returns) == 0 {
		return nil
	}
	states := make(map[int]rules.NilState)
	for index := range results.Len() {
		result := results.At(index)
		resultType := result.Type()
		if !nilCapableType(resultType) {
			continue
		}
		if namedResultMayChangeAfterReturn(function.Body, result, info) {
			continue
		}
		state := a.aggregateResultState(returns, results.Len(), index, resultType, info)
		if state != rules.NilStateUnknown {
			states[index] = state
		}
	}
	return states
}

func (a *returnStateAnalysis) hasDeferredNoReturn(body *ast.BlockStmt, info *types.Info) bool {
	if body == nil || info == nil {
		return false
	}
	found := false
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if found || node == nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			deferred, ok := node.(*ast.DeferStmt)
			if !ok || deferred.Call == nil {
				return true
			}
			found = !a.resultStateCallMayReturn(deferred.Call, info, 0)
			return !found
		},
	)
	return found
}

func (a *returnStateAnalysis) resultStateCallMayReturn(
	call *ast.CallExpr,
	info *types.Info,
	depth int,
) bool {
	if call == nil || info == nil || depth >= maxResultStateBuildDepth {
		return true
	}
	if literal, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		graph := cfg.New(
			literal.Body,
			func(nested *ast.CallExpr) bool {
				return a.resultStateCallMayReturn(nested, info, depth + 1)
			},
		)
		return !graph.NoReturn()
	}
	identifier, _ := ast.Unparen(call.Fun).(*ast.Ident)
	builtin, _ := info.ObjectOf(identifier).(*types.Builtin)
	if builtin != nil && builtin.Name() == "panic" {
		return false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil {
		return true
	}
	if a.noReturns != nil {
		return !a.noReturns.noReturn(function)
	}
	return !isAuthoritativeNoReturn(function) &&
		(a.effects == nil || !a.effects.noReturn(function))
}

func namedResultMayChangeAfterReturn(
	body *ast.BlockStmt,
	result *types.Var,
	info *types.Info,
) bool {
	if body == nil || result == nil || result.Name() == "" || info == nil {
		return false
	}
	risky := false
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if risky || node == nil {
				return false
			}
			if literal, ok := node.(*ast.FuncLit); ok {
				risky = syntaxUsesObject(literal.Body, result, info)
				return false
			}
			unary, ok := node.(*ast.UnaryExpr)
			if ok &&
				unary.Op.String() == "&" &&
				directSyntaxObject(unary.X, info) == result {
				risky = true
				return false
			}
			return true
		},
	)
	return risky
}

func syntaxUsesObject(node ast.Node, object types.Object, info *types.Info) bool {
	if node == nil || object == nil || info == nil {
		return false
	}
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if found || current == nil {
				return false
			}
			identifier, ok := current.(*ast.Ident)
			if ok && info.ObjectOf(identifier) == object {
				found = true
				return false
			}
			return true
		},
	)
	return found
}

func directSyntaxObject(expression ast.Expr, info *types.Info) types.Object {
	identifier, _ := ast.Unparen(expression).(*ast.Ident)
	if identifier == nil || info == nil {
		return nil
	}
	return info.ObjectOf(identifier)
}

func (a *returnStateAnalysis) aggregateResultState(
	returns []*ast.ReturnStmt,
	resultCount int,
	resultIndex int,
	resultType types.Type,
	info *types.Info,
) rules.NilState {
	state := rules.NilStateUnknown
	for _, returned := range returns {
		if returned == nil || len(returned.Results) == 0 {
			return rules.NilStateUnknown
		}
		var candidate rules.NilState
		switch {
		case len(returned.Results) == resultCount && resultIndex < len(returned.Results):
			expression := returned.Results[resultIndex]
			candidate = classifyResultExpression(expression, resultType, info)
			if candidate == rules.NilStateUnknown {
				candidate = a.delegatedResultState(
					expression,
					0,
					1,
					resultType,
					info,
				)
			}
		case len(returned.Results) == 1:
			candidate = a.delegatedResultState(
				returned.Results[0],
				resultIndex,
				resultCount,
				resultType,
				info,
			)
		default:
			return rules.NilStateUnknown
		}
		if candidate == rules.NilStateUnknown {
			return rules.NilStateUnknown
		}
		if state != rules.NilStateUnknown && state != candidate {
			return rules.NilStateUnknown
		}
		state = candidate
	}
	return state
}

func (a *returnStateAnalysis) delegatedResultState(
	expression ast.Expr,
	resultIndex int,
	wantResults int,
	wantType types.Type,
	info *types.Info,
) rules.NilState {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil || info == nil || resultIndex < 0 || wantResults <= 0 || wantType == nil {
		return rules.NilStateUnknown
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil {
		return rules.NilStateUnknown
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil ||
		signature.Results() == nil ||
		signature.Results().Len() != wantResults ||
		resultIndex >= signature.Results().Len() ||
		!types.Identical(signature.Results().At(resultIndex).Type(), wantType) {
		return rules.NilStateUnknown
	}
	if definition := a.definitionsByFunction[function]; definition != nil {
		return a.buildDefinitionResultStates(definition)[resultIndex]
	}
	if a.effects == nil {
		return rules.NilStateUnknown
	}
	return a.effects.ResultState(function, resultIndex)
}

func classifyResultExpression(
	expression ast.Expr,
	resultType types.Type,
	info *types.Info,
) rules.NilState {
	if isBuiltinError(resultType) {
		return classifyErrorExpression(expression, info)
	}
	return classifyNilExpression(expression, info)
}

func summarizeReturnStates(
	function *ast.FuncDecl,
	signature *types.Signature,
	info *types.Info,
) map[returnStateKey]rules.ReturnStateSummary {
	results := signature.Results()
	if function == nil || function.Body == nil || results == nil || info == nil {
		return nil
	}
	valueIndexes := make([]int, 0, results.Len())
	errorIndexes := make([]int, 0, 1)
	for index := range results.Len() {
		resultType := results.At(index).Type()
		if nilCapableType(resultType) {
			valueIndexes = append(valueIndexes, index)
		}
		if isBuiltinError(resultType) {
			errorIndexes = append(errorIndexes, index)
		}
	}
	if len(valueIndexes) == 0 || len(errorIndexes) == 0 {
		return nil
	}

	returns := explicitFunctionReturns(function.Body)
	if len(returns) == 0 {
		return nil
	}
	summaries := make(map[returnStateKey]rules.ReturnStateSummary)
	for _, valueIndex := range valueIndexes {
		for _, errorIndex := range errorIndexes {
			if valueIndex == errorIndex {
				continue
			}
			whenNil := aggregateReturnState(returns, valueIndex, errorIndex, true, info)
			whenNonNil := aggregateReturnState(
				returns,
				valueIndex,
				errorIndex,
				false,
				info,
			)
			if whenNil == rules.NilStateUnknown && whenNonNil == rules.NilStateUnknown {
				continue
			}
			summaries[returnStateKey{value: valueIndex, error: errorIndex}] = rules.ReturnStateSummary{
				WhenErrorNil: whenNil,
				WhenErrorNonNil: whenNonNil,
			}
		}
	}
	return summaries
}

func explicitFunctionReturns(body *ast.BlockStmt) []*ast.ReturnStmt {
	returns := make([]*ast.ReturnStmt, 0)
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if returned, ok := node.(*ast.ReturnStmt); ok {
				returns = append(returns, returned)
			}
			return true
		},
	)
	return returns
}

func aggregateReturnState(
	returns []*ast.ReturnStmt,
	valueIndex int,
	errorIndex int,
	errorIsNil bool,
	info *types.Info,
) rules.NilState {
	state := rules.NilStateUnknown
	found := false
	for _, returned := range returns {
		if returned == nil ||
			len(returned.Results) <= valueIndex ||
			len(returned.Results) <= errorIndex {
			return rules.NilStateUnknown
		}
		errorState := classifyErrorExpression(returned.Results[errorIndex], info)
		if errorState == rules.NilStateUnknown {
			return rules.NilStateUnknown
		}
		if (errorState == rules.NilStateNil) != errorIsNil {
			continue
		}
		valueState := classifyNilExpression(returned.Results[valueIndex], info)
		if valueState == rules.NilStateUnknown {
			return rules.NilStateUnknown
		}
		if found && state != valueState {
			return rules.NilStateUnknown
		}
		state = valueState
		found = true
	}
	if !found {
		return rules.NilStateUnknown
	}
	return state
}

func classifyErrorExpression(expression ast.Expr, info *types.Info) rules.NilState {
	if isNilExpression(expression, info) {
		return rules.NilStateNil
	}
	call, ok := ast.Unparen(expression).(*ast.CallExpr)
	if !ok {
		return rules.NilStateUnknown
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil {
		return rules.NilStateUnknown
	}
	switch function.Pkg().Path() {
	case "errors":
		if function.Name() == "New" {
			return rules.NilStateNonNil
		}
	case "fmt":
		if function.Name() == "Errorf" {
			return rules.NilStateNonNil
		}
	}
	return rules.NilStateUnknown
}

func classifyNilExpression(expression ast.Expr, info *types.Info) rules.NilState {
	expression = ast.Unparen(expression)
	if isNilExpression(expression, info) {
		return rules.NilStateNil
	}
	switch expression := expression.(type) {
	case *ast.UnaryExpr:
		if expression.Op.String() == "&" {
			if _, indirect := ast.Unparen(expression.X).(*ast.StarExpr); indirect {
				return rules.NilStateUnknown
			}
			return rules.NilStateNonNil
		}
	case *ast.FuncLit:
		return rules.NilStateNonNil
	case *ast.CompositeLit:
		if literalType := info.TypeOf(expression); literalType != nil {
			switch literalType.Underlying().(type) {
			case *types.Slice, *types.Map:
				return rules.NilStateNonNil
			}
		}
	case *ast.CallExpr:
		identifier, ok := ast.Unparen(expression.Fun).(*ast.Ident)
		if !ok {
			break
		}
		builtin, _ := info.Uses[identifier].(*types.Builtin)
		if builtin != nil && (builtin.Name() == "new" || builtin.Name() == "make") {
			return rules.NilStateNonNil
		}
	}
	return rules.NilStateUnknown
}

func isNilExpression(expression ast.Expr, info *types.Info) bool {
	identifier, ok := ast.Unparen(expression).(*ast.Ident)
	return ok &&
		identifier.Name == "nil" &&
		info.Uses[identifier] == types.Universe.Lookup("nil")
}

func nilCapableType(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	switch type_.Underlying().(type) {
	case *types.Pointer,
		*types.Slice,
		*types.Map,
		*types.Chan,
		*types.Signature,
		*types.Interface:
		return true
	default:
		return false
	}
}

func isBuiltinError(type_ types.Type) bool {
	errorObject := types.Universe.Lookup("error")
	return errorObject != nil && types.Identical(type_, errorObject.Type())
}
