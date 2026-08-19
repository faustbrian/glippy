package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

const maxTrackedFunctionChannels = 4096

type channelUsedAfterCloseRule struct{}

type channelUseState uint8

const (
	channelUseUntracked channelUseState = 1 << iota
	channelUseOpen
	channelUseClosed
	channelUseUnknown
)

type channelUseFlowState struct {
	values map[types.Object]channelUseState
}

type channelUseIssueKind uint8

const (
	channelSendAfterClose channelUseIssueKind = iota
	channelCloseAfterClose
)

type channelUseIssue struct {
	kind channelUseIssueKind
	object types.Object
	node ast.Node
	close *ast.CallExpr
}

type channelUseAnalysis struct {
	complete bool
	issues []channelUseIssue
}

type channelUseBuilder struct {
	ctx *ControlFlowContext
	objects []types.Object
	acquisitions map[ast.Node]map[types.Object]struct{}
	closeCalls map[types.Object][]*ast.CallExpr
	issues map[token.Pos]channelUseIssue
	record bool
}

type channelUseOperationKind uint8

const (
	channelUseOperationClose channelUseOperationKind = iota
	channelUseOperationSend
	channelUseOperationSendAndEscape
	channelUseOperationUnknown
)

type channelUseOperation struct {
	kind channelUseOperationKind
	node ast.Node
}

// NewChannelUsedAfterCloseRule constructs the local channel state rule.
func NewChannelUsedAfterCloseRule() Rule {
	return channelUsedAfterCloseRule{}
}

func (channelUsedAfterCloseRule) Metadata() Metadata {
	return Metadata{
		ID: "channel-used-after-close",
		Summary: "detects sends and repeated closes on definitely closed local channels",
		Documentation: "Sending to or closing an already closed channel panics. The rule follows direct local channels initialized by the built-in make function through bounded control flow and reports only when every reaching state proves the channel closed.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The initial contract tracks only direct local channel variables initialized by the exact built-in make function.",
			"A finding requires every reaching path to prove the channel closed; conditional close, aliases, escapes, helper calls, asynchronous operations, and uncertain evaluation order become conservative unknown state.",
			"A later exact close reestablishes closed state after an alias or escape only on the normal continuation where that close returned.",
			"Receiving from a closed channel is legal and is never reported.",
			"Deferred close calls do not close a channel at registration time, and direct reinitialization with make establishes a new open channel.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Do not send after closing a channel",
				Incorrect: "updates := make(chan int)\nclose(updates)\nupdates <- 1",
				Correct: "updates := make(chan int)\nupdates <- 1\nclose(updates)",
			},
		},
	}
}

