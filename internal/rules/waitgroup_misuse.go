package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

const maxWaitGroupLaunchTraceDepth = 32

type waitGroupMisuseRule struct{}

type waitGroupFunctionBody struct {
	file PackageFile
	body *ast.BlockStmt
	function *types.Func
}

type waitGroupAddSite struct {
	file PackageFile
	call *ast.CallExpr
	root types.Object
	indirect bool
}

// NewWaitGroupMisuseRule constructs the launched WaitGroup.Add ordering rule.
func NewWaitGroupMisuseRule() Rule {
	return waitGroupMisuseRule{}
}

func (waitGroupMisuseRule) Metadata() Metadata {
	return Metadata{
		ID: "waitgroup-misuse",
		Summary: "detects WaitGroup.Add calls made inside launched goroutines",
		Documentation: "Calling WaitGroup.Add with a positive delta from inside the goroutine being counted races with an outside Wait: the waiting goroutine may observe a zero count and return before Add executes. The count must be incremented before launching the goroutine. Glippy covers direct launched calls and function literals plus bounded same-package helper chains whose receiver maps through exact parameters to a stable WaitGroup receiver waited on later in the launching function body's straight-line statements.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The rule follows only an unconditional first-statement chain from a go statement to an exact sync.WaitGroup.Add call with a constant positive delta.",
			"The launching block before the go statement must be straight-line: prior control-flow constructs and calls other than exact counter transitions suppress the finding because launch reachability is otherwise unproven at the types tier.",
			"Launches and outside waits nested in conditional, loop, switch, select, case, communication, or standalone blocks remain conservative until the rule uses the shared control-flow tier.",
			"A syntactically earlier positive Add on the same receiver suppresses the finding because the counter may already prevent an early Wait return.",
			"Findings require a direct outside Wait later in the same launching block, with no intervening assignment to the mapped receiver and no intervening synchronously evaluated call or explicit channel synchronization.",
			"Zero, negative, and non-constant deltas remain conservative instead of assuming that they increase the counter.",
			"Fields, globals hidden behind helpers, dynamic calls, interface dispatch, side-effecting launch evaluation, nested control-flow waits, synchronization before Add, and helper chains deeper than the fixed bound remain conservative instead of producing speculative findings.",
		},
		Examples: []Example{
			{
				Title: "Increment before launching",
				Incorrect: "go func() { wg.Add(1); defer wg.Done() }()\nwg.Wait()",
				Correct: "wg.Add(1)\ngo func() { defer wg.Done() }()\nwg.Wait()",
			},
			{
				Title: "Increment before launching a helper",
				Incorrect: "func work(wg *sync.WaitGroup) { wg.Add(1); defer wg.Done() }\ngo work(&wg)\nwg.Wait()",
				Correct: "func work(wg *sync.WaitGroup) { defer wg.Done() }\nwg.Add(1)\ngo work(&wg)\nwg.Wait()",
			},
		},
	}
}

func (waitGroupMisuseRule) RunPackage(ctx *PackageContext) ([]PackageFinding, error) {
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("WaitGroup ordering analysis requires package types")
	}
	functions := make(map[*types.Func]waitGroupFunctionBody)
	for _, file := range ctx.Files() {
		for _, declaration := range file.Syntax().Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Name == nil {
				continue
			}
			object, _ := ctx.Info().Defs[function.Name].(*types.Func)
			if object != nil {
				functions[object] = waitGroupFunctionBody{
					file: file,
					body: function.Body,
					function: object,
				}
			}
		}
	}

	sites := make([]waitGroupAddSite, 0)
	seen := make(map[ast.Node]struct{})
	for _, file := range ctx.Files() {
		parents := waitGroupParentNodes(file.Syntax())
		ast.Inspect(
			file.Syntax(),
			func(node ast.Node) bool {
				var statements []ast.Stmt
				switch current := node.(type) {
				case *ast.BlockStmt:
					if !waitGroupDirectFunctionBody(parents, current) {
						return true
					}
					statements = current.List
				default:
					return true
				}
				for index, current := range statements {
					statement := waitGroupGoStatement(current)
					if statement == nil || statement.Call == nil {
						continue
					}
					site, found := traceRootLaunchedWaitGroupAdd(
						ctx.Info(),
						functions,
						file,
						statement.Call,
					)
					if !found ||
						!waitGroupStatementsReachLaunch(
							ctx.Info(),
							statements[:index],
							site.root,
						) ||
						waitGroupCounterEstablishedBefore(
							ctx.Info(),
							file.Syntax(),
							statement,
							site.root,
						) ||
						!waitGroupWaitFollows(
							ctx.Info(),
							statements[index + 1:],
							site.root,
						) {
						continue
					}
					if _, duplicate := seen[site.call]; !duplicate {
						seen[site.call] = struct{}{}
						sites = append(sites, site)
					}
				}
				return true
			},
		)
	}

	findings := make([]PackageFinding, 0, len(sites))
	for _, site := range sites {
		if !ctx.OwnsTarget(site.file) {
			continue
		}
		range_, err := site.file.PositionRange(site.call.Lparen, site.call.Lparen)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			PackageFinding{
				File: site.file,
				Finding: Finding{
					MessageKey: "waitgroup-add-inside-goroutine",
					Message: "WaitGroup.Add called from inside new goroutine",
					Range: range_,
					Help: "increment the WaitGroup before launching the goroutine",
				},
			},
		)
	}
	return findings, nil
}

