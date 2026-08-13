package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type resourceNotClosedRule struct{}

type localCloserCandidate struct {
	identifier *ast.Ident
	object types.Object
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
		Documentation: "A call result with a conventional Close method usually owns a file, connection, compressor, or similar resource. A local result that is never closed and never transferred can retain descriptors, connections, buffers, or other external state until process termination or garbage collection.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeFile},
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"The initial contract treats a direct argument, return, send, composite-literal insertion, closure capture, or assignment to another variable as an ownership transfer and does not analyze the callee.",
			"Any direct Close call counts as cleanup; path-sensitive proof that cleanup runs on every successful path is deferred to a control-flow expansion.",
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

func (resourceNotClosedRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	file, ok := node.(*ast.File)
	if !ok {
		return nil, fmt.Errorf("resource-not-closed requires a file")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("resource-not-closed requires complete type information")
	}
	findings := make([]Finding, 0)
	var runErr error
	ast.Inspect(
		file,
		func(current ast.Node) bool {
			if runErr != nil {
				return false
			}
			var body *ast.BlockStmt
			switch function := current.(type) {
			case *ast.FuncDecl:
				body = function.Body
			case *ast.FuncLit:
				body = function.Body
			default:
				return true
			}
			if body == nil {
				return false
			}
			for _, candidate := range localCloserCandidates(ctx.Info(), body) {
				closed, transferred := closerDisposition(
					ctx.Info(),
					body,
					candidate,
				)
				if closed || transferred {
					continue
				}
				range_, err := ctx.Range(candidate.identifier)
				if err != nil {
					runErr = err
					return false
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "resource-not-closed",
						Message: fmt.Sprintf(
							"resource %q is not closed or transferred",
							candidate.identifier.Name,
						),
						Range: range_,
						Help: "close the resource after checking the constructor error or transfer ownership explicitly",
					},
				)
			}
			return true
		},
	)
	return findings, runErr
}

func localCloserCandidates(info *types.Info, body *ast.BlockStmt) []localCloserCandidate {
	result := make([]localCloserCandidate, 0)
	ast.Inspect(
		body,
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				return true
			}
			call, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if call == nil {
				return true
			}
			signature, _ := types.Unalias(info.TypeOf(call.Fun)).(*types.Signature)
			if signature == nil ||
				signature.Results() == nil ||
				signature.Results().Len() != len(assignment.Lhs) {
				return true
			}
			for index, left := range assignment.Lhs {
				identifier, _ := left.(*ast.Ident)
				if identifier == nil ||
					identifier.Name == "_" ||
					!conventionalCloser(signature.Results().At(index).Type()) {
					continue
				}
				object := info.ObjectOf(identifier)
				if object != nil {
					result = append(
						result,
						localCloserCandidate{
							identifier: identifier,
							object: object,
						},
					)
				}
			}
			return true
		},
	)
	return result
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

func closerDisposition(
	info *types.Info,
	body *ast.BlockStmt,
	candidate localCloserCandidate,
) (bool, bool) {
	closed := false
	transferred := false
	ast.PreorderStack(
		body,
		nil,
		func(node ast.Node, stack []ast.Node) bool {
			if closed || transferred {
				return false
			}
			if literal, nested := node.(*ast.FuncLit); nested {
				if expressionUsesObject(info, literal.Body, candidate.object) {
					transferred = true
				}
				return false
			}
			switch current := node.(type) {
			case *ast.CallExpr:
				if closeCallUsesObject(info, current, candidate.object) {
					closed = true
					return false
				}
				for _, argument := range current.Args {
					if directObject(info, argument) == candidate.object ||
						methodValueUsesObject(
							info,
							argument,
							candidate.object,
						) {
						transferred = true
						return false
					}
				}
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directObject(info, expression) == candidate.object {
						transferred = true
						return false
					}
				}
			case *ast.SendStmt:
				if directObject(info, current.Value) == candidate.object {
					transferred = true
					return false
				}
			case *ast.AssignStmt:
				for _, expression := range current.Rhs {
					if directObject(info, expression) != candidate.object {
						continue
					}
					for _, target := range current.Lhs {
						identifier, _ := target.(*ast.Ident)
						if identifier == nil || identifier.Name != "_" {
							transferred = true
							return false
						}
					}
				}
			case *ast.CompositeLit:
				for _, element := range current.Elts {
					if directObject(info, element) == candidate.object {
						transferred = true
						return false
					}
				}
			}
			return true
		},
	)
	return closed, transferred
}

func closeCallUsesObject(info *types.Info, call *ast.CallExpr, object types.Object) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	return selector != nil &&
		selector.Sel.Name == "Close" &&
		directObject(info, selector.X) == object
}

func methodValueUsesObject(info *types.Info, expression ast.Expr, object types.Object) bool {
	selector, _ := ast.Unparen(expression).(*ast.SelectorExpr)
	return selector != nil && directObject(info, selector.X) == object
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
