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
		Documentation: "The cancellation function returned by context.WithCancel, context.WithTimeout, or context.WithDeadline releases resources owned by the derived context. Discarding it or returning along a path that never uses it can retain timers, children, and references longer than intended. Glippy implements the standard lostcancel contract over its shared control-flow tier because executing the analyzer's transitive no-return fact graph would require loading dependency syntax for the complete import closure.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"As in the standard lostcancel analyzer, any reference to the cancellation variable counts as a use; the rule does not prove that the referenced function is eventually called.",
			"Cancellation transferred through helpers, fields, or containers is outside the intraprocedural ownership model.",
			"The shared CFG propagates package-local no-return behavior and recognizes exact standard-library terminal calls from os, runtime, syscall, log, and testing; other imported helpers without source or analyzer facts remain conservatively returning.",
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
		if contextCancelPath(ctx.Graph(), ctx.Info(), candidate, signature) == nil {
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
	graph *cfg.CFG,
	info *types.Info,
	candidate contextCancelCandidate,
	signature *types.Signature,
) *ast.ReturnStmt {
	namedResult := tupleContainsVariable(signature.Results(), candidate.variable)
	uses := func(nodes []ast.Node) bool {
		for _, node := range nodes {
			found := false
			ast.Inspect(
				node,
				func(current ast.Node) bool {
					switch current := current.(type) {
					case *ast.Ident:
						found = info.Uses[current] == candidate.variable
					case *ast.ReturnStmt:
						found = current.Results == nil && namedResult
					}
					return !found
				},
			)
			if found {
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
