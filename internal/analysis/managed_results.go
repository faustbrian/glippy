package analysis

import (
	"context"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const maxManagedResultBuildDepth = 4096

type managedResultDefinition struct {
	function *types.Func
	declaration *ast.FuncDecl
	signature *types.Signature
	info *types.Info
}

type managedResultAnalysis struct {
	ctx context.Context
	definitions []managedResultDefinition
	definitionsByFunction map[*types.Func]*managedResultDefinition
	effects *nativeEffectFacts
	noReturns *noReturnAnalysis
	summaries map[*types.Func]map[int]struct{}
	summariesBuilt map[*types.Func]bool
	summariesStarted map[*types.Func]bool
	buildDepth int
}

func newManagedResultAnalysis(
	ctx context.Context,
	packages_ []*packages.Package,
	effects *nativeEffectFacts,
	noReturns *noReturnAnalysis,
) *managedResultAnalysis {
	analysis := &managedResultAnalysis{
		ctx: ctx,
		definitions: make([]managedResultDefinition, 0),
		definitionsByFunction: make(map[*types.Func]*managedResultDefinition),
		effects: effects,
		noReturns: noReturns,
		summaries: make(map[*types.Func]map[int]struct{}),
		summariesBuilt: make(map[*types.Func]bool),
		summariesStarted: make(map[*types.Func]bool),
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
				if signature == nil || signature.Results() == nil {
					continue
				}
				analysis.definitions = append(
					analysis.definitions,
					managedResultDefinition{
						function: object,
						declaration: function,
						signature: signature,
						info: pkg.TypesInfo,
					},
				)
			}
		}
	}
	for index := range analysis.definitions {
		definition := &analysis.definitions[index]
		analysis.definitionsByFunction[definition.function] = definition
	}
	return analysis
}

func (a *managedResultAnalysis) buildAll() {
	if a == nil || a.ctx == nil {
		return
	}
	for index := range a.definitions {
		if a.ctx.Err() != nil {
			return
		}
		a.buildDefinition(&a.definitions[index])
	}
}

