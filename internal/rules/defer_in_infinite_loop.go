package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type deferInInfiniteLoopRule struct{}

func (deferInInfiniteLoopRule) Metadata() Metadata {
	return Metadata{
		ID:               "defer-in-infinite-loop",
		Summary:          "detects defers that cannot run inside infinite loops",
		Documentation:    "A defer statement schedules work for function return or panic unwinding, not for the end of a loop iteration. This rule reports a defer in a conditionless for loop when the function control-flow graph cannot reach a return, built-in panic, or runtime.Goexit after that defer.",
		DefaultSeverity:  SeverityWarn,
		Presets:          []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement:      RequireControlFlow,
		Categories:       []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Calls to functions that panic, terminate the goroutine, or terminate the process are not treated as exits unless they are the predeclared panic function or runtime.Goexit.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{{
			Title: "Release per-iteration resources explicitly",
			Incorrect: `for {
	defer cleanup()
	work()
}`,
			Correct: `for {
	work()
	cleanup()
}`,
		}},
	}
}

func (deferInInfiniteLoopRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("defer-in-infinite-loop requires a complete control-flow context")
	}

	candidates := conditionlessLoopDefers(ctx.Body())
	if len(candidates) == 0 {
		return nil, nil
	}
	blocks := liveDeferBlocks(ctx.Graph())
	exitReachable := blocksReachingDeferExecution(ctx.Graph(), ctx.Info())
	findings := make([]Finding, 0, len(candidates))
	for _, statement := range candidates {
		block, live := blocks[statement]
		if !live || exitReachable[block] {
			continue
		}
		range_, err := ctx.PositionRange(statement.Defer, statement.Defer+token.Pos(len("defer")))
		if err != nil {
			return nil, err
		}
		findings = append(findings, Finding{
			MessageKey: "defer-never-runs",
			Message:    "defer in this infinite loop cannot reach function exit and will never run",
			Range:      range_,
			Help:       "invoke the cleanup explicitly in each iteration or make a function exit reachable",
		})
	}
	return findings, nil
}

func conditionlessLoopDefers(body *ast.BlockStmt) []*ast.DeferStmt {
	result := make([]*ast.DeferStmt, 0)
	conditionlessDepth := 0
	enteredLoops := make([]bool, 0)
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			last := len(enteredLoops) - 1
			if enteredLoops[last] {
				conditionlessDepth--
			}
			enteredLoops = enteredLoops[:last]
			return true
		}
		if _, nestedFunction := node.(*ast.FuncLit); nestedFunction {
			return false
		}
		loop, isLoop := node.(*ast.ForStmt)
		enteredLoop := isLoop && loop.Cond == nil
		enteredLoops = append(enteredLoops, enteredLoop)
		if enteredLoop {
			conditionlessDepth++
		}
		if statement, isDefer := node.(*ast.DeferStmt); isDefer && conditionlessDepth > 0 {
			result = append(result, statement)
		}
		return true
	})
	return result
}

func liveDeferBlocks(graph *cfg.CFG) map[*ast.DeferStmt]*cfg.Block {
	result := make(map[*ast.DeferStmt]*cfg.Block)
	for _, block := range graph.Blocks {
		if !block.Live {
			continue
		}
		for _, node := range block.Nodes {
			if statement, ok := node.(*ast.DeferStmt); ok {
				result[statement] = block
			}
		}
	}
	return result
}

func blocksReachingDeferExecution(graph *cfg.CFG, info *types.Info) map[*cfg.Block]bool {
	predecessors := make(map[*cfg.Block][]*cfg.Block, len(graph.Blocks))
	work := make([]*cfg.Block, 0)
	reachesExit := make(map[*cfg.Block]bool, len(graph.Blocks))
	for _, block := range graph.Blocks {
		if !block.Live {
			continue
		}
		for _, successor := range block.Succs {
			if successor.Live {
				predecessors[successor] = append(predecessors[successor], block)
			}
		}
		if block.Return() != nil || blockExecutesDefers(block, info) {
			reachesExit[block] = true
			work = append(work, block)
		}
	}
	for len(work) > 0 {
		block := work[len(work)-1]
		work = work[:len(work)-1]
		for _, predecessor := range predecessors[block] {
			if reachesExit[predecessor] {
				continue
			}
			reachesExit[predecessor] = true
			work = append(work, predecessor)
		}
	}
	return reachesExit
}

func blockExecutesDefers(block *cfg.Block, info *types.Info) bool {
	for _, node := range block.Nodes {
		statement, ok := node.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := statement.X.(*ast.CallExpr)
		if ok && callExecutesDefers(call, info) {
			return true
		}
	}
	return false
}

func callExecutesDefers(call *ast.CallExpr, info *types.Info) bool {
	function := ast.Unparen(call.Fun)
	var object types.Object
	switch function := function.(type) {
	case *ast.Ident:
		object = info.Uses[function]
	case *ast.SelectorExpr:
		object = info.Uses[function.Sel]
	default:
		return false
	}
	if object == types.Universe.Lookup("panic") {
		return true
	}
	functionObject, ok := object.(*types.Func)
	return ok && functionObject.Pkg() != nil && functionObject.Pkg().Path() == "runtime" &&
		functionObject.Name() == "Goexit"
}
