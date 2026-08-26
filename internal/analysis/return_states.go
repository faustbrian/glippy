package analysis

import (
	"context"
	"go/ast"
	"go/build/constraint"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const maxReturnStateBuildDepth = 4096

type returnStateAnalysis struct {
	ctx context.Context
	effects *nativeEffectFacts
	noReturns *noReturnAnalysis
	summaries map[*types.Func]map[returnStateKey]rules.ReturnStateSummary
	resultStates map[*types.Func]map[int]rules.NilState
	definitions []returnStateDefinition
	definitionsByFunction map[*types.Func]*returnStateDefinition
	summariesBuilt map[*types.Func]bool
	summariesStarted map[*types.Func]bool
	resultStatesBuilt map[*types.Func]bool
	resultStatesStarted map[*types.Func]bool
	summaryBuildDepth int
	resultStateBuildDepth int
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
		summariesBuilt: make(map[*types.Func]bool),
		summariesStarted: make(map[*types.Func]bool),
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
			if !returnStateSourceIsBuildInvariant(pkg, file) {
				continue
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

func returnStateSourceIsBuildInvariant(pkg *packages.Package, file *ast.File) bool {
	if pkg == nil || pkg.Fset == nil || file == nil {
		return false
	}
	for _, group := range file.Comments {
		if group == nil || group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			if comment != nil &&
				(constraint.IsGoBuild(comment.Text) ||
					constraint.IsPlusBuild(comment.Text)) {
				return false
			}
		}
	}
	position := pkg.Fset.PositionFor(file.Package, false)
	return position.Filename != "" &&
		!goFileNameHasPlatformSuffix(filepath.Base(position.Filename))
}

func goFileNameHasPlatformSuffix(name string) bool {
	if !strings.HasSuffix(name, ".go") {
		return false
	}
	name, _, _ = strings.Cut(name, ".")
	if !strings.Contains(name, "_") {
		return false
	}
	parts := strings.Split(name, "_")
	if parts[len(parts) - 1] == "test" {
		parts = parts[:len(parts) - 1]
	}
	if len(parts) < 2 {
		return false
	}
	suffix := parts[len(parts) - 1]
	return knownBuildOS[suffix] || knownBuildArch[suffix]
}

var knownBuildOS = map[string]bool{
	"aix": true,
	"android": true,
	"darwin": true,
	"dragonfly": true,
	"freebsd": true,
	"hurd": true,
	"illumos": true,
	"ios": true,
	"js": true,
	"linux": true,
	"nacl": true,
	"netbsd": true,
	"openbsd": true,
	"plan9": true,
	"solaris": true,
	"wasip1": true,
	"windows": true,
	"zos": true,
}

var knownBuildArch = map[string]bool{
	"386": true,
	"amd64": true,
	"amd64p32": true,
	"arm": true,
	"armbe": true,
	"arm64": true,
	"arm64be": true,
	"loong64": true,
	"mips": true,
	"mipsle": true,
	"mips64": true,
	"mips64le": true,
	"mips64p32": true,
	"mips64p32le": true,
	"ppc": true,
	"ppc64": true,
	"ppc64le": true,
	"riscv": true,
	"riscv64": true,
	"s390": true,
	"s390x": true,
	"sparc": true,
	"sparc64": true,
	"wasm": true,
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
		a.buildDefinitionReturnStates(definition)
		a.buildDefinitionResultStates(definition)
	}
}

func (a *returnStateAnalysis) buildDefinitionReturnStates(
	definition *returnStateDefinition,
) map[returnStateKey]rules.ReturnStateSummary {
	if a == nil ||
		definition == nil ||
		definition.function == nil ||
		a.ctx == nil ||
		a.ctx.Err() != nil ||
		a.summaryBuildDepth >= maxReturnStateBuildDepth {
		return nil
	}
	if a.summariesBuilt[definition.function] {
		return a.summaries[definition.function]
	}
	if a.noReturns != nil && a.noReturns.noReturn(definition.function) {
		a.summariesBuilt[definition.function] = true
		return nil
	}
	if a.summariesStarted[definition.function] {
		return nil
	}
	a.summariesStarted[definition.function] = true
	a.summaryBuildDepth++
	defer func() {
		a.summaryBuildDepth--
		delete(a.summariesStarted, definition.function)
	}()
	summaries := a.summarizeReturnStates(
		definition.declaration,
		definition.signature,
		definition.info,
	)
	if len(summaries) != 0 {
		a.summaries[definition.function] = summaries
	}
	a.summariesBuilt[definition.function] = true
	return summaries
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
		a.resultStateBuildDepth >= maxReturnStateBuildDepth {
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
	a.resultStateBuildDepth++
	defer func() {
		a.resultStateBuildDepth--
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
	if a == nil || info == nil {
		return false
	}
	return functionHasDeferredNoReturn(
		body,
		func(call *ast.CallExpr, depth int) bool {
			return a.resultStateCallMayReturn(call, info, depth)
		},
	)
}

func functionHasDeferredNoReturn(
	body *ast.BlockStmt,
	callMayReturn func(*ast.CallExpr, int) bool,
) bool {
	if body == nil || callMayReturn == nil {
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
			found = !callMayReturn(deferred.Call, 0)
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
	if a == nil {
		return true
	}
	return resultFactCallMayReturn(call, info, a.effects, a.noReturns, depth)
}

func resultFactCallMayReturn(
	call *ast.CallExpr,
	info *types.Info,
	effects *nativeEffectFacts,
	noReturns *noReturnAnalysis,
	depth int,
) bool {
	if call == nil || info == nil || depth >= maxReturnStateBuildDepth {
		return true
	}
	if literal, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		graph := cfg.New(
			literal.Body,
			func(nested *ast.CallExpr) bool {
				return resultFactCallMayReturn(
					nested,
					info,
					effects,
					noReturns,
					depth + 1,
				)
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
	if noReturns != nil {
		return !noReturns.noReturn(function)
	}
	return !isAuthoritativeNoReturn(function) && (effects == nil || !effects.noReturn(function))
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

func (a *returnStateAnalysis) summarizeReturnStates(
	function *ast.FuncDecl,
	signature *types.Signature,
	info *types.Info,
) map[returnStateKey]rules.ReturnStateSummary {
	results := signature.Results()
	if function == nil || function.Body == nil || results == nil || info == nil {
		return nil
	}
	if a.hasDeferredNoReturn(function.Body, info) {
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
			whenNil := a.aggregateReturnState(
				returns,
				results.Len(),
				valueIndex,
				errorIndex,
				results.At(valueIndex).Type(),
				results.At(errorIndex).Type(),
				true,
				info,
			)
			whenNonNil := a.aggregateReturnState(
				returns,
				results.Len(),
				valueIndex,
				errorIndex,
				results.At(valueIndex).Type(),
				results.At(errorIndex).Type(),
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

func (a *returnStateAnalysis) aggregateReturnState(
	returns []*ast.ReturnStmt,
	resultCount int,
	valueIndex int,
	errorIndex int,
	valueType types.Type,
	errorType types.Type,
	errorIsNil bool,
	info *types.Info,
) rules.NilState {
	state := rules.NilStateUnknown
	found := false
	for _, returned := range returns {
		if returned == nil || len(returned.Results) == 0 {
			return rules.NilStateUnknown
		}
		valueState := rules.NilStateUnknown
		switch {
		case len(returned.Results) == resultCount &&
			len(returned.Results) > valueIndex &&
			len(returned.Results) > errorIndex:
			errorState := classifyErrorExpression(returned.Results[errorIndex], info)
			if errorState == rules.NilStateUnknown {
				return rules.NilStateUnknown
			}
			if (errorState == rules.NilStateNil) != errorIsNil {
				continue
			}
			valueState = classifyNilExpression(returned.Results[valueIndex], info)
		case len(returned.Results) == 1:
			summary := a.delegatedReturnState(
				returned.Results[0],
				resultCount,
				valueIndex,
				errorIndex,
				valueType,
				errorType,
				info,
			)
			if errorIsNil {
				valueState = summary.WhenErrorNil
			} else {
				valueState = summary.WhenErrorNonNil
			}
		default:
			return rules.NilStateUnknown
		}
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

func (a *returnStateAnalysis) delegatedReturnState(
	expression ast.Expr,
	wantResults int,
	valueIndex int,
	errorIndex int,
	wantValueType types.Type,
	wantErrorType types.Type,
	info *types.Info,
) rules.ReturnStateSummary {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil ||
		info == nil ||
		wantResults <= 0 ||
		valueIndex < 0 ||
		errorIndex < 0 ||
		wantValueType == nil ||
		wantErrorType == nil {
		return rules.ReturnStateSummary{}
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil {
		return rules.ReturnStateSummary{}
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil ||
		signature.Results() == nil ||
		signature.Results().Len() != wantResults ||
		valueIndex >= signature.Results().Len() ||
		errorIndex >= signature.Results().Len() ||
		!types.Identical(signature.Results().At(valueIndex).Type(), wantValueType) ||
		!types.Identical(signature.Results().At(errorIndex).Type(), wantErrorType) {
		return rules.ReturnStateSummary{}
	}
	if definition := a.definitionsByFunction[function]; definition != nil {
		return a.buildDefinitionReturnStates(
			definition,
		)[returnStateKey{value: valueIndex, error: errorIndex}]
	}
	if a.effects == nil {
		return rules.ReturnStateSummary{}
	}
	return a.effects.ReturnState(function, valueIndex, errorIndex)
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
