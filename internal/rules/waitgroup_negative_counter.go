package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
)

const (
	maxTrackedFunctionWaitGroups = 4096
	waitGroupUntrackedCount uint64 = 1 << 61
	waitGroupUnknownCount uint64 = 1 << 62
	waitGroupEscapedCount uint64 = 1 << 63
	waitGroupTrackedCountLimit = 60
)

type waitGroupNegativeCounterRule struct{}

type waitGroupCounterFlowState struct {
	values map[types.Object]uint64
}

type waitGroupCounterIssue struct {
	object types.Object
	call *ast.CallExpr
}

type waitGroupCounterAnalysis struct {
	complete bool
	issues []waitGroupCounterIssue
}

type waitGroupCounterBuilder struct {
	ctx *ControlFlowContext
	objects []types.Object
	initializations map[ast.Node]map[types.Object]bool
	issues map[token.Pos]waitGroupCounterIssue
	record bool
}

type waitGroupCounterOperationKind uint8

const (
	waitGroupCounterAdd waitGroupCounterOperationKind = iota
	waitGroupCounterWait
	waitGroupCounterUnknown
	waitGroupCounterEscape
)

type waitGroupCounterOperation struct {
	kind waitGroupCounterOperationKind
	call *ast.CallExpr
	delta int64
}

// NewWaitGroupNegativeCounterRule constructs the local WaitGroup counter rule.
func NewWaitGroupNegativeCounterRule() Rule {
	return waitGroupNegativeCounterRule{}
}

func (waitGroupNegativeCounterRule) Metadata() Metadata {
	return Metadata{
		ID: "waitgroup-negative-counter",
		Summary: "detects WaitGroup operations that definitely make the counter negative",
		Documentation: "sync.WaitGroup panics when Add or Done makes its task counter negative. The rule follows directly initialized local WaitGroups through bounded control flow and reports only when every reaching counter state underflows.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The initial contract tracks only direct local WaitGroup values and pointers initialized from the exact zero value.",
			"Only exact integer Add arguments and direct Done and Wait calls participate in counter propagation.",
			"Aliases, helper calls, closure capture, asynchronous operations, and uncertain evaluation order become conservative unknown state.",
			"Deferred counter operations are not applied at registration and are not modeled at function exit.",
			"Counts above the bounded exact-state limit become unknown rather than producing a speculative finding.",
			"Reinitialization after an alias or other unknown use does not reestablish precise state because pointers may still refer to the same WaitGroup storage.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Do not decrement an empty WaitGroup",
				Incorrect: "var group sync.WaitGroup\ngroup.Done()",
				Correct: "var group sync.WaitGroup\ngroup.Add(1)\ngroup.Done()",
			},
		},
	}
}

func (waitGroupNegativeCounterRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"waitgroup-negative-counter requires a complete control-flow context",
		)
	}
	analysis := waitGroupCounterAnalysisFor(ctx)
	if analysis == nil || !analysis.complete {
		return nil, nil
	}
	findings := make([]Finding, 0, len(analysis.issues))
	for _, issue := range analysis.issues {
		range_, err := ctx.Range(issue.call)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				Range: range_,
				MessageKey: "negative-counter",
				Message: fmt.Sprintf(
					"WaitGroup %q counter becomes negative and panics",
					issue.object.Name(),
				),
				Help: "balance Add and Done calls on every path before decrementing the counter",
			},
		)
	}
	return findings, nil
}

func waitGroupCounterAnalysisFor(ctx *ControlFlowContext) *waitGroupCounterAnalysis {
	if ctx.shared == nil {
		return buildWaitGroupCounterAnalysis(ctx)
	}
	ctx.shared.waitGroupCounterOnce.Do(
		func() {
			ctx.shared.waitGroupCounter = buildWaitGroupCounterAnalysis(ctx)
		},
	)
	return ctx.shared.waitGroupCounter
}