func (a *managedResultAnalysis) buildDefinition(
	definition *managedResultDefinition,
) map[int]struct{} {
	if a == nil ||
		definition == nil ||
		definition.function == nil ||
		a.ctx == nil ||
		a.ctx.Err() != nil ||
		a.buildDepth >= maxManagedResultBuildDepth {
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
	a.buildDepth++
	defer func() {
		a.buildDepth--
		delete(a.summariesStarted, definition.function)
	}()
	summaries := a.summarizeDefinition(definition)
	if len(summaries) != 0 {
		a.summaries[definition.function] = summaries
	}
	a.summariesBuilt[definition.function] = true
	return summaries
}

func (a *managedResultAnalysis) summarizeDefinition(
	definition *managedResultDefinition,
) map[int]struct{} {
	if definition == nil ||
		definition.declaration == nil ||
		definition.declaration.Body == nil ||
		definition.signature == nil ||
		definition.info == nil ||
		definition.function == nil ||
		functionHasDeferredNoReturn(
			definition.declaration.Body,
			func(call *ast.CallExpr, depth int) bool {
				return resultFactCallMayReturn(
					call,
					definition.info,
					a.effects,
					a.noReturns,
					depth,
				)
			},
		) {
		return nil
	}
	graph := a.graphFor(definition.declaration, definition.declaration.Body, definition.info)
	summaries := make(map[int]struct{})
	for result := range definition.signature.Results().Len() {
		object, found := stableReturnedLocalObject(
			graph,
			definition.declaration.Body,
			definition.info,
			result,
			definition.signature.Results().Len(),
		)
		if found &&
			!localObjectIsReassigned(
				definition.declaration.Body,
				definition.info,
				object,
			) &&
			!a.localObjectIsAliasedOrEscaped(
				definition.declaration.Body,
				definition.info,
				object,
			) &&
			a.cleanupRegisteredBeforeEveryReturn(graph, definition.info, object) {
			summaries[result] = struct{}{}
			continue
		}
		if a.delegatedResultIsManaged(
			definition.declaration.Body,
			definition.signature,
			definition.info,
			result,
		) {
			summaries[result] = struct{}{}
		}
	}
	return summaries
}

func (a *managedResultAnalysis) delegatedResultIsManaged(
	body *ast.BlockStmt,
	signature *types.Signature,
	info *types.Info,
	result int,
) bool {
	if body == nil || signature == nil || signature.Results() == nil || info == nil {
		return false
	}
	results := signature.Results()
	if result < 0 || result >= results.Len() {
		return false
	}
	returns := explicitFunctionReturns(body)
	if len(returns) == 0 {
		return false
	}
	for _, returned := range returns {
		if returned == nil || len(returned.Results) == 0 {
			return false
		}
		expression := ast.Expr(nil)
		calleeResult := 0
		calleeResults := 1
		switch {
		case len(returned.Results) == results.Len():
			expression = returned.Results[result]
		case len(returned.Results) == 1:
			expression = returned.Results[0]
			calleeResult = result
			calleeResults = results.Len()
		default:
			return false
		}
		if !a.delegatedExpressionIsManaged(
			expression,
			calleeResult,
			calleeResults,
			results.At(result).Type(),
			info,
		) {
			return false
		}
	}
	return true
}

func (a *managedResultAnalysis) delegatedExpressionIsManaged(
	expression ast.Expr,
	result int,
	wantResults int,
	wantType types.Type,
	info *types.Info,
) bool {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil || info == nil || result < 0 || wantResults <= 0 || wantType == nil {
		return false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil {
		return false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil ||
		signature.Results() == nil ||
		signature.Results().Len() != wantResults ||
		result >= signature.Results().Len() ||
		!types.Identical(signature.Results().At(result).Type(), wantType) {
		return false
	}
	if definition := a.definitionsByFunction[function]; definition != nil {
		_, managed := a.buildDefinition(definition)[result]
		return managed
	}
	return a.effects != nil && a.effects.CleanupManagedResult(function, result)
}

func stableReturnedLocalObject(
	graph *cfg.CFG,
	body *ast.BlockStmt,
	info *types.Info,
	result int,
	resultCount int,
) (types.Object, bool) {
	if graph == nil || body == nil || info == nil || result < 0 || result >= resultCount {
		return nil, false
	}
	var returnedObject types.Object
	foundReturn := false
	for _, block := range graph.Blocks {
		if block == nil || !block.Live {
			continue
		}
		returned := block.Return()
		if returned == nil {
			continue
		}
		if len(returned.Results) != resultCount {
			return nil, false
		}
		object := directEffectObject(info, returned.Results[result])
		if object == nil || returnedObject != nil && returnedObject != object {
			return nil, false
		}
		returnedObject = object
		foundReturn = true
	}
	if !foundReturn ||
		returnedObject == nil ||
		!objectDefinedInBody(body, info, returnedObject) {
		return nil, false
	}
	_, local := returnedObject.(*types.Var)
	return returnedObject, local
}

func objectDefinedInBody(body *ast.BlockStmt, info *types.Info, object types.Object) bool {
	found := false
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if found {
				return false
			}
			if literal, nested := node.(*ast.FuncLit); nested && literal != nil {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if ok && info.Defs[identifier] == object {
				found = true
				return false
			}
			return true
		},
	)
	return found
}

func localObjectIsReassigned(body *ast.BlockStmt, info *types.Info, object types.Object) bool {
	reassigned := false
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if reassigned {
				return false
			}
			switch node := node.(type) {
			case *ast.AssignStmt:
				for _, target := range node.Lhs {
					identifier, _ := ast.Unparen(target).(*ast.Ident)
					if identifier != nil && info.Uses[identifier] == object {
						reassigned = true
						return false
					}
				}
			case *ast.IncDecStmt:
				if directEffectObject(info, node.X) == object {
					reassigned = true
					return false
				}
			case *ast.RangeStmt:
				if directEffectObject(info, node.Key) == object ||
					directEffectObject(info, node.Value) == object {
					reassigned = true
					return false
				}
			}
			return true
		},
	)
	return reassigned
}

func (a *managedResultAnalysis) localObjectIsAliasedOrEscaped(
	body *ast.BlockStmt,
	info *types.Info,
	object types.Object,
) bool {
	if body == nil || info == nil || object == nil {
		return true
	}
	cleanupCallbacks := a.cleanupCallbacksForObject(body, info, object)
	unstable := false
	ast.PreorderStack(
		body,
		nil,
		func(node ast.Node, stack []ast.Node) bool {
			if unstable {
				return false
			}
			switch node := node.(type) {
			case *ast.FuncLit:
				if expressionUsesObjectForEffects(info, node.Body, object) &&
					!cleanupCallbacks[node] {
					unstable = true
					return false
				}
			case *ast.AssignStmt:
				for index, value := range node.Rhs {
					if directEffectObject(info, value) == object &&
						!correspondingAssignmentTargetIsBlank(node, index) {
						unstable = true
						return false
					}
				}
			case *ast.ValueSpec:
				for index, value := range node.Values {
					if directEffectObject(info, value) == object &&
						(index >= len(node.Names) ||
							node.Names[index].Name != "_") {
						unstable = true
						return false
					}
				}
			case *ast.UnaryExpr:
				if node.Op == token.AND &&
					directEffectObject(info, node.X) == object {
					unstable = true
					return false
				}
			case *ast.SendStmt:
				if directEffectObject(info, node.Value) == object {
					unstable = true
					return false
				}
			case *ast.CompositeLit:
				if compositeLiteralUsesDirectObject(info, node, object) {
					unstable = true
					return false
				}
			case *ast.CallExpr:
				callback := enclosingFunctionLiteral(stack)
				for argument, expression := range node.Args {
					if directEffectObject(info, expression) != object {
						continue
					}
					if cleanupCallbacks[callback] &&
						(a.callArgumentGuaranteesClose(
							info,
							node,
							object,
							argument,
						) ||
							a.callReceiverGuaranteesClose(
								info,
								node,
								object,
							)) {
						continue
					}
					unstable = true
					return false
				}
			case *ast.SelectorExpr:
				if directEffectObject(info, node.X) == object &&
					!selectorIsDirectlyCalled(node, stack) {
					unstable = true
					return false
				}
			}
			return true
		},
	)
	return unstable
}

