package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

type loopCaptureRule struct{}

// NewLoopCaptureRule constructs the native modern-Go loop capture rule for
// product registry composition.
func NewLoopCaptureRule() Rule {
	return loopCaptureRule{}
}

func (loopCaptureRule) Metadata() Metadata {
	return Metadata{
		ID: "loop-capture",
		Summary: "detects reused loop variables captured by escaping closures",
		Documentation: "Go 1.22 gives iteration-local identity to variables declared by a loop, but variables declared outside the loop and assigned by it remain shared across iterations. Capturing one of those reused variables in a goroutine, defer, errgroup task, or parallel subtest can observe a later value or race with the loop. Glippy follows the conservative standard loopclosure escape patterns while retaining the modern-Go cases that the standard analyzer no longer reports.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeForStmt, NodeRangeStmt},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The rule deliberately reports only loop variables declared outside the loop; variables declared by Go 1.22 and later loop syntax have per-iteration identity.",
			"As in the standard loopclosure analyzer, ordinary goroutine and defer captures are checked only when the launch is the loop body's recursively last statement, avoiding cases that may synchronize before the next iteration.",
			"Closures passed through arbitrary helpers are outside the recognized go, defer, errgroup.Group.Go, and testing.T.Run patterns.",
		},
		Examples: []Example{
			{
				Title: "Declare the iteration variable in the range clause",
				Incorrect: "var value item\nfor _, value = range values { go func() { use(value) }() }",
				Correct: "for _, value := range values { go func() { use(value) }() }",
			},
		},
	}
}

func (loopCaptureRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("loop-capture requires complete type information")
	}
	variables, body, err := reusedLoopVariables(ctx.Info(), node)
	if err != nil {
		return nil, err
	}
	if len(variables) == 0 || body == nil {
		return nil, nil
	}

	identifiers := capturedLoopIdentifiers(ctx.Info(), variables, body)
	findings := make([]Finding, 0, len(identifiers))
	for _, identifier := range identifiers {
		range_, err := ctx.Range(identifier)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "reused-variable-captured",
				Message: fmt.Sprintf(
					"loop variable %s captured by func literal",
					identifier.Name,
				),
				Range: range_,
				Help: "declare the iteration variable in the loop or pass it as a closure argument",
			},
		)
	}
	return findings, nil
}

func reusedLoopVariables(info *types.Info, node ast.Node) ([]types.Object, *ast.BlockStmt, error) {
	variables := make([]types.Object, 0, 2)
	add := func(expression ast.Expr, loop token.Pos) {
		identifier, _ := expression.(*ast.Ident)
		if identifier == nil {
			return
		}
		object := info.ObjectOf(identifier)
		if object != nil && object.Pos().IsValid() && object.Pos() < loop {
			variables = append(variables, object)
		}
	}
	switch loop := node.(type) {
	case *ast.RangeStmt:
		if loop.Tok != token.ASSIGN {
			return nil, loop.Body, nil
		}
		add(loop.Key, loop.Pos())
		add(loop.Value, loop.Pos())
		return variables, loop.Body, nil
	case *ast.ForStmt:
		declaredByInit := make(map[types.Object]struct{})
		if init, ok := loop.Init.(*ast.AssignStmt); ok && init.Tok == token.DEFINE {
			for _, expression := range init.Lhs {
				identifier, _ := expression.(*ast.Ident)
				if identifier != nil {
					declaredByInit[info.ObjectOf(identifier)] = struct{}{}
				}
			}
		}
		switch post := loop.Post.(type) {
		case *ast.AssignStmt:
			for _, expression := range post.Lhs {
				identifier, _ := expression.(*ast.Ident)
				if identifier == nil {
					continue
				}
				if _, perIteration := declaredByInit[info.ObjectOf(identifier)];
					!perIteration {
					add(expression, loop.Pos())
				}
			}
		case *ast.IncDecStmt:
			identifier, _ := post.X.(*ast.Ident)
			if identifier == nil {
				break
			}
			if _, perIteration := declaredByInit[info.ObjectOf(identifier)];
				!perIteration {
				add(post.X, loop.Pos())
			}
		}
		return variables, loop.Body, nil
	default:
		return nil, nil, fmt.Errorf("loop-capture requires a for or range statement")
	}
}

