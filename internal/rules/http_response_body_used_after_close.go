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
	ioPackagePath = "io"
	maxTrackedFunctionHTTPResponseBodies = 4096
)

type httpResponseBodyUsedAfterCloseRule struct{}

type httpResponseBodyState uint8

const (
	httpResponseBodyOpen httpResponseBodyState = 1 << iota
	httpResponseBodyClosed
	httpResponseBodyUnknown
)

type httpResponseBodyFlowState struct {
	value *httpResponseBodyState
}

type httpResponseBodyStateIssue struct {
	object types.Object
	call *ast.CallExpr
	close *ast.CallExpr
}

type httpResponseBodyStateAnalysis struct {
	complete bool
	issues []httpResponseBodyStateIssue
}

type httpResponseBodyStateBuilder struct {
	ctx *ControlFlowContext
	candidate httpResponseCandidate
	closeCalls []*ast.CallExpr
	issues map[token.Pos]httpResponseBodyStateIssue
	record bool
}

type httpResponseBodyCallKind uint8

const (
	httpResponseBodyCallClose httpResponseBodyCallKind = iota
	httpResponseBodyCallOperation
	httpResponseBodyCallBorrow
	httpResponseBodyCallUnknown
)

type httpResponseBodyCallEffect struct {
	kind httpResponseBodyCallKind
	call *ast.CallExpr
}

// NewHTTPResponseBodyUsedAfterCloseRule constructs the HTTP response-body
// state rule for product registry composition.
func NewHTTPResponseBodyUsedAfterCloseRule() Rule {
	return httpResponseBodyUsedAfterCloseRule{}
}

func (httpResponseBodyUsedAfterCloseRule) Metadata() Metadata {
	return Metadata{
		ID: "http-response-body-used-after-close",
		Summary: "detects operations on definitely closed HTTP response bodies",
		Documentation: "Reading or closing a net/http response body after it is definitely closed usually returns an unusable result and can conceal lifecycle mistakes. The rule follows direct package and Client acquisitions through bounded control flow, consumes proven project close effects, and reports only when every reaching state is closed.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"The acquisition boundary matches http-response-body-not-closed: direct net/http package or Client helpers followed immediately by a returning err != nil guard.",
			"The rule recognizes direct Body.Read and Body.Close calls plus exact io.ReadAll, ReadAtLeast, ReadFull, Copy, CopyN, and CopyBuffer reader arguments.",
			"A finding requires every reaching path to have closed the body; conditional closure, aliases, reassignment, ownership transfer, asynchronous or deferred execution, and unknown helpers become conservative unknown state.",
			"A statically resolved helper with a proven close effect establishes closed state, while a proven borrow preserves state and a proven transfer stops tracking.",
			"A CFG node containing multiple tracked calls becomes unknown because AST preorder does not prove every nested Go evaluation order.",
			"The rule remains suspicious because a custom RoundTripper can provide an io.ReadCloser with implementation-specific post-close behavior.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Read the body before closing it",
				Incorrect: "response, err := http.Get(url)\nif err != nil { return err }\nresponse.Body.Close()\n_, err = io.ReadAll(response.Body)",
				Correct: "response, err := http.Get(url)\nif err != nil { return err }\n_, err = io.ReadAll(response.Body)\nresponse.Body.Close()",
			},
		},
	}
}

