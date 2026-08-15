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
	}
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
