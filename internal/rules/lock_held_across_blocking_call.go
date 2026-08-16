package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"github.com/faustbrian/glippy/internal/source"
)

const (
	maxTrackedFunctionLocks = 4096
)

type lockHeldAcrossBlockingCallRule struct{}

type lockNotReleasedRule struct{}

type unlockWithoutLockRule struct{}

type lockIssueKind uint8

const (
	lockIssueBlocking lockIssueKind = iota
	lockIssueNotReleased
	lockIssueInvalidUnlock
)

type lockKind uint8

const (
	mutexLock lockKind = iota
	rwMutexLock
)

type lockOperation uint8

const (
	lockOperationLock lockOperation = iota
	lockOperationReadLock
	lockOperationUnlock
	lockOperationReadUnlock
)

type lockStateSet uint16

const (
	lockStateUnknown lockStateSet = 1 << iota
	lockStateUnlocked
	lockStateWrite
	lockStateReadOne
	lockStateReadTwo
	lockStateReadThree
	lockStateReadFour
	lockStateReadFive
	lockStateReadSix
	lockStateReadSeven
	lockStateReadEight
)

const lockStateRead = lockStateReadOne |
	lockStateReadTwo |
	lockStateReadThree |
	lockStateReadFour |
	lockStateReadFive |
	lockStateReadSix |
	lockStateReadSeven |
	lockStateReadEight

const lockStateHeld = lockStateWrite | lockStateRead

type deferredLockOperation struct {
	operation lockOperation
	call *ast.CallExpr
	ambiguous bool
	present bool
}

type lockKey struct {
	base types.Object
	field *types.Var
	kind lockKind
	local bool
}

type lockValue struct {
	states lockStateSet
	deferred deferredLockOperation
}

type lockFlowState struct {
	values map[lockKey]lockValue
}

type lockIssue struct {
	kind lockIssueKind
	call *ast.CallExpr
	acquisition *ast.CallExpr
	position token.Pos
	message string
}

type lockStateAnalysis struct {
	complete bool
	issues []lockIssue
}

type lockAnalysisBuilder struct {
	ctx *ControlFlowContext
	keys []lockKey
	acquisitions map[lockKey][]*ast.CallExpr
	issues map[lockIssueIdentity]lockIssue
	record bool
}

type lockIssueIdentity struct {
	kind lockIssueKind
	position token.Pos
}

// NewLockHeldAcrossBlockingCallRule constructs the known-blocking-call lock
// rule for product registry composition.
func NewLockHeldAcrossBlockingCallRule() Rule {
	return lockHeldAcrossBlockingCallRule{}
}

// NewLockNotReleasedRule constructs the path-sensitive missing-release rule.
func NewLockNotReleasedRule() Rule {
	return lockNotReleasedRule{}
}

// NewUnlockWithoutLockRule constructs the path-sensitive invalid-unlock rule.
func NewUnlockWithoutLockRule() Rule {
	return unlockWithoutLockRule{}
}