func waitGroupDirectFunctionBody(parents map[ast.Node]ast.Node, block *ast.BlockStmt) bool {
	switch parents[block].(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		return true
	default:
		return false
	}
}

func waitGroupStatementsReachLaunch(
	info *types.Info,
	statements []ast.Stmt,
	root types.Object,
) bool {
	counter := int64(0)
	for _, statement := range statements {
		unwrapped := waitGroupUnwrapLabeledStatement(statement)
		switch unwrapped.(type) {
		case *ast.ReturnStmt,
			*ast.BranchStmt,
			*ast.IfStmt,
			*ast.SwitchStmt,
			*ast.TypeSwitchStmt,
			*ast.SelectStmt,
			*ast.ForStmt,
			*ast.RangeStmt,
			*ast.BlockStmt:
			return false
		}
		if expression, ok := unwrapped.(*ast.ExprStmt); ok {
			call, _ := ast.Unparen(expression.X).(*ast.CallExpr)
			if delta, recognized := waitGroupCounterDelta(info, call, root);
				recognized {
				counter += delta
				if counter < 0 {
					return false
				}
				continue
			}
		}
		if waitGroupStatementMaySynchronize(unwrapped) {
			return false
		}
	}
	return true
}

func waitGroupGoStatement(statement ast.Stmt) *ast.GoStmt {
	for {
		switch current := statement.(type) {
		case *ast.GoStmt:
			return current
		case *ast.LabeledStmt:
			statement = current.Stmt
		default:
			return nil
		}
	}
}

func traceRootLaunchedWaitGroupAdd(
	info *types.Info,
	functions map[*types.Func]waitGroupFunctionBody,
	file PackageFile,
	call *ast.CallExpr,
) (waitGroupAddSite, bool) {
	if call == nil || !waitGroupLaunchEvaluationSafe(call) {
		return waitGroupAddSite{}, false
	}
	if positiveWaitGroupAdd(info, call) {
		root := waitGroupAddRootObject(info, call, nil, true)
		return waitGroupAddSite{file: file, call: call, root: root}, root != nil
	}
	if literal, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		bindings := bindWaitGroupLiteralArguments(info, literal, call, nil, true)
		return traceFirstWaitGroupStatement(
			info,
			functions,
			file,
			literal.Body,
			bindings,
			true,
			false,
			make(map[*types.Func]struct{}),
			0,
		)
	}
	callee := typeutil.StaticCallee(info, call)
	body, found := functions[callee]
	if !found {
		return waitGroupAddSite{}, false
	}
	bindings := bindWaitGroupCallArguments(info, body.function, call, nil, true)
	return traceFirstWaitGroupStatement(
		info,
		functions,
		body.file,
		body.body,
		bindings,
		false,
		true,
		map[*types.Func]struct{}{callee: {}},
		1,
	)
}

