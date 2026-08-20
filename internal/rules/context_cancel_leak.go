package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

const contextPackagePath = "context"

type contextCancelLeakRule struct{}

type contextCancelCandidate struct {
	variable *types.Var
	statement ast.Node
}

// NewContextCancelLeakRule constructs the native rule for product registry
// composition without changing the low-level native default registry.
func NewContextCancelLeakRule() Rule {
	return contextCancelLeakRule{}
}

func (contextCancelLeakRule) Metadata() Metadata {
	return Metadata{
		ID: "context-cancel-leak",
		Summary: "detects context cancellation functions that may not run",
		Documentation: "The cancellation function returned by context.WithCancel, context.WithTimeout, or context.WithDeadline releases resources owned by the derived context. Discarding it or returning along a path that never invokes or transfers it can retain timers, children, and references longer than intended. Glippy implements the standard lostcancel contract over its shared control-flow tier and consumes versioned no-return and parameter-effect summaries for imported helpers in the selected modules.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"A statically resolved same-module helper that provably borrows the cancellation function does not discharge the obligation; guaranteed invocation or ownership transfer must cover every normally returning helper path.",
			"An exact returned-alias contract preserves the obligation when the result is assigned back to the same cancellation variable; new alias bindings remain outside the tracked ownership identity.",
			"Dynamic calls, interface dispatch, recursion, local aliases, and helpers outside selected modules retain the conservative use-or-transfer behavior when no summary is available.",
			"The shared CFG propagates no-return behavior through the selected package and same-module imported helpers. Third-party helpers outside the selected modules remain conservatively returning unless they match an exact standard-library terminal API.",
		},
		Examples: []Example{
			{
				Title: "Retain and call the cancellation function",
				Incorrect: "child, _ := context.WithCancel(parent)",
				Correct: "child, cancel := context.WithCancel(parent)\ndefer cancel()",
			},
		},
	}
}

func (contextCancelLeakRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil ||
		ctx.Function() == nil ||
		ctx.Graph() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf(
			"context-cancel-leak requires a complete control-flow context",
		)
	}
	if !packageImports(ctx.Package(), contextPackagePath) {
		return nil, nil
	}

	signature, scope := contextFunctionSignatureAndScope(ctx)
	if signature == nil || scope == nil || isMainFunction(ctx, signature) {
		return nil, nil
	}

	candidates, findings, err := contextCancelCandidates(ctx, scope)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		if contextCancelCapturedByNestedFunction(
			ctx.Function(),
			ctx.Info(),
			candidate.variable,
		) {
			continue
		}
		if contextCancelPath(ctx, candidate, signature) == nil {
			continue
		}
		range_, err := ctx.Range(candidate.statement)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "cancel-not-used-on-all-paths",
				Message: fmt.Sprintf(
					"the %s function is not used on all paths (possible context leak)",
					candidate.variable.Name(),
				),
				Range: range_,
				Help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel",
			},
		)
	}
	return findings, nil
}

func contextCancelCapturedByNestedFunction(
	function ast.Node,
	info *types.Info,
	variable *types.Var,
) bool {
	captured := false
	ast.PreorderStack(
		function,
		nil,
		func(node ast.Node, stack []ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || info.Uses[identifier] != variable {
				return true
			}
			for _, ancestor := range stack {
				literal, nested := ancestor.(*ast.FuncLit)
				if nested && literal != function {
					captured = true
					return false
				}
			}
			return true
		},
	)
	return captured
}

func packageImports(package_ *types.Package, path string) bool {
	for _, imported := range package_.Imports() {
		if imported.Path() == path {
			return true
		}
	}
	return false
}

func contextFunctionSignatureAndScope(ctx *ControlFlowContext) (*types.Signature, *types.Scope) {
	info := ctx.Info()
	switch function := ctx.Function().(type) {
	case *ast.FuncDecl:
		object, _ := info.Defs[function.Name].(*types.Func)
		if object == nil {
			return nil, nil
		}
		signature, _ := object.Type().(*types.Signature)
		return signature, info.Scopes[function.Type]
	case *ast.FuncLit:
		typeAndValue, found := info.Types[function.Type]
		if !found {
			return nil, nil
		}
		signature, _ := typeAndValue.Type.(*types.Signature)
		return signature, info.Scopes[function.Type]
	default:
		return nil, nil
	}
}