func (a *managedResultAnalysis) cleanupCallbacksForObject(
	body *ast.BlockStmt,
	info *types.Info,
	object types.Object,
) map[*ast.FuncLit]bool {
	result := make(map[*ast.FuncLit]bool)
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			call, _ := node.(*ast.CallExpr)
			if call == nil {
				return true
			}
			callback := testingCleanupCallback(info, call)
			if callback != nil && a.callbackGuaranteesClose(info, callback, object) {
				result[callback] = true
			}
			return true
		},
	)
	return result
}

func correspondingAssignmentTargetIsBlank(assignment *ast.AssignStmt, value int) bool {
	if assignment == nil ||
		len(assignment.Lhs) != len(assignment.Rhs) ||
		value < 0 ||
		value >= len(assignment.Lhs) {
		return false
	}
	identifier, _ := ast.Unparen(assignment.Lhs[value]).(*ast.Ident)
	return identifier != nil && identifier.Name == "_"
}

func compositeLiteralUsesDirectObject(
	info *types.Info,
	literal *ast.CompositeLit,
	object types.Object,
) bool {
	if literal == nil {
		return false
	}
	for _, element := range literal.Elts {
		keyed, _ := element.(*ast.KeyValueExpr)
		if keyed != nil {
			if directEffectObject(info, keyed.Key) == object ||
				directEffectObject(info, keyed.Value) == object {
				return true
			}
			continue
		}
		expression, _ := element.(ast.Expr)
		if directEffectObject(info, expression) == object {
			return true
		}
	}
	return false
}

func enclosingFunctionLiteral(stack []ast.Node) *ast.FuncLit {
	for index := len(stack) - 1; index >= 0; index-- {
		if literal, ok := stack[index].(*ast.FuncLit); ok {
			return literal
		}
	}
	return nil
}

func selectorIsDirectlyCalled(selector *ast.SelectorExpr, stack []ast.Node) bool {
	if selector == nil || len(stack) == 0 {
		return false
	}
	call, _ := stack[len(stack) - 1].(*ast.CallExpr)
	return call != nil && ast.Unparen(call.Fun) == selector
}

type cleanupManagedWork struct {
	block *cfg.Block
	managed bool
}

func (a *managedResultAnalysis) cleanupRegisteredBeforeEveryReturn(
	graph *cfg.CFG,
	info *types.Info,
	object types.Object,
) bool {
	if graph == nil || len(graph.Blocks) == 0 || info == nil || object == nil {
		return false
	}
	work := []cleanupManagedWork{{block: graph.Blocks[0]}}
	seen := make(map[cleanupManagedWork]bool)
	foundReturn := false
	for len(work) != 0 {
		if a.ctx.Err() != nil {
			return false
		}
		current := work[len(work) - 1]
		work = work[:len(work) - 1]
		if current.block == nil || !current.block.Live || seen[current] {
			continue
		}
		seen[current] = true
		managed := current.managed
		for _, node := range current.block.Nodes {
			if !managed && a.nodeRegistersCleanup(info, node, object) {
				managed = true
			}
		}
		if current.block.Return() != nil {
			foundReturn = true
			if !managed {
				return false
			}
			continue
		}
		for _, successor := range current.block.Succs {
			work = append(work, cleanupManagedWork{block: successor, managed: managed})
		}
	}
	return foundReturn
}

func (a *managedResultAnalysis) nodeRegistersCleanup(
	info *types.Info,
	node ast.Node,
	object types.Object,
) bool {
	registered := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if registered {
				return false
			}
			if _, asynchronous := current.(*ast.GoStmt); asynchronous {
				return false
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			call, ok := current.(*ast.CallExpr)
			if !ok {
				return true
			}
			callback := testingCleanupCallback(info, call)
			if callback != nil && a.callbackGuaranteesClose(info, callback, object) {
				registered = true
				return false
			}
			return true
		},
	)
	return registered
}