func buildWaitGroupCounterAnalysis(ctx *ControlFlowContext) *waitGroupCounterAnalysis {
	objects, initializations := collectWaitGroupCounterCandidates(ctx.Info(), ctx.Body())
	if len(objects) == 0 {
		return &waitGroupCounterAnalysis{complete: true}
	}
	if len(objects) > maxTrackedFunctionWaitGroups {
		return &waitGroupCounterAnalysis{}
	}
	builder := &waitGroupCounterBuilder{
		ctx: ctx,
		objects: objects,
		initializations: initializations,
		issues: make(map[token.Pos]waitGroupCounterIssue),
	}
	initial := waitGroupCounterFlowState{values: make(map[types.Object]uint64, len(objects))}
	for _, object := range objects {
		initial.values[object] = waitGroupUntrackedCount
	}
	changeBound := len(ctx.Graph().Blocks) * (len(objects) * 64 + 4)
	if changeBound <= 0 || changeBound > maxStateTransitionChanges {
		changeBound = maxStateTransitionChanges
	}
	snapshot, complete := runStateTransitions(
		ctx.Graph(),
		stateTransitionModel[waitGroupCounterFlowState]{
			Initial: initial,
			Clone: cloneWaitGroupCounterFlowState,
			Merge: mergeWaitGroupCounterFlowState,
			Transfer: builder.transfer,
			MaxChanges: changeBound,
		},
	)
	if !complete {
		return &waitGroupCounterAnalysis{}
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
		state := cloneWaitGroupCounterFlowState(snapshot.entries[block.Index])
		for _, node := range block.Nodes {
			if !builder.transfer(state, node) {
				break
			}
		}
	}
	issues := make([]waitGroupCounterIssue, 0, len(builder.issues))
	for _, issue := range builder.issues {
		issues = append(issues, issue)
	}
	sort.Slice(
		issues,
		func(left, right int) bool {
			return issues[left].call.Pos() < issues[right].call.Pos()
		},
	)
	return &waitGroupCounterAnalysis{complete: true, issues: issues}
}

func collectWaitGroupCounterCandidates(
	info *types.Info,
	body *ast.BlockStmt,
) ([]types.Object, map[ast.Node]map[types.Object]bool) {
	objects := make([]types.Object, 0)
	seen := make(map[types.Object]struct{})
	initializations := make(map[ast.Node]map[types.Object]bool)
	root := body
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested && literal.Body != root {
				return false
			}
			for object, fresh := range waitGroupInitializations(info, node) {
				if !channelObjectDeclaredInBody(body, object) {
					continue
				}
				if _, found := seen[object]; !found {
					seen[object] = struct{}{}
					objects = append(objects, object)
				}
				set := initializations[node]
				if set == nil {
					set = make(map[types.Object]bool)
					initializations[node] = set
				}
				set[object] = set[object] || fresh
			}
			return true
		},
	)
	sort.Slice(
		objects,
		func(left, right int) bool {
			return objects[left].Pos() < objects[right].Pos()
		},
	)
	return objects, initializations
}

func waitGroupInitializations(info *types.Info, node ast.Node) map[types.Object]bool {
	result := make(map[types.Object]bool)
	switch current := node.(type) {
	case *ast.AssignStmt:
		if len(current.Lhs) != len(current.Rhs) {
			return result
		}
		for index, target := range current.Lhs {
			object := directObject(info, target)
			if object != nil && zeroWaitGroupExpression(info, current.Rhs[index]) {
				identifier, _ := ast.Unparen(target).(*ast.Ident)
				result[object] = identifier != nil &&
					info.Defs[identifier] == object
			}
		}
	case *ast.DeclStmt:
		declaration, _ := current.Decl.(*ast.GenDecl)
		if declaration == nil || declaration.Tok != token.VAR {
			return result
		}
		for _, specification := range declaration.Specs {
			value, _ := specification.(*ast.ValueSpec)
			collectWaitGroupValueSpecInitializations(info, value, result)
		}
	case *ast.ValueSpec:
		collectWaitGroupValueSpecInitializations(info, current, result)
	}
	return result
}

func collectWaitGroupValueSpecInitializations(
	info *types.Info,
	value *ast.ValueSpec,
	result map[types.Object]bool,
) {
	if value == nil {
		return
	}
	for index, name := range value.Names {
		object := info.ObjectOf(name)
		if object == nil {
			continue
		}
		if len(value.Values) == 0 {
			if exactWaitGroupValueType(info.TypeOf(name)) {
				result[object] = true
			}
			continue
		}
		if index < len(value.Values) && zeroWaitGroupExpression(info, value.Values[index]) {
			result[object] = true
		}
	}
}

func exactWaitGroupValueType(type_ types.Type) bool {
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "sync" &&
		named.Obj().Name() == "WaitGroup"
}

func exactWaitGroupType(type_ types.Type) bool {
	if pointer, ok := types.Unalias(type_).(*types.Pointer); ok {
		type_ = pointer.Elem()
	}
	return exactWaitGroupValueType(type_)
}