func isMainFunction(ctx *ControlFlowContext, signature *types.Signature) bool {
	declaration, ok := ctx.Function().(*ast.FuncDecl)
	return ok &&
		declaration.Name.Name == "main" &&
		signature.Recv() == nil &&
		ctx.Package().Name() == "main"
}

func contextCancelCandidates(
	ctx *ControlFlowContext,
	scope *types.Scope,
) ([]contextCancelCandidate, []Finding, error) {
	info := ctx.Info()
	candidates := make([]contextCancelCandidate, 0)
	findings := make([]Finding, 0)
	seen := make(map[*types.Var]struct{})
	root := ctx.Function()
	var visitErr error
	ast.PreorderStack(
		root,
		nil,
		func(node ast.Node, stack []ast.Node) bool {
			if visitErr != nil {
				return false
			}
			if literal, nested := node.(*ast.FuncLit); nested && literal != root {
				return false
			}
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !isContextCancellationSelector(info, selector) || len(stack) < 2 {
				return true
			}
			call, ok := stack[len(stack) - 1].(*ast.CallExpr)
			if !ok || call.Fun != selector {
				return true
			}
			statement := stack[len(stack) - 2]
			identifier := contextCancelIdentifier(statement)
			if identifier == nil {
				return true
			}
			if identifier.Name == "_" {
				range_, err := ctx.Range(identifier)
				if err != nil {
					visitErr = err
					return false
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "cancel-discarded",
						Message: fmt.Sprintf(
							"the cancel function returned by context.%s should be called, not discarded, to avoid a context leak",
							selector.Sel.Name,
						),
						Range: range_,
						Help: "https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel",
					},
				)
				return true
			}
			variable, _ := info.Uses[identifier].(*types.Var)
			if variable != nil && !scope.Contains(variable.Pos()) {
				return true
			}
			if variable == nil {
				variable, _ = info.Defs[identifier].(*types.Var)
			}
			if variable == nil {
				return true
			}
			if _, duplicate := seen[variable]; duplicate {
				return true
			}
			seen[variable] = struct{}{}
			candidates = append(
				candidates,
				contextCancelCandidate{variable: variable, statement: statement},
			)
			return true
		},
	)
	return candidates, findings, visitErr
}