func testingCleanupCallback(info *types.Info, call *ast.CallExpr) *ast.FuncLit {
	if info == nil || call == nil || len(call.Args) != 1 {
		return nil
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "testing" ||
		function.Name() != "Cleanup" {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || !testingTType(info.TypeOf(selector.X)) {
		return nil
	}
	callback, _ := ast.Unparen(call.Args[0]).(*ast.FuncLit)
	return callback
}

func testingTType(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	type_ = types.Unalias(type_)
	pointer, ok := type_.(*types.Pointer)
	if !ok {
		return false
	}
	type_ = types.Unalias(pointer.Elem())
	named, _ := type_.(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "testing" &&
		named.Obj().Name() == "T"
}

func (a *managedResultAnalysis) callbackGuaranteesClose(
	info *types.Info,
	callback *ast.FuncLit,
	object types.Object,
) bool {
	if callback == nil || callback.Body == nil {
		return false
	}
	graph := cfg.New(callback.Body, a.callMayReturn(info))
	if graph == nil || len(graph.Blocks) == 0 {
		return false
	}
	work := []*cfg.Block{graph.Blocks[0]}
	seen := make(map[*cfg.Block]bool)
	closedPath := false
	for len(work) != 0 {
		if a.ctx.Err() != nil {
			return false
		}
		block := work[len(work) - 1]
		work = work[:len(work) - 1]
		if block == nil || !block.Live || seen[block] {
			continue
		}
		seen[block] = true
		closed := false
		for _, node := range block.Nodes {
			if a.nodeClosesObject(info, node, object) {
				closed = true
				closedPath = true
				break
			}
		}
		if closed {
			continue
		}
		if block.Return() != nil {
			return false
		}
		work = append(work, block.Succs...)
	}
	return closedPath
}

func (a *managedResultAnalysis) nodeClosesObject(
	info *types.Info,
	node ast.Node,
	object types.Object,
) bool {
	closed := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if closed {
				return false
			}
			if _, asynchronous := current.(*ast.GoStmt); asynchronous {
				return false
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			call, ok := current.(*ast.CallExpr)
			if !ok {
				return true
			}
			if directParameterCompletion(info, call, object) ==
				rules.ParameterEffectClose {
				closed = true
				return false
			}
			if a.callReceiverGuaranteesClose(info, call, object) {
				closed = true
				return false
			}
			callee := typeutil.StaticCallee(info, call)
			if callee == nil {
				return true
			}
			for argument, expression := range call.Args {
				if directEffectObject(info, expression) != object {
					continue
				}
				if a.callArgumentGuaranteesClose(info, call, object, argument) {
					closed = true
					return false
				}
			}
			return true
		},
	)
	return closed
}

func (a *managedResultAnalysis) callReceiverGuaranteesClose(
	info *types.Info,
	call *ast.CallExpr,
	object types.Object,
) bool {
	if info == nil || call == nil || object == nil || a.effects == nil {
		return false
	}
	callee := typeutil.StaticCallee(info, call)
	return callee != nil &&
		staticCallReceiverObject(info, call) == object &&
		a.effects.ReceiverEffect(callee).GuaranteesAny(rules.ParameterEffectClose)
}

func (a *managedResultAnalysis) callArgumentGuaranteesClose(
	info *types.Info,
	call *ast.CallExpr,
	object types.Object,
	argument int,
) bool {
	if info == nil ||
		call == nil ||
		object == nil ||
		argument < 0 ||
		argument >= len(call.Args) ||
		directEffectObject(info, call.Args[argument]) != object {
		return false
	}
	callee := typeutil.StaticCallee(info, call)
	if callee == nil {
		return false
	}
	parameter, valid := rules.StaticCallParameter(info, call, callee, argument)
	return valid &&
		a.effects != nil &&
		a.effects.ParameterEffect(callee, parameter).GuaranteesAny(
			rules.ParameterEffectClose,
		)
}

func (a *managedResultAnalysis) callMayReturn(info *types.Info) func(*ast.CallExpr) bool {
	if a.noReturns != nil {
		return a.noReturns.mayReturn(info)
	}
	return func(*ast.CallExpr) bool {
		return true
	}
}

func (a *managedResultAnalysis) graphFor(
	function ast.Node,
	body *ast.BlockStmt,
	info *types.Info,
) *cfg.CFG {
	if a.noReturns != nil {
		return a.noReturns.graphFor(function, body, info)
	}
	return cfg.New(body, a.callMayReturn(info))
}