func (lockHeldAcrossBlockingCallRule) Metadata() Metadata {
	return Metadata{
		ID: "lock-held-across-blocking-call",
		Summary: "detects known blocking calls made while a sync lock is held",
		Documentation: "Sleeping or waiting while holding a mutex or read lock can stall every competing goroutine and turn an ordinary external delay into lock contention or deadlock pressure. The rule propagates exact sync lock state through the shared control-flow graph and recognizes both a deliberately small standard-library set and configured blocking-function contracts.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategorySafety, CategorySuspicious, CategoryPerformance},
		KnownLimitations: []string{
			"Known standard-library blocking APIs are time.Sleep, sync.WaitGroup.Wait, and os/exec.Cmd.Wait; project-specific calls require an exact blocking contract.",
			"sync.Cond.Wait is excluded because its contract requires the associated Locker to be held and releases that Locker while waiting.",
			"Dynamic lock aliases, indexed receivers, helper-managed lock transitions, and ambiguous deferred-unlock stacks become unknown rather than producing speculative findings.",
			"Local read-lock depth is exact through eight acquisitions; deeper nesting becomes unknown.",
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

func (lockNotReleasedRule) Metadata() Metadata {
	return Metadata{
		ID: "lock-not-released",
		Summary: "detects sync locks left held on a normally returning path",
		Documentation: "Returning while a sync.Mutex or sync.RWMutex remains held can permanently block later users of shared state. The rule follows direct identifier and one-field receivers through branches and loops, applies an exact ordinary deferred unlock at normal exits, and reports only a path that remains locally held without an observed escape.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"Returning with a lock held can be an intentional cross-function handoff because Go locks are not goroutine-owned, so the rule remains suspicious rather than default correctness.",
			"Passing or capturing the receiver, assigning an alias, calling an unknown receiver method, or observing an ambiguous deferred-unlock stack makes the state unknown and suppresses a finding.",
			"Local read-lock depth is exact through eight acquisitions; deeper nesting becomes unknown instead of risking a saturation-based false positive.",
			"Indexed and multi-level selector receivers are excluded until a stable storage identity can be proven.",
		},
		Examples: []Example{
			{
				Title: "Release before every return",
				Incorrect: "mu.Lock()\nif failed { return }\nmu.Unlock()",
				Correct: "mu.Lock()\ndefer mu.Unlock()\nif failed { return }",
			},
		},
	}
}

func (unlockWithoutLockRule) Metadata() Metadata {
	return Metadata{
		ID: "unlock-without-lock",
		Summary: "detects path-proven unmatched or mismatched sync unlocks",
		Documentation: "sync.Mutex.Unlock, sync.RWMutex.Unlock, and sync.RWMutex.RUnlock terminate the process when the required lock mode is absent. The rule propagates finite lock states through branches and loops, distinguishes read and write modes, and evaluates an exact deferred unlock against every normal return state.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Parameters, package variables, and fields start unknown; an unmatched unlock reports only after a local zero-value declaration or an observed transition establishes an incompatible state.",
			"Passing or capturing the receiver, assigning an alias, calling an unknown receiver method, or observing multiple conditional deferred unlocks makes the state unknown.",
			"Local read-lock depth is exact through eight acquisitions; deeper nesting becomes unknown instead of risking a saturation-based false positive.",
			"The rule offers no fix because the intended acquisition, release placement, and lock mode require human judgment.",
		},
		Examples: []Example{
			{
				Title: "Do not execute a deferred read unlock after an early manual release",
				Incorrect: "mu.RLock()\ndefer mu.RUnlock()\nmu.RUnlock()\nreturn",
				Correct: "mu.RLock()\ndefer mu.RUnlock()\nreturn",
			},
		},
	}
}

func (lockHeldAcrossBlockingCallRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	return lockFindings(ctx, lockIssueBlocking)
}

func (lockNotReleasedRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	return lockFindings(ctx, lockIssueNotReleased)
}

func (unlockWithoutLockRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	return lockFindings(ctx, lockIssueInvalidUnlock)
}

func lockFindings(ctx *ControlFlowContext, wanted lockIssueKind) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"lock state analysis requires a complete control-flow context",
		)
	}
	analysis := lockAnalysisFor(ctx)
	if analysis == nil || !analysis.complete {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for _, issue := range analysis.issues {
		if issue.kind != wanted {
			continue
		}
		var range_ source.Range
		var err error
		if issue.kind == lockIssueNotReleased {
			range_, err = ctx.TokenRange(issue.position)
		} else if issue.call != nil {
			range_, err = ctx.Range(issue.call)
		} else {
			continue
		}
		if err != nil {
			return nil, err
		}
		finding := Finding{Range: range_}
		switch issue.kind {
		case lockIssueBlocking:
			finding.MessageKey = "lock-held-across-blocking-call"
			finding.Message = "known blocking call executes while a sync lock may be held"
			finding.Help = "release the lock before waiting or narrow the critical section"
			if issue.acquisition != nil {
				acquisitionRange, rangeErr := ctx.Range(issue.acquisition)
				if rangeErr != nil {
					return nil, rangeErr
				}
				finding.Related = []Related{
					{Range: acquisitionRange, Message: "lock acquired here"},
				}
			}
		case lockIssueNotReleased:
			finding.MessageKey = "lock-not-released"
			finding.Message = "function returns while a sync lock may remain held"
			finding.Help = "release the lock on every return path or use an immediate defer"
		case lockIssueInvalidUnlock:
			finding.MessageKey = "unlock-without-lock"
			finding.Message = issue.message
			finding.Help = "pair this operation with the matching lock mode on every path"
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func lockAnalysisFor(ctx *ControlFlowContext) *lockStateAnalysis {
	if ctx.shared == nil {
		return buildLockStateAnalysis(ctx)
	}
	ctx.shared.lockStateOnce.Do(
		func() {
			ctx.shared.lockState = buildLockStateAnalysis(ctx)
		},
	)
	return ctx.shared.lockState
}

func buildLockStateAnalysis(ctx *ControlFlowContext) *lockStateAnalysis {
	keys, acquisitions := collectLockKeys(ctx)
	if len(keys) == 0 {
		return &lockStateAnalysis{complete: true, issues: []lockIssue{}}
	}
	if len(keys) > maxTrackedFunctionLocks {
		return &lockStateAnalysis{}
	}
	builder := &lockAnalysisBuilder{
		ctx: ctx,
		keys: keys,
		acquisitions: acquisitions,
		issues: make(map[lockIssueIdentity]lockIssue),
	}
	initial := lockFlowState{values: make(map[lockKey]lockValue, len(keys))}
	for _, key := range keys {
		states := lockStateUnknown
		if key.local {
			states = 0
		}
		initial.values[key] = lockValue{states: states}
	}
	changeBound := len(ctx.Graph().Blocks) * (len(keys) * 16 + 4)
	if changeBound <= 0 || changeBound > maxStateTransitionChanges {
		changeBound = maxStateTransitionChanges
	}
	snapshot, complete := runStateTransitions(
		ctx.Graph(),
		stateTransitionModel[lockFlowState]{
			Initial: initial,
			Clone: cloneLockFlowState,
			Merge: mergeLockFlowState,
			Transfer: builder.transfer,
			MaxChanges: changeBound,
		},
	)
	if !complete {
		return &lockStateAnalysis{}
	}
	builder.record = true
	for _, block := range ctx.Graph().Blocks {
		if block == nil ||
			!block.Live ||
			block.Index < 0 ||
			int(block.Index) >= len(snapshot.entries) ||
			!snapshot.present[block.Index] {
			continue
		}
		state := cloneLockFlowState(snapshot.entries[block.Index])
		reachable := true
		for _, node := range block.Nodes {
			if !builder.transfer(state, node) {
				reachable = false
				break
			}
		}
		if reachable {
			if returned := block.Return(); returned != nil {
				builder.returned(state, returned)
			}
		}
	}
	issues := make([]lockIssue, 0, len(builder.issues))
	for _, issue := range builder.issues {
		issues = append(issues, issue)
	}
	sort.Slice(
		issues,
		func(left, right int) bool {
			leftPosition := lockIssuePosition(issues[left])
			rightPosition := lockIssuePosition(issues[right])
			if leftPosition != rightPosition {
				return leftPosition < rightPosition
			}
			return issues[left].kind < issues[right].kind
		},
	)
	return &lockStateAnalysis{complete: true, issues: issues}
}

func collectLockKeys(ctx *ControlFlowContext) ([]lockKey, map[lockKey][]*ast.CallExpr) {
	keys := make([]lockKey, 0)
	seen := make(map[lockKey]struct{})
	acquisitions := make(map[lockKey][]*ast.CallExpr)
	root := ctx.Function()
	ast.Inspect(
		root,
		func(node ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested && literal != root {
				return false
			}
			call, _ := node.(*ast.CallExpr)
			if call == nil {
				return true
			}
			key, operation, found := syncLockOperation(ctx, call)
			if !found {
				return true
			}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				keys = append(keys, key)
			}
			if operation == lockOperationLock || operation == lockOperationReadLock {
				acquisitions[key] = append(acquisitions[key], call)
			}
			return true
		},
	)
	sort.Slice(
		keys,
		func(left, right int) bool {
			leftCalls := acquisitions[keys[left]]
			rightCalls := acquisitions[keys[right]]
			leftPosition, rightPosition := token.NoPos, token.NoPos
			if len(leftCalls) > 0 {
				leftPosition = leftCalls[0].Pos()
			}
			if len(rightCalls) > 0 {
				rightPosition = rightCalls[0].Pos()
			}
			if leftPosition != rightPosition {
				return leftPosition < rightPosition
			}
			return keys[left].base.Pos() < keys[right].base.Pos()
		},
	)
	return keys, acquisitions
}

func cloneLockFlowState(state lockFlowState) lockFlowState {
	result := lockFlowState{values: make(map[lockKey]lockValue, len(state.values))}
	for key, value := range state.values {
		result.values[key] = value
	}
	return result
}

func mergeLockFlowState(existing *lockFlowState, incoming lockFlowState) bool {
	changed := false
	for key, incomingValue := range incoming.values {
		value := existing.values[key]
		states := value.states | incomingValue.states
		if value.states & lockStateUnknown != 0 ||
			incomingValue.states & lockStateUnknown != 0 {
			states = lockStateUnknown
		}
		deferred := mergeDeferredLockOperation(value.deferred, incomingValue.deferred)
		if states != value.states || deferred != value.deferred {
			existing.values[key] = lockValue{states: states, deferred: deferred}
			changed = true
		}
	}
	return changed
}

func mergeDeferredLockOperation(left, right deferredLockOperation) deferredLockOperation {
	if left == right {
		return left
	}
	if left.ambiguous ||
		right.ambiguous ||
		left.present != right.present ||
		left.operation != right.operation ||
		left.call != right.call {
		return deferredLockOperation{ambiguous: true, present: true}
	}
	return left
}

func (b *lockAnalysisBuilder) transfer(state lockFlowState, node ast.Node) bool {
	b.applyLockInitialization(state, node)
	if call, deferred, found := directLockStatementCall(node); found {
		key, operation, lockCall := syncLockOperation(b.ctx, call)
		if lockCall {
			if deferred {
				if operation == lockOperationUnlock ||
					operation == lockOperationReadUnlock {
					value := state.values[key]
					if value.deferred.present {
						value.deferred = deferredLockOperation{
							ambiguous: true,
							present: true,
						}
					} else {
						value.deferred = deferredLockOperation{
							operation: operation,
							call: call,
							present: true,
						}
					}
					state.values[key] = value
				} else {
					value := state.values[key]
					value.states = lockStateUnknown
					state.values[key] = value
				}
				return true
			}
			return b.applyOperation(state, key, operation, call)
		}
	}
	for _, call := range callsInLockNode(node) {
		if !b.blockingCall(call) {
			continue
		}
		key, acquisition, held := b.heldLockAt(state, call.Pos())
		if held {
			_ = key
			b.addIssue(
				lockIssue{
					kind: lockIssueBlocking,
					call: call,
					acquisition: acquisition,
				},
			)
		}
	}
	for _, key := range b.keys {
		if lockKeyEscapesInNode(b.ctx.Info(), node, key) {
			value := state.values[key]
			value.states = lockStateUnknown
			if value.deferred.present {
				value.deferred = deferredLockOperation{
					ambiguous: true,
					present: true,
				}
			}
			state.values[key] = value
		}
	}
	return true
}

func (b *lockAnalysisBuilder) applyOperation(
	state lockFlowState,
	key lockKey,
	operation lockOperation,
	call *ast.CallExpr,
) bool {
	value := state.values[key]
	switch operation {
	case lockOperationLock:
		value.states = lockStateWrite
	case lockOperationReadLock:
		value.states = acquireReadLockStates(value.states)
	case lockOperationUnlock, lockOperationReadUnlock:
		states, invalid, reachable := releaseLockStates(value.states, operation)
		if invalid {
			b.addIssue(
				lockIssue{
					kind: lockIssueInvalidUnlock,
					call: call,
					message: invalidUnlockMessage(
						operation,
						value.states,
						false,
					),
				},
			)
		}
		if !reachable {
			return false
		}
		value.states = states
	}
	state.values[key] = value
	return true
}

func acquireReadLockStates(states lockStateSet) lockStateSet {
	if states == 0 || states & lockStateUnknown != 0 {
		return lockStateReadOne
	}
	if states & lockStateReadEight != 0 {
		return lockStateUnknown
	}
	result := lockStateSet(0)
	if states & (lockStateUnlocked | lockStateWrite) != 0 {
		result |= lockStateReadOne
	}
	result |= shiftReadLockStatesUp(states)
	return result
}

func shiftReadLockStatesUp(states lockStateSet) lockStateSet {
	result := (states &
		(lockStateReadOne |
			lockStateReadTwo |
			lockStateReadThree |
			lockStateReadFour |
			lockStateReadFive |
			lockStateReadSix |
			lockStateReadSeven)) <<
		1
	return result
}

func shiftReadLockStatesDown(states lockStateSet) lockStateSet {
	result := (states &
		(lockStateReadTwo |
			lockStateReadThree |
			lockStateReadFour |
			lockStateReadFive |
			lockStateReadSix |
			lockStateReadSeven |
			lockStateReadEight)) >>
		1
	if states & lockStateReadOne != 0 {
		result |= lockStateUnlocked
	}
	return result
}

func releaseLockStates(states lockStateSet, operation lockOperation) (lockStateSet, bool, bool) {
	if states == 0 || states & lockStateUnknown != 0 {
		if operation == lockOperationUnlock {
			return lockStateUnlocked, false, true
		}
		return lockStateUnknown, false, true
	}
	var valid, invalid lockStateSet
	switch operation {
	case lockOperationUnlock:
		valid = states & lockStateWrite
		invalid = states & (lockStateUnlocked | lockStateRead)
		if valid != 0 {
			return lockStateUnlocked, invalid != 0, true
		}
	case lockOperationReadUnlock:
		valid = states & lockStateRead
		invalid = states & (lockStateUnlocked | lockStateWrite)
		if valid != 0 {
			return shiftReadLockStatesDown(valid), invalid != 0, true
		}
	}
	return 0, invalid != 0, false
}

func invalidUnlockMessage(operation lockOperation, states lockStateSet, deferred bool) string {
	prefix := "sync unlock"
	if operation == lockOperationUnlock {
		prefix = "Unlock"
	} else if operation == lockOperationReadUnlock {
		prefix = "RUnlock"
	}
	if deferred {
		prefix = "deferred " + prefix
	}
	switch {
	case operation == lockOperationUnlock && states & lockStateRead != 0:
		return prefix + " may execute while the RWMutex is only read-locked"
	case operation == lockOperationReadUnlock && states & lockStateWrite != 0:
		return prefix + " may execute while the RWMutex is write-locked"
	default:
		return prefix + " may execute without a matching held lock"
	}
}

func (b *lockAnalysisBuilder) returned(state lockFlowState, returned *ast.ReturnStmt) {
	for _, key := range b.keys {
		value := state.values[key]
		if value.deferred.ambiguous {
			continue
		}
		if value.deferred.present {
			states, invalid, reachable := releaseLockStates(
				value.states,
				value.deferred.operation,
			)
			if invalid {
				b.addIssue(
					lockIssue{
						kind: lockIssueInvalidUnlock,
						call: value.deferred.call,
						message: invalidUnlockMessage(
							value.deferred.operation,
							value.states,
							true,
						),
					},
				)
			}
			if !reachable {
				continue
			}
			value.states = states
		}
		if value.states == 0 ||
			value.states & lockStateUnknown != 0 ||
			value.states & lockStateHeld == 0 {
			continue
		}
		acquisition := b.latestAcquisition(key, returned.Pos())
		if acquisition == nil {
			continue
		}
		b.addIssue(lockIssue{kind: lockIssueNotReleased, position: returned.Pos()})
	}
}

func (b *lockAnalysisBuilder) addIssue(issue lockIssue) {
	position := lockIssuePosition(issue)
	if !b.record || !position.IsValid() {
		return
	}
	identity := lockIssueIdentity{kind: issue.kind, position: position}
	if _, exists := b.issues[identity]; !exists {
		b.issues[identity] = issue
	}
}

func lockIssuePosition(issue lockIssue) token.Pos {
	if issue.position.IsValid() {
		return issue.position
	}
	if issue.call != nil {
		return issue.call.Pos()
	}
	return token.NoPos
}

func (b *lockAnalysisBuilder) latestAcquisition(key lockKey, before token.Pos) *ast.CallExpr {
	var result *ast.CallExpr
	for _, call := range b.acquisitions[key] {
		if call.Pos() >= before {
			break
		}
		result = call
	}
	return result
}

func (b *lockAnalysisBuilder) heldLockAt(
	state lockFlowState,
	position token.Pos,
) (lockKey, *ast.CallExpr, bool) {
	for _, key := range b.keys {
		value := state.values[key]
		if value.states == 0 ||
			value.states & lockStateUnknown != 0 ||
			value.states & lockStateHeld == 0 {
			continue
		}
		if b.latestAcquisition(key, position) == nil {
			continue
		}
		return key, b.unambiguousAcquisition(key, position), true
	}
	return lockKey{}, nil, false
}

func (b *lockAnalysisBuilder) unambiguousAcquisition(key lockKey, before token.Pos) *ast.CallExpr {
	var result *ast.CallExpr
	for _, call := range b.acquisitions[key] {
		if call.Pos() >= before {
			break
		}
		if result != nil {
			return nil
		}
		result = call
	}
	return result
}

func (b *lockAnalysisBuilder) blockingCall(call *ast.CallExpr) bool {
	return knownBlockingCall(b.ctx.Info(), call) || b.ctx.Blocking(call)
}

func (b *lockAnalysisBuilder) applyLockInitialization(state lockFlowState, node ast.Node) {
	switch node := node.(type) {
	case *ast.ValueSpec:
		for index, name := range node.Names {
			key, found := b.directKeyForObject(b.ctx.Info().Defs[name])
			if !found {
				continue
			}
			states := lockStateUnknown
			if len(node.Values) == 0 &&
				zeroValueLockType(b.ctx.Info().TypeOf(name), key.kind) {
				states = lockStateUnlocked
			} else if index < len(node.Values) &&
				zeroLockExpression(b.ctx.Info(), node.Values[index], key.kind) {
				states = lockStateUnlocked
			}
			value := state.values[key]
			value.states = states
			state.values[key] = value
		}
	case *ast.AssignStmt:
		if len(node.Lhs) != len(node.Rhs) {
			return
		}
		for index, left := range node.Lhs {
			identifier, _ := ast.Unparen(left).(*ast.Ident)
			if identifier == nil {
				continue
			}
			object := b.ctx.Info().ObjectOf(identifier)
			key, found := b.directKeyForObject(object)
			if !found {
				continue
			}
			states := lockStateUnknown
			if zeroLockExpression(b.ctx.Info(), node.Rhs[index], key.kind) {
				states = lockStateUnlocked
			}
			value := state.values[key]
			if value.deferred.present {
				value.deferred = deferredLockOperation{
					ambiguous: true,
					present: true,
				}
			}
			value.states = states
			state.values[key] = value
		}
	}
}

func (b *lockAnalysisBuilder) directKeyForObject(object types.Object) (lockKey, bool) {
	for _, key := range b.keys {
		if key.field == nil && key.base == object {
			return key, true
		}
	}
	return lockKey{}, false
}

func directLockStatementCall(node ast.Node) (*ast.CallExpr, bool, bool) {
	switch node := node.(type) {
	case *ast.ExprStmt:
		call, _ := ast.Unparen(node.X).(*ast.CallExpr)
		return call, false, call != nil
	case *ast.DeferStmt:
		return node.Call, true, node.Call != nil
	default:
		return nil, false, false
	}
}

func syncLockOperation(ctx *ControlFlowContext, call *ast.CallExpr) (lockKey, lockOperation, bool) {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return lockKey{}, 0, false
	}
	selection := ctx.Info().Selections[selector]
	function, _ := selectionObject(selection).(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "sync" {
		return lockKey{}, 0, false
	}
	kind, found := exactSyncLockKind(selection.Recv())
	if !found {
		return lockKey{}, 0, false
	}
	var operation lockOperation
	switch function.Name() {
	case "Lock":
		operation = lockOperationLock
	case "Unlock":
		operation = lockOperationUnlock
	case "RLock":
		if kind != rwMutexLock {
			return lockKey{}, 0, false
		}
		operation = lockOperationReadLock
	case "RUnlock":
		if kind != rwMutexLock {
			return lockKey{}, 0, false
		}
		operation = lockOperationReadUnlock
	default:
		return lockKey{}, 0, false
	}
	key, found := lockReceiverKey(ctx, selector.X, kind)
	return key, operation, found
}

func lockReceiverKey(ctx *ControlFlowContext, expression ast.Expr, kind lockKind) (lockKey, bool) {
	expression = ast.Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		object := ctx.Info().ObjectOf(identifier)
		if object == nil {
			return lockKey{}, false
		}
		return lockKey{
			base: object,
			kind: kind,
			local: object.Pos() >= ctx.Body().Pos() && object.Pos() < ctx.Body().End(),
		}, true
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return lockKey{}, false
	}
	base, _ := ast.Unparen(selector.X).(*ast.Ident)
	selection := ctx.Info().Selections[selector]
	field, _ := selectionObject(selection).(*types.Var)
	if base == nil || selection == nil || selection.Kind() != types.FieldVal || field == nil {
		return lockKey{}, false
	}
	baseObject := ctx.Info().ObjectOf(base)
	if baseObject == nil {
		return lockKey{}, false
	}
	return lockKey{base: baseObject, field: field, kind: kind}, true
}

func exactSyncLockKind(type_ types.Type) (lockKind, bool) {
	if pointer, ok := types.Unalias(type_).(*types.Pointer); ok {
		type_ = pointer.Elem()
	}
	named, _ := types.Unalias(type_).(*types.Named)
	if named == nil ||
		named.Obj() == nil ||
		named.Obj().Pkg() == nil ||
		named.Obj().Pkg().Path() != "sync" {
		return 0, false
	}
	switch named.Obj().Name() {
	case "Mutex":
		return mutexLock, true
	case "RWMutex":
		return rwMutexLock, true
	default:
		return 0, false
	}
}

func zeroValueLockType(type_ types.Type, kind lockKind) bool {
	actual, found := exactSyncLockKind(type_)
	if !found || actual != kind {
		return false
	}
	_, pointer := types.Unalias(type_).(*types.Pointer)
	return !pointer
}

func zeroLockExpression(info *types.Info, expression ast.Expr, kind lockKind) bool {
	expression = ast.Unparen(expression)
	switch expression := expression.(type) {
	case *ast.CompositeLit:
		actual, found := exactSyncLockKind(info.TypeOf(expression))
		return found && actual == kind && len(expression.Elts) == 0
	case *ast.UnaryExpr:
		if expression.Op != token.AND {
			return false
		}
		literal, _ := ast.Unparen(expression.X).(*ast.CompositeLit)
		actual, found := exactSyncLockKind(info.TypeOf(expression))
		return literal != nil && found && actual == kind && len(literal.Elts) == 0
	case *ast.CallExpr:
		identifier, _ := ast.Unparen(expression.Fun).(*ast.Ident)
		if identifier == nil ||
			info.ObjectOf(identifier) != types.Universe.Lookup("new") ||
			len(expression.Args) != 1 {
			return false
		}
		actual, found := exactSyncLockKind(info.TypeOf(expression))
		return found && actual == kind
	default:
		return false
	}
}

func callsInLockNode(node ast.Node) []*ast.CallExpr {
	result := make([]*ast.CallExpr, 0)
	root := node
	var deferredOrAsynchronous *ast.CallExpr
	switch statement := node.(type) {
	case *ast.DeferStmt:
		deferredOrAsynchronous = statement.Call
	case *ast.GoStmt:
		deferredOrAsynchronous = statement.Call
	}
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			if literal, nested := current.(*ast.FuncLit); nested && literal != root {
				return false
			}
			if call, ok := current.(*ast.CallExpr);
				ok && call != deferredOrAsynchronous {
				result = append(result, call)
			}
			return true
		},
	)
	return result
}

