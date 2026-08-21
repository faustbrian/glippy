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

type parameterEffectDefinition struct {
	function ast.Node
	body *ast.BlockStmt
	info *types.Info
	signature *types.Signature
	closeMethod bool
	noOpClose bool
	summaries map[int]rules.ParameterEffectSummary
	started map[int]bool
	receiver rules.ParameterEffectSummary
	receiverBuilt bool
	receiverStarted bool
}

type parameterEffectAnalysis struct {
	ctx context.Context
	definitions map[*types.Func]*parameterEffectDefinition
	ordered []*parameterEffectDefinition
	effects *nativeEffectFacts
	noReturns *noReturnAnalysis
}

func newParameterEffectAnalysis(
	ctx context.Context,
	packages_ []*packages.Package,
	effects *nativeEffectFacts,
	noReturns *noReturnAnalysis,
) *parameterEffectAnalysis {
	analysis := &parameterEffectAnalysis{
		ctx: ctx,
		definitions: make(map[*types.Func]*parameterEffectDefinition),
		ordered: make([]*parameterEffectDefinition, 0),
		effects: effects,
		noReturns: noReturns,
	}
	for _, pkg := range packages_ {
		if ctx.Err() != nil {
			break
		}
		if pkg == nil || pkg.TypesInfo == nil || pkg.IllTyped {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, declaration := range file.Decls {
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
				definition := &parameterEffectDefinition{
					function: function,
					body: function.Body,
					info: pkg.TypesInfo,
					signature: signature,
					closeMethod: conventionalCloseMethod(object, signature),
					noOpClose: sourceProvesNoOpClose(function, pkg.TypesInfo),
					summaries: make(map[int]rules.ParameterEffectSummary),
					started: make(map[int]bool),
				}
				analysis.definitions[object] = definition
				analysis.ordered = append(analysis.ordered, definition)
			}
		}
	}
	return analysis
}

func conventionalCloseMethod(function *types.Func, signature *types.Signature) bool {
	if function == nil ||
		function.Name() != "Close" ||
		signature == nil ||
		signature.Recv() == nil ||
		signature.Params() != nil && signature.Params().Len() != 0 ||
		signature.Results() == nil ||
		signature.Results().Len() != 1 {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	return types.AssignableTo(signature.Results().At(0).Type(), errorType)
}

func sourceProvesNoOpClose(function *ast.FuncDecl, info *types.Info) bool {
	if function == nil ||
		function.Body == nil ||
		function.Recv == nil ||
		len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 ||
		info == nil ||
		len(function.Body.List) != 1 {
		return false
	}
	returned, _ := function.Body.List[0].(*ast.ReturnStmt)
	if returned == nil || len(returned.Results) != 1 {
		return false
	}
	receiver := info.ObjectOf(function.Recv.List[0].Names[0])
	return receiver != nil && receiverFieldExpression(info, returned.Results[0], receiver)
}

func receiverFieldExpression(info *types.Info, expression ast.Expr, receiver types.Object) bool {
	if info == nil || expression == nil || receiver == nil {
		return false
	}
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	selection := info.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return false
	}
	if identifier, direct := ast.Unparen(selector.X).(*ast.Ident); direct {
		return info.ObjectOf(identifier) == receiver
	}
	return receiverFieldExpression(info, selector.X, receiver)
}

func (a *parameterEffectAnalysis) buildAll() {
	for _, definition := range a.ordered {
		if a.ctx.Err() != nil {
			return
		}
		if definition == nil || definition.signature == nil {
			continue
		}
		if definition.signature.Recv() != nil {
			a.buildReceiver(definition)
		}
		if definition.signature.Params() == nil {
			continue
		}
		for index := range definition.signature.Params().Len() {
			a.build(definition, index)
		}
	}
}

func (a *parameterEffectAnalysis) receiverSummary(
	function *types.Func,
) rules.ParameterEffectSummary {
	if a == nil || function == nil {
		return rules.ParameterEffectSummary{}
	}
	if definition := a.definitions[function]; definition != nil {
		return a.buildReceiver(definition)
	}
	return a.effects.ReceiverEffect(function)
}

func (a *parameterEffectAnalysis) buildReceiver(
	definition *parameterEffectDefinition,
) rules.ParameterEffectSummary {
	if definition == nil ||
		definition.signature == nil ||
		definition.signature.Recv() == nil ||
		a.ctx.Err() != nil {
		return rules.ParameterEffectSummary{}
	}
	if definition.receiverBuilt {
		return definition.receiver
	}
	if definition.receiverStarted {
		return rules.ParameterEffectSummary{}
	}
	definition.receiverStarted = true
	defer func() {
		definition.receiverStarted = false
	}()

	definition.receiver = a.summarizeParameter(
		a.graphFor(definition),
		definition.info,
		definition.signature.Recv(),
		true,
	)
	definition.receiverBuilt = true
	return definition.receiver
}

func (a *parameterEffectAnalysis) summary(
	function *types.Func,
	index int,
) rules.ParameterEffectSummary {
	if a == nil || function == nil || index < 0 {
		return rules.ParameterEffectSummary{}
	}
	if definition := a.definitions[function]; definition != nil {
		return a.build(definition, index)
	}
	return a.effects.ParameterEffect(function, index)
}