func isContextCancellationSelector(info *types.Info, selector *ast.SelectorExpr) bool {
	switch selector.Sel.Name {
	case "WithCancel",
		"WithCancelCause",
		"WithTimeout",
		"WithTimeoutCause",
		"WithDeadline",
		"WithDeadlineCause":
	default:
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	packageName, _ := info.Uses[identifier].(*types.PkgName)
	return packageName != nil && packageName.Imported().Path() == contextPackagePath
}

func contextCancelIdentifier(statement ast.Node) *ast.Ident {
	switch statement := statement.(type) {
	case *ast.ValueSpec:
		if len(statement.Names) > 1 {
			return statement.Names[1]
		}
	case *ast.AssignStmt:
		if len(statement.Lhs) > 1 {
			identifier, _ := statement.Lhs[1].(*ast.Ident)
			return identifier
		}
	}
	return nil
}

func contextCancelPath(
	ctx *ControlFlowContext,
	candidate contextCancelCandidate,
	signature *types.Signature,
) *ast.ReturnStmt {
	graph := ctx.Graph()
	info := ctx.Info()
	namedResult := tupleContainsVariable(signature.Results(), candidate.variable)
	uses := func(nodes []ast.Node) bool {
		for _, node := range nodes {
			if contextCancelNodeUses(ctx, info, node, candidate.variable, namedResult) {
				return true
			}
		}
		return false
	}

	var definition *cfg.Block
	var remainder []ast.Node
	for _, block := range graph.Blocks {
		for index, node := range block.Nodes {
			if node == candidate.statement {
				definition = block
				remainder = block.Nodes[index + 1:]
				break
			}
		}
		if definition != nil {
			break
		}
	}
	if definition == nil || uses(remainder) {
		return nil
	}
	if returned := definition.Return(); returned != nil {
		return returned
	}

	seen := make(map[*cfg.Block]bool)
	useMemo := make(map[*cfg.Block]bool)
	var search func([]*cfg.Block) *ast.ReturnStmt
	search = func(blocks []*cfg.Block) *ast.ReturnStmt {
		for _, block := range blocks {
			if seen[block] {
				continue
			}
			seen[block] = true
			used, found := useMemo[block]
			if !found {
				used = uses(block.Nodes)
				useMemo[block] = used
			}
			if used {
				continue
			}
			if returned := block.Return(); returned != nil {
				return returned
			}
			if returned := search(block.Succs); returned != nil {
				return returned
			}
		}
		return nil
	}
	return search(definition.Succs)
}

func contextCancelNodeUses(
	ctx *ControlFlowContext,
	info *types.Info,
	node ast.Node,
	variable *types.Var,
	namedResult bool,
) bool {
	var preservingAssignment *ast.AssignStmt
	if assignment, ok := node.(*ast.AssignStmt); ok {
		targetsCancel := false
		for _, target := range assignment.Lhs {
			if directObject(info, target) == variable {
				targetsCancel = true
				break
			}
		}
		if targetsCancel {
			effect := objectObligationEffect(
				info,
				assignment,
				variable,
				func(call *ast.CallExpr) bool {
					return directObject(info, call.Fun) == variable
				},
				ctx.ParameterEffect,
				ParameterEffectCancelInvoke | ParameterEffectTransfer,
				ctx.ReturnAliasesArgument,
			)
			if effect != obligationOpen {
				return true
			}
			preservingAssignment = assignment
		}
	}
	used := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, stack []ast.Node) bool {
			if used {
				return false
			}
			if returned, ok := current.(*ast.ReturnStmt);
				ok && returned.Results == nil && namedResult {
				used = true
				return false
			}
			identifier, ok := current.(*ast.Ident)
			if !ok || info.Uses[identifier] != variable {
				return true
			}
			if contextCancelPreservedAssignmentIdentifier(
				ctx,
				info,
				preservingAssignment,
				variable,
				identifier,
				stack,
			) {
				return true
			}
			for index := len(stack) - 1; index >= 0; index-- {
				call, ok := stack[index].(*ast.CallExpr)
				if !ok {
					continue
				}
				if ast.Unparen(call.Fun) == identifier {
					used = true
					return false
				}
				for argument, expression := range call.Args {
					if ast.Unparen(expression) != identifier {
						continue
					}
					summary := ctx.ParameterEffect(call, argument)
					if !summary.Known ||
						summary.GuaranteesAny(
							ParameterEffectCancelInvoke |
								ParameterEffectTransfer,
						) {
						used = true
					}
					return false
				}
			}
			used = true
			return false
		},
	)
	return used
}

func contextCancelPreservedAssignmentIdentifier(
	ctx *ControlFlowContext,
	info *types.Info,
	assignment *ast.AssignStmt,
	variable *types.Var,
	identifier *ast.Ident,
	stack []ast.Node,
) bool {
	if ctx == nil || info == nil || assignment == nil || variable == nil || identifier == nil {
		return false
	}
	for _, target := range assignment.Lhs {
		if ast.Unparen(target) == identifier {
			return true
		}
	}
	if len(assignment.Rhs) == len(assignment.Lhs) {
		for index, expression := range assignment.Rhs {
			if ast.Unparen(expression) == identifier &&
				directObject(info, assignment.Lhs[index]) == variable {
				return true
			}
		}
	}
	for index := len(stack) - 1; index >= 0; index-- {
		call, ok := stack[index].(*ast.CallExpr)
		if !ok {
			continue
		}
		for argument, expression := range call.Args {
			if ast.Unparen(expression) == identifier &&
				assignmentReturnsObjectAlias(
					info,
					assignment,
					variable,
					call,
					argument,
					ctx.ReturnAliasesArgument,
				) {
				return true
			}
		}
		return false
	}
	return false
}

func tupleContainsVariable(tuple *types.Tuple, variable *types.Var) bool {
	if tuple == nil {
		return false
	}
	for index := 0; index < tuple.Len(); index++ {
		if tuple.At(index) == variable {
			return true
		}
	}
	return false
}