func lockKeyEscapesInNode(info *types.Info, node ast.Node, key lockKey) bool {
	if info == nil || node == nil {
		return false
	}
	escapes := false
	root := node
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, stack []ast.Node) bool {
			if escapes {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested && literal != root {
				if lockNodeUsesBase(info, literal.Body, key.base) {
					escapes = true
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				selector, _ := ast.Unparen(current.Fun).(*ast.SelectorExpr)
				if selector != nil &&
					lockExpressionUsesBase(info, selector.X, key.base) {
					escapes = true
					return false
				}
				for _, argument := range current.Args {
					if lockExpressionUsesBase(info, argument, key.base) {
						escapes = true
						return false
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if lockExpressionUsesBase(info, expression, key.base) {
						escapes = true
						return false
					}
				}
			case *ast.SendStmt:
				if lockExpressionUsesBase(info, current.Value, key.base) {
					escapes = true
					return false
				}
			case *ast.GoStmt:
				if lockNodeUsesBase(info, current.Call, key.base) {
					escapes = true
					return false
				}
			case *ast.AssignStmt:
				for _, expression := range current.Rhs {
					if !lockExpressionUsesBase(info, expression, key.base) {
						continue
					}
					for _, target := range current.Lhs {
						identifier, _ := ast.Unparen(target).(*ast.Ident)
						if identifier == nil ||
							info.ObjectOf(identifier) != key.base {
							escapes = true
							return false
						}
					}
				}
			}
			_ = stack
			return true
		},
	)
	return escapes
}

func lockExpressionUsesBase(info *types.Info, expression ast.Expr, base types.Object) bool {
	return lockNodeUsesBase(info, expression, base)
}

func lockNodeUsesBase(info *types.Info, node ast.Node, base types.Object) bool {
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			identifier, ok := current.(*ast.Ident)
			if ok && info.ObjectOf(identifier) == base {
				found = true
				return false
			}
			return !found
		},
	)
	return found
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
		return function.Name() == "Wait" && namedTypeName(selection.Recv()) == "WaitGroup"
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