func traceLaunchedWaitGroupAdd(
	info *types.Info,
	functions map[*types.Func]waitGroupFunctionBody,
	file PackageFile,
	call *ast.CallExpr,
	bindings map[types.Object]types.Object,
	allowDirect bool,
	indirect bool,
	visited map[*types.Func]struct{},
	depth int,
) (waitGroupAddSite, bool) {
	if call == nil || depth >= maxWaitGroupLaunchTraceDepth {
		return waitGroupAddSite{}, false
	}
	if !waitGroupLaunchEvaluationSafe(call) {
		return waitGroupAddSite{}, false
	}
	if positiveWaitGroupAdd(info, call) {
		root := waitGroupAddRootObject(info, call, bindings, allowDirect)
		return waitGroupAddSite{
			file: file,
			call: call,
			root: root,
			indirect: indirect,
		}, root != nil
	}
	if literal, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		literalBindings := bindWaitGroupLiteralArguments(
			info,
			literal,
			call,
			bindings,
			allowDirect,
		)
		return traceFirstWaitGroupStatement(
			info,
			functions,
			file,
			literal.Body,
			literalBindings,
			allowDirect,
			indirect,
			visited,
			depth,
		)
	}
	callee := typeutil.StaticCallee(info, call)
	body, found := functions[callee]
	if !found {
		return waitGroupAddSite{}, false
	}
	if _, recursive := visited[callee]; recursive {
		return waitGroupAddSite{}, false
	}
	visited[callee] = struct{}{}
	defer delete(visited, callee)
	childBindings := bindWaitGroupCallArguments(
		info,
		body.function,
		call,
		bindings,
		allowDirect,
	)
	return traceFirstWaitGroupStatement(
		info,
		functions,
		body.file,
		body.body,
		childBindings,
		false,
		true,
		visited,
		depth + 1,
	)
}

func waitGroupLaunchEvaluationSafe(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	expressions := make([]ast.Expr, 0, len(call.Args) + 1)
	expressions = append(expressions, call.Fun)
	expressions = append(expressions, call.Args...)
	for _, expression := range expressions {
		safe := true
		ast.Inspect(
			expression,
			func(node ast.Node) bool {
				if !safe || node == nil {
					return false
				}
				switch node := node.(type) {
				case *ast.CallExpr:
					safe = false
					return false
				case *ast.UnaryExpr:
					if node.Op == token.ARROW {
						safe = false
						return false
					}
				case *ast.FuncLit:
					return false
				}
				return true
			},
		)
		if !safe {
			return false
		}
	}
	return true
}

func traceFirstWaitGroupStatement(
	info *types.Info,
	functions map[*types.Func]waitGroupFunctionBody,
	file PackageFile,
	body *ast.BlockStmt,
	bindings map[types.Object]types.Object,
	allowDirect bool,
	indirect bool,
	visited map[*types.Func]struct{},
	depth int,
) (waitGroupAddSite, bool) {
	if body == nil || len(body.List) == 0 {
		return waitGroupAddSite{}, false
	}
	expression, ok := body.List[0].(*ast.ExprStmt)
	if !ok {
		return waitGroupAddSite{}, false
	}
	call, _ := ast.Unparen(expression.X).(*ast.CallExpr)
	return traceLaunchedWaitGroupAdd(
		info,
		functions,
		file,
		call,
		bindings,
		allowDirect,
		indirect,
		visited,
		depth,
	)
}

