package analysis

import (
	"context"
	"go/ast"
	"go/types"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

type writerBorrowDefinition struct {
	function *types.Func
	body *ast.BlockStmt
	info *types.Info
	signature *types.Signature
}

type writerBorrowAnalysis struct {
	ctx context.Context
	effects *nativeEffectFacts
	definitions map[*types.Func]*writerBorrowDefinition
	ordered []*writerBorrowDefinition
	borrows map[*types.Func]map[int]bool
	built map[*types.Func]map[int]bool
	started map[*types.Func]map[int]bool
}

func newWriterBorrowAnalysis(
	ctx context.Context,
	packages_ []*packages.Package,
	effects *nativeEffectFacts,
) *writerBorrowAnalysis {
	analysis := &writerBorrowAnalysis{
		ctx: ctx,
		effects: effects,
		definitions: make(map[*types.Func]*writerBorrowDefinition),
		ordered: make([]*writerBorrowDefinition, 0),
		borrows: make(map[*types.Func]map[int]bool),
		built: make(map[*types.Func]map[int]bool),
		started: make(map[*types.Func]map[int]bool),
	}
	for _, pkg := range packages_ {
		if ctx == nil || ctx.Err() != nil {
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
				signature, _ := types.Unalias(object.Type()).(*types.Signature)
				if signature == nil {
					continue
				}
				definition := &writerBorrowDefinition{
					function: object,
					body: function.Body,
					info: pkg.TypesInfo,
					signature: signature,
				}
				analysis.definitions[object] = definition
				analysis.ordered = append(analysis.ordered, definition)
			}
		}
	}
	return analysis
}

func (a *writerBorrowAnalysis) buildAll() {
	if a == nil || a.ctx == nil {
		return
	}
	for _, definition := range a.ordered {
		if a.ctx.Err() != nil {
			return
		}
		parameters := definition.signature.Params()
		if parameters == nil {
			continue
		}
		for index := range parameters.Len() {
			if exactIOWriter(parameters.At(index).Type()) {
				a.build(definition, index)
			}
		}
	}
}

func (a *writerBorrowAnalysis) build(definition *writerBorrowDefinition, index int) bool {
	if a == nil ||
		definition == nil ||
		definition.function == nil ||
		definition.body == nil ||
		definition.info == nil ||
		definition.signature == nil ||
		definition.signature.Params() == nil ||
		index < 0 ||
		index >= definition.signature.Params().Len() ||
		a.ctx == nil ||
		a.ctx.Err() != nil ||
		!exactIOWriter(definition.signature.Params().At(index).Type()) {
		return false
	}
	if a.built[definition.function][index] {
		return a.borrows[definition.function][index]
	}
	if a.started[definition.function][index] {
		return false
	}
	if a.started[definition.function] == nil {
		a.started[definition.function] = make(map[int]bool)
	}
	a.started[definition.function][index] = true
	defer delete(a.started[definition.function], index)
	parameter := definition.signature.Params().At(index)
	borrows := sourceProvesWriterBorrow(
		definition.body,
		definition.info,
		parameter,
		func(function *types.Func, parameter int) bool {
			if nested := a.definitions[function]; nested != nil {
				return a.build(nested, parameter)
			}
			return a.effects != nil && a.effects.WriterBorrow(function, parameter)
		},
	)
	if a.borrows[definition.function] == nil {
		a.borrows[definition.function] = make(map[int]bool)
	}
	if a.built[definition.function] == nil {
		a.built[definition.function] = make(map[int]bool)
	}
	a.borrows[definition.function][index] = borrows
	a.built[definition.function][index] = true
	return borrows
}

