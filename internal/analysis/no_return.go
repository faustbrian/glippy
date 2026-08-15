package analysis

import (
	"context"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const maxNoReturnBuildDepth = 4096

type noReturnDefinition struct {
	body *ast.BlockStmt
	info *types.Info
	graph *cfg.CFG
	started bool
	noReturn bool
}

type noReturnAnalysis struct {
	ctx context.Context
	definitions map[*types.Func]*noReturnDefinition
	ordered []*noReturnDefinition
	panicObject types.Object
	buildDepth int
}

func newNoReturnAnalysis(ctx context.Context, packages_ []*packages.Package) *noReturnAnalysis {
	analysis := &noReturnAnalysis{
		ctx: ctx,
		definitions: make(map[*types.Func]*noReturnDefinition),
		ordered: make([]*noReturnDefinition, 0),
		panicObject: types.Universe.Lookup("panic"),
	}
	for _, pkg := range packages_ {
		if ctx.Err() != nil {
			break
		}
		if pkg == nil || pkg.TypesInfo == nil {
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
				definition := &noReturnDefinition{
					body: function.Body,
					info: pkg.TypesInfo,
				}
				analysis.definitions[object] = definition
				analysis.ordered = append(analysis.ordered, definition)
			}
		}
	}
	return analysis
}

func (a *noReturnAnalysis) buildAll() {
	for _, definition := range a.ordered {
		if a.ctx.Err() != nil {
			return
		}
		a.build(definition)
	}
}

func (a *noReturnAnalysis) graphFor(
	function ast.Node,
	body *ast.BlockStmt,
	info *types.Info,
) *cfg.CFG {
	if declaration, ok := function.(*ast.FuncDecl); ok && info != nil {
		object, _ := info.Defs[declaration.Name].(*types.Func)
		if definition := a.definitions[object]; definition != nil {
			a.build(definition)
			return definition.graph
		}
	}
	return cfg.New(body, a.mayReturn(info))
}

func (a *noReturnAnalysis) noReturn(function *types.Func) bool {
	if isAuthoritativeNoReturn(function) {
		return true
	}
	definition := a.definitions[function]
	if definition == nil {
		return false
	}
	a.build(definition)
	return definition.noReturn
}

func (a *noReturnAnalysis) predicate() func(*types.Func) bool {
	noReturns := make(map[*types.Func]bool)
	for function, definition := range a.definitions {
		if definition.noReturn {
			noReturns[function] = true
		}
	}
	return func(function *types.Func) bool {
		return noReturns[function] || isAuthoritativeNoReturn(function)
	}
}

func isAuthoritativeNoReturn(function *types.Func) bool {
	if function == nil || function.Pkg() == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	receiver := signature != nil && signature.Recv() != nil
	switch function.Pkg().Path() {
	case "os":
		return !receiver && function.Name() == "Exit"
	case "runtime":
		return !receiver && function.Name() == "Goexit"
	case "syscall":
		return !receiver && function.Name() == "Exit"
	case "log":
		switch function.Name() {
		case "Fatal", "Fatalf", "Fatalln", "Panic", "Panicf", "Panicln":
			return true
		}
	case "testing":
		if !receiver {
			return false
		}
		switch function.Name() {
		case "FailNow", "Fatal", "Fatalf", "SkipNow", "Skip", "Skipf":
			return true
		}
	}
	return false
}

func (a *noReturnAnalysis) build(definition *noReturnDefinition) {
	if definition == nil ||
		definition.graph != nil ||
		definition.started ||
		a.ctx.Err() != nil ||
		a.buildDepth >= maxNoReturnBuildDepth {
		return
	}
	definition.started = true
	a.buildDepth++
	defer func() {
		a.buildDepth--
	}()
	definition.graph = cfg.New(definition.body, a.mayReturn(definition.info))
	definition.noReturn = definition.graph.NoReturn()
}

func (a *noReturnAnalysis) mayReturn(info *types.Info) func(*ast.CallExpr) bool {
	return func(call *ast.CallExpr) bool {
		if isBuiltinPanicCall(info, call, a.panicObject) {
			return false
		}
		callee := typeutil.StaticCallee(info, call)
		if callee == nil {
			return true
		}
		return !a.noReturn(callee)
	}
}

func isBuiltinPanicCall(info *types.Info, call *ast.CallExpr, panicObject types.Object) bool {
	if info == nil || call == nil || panicObject == nil {
		return false
	}
	function := call.Fun
	for {
		parenthesized, ok := function.(*ast.ParenExpr)
		if !ok {
			break
		}
		function = parenthesized.X
	}
	identifier, ok := function.(*ast.Ident)
	return ok && info.Uses[identifier] == panicObject
}