func bindWaitGroupCallArguments(
	info *types.Info,
	function *types.Func,
	call *ast.CallExpr,
	parent map[types.Object]types.Object,
	allowDirect bool,
) map[types.Object]types.Object {
	result := make(map[types.Object]types.Object)
	if function == nil || call == nil {
		return result
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Params() == nil {
		return result
	}
	count := min(signature.Params().Len(), len(call.Args))
	for index := 0; index < count; index++ {
		root := waitGroupBoundObject(info, call.Args[index], parent, allowDirect)
		if root != nil {
			result[signature.Params().At(index)] = root
		}
	}
	return result
}

func bindWaitGroupLiteralArguments(
	info *types.Info,
	literal *ast.FuncLit,
	call *ast.CallExpr,
	parent map[types.Object]types.Object,
	allowDirect bool,
) map[types.Object]types.Object {
	result := make(map[types.Object]types.Object, len(parent))
	for object, root := range parent {
		result[object] = root
	}
	if info == nil ||
		literal == nil ||
		literal.Type == nil ||
		literal.Type.Params == nil ||
		call == nil {
		return result
	}
	argumentIndex := 0
	for _, field := range literal.Type.Params.List {
		parameterCount := max(1, len(field.Names))
		for parameterIndex := 0; parameterIndex < parameterCount; parameterIndex++ {
			if argumentIndex >= len(call.Args) {
				return result
			}
			if parameterIndex < len(field.Names) {
				parameter := info.Defs[field.Names[parameterIndex]]
				root := waitGroupBoundObject(
					info,
					call.Args[argumentIndex],
					parent,
					allowDirect,
				)
				if parameter != nil && root != nil {
					result[parameter] = root
				}
			}
			argumentIndex++
		}
	}
	return result
}

func waitGroupCounterEstablishedBefore(
	info *types.Info,
	file *ast.File,
	launch *ast.GoStmt,
	root types.Object,
) bool {
	if info == nil || file == nil || launch == nil || root == nil {
		return false
	}
	parents := waitGroupParentNodes(file)
	statements, launchIndex, found := waitGroupContainingStatements(parents, launch)
	if !found {
		return false
	}
	counter := int64(0)
	for _, statement := range statements[:launchIndex] {
		unwrapped := waitGroupUnwrapLabeledStatement(statement)
		expression, ok := unwrapped.(*ast.ExprStmt)
		if !ok {
			if waitGroupStatementWritesRoot(info, unwrapped, root) ||
				waitGroupStatementMaySynchronize(unwrapped) {
				return false
			}
			continue
		}
		call, _ := ast.Unparen(expression.X).(*ast.CallExpr)
		delta, recognized := waitGroupCounterDelta(info, call, root)
		if !recognized {
			if waitGroupStatementMaySynchronize(statement) {
				return false
			}
			continue
		}
		counter += delta
		if counter < 0 {
			return false
		}
	}
	return counter > 0
}

func waitGroupParentNodes(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	stack := make([]ast.Node, 0)
	ast.Inspect(
		file,
		func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack) - 1]
				return true
			}
			if len(stack) != 0 {
				parents[node] = stack[len(stack) - 1]
			}
			stack = append(stack, node)
			return true
		},
	)
	return parents
}

func waitGroupContainingStatements(
	parents map[ast.Node]ast.Node,
	launch *ast.GoStmt,
) ([]ast.Stmt, int, bool) {
	var statement ast.Stmt = launch
	for parent := parents[statement]; parent != nil; parent = parents[parent] {
		if labeled, ok := parent.(*ast.LabeledStmt); ok && labeled.Stmt == statement {
			statement = labeled
			continue
		}
		var statements []ast.Stmt
		switch container := parent.(type) {
		case *ast.BlockStmt:
			statements = container.List
		case *ast.CaseClause:
			statements = container.Body
		case *ast.CommClause:
			statements = container.Body
		default:
			return nil, 0, false
		}
		for index, candidate := range statements {
			if candidate == statement {
				return statements, index, true
			}
		}
		return nil, 0, false
	}
	return nil, 0, false
}

func waitGroupCounterDelta(info *types.Info, call *ast.CallExpr, root types.Object) (int64, bool) {
	if call == nil || root == nil {
		return 0, false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "sync" {
		return 0, false
	}
	receiver := waitGroupAddReceiver(call, info)
	if waitGroupBoundObject(info, receiver, nil, true) != root {
		return 0, false
	}
	switch function.Name() {
	case "Done":
		return -1, true
	case "Add":
		deltaIndex := 0
		if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
			selection := info.Selections[selector]
			if selection != nil && selection.Kind() == types.MethodExpr {
				deltaIndex = 1
			}
		}
		if len(call.Args) != deltaIndex + 1 {
			return 0, false
		}
		value := info.Types[call.Args[deltaIndex]].Value
		if value == nil {
			return 0, false
		}
		delta, exact := constant.Int64Val(value)
		return delta, exact
	default:
		return 0, false
	}
}

func waitGroupAddRootObject(
	info *types.Info,
	call *ast.CallExpr,
	bindings map[types.Object]types.Object,
	allowDirect bool,
) types.Object {
	receiver := waitGroupAddReceiver(call, info)
	return waitGroupBoundObject(info, receiver, bindings, allowDirect)
}

func waitGroupBoundObject(
	info *types.Info,
	expression ast.Expr,
	bindings map[types.Object]types.Object,
	allowDirect bool,
) types.Object {
	if info == nil || expression == nil {
		return nil
	}
	expression = ast.Unparen(expression)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expression = ast.Unparen(unary.X)
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return nil
	}
	object := info.ObjectOf(identifier)
	if root := bindings[object]; root != nil {
		return root
	}
	if allowDirect {
		return object
	}
	return nil
}