func (a *parameterEffectAnalysis) build(
	definition *parameterEffectDefinition,
	index int,
) rules.ParameterEffectSummary {
	if definition == nil ||
		definition.signature == nil ||
		definition.signature.Params() == nil ||
		index < 0 ||
		index >= definition.signature.Params().Len() ||
		a.ctx.Err() != nil {
		return rules.ParameterEffectSummary{}
	}
	if summary, built := definition.summaries[index]; built {
		return summary
	}
	if definition.started[index] {
		return rules.ParameterEffectSummary{}
	}
	definition.started[index] = true
	defer delete(definition.started, index)

	summary := a.summarizeParameter(
		a.graphFor(definition),
		definition.info,
		definition.signature.Params().At(index),
		false,
	)
	definition.summaries[index] = summary
	return summary
}

func (a *parameterEffectAnalysis) graphFor(definition *parameterEffectDefinition) *cfg.CFG {
	if definition == nil || definition.body == nil || definition.info == nil {
		return nil
	}
	if a.noReturns != nil {
		return a.noReturns.graphFor(definition.function, definition.body, definition.info)
	}
	return cfg.New(
		definition.body,
		func(*ast.CallExpr) bool {
			return true
		},
	)
}

type parameterEffectWork struct {
	block *cfg.Block
	offset int
}

func (a *parameterEffectAnalysis) summarizeParameter(
	graph *cfg.CFG,
	info *types.Info,
	parameter *types.Var,
	receiver bool,
) rules.ParameterEffectSummary {
	if graph == nil || len(graph.Blocks) == 0 || info == nil || parameter == nil {
		return rules.ParameterEffectSummary{}
	}
	work := []parameterEffectWork{{block: graph.Blocks[0]}}
	seen := make(map[parameterEffectWork]bool)
	known := true
	openReturn := false
	terminal := false
	var kinds rules.ParameterEffectKind
	var guaranteedKinds rules.ParameterEffectKind
	for len(work) > 0 {
		if a.ctx.Err() != nil {
			return rules.ParameterEffectSummary{}
		}
		current := work[len(work) - 1]
		work = work[:len(work) - 1]
		if current.block == nil || !current.block.Live || seen[current] {
			continue
		}
		seen[current] = true
		pathTerminated := false
		for _, node := range current.block.Nodes[current.offset:] {
			effect, guaranteed, ambiguous := a.parameterNodeEffect(
				info,
				node,
				parameter,
				receiver,
			)
			if ambiguous {
				known = false
				pathTerminated = true
				break
			}
			if effect != 0 {
				if !terminal {
					guaranteedKinds = guaranteed
				} else {
					guaranteedKinds &= guaranteed
				}
				terminal = true
				kinds |= effect
				pathTerminated = true
				break
			}
		}
		if pathTerminated {
			continue
		}
		if current.block.Return() != nil {
			openReturn = true
			continue
		}
		for _, successor := range current.block.Succs {
			work = append(work, parameterEffectWork{block: successor})
		}
	}
	if !known {
		return rules.ParameterEffectSummary{}
	}
	return rules.ParameterEffectSummary{
		Known: true,
		Always: terminal && !openReturn,
		Kinds: kinds,
		GuaranteedKinds: func() rules.ParameterEffectKind {
			if openReturn {
				return 0
			}
			return guaranteedKinds
		}(),
	}
}