func zeroWaitGroupExpression(info *types.Info, expression ast.Expr) bool {
	expression = ast.Unparen(expression)
	switch current := expression.(type) {
	case *ast.CompositeLit:
		return len(current.Elts) == 0 && exactWaitGroupValueType(info.TypeOf(current))
	case *ast.UnaryExpr:
		return current.Op == token.AND && zeroWaitGroupExpression(info, current.X)
	case *ast.CallExpr:
		identifier, _ := ast.Unparen(current.Fun).(*ast.Ident)
		return identifier != nil &&
			info.ObjectOf(identifier) == types.Universe.Lookup("new") &&
			len(current.Args) == 1 &&
			exactWaitGroupType(info.TypeOf(current))
	default:
		return false
	}
}

func cloneWaitGroupCounterFlowState(state waitGroupCounterFlowState) waitGroupCounterFlowState {
	result := waitGroupCounterFlowState{
		values: make(map[types.Object]uint64, len(state.values)),
	}
	for object, value := range state.values {
		result.values[object] = value
	}
	return result
}

func mergeWaitGroupCounterFlowState(
	existing *waitGroupCounterFlowState,
	incoming waitGroupCounterFlowState,
) bool {
	changed := false
	for object, incomingValue := range incoming.values {
		value := existing.values[object]
		merged := value | incomingValue
		if merged != value {
			existing.values[object] = merged
			changed = true
		}
	}
	return changed
}

func (b *waitGroupCounterBuilder) transfer(state waitGroupCounterFlowState, node ast.Node) bool {
	for _, object := range b.objects {
		if fresh, initialized := b.initializations[node][object]; initialized {
			if waitGroupInitializationRetainsReference(b.ctx.Info(), node, object) {
				state.values[object] = waitGroupEscapedCount
				continue
			}
			if fresh || state.values[object] & waitGroupEscapedCount == 0 {
				state.values[object] = 1
			}
			continue
		}
		if !b.transferObject(state, node, object) {
			return false
		}
	}
	return true
}

func waitGroupInitializationRetainsReference(
	info *types.Info,
	node ast.Node,
	object types.Object,
) bool {
	assignment, ok := node.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, target := range assignment.Lhs {
		if directObject(info, target) == object {
			continue
		}
		if expressionUsesObject(info, target, object) {
			return true
		}
	}
	for _, expression := range assignment.Rhs {
		if expressionUsesObject(info, expression, object) {
			return true
		}
	}
	return false
}

func (b *waitGroupCounterBuilder) transferObject(
	state waitGroupCounterFlowState,
	node ast.Node,
	object types.Object,
) bool {
	if deferred, ok := node.(*ast.DeferStmt);
		ok && expressionUsesObject(b.ctx.Info(), deferred.Call, object) {
		return true
	}
	if asynchronous, ok := node.(*ast.GoStmt);
		ok && expressionUsesObject(b.ctx.Info(), asynchronous.Call, object) {
		state.values[object] = waitGroupEscapedCount
		return true
	}
	operations := b.operations(node, object)
	if len(operations) > 1 {
		state.values[object] = waitGroupUnknownCount
		for _, operation := range operations {
			if operation.kind == waitGroupCounterEscape {
				state.values[object] = waitGroupEscapedCount
				break
			}
		}
		return true
	}
	if len(operations) == 0 {
		return true
	}
	operation := operations[0]
	switch operation.kind {
	case waitGroupCounterAdd:
		updated, underflow := applyWaitGroupDelta(state.values[object], operation.delta)
		if underflow {
			b.addIssue(object, operation.call)
			return false
		}
		state.values[object] = updated
	case waitGroupCounterWait:
		if state.values[object] & waitGroupEscapedCount != 0 {
			state.values[object] = waitGroupEscapedCount
			return true
		}
		if state.values[object] & waitGroupUnknownCount == 0 &&
			state.values[object] & 1 == 0 {
			return false
		}
		state.values[object] = 1
	case waitGroupCounterUnknown:
		state.values[object] = waitGroupUnknownCount
	case waitGroupCounterEscape:
		state.values[object] = waitGroupEscapedCount
	}
	return true
}

func applyWaitGroupDelta(states uint64, delta int64) (uint64, bool) {
	if states & waitGroupEscapedCount != 0 {
		return waitGroupEscapedCount, false
	}
	if states & (waitGroupUnknownCount | waitGroupUntrackedCount) != 0 {
		return waitGroupUnknownCount, false
	}
	result := uint64(0)
	for count := int64(0); count <= waitGroupTrackedCountLimit; count++ {
		if states & (uint64(1) << count) == 0 {
			continue
		}
		updated := count + delta
		if updated < 0 {
			continue
		}
		if updated > waitGroupTrackedCountLimit {
			result |= waitGroupUnknownCount
			continue
		}
		result |= uint64(1) << updated
	}
	return result, result == 0
}