func waitGroupAddReceiver(call *ast.CallExpr, info *types.Info) ast.Expr {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil
	}
	selection := info.Selections[selector]
	if selection != nil && selection.Kind() == types.MethodExpr {
		if len(call.Args) == 0 {
			return nil
		}
		return call.Args[0]
	}
	return selector.X
}

func waitGroupWaitFollows(info *types.Info, statements []ast.Stmt, root types.Object) bool {
	for _, statement := range statements {
		unwrapped := waitGroupUnwrapLabeledStatement(statement)
		if expression, ok := unwrapped.(*ast.ExprStmt); ok {
			call, _ := ast.Unparen(expression.X).(*ast.CallExpr)
			if waitGroupWaitOnRoot(info, call, root) {
				return true
			}
		}
		if waitGroupStatementWritesRoot(info, unwrapped, root) {
			return false
		}
		switch unwrapped.(type) {
		case *ast.ReturnStmt,
			*ast.BranchStmt,
			*ast.IfStmt,
			*ast.SwitchStmt,
			*ast.TypeSwitchStmt,
			*ast.SelectStmt,
			*ast.ForStmt,
			*ast.RangeStmt,
			*ast.BlockStmt:
			return false
		}
		if waitGroupStatementMaySynchronize(unwrapped) {
			return false
		}
	}
	return false
}

func waitGroupUnwrapLabeledStatement(statement ast.Stmt) ast.Stmt {
	for {
		labeled, ok := statement.(*ast.LabeledStmt)
		if !ok {
			return statement
		}
		statement = labeled.Stmt
	}
}

func waitGroupStatementMaySynchronize(statement ast.Stmt) bool {
	maySynchronize := false
	var asynchronousCall *ast.CallExpr
	switch current := statement.(type) {
	case *ast.GoStmt:
		asynchronousCall = current.Call
	case *ast.DeferStmt:
		asynchronousCall = current.Call
	}
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			if maySynchronize || node == nil {
				return false
			}
			switch current := node.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CallExpr:
				if current == asynchronousCall {
					return true
				}
				maySynchronize = true
				return false
			case *ast.SendStmt, *ast.SelectStmt:
				maySynchronize = true
				return false
			case *ast.UnaryExpr:
				if current.Op == token.ARROW {
					maySynchronize = true
					return false
				}
			}
			return true
		},
	)
	return maySynchronize
}

func waitGroupStatementWritesRoot(info *types.Info, statement ast.Stmt, root types.Object) bool {
	writes := make([]ast.Expr, 0, 2)
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.FuncLit:
				return false
			case *ast.AssignStmt:
				writes = append(writes, current.Lhs...)
				return false
			case *ast.IncDecStmt:
				writes = append(writes, current.X)
				return false
			case *ast.RangeStmt:
				if current.Key != nil {
					writes = append(writes, current.Key)
				}
				if current.Value != nil {
					writes = append(writes, current.Value)
				}
			}
			return true
		},
	)
	for _, expression := range writes {
		found := false
		ast.Inspect(
			expression,
			func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok && info.ObjectOf(identifier) == root {
					found = true
					return false
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

func waitGroupWaitOnRoot(info *types.Info, call *ast.CallExpr, root types.Object) bool {
	if call == nil || root == nil {
		return false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil ||
		function.Name() != "Wait" ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "sync" {
		return false
	}
	receiver := waitGroupAddReceiver(call, info)
	return waitGroupBoundObject(info, receiver, nil, true) == root
}

func positiveWaitGroupAdd(info *types.Info, call *ast.CallExpr) bool {
	function := typeutil.StaticCallee(info, call)
	if function == nil ||
		function.Name() != "Add" ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "sync" {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	type_ := signature.Recv().Type()
	if pointer, ok := types.Unalias(type_).(*types.Pointer); ok {
		type_ = pointer.Elem()
	}
	named, _ := types.Unalias(type_).(*types.Named)
	if named == nil ||
		named.Obj() == nil ||
		named.Obj().Pkg() == nil ||
		named.Obj().Pkg().Path() != "sync" ||
		named.Obj().Name() != "WaitGroup" {
		return false
	}
	deltaIndex := 0
	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		selection := info.Selections[selector]
		if selection != nil && selection.Kind() == types.MethodExpr {
			deltaIndex = 1
		}
	}
	if len(call.Args) != deltaIndex + 1 {
		return false
	}
	value := info.Types[call.Args[deltaIndex]].Value
	return value != nil && constant.Sign(value) > 0
}
