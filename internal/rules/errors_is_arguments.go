package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type errorsIsArgumentsRule struct{}

func (errorsIsArgumentsRule) Metadata() Metadata {
	return Metadata{
		ID: "errors-is-arguments",
		Summary: "detects reversed errors.Is arguments",
		Documentation: "errors.Is expects the error being inspected first and the target sentinel second. A package-level sentinel from another package in the first position usually means those arguments were reversed, so wrapped errors will not match as intended.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only the standard library errors.Is function is recognized, by typed object identity.",
			"The first argument must directly reference a package-level variable from another package; calls, fields, local aliases, and package variables declared by the analyzed package are not reported.",
			"Calls with package-level variables from other packages in both positions are excluded because they can intentionally test compatibility between sentinels.",
		},
		Examples: []Example{
			{
				Title: "Inspect the dynamic error before the sentinel",
				Incorrect: "errors.Is(io.EOF, err)",
				Correct: "errors.Is(err, io.EOF)",
			},
		},
	}
}

func (errorsIsArgumentsRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("errors-is-arguments requires a call expression")
	}
	if len(call.Args) != 2 || !isErrorsIs(ctx.Info(), call.Fun) {
		return nil, nil
	}
	if !isExternalPackageVariable(ctx.Info(), ctx.Package(), call.Args[0]) ||
		isExternalPackageVariable(ctx.Info(), ctx.Package(), call.Args[1]) {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "reversed-arguments",
			Message: "errors.Is arguments appear to be reversed",
			Range: range_,
			Help: "pass the error value first and the package sentinel second",
		},
	}, nil
}

func isErrorsIs(info *types.Info, expression ast.Expr) bool {
	function, ok := referencedObject(info, expression).(*types.Func)
	return ok &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "errors" &&
		function.Name() == "Is"
}

func isExternalPackageVariable(info *types.Info, current *types.Package, expression ast.Expr) bool {
	variable, ok := referencedObject(info, expression).(*types.Var)
	if !ok || variable.Pkg() == nil || variable.Parent() != variable.Pkg().Scope() {
		return false
	}
	return current == nil || variable.Pkg().Path() != current.Path()
}

func referencedObject(info *types.Info, expression ast.Expr) types.Object {
	if info == nil {
		return nil
	}
	expression = ast.Unparen(expression)
	switch expression := expression.(type) {
	case *ast.Ident:
		return info.Uses[expression]
	case *ast.SelectorExpr:
		return info.Uses[expression.Sel]
	default:
		return nil
	}
}