func (b *waitGroupCounterBuilder) operations(
	node ast.Node,
	object types.Object,
) []waitGroupCounterOperation {
	result := make([]waitGroupCounterOperation, 0, 1)
	root := node
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if literal, nested := current.(*ast.FuncLit); nested && literal != root {
				if expressionUsesObject(b.ctx.Info(), literal.Body, object) {
					result = append(
						result,
						waitGroupCounterOperation{
							kind: waitGroupCounterEscape,
						},
					)
				}
				return false
			}
			call, _ := current.(*ast.CallExpr)
			if call != nil {
				if operation, found := directWaitGroupCounterOperation(
					b.ctx.Info(),
					call,
					object,
				);
					found {
					result = append(result, operation)
					return true
				}
				if expressionUsesObject(b.ctx.Info(), call, object) {
					result = append(
						result,
						waitGroupCounterOperation{
							kind: waitGroupCounterEscape,
						},
					)
					return false
				}
			}
			switch current := current.(type) {
			case *ast.SendStmt:
				if expressionUsesObject(b.ctx.Info(), current.Value, object) {
					result = append(
						result,
						waitGroupCounterOperation{
							kind: waitGroupCounterEscape,
						},
					)
				}
			case *ast.AssignStmt:
				if waitGroupAssignmentUsesObject(b.ctx.Info(), current, object) {
					result = append(
						result,
						waitGroupCounterOperation{
							kind: waitGroupCounterEscape,
						},
					)
				}
			case *ast.ValueSpec:
				if waitGroupValueSpecUsesObject(b.ctx.Info(), current, object) {
					result = append(
						result,
						waitGroupCounterOperation{
							kind: waitGroupCounterEscape,
						},
					)
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directObject(b.ctx.Info(), expression) == object {
						result = append(
							result,
							waitGroupCounterOperation{
								kind: waitGroupCounterEscape,
							},
						)
						break
					}
				}
			}
			return true
		},
	)
	return result
}

func waitGroupAssignmentUsesObject(
	info *types.Info,
	assignment *ast.AssignStmt,
	object types.Object,
) bool {
	for _, target := range assignment.Lhs {
		if directObject(info, target) == object {
			return true
		}
	}
	for _, expression := range assignment.Rhs {
		if expressionUsesObject(info, expression, object) {
			return true
		}
	}
	return false
}

func waitGroupValueSpecUsesObject(
	info *types.Info,
	value *ast.ValueSpec,
	object types.Object,
) bool {
	if value == nil {
		return false
	}
	for _, expression := range value.Values {
		if expressionUsesObject(info, expression, object) {
			return true
		}
	}
	return false
}

func directWaitGroupCounterOperation(
	info *types.Info,
	call *ast.CallExpr,
	object types.Object,
) (waitGroupCounterOperation, bool) {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || directObject(info, selector.X) != object {
		return waitGroupCounterOperation{}, false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	selection := info.Selections[selector]
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "sync" ||
		selection == nil ||
		!exactWaitGroupType(selection.Recv()) {
		return waitGroupCounterOperation{}, false
	}
	switch function.Name() {
	case "Done":
		if len(call.Args) == 0 {
			return waitGroupCounterOperation{
				kind: waitGroupCounterAdd,
				call: call,
				delta: -1,
			}, true
		}
	case "Add":
		if len(call.Args) != 1 {
			return waitGroupCounterOperation{}, false
		}
		value := info.Types[call.Args[0]].Value
		if value == nil {
			return waitGroupCounterOperation{
				kind: waitGroupCounterUnknown,
				call: call,
			}, true
		}
		delta, exact := constant.Int64Val(value)
		if !exact {
			return waitGroupCounterOperation{
				kind: waitGroupCounterUnknown,
				call: call,
			}, true
		}
		return waitGroupCounterOperation{
			kind: waitGroupCounterAdd,
			call: call,
			delta: delta,
		}, true
	case "Wait":
		if len(call.Args) == 0 {
			return waitGroupCounterOperation{
				kind: waitGroupCounterWait,
				call: call,
			}, true
		}
	case "Go":
		return waitGroupCounterOperation{kind: waitGroupCounterEscape, call: call}, true
	}
	return waitGroupCounterOperation{}, false
}

func (b *waitGroupCounterBuilder) addIssue(object types.Object, call *ast.CallExpr) {
	if !b.record || call == nil || !call.Pos().IsValid() {
		return
	}
	if _, found := b.issues[call.Pos()]; found {
		return
	}
	b.issues[call.Pos()] = waitGroupCounterIssue{object: object, call: call}
}
