package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

const netHTTPPackagePath = "net/http"

type httpResponseBodyNotClosedRule struct{}

type httpResponseCandidate struct {
	identifier *ast.Ident
	object types.Object
	start obligationStart
}

// NewHTTPResponseBodyNotClosedRule constructs the local HTTP response-body
// ownership rule for product registry composition.
func NewHTTPResponseBodyNotClosedRule() Rule {
	return httpResponseBodyNotClosedRule{}
}

func (httpResponseBodyNotClosedRule) Metadata() Metadata {
	return Metadata{
		ID: "http-response-body-not-closed",
		Summary: "detects HTTP response bodies not closed on every normal return",
		Documentation: "A successful net/http client request returns a response whose Body must be closed. A locally owned response that reaches a normal return without closing or conservatively transferring its body can leak connections and prevent transport reuse. This rule follows direct package and Client request helpers through the shared control-flow graph after a conventional acquisition-error guard.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract recognizes direct net/http Get, Head, Post, and PostForm functions plus Client.Do, Get, Head, Post, and PostForm methods followed immediately by an err != nil guard whose body returns.",
			"Returning, passing, sending, storing, or capturing the response transfers ownership. A body argument transfers ownership only when the destination parameter itself has Close() error; passing Body as an io.Reader does not discharge the obligation.",
			"The rule is intraprocedural and does not infer cleanup performed by arbitrary helper functions, response wrappers, or middleware.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Close a successful HTTP response body",
				Incorrect: "response, err := http.Get(url)\nif err != nil { return err }\nreturn use(response.Body)",
				Correct: "response, err := http.Get(url)\nif err != nil { return err }\ndefer response.Body.Close()\nreturn use(response.Body)",
			},
		},
	}
}

func (httpResponseBodyNotClosedRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil ||
		ctx.Body() == nil ||
		ctx.Graph() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf(
			"http-response-body-not-closed requires a complete control-flow context",
		)
	}
	if !packageImports(ctx.Package(), netHTTPPackagePath) {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for _, candidate := range httpResponseCandidates(ctx.Body(), ctx.Graph(), ctx.Info()) {
		if !obligationReachesOpenReturn(
			candidate.start,
			func(node ast.Node) obligationEffect {
				return httpResponseObligationEffect(
					ctx.Info(),
					node,
					candidate.object,
				)
			},
		) {
			continue
		}
		range_, err := ctx.Range(candidate.identifier)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "http-response-body-not-closed",
				Message: fmt.Sprintf(
					"HTTP response %q body is not closed or transferred on every normally returning path",
					candidate.identifier.Name,
				),
				Range: range_,
				Help: "defer response.Body.Close after checking the request error or transfer ownership explicitly",
			},
		)
	}
	return findings, nil
}

func httpResponseCandidates(
	body *ast.BlockStmt,
	graph *cfg.CFG,
	info *types.Info,
) []httpResponseCandidate {
	doneBlocks := make(map[*ast.IfStmt]*cfg.Block)
	for _, block := range graph.Blocks {
		statement, ok := block.Stmt.(*ast.IfStmt)
		if block.Live && ok && block.Kind == cfg.KindIfDone {
			doneBlocks[statement] = block
		}
	}
	result := make([]httpResponseCandidate, 0)
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			block, ok := node.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for index := 0; index + 1 < len(block.List); index++ {
				assignment, ok := block.List[index].(*ast.AssignStmt)
				if !ok {
					continue
				}
				response, object, errorObject, matched := httpResponseAssignment(
					info,
					assignment,
				)
				guard, ok := block.List[index + 1].(*ast.IfStmt)
				start := doneBlocks[guard]
				if !matched ||
					!returningNonNilErrorGuard(info, guard, errorObject) ||
					start == nil ||
					!start.Live {
					continue
				}
				result = append(
					result,
					httpResponseCandidate{
						identifier: response,
						object: object,
						start: obligationStartAt(start),
					},
				)
			}
			return true
		},
	)
	return result
}

func httpResponseAssignment(
	info *types.Info,
	assignment *ast.AssignStmt,
) (*ast.Ident, types.Object, types.Object, bool) {
	if info == nil ||
		assignment == nil ||
		len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 {
		return nil, nil, nil, false
	}
	response, _ := assignment.Lhs[0].(*ast.Ident)
	errorIdentifier, _ := assignment.Lhs[1].(*ast.Ident)
	call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if response == nil ||
		response.Name == "_" ||
		errorIdentifier == nil ||
		errorIdentifier.Name == "_" ||
		call == nil ||
		!httpResponseAcquisitionCall(info, call) ||
		!namedReceiver(info.TypeOf(response), netHTTPPackagePath, "Response") {
		return nil, nil, nil, false
	}
	responseObject := info.ObjectOf(response)
	errorObject := info.ObjectOf(errorIdentifier)
	if responseObject == nil || errorObject == nil {
		return nil, nil, nil, false
	}
	return response, responseObject, errorObject, true
}