func (httpResponseBodyUsedAfterCloseRule) RunControlFlow(
	ctx *ControlFlowContext,
) ([]Finding, error) {
	if ctx == nil ||
		ctx.Body() == nil ||
		ctx.Graph() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf(
			"http-response-body-used-after-close requires a complete control-flow context",
		)
	}
	if !packageImports(ctx.Package(), netHTTPPackagePath) {
		return nil, nil
	}
	analysis := httpResponseBodyStateAnalysisFor(ctx)
	if analysis == nil || !analysis.complete {
		return nil, nil
	}
	findings := make([]Finding, 0, len(analysis.issues))
	for _, issue := range analysis.issues {
		range_, err := ctx.Range(issue.call)
		if err != nil {
			return nil, err
		}
		finding := Finding{
			MessageKey: "http-response-body-used-after-close",
			Message: fmt.Sprintf(
				"HTTP response %q body is used after it is closed",
				issue.object.Name(),
			),
			Range: range_,
			Help: "move the operation before Body.Close or issue a new request",
		}
		if issue.close != nil {
			closeRange, rangeErr := ctx.Range(issue.close)
			if rangeErr != nil {
				return nil, rangeErr
			}
			finding.Related = []Related{
				{Range: closeRange, Message: "response body closed here"},
			}
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func httpResponseBodyStateAnalysisFor(ctx *ControlFlowContext) *httpResponseBodyStateAnalysis {
	if ctx.shared == nil {
		return buildHTTPResponseBodyStateAnalysis(ctx)
	}
	ctx.shared.httpResponseBodyStateOnce.Do(
		func() {
			ctx.shared.httpResponseBodyState = buildHTTPResponseBodyStateAnalysis(ctx)
		},
	)
	return ctx.shared.httpResponseBodyState
}

func buildHTTPResponseBodyStateAnalysis(ctx *ControlFlowContext) *httpResponseBodyStateAnalysis {
	candidates := httpResponseCandidates(ctx.Body(), ctx.Graph(), ctx.Info())
	if len(candidates) == 0 {
		return &httpResponseBodyStateAnalysis{complete: true}
	}
	if len(candidates) > maxTrackedFunctionHTTPResponseBodies {
		return &httpResponseBodyStateAnalysis{}
	}
	issues := make(map[token.Pos]httpResponseBodyStateIssue)
	for _, candidate := range candidates {
		builder := &httpResponseBodyStateBuilder{
			ctx: ctx,
			candidate: candidate,
			closeCalls: collectHTTPResponseBodyCloseCalls(ctx, candidate.object),
			issues: issues,
		}
		changeBound := len(ctx.Graph().Blocks) * 8
		if changeBound <= 0 || changeBound > maxStateTransitionChanges {
			changeBound = maxStateTransitionChanges
		}
		initial := httpResponseBodyOpen
		snapshot, complete := runStateTransitions(
			ctx.Graph(),
			stateTransitionModel[httpResponseBodyFlowState]{
				Initial: httpResponseBodyFlowState{value: &initial},
				Entry: candidate.start.block,
				Clone: func(
					state httpResponseBodyFlowState,
				) httpResponseBodyFlowState {
					value := *state.value
					return httpResponseBodyFlowState{value: &value}
				},
				Merge: func(
					existing *httpResponseBodyFlowState,
					incoming httpResponseBodyFlowState,
				) bool {
					merged := *existing.value | *incoming.value
					if merged == *existing.value {
						return false
					}
					*existing.value = merged
					return true
				},
				Transfer: builder.transfer,
				MaxChanges: changeBound,
			},
		)
		if !complete {
			return &httpResponseBodyStateAnalysis{}
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
			state := snapshot.entries[block.Index]
			for _, node := range block.Nodes {
				builder.transfer(state, node)
			}
		}
	}
	result := make([]httpResponseBodyStateIssue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, issue)
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			return result[left].call.Pos() < result[right].call.Pos()
		},
	)
	return &httpResponseBodyStateAnalysis{complete: true, issues: result}
}

func (b *httpResponseBodyStateBuilder) transfer(
	flow httpResponseBodyFlowState,
	node ast.Node,
) bool {
	state := *flow.value
	if deferredOrAsynchronousResponseBodyUse(b.ctx.Info(), node, b.candidate.object) {
		*flow.value = httpResponseBodyUnknown
		return true
	}
	effects := b.callEffects(node)
	if len(effects) > 1 {
		*flow.value = httpResponseBodyUnknown
		return true
	}
	if len(effects) == 1 {
		effect := effects[0]
		switch effect.kind {
		case httpResponseBodyCallClose:
			if state == httpResponseBodyClosed {
				b.addIssue(effect.call)
			}
			state = httpResponseBodyClosed
		case httpResponseBodyCallOperation:
			if state == httpResponseBodyClosed {
				b.addIssue(effect.call)
			}
		case httpResponseBodyCallBorrow:
		case httpResponseBodyCallUnknown:
			state = httpResponseBodyUnknown
		}
		*flow.value = state
		return true
	}
	if responseBodyEscapesInNode(b.ctx.Info(), node, b.candidate.object) ||
		responseReassignedInNode(b.ctx.Info(), node, b.candidate.object) {
		state = httpResponseBodyUnknown
	}
	effect := objectObligationEffect(b.ctx.Info(), node, b.candidate.object, nil, nil, 0)
	if effect == obligationTransferred || effect == obligationLost {
		state = httpResponseBodyUnknown
	}
	*flow.value = state
	return true
}

