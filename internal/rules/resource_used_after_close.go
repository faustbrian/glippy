package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

const maxTrackedFunctionResources = 4096

type resourceUsedAfterCloseRule struct{}

type resourceUseState uint8

const (
	resourceUseUntracked resourceUseState = 1 << iota
	resourceUseOpen
	resourceUseClosed
	resourceUseUnknown
)

type resourceUseFlowState struct {
	values map[types.Object]resourceUseState
}

type resourceUseIssue struct {
	object types.Object
	call *ast.CallExpr
	close *ast.CallExpr
}

type resourceUseAnalysis struct {
	complete bool
	issues []resourceUseIssue
}

type resourceUseBuilder struct {
	ctx *ControlFlowContext
	objects []types.Object
	acquisitions map[*ast.AssignStmt]map[types.Object]struct{}
	closeCalls map[types.Object][]*ast.CallExpr
	issues map[token.Pos]resourceUseIssue
	record bool
}

type resourceCallKind uint8

const (
	resourceCallClose resourceCallKind = iota
	resourceCallOperation
	resourceCallUnknown
)

type resourceCallEffect struct {
	kind resourceCallKind
	call *ast.CallExpr
}

var closedResourceOperationNames = map[string]struct{}{
	"Accept": {},
	"Chdir": {},
	"Chmod": {},
	"Chown": {},
	"Fd": {},
	"Flush": {},
	"Read": {},
	"ReadAt": {},
	"ReadByte": {},
	"ReadDir": {},
	"ReadFrom": {},
	"ReadRune": {},
	"Readdir": {},
	"Readdirnames": {},
	"Seek": {},
	"SetDeadline": {},
	"SetReadDeadline": {},
	"SetWriteDeadline": {},
	"Stat": {},
	"Sync": {},
	"Truncate": {},
	"Write": {},
	"WriteAt": {},
	"WriteByte": {},
	"WriteRune": {},
	"WriteString": {},
	"WriteTo": {},
}

// NewResourceUsedAfterCloseRule constructs the local closed-resource use rule.
func NewResourceUsedAfterCloseRule() Rule {
	return resourceUsedAfterCloseRule{}
}

func (resourceUsedAfterCloseRule) Metadata() Metadata {
	return Metadata{
		ID: "resource-used-after-close",
		Summary: "detects direct operations on definitely closed local resources",
		Documentation: "Calling an operational method after a resource has definitely been closed usually returns a closed-handle error, loses work, or panics. The rule follows locally acquired Close() error values through the shared control-flow graph, consumes proven project close effects, and reports only when every reaching state is closed.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract tracks direct local call results whose static type has Close() error and a curated set of I/O, file, deadline, synchronization, and accept operations.",
			"An open/closed branch join, alias, escape, asynchronous close, unknown helper, reassignment, reinitializer, observer, or arbitrary method becomes unknown instead of producing a speculative finding.",
			"A CFG node containing multiple tracked calls becomes unknown because AST preorder does not by itself prove Go evaluation order for every nested call shape.",
			"A statically resolved helper with a proven close effect establishes closed state; every other helper use becomes unknown because ownership borrowing does not prove resource state preservation.",
			"Deferred close calls do not close the resource at registration time and therefore do not affect later statements in the same function.",
			"The rule remains suspicious because a conventional Close method does not standardize every concrete resource's post-close behavior.",
		},
		Examples: []Example{
			{
				Title: "Do not read a file after closing it",
				Incorrect: "file, err := os.Open(path)\nif err != nil { return err }\nfile.Close()\n_, err = file.Read(buffer)",
				Correct: "file, err := os.Open(path)\nif err != nil { return err }\n_, err = file.Read(buffer)\nfile.Close()",
			},
		},
	}
}

func (resourceUsedAfterCloseRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"resource-used-after-close requires a complete control-flow context",
		)
	}
	analysis := resourceUseAnalysisFor(ctx)
	if analysis == nil || !analysis.complete {
		return nil, nil
	}
	findings := make([]Finding, 0, len(analysis.issues))
	for _, issue := range analysis.issues {
		range_, err := ctx.Range(issue.call)
		if err != nil {
			return nil, err
		}
		method := resourceMethodName(ctx.Info(), issue.call, issue.object)
		finding := Finding{
			MessageKey: "resource-used-after-close",
			Message: fmt.Sprintf(
				"resource %q is used by %s after it is closed",
				issue.object.Name(),
				method,
			),
			Range: range_,
			Help: "move the operation before Close or reacquire the resource",
		}
		if issue.close != nil {
			closeRange, rangeErr := ctx.Range(issue.close)
			if rangeErr != nil {
				return nil, rangeErr
			}
			finding.Related = []Related{
				{Range: closeRange, Message: "resource closed here"},
			}
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func resourceUseAnalysisFor(ctx *ControlFlowContext) *resourceUseAnalysis {
	if ctx.shared == nil {
		return buildResourceUseAnalysis(ctx)
	}
	ctx.shared.resourceUseOnce.Do(
		func() {
			ctx.shared.resourceUse = buildResourceUseAnalysis(ctx)
		},
	)
	return ctx.shared.resourceUse
}

func buildResourceUseAnalysis(ctx *ControlFlowContext) *resourceUseAnalysis {
	objects, acquisitions := collectResourceUseCandidates(
		ctx.Info(),
		ctx.Body(),
		ctx.ReturnAliasesArgument,
	)
	if len(objects) == 0 {
		return &resourceUseAnalysis{complete: true, issues: []resourceUseIssue{}}
	}
	if len(objects) > maxTrackedFunctionResources {
		return &resourceUseAnalysis{}
	}
	builder := &resourceUseBuilder{
		ctx: ctx,
		objects: objects,
		acquisitions: acquisitions,
		closeCalls: collectResourceCloseCalls(ctx, objects),
		issues: make(map[token.Pos]resourceUseIssue),
	}
	initial := resourceUseFlowState{
		values: make(map[types.Object]resourceUseState, len(objects)),
	}
	for _, object := range objects {
		initial.values[object] = resourceUseUntracked
	}
	changeBound := len(ctx.Graph().Blocks) * (len(objects) * 8 + 4)
	if changeBound <= 0 || changeBound > maxStateTransitionChanges {
		changeBound = maxStateTransitionChanges
	}
	snapshot, complete := runStateTransitions(
		ctx.Graph(),
		stateTransitionModel[resourceUseFlowState]{
			Initial: initial,
			Clone: cloneResourceUseFlowState,
			Merge: mergeResourceUseFlowState,
			Transfer: builder.transfer,
			MaxChanges: changeBound,
		},
	)
	if !complete {
		return &resourceUseAnalysis{}
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
		state := cloneResourceUseFlowState(snapshot.entries[block.Index])
		for _, node := range block.Nodes {
			builder.transfer(state, node)
		}
	}
	issues := make([]resourceUseIssue, 0, len(builder.issues))
	for _, issue := range builder.issues {
		issues = append(issues, issue)
	}
	sort.Slice(
		issues,
		func(left, right int) bool {
			return issues[left].call.Pos() < issues[right].call.Pos()
		},
	)
	return &resourceUseAnalysis{complete: true, issues: issues}
}

func collectResourceUseCandidates(
	info *types.Info,
	body *ast.BlockStmt,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) ([]types.Object, map[*ast.AssignStmt]map[types.Object]struct{}) {
	objects := make([]types.Object, 0)
	seen := make(map[types.Object]struct{})
	acquisitions := make(map[*ast.AssignStmt]map[types.Object]struct{})
	for _, candidate := range localCloserCandidates(info, body, returnsAlias, nil) {
		if _, found := seen[candidate.object]; !found {
			seen[candidate.object] = struct{}{}
			objects = append(objects, candidate.object)
		}
		set := acquisitions[candidate.statement]
		if set == nil {
			set = make(map[types.Object]struct{})
			acquisitions[candidate.statement] = set
		}
		set[candidate.object] = struct{}{}
	}
	sort.Slice(
		objects,
		func(left, right int) bool {
			return objects[left].Pos() < objects[right].Pos()
		},
	)
	return objects, acquisitions
}

func cloneResourceUseFlowState(state resourceUseFlowState) resourceUseFlowState {
	result := resourceUseFlowState{
		values: make(map[types.Object]resourceUseState, len(state.values)),
	}
	for object, value := range state.values {
		result.values[object] = value
	}
	return result
}

func mergeResourceUseFlowState(existing *resourceUseFlowState, incoming resourceUseFlowState) bool {
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

func (b *resourceUseBuilder) transfer(state resourceUseFlowState, node ast.Node) bool {
	for _, object := range b.objects {
		if b.acquires(node, object) {
			state.values[object] = resourceUseOpen
			continue
		}
		b.transferObject(state, node, object)
	}
	return true
}

func (b *resourceUseBuilder) transferObject(
	state resourceUseFlowState,
	node ast.Node,
	object types.Object,
) {
	value := state.values[object]
	effects := b.callEffects(node, object)
	if len(effects) > 1 {
		state.values[object] = resourceUseUnknown
		return
	}
	if len(effects) == 1 {
		effect := effects[0]
		switch effect.kind {
		case resourceCallClose:
			value = closeResourceUseState(value)
		case resourceCallOperation:
			if value == resourceUseClosed {
				b.addIssue(
					resourceUseIssue{
						object: object,
						call: effect.call,
						close: b.unambiguousClose(
							object,
							effect.call.Pos(),
						),
					},
				)
			}
		case resourceCallUnknown:
			value = resourceUseUnknown
		}
	}
	if deferred, ok := node.(*ast.DeferStmt); ok && deferred.Call != nil {
		state.values[object] = value
		return
	}
	if asynchronous, ok := node.(*ast.GoStmt);
		ok &&
			asynchronous.Call != nil &&
			expressionUsesObject(b.ctx.Info(), asynchronous.Call, object) {
		state.values[object] = resourceUseUnknown
		return
	}
	effect := objectObligationEffect(
		b.ctx.Info(),
		node,
		object,
		nil,
		b.ctx.ParameterEffect,
		ParameterEffectClose | ParameterEffectTransfer,
		nil,
	)
	if effect == obligationTransferred || effect == obligationLost {
		value = resourceUseUnknown
	}
	if assignmentReplacesObject(b.ctx.Info(), node, object, b.ctx.ReturnAliasesArgument) {
		value = resourceUseUnknown
	}
	state.values[object] = value
}

func (b *resourceUseBuilder) acquires(node ast.Node, object types.Object) bool {
	assignment, _ := node.(*ast.AssignStmt)
	if assignment == nil {
		return false
	}
	_, found := b.acquisitions[assignment][object]
	return found
}

func (b *resourceUseBuilder) callEffects(node ast.Node, object types.Object) []resourceCallEffect {
	result := make([]resourceCallEffect, 0, 1)
	for _, call := range callsInLockNode(node) {
		if closeCallUsesObject(b.ctx.Info(), call, object) {
			result = append(
				result,
				resourceCallEffect{kind: resourceCallClose, call: call},
			)
			continue
		}
		if method := resourceMethodName(b.ctx.Info(), call, object); method != "" {
			if _, operational := closedResourceOperationNames[method]; operational {
				result = append(
					result,
					resourceCallEffect{kind: resourceCallOperation, call: call},
				)
			} else if statefulResourceMethod(method) {
				result = append(
					result,
					resourceCallEffect{kind: resourceCallUnknown, call: call},
				)
			}
			continue
		}
		for index, argument := range call.Args {
			if directObject(b.ctx.Info(), argument) != object {
				continue
			}
			summary := b.ctx.ParameterEffect(call, index)
			if summary.GuaranteesAny(ParameterEffectClose) {
				result = append(
					result,
					resourceCallEffect{kind: resourceCallClose, call: call},
				)
			} else {
				result = append(
					result,
					resourceCallEffect{kind: resourceCallUnknown, call: call},
				)
			}
		}
	}
	return result
}

func statefulResourceMethod(method string) bool {
	return method != "Close"
}

func closeResourceUseState(state resourceUseState) resourceUseState {
	if state & resourceUseUntracked != 0 {
		return resourceUseUnknown
	}
	return resourceUseClosed
}

func resourceMethodName(info *types.Info, call *ast.CallExpr, object types.Object) string {
	if info == nil || call == nil || object == nil {
		return ""
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || directObject(info, selector.X) != object {
		return ""
	}
	selection := info.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return ""
	}
	return selector.Sel.Name
}

func collectResourceCloseCalls(
	ctx *ControlFlowContext,
	objects []types.Object,
) map[types.Object][]*ast.CallExpr {
	result := make(map[types.Object][]*ast.CallExpr, len(objects))
	root := ctx.Function()
	ast.PreorderStack(
		root,
		nil,
		func(node ast.Node, stack []ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested && literal != root {
				return false
			}
			call, _ := node.(*ast.CallExpr)
			if call == nil || deferredOrAsynchronousCall(call, stack) {
				return true
			}
			for _, object := range objects {
				if closeCallUsesObject(ctx.Info(), call, object) ||
					helperGuaranteesResourceClose(ctx, call, object) {
					result[object] = append(result[object], call)
				}
			}
			return true
		},
	)
	return result
}

func deferredOrAsynchronousCall(call *ast.CallExpr, stack []ast.Node) bool {
	if len(stack) == 0 {
		return false
	}
	switch parent := stack[len(stack) - 1].(type) {
	case *ast.DeferStmt:
		return parent.Call == call
	case *ast.GoStmt:
		return parent.Call == call
	default:
		return false
	}
}

func helperGuaranteesResourceClose(
	ctx *ControlFlowContext,
	call *ast.CallExpr,
	object types.Object,
) bool {
	for index, argument := range call.Args {
		if directObject(ctx.Info(), argument) == object &&
			ctx.ParameterEffect(call, index).GuaranteesAny(ParameterEffectClose) {
			return true
		}
	}
	return false
}

func (b *resourceUseBuilder) unambiguousClose(object types.Object, before token.Pos) *ast.CallExpr {
	var result *ast.CallExpr
	for _, call := range b.closeCalls[object] {
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

func (b *resourceUseBuilder) addIssue(issue resourceUseIssue) {
	if !b.record || issue.call == nil || !issue.call.Pos().IsValid() {
		return
	}
	if _, found := b.issues[issue.call.Pos()]; !found {
		b.issues[issue.call.Pos()] = issue
	}
}