func httpResponseAcquisitionCall(info *types.Info, call *ast.CallExpr) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	selection := info.Selections[selector]
	function, _ := selectionObject(selection).(*types.Func)
	if selection == nil {
		function, _ = info.ObjectOf(selector.Sel).(*types.Func)
		if function == nil ||
			function.Pkg() == nil ||
			function.Pkg().Path() != netHTTPPackagePath {
			return false
		}
		switch function.Name() {
		case "Get", "Head", "Post", "PostForm":
			return true
		default:
			return false
		}
	}
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != netHTTPPackagePath ||
		!namedReceiver(selection.Recv(), netHTTPPackagePath, "Client") {
		return false
	}
	switch function.Name() {
	case "Do", "Get", "Head", "Post", "PostForm":
		return true
	default:
		return false
	}
}

func httpResponseObligationEffect(
	info *types.Info,
	node ast.Node,
	response types.Object,
) obligationEffect {
	if nodeContainsResponseBodyClose(info, node, response) {
		return obligationCompleted
	}
	if nodeTransfersResponseBody(info, node, response) {
		return obligationTransferred
	}
	return objectObligationEffect(info, node, response, nil)
}

func nodeContainsResponseBodyClose(info *types.Info, node ast.Node, response types.Object) bool {
	closed := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if closed {
				return false
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			call, ok := current.(*ast.CallExpr)
			if ok && responseBodyCloseCall(info, call, response) {
				closed = true
				return false
			}
			return true
		},
	)
	return closed
}

func responseBodyCloseCall(info *types.Info, call *ast.CallExpr, response types.Object) bool {
	if call == nil || len(call.Args) != 0 {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil || selector.Sel.Name != "Close" {
		return false
	}
	function, _ := selectionObject(info.Selections[selector]).(*types.Func)
	return function != nil &&
		function.Name() == "Close" &&
		directResponseBody(info, selector.X, response)
}

func nodeTransfersResponseBody(info *types.Info, node ast.Node, response types.Object) bool {
	transferred := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if transferred {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				if expressionUsesObject(info, literal.Body, response) {
					transferred = true
				}
				return false
			}
			switch current := current.(type) {
			case *ast.CallExpr:
				if callTransfersResponseBody(info, current, response) {
					transferred = true
					return false
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directResponseBody(info, expression, response) {
						transferred = true
						return false
					}
				}
			case *ast.SendStmt:
				if directResponseBody(info, current.Value, response) {
					transferred = true
					return false
				}
			case *ast.AssignStmt:
				for _, expression := range current.Rhs {
					if directResponseBody(info, expression, response) ||
						responseBodyMethodValue(
							info,
							expression,
							response,
						) {
						transferred = true
						return false
					}
				}
			case *ast.CompositeLit:
				for _, element := range current.Elts {
					if compositeElementUsesResponseBody(
						info,
						element,
						response,
					) {
						transferred = true
						return false
					}
				}
			}
			return true
		},
	)
	return transferred
}

func callTransfersResponseBody(info *types.Info, call *ast.CallExpr, response types.Object) bool {
	signature, _ := types.Unalias(info.TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || signature.Params() == nil {
		return false
	}
	for index, argument := range call.Args {
		if !directResponseBody(info, argument, response) {
			continue
		}
		parameterIndex := index
		if signature.Variadic() && parameterIndex >= signature.Params().Len() - 1 {
			parameterIndex = signature.Params().Len() - 1
		}
		if parameterIndex < 0 || parameterIndex >= signature.Params().Len() {
			continue
		}
		parameterType := signature.Params().At(parameterIndex).Type()
		if signature.Variadic() && parameterIndex == signature.Params().Len() - 1 {
			slice, _ := types.Unalias(parameterType).(*types.Slice)
			if slice != nil {
				parameterType = slice.Elem()
			}
		}
		if conventionalCloser(parameterType) {
			return true
		}
	}
	return false
}

func directResponseBody(info *types.Info, expression ast.Expr, response types.Object) bool {
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	if selector == nil ||
		selector.Sel.Name != "Body" ||
		directObject(info, selector.X) != response {
		return false
	}
	field, _ := selectionObject(info.Selections[selector]).(*types.Var)
	return field != nil &&
		field.IsField() &&
		field.Pkg() != nil &&
		field.Pkg().Path() == netHTTPPackagePath &&
		field.Name() == "Body"
}

func responseBodyMethodValue(info *types.Info, expression ast.Expr, response types.Object) bool {
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	return selector != nil && directResponseBody(info, selector.X, response)
}

func compositeElementUsesResponseBody(
	info *types.Info,
	element ast.Expr,
	response types.Object,
) bool {
	if keyed, ok := element.(*ast.KeyValueExpr); ok {
		element = keyed.Value
	}
	return directResponseBody(info, element, response)
}
