package analysis

import (
	"context"
	"go/ast"
	"go/types"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

type returnStateAnalysis struct {
	ctx context.Context
	summaries map[*types.Func]map[returnStateKey]rules.ReturnStateSummary
	resultStates map[*types.Func]map[int]rules.NilState
	definitions []returnStateDefinition
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
) *returnStateAnalysis {
	analysis := &returnStateAnalysis{
		ctx: ctx,
		summaries: make(map[*types.Func]map[returnStateKey]rules.ReturnStateSummary),
		resultStates: make(map[*types.Func]map[int]rules.NilState),
		definitions: make([]returnStateDefinition, 0),
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
				analysis.definitions = append(
					analysis.definitions,
					returnStateDefinition{
						function: object,
						declaration: function,
						signature: signature,
						info: pkg.TypesInfo,
					},
				)
			}
		}
	}
	return analysis
}

func (a *returnStateAnalysis) buildAll() {
	if a == nil || a.ctx == nil {
		return
	}
	for _, definition := range a.definitions {
		if a.ctx.Err() != nil {
			return
		}
		summaries := summarizeReturnStates(
			definition.declaration,
			definition.signature,
			definition.info,
		)
		if len(summaries) != 0 {
			a.summaries[definition.function] = summaries
		}
		states := summarizeResultStates(
			definition.declaration,
			definition.signature,
			definition.info,
		)
		if len(states) != 0 {
			a.resultStates[definition.function] = states
		}
	}
}

func (a *returnStateAnalysis) buildResultStates() {
	if a == nil || a.ctx == nil {
		return
	}
	for _, definition := range a.definitions {
		if a.ctx.Err() != nil {
			return
		}
		states := summarizeResultStates(
			definition.declaration,
			definition.signature,
			definition.info,
		)
		if len(states) != 0 {
			a.resultStates[definition.function] = states
		}
	}
}

func summarizeResultStates(
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
		state := aggregateResultState(returns, results.Len(), index, resultType, info)
		if state != rules.NilStateUnknown {
			states[index] = state
		}
	}
	return states
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

func aggregateResultState(
	returns []*ast.ReturnStmt,
	resultCount int,
	resultIndex int,
	resultType types.Type,
	info *types.Info,
) rules.NilState {
	state := rules.NilStateUnknown
	for _, returned := range returns {
		if returned == nil ||
			len(returned.Results) != resultCount ||
			resultIndex >= len(returned.Results) {
			return rules.NilStateUnknown
		}
		candidate := classifyResultExpression(
			returned.Results[resultIndex],
			resultType,
			info,
		)
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