func (channelUsedAfterCloseRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"channel-used-after-close requires a complete control-flow context",
		)
	}
	analysis := channelUseAnalysisFor(ctx)
	if analysis == nil || !analysis.complete {
		return nil, nil
	}
	findings := make([]Finding, 0, len(analysis.issues))
	for _, issue := range analysis.issues {
		range_, err := ctx.Range(issue.node)
		if err != nil {
			return nil, err
		}
		finding := Finding{Range: range_}
		switch issue.kind {
		case channelSendAfterClose:
			finding.MessageKey = "send-after-close"
			finding.Message = fmt.Sprintf(
				"channel %q is sent to after it is closed",
				issue.object.Name(),
			)
			finding.Help = "move the send before close or create a new channel"
		case channelCloseAfterClose:
			finding.MessageKey = "close-after-close"
			finding.Message = fmt.Sprintf(
				"channel %q is closed after it is already closed",
				issue.object.Name(),
			)
			finding.Help = "remove the repeated close or give one owner responsibility for closing"
		default:
			continue
		}
		if issue.close != nil {
			closeRange, rangeErr := ctx.Range(issue.close)
			if rangeErr != nil {
				return nil, rangeErr
			}
			finding.Related = []Related{
				{Range: closeRange, Message: "channel closed here"},
			}
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func channelUseAnalysisFor(ctx *ControlFlowContext) *channelUseAnalysis {
	if ctx.shared == nil {
		return buildChannelUseAnalysis(ctx)
	}
	ctx.shared.channelUseOnce.Do(
		func() {
			ctx.shared.channelUse = buildChannelUseAnalysis(ctx)
		},
	)
	return ctx.shared.channelUse
}

func buildChannelUseAnalysis(ctx *ControlFlowContext) *channelUseAnalysis {
	objects, acquisitions := collectChannelUseCandidates(ctx.Info(), ctx.Body())
	if len(objects) == 0 {
		return &channelUseAnalysis{complete: true}
	}
	if len(objects) > maxTrackedFunctionChannels {
		return &channelUseAnalysis{}
	}
	builder := &channelUseBuilder{
		ctx: ctx,
		objects: objects,
		acquisitions: acquisitions,
		closeCalls: collectChannelCloseCalls(ctx, objects),
		issues: make(map[token.Pos]channelUseIssue),
	}
	initial := channelUseFlowState{values: make(map[types.Object]channelUseState, len(objects))}
	for _, object := range objects {
		initial.values[object] = channelUseUntracked
	}
	changeBound := len(ctx.Graph().Blocks) * (len(objects) * 8 + 4)
	if changeBound <= 0 || changeBound > maxStateTransitionChanges {
		changeBound = maxStateTransitionChanges
	}
	snapshot, complete := runStateTransitions(
		ctx.Graph(),
		stateTransitionModel[channelUseFlowState]{
			Initial: initial,
			Clone: cloneChannelUseFlowState,
			Merge: mergeChannelUseFlowState,
			Transfer: builder.transfer,
			MaxChanges: changeBound,
		},
	)
	if !complete {
		return &channelUseAnalysis{}
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
		state := cloneChannelUseFlowState(snapshot.entries[block.Index])
		for _, node := range block.Nodes {
			builder.transfer(state, node)
		}
	}
	issues := make([]channelUseIssue, 0, len(builder.issues))
	for _, issue := range builder.issues {
		issues = append(issues, issue)
	}
	sort.Slice(
		issues,
		func(left, right int) bool {
			return issues[left].node.Pos() < issues[right].node.Pos()
		},
	)
	return &channelUseAnalysis{complete: true, issues: issues}
}

func collectChannelUseCandidates(
	info *types.Info,
	body *ast.BlockStmt,
) ([]types.Object, map[ast.Node]map[types.Object]struct{}) {
	objects := make([]types.Object, 0)
	seen := make(map[types.Object]struct{})
	acquisitions := make(map[ast.Node]map[types.Object]struct{})
	root := body
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if literal, nested := node.(*ast.FuncLit); nested && literal.Body != root {
				return false
			}
			for object := range channelAcquisitions(info, node) {
				if !channelObjectDeclaredInBody(body, object) {
					continue
				}
				if _, found := seen[object]; !found {
					seen[object] = struct{}{}
					objects = append(objects, object)
				}
				set := acquisitions[node]
				if set == nil {
					set = make(map[types.Object]struct{})
					acquisitions[node] = set
				}
				set[object] = struct{}{}
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
	return objects, acquisitions
}

func channelObjectDeclaredInBody(body *ast.BlockStmt, object types.Object) bool {
	return body != nil &&
		object != nil &&
		object.Pos().IsValid() &&
		object.Pos() >= body.Pos() &&
		object.Pos() <= body.End()
}

func channelAcquisitions(info *types.Info, node ast.Node) map[types.Object]struct{} {
	result := make(map[types.Object]struct{})
	switch current := node.(type) {
	case *ast.AssignStmt:
		if len(current.Lhs) != len(current.Rhs) {
			return result
		}
		for index, target := range current.Lhs {
			object := directObject(info, target)
			if object != nil && exactBuiltinMakeChannel(info, current.Rhs[index]) {
				result[object] = struct{}{}
			}
		}
	case *ast.DeclStmt:
		declaration, _ := current.Decl.(*ast.GenDecl)
		if declaration == nil || declaration.Tok != token.VAR {
			return result
		}
		for _, specification := range declaration.Specs {
			value, _ := specification.(*ast.ValueSpec)
			if value == nil || len(value.Names) != len(value.Values) {
				continue
			}
			for index, name := range value.Names {
				object := info.ObjectOf(name)
				if object != nil &&
					exactBuiltinMakeChannel(info, value.Values[index]) {
					result[object] = struct{}{}
				}
			}
		}
	case *ast.ValueSpec:
		collectChannelValueSpecAcquisitions(info, current, result)
	}
	return result
}

func collectChannelValueSpecAcquisitions(
	info *types.Info,
	value *ast.ValueSpec,
	result map[types.Object]struct{},
) {
	if value == nil || len(value.Names) != len(value.Values) {
		return
	}
	for index, name := range value.Names {
		object := info.ObjectOf(name)
		if object != nil && exactBuiltinMakeChannel(info, value.Values[index]) {
			result[object] = struct{}{}
		}
	}
}

func exactBuiltinMakeChannel(info *types.Info, expression ast.Expr) bool {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil {
		return false
	}
	identifier, _ := ast.Unparen(call.Fun).(*ast.Ident)
	if identifier == nil || info.ObjectOf(identifier) != types.Universe.Lookup("make") {
		return false
	}
	_, channel := types.Unalias(info.TypeOf(call)).Underlying().(*types.Chan)
	return channel
}

func cloneChannelUseFlowState(state channelUseFlowState) channelUseFlowState {
	result := channelUseFlowState{
		values: make(map[types.Object]channelUseState, len(state.values)),
	}
	for object, value := range state.values {
		result.values[object] = value
	}
	return result
}

func mergeChannelUseFlowState(existing *channelUseFlowState, incoming channelUseFlowState) bool {
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

func (b *channelUseBuilder) transfer(state channelUseFlowState, node ast.Node) bool {
	for _, object := range b.objects {
		if _, acquired := b.acquisitions[node][object]; acquired {
			state.values[object] = channelUseOpen
			continue
		}
		b.transferObject(state, node, object)
	}
	return true
}

func (b *channelUseBuilder) transferObject(
	state channelUseFlowState,
	node ast.Node,
	object types.Object,
) {
	value := state.values[object]
	if deferred, ok := node.(*ast.DeferStmt);
		ok && expressionUsesObject(b.ctx.Info(), deferred.Call, object) {
		return
	}
	if asynchronous, ok := node.(*ast.GoStmt);
		ok && expressionUsesObject(b.ctx.Info(), asynchronous.Call, object) {
		state.values[object] = channelUseUnknown
		return
	}
	operations := b.operations(node, object)
	if len(operations) > 1 {
		state.values[object] = channelUseUnknown
		return
	}
	if len(operations) == 1 {
		operation := operations[0]
		switch operation.kind {
		case channelUseOperationClose:
			if value == channelUseClosed {
				b.addIssue(channelCloseAfterClose, object, operation.node)
			}
			if value != channelUseUntracked {
				value = channelUseClosed
			} else {
				value = channelUseUnknown
			}
		case channelUseOperationSend, channelUseOperationSendAndEscape:
			if value == channelUseClosed {
				b.addIssue(channelSendAfterClose, object, operation.node)
			}
			if operation.kind == channelUseOperationSendAndEscape {
				value = channelUseUnknown
			}
		case channelUseOperationUnknown:
			value = channelUseUnknown
		}
	}
	state.values[object] = value
}

func (b *channelUseBuilder) operations(node ast.Node, object types.Object) []channelUseOperation {
	result := make([]channelUseOperation, 0, 1)
	root := node
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, stack []ast.Node) bool {
			if literal, nested := current.(*ast.FuncLit); nested && literal != root {
				if expressionUsesObject(b.ctx.Info(), literal.Body, object) {
					result = append(
						result,
						channelUseOperation{
							kind: channelUseOperationUnknown,
							node: literal,
						},
					)
				}
				return false
			}
			switch current := current.(type) {
			case *ast.SendStmt:
				if directObject(b.ctx.Info(), current.Chan) == object {
					kind := channelUseOperationSend
					if expressionMayAliasChannel(
						b.ctx.Info(),
						current.Value,
						object,
					) {
						kind = channelUseOperationSendAndEscape
					}
					result = append(
						result,
						channelUseOperation{kind: kind, node: current},
					)
				} else if expressionMayAliasChannel(
					b.ctx.Info(),
					current.Value,
					object,
				) {
					result = append(
						result,
						channelUseOperation{
							kind: channelUseOperationUnknown,
							node: current,
						},
					)
				}
			case *ast.CallExpr:
				if exactBuiltinCloseCall(b.ctx.Info(), current, object) {
					result = append(
						result,
						channelUseOperation{
							kind: channelUseOperationClose,
							node: current,
						},
					)
					return true
				}
				if channelObserverBuiltin(b.ctx.Info(), current) {
					return true
				}
				if expressionUsesObject(b.ctx.Info(), current, object) {
					result = append(
						result,
						channelUseOperation{
							kind: channelUseOperationUnknown,
							node: current,
						},
					)
					return false
				}
			case *ast.AssignStmt:
				if channelAssignmentAliasesObject(b.ctx.Info(), current, object) {
					result = append(
						result,
						channelUseOperation{
							kind: channelUseOperationUnknown,
							node: current,
						},
					)
				}
			case *ast.ValueSpec:
				if channelValueSpecAliasesObject(b.ctx.Info(), current, object) {
					result = append(
						result,
						channelUseOperation{
							kind: channelUseOperationUnknown,
							node: current,
						},
					)
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if expressionMayAliasChannel(
						b.ctx.Info(),
						expression,
						object,
					) {
						result = append(
							result,
							channelUseOperation{
								kind: channelUseOperationUnknown,
								node: current,
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

func exactBuiltinCloseCall(info *types.Info, call *ast.CallExpr, object types.Object) bool {
	if call == nil || len(call.Args) != 1 || directObject(info, call.Args[0]) != object {
		return false
	}
	identifier, _ := ast.Unparen(call.Fun).(*ast.Ident)
	return identifier != nil && info.ObjectOf(identifier) == types.Universe.Lookup("close")
}

func channelObserverBuiltin(info *types.Info, call *ast.CallExpr) bool {
	identifier, _ := ast.Unparen(call.Fun).(*ast.Ident)
	if identifier == nil {
		return false
	}
	object, _ := info.ObjectOf(identifier).(*types.Builtin)
	if object == nil {
		return false
	}
	switch object.Name() {
	case "cap", "len", "print", "println":
		return true
	default:
		return false
	}
}

func channelAssignmentAliasesObject(
	info *types.Info,
	assignment *ast.AssignStmt,
	object types.Object,
) bool {
	for _, target := range assignment.Lhs {
		if directObject(info, target) == object {
			return true
		}
	}
	for index, expression := range assignment.Rhs {
		if !expressionMayAliasChannel(info, expression, object) {
			continue
		}
		if index >= len(assignment.Lhs) {
			return true
		}
		identifier, blank := ast.Unparen(assignment.Lhs[index]).(*ast.Ident)
		if !blank || identifier.Name != "_" {
			return true
		}
	}
	return false
}

func channelValueSpecAliasesObject(
	info *types.Info,
	value *ast.ValueSpec,
	object types.Object,
) bool {
	if value == nil {
		return false
	}
	for index, expression := range value.Values {
		if !expressionMayAliasChannel(info, expression, object) {
			continue
		}
		if index >= len(value.Names) || value.Names[index].Name != "_" {
			return true
		}
	}
	return false
}

func expressionMayAliasChannel(info *types.Info, expression ast.Expr, object types.Object) bool {
	expression = ast.Unparen(expression)
	if directObject(info, expression) == object {
		return true
	}
	switch expression := expression.(type) {
	case *ast.CompositeLit:
		return expressionUsesObject(info, expression, object)
	case *ast.UnaryExpr:
		return expression.Op == token.AND &&
			expressionUsesObject(info, expression.X, object)
	case *ast.CallExpr:
		return info.Types[expression.Fun].IsType() &&
			expressionUsesObject(info, expression, object)
	default:
		return false
	}
}

func collectChannelCloseCalls(
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
				if exactBuiltinCloseCall(ctx.Info(), call, object) {
					result[object] = append(result[object], call)
				}
			}
			return true
		},
	)
	return result
}

func (b *channelUseBuilder) unambiguousClose(object types.Object, before token.Pos) *ast.CallExpr {
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

func (b *channelUseBuilder) addIssue(kind channelUseIssueKind, object types.Object, node ast.Node) {
	if !b.record || node == nil || !node.Pos().IsValid() {
		return
	}
	if _, found := b.issues[node.Pos()]; found {
		return
	}
	b.issues[node.Pos()] = channelUseIssue{
		kind: kind,
		object: object,
		node: node,
		close: b.unambiguousClose(object, node.Pos()),
	}
}
