package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/cfg"
)

type resourceNotClosedRule struct{}

type localCloserCandidate struct {
	identifier *ast.Ident
	object types.Object
	statement *ast.AssignStmt
	acquisitionGuard *ast.IfStmt
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
		Documentation: "A call result with a conventional Close method usually owns a file, connection, compressor, or similar resource. A locally owned result that reaches a normal return without being closed or transferred can retain descriptors, connections, buffers, or other external state until process termination or garbage collection. Exact nil-result branches carry no ownership obligation. Versioned parameter-effect, receiver-effect, returned-alias, and cleanup-managed-result summaries distinguish retained obligations from guaranteed closure, ownership transfer, or test-lifetime cleanup.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"Only direct resource == nil and resource != nil conditions discharge the nil branch; compound nilness, aliases, and indirect comparisons remain conservative.",
			"Exact tar, gzip, and multipart writer constructors belong to writer-not-finalized and are excluded from this generic closer rule.",
			"A direct function-literal constructor argument, or the final direct same-block value of a local argument, that captures the assigned resource conservatively transfers ownership because callback retention and execution are unknown; conditional or nested assignments do not prove transfer.",
			"A statically resolved helper in a selected local-source module that provably borrows the resource leaves the obligation open; guaranteed closure or transfer must cover every normally returning helper path.",
			"An exact returned-alias contract preserves the obligation when the result is assigned back to the same resource variable; new alias bindings remain outside the tracked ownership identity.",
			"Cleanup-managed results require one stable direct local result, an exact testing.T Cleanup call on a pointer receiver with a function-literal callback, and direct, parameter-effect, or receiver-effect proven Close on every normally returning callback path. Copied testing.T values, conditional registration or closure, asynchronous or nested closure, reassignment, aliases, and non-testing cleanup APIs remain conservative.",
			"Receiver effects require a direct method selection; a promoted method does not prove an effect for the outer receiver value.",
			"Dynamic calls, interface dispatch, recursion, local aliases, and helpers outside selected modules retain the conservative ownership-transfer behavior when no summary is available.",
			"Pipes returned by os/exec.Cmd are owned by Cmd.Start and Cmd.Wait under the standard-library contract and are not treated as caller-owned closers.",
			"Cleanup and ownership transfer must cover every normally returning path after a conventional acquisition guard when one is present.",
			"Only call results whose static type has Close() error are considered resources; zero-result Close methods are too broad for the initial ownership contract.",
		},
		Examples: []Example{
			{
				Title: "Close an opened resource after the error check",
				Incorrect: "file, err := os.Open(path)\nif err != nil { return err }\nuse(file)",
				Correct: "file, err := os.Open(path)\nif err != nil { return err }\ndefer file.Close()",
			},
			{
				Title: "Register cleanup before returning a test resource",
				Incorrect: "func open(t *testing.T) *os.File {\n\tfile, _ := os.Open(path)\n\treturn file\n}",
				Correct: "func open(t *testing.T) *os.File {\n\tfile, _ := os.Open(path)\n\tt.Cleanup(func() { _ = file.Close() })\n\treturn file\n}",
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
	for _, candidate := range
		localCloserCandidates(
			ctx.Info(),
			ctx.Body(),
			ctx.ReturnAliasesArgument,
			ctx.CleanupManagedResult,
			true,
		) {
		start, found := localCloserObligationStart(ctx.Graph(), ctx.Info(), candidate)
		if !found ||
			!obligationReachesOpenReturnWithEdgeDischarge(
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
						ctx.ReceiverEffect,
						ctx.ParameterEffect,
						ParameterEffectClose | ParameterEffectTransfer,
						ctx.ReturnAliasesArgument,
					)
				},
				func(from, to *cfg.Block) bool {
					return edgeProvesResourceNil(
						ctx.Info(),
						from,
						to,
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

func edgeProvesResourceNil(
	info *types.Info,
	from *cfg.Block,
	to *cfg.Block,
	resourceObject types.Object,
) bool {
	if info == nil ||
		from == nil ||
		to == nil ||
		resourceObject == nil ||
		len(from.Nodes) == 0 {
		return false
	}
	guard, _ := to.Stmt.(*ast.IfStmt)
	if guard == nil || from.Nodes[len(from.Nodes) - 1] != guard.Cond {
		return false
	}
	comparison, _ := ast.Unparen(guard.Cond).(*ast.BinaryExpr)
	if comparison == nil || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return false
	}
	nilObject := types.Universe.Lookup("nil")
	comparesResourceToNil := directObject(info, comparison.X) == resourceObject &&
		directObject(info, comparison.Y) == nilObject ||
		directObject(info, comparison.Y) == resourceObject &&
			directObject(info, comparison.X) == nilObject
	if !comparesResourceToNil {
		return false
	}
	if comparison.Op == token.EQL {
		return to.Kind == cfg.KindIfThen
	}
	return to.Kind == cfg.KindIfElse || to.Kind == cfg.KindIfDone
}

func localCloserCandidates(
	info *types.Info,
	body *ast.BlockStmt,
	returnsAlias func(*ast.CallExpr, int, int) bool,
	cleanupManaged func(*ast.CallExpr, int) bool,
	constructorCallbacksTransfer bool,
) []localCloserCandidate {
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
				_, _, specializedWriter := writerLifecycleConstructor(info, call)
				if call == nil ||
					commandManagedPipe(info, call) ||
					specializedWriter {
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
				for index, left := range assignment.Lhs {
					identifier, _ := left.(*ast.Ident)
					if identifier == nil ||
						identifier.Name == "_" ||
						cleanupManaged != nil &&
							cleanupManaged(call, index) ||
						callResultAliasesArgument(
							call,
							index,
							returnsAlias,
						) ||
						!conventionalCloser(
							signature.Results().At(index).Type(),
						) {
						continue
					}
					object := info.ObjectOf(identifier)
					if object != nil &&
						(!constructorCallbacksTransfer ||
							!constructorCallbackCapturesObject(
								info,
								block,
								call,
								object,
							)) {
						acquisitionGuard := immediateFollowingGuard(
							block.List,
							statementIndex,
						)
						guard := followingAcquisitionErrorGuard(
							info,
							block.List[statementIndex + 1:],
							assignment,
						)
						result = append(
							result,
							localCloserCandidate{
								identifier: identifier,
								object: object,
								statement: assignment,
								acquisitionGuard: acquisitionGuard,
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

func constructorCallbackCapturesObject(
	info *types.Info,
	body *ast.BlockStmt,
	call *ast.CallExpr,
	object types.Object,
) bool {
	if info == nil || body == nil || call == nil || object == nil {
		return false
	}
	for _, argument := range call.Args {
		if expressionContainsCapturingCallback(info, argument, object) {
			return true
		}
	}
	references := make(map[types.Object]struct{})
	for _, argument := range call.Args {
		ast.Inspect(
			argument,
			func(node ast.Node) bool {
				identifier, found := node.(*ast.Ident)
				if !found {
					return true
				}
				reference := info.ObjectOf(identifier)
				if _, local := reference.(*types.Var);
					local && reference != object {
					references[reference] = struct{}{}
				}
				return true
			},
		)
	}
	for reference := range references {
		expression := finalLocalValueBefore(info, body, reference, call.Pos())
		if expressionContainsCapturingCallback(info, expression, object) {
			return true
		}
	}
	return false
}

func expressionContainsCapturingCallback(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
) bool {
	if info == nil || expression == nil || object == nil {
		return false
	}
	captured := false
	ast.Inspect(
		expression,
		func(node ast.Node) bool {
			literal, nested := node.(*ast.FuncLit)
			if !nested {
				return true
			}
			if expressionUsesObject(info, literal.Body, object) {
				captured = true
			}
			return false
		},
	)
	return captured
}

func finalLocalValueBefore(
	info *types.Info,
	body *ast.BlockStmt,
	object types.Object,
	before token.Pos,
) ast.Expr {
	if info == nil || body == nil || object == nil || !before.IsValid() {
		return nil
	}
	var expression ast.Expr
	for _, statement := range body.List {
		assignment, assigned := statement.(*ast.AssignStmt)
		if !assigned ||
			assignment.Pos() >= before ||
			len(assignment.Lhs) != len(assignment.Rhs) {
			continue
		}
		for index, target := range assignment.Lhs {
			if directObject(info, target) == object {
				expression = assignment.Rhs[index]
			}
		}
	}
	return expression
}

func callResultAliasesArgument(
	call *ast.CallExpr,
	result int,
	returnsAlias func(*ast.CallExpr, int, int) bool,
) bool {
	if call == nil || result < 0 || returnsAlias == nil {
		return false
	}
	for argument := range call.Args {
		if returnsAlias(call, result, argument) {
			return true
		}
	}
	return false
}

func commandManagedPipe(info *types.Info, call *ast.CallExpr) bool {
	if info == nil || call == nil {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "os/exec" {
		return false
	}
	switch function.Name() {
	case "StdinPipe", "StdoutPipe", "StderrPipe":
		selection := info.Selections[selector]
		return selection != nil && namedTypeName(selection.Recv()) == "Cmd"
	default:
		return false
	}
}

func immediateFollowingGuard(statements []ast.Stmt, index int) *ast.IfStmt {
	if index < 0 || index + 1 >= len(statements) {
		return nil
	}
	guard, _ := statements[index + 1].(*ast.IfStmt)
	return guard
}

func followingAcquisitionErrorGuard(
	info *types.Info,
	statements []ast.Stmt,
	assignment *ast.AssignStmt,
) *ast.IfStmt {
	errorObject := assignmentErrorObject(info, assignment)
	if errorObject == nil {
		return nil
	}
	var first *ast.IfStmt
	for _, statement := range statements {
		guard, ok := statement.(*ast.IfStmt)
		if !ok || !returningErrorBranch(info, guard, errorObject) {
			return first
		}
		if first == nil {
			first = guard
		}
		if returningNonNilErrorGuard(info, guard, errorObject) {
			return guard
		}
	}
	return first
}

func returningErrorBranch(info *types.Info, guard *ast.IfStmt, errorObject types.Object) bool {
	if info == nil ||
		guard == nil ||
		errorObject == nil ||
		guard.Init != nil ||
		guard.Else != nil ||
		guard.Body == nil ||
		len(guard.Body.List) == 0 ||
		!expressionUsesObject(info, guard.Cond, errorObject) {
		return false
	}
	_, returns := guard.Body.List[len(guard.Body.List) - 1].(*ast.ReturnStmt)
	return returns
}

func localCloserObligationStart(
	graph *cfg.CFG,
	info *types.Info,
	candidate localCloserCandidate,
) (obligationStart, bool) {
	if constructionFailureProvesNil(graph, info, candidate.acquisitionGuard, candidate.object) {
		return obligationStart{}, false
	}
	errorObject := assignmentErrorObject(info, candidate.statement)
	if successfulAcquisitionTransfers(info, candidate.guard, errorObject, candidate.object) {
		return obligationStart{}, false
	}
	if errorObject != nil && returningNonNilErrorGuard(info, candidate.guard, errorObject) {
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

func constructionFailureProvesNil(
	graph *cfg.CFG,
	info *types.Info,
	guard *ast.IfStmt,
	resourceObject types.Object,
) bool {
	if graph == nil ||
		info == nil ||
		guard == nil ||
		resourceObject == nil ||
		guard.Init != nil ||
		guard.Else != nil ||
		!trueWhenObjectIsNonNil(info, guard.Cond, resourceObject) {
		return false
	}
	var thenBlock *cfg.Block
	var doneBlock *cfg.Block
	for _, block := range graph.Blocks {
		if !block.Live || block.Stmt != guard {
			continue
		}
		switch block.Kind {
		case cfg.KindIfThen:
			thenBlock = block
		case cfg.KindIfDone:
			doneBlock = block
		}
	}
	if thenBlock == nil || doneBlock == nil {
		return false
	}
	seen := make(map[*cfg.Block]bool)
	work := []*cfg.Block{thenBlock}
	for len(work) > 0 {
		block := work[len(work) - 1]
		work = work[:len(work) - 1]
		if block == nil || !block.Live || seen[block] {
			continue
		}
		if block == doneBlock {
			return false
		}
		seen[block] = true
		work = append(work, block.Succs...)
	}
	return true
}

func trueWhenObjectIsNonNil(info *types.Info, expression ast.Expr, object types.Object) bool {
	binary, _ := ast.Unparen(expression).(*ast.BinaryExpr)
	if binary == nil {
		return false
	}
	if binary.Op == token.LOR {
		return trueWhenObjectIsNonNil(info, binary.X, object) ||
			trueWhenObjectIsNonNil(info, binary.Y, object)
	}
	if binary.Op != token.NEQ {
		return false
	}
	nilObject := types.Universe.Lookup("nil")
	return directObject(info, binary.X) == object &&
		directObject(info, binary.Y) == nilObject ||
		directObject(info, binary.Y) == object && directObject(info, binary.X) == nilObject
}

func successfulAcquisitionTransfers(
	info *types.Info,
	guard *ast.IfStmt,
	errorObject types.Object,
	resourceObject types.Object,
) bool {
	if info == nil ||
		guard == nil ||
		errorObject == nil ||
		resourceObject == nil ||
		guard.Init != nil ||
		guard.Else != nil ||
		guard.Body == nil ||
		len(guard.Body.List) != 1 {
		return false
	}
	comparison, _ := ast.Unparen(guard.Cond).(*ast.BinaryExpr)
	if comparison == nil || comparison.Op != token.EQL {
		return false
	}
	nilObject := types.Universe.Lookup("nil")
	if !(directObject(info, comparison.X) == errorObject &&
		directObject(info, comparison.Y) == nilObject ||
		directObject(info, comparison.Y) == errorObject &&
			directObject(info, comparison.X) == nilObject) {
		return false
	}
	statement, _ := guard.Body.List[0].(*ast.ReturnStmt)
	if statement == nil {
		return false
	}
	for _, expression := range statement.Results {
		if directObject(info, expression) == resourceObject {
			return true
		}
	}
	return false
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