func (b *httpResponseBodyStateBuilder) callEffects(node ast.Node) []httpResponseBodyCallEffect {
	result := make([]httpResponseBodyCallEffect, 0, 1)
	for _, call := range callsInLockNode(node) {
		if method := httpResponseBodyMethodName(b.ctx.Info(), call, b.candidate.object);
			method != "" {
			kind := httpResponseBodyCallOperation
			if method == "Close" {
				kind = httpResponseBodyCallClose
			}
			result = append(result, httpResponseBodyCallEffect{kind: kind, call: call})
			continue
		}
		if httpResponseBodyIOConsumer(b.ctx.Info(), call, b.candidate.object) {
			result = append(
				result,
				httpResponseBodyCallEffect{
					kind: httpResponseBodyCallOperation,
					call: call,
				},
			)
			continue
		}
		for index, argument := range call.Args {
			if directResponseBody(b.ctx.Info(), argument, b.candidate.object) {
				kind := httpResponseBodyCallUnknown
				summary := b.ctx.ParameterEffect(call, index)
				switch {
				case summary.GuaranteesAny(ParameterEffectClose):
					kind = httpResponseBodyCallClose
				case summary.Known && summary.Kinds == 0:
					kind = httpResponseBodyCallBorrow
				}
				result = append(
					result,
					httpResponseBodyCallEffect{kind: kind, call: call},
				)
				continue
			}
			if expressionUsesResponseBody(b.ctx.Info(), argument, b.candidate.object) {
				result = append(
					result,
					httpResponseBodyCallEffect{
						kind: httpResponseBodyCallUnknown,
						call: call,
					},
				)
			}
		}
	}
	return result
}

func httpResponseBodyMethodName(
	info *types.Info,
	call *ast.CallExpr,
	response types.Object,
) string {
	if responseBodyCloseCall(info, call, response) {
		return "Close"
	}
	if call == nil || len(call.Args) != 1 {
		return ""
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil ||
		selector.Sel.Name != "Read" ||
		!directResponseBody(info, selector.X, response) {
		return ""
	}
	function, _ := selectionObject(info.Selections[selector]).(*types.Func)
	if function == nil || function.Name() != "Read" {
		return ""
	}
	return function.Name()
}

func httpResponseBodyIOConsumer(info *types.Info, call *ast.CallExpr, response types.Object) bool {
	if info == nil || call == nil {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != ioPackagePath {
		return false
	}
	reader := -1
	switch function.Name() {
	case "ReadAll", "ReadAtLeast", "ReadFull":
		reader = 0
	case "Copy", "CopyN", "CopyBuffer":
		reader = 1
	}
	return reader >= 0 &&
		reader < len(call.Args) &&
		directResponseBody(info, call.Args[reader], response) &&
		!httpResponseBodyIOCallSkipsRead(info, function.Name(), call)
}

func httpResponseBodyIOCallSkipsRead(info *types.Info, name string, call *ast.CallExpr) bool {
	switch name {
	case "ReadAtLeast":
		if len(call.Args) != 3 {
			return false
		}
		minimum := info.Types[call.Args[2]].Value
		return minimum != nil &&
			minimum.Kind() == constant.Int &&
			constant.Sign(minimum) <= 0 ||
			knownEmptySlice(info, call.Args[1])
	case "ReadFull":
		return len(call.Args) == 2 && knownEmptySlice(info, call.Args[1])
	case "CopyN":
		if len(call.Args) != 3 {
			return false
		}
		count := info.Types[call.Args[2]].Value
		return count != nil && count.Kind() == constant.Int && constant.Sign(count) <= 0
	default:
		return false
	}
}

func knownEmptySlice(info *types.Info, expression ast.Expr) bool {
	expression = ast.Unparen(expression)
	identifier, _ := expression.(*ast.Ident)
	if identifier != nil && info.ObjectOf(identifier) == types.Universe.Lookup("nil") {
		return true
	}
	literal, _ := expression.(*ast.CompositeLit)
	if literal != nil {
		_, slice := types.Unalias(info.TypeOf(literal)).(*types.Slice)
		return slice && len(literal.Elts) == 0
	}
	call, _ := expression.(*ast.CallExpr)
	if call == nil || len(call.Args) < 2 {
		return false
	}
	builtin, _ := ast.Unparen(call.Fun).(*ast.Ident)
	if builtin == nil || info.ObjectOf(builtin) != types.Universe.Lookup("make") {
		return false
	}
	length := info.Types[call.Args[1]].Value
	return length != nil && length.Kind() == constant.Int && constant.Sign(length) == 0
}

func deferredOrAsynchronousResponseBodyUse(
	info *types.Info,
	node ast.Node,
	response types.Object,
) bool {
	var call *ast.CallExpr
	switch statement := node.(type) {
	case *ast.DeferStmt:
		call = statement.Call
	case *ast.GoStmt:
		call = statement.Call
	}
	return call != nil && expressionUsesResponseBody(info, call, response)
}

func expressionUsesResponseBody(info *types.Info, node ast.Node, response types.Object) bool {
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			expression, ok := current.(ast.Expr)
			if ok &&
				(directResponseBody(info, expression, response) ||
					responseBodyMethodValue(info, expression, response)) {
				found = true
				return false
			}
			return !found
		},
	)
	return found
}