func sourceProvesWriterBorrow(
	body *ast.BlockStmt,
	info *types.Info,
	parameter types.Object,
	delegated func(*types.Func, int) bool,
) bool {
	if body == nil || info == nil || parameter == nil || delegated == nil {
		return false
	}
	if writerBorrowHasDeferredOrAsyncUse(body, info, parameter) {
		return false
	}
	safe := make(map[*ast.Ident]struct{})
	valid := true
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if !valid || node == nil {
				return false
			}
			if literal, nested := node.(*ast.FuncLit); nested {
				if expressionUsesObjectForEffects(info, literal.Body, parameter) {
					valid = false
				}
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := typeutil.StaticCallee(info, call)
			selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
			if selector != nil && directEffectObject(info, selector.X) == parameter {
				identifier, _ := ast.Unparen(selector.X).(*ast.Ident)
				selection := info.Selections[selector]
				method, _ := selectionObject(selection).(*types.Func)
				if identifier == nil || !exactIOWriterWrite(method) {
					valid = false
					return false
				}
				safe[identifier] = struct{}{}
			}
			for argument, expression := range call.Args {
				if directEffectObject(info, expression) != parameter {
					continue
				}
				identifier, _ := ast.Unparen(expression).(*ast.Ident)
				parameterIndex, mapped := staticWriterBorrowParameter(
					info,
					call,
					callee,
					argument,
				)
				if identifier == nil ||
					!mapped ||
					!authoritativeWriterBorrow(callee, parameterIndex) &&
						!delegated(callee, parameterIndex) {
					valid = false
					return false
				}
				safe[identifier] = struct{}{}
			}
			return valid
		},
	)
	if !valid {
		return false
	}
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if !valid || node == nil {
				return false
			}
			identifier, _ := node.(*ast.Ident)
			if identifier != nil && info.ObjectOf(identifier) == parameter {
				if _, allowed := safe[identifier]; !allowed {
					valid = false
					return false
				}
			}
			return true
		},
	)
	return valid
}

func writerBorrowHasDeferredOrAsyncUse(
	body *ast.BlockStmt,
	info *types.Info,
	parameter types.Object,
) bool {
	unsafe := false
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if unsafe || node == nil {
				return false
			}
			switch statement := node.(type) {
			case *ast.DeferStmt:
				unsafe = expressionUsesObjectForEffects(
					info,
					statement.Call,
					parameter,
				)
			case *ast.GoStmt:
				unsafe = expressionUsesObjectForEffects(
					info,
					statement.Call,
					parameter,
				)
			}
			return !unsafe
		},
	)
	return unsafe
}

func selectionObject(selection *types.Selection) types.Object {
	if selection == nil {
		return nil
	}
	return selection.Obj()
}

func exactIOWriter(type_ types.Type) bool {
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "io" &&
		named.Obj().Name() == "Writer"
}

func exactIOWriterWrite(function *types.Func) bool {
	if function == nil || function.Name() != "Write" {
		return false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	return signature != nil &&
		signature.Recv() != nil &&
		exactIOWriter(signature.Recv().Type()) &&
		signature.Params() != nil &&
		signature.Params().Len() == 1 &&
		types.Identical(
			signature.Params().At(0).Type(),
			types.NewSlice(types.Typ[types.Byte]),
		) &&
		signature.Results() != nil &&
		signature.Results().Len() == 2 &&
		types.Identical(signature.Results().At(0).Type(), types.Typ[types.Int]) &&
		types.Identical(
			signature.Results().At(1).Type(),
			types.Universe.Lookup("error").Type(),
		)
}

func staticWriterBorrowParameter(
	info *types.Info,
	call *ast.CallExpr,
	callee *types.Func,
	argument int,
) (int, bool) {
	if authoritativeWriterBorrow(callee, argument) {
		return argument, true
	}
	return rules.StaticCallParameter(info, call, callee, argument)
}

func authoritativeWriterBorrow(function *types.Func, parameter int) bool {
	if function == nil || function.Pkg() == nil || parameter != 0 {
		return false
	}
	switch function.Pkg().Path() {
	case "fmt":
		return function.Name() == "Fprint" ||
			function.Name() == "Fprintf" ||
			function.Name() == "Fprintln"
	case "io":
		return function.Name() == "WriteString"
	default:
		return false
	}
}