func capturedLoopIdentifiers(
	info *types.Info,
	variables []types.Object,
	body *ast.BlockStmt,
) []*ast.Ident {
	result := make([]*ast.Ident, 0)
	appendCaptured := func(statements []ast.Stmt) {
		for _, statement := range statements {
			ast.Inspect(
				statement,
				func(node ast.Node) bool {
					identifier, ok := node.(*ast.Ident)
					if !ok {
						return true
					}
					object := info.Uses[identifier]
					for _, variable := range variables {
						if object == variable {
							result = append(result, identifier)
							break
						}
					}
					return true
				},
			)
		}
	}

	forEachLoopLastStatement(
		body.List,
		func(statement ast.Stmt) {
			switch statement := statement.(type) {
			case *ast.GoStmt:
				appendCaptured(loopClosureStatements(statement.Call.Fun))
			case *ast.DeferStmt:
				appendCaptured(loopClosureStatements(statement.Call.Fun))
			case *ast.ExprStmt:
				call, _ := statement.X.(*ast.CallExpr)
				if call != nil &&
					isLoopCaptureMethod(
						info,
						call,
						"golang.org/x/sync/errgroup",
						"Group",
						"Go",
					) &&
					len(call.Args) == 1 {
					appendCaptured(loopClosureStatements(call.Args[0]))
				}
			}
		},
	)
	for _, statement := range body.List {
		expression, ok := statement.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, _ := expression.X.(*ast.CallExpr)
		if call != nil {
			appendCaptured(parallelSubtestStatements(info, call))
		}
	}
	return result
}

func forEachLoopLastStatement(statements []ast.Stmt, visit func(ast.Stmt)) {
	if len(statements) == 0 {
		return
	}
	statement := statements[len(statements) - 1]
	switch statement := statement.(type) {
	case *ast.IfStmt:
		for {
			forEachLoopLastStatement(statement.Body.List, visit)
			switch alternative := statement.Else.(type) {
			case *ast.BlockStmt:
				forEachLoopLastStatement(alternative.List, visit)
				return
			case *ast.IfStmt:
				statement = alternative
			case nil:
				return
			}
		}
	case *ast.ForStmt:
		forEachLoopLastStatement(statement.Body.List, visit)
	case *ast.RangeStmt:
		forEachLoopLastStatement(statement.Body.List, visit)
	case *ast.SwitchStmt:
		for _, clause := range statement.Body.List {
			forEachLoopLastStatement(clause.(*ast.CaseClause).Body, visit)
		}
	case *ast.TypeSwitchStmt:
		for _, clause := range statement.Body.List {
			forEachLoopLastStatement(clause.(*ast.CaseClause).Body, visit)
		}
	case *ast.SelectStmt:
		for _, clause := range statement.Body.List {
			forEachLoopLastStatement(clause.(*ast.CommClause).Body, visit)
		}
	default:
		visit(statement)
	}
}

func loopClosureStatements(expression ast.Expr) []ast.Stmt {
	literal, _ := ast.Unparen(expression).(*ast.FuncLit)
	if literal == nil {
		return nil
	}
	return literal.Body.List
}

func isLoopCaptureMethod(
	info *types.Info,
	call *ast.CallExpr,
	packagePath string,
	typeName string,
	methodName string,
) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != packagePath ||
		function.Name() != methodName {
		return false
	}
	receiver := types.Unalias(selection.Recv())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, _ := receiver.(*types.Named)
	return named != nil && named.Obj().Name() == typeName
}

func parallelSubtestStatements(info *types.Info, call *ast.CallExpr) []ast.Stmt {
	if !isLoopCaptureMethod(info, call, "testing", "T", "Run") || len(call.Args) != 2 {
		return nil
	}
	literal, _ := ast.Unparen(call.Args[1]).(*ast.FuncLit)
	if literal == nil ||
		literal.Type.Params == nil ||
		len(literal.Type.Params.List) == 0 ||
		len(literal.Type.Params.List[0].Names) == 0 {
		return nil
	}
	testVariable := info.Defs[literal.Type.Params.List[0].Names[0]]
	if testVariable == nil {
		return nil
	}

	statements := make([]ast.Stmt, 0)
	afterParallel := false
	for _, original := range literal.Body.List {
		statement, labeled := unlabelLoopStatement(original)
		if labeled {
			statements = nil
			afterParallel = false
		}
		if afterParallel {
			statements = append(statements, statement)
			continue
		}
		expression, _ := statement.(*ast.ExprStmt)
		if expression == nil {
			continue
		}
		parallelCall, _ := expression.X.(*ast.CallExpr)
		if parallelCall == nil ||
			!isLoopCaptureMethod(info, parallelCall, "testing", "T", "Parallel") {
			continue
		}
		selector, _ := ast.Unparen(parallelCall.Fun).(*ast.SelectorExpr)
		identifier, _ := selector.X.(*ast.Ident)
		if identifier != nil && info.Uses[identifier] == testVariable {
			afterParallel = true
		}
	}
	return statements
}

func unlabelLoopStatement(statement ast.Stmt) (ast.Stmt, bool) {
	labeled := false
	for {
		label, ok := statement.(*ast.LabeledStmt)
		if !ok {
			return statement, labeled
		}
		labeled = true
		statement = label.Stmt
	}
}
