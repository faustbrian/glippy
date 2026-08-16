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

type parameterEffectDefinition struct {
	function ast.Node
	body *ast.BlockStmt
	info *types.Info
	signature *types.Signature
	summaries map[int]rules.ParameterEffectSummary
	started map[int]bool
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

func (a *parameterEffectAnalysis) buildAll() {
	for _, definition := range a.ordered {
		if a.ctx.Err() != nil {
			return
		}
		if definition == nil ||
			definition.signature == nil ||
			definition.signature.Params() == nil {
			continue
		}
		for index := range definition.signature.Params().Len() {
			a.build(definition, index)
		}
	}
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

	var graph *cfg.CFG
	if a.noReturns != nil {
		graph = a.noReturns.graphFor(definition.function, definition.body, definition.info)
	} else {
		graph = cfg.New(
			definition.body,
			func(*ast.CallExpr) bool {
				return true
			},
		)
	}
	summary := a.summarizeParameter(
		graph,
		definition.info,
		definition.signature.Params().At(index),
	)
	definition.summaries[index] = summary
	return summary
}

type parameterEffectWork struct {
	block *cfg.Block
	offset int
}

func (a *parameterEffectAnalysis) summarizeParameter(
	graph *cfg.CFG,
	info *types.Info,
	parameter *types.Var,
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
			effect, ambiguous := a.parameterNodeEffect(info, node, parameter)
			if ambiguous {
				known = false
				pathTerminated = true
				break
			}
			if effect != 0 {
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
	}
}

func (a *parameterEffectAnalysis) parameterNodeEffect(
	info *types.Info,
	node ast.Node,
	parameter types.Object,
) (rules.ParameterEffectKind, bool) {
	if asynchronous, ok := node.(*ast.GoStmt);
		ok && expressionUsesObjectForEffects(info, asynchronous.Call, parameter) {
		return rules.ParameterEffectTransfer, false
	}
	var effect rules.ParameterEffectKind
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
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				if directEffectObject(info, current.Fun) == parameter {
					effect = rules.ParameterEffectCancelInvoke
					return false
				}
				if completion := directParameterCompletion(
					info,
					current,
					parameter,
				);
					completion != 0 {
					effect = completion
					return false
				}
				callee := typeutil.StaticCallee(info, current)
				for index, argument := range current.Args {
					if directEffectObject(info, argument) != parameter {
						continue
					}
					if callee == nil {
						ambiguous = true
						return false
					}
					parameterIndex := parameterEffectArgumentIndex(
						callee,
						index,
					)
					summary := a.summary(callee, parameterIndex)
					if !summary.Known {
						ambiguous = true
						return false
					}
					if summary.Always {
						effect |= summary.Kinds
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directEffectObject(info, expression) == parameter {
						effect = rules.ParameterEffectTransfer
						return false
					}
				}
			case *ast.SendStmt:
				if directEffectObject(info, current.Value) == parameter {
					effect = rules.ParameterEffectTransfer
					return false
				}
			case *ast.AssignStmt:
				for _, expression := range current.Rhs {
					if directEffectObject(info, expression) != parameter {
						continue
					}
					for _, target := range current.Lhs {
						identifier, named := ast.Unparen(
							target,
						).(*ast.Ident)
						if named && identifier.Name == "_" {
							continue
						}
						object := directEffectObject(info, target)
						variable, local := object.(*types.Var)
						if local &&
							variable.Pkg() != nil &&
							variable.Parent() ==
								variable.Pkg().Scope() {
							effect = rules.ParameterEffectTransfer
							return false
						}
						if !local {
							effect = rules.ParameterEffectTransfer
							return false
						}
						ambiguous = true
						return false
					}
				}
			case *ast.CompositeLit:
				if expressionUsesObjectForEffects(info, current, parameter) {
					effect = rules.ParameterEffectTransfer
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
			}
			return true
		},
	)
	return effect, ambiguous
}

func parameterEffectArgumentIndex(function *types.Func, argument int) int {
	if function == nil || argument < 0 {
		return -1
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Params() == nil {
		return -1
	}
	if signature.Variadic() && argument >= signature.Params().Len() - 1 {
		argument = signature.Params().Len() - 1
	}
	if argument >= signature.Params().Len() {
		return -1
	}
	return argument
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
