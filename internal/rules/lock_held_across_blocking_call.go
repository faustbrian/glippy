package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type lockHeldAcrossBlockingCallRule struct{}

type heldLock struct {
	object types.Object
	acquisition *ast.CallExpr
}

// NewLockHeldAcrossBlockingCallRule constructs the known-blocking-call lock
// rule for product registry composition.
func NewLockHeldAcrossBlockingCallRule() Rule {
	return lockHeldAcrossBlockingCallRule{}
}

func (lockHeldAcrossBlockingCallRule) Metadata() Metadata {
	return Metadata{
		ID: "lock-held-across-blocking-call",
		Summary: "detects known blocking calls made while a sync lock is held",
		Documentation: "Sleeping or waiting while holding a mutex or read lock can stall every competing goroutine and turn an ordinary external delay into lock contention or deadlock pressure. The rule recognizes sync lock methods and a deliberately small set of APIs whose contract is to wait.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBlockStmt},
		Categories: []Category{CategorySafety, CategorySuspicious, CategoryPerformance},
		KnownLimitations: []string{
			"The initial contract tracks direct identifier receivers through one lexical statement list; locks held across nested control-flow blocks require a later CFG expansion.",
			"Known blocking APIs are time.Sleep, sync.Cond.Wait, sync.WaitGroup.Wait, and os/exec.Cmd.Wait; arbitrary calls are not guessed to block.",
			"A blocking call may be deliberate coordination, so this rule remains opt-in suspicious and offers no automatic fix.",
		},
		Examples: []Example{
			{
				Title: "Release a lock before waiting",
				Incorrect: "mu.Lock()\ntime.Sleep(delay)\nmu.Unlock()",
				Correct: "mu.Lock()\nupdate()\nmu.Unlock()\ntime.Sleep(delay)",
			},
		},
	}
}

func (lockHeldAcrossBlockingCallRule) RunTypes(
	ctx *TypesContext,
	node ast.Node,
) ([]Finding, error) {
	block, ok := node.(*ast.BlockStmt)
	if !ok {
		return nil, fmt.Errorf("lock-held-across-blocking-call requires a block statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"lock-held-across-blocking-call requires complete type information",
		)
	}
	held := make([]heldLock, 0)
	findings := make([]Finding, 0)
	for _, statement := range block.List {
		call := directExpressionCall(statement)
		if call != nil {
			object, operation, syncLock := syncLockOperation(ctx.Info(), call)
			if syncLock {
				switch operation {
				case "Lock", "RLock":
					held = acquireLock(held, object, call)
				case "Unlock", "RUnlock":
					held = releaseLock(held, object)
				}
				continue
			}
		}
		if len(held) == 0 {
			continue
		}
		blocking := firstKnownBlockingCall(ctx.Info(), statement)
		if blocking == nil {
			continue
		}
		blockingRange, err := ctx.Range(blocking)
		if err != nil {
			return nil, err
		}
		for _, acquisition := range held {
			acquisitionRange, rangeErr := ctx.Range(acquisition.acquisition)
			if rangeErr != nil {
				return nil, rangeErr
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "lock-held-across-blocking-call",
					Message: "known blocking call executes while this lock is held",
					Range: blockingRange,
					Related: []Related{
						{
							Range: acquisitionRange,
							Message: "lock acquired here",
						},
					},
					Help: "release the lock before waiting or narrow the critical section",
				},
			)
			break
		}
	}
	return findings, nil
}

func acquireLock(held []heldLock, object types.Object, call *ast.CallExpr) []heldLock {
	for index := range held {
		if held[index].object == object {
			held[index].acquisition = call
			return held
		}
	}
	return append(held, heldLock{object: object, acquisition: call})
}

func releaseLock(held []heldLock, object types.Object) []heldLock {
	for index := range held {
		if held[index].object == object {
			return append(held[:index], held[index + 1:]...)
		}
	}
	return held
}

func directExpressionCall(statement ast.Stmt) *ast.CallExpr {
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return nil
	}
	call, _ := ast.Unparen(expression.X).(*ast.CallExpr)
	return call
}

func syncLockOperation(info *types.Info, call *ast.CallExpr) (types.Object, string, bool) {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil, "", false
	}
	receiver := directObject(info, selector.X)
	selection := info.Selections[selector]
	if receiver == nil || selection == nil {
		return nil, "", false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "sync" {
		return nil, "", false
	}
	receiverName := namedTypeName(selection.Recv())
	if receiverName != "Mutex" && receiverName != "RWMutex" {
		return nil, "", false
	}
	switch function.Name() {
	case "Lock", "Unlock":
		return receiver, function.Name(), true
	case "RLock", "RUnlock":
		return receiver, function.Name(), receiverName == "RWMutex"
	default:
		return nil, "", false
	}
}

func firstKnownBlockingCall(info *types.Info, node ast.Node) *ast.CallExpr {
	var result *ast.CallExpr
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if result != nil {
				return false
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			call, ok := current.(*ast.CallExpr)
			if ok && knownBlockingCall(info, call) {
				result = call
				return false
			}
			return true
		},
	)
	return result
}

func knownBlockingCall(info *types.Info, call *ast.CallExpr) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil {
		return false
	}
	if function.Pkg().Path() == "time" && function.Name() == "Sleep" {
		return true
	}
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	switch function.Pkg().Path() {
	case "sync":
		return function.Name() == "Wait" &&
			(namedTypeName(selection.Recv()) == "Cond" ||
				namedTypeName(selection.Recv()) == "WaitGroup")
	case "os/exec":
		return function.Name() == "Wait" && namedTypeName(selection.Recv()) == "Cmd"
	default:
		return false
	}
}

func namedTypeName(type_ types.Type) string {
	if pointer, ok := types.Unalias(type_).(*types.Pointer); ok {
		type_ = pointer.Elem()
	}
	named, _ := types.Unalias(type_).(*types.Named)
	if named == nil || named.Obj() == nil {
		return ""
	}
	return named.Obj().Name()
}
