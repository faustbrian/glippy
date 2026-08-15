package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type resourceNotClosedRule struct{}

type localCloserCandidate struct {
	identifier *ast.Ident
	object types.Object
	statement *ast.AssignStmt
	guard *ast.IfStmt
}

// NewResourceNotClosedRule constructs the local closer-ownership rule for
// product registry composition.
func NewResourceNotClosedRule() Rule {
	return resourceNotClosedRule{}
}

func (resourceNotClosedRule) Metadata() Metadata {
	return Metadata{
		ID: "resource-not-closed",
		Summary: "detects locally owned closers that are neither closed nor transferred",
		Documentation: "A call result with a conventional Close method usually owns a file, connection, compressor, or similar resource. A locally owned result that reaches a normal return without being closed or transferred can retain descriptors, connections, buffers, or other external state until process termination or garbage collection.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract treats a direct argument, return, send, composite-literal insertion, closure capture, or assignment to another variable as an ownership transfer and does not analyze the callee.",
			"Cleanup and ownership transfer must cover every normally returning path after a conventional acquisition guard when one is present.",
			"Only call results whose static type has Close() error are considered resources; zero-result Close methods are too broad for the initial ownership contract.",
		},
		Examples: []Example{
			{
				Title: "Close an opened resource after the error check",
				Incorrect: "file, err := os.Open(path)\nif err != nil { return err }\nuse(file)",
				Correct: "file, err := os.Open(path)\nif err != nil { return err }\ndefer file.Close()",
			},
		},
	}
}

func (resourceNotClosedRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"resource-not-closed requires a complete control-flow context",
		)
	}
	findings := make([]Finding, 0)
	for _, candidate := range localCloserCandidates(ctx.Info(), ctx.Body()) {
		start, found := localCloserObligationStart(ctx.Graph(), ctx.Info(), candidate)
		if !found ||
			!obligationReachesOpenReturn(
				start,
				func(node ast.Node) obligationEffect {
					return objectObligationEffect(
						ctx.Info(),
						node,
						candidate.object,
						func(call *ast.CallExpr) bool {
							return closeCallUsesObject(
								ctx.Info(),
								call,
								candidate.object,
							)
						},
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
				MessageKey: "resource-not-closed",
				Message: fmt.Sprintf(
					"resource %q is not closed or transferred on every normally returning path",
					candidate.identifier.Name,
				),
				Range: range_,
				Help: "close the resource after checking the constructor error or transfer ownership explicitly",
			},
		)
	}
	return findings, nil
}

func localCloserCandidates(info *types.Info, body *ast.BlockStmt) []localCloserCandidate {
	result := make([]localCloserCandidate, 0)
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
			for statementIndex, statement := range block.List {
				assignment, ok := statement.(*ast.AssignStmt)
				if !ok || len(assignment.Rhs) != 1 {
					continue
				}
				call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
				if call == nil {
					continue
				}
				signature, _ := types.Unalias(
					info.TypeOf(call.Fun),
				).(*types.Signature)
				if signature == nil ||
					signature.Results() == nil ||
					signature.Results().Len() != len(assignment.Lhs) {
					continue
				}
				var guard *ast.IfStmt
				if statementIndex + 1 < len(block.List) {
					guard, _ = block.List[statementIndex + 1].(*ast.IfStmt)
				}
				for index, left := range assignment.Lhs {
					identifier, _ := left.(*ast.Ident)
					if identifier == nil ||
						identifier.Name == "_" ||
						!conventionalCloser(
							signature.Results().At(index).Type(),
						) {
						continue
					}
					object := info.ObjectOf(identifier)
					if object != nil {
						result = append(
							result,
							localCloserCandidate{
								identifier: identifier,
								object: object,
								statement: assignment,
								guard: guard,
							},
						)
					}
				}
			}
			return true
		},
	)
	return result
}

func localCloserObligationStart(
	graph *cfg.CFG,
	info *types.Info,
	candidate localCloserCandidate,
) (obligationStart, bool) {
	if errorObject := assignmentErrorObject(info, candidate.statement);
		errorObject != nil &&
			returningNonNilErrorGuard(info, candidate.guard, errorObject) {
		for _, block := range graph.Blocks {
			if block.Live &&
				block.Kind == cfg.KindIfDone &&
				block.Stmt == candidate.guard {
				return obligationStartAt(block), true
			}
		}
	}
	return obligationStartAfter(graph, candidate.statement)
}

func assignmentErrorObject(info *types.Info, assignment *ast.AssignStmt) types.Object {
	if info == nil || assignment == nil || len(assignment.Rhs) != 1 {
		return nil
	}
	call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if call == nil {
		return nil
	}
	signature, _ := types.Unalias(info.TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || signature.Results().Len() != len(assignment.Lhs) {
		return nil
	}
	for index, left := range assignment.Lhs {
		identifier, _ := left.(*ast.Ident)
		if identifier != nil &&
			identifier.Name != "_" &&
			isErrorType(signature.Results().At(index).Type()) {
			return info.ObjectOf(identifier)
		}
	}
	return nil
}

func conventionalCloser(type_ types.Type) bool {
	object, _, _ := types.LookupFieldOrMethod(type_, true, nil, "Close")
	method, _ := object.(*types.Func)
	if method == nil {
		return false
	}
	signature, _ := method.Type().(*types.Signature)
	if signature == nil || signature.Params().Len() != 0 {
		return false
	}
	results := signature.Results()
	return results.Len() == 1 && isErrorType(results.At(0).Type())
}

func isErrorType(type_ types.Type) bool {
	errorInterface, _ := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return errorInterface != nil && types.Implements(type_, errorInterface)
}

func closeCallUsesObject(info *types.Info, call *ast.CallExpr, object types.Object) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	return selector != nil &&
		selector.Sel.Name == "Close" &&
		directObject(info, selector.X) == object
}

func methodValueUsesObject(info *types.Info, expression ast.Expr, object types.Object) bool {
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	selection := info.Selections[selector]
	return selector != nil &&
		selection != nil &&
		selection.Kind() == types.MethodVal &&
		directObject(info, selector.X) == object
}

func directObject(info *types.Info, expression ast.Expr) types.Object {
	identifier, _ := ast.Unparen(expression).(*ast.Ident)
	if identifier == nil {
		return nil
	}
	return info.ObjectOf(identifier)
}

func expressionUsesObject(info *types.Info, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			identifier, ok := current.(*ast.Ident)
			if ok && info.ObjectOf(identifier) == object {
				found = true
				return false
			}
			return !found
		},
	)
	return found
}