func responseBodyEscapesInNode(info *types.Info, node ast.Node, response types.Object) bool {
	if !expressionUsesResponseBody(info, node, response) {
		return false
	}
	switch current := node.(type) {
	case *ast.AssignStmt:
		for _, target := range current.Lhs {
			identifier, blank := target.(*ast.Ident)
			if !blank || identifier.Name != "_" {
				return true
			}
		}
		return false
	case *ast.ReturnStmt, *ast.SendStmt, *ast.CompositeLit:
		return true
	default:
		return true
	}
}

func responseReassignedInNode(info *types.Info, node ast.Node, response types.Object) bool {
	assignment, _ := node.(*ast.AssignStmt)
	if assignment == nil {
		return false
	}
	for _, target := range assignment.Lhs {
		if directObject(info, target) == response {
			return true
		}
	}
	return false
}

func collectHTTPResponseBodyCloseCalls(
	ctx *ControlFlowContext,
	response types.Object,
) []*ast.CallExpr {
	result := make([]*ast.CallExpr, 0)
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
			if responseBodyCloseCall(ctx.Info(), call, response) ||
				helperGuaranteesHTTPResponseBodyClose(ctx, call, response) {
				result = append(result, call)
			}
			return true
		},
	)
	return result
}

func helperGuaranteesHTTPResponseBodyClose(
	ctx *ControlFlowContext,
	call *ast.CallExpr,
	response types.Object,
) bool {
	for index, argument := range call.Args {
		if directResponseBody(ctx.Info(), argument, response) &&
			ctx.ParameterEffect(call, index).GuaranteesAny(ParameterEffectClose) {
			return true
		}
	}
	return false
}

func (b *httpResponseBodyStateBuilder) unambiguousClose(before token.Pos) *ast.CallExpr {
	var result *ast.CallExpr
	for _, call := range b.closeCalls {
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

func (b *httpResponseBodyStateBuilder) addIssue(call *ast.CallExpr) {
	if !b.record || call == nil || !call.Pos().IsValid() {
		return
	}
	if _, found := b.issues[call.Pos()]; found {
		return
	}
	b.issues[call.Pos()] = httpResponseBodyStateIssue{
		object: b.candidate.object,
		call: call,
		close: b.unambiguousClose(call.Pos()),
	}
}