func (a *parameterEffectAnalysis) parameterNodeEffect(
	info *types.Info,
	node ast.Node,
	parameter types.Object,
	receiver bool,
) (rules.ParameterEffectKind, rules.ParameterEffectKind, bool) {
	if asynchronous, ok := node.(*ast.GoStmt);
		ok && expressionUsesObjectForEffects(info, asynchronous.Call, parameter) {
		return rules.ParameterEffectTransfer, rules.ParameterEffectTransfer, false
	}
	var effect rules.ParameterEffectKind
	var guaranteed rules.ParameterEffectKind
	ambiguous := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if effect != 0 || ambiguous {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				if expressionUsesObjectForEffects(info, literal.Body, parameter) {
					effect = rules.ParameterEffectTransfer
					guaranteed = effect
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				if directEffectObject(info, current.Fun) == parameter {
					effect = rules.ParameterEffectCancelInvoke
					guaranteed = effect
					return false
				}
				if completion := directParameterCompletion(
					info,
					current,
					parameter,
				);
					completion != 0 {
					effect = completion
					guaranteed = effect
					return false
				}
				callee := typeutil.StaticCallee(info, current)
				receiverObject, receiverArgument := staticCallReceiverArgument(
					info,
					current,
				)
				if receiverObject == parameter {
					if callee == nil {
						if receiver {
							ambiguous = true
							return false
						}
						return true
					}
					summary := a.receiverSummary(callee)
					if !summary.Known {
						if receiver {
							ambiguous = true
							return false
						}
						return true
					}
					if summary.Always {
						effect |= summary.Kinds
						guaranteed |= summary.GuaranteedKinds
					}
				}
				for index, argument := range current.Args {
					if directEffectObject(info, argument) != parameter {
						continue
					}
					if receiverObject == parameter &&
						receiverArgument == index {
						continue
					}
					if callee == nil {
						ambiguous = true
						return false
					}
					parameterIndex, valid := rules.StaticCallParameter(
						info,
						current,
						callee,
						index,
					)
					if !valid {
						ambiguous = true
						return false
					}
					summary := a.summary(callee, parameterIndex)
					if !summary.Known {
						ambiguous = true
						return false
					}
					if summary.Always {
						effect |= summary.Kinds
						guaranteed |= summary.GuaranteedKinds
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directEffectObject(info, expression) == parameter {
						effect = rules.ParameterEffectTransfer
						guaranteed = effect
						return false
					}
				}
			case *ast.SendStmt:
				if directEffectObject(info, current.Value) == parameter {
					effect = rules.ParameterEffectTransfer
					guaranteed = effect
					return false
				}
			case *ast.AssignStmt:
				if receiver {
					for _, target := range current.Lhs {
						if directEffectObject(info, target) == parameter {
							ambiguous = true
							return false
						}
					}
				}
				for index, expression := range current.Rhs {
					if directEffectObject(info, expression) != parameter {
						continue
					}
					if len(current.Rhs) != len(current.Lhs) {
						ambiguous = true
						return false
					}
					target := current.Lhs[index]
					identifier, named := ast.Unparen(target).(*ast.Ident)
					if named && identifier.Name == "_" {
						continue
					}
					object := directEffectObject(info, target)
					if object == parameter {
						continue
					}
					variable, local := object.(*types.Var)
					if local &&
						variable.Pkg() != nil &&
						variable.Parent() == variable.Pkg().Scope() {
						effect = rules.ParameterEffectTransfer
						guaranteed = effect
						return false
					}
					if !local {
						effect = rules.ParameterEffectTransfer
						guaranteed = effect
						return false
					}
					ambiguous = true
					return false
				}
			case *ast.CompositeLit:
				if expressionUsesObjectForEffects(info, current, parameter) {
					effect = rules.ParameterEffectTransfer
					guaranteed = effect
					return false
				}
			case *ast.IndexExpr:
				if directEffectObject(info, current.X) == parameter {
					ambiguous = true
					return false
				}
			case *ast.SliceExpr:
				if directEffectObject(info, current.X) == parameter {
					ambiguous = true
					return false
				}
			case *ast.RangeStmt:
				if directEffectObject(info, current.X) == parameter {
					ambiguous = true
					return false
				}
			case *ast.UnaryExpr:
				if receiver &&
					current.Op == token.AND &&
					directEffectObject(info, current.X) == parameter {
					ambiguous = true
					return false
				}
			}
			return true
		},
	)
	return effect, guaranteed, ambiguous
}

func staticCallReceiverObject(info *types.Info, call *ast.CallExpr) types.Object {
	object, _ := staticCallReceiverArgument(info, call)
	return object
}

func staticCallReceiverArgument(info *types.Info, call *ast.CallExpr) (types.Object, int) {
	if info == nil || call == nil {
		return nil, -1
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil, -1
	}
	selection := info.Selections[selector]
	if selection == nil || len(selection.Index()) != 1 {
		return nil, -1
	}
	switch selection.Kind() {
	case types.MethodVal:
		return directEffectObject(info, selector.X), -1
	case types.MethodExpr:
		if len(call.Args) != 0 {
			return directEffectObject(info, call.Args[0]), 0
		}
	}
	return nil, -1
}

func directParameterCompletion(
	info *types.Info,
	call *ast.CallExpr,
	parameter types.Object,
) rules.ParameterEffectKind {
	if info == nil || call == nil || len(call.Args) != 0 {
		return 0
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || directEffectObject(info, selector.X) != parameter {
		return 0
	}
	selection := info.Selections[selector]
	function, _ := selectionObjectForEffects(selection).(*types.Func)
	if function == nil {
		return 0
	}
	switch function.Name() {
	case "Close":
		return rules.ParameterEffectClose
	case "Commit", "Rollback":
		if function.Pkg() != nil && function.Pkg().Path() == "database/sql" {
			return rules.ParameterEffectTransactionComplete
		}
	}
	return 0
}

func directEffectObject(info *types.Info, expression ast.Expr) types.Object {
	if info == nil || expression == nil {
		return nil
	}
	switch expression := ast.Unparen(expression).(type) {
	case *ast.Ident:
		if object := info.Uses[expression]; object != nil {
			return object
		}
		return info.Defs[expression]
	case *ast.SelectorExpr:
		return info.ObjectOf(expression.Sel)
	default:
		return nil
	}
}

func selectionObjectForEffects(selection *types.Selection) types.Object {
	if selection == nil {
		return nil
	}
	return selection.Obj()
}

func expressionUsesObjectForEffects(info *types.Info, node ast.Node, object types.Object) bool {
	used := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			identifier, ok := current.(*ast.Ident)
			if ok && info.Uses[identifier] == object {
				used = true
				return false
			}
			return !used
		},
	)
	return used
}
