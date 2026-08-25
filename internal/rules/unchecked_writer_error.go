package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/types/typeutil"
)

type uncheckedWriterErrorRule struct{}

type writerFinalizerSpec struct {
	packagePath string
	typeName string
	methodName string
	constructor string
}

var writerFinalizerSpecs = []writerFinalizerSpec{
	{packagePath: "archive/tar", typeName: "Writer", methodName: "Close"},
	{packagePath: "archive/tar", typeName: "Writer", methodName: "Flush"},
	{packagePath: "archive/zip", typeName: "Writer", methodName: "Close"},
	{packagePath: "archive/zip", typeName: "Writer", methodName: "Flush"},
	{packagePath: "bufio", typeName: "Writer", methodName: "Flush"},
	{packagePath: "compress/flate", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/flate", typeName: "Writer", methodName: "Flush"},
	{packagePath: "compress/gzip", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/gzip", typeName: "Writer", methodName: "Flush"},
	{packagePath: "compress/lzw", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/zlib", typeName: "Writer", methodName: "Close"},
	{packagePath: "compress/zlib", typeName: "Writer", methodName: "Flush"},
	{packagePath: "encoding/xml", typeName: "Encoder", methodName: "Close"},
	{packagePath: "encoding/xml", typeName: "Encoder", methodName: "Flush"},
	{packagePath: "mime/multipart", typeName: "Writer", methodName: "Close"},
	{packagePath: "mime/quotedprintable", typeName: "Writer", methodName: "Close"},
	{packagePath: "text/tabwriter", typeName: "Writer", methodName: "Flush"},
}

var writerEncoderSpecs = []writerFinalizerSpec{
	{packagePath: "encoding/ascii85", methodName: "Close", constructor: "NewEncoder"},
	{packagePath: "encoding/base32", methodName: "Close", constructor: "NewEncoder"},
	{packagePath: "encoding/base64", methodName: "Close", constructor: "NewEncoder"},
}

// NewUncheckedWriterErrorRule constructs the buffered-writer finalization rule
// for product registry composition.
func NewUncheckedWriterErrorRule() Rule {
	return uncheckedWriterErrorRule{}
}

func (uncheckedWriterErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "unchecked-writer-error",
		Summary: "detects discarded errors from buffered writer finalization",
		Documentation: "Buffered, compressed, archive, multipart, and encoded writers can report their first failed output or emit required trailers only from Flush or Close. Discarding that result can report success while leaving output truncated or structurally incomplete. The rule targets exact standard-library finalizers whose documented contract writes pending data or required framing, including direct stable values returned by the streaming encoder constructors. Finalizers are excluded when a stable bufio or tabwriter chain terminates at an exact bytes.Buffer, strings.Builder, or selected-module writer whose exact Write error result is proven nil; gzip additionally requires an unmodified default header. A deferred archive/tar Close is also excluded when a later straight-line Close on the same stable writer passes its error to a consumer and no later writer use remains, because subsequent Close calls are no-ops. Exact test recovery guards that fail when an immediately following finalizer is the function body's terminal action and does not panic are treated as expected-panic assertions rather than successful error discards.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only exact standard-library writer finalizers with an error result are covered; user-defined writers and unproven interface-dispatched finalizers remain outside the contract.",
			"The in-memory exclusion follows stable local bufio, gzip, and tabwriter constructor or straight-line Reset/Init chains to exact bytes.Buffer and strings.Builder sinks or selected-module concrete writers whose exact Write error result is proven nil; caller-owned, conditionally rebound, interface-typed, escaped, cyclic, package-variant-disagreeing, and unproven chains remain conservative.",
			"Gzip in-memory exclusions require the default Header to remain unmodified because invalid Name, Comment, or Extra values can fail independently of the sink.",
			"Stable in-memory fmt consumers accept basic values and exact time.Time and io/fs.FileMode values; user-defined formatting callbacks remain conservative because they can capture and rebind the writer.",
			"The redundant deferred archive/tar exclusion requires a writer declared directly in the same block, a later same-block Close whose error is passed to another call, no intervening return or branch, and no later or escaping receiver use.",
			"Expected-panic suppression requires an immediately preceding unconditional recovery defer in a _test.go file whose exact recovered == nil branch calls a testing failure method and whose finalizer is the function body's terminal action; nested blocks, conditional guards, intervening statements, lookalike recovery, asynchronous calls, and non-test files remain diagnostics.",
			"Streaming encoder coverage requires a direct constructor result or a direct identifier initialized by encoding/ascii85, encoding/base32, or encoding/base64 NewEncoder and not reassigned before Close.",
			"encoding/csv.Writer.Flush returns no error; unchecked-csv-writer-error owns its separate Flush then Error observation protocol.",
			"No fix is offered because correct propagation from a deferred, asynchronous, or ordinary call depends on the surrounding function contract.",
		},
		Examples: []Example{
			{
				Title: "Propagate gzip finalization failures",
				Incorrect: "defer writer.Close()",
				Correct: "if err := writer.Close(); err != nil { return err }",
			},
		},
	}
}

func (uncheckedWriterErrorRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.typesContext == nil || ctx.Info() == nil || ctx.Body() == nil {
		return nil, fmt.Errorf("unchecked-writer-error requires complete type information")
	}
	findings := make([]Finding, 0)
	var runErr error
	ast.Inspect(
		ctx.Body(),
		func(node ast.Node) bool {
			if node == nil || runErr != nil {
				return false
			}
			if literal, nested := node.(*ast.FuncLit);
				nested && literal.Body != ctx.Body() {
				return false
			}
			switch node.(type) {
			case *ast.ExprStmt, *ast.AssignStmt, *ast.GoStmt, *ast.DeferStmt:
				var current []Finding
				current, runErr = runUncheckedWriterError(ctx, node)
				findings = append(findings, current...)
			}
			return true
		},
	)
	return findings, runErr
}

func runUncheckedWriterError(ctx *ControlFlowContext, node ast.Node) ([]Finding, error) {
	typesContext := ctx.typesContext
	call, discarded := discardedCall(node)
	if !discarded {
		return nil, nil
	}
	spec, matched := writerFinalizer(typesContext, call)
	if !matched {
		return nil, nil
	}
	if expectedPanicWriterFinalizer(typesContext, node, call) {
		return nil, nil
	}
	if redundantDeferredTarFinalizer(typesContext, node, call, spec) {
		return nil, nil
	}
	if infallibleWriterFinalizer(ctx, node, call, spec) {
		return nil, nil
	}
	range_, err := typesContext.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "unchecked-writer-error",
			Message: fmt.Sprintf(
				"error returned by %s is discarded; buffered output may be incomplete",
				spec.target(),
			),
			Range: range_,
			Help: "observe and propagate the finalization error before reporting success",
		},
	}, nil
}

func expectedPanicWriterFinalizer(
	ctx *TypesContext,
	discard ast.Node,
	finalizer *ast.CallExpr,
) bool {
	if ctx == nil ||
		ctx.File() == nil ||
		ctx.Info() == nil ||
		ctx.Syntax() == nil ||
		finalizer == nil ||
		!strings.HasSuffix(ctx.File().Path(), "_test.go") {
		return false
	}
	switch discard.(type) {
	case *ast.ExprStmt, *ast.AssignStmt:
	default:
		return false
	}
	value := ctx.memoized(
		"unchecked-writer-error/expected-panic-finalizer-v1",
		func() any {
			return expectedPanicWriterFinalizers(ctx.Info(), ctx.Syntax())
		},
	)
	expected, _ := value.(map[*ast.CallExpr]struct{})
	_, matched := expected[finalizer]
	return matched
}

func expectedPanicWriterFinalizers(info *types.Info, syntax *ast.File) map[*ast.CallExpr]struct{} {
	result := make(map[*ast.CallExpr]struct{})
	if info == nil || syntax == nil {
		return result
	}
	parents := writerASTParents(syntax)
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			block, _ := node.(*ast.BlockStmt)
			if block == nil || !writerFunctionBody(parents, block) {
				return true
			}
			for index, statement := range block.List {
				deferred, _ := statement.(*ast.DeferStmt)
				if deferred == nil ||
					!expectedPanicRecoveryGuard(info, deferred) ||
					index + 2 != len(block.List) {
					continue
				}
				if call, discarded := discardedCall(block.List[index + 1]);
					discarded {
					result[call] = struct{}{}
				}
			}
			return true
		},
	)
	return result
}

func writerFunctionBody(parents map[ast.Node]ast.Node, block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	switch owner := parents[block].(type) {
	case *ast.FuncDecl:
		return owner.Body == block
	case *ast.FuncLit:
		return owner.Body == block
	default:
		return false
	}
}

func expectedPanicRecoveryGuard(info *types.Info, deferred *ast.DeferStmt) bool {
	if info == nil || deferred == nil || deferred.Call == nil || len(deferred.Call.Args) != 0 {
		return false
	}
	literal, _ := ast.Unparen(deferred.Call.Fun).(*ast.FuncLit)
	if literal == nil || literal.Body == nil || len(literal.Body.List) != 1 {
		return false
	}
	condition, _ := literal.Body.List[0].(*ast.IfStmt)
	if condition == nil ||
		condition.Else != nil ||
		!directTestingFailure(info, condition.Body) {
		return false
	}
	assignment, _ := condition.Init.(*ast.AssignStmt)
	if assignment == nil || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	recovered := directObject(info, assignment.Lhs[0])
	recoverCall, _ := ast.Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if recovered == nil || recoverCall == nil || len(recoverCall.Args) != 0 {
		return false
	}
	recoverIdentifier, _ := ast.Unparen(recoverCall.Fun).(*ast.Ident)
	recoverBuiltin, _ := info.ObjectOf(recoverIdentifier).(*types.Builtin)
	if recoverBuiltin == nil || recoverBuiltin.Name() != "recover" {
		return false
	}
	comparison, _ := ast.Unparen(condition.Cond).(*ast.BinaryExpr)
	if comparison == nil || comparison.Op != token.EQL {
		return false
	}
	return recoveredNilComparison(info, comparison.X, comparison.Y, recovered) ||
		recoveredNilComparison(info, comparison.Y, comparison.X, recovered)
}

func recoveredNilComparison(
	info *types.Info,
	recoveredExpression ast.Expr,
	nilExpression ast.Expr,
	recovered types.Object,
) bool {
	if info == nil || recovered == nil || directObject(info, recoveredExpression) != recovered {
		return false
	}
	nilIdentifier, _ := ast.Unparen(nilExpression).(*ast.Ident)
	nilBuiltin, _ := info.ObjectOf(nilIdentifier).(*types.Nil)
	return nilBuiltin != nil
}

func redundantDeferredTarFinalizer(
	ctx *TypesContext,
	discard ast.Node,
	finalizer *ast.CallExpr,
	spec writerFinalizerSpec,
) bool {
	if ctx == nil ||
		ctx.Info() == nil ||
		ctx.Syntax() == nil ||
		finalizer == nil ||
		spec.packagePath != "archive/tar" ||
		spec.typeName != "Writer" ||
		spec.methodName != "Close" {
		return false
	}
	if _, deferred := discard.(*ast.DeferStmt); !deferred {
		return false
	}
	value := ctx.memoized(
		"unchecked-writer-error/redundant-deferred-tar-v1",
		func() any {
			return redundantDeferredTarFinalizers(ctx.Info(), ctx.Syntax())
		},
	)
	redundant, _ := value.(map[*ast.CallExpr]struct{})
	_, matched := redundant[finalizer]
	return matched
}

func redundantDeferredTarFinalizers(info *types.Info, syntax *ast.File) map[*ast.CallExpr]struct{} {
	result := make(map[*ast.CallExpr]struct{})
	if info == nil || syntax == nil {
		return result
	}
	parents := writerASTParents(syntax)
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			block, _ := node.(*ast.BlockStmt)
			if block == nil {
				return true
			}
			unstable, lastUses := redundantTarBlockFacts(info, block, parents)
			observed := redundantTarObservedCloses(info, block)
			bypasses := make([]int, len(block.List) + 1)
			for index, statement := range block.List {
				bypasses[index + 1] = bypasses[index]
				if writerStatementMayBypassFollowing(statement) {
					bypasses[index + 1]++
				}
			}
			for index, statement := range block.List {
				deferred, _ := statement.(*ast.DeferStmt)
				if deferred == nil || deferred.Call == nil {
					continue
				}
				receiver, matched := exactTarWriterCloseReceiver(
					info,
					deferred.Call,
				)
				if !matched {
					continue
				}
				if !writerDirectLocalBinding(info, block, parents, receiver) {
					continue
				}
				if _, unsafe := unstable[receiver]; unsafe {
					continue
				}
				candidates := observed[receiver]
				candidate := sort.Search(
					len(candidates),
					func(candidate int) bool {
						return candidates[candidate].statement > index
					},
				)
				if candidate == len(candidates) {
					continue
				}
				close := candidates[candidate]
				if bypasses[close.statement] != bypasses[index + 1] ||
					lastUses[receiver] >= close.call.End() {
					continue
				}
				result[deferred.Call] = struct{}{}
			}
			return true
		},
	)
	return result
}

func writerDirectLocalBinding(
	info *types.Info,
	block *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
	receiver types.Object,
) bool {
	if info == nil || block == nil || receiver == nil {
		return false
	}
	variable, local := receiver.(*types.Var)
	if !local || variable.IsField() {
		return false
	}
	for identifier, object := range info.Defs {
		if object != receiver {
			continue
		}
		for ancestor := parents[identifier]; ancestor != nil; ancestor = parents[ancestor] {
			if owner, ok := ancestor.(*ast.BlockStmt); ok {
				return owner == block
			}
		}
		return false
	}
	return false
}

type observedTarWriterFinalizer struct {
	statement int
	call *ast.CallExpr
}

func redundantTarObservedCloses(
	info *types.Info,
	block *ast.BlockStmt,
) map[types.Object][]observedTarWriterFinalizer {
	result := make(map[types.Object][]observedTarWriterFinalizer)
	if info == nil || block == nil {
		return result
	}
	for index, statement := range block.List {
		for _, observed := range observedTarWriterCloses(info, statement) {
			receiver, matched := exactTarWriterCloseReceiver(info, observed)
			if !matched {
				continue
			}
			result[receiver] = append(
				result[receiver],
				observedTarWriterFinalizer{statement: index, call: observed},
			)
		}
	}
	return result
}

func redundantTarBlockFacts(
	info *types.Info,
	block *ast.BlockStmt,
	parents map[ast.Node]ast.Node,
) (map[types.Object]struct{}, map[types.Object]token.Pos) {
	unstable := make(map[types.Object]struct{})
	lastUses := make(map[types.Object]token.Pos)
	if info == nil || block == nil {
		return unstable, lastUses
	}
	ast.Inspect(
		block,
		func(node ast.Node) bool {
			if node == nil {
				return false
			}
			identifier, _ := node.(*ast.Ident)
			if identifier == nil {
				return true
			}
			object := directObject(info, identifier)
			if object == nil {
				return true
			}
			if identifier.Pos() > lastUses[object] {
				lastUses[object] = identifier.Pos()
			}
			if info.Defs[identifier] == object {
				return true
			}
			selector, _ := parents[identifier].(*ast.SelectorExpr)
			call, _ := parents[selector].(*ast.CallExpr)
			if selector == nil ||
				selector.X != identifier ||
				call == nil ||
				ast.Unparen(call.Fun) != selector {
				unstable[object] = struct{}{}
				return true
			}
			for ancestor := ast.Node(identifier); ancestor != block; {
				ancestor = parents[ancestor]
				if ancestor == nil {
					unstable[object] = struct{}{}
					return true
				}
				if _, closure := ancestor.(*ast.FuncLit); closure {
					unstable[object] = struct{}{}
					return true
				}
			}
			return true
		},
	)
	return unstable, lastUses
}

func exactTarWriterCloseReceiver(info *types.Info, call *ast.CallExpr) (types.Object, bool) {
	if info == nil || call == nil {
		return nil, false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil, false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return nil, false
	}
	function, _ := selection.Obj().(*types.Func)
	spec, matched := standardWriterFinalizer(function)
	if !matched ||
		spec.packagePath != "archive/tar" ||
		spec.typeName != "Writer" ||
		spec.methodName != "Close" {
		return nil, false
	}
	receiver := writerFinalizerReceiver(info, call)
	return receiver, receiver != nil
}

func observedTarWriterCloses(info *types.Info, statement ast.Stmt) []*ast.CallExpr {
	if info == nil || statement == nil {
		return nil
	}
	expression, _ := statement.(*ast.ExprStmt)
	if expression == nil {
		return nil
	}
	outer, _ := ast.Unparen(expression.X).(*ast.CallExpr)
	if outer == nil {
		return nil
	}
	observed := make([]*ast.CallExpr, 0)
	for _, argument := range outer.Args {
		ast.Inspect(
			argument,
			func(node ast.Node) bool {
				if node == nil {
					return false
				}
				if _, closure := node.(*ast.FuncLit); closure {
					return false
				}
				call, _ := node.(*ast.CallExpr)
				if call == nil {
					return true
				}
				if _, matched := exactTarWriterCloseReceiver(info, call); matched {
					observed = append(observed, call)
				}
				return true
			},
		)
	}
	return observed
}

func writerStatementMayBypassFollowing(statement ast.Stmt) bool {
	if statement == nil {
		return true
	}
	bypasses := false
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			if bypasses || node == nil {
				return false
			}
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			switch node.(type) {
			case *ast.ReturnStmt, *ast.BranchStmt:
				bypasses = true
				return false
			default:
				return true
			}
		},
	)
	return bypasses
}

func infallibleWriterFinalizer(
	ctx *ControlFlowContext,
	discard ast.Node,
	finalizer *ast.CallExpr,
	spec writerFinalizerSpec,
) bool {
	if ctx == nil ||
		ctx.typesContext == nil ||
		ctx.Info() == nil ||
		ctx.typesContext.Syntax() == nil ||
		finalizer == nil {
		return false
	}
	typesContext := ctx.typesContext
	if emptyInMemoryTarFinalizer(typesContext, discard, finalizer, spec) {
		return true
	}
	if !infallibleSinkFinalizer(spec) {
		return false
	}
	receiverExpression := writerFinalizerReceiverExpression(ctx.Info(), finalizer)
	if receiverExpression == nil {
		return false
	}
	constructor, _ := ast.Unparen(receiverExpression).(*ast.CallExpr)
	bindings := map[types.Object]*ast.CallExpr(nil)
	helperSinks := map[*ast.CallExpr]ast.Expr(nil)
	if constructor == nil {
		if directObject(ctx.Info(), receiverExpression) == nil {
			return false
		}
		binding := stableWriterConstructor(typesContext, finalizer)
		constructor = binding.candidate
		bindings = binding.candidates
		helperSinks = binding.helperSinks
	}
	return infallibleWriterConstructor(
		ctx.Info(),
		ctx,
		constructor,
		bindings,
		helperSinks,
		make(map[*ast.CallExpr]struct{}),
	)
}

func emptyInMemoryTarFinalizer(
	ctx *TypesContext,
	discard ast.Node,
	finalizer *ast.CallExpr,
	spec writerFinalizerSpec,
) bool {
	if ctx == nil ||
		ctx.Info() == nil ||
		ctx.Syntax() == nil ||
		finalizer == nil ||
		spec.packagePath != "archive/tar" ||
		spec.typeName != "Writer" ||
		spec.methodName != "Close" ||
		writerDiscardTiming(discard) != writerCallImmediate {
		return false
	}
	receiver := writerFinalizerReceiverExpression(ctx.Info(), finalizer)
	if receiver == nil {
		return false
	}
	constructor, _ := ast.Unparen(receiver).(*ast.CallExpr)
	if constructor != nil {
		return emptyTarConstructorHasInfallibleSink(ctx.Info(), constructor)
	}
	receiverObject := directObject(ctx.Info(), receiver)
	if receiverObject == nil {
		return false
	}
	binding := stableWriterConstructor(ctx, finalizer)
	if binding.candidate == nil || binding.timing != writerCallImmediate {
		return false
	}
	if writerReceiverUsedBetween(
		ctx.Info(),
		ctx.Syntax(),
		receiverObject,
		binding.candidate.End(),
		finalizer.Pos(),
	) {
		return false
	}
	return emptyTarConstructorHasInfallibleSink(ctx.Info(), binding.candidate)
}

func writerDiscardTiming(node ast.Node) writerCallTiming {
	switch node.(type) {
	case *ast.DeferStmt:
		return writerCallDeferred
	case *ast.GoStmt:
		return writerCallAsynchronous
	default:
		return writerCallImmediate
	}
}

func emptyTarConstructorHasInfallibleSink(info *types.Info, constructor *ast.CallExpr) bool {
	if info == nil || constructor == nil || len(constructor.Args) != 1 {
		return false
	}
	function := typeutil.StaticCallee(info, constructor)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "archive/tar" &&
		function.Name() == "NewWriter" &&
		infallibleMemoryWriter(info.TypeOf(constructor.Args[0]))
}

func writerReceiverUsedBetween(
	info *types.Info,
	syntax ast.Node,
	receiver types.Object,
	start token.Pos,
	end token.Pos,
) bool {
	if info == nil || syntax == nil || receiver == nil || !start.IsValid() || !end.IsValid() {
		return true
	}
	used := false
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			if used || node == nil {
				return false
			}
			if node.End() <= start || node.Pos() >= end {
				return true
			}
			identifier, _ := node.(*ast.Ident)
			if identifier != nil && directObject(info, identifier) == receiver {
				used = true
				return false
			}
			return true
		},
	)
	return used
}

func infallibleSinkFinalizer(spec writerFinalizerSpec) bool {
	switch spec.packagePath {
	case "bufio", "compress/gzip", "text/tabwriter":
		return true
	default:
		return false
	}
}

func infallibleWriterConstructor(
	info *types.Info,
	ctx *ControlFlowContext,
	constructor *ast.CallExpr,
	bindings map[types.Object]*ast.CallExpr,
	helperSinks map[*ast.CallExpr]ast.Expr,
	visiting map[*ast.CallExpr]struct{},
) bool {
	if info == nil || constructor == nil {
		return false
	}
	if _, cycle := visiting[constructor]; cycle {
		return false
	}
	visiting[constructor] = struct{}{}
	defer delete(visiting, constructor)
	sink := writerSinkExpressionWithHelpers(info, constructor, helperSinks)
	if sink == nil {
		return false
	}
	if infallibleMemoryWriter(info.TypeOf(sink)) {
		return true
	}
	if infallibleWriteMethod(ctx, info.TypeOf(sink)) {
		return true
	}
	inline, _ := ast.Unparen(sink).(*ast.CallExpr)
	if inline != nil {
		return infallibleWriterConstructor(
			info,
			ctx,
			inline,
			bindings,
			helperSinks,
			visiting,
		)
	}
	nested := bindings[directObject(info, sink)]
	return nested != nil &&
		infallibleWriterConstructor(info, ctx, nested, bindings, helperSinks, visiting)
}

func infallibleWriteMethod(ctx *ControlFlowContext, type_ types.Type) bool {
	if ctx == nil || type_ == nil {
		return false
	}
	object, _, _ := types.LookupFieldOrMethod(type_, true, ctx.Package(), "Write")
	method, _ := object.(*types.Func)
	if method == nil {
		return false
	}
	signature, _ := types.Unalias(method.Type()).(*types.Signature)
	if signature == nil ||
		signature.Params() == nil ||
		signature.Params().Len() != 1 ||
		signature.Results() == nil ||
		signature.Results().Len() != 2 ||
		!types.Identical(
			signature.Params().At(0).Type(),
			types.NewSlice(types.Typ[types.Byte]),
		) ||
		!types.Identical(signature.Results().At(0).Type(), types.Typ[types.Int]) ||
		!isBuiltinErrorType(signature.Results().At(1).Type()) {
		return false
	}
	return ctx.ResultStateFor(method, 1) == NilStateNil
}

func infallibleMemoryWriter(type_ types.Type) bool {
	return namedReceiver(type_, "bytes", "Buffer") || namedReceiver(type_, "strings", "Builder")
}

func trackedInfallibleWriterType(type_ types.Type) bool {
	return namedReceiver(type_, "bufio", "Writer") ||
		namedReceiver(type_, "compress/gzip", "Writer") ||
		namedReceiver(type_, "text/tabwriter", "Writer")
}

func writerSinkExpression(info *types.Info, call *ast.CallExpr) ast.Expr {
	return writerSinkExpressionWithHelpers(info, call, nil)
}

func writerSinkExpressionWithHelpers(
	info *types.Info,
	call *ast.CallExpr,
	helperSinks map[*ast.CallExpr]ast.Expr,
) ast.Expr {
	if info == nil || call == nil {
		return nil
	}
	if sink := helperSinks[call]; sink != nil {
		return sink
	}
	if index, matched := writerConstructorSinkIndex(typeutil.StaticCallee(info, call));
		matched && index < len(call.Args) {
		return call.Args[index]
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil
	}
	selection := info.Selections[selector]
	if selection == nil {
		return nil
	}
	function, _ := selection.Obj().(*types.Func)
	if !exactWriterRebind(function) {
		return nil
	}
	switch selection.Kind() {
	case types.MethodVal:
		if len(call.Args) != 0 {
			return call.Args[0]
		}
	case types.MethodExpr:
		if len(call.Args) > 1 {
			return call.Args[1]
		}
	}
	return nil
}

func writerConstructorSinkIndex(function *types.Func) (int, bool) {
	if function == nil || function.Pkg() == nil {
		return 0, false
	}
	switch function.Pkg().Path() {
	case "bufio":
		return 0, function.Name() == "NewWriter" || function.Name() == "NewWriterSize"
	case "compress/gzip":
		return 0, function.Name() == "NewWriter" || function.Name() == "NewWriterLevel"
	case "text/tabwriter":
		return 0, function.Name() == "NewWriter"
	default:
		return 0, false
	}
}

type stableWriterBinding struct {
	candidate *ast.CallExpr
	candidates map[types.Object]*ast.CallExpr
	helperSinks map[*ast.CallExpr]ast.Expr
	timing writerCallTiming
}

type stableWriterQuery struct {
	call *ast.CallExpr
	receiver types.Object
	boundary token.Pos
	lateUntil token.Pos
	repeatBodies []writerLoopBody
}

type writerCallTiming uint8

const (
	writerCallImmediate writerCallTiming = iota
	writerCallDeferred
	writerCallAsynchronous
)

type deferredWriterEffect struct {
	position token.Pos
	candidates map[*ast.CallExpr]struct{}
}

type stableWriterState struct {
	candidates map[types.Object]*ast.CallExpr
	dependents map[*ast.CallExpr]map[*ast.CallExpr]struct{}
	invalidated map[*ast.CallExpr]struct{}
	invalidationPositions map[*ast.CallExpr][]token.Pos
	assigned map[types.Object]struct{}
	bindingChanges map[types.Object][]token.Pos
	closures map[types.Object]map[*ast.CallExpr]struct{}
	mutationClosures map[types.Object]map[*ast.CallExpr]struct{}
	closureMutationParameters map[types.Object]map[int]struct{}
	mutationSummaries writerMutationSummaries
	helperSinks map[*ast.CallExpr]ast.Expr
	callTiming map[*ast.CallExpr]writerCallTiming
	straightLineRebinds map[*ast.CallExpr]struct{}
	deferredEffects []deferredWriterEffect
}

type writerMutationSummaries struct {
	functions map[*types.Func]map[int]struct{}
	literals map[*ast.FuncLit]map[int]struct{}
	returnedSinks map[*types.Func]writerReturnedSinkSummary
}

type writerReturnedSinkSummary struct {
	parameter int
	constructor bool
}

type writerMutationSummaryNode struct {
	function *types.Func
	literal *ast.FuncLit
	type_ *ast.FuncType
	body *ast.BlockStmt
	mutated map[int]struct{}
	reverse map[int][]writerMutationTarget
}

type writerMutationTarget struct {
	node *writerMutationSummaryNode
	parameter int
}

func stableWriterConstructor(ctx *TypesContext, finalizer *ast.CallExpr) stableWriterBinding {
	if ctx == nil || finalizer == nil {
		return stableWriterBinding{}
	}
	value := ctx.memoized(
		"unchecked-writer-error/infallible-writer-stability-v2",
		func() any {
			return stableWriterConstructors(
				ctx.Info(),
				ctx.Syntax(),
				packageWriterMutationSummaries(
					ctx.Info(),
					ctx.PackageSyntax(),
					ctx.Syntax(),
				),
			)
		},
	)
	constructors, _ := value.(map[*ast.CallExpr]stableWriterBinding)
	return constructors[finalizer]
}

func stableWriterConstructors(
	info *types.Info,
	syntax *ast.File,
	mutationSummaries writerMutationSummaries,
) map[*ast.CallExpr]stableWriterBinding {
	constructors := stableWriterConstructorsInNode(info, syntax, mutationSummaries)
	if info == nil || syntax == nil {
		return constructors
	}
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			literal, _ := node.(*ast.FuncLit)
			if literal == nil {
				return true
			}
			for call, candidate := range
				stableWriterConstructorsInNode(
					info,
					literal.Body,
					mutationSummaries,
				) {
				constructors[call] = candidate
			}
			return true
		},
	)
	return constructors
}

func stableWriterConstructorsInNode(
	info *types.Info,
	syntax ast.Node,
	mutationSummaries writerMutationSummaries,
) map[*ast.CallExpr]stableWriterBinding {
	constructors := make(map[*ast.CallExpr]stableWriterBinding)
	if info == nil || syntax == nil {
		return constructors
	}
	queries, callTiming := stableWriterQueries(info, syntax)
	if len(queries) == 0 {
		return constructors
	}
	sort.Slice(
		queries,
		func(left, right int) bool {
			if queries[left].boundary != queries[right].boundary {
				return queries[left].boundary < queries[right].boundary
			}
			return queries[left].call.Pos() < queries[right].call.Pos()
		},
	)
	state := stableWriterState{
		candidates: make(map[types.Object]*ast.CallExpr),
		dependents: make(map[*ast.CallExpr]map[*ast.CallExpr]struct{}),
		invalidated: make(map[*ast.CallExpr]struct{}),
		invalidationPositions: make(map[*ast.CallExpr][]token.Pos),
		assigned: make(map[types.Object]struct{}),
		bindingChanges: make(map[types.Object][]token.Pos),
		closures: make(map[types.Object]map[*ast.CallExpr]struct{}),
		mutationClosures: make(map[types.Object]map[*ast.CallExpr]struct{}),
		closureMutationParameters: make(map[types.Object]map[int]struct{}),
		mutationSummaries: mutationSummaries,
		helperSinks: make(map[*ast.CallExpr]ast.Expr),
		callTiming: callTiming,
		straightLineRebinds: straightLineWriterRebinds(syntax),
	}
	lateConstructors := make(map[*ast.CallExpr]*ast.CallExpr)
	nextQuery := 0
	resolve := func(position token.Pos) {
		for nextQuery < len(queries) && queries[nextQuery].boundary <= position {
			query := queries[nextQuery]
			candidate := state.candidates[query.receiver]
			if candidate != nil {
				if _, invalid := state.invalidated[candidate]; !invalid {
					constructors[query.call] = stableWriterBinding{
						candidate: candidate,
						candidates: stableWriterCandidates(state),
						helperSinks: cloneWriterHelperSinks(
							state.helperSinks,
						),
						timing: state.callTiming[query.call],
					}
					if query.lateUntil.IsValid() {
						lateConstructors[query.call] = candidate
					}
				}
			}
			nextQuery++
		}
	}
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			resolve(node.Pos())
			return state.observe(info, node)
		},
	)
	resolve(token.Pos(^uint(0) >> 1))
	queriesByCall := make(map[*ast.CallExpr]stableWriterQuery, len(queries))
	for _, query := range queries {
		queriesByCall[query.call] = query
	}
	for call, candidate := range lateConstructors {
		query := queriesByCall[call]
		for _, invalidatedAt := range state.invalidationPositions[candidate] {
			if invalidatedAt > query.boundary &&
				invalidatedAt < query.lateUntil &&
				writerQueryRepeatsAfterInvalidation(
					query,
					candidate,
					invalidatedAt,
				) {
				delete(constructors, call)
				break
			}
		}
		if _, retained := constructors[call]; !retained || len(query.repeatBodies) == 0 {
			continue
		}
		for _, changedAt := range state.bindingChanges[query.receiver] {
			if changedAt > query.boundary &&
				changedAt < query.lateUntil &&
				writerQueryRepeatsAfterInvalidation(query, candidate, changedAt) {
				delete(constructors, call)
				break
			}
		}
	}
	for _, effect := range state.deferredEffects {
		for call, candidate := range lateConstructors {
			if call.Pos() >= effect.position {
				continue
			}
			if _, mutates := effect.candidates[candidate]; mutates {
				delete(constructors, call)
			}
		}
	}
	return constructors
}

func straightLineWriterRebinds(syntax ast.Node) map[*ast.CallExpr]struct{} {
	result := make(map[*ast.CallExpr]struct{})
	var recordBlock func(*ast.BlockStmt)
	recordBlock = func(block *ast.BlockStmt) {
		if block == nil {
			return
		}
		for _, statement := range block.List {
			switch statement := statement.(type) {
			case *ast.BlockStmt:
				recordBlock(statement)
			case *ast.ExprStmt:
				call, _ := ast.Unparen(statement.X).(*ast.CallExpr)
				if call == nil {
					continue
				}
				result[call] = struct{}{}
				literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit)
				if literal != nil {
					recordBlock(literal.Body)
				}
			}
		}
	}
	switch syntax := syntax.(type) {
	case *ast.File:
		for _, declaration := range syntax.Decls {
			function, _ := declaration.(*ast.FuncDecl)
			if function != nil {
				recordBlock(function.Body)
			}
		}
	case *ast.BlockStmt:
		recordBlock(syntax)
	}
	return result
}

func stableWriterCandidates(state stableWriterState) map[types.Object]*ast.CallExpr {
	result := make(map[types.Object]*ast.CallExpr, len(state.candidates))
	for object, candidate := range state.candidates {
		if _, invalid := state.invalidated[candidate]; !invalid {
			result[object] = candidate
		}
	}
	return result
}

func cloneWriterHelperSinks(source map[*ast.CallExpr]ast.Expr) map[*ast.CallExpr]ast.Expr {
	if len(source) == 0 {
		return nil
	}
	result := make(map[*ast.CallExpr]ast.Expr, len(source))
	for call, sink := range source {
		result[call] = sink
	}
	return result
}

func stableWriterQueries(
	info *types.Info,
	syntax ast.Node,
) ([]stableWriterQuery, map[*ast.CallExpr]writerCallTiming) {
	if info == nil || syntax == nil {
		return nil, nil
	}
	queries := make([]stableWriterQuery, 0)
	callTiming := make(map[*ast.CallExpr]writerCallTiming)
	loopBodies := writerLoopBodies(syntax)
	lateThroughFunction := token.Pos(^uint(0) >> 1)
	recordDeferred := func(body *ast.BlockStmt) {
		if body == nil {
			return
		}
		ast.Inspect(
			body,
			func(node ast.Node) bool {
				if node == nil {
					return false
				}
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				var call *ast.CallExpr
				timing := writerCallImmediate
				switch statement := node.(type) {
				case *ast.DeferStmt:
					call = statement.Call
					timing = writerCallDeferred
				case *ast.GoStmt:
					call = statement.Call
					timing = writerCallAsynchronous
				default:
					return true
				}
				callTiming[call] = timing
				receiver := writerFinalizerReceiver(info, call)
				if receiver != nil {
					repeatBodies := containingWriterLoopBodies(
						loopBodies,
						call.Pos(),
					)
					queries = append(
						queries,
						stableWriterQuery{
							call: call,
							receiver: receiver,
							// A deferred call evaluates and snapshots its
							// receiver when the defer statement executes.
							boundary: call.Pos(),
							lateUntil: lateThroughFunction,
							repeatBodies: repeatBodies,
						},
					)
				}
				return false
			},
		)
	}
	if body, _ := syntax.(*ast.BlockStmt); body != nil {
		recordDeferred(body)
	}
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.FuncDecl:
				recordDeferred(node.Body)
			case *ast.FuncLit:
				// A literal body executes only if and when the value is called;
				// source order cannot prove the receiver state at that point.
				return false
			}
			return true
		},
	)
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			if _, literal := node.(*ast.FuncLit); literal {
				return false
			}
			call, _ := node.(*ast.CallExpr)
			if call == nil {
				return true
			}
			if _, found := callTiming[call]; found {
				return true
			}
			receiver := writerFinalizerReceiver(info, call)
			if receiver != nil {
				repeatBodies := containingWriterLoopBodies(loopBodies, call.Pos())
				lateUntil := outermostWriterLoopEnd(repeatBodies)
				queries = append(
					queries,
					stableWriterQuery{
						call: call,
						receiver: receiver,
						boundary: call.Pos(),
						lateUntil: lateUntil,
						repeatBodies: repeatBodies,
					},
				)
			}
			return true
		},
	)
	return queries, callTiming
}

type writerLoopBody struct {
	start token.Pos
	end token.Pos
	body *ast.BlockStmt
}

func writerLoopBodies(syntax ast.Node) []writerLoopBody {
	bodies := make([]writerLoopBody, 0)
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			var body *ast.BlockStmt
			var start token.Pos
			switch loop := node.(type) {
			case *ast.ForStmt:
				body = loop.Body
				start = body.Pos()
				for _, repeated := range []ast.Node{loop.Cond, loop.Post} {
					if repeated != nil && repeated.Pos() < start {
						start = repeated.Pos()
					}
				}
			case *ast.RangeStmt:
				body = loop.Body
				start = body.Pos()
			default:
				return true
			}
			if body != nil {
				bodies = append(
					bodies,
					writerLoopBody{start: start, end: body.End(), body: body},
				)
			}
			return true
		},
	)
	return bodies
}

func containingWriterLoopBodies(bodies []writerLoopBody, position token.Pos) []writerLoopBody {
	result := make([]writerLoopBody, 0)
	for _, body := range bodies {
		if position >= body.start && position < body.end {
			result = append(result, body)
		}
	}
	return result
}

func outermostWriterLoopEnd(bodies []writerLoopBody) token.Pos {
	var end token.Pos
	for _, body := range bodies {
		if !end.IsValid() || body.end > end {
			end = body.end
		}
	}
	return end
}

func writerQueryRepeatsAfterInvalidation(
	query stableWriterQuery,
	candidate *ast.CallExpr,
	invalidatedAt token.Pos,
) bool {
	if len(query.repeatBodies) == 0 {
		return true
	}
	for _, repeated := range query.repeatBodies {
		if invalidatedAt < repeated.start || invalidatedAt >= repeated.end {
			continue
		}
		if !loopBackedgeStructurallyReachable(repeated.body, invalidatedAt) {
			continue
		}
		if !writerCandidateReacquiredBeforeQuery(repeated.body, candidate, query.call) {
			return true
		}
	}
	return false
}

func writerCandidateReacquiredBeforeQuery(
	body *ast.BlockStmt,
	candidate *ast.CallExpr,
	query *ast.CallExpr,
) bool {
	if body == nil || candidate == nil || query == nil {
		return false
	}
	for _, statement := range body.List {
		containsCandidate := statement.Pos() <= candidate.Pos() &&
			candidate.End() <= statement.End()
		containsQuery := statement.Pos() <= query.Pos() && query.End() <= statement.End()
		if containsCandidate && containsQuery {
			var nested *ast.BlockStmt
			switch statement := statement.(type) {
			case *ast.BlockStmt:
				nested = statement
			case *ast.ForStmt:
				nested = statement.Body
			case *ast.RangeStmt:
				nested = statement.Body
			}
			if nested != nil {
				return writerCandidateReacquiredBeforeQuery(
					nested,
					candidate,
					query,
				)
			}
			return false
		}
		if containsCandidate {
			switch statement.(type) {
			case *ast.AssignStmt, *ast.DeclStmt:
				return statement.End() <= query.Pos()
			default:
				return false
			}
		}
		if containsQuery {
			return false
		}
	}
	return false
}

func writerFinalizerReceiver(info *types.Info, call *ast.CallExpr) types.Object {
	if info == nil || call == nil {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil
	}
	selection := info.Selections[selector]
	if selection == nil {
		return nil
	}
	function, _ := selection.Obj().(*types.Func)
	if _, matched := standardWriterFinalizer(function); !matched {
		return nil
	}
	return directObject(info, writerFinalizerReceiverExpression(info, call))
}

func writerFinalizerReceiverExpression(info *types.Info, call *ast.CallExpr) ast.Expr {
	if info == nil || call == nil {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil
	}
	selection := info.Selections[selector]
	if selection == nil {
		return nil
	}
	function, _ := selection.Obj().(*types.Func)
	if _, matched := standardWriterFinalizer(function); !matched {
		return nil
	}
	switch selection.Kind() {
	case types.MethodVal:
		return selector.X
	case types.MethodExpr:
		if len(call.Args) != 0 {
			return call.Args[0]
		}
	}
	return nil
}

func standardWriterFinalizer(function *types.Func) (writerFinalizerSpec, bool) {
	if function == nil || function.Pkg() == nil {
		return writerFinalizerSpec{}, false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return writerFinalizerSpec{}, false
	}
	for _, spec := range writerFinalizerSpecs {
		if function.Pkg().Path() == spec.packagePath &&
			function.Name() == spec.methodName &&
			namedReceiver(signature.Recv().Type(), spec.packagePath, spec.typeName) {
			return spec, true
		}
	}
	return writerFinalizerSpec{}, false
}

func (state *stableWriterState) observe(info *types.Info, node ast.Node) bool {
	if state == nil || info == nil || node == nil {
		return false
	}
	var left []ast.Expr
	var right []ast.Expr
	switch node := node.(type) {
	case *ast.DeferStmt:
		state.recordDeferredEffect(info, node)
		return true
	case *ast.AssignStmt:
		left = node.Lhs
		right = node.Rhs
		for _, expression := range left {
			if directObject(info, expression) == nil {
				state.invalidateCandidateReferences(info, expression)
			}
		}
		for index, expression := range right {
			if index < len(left) && blankIdentifier(left[index]) {
				continue
			}
			if index >= len(left) ||
				directObject(info, left[index]) == nil ||
				packageScopeObject(directObject(info, left[index])) {
				state.invalidateCandidateReferences(info, expression)
				state.invalidateClosure(info, expression)
			}
		}
	case *ast.ValueSpec:
		left = make([]ast.Expr, len(node.Names))
		for index, name := range node.Names {
			left[index] = name
		}
		right = node.Values
	case *ast.RangeStmt:
		for _, expression := range []ast.Expr{node.Key, node.Value} {
			if object := directObject(info, expression); object != nil {
				state.assigned[object] = struct{}{}
				delete(state.candidates, object)
			}
		}
		return true
	case *ast.ReturnStmt:
		for _, expression := range node.Results {
			state.invalidateCandidateReferences(info, expression)
			state.invalidateClosure(info, expression)
		}
		return true
	case *ast.SendStmt:
		state.invalidateCandidateReferences(info, node.Value)
		state.invalidateClosure(info, node.Value)
		return true
	case *ast.CallExpr:
		if state.callTiming[node] == writerCallDeferred {
			return false
		}
		if receiver := writerRebindReceiverExpression(info, node); receiver != nil {
			if object := directObject(info, receiver); object != nil {
				previous := state.candidates[object]
				_, straightLine := state.straightLineRebinds[node]
				_, invalid := state.invalidated[previous]
				if previous != nil &&
					writerSinkExpressionWithHelpers(
						info,
						previous,
						state.helperSinks,
					) !=
						nil &&
					straightLine &&
					!invalid {
					state.invalidateCandidate(previous, node.Pos())
					state.candidates[object] = node
					state.registerCandidateDependencies(info, node)
					return false
				}
			}
		}
		if literal, _ := ast.Unparen(node.Fun).(*ast.FuncLit); literal != nil {
			state.observeExecutedBlock(info, literal.Body)
		}
		state.invalidateClosure(info, node.Fun)
		if candidate := writerRebindCandidate(info, node, state.candidates);
			candidate != nil {
			state.invalidateCandidate(candidate, node.Pos())
		}
		for index, argument := range node.Args {
			if standardWriterOperation(info, node, index) ||
				state.writerArgumentCannotRebind(info, node, index) {
				continue
			}
			if candidate := state.candidates[directObject(info, argument)];
				candidate != nil {
				state.invalidateCandidate(candidate, argument.Pos())
			}
			if !state.harmlessWriterClosureArgument(info, argument) {
				state.invalidateClosure(info, argument)
			}
		}
		return true
	case *ast.SelectorExpr:
		if candidate := writerRebindSelectorCandidate(info, node, state.candidates);
			candidate != nil {
			state.invalidateCandidate(candidate, node.Pos())
		}
		return true
	case *ast.UnaryExpr:
		if node.Op == token.AND {
			state.invalidateCandidateReferences(info, node.X)
		}
		return true
	case *ast.CompositeLit:
		state.invalidateCandidateReferences(info, node)
		state.invalidateClosure(info, node)
		return true
	default:
		return true
	}
	if len(left) != len(right) {
		for _, expression := range left {
			if object := directObject(info, expression); object != nil {
				state.assigned[object] = struct{}{}
				delete(state.candidates, object)
			}
		}
		return true
	}
	type binding struct {
		object types.Object
		candidate *ast.CallExpr
		closures map[*ast.CallExpr]struct{}
		mutationClosures map[*ast.CallExpr]struct{}
		mutationParameters map[int]struct{}
		position token.Pos
	}
	updates := make([]binding, 0, len(left))
	for index, expression := range right {
		object := directObject(info, left[index])
		if object == nil || packageScopeObject(object) {
			continue
		}
		sourceCandidate := state.candidates[directObject(info, expression)]
		if sourceCandidate != nil && !trackedInfallibleWriterType(object.Type()) {
			state.invalidateCandidate(sourceCandidate, expression.Pos())
			updates = append(
				updates,
				binding{object: object, position: expression.Pos()},
			)
			continue
		}
		if candidate, _ := ast.Unparen(expression).(*ast.CallExpr); candidate != nil {
			updates = append(
				updates,
				binding{
					object: object,
					candidate: candidate,
					closures: state.closureCandidates(info, expression),
					mutationClosures: state.closureMutationCandidates(
						info,
						expression,
					),
					position: expression.Pos(),
				},
			)
			continue
		}
		var mutationParameters map[int]struct{}
		if literal, _ := ast.Unparen(expression).(*ast.FuncLit); literal != nil {
			mutationParameters = state.mutationSummaries.literals[literal]
		} else {
			mutationParameters = state.closureMutationParameters[directObject(
				info,
				expression,
			)]
		}
		updates = append(
			updates,
			binding{
				object: object,
				candidate: state.candidates[directObject(info, expression)],
				closures: state.closureCandidates(info, expression),
				mutationClosures: state.closureMutationCandidates(info, expression),
				mutationParameters: cloneWriterMutationParameters(
					mutationParameters,
				),
				position: expression.Pos(),
			},
		)
	}
	for _, update := range updates {
		if _, reassigned := state.assigned[update.object]; reassigned {
			if update.position.IsValid() {
				state.bindingChanges[update.object] = append(
					state.bindingChanges[update.object],
					update.position,
				)
			}
			delete(state.candidates, update.object)
			delete(state.closures, update.object)
			delete(state.mutationClosures, update.object)
			delete(state.closureMutationParameters, update.object)
			continue
		}
		state.assigned[update.object] = struct{}{}
		if update.candidate == nil {
			delete(state.candidates, update.object)
		} else {
			state.recordReturnedWriterSink(info, update.candidate)
			state.candidates[update.object] = update.candidate
			state.registerCandidateDependencies(info, update.candidate)
		}
		if len(update.closures) == 0 {
			delete(state.closures, update.object)
		} else {
			state.closures[update.object] = update.closures
		}
		if len(update.mutationClosures) == 0 {
			delete(state.mutationClosures, update.object)
		} else {
			state.mutationClosures[update.object] = update.mutationClosures
		}
		if len(update.mutationParameters) == 0 {
			delete(state.closureMutationParameters, update.object)
		} else {
			state.closureMutationParameters[update.object] = update.mutationParameters
		}
	}
	return true
}

func (state *stableWriterState) recordReturnedWriterSink(
	info *types.Info,
	candidate *ast.CallExpr,
) {
	if state == nil || info == nil || candidate == nil {
		return
	}
	summary, found := state.
		mutationSummaries.
		returnedSinks[typeutil.StaticCallee(info, candidate)]
	if !found || !summary.constructor {
		return
	}
	argument := summary.parameter + writerCallParameterOffset(info, candidate)
	if argument >= 0 && argument < len(candidate.Args) {
		state.helperSinks[candidate] = candidate.Args[argument]
	}
}

func (state *stableWriterState) observeExecutedBlock(info *types.Info, body *ast.BlockStmt) {
	if state == nil || info == nil || body == nil {
		return
	}
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if _, literal := current.(*ast.FuncLit); literal {
				return false
			}
			return state.observe(info, current)
		},
	)
}

func (state *stableWriterState) recordDeferredEffect(info *types.Info, statement *ast.DeferStmt) {
	if state == nil || info == nil || statement == nil || statement.Call == nil {
		return
	}
	candidates := make(map[*ast.CallExpr]struct{})
	if candidate := writerRebindCandidate(info, statement.Call, state.candidates);
		candidate != nil {
		candidates[candidate] = struct{}{}
	}
	for candidate := range state.mutationClosures[directObject(info, statement.Call.Fun)] {
		candidates[candidate] = struct{}{}
	}
	for candidate := range state.deferredParameterMutationCandidates(info, statement.Call) {
		candidates[candidate] = struct{}{}
	}
	if literal, _ := ast.Unparen(statement.Call.Fun).(*ast.FuncLit); literal != nil {
		ast.Inspect(
			literal.Body,
			func(current ast.Node) bool {
				if current == nil {
					return false
				}
				if nested, _ := current.(*ast.FuncLit); nested != nil {
					return false
				}
				call, _ := current.(*ast.CallExpr)
				if candidate := writerRebindCandidate(info, call, state.candidates);
					candidate != nil {
					candidates[candidate] = struct{}{}
				}
				return true
			},
		)
	}
	if len(candidates) != 0 {
		state.deferredEffects = append(
			state.deferredEffects,
			deferredWriterEffect{position: statement.Pos(), candidates: candidates},
		)
	}
}

func (state *stableWriterState) deferredParameterMutationCandidates(
	info *types.Info,
	call *ast.CallExpr,
) map[*ast.CallExpr]struct{} {
	if state == nil || info == nil || call == nil {
		return nil
	}
	parameters := state.mutationSummaries.functions[typeutil.StaticCallee(info, call)]
	if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit); literal != nil {
		parameters = state.mutationSummaries.literals[literal]
	} else if len(parameters) == 0 {
		parameters = state.closureMutationParameters[directObject(info, call.Fun)]
	}
	if len(parameters) == 0 {
		return nil
	}
	offset := writerCallParameterOffset(info, call)
	candidates := make(map[*ast.CallExpr]struct{})
	for parameter := range parameters {
		argument := parameter + offset
		if argument < 0 || argument >= len(call.Args) {
			continue
		}
		if candidate := state.candidates[directObject(info, call.Args[argument])];
			candidate != nil {
			candidates[candidate] = struct{}{}
		}
	}
	return candidates
}

func writerCallParameterOffset(info *types.Info, call *ast.CallExpr) int {
	if info == nil || call == nil {
		return 0
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selection := info.Selections[selector];
		selection != nil && selection.Kind() == types.MethodExpr {
		return 1
	}
	return 0
}

func packageWriterMutationSummaries(
	info *types.Info,
	files *PackageSyntax,
	fallback ast.Node,
) writerMutationSummaries {
	if files == nil {
		return collectWriterMutationSummaries(info, fallback)
	}
	value := files.memoized(
		"unchecked-writer-error/mutation-parameters-v1",
		func() any {
			syntaxes := make([]ast.Node, 0, files.Len())
			for index := 0; index < files.Len(); index++ {
				if file := files.At(index); file != nil {
					syntaxes = append(syntaxes, file)
				}
			}
			return collectWriterMutationSummaries(info, syntaxes...)
		},
	)
	summaries, _ := value.(writerMutationSummaries)
	return summaries
}

func collectWriterMutationSummaries(
	info *types.Info,
	syntaxes ...ast.Node,
) writerMutationSummaries {
	summaries := writerMutationSummaries{
		functions: make(map[*types.Func]map[int]struct{}),
		literals: make(map[*ast.FuncLit]map[int]struct{}),
		returnedSinks: make(map[*types.Func]writerReturnedSinkSummary),
	}
	if info == nil || len(syntaxes) == 0 {
		return summaries
	}
	nodes := make([]*writerMutationSummaryNode, 0)
	functions := make(map[*types.Func]*writerMutationSummaryNode)
	literals := make(map[*ast.FuncLit]*writerMutationSummaryNode)
	for _, syntax := range syntaxes {
		if syntax == nil {
			continue
		}
		ast.Inspect(
			syntax,
			func(current ast.Node) bool {
				var node *writerMutationSummaryNode
				switch current := current.(type) {
				case *ast.FuncDecl:
					function, _ := info.Defs[current.Name].(*types.Func)
					if function == nil {
						return true
					}
					node = &writerMutationSummaryNode{
						function: function,
						type_: current.Type,
						body: current.Body,
					}
					functions[function] = node
				case *ast.FuncLit:
					node = &writerMutationSummaryNode{
						literal: current,
						type_: current.Type,
						body: current.Body,
					}
					literals[current] = node
				default:
					return true
				}
				node.reverse = make(map[int][]writerMutationTarget)
				nodes = append(nodes, node)
				return true
			},
		)
	}
	for _, node := range nodes {
		node.mutated = writerMutationParameters(
			info,
			node.type_,
			node.body,
			functions,
			literals,
		)
		if node.mutated == nil {
			node.mutated = make(map[int]struct{})
		}
	}
	for _, caller := range nodes {
		aliases := newWriterParameterAliasResolver(info, caller.type_, caller.body)
		localFunctions := writerLocalFunctionLiterals(info, caller.body)
		ast.Inspect(
			caller.body,
			func(current ast.Node) bool {
				if current == nil {
					return false
				}
				if _, nested := current.(*ast.FuncLit); nested {
					return false
				}
				call, _ := current.(*ast.CallExpr)
				if call == nil {
					return true
				}
				callee := functions[typeutil.StaticCallee(info, call)]
				if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit);
					literal != nil {
					callee = literals[literal]
				} else if literal := localFunctions[directObject(info, call.Fun)];
					literal != nil {
					callee = literals[literal]
				}
				if callee == nil {
					return true
				}
				offset := writerCallParameterOffset(info, call)
				for argument, expression := range call.Args {
					calleeParameter := argument - offset
					if calleeParameter < 0 {
						continue
					}
					for callerParameter := range
						aliases.parameters(
							directObject(info, expression),
							call.Pos(),
						) {
						callee.reverse[calleeParameter] = append(
							callee.reverse[calleeParameter],
							writerMutationTarget{
								node: caller,
								parameter: callerParameter,
							},
						)
					}
				}
				return true
			},
		)
	}
	type mutation struct {
		node *writerMutationSummaryNode
		parameter int
	}
	work := make([]mutation, 0)
	for _, node := range nodes {
		for parameter := range node.mutated {
			work = append(work, mutation{node: node, parameter: parameter})
		}
	}
	for len(work) != 0 {
		current := work[0]
		work = work[1:]
		for _, target := range current.node.reverse[current.parameter] {
			if _, found := target.node.mutated[target.parameter]; found {
				continue
			}
			target.node.mutated[target.parameter] = struct{}{}
			work = append(
				work,
				mutation{node: target.node, parameter: target.parameter},
			)
		}
	}
	for _, node := range nodes {
		if node.function != nil {
			if summary, ok := straightLineReturnedWriterSink(info, node); ok {
				summaries.returnedSinks[node.function] = summary
			}
		}
		if node.function != nil {
			summaries.functions[node.function] = node.mutated
		} else {
			summaries.literals[node.literal] = node.mutated
		}
	}
	return summaries
}

func straightLineReturnedWriterSink(
	info *types.Info,
	node *writerMutationSummaryNode,
) (writerReturnedSinkSummary, bool) {
	if info == nil ||
		node == nil ||
		node.function == nil ||
		node.type_ == nil ||
		node.type_.Params == nil ||
		node.body == nil ||
		len(node.body.List) < 3 {
		return writerReturnedSinkSummary{}, false
	}
	signature, _ := types.Unalias(node.function.Type()).(*types.Signature)
	if signature == nil ||
		signature.Results() == nil ||
		signature.Results().Len() != 1 ||
		!trackedInfallibleWriterType(signature.Results().At(0).Type()) {
		return writerReturnedSinkSummary{}, false
	}
	returnStatement, _ := node.body.List[len(node.body.List) - 1].(*ast.ReturnStmt)
	if returnStatement == nil || len(returnStatement.Results) != 1 {
		return writerReturnedSinkSummary{}, false
	}
	returned := directObject(info, returnStatement.Results[0])
	if returned == nil || packageScopeObject(returned) {
		return writerReturnedSinkSummary{}, false
	}
	rebindStatement, _ := node.body.List[len(node.body.List) - 2].(*ast.ExprStmt)
	if rebindStatement == nil {
		return writerReturnedSinkSummary{}, false
	}
	rebind, _ := ast.Unparen(rebindStatement.X).(*ast.CallExpr)
	if rebind == nil ||
		directObject(info, writerRebindReceiverExpression(info, rebind)) != returned {
		return writerReturnedSinkSummary{}, false
	}
	sink := writerSinkExpression(info, rebind)
	sinkObject := directObject(info, sink)
	if sinkObject == nil {
		return writerReturnedSinkSummary{}, false
	}
	parameter := writerFunctionParameterIndex(info, node.type_, sinkObject)
	if parameter < 0 ||
		!writerObjectInitializedLocallyBefore(info, node.body, returned, rebind.Pos()) {
		return writerReturnedSinkSummary{}, false
	}
	unsafe := false
	ast.Inspect(
		node.body,
		func(current ast.Node) bool {
			if unsafe || current == nil {
				return !unsafe
			}
			switch statement := current.(type) {
			case *ast.DeferStmt:
				if expressionUsesObject(info, statement, returned) {
					unsafe = true
				}
			case *ast.GoStmt:
				if expressionUsesObject(info, statement, returned) {
					unsafe = true
				}
			}
			return !unsafe
		},
	)
	if unsafe {
		return writerReturnedSinkSummary{}, false
	}
	return writerReturnedSinkSummary{
		parameter: parameter,
		constructor: writerObjectInitializedByOwnedAcquisition(
			info,
			node.body,
			returned,
			rebind.Pos(),
		),
	}, true
}

func writerObjectInitializedByOwnedAcquisition(
	info *types.Info,
	body *ast.BlockStmt,
	object types.Object,
	boundary token.Pos,
) bool {
	if info == nil || body == nil || object == nil || !boundary.IsValid() {
		return false
	}
	acquiredAt := -1
	var acquisition ast.Expr
	for statementIndex, statement := range body.List {
		if statement.Pos() >= boundary {
			break
		}
		assignment, _ := statement.(*ast.AssignStmt)
		if assignment == nil || len(assignment.Rhs) != len(assignment.Lhs) {
			continue
		}
		for index, target := range assignment.Lhs {
			if directObject(info, target) != object ||
				!writerOwnedAcquisition(info, assignment.Rhs[index]) {
				continue
			}
			acquiredAt = statementIndex
			acquisition = assignment.Rhs[index]
			break
		}
	}
	if acquiredAt < 0 ||
		writerObjectExposedBetween(info, body, object, token.NoPos, acquisition.Pos()) {
		return false
	}
	for _, statement := range body.List[acquiredAt + 1:] {
		if statement.Pos() >= boundary {
			break
		}
		if expressionUsesObject(info, statement, object) {
			return false
		}
	}
	return true
}

func writerOwnedAcquisition(info *types.Info, expression ast.Expr) bool {
	if info == nil || expression == nil {
		return false
	}
	if call, _ := ast.Unparen(expression).(*ast.CallExpr); call != nil {
		_, tracked := writerConstructorSinkIndex(typeutil.StaticCallee(info, call))
		return tracked
	}
	assertion, _ := ast.Unparen(expression).(*ast.TypeAssertExpr)
	if assertion == nil {
		return false
	}
	call, _ := ast.Unparen(assertion.X).(*ast.CallExpr)
	if call == nil {
		return false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "sync" ||
		function.Name() != "Get" ||
		!trackedInfallibleWriterType(info.TypeOf(assertion)) {
		return false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	return signature != nil &&
		signature.Recv() != nil &&
		namedReceiver(signature.Recv().Type(), "sync", "Pool")
}

func writerFunctionParameterIndex(
	info *types.Info,
	function *ast.FuncType,
	object types.Object,
) int {
	if info == nil || function == nil || function.Params == nil || object == nil {
		return -1
	}
	index := 0
	for _, field := range function.Params.List {
		if len(field.Names) == 0 {
			index++
			continue
		}
		for _, name := range field.Names {
			if info.ObjectOf(name) == object {
				return index
			}
			index++
		}
	}
	return -1
}

func writerObjectInitializedLocallyBefore(
	info *types.Info,
	body *ast.BlockStmt,
	object types.Object,
	boundary token.Pos,
) bool {
	if info == nil || body == nil || object == nil || !boundary.IsValid() {
		return false
	}
	initialized := false
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil || current.Pos() >= boundary {
				return false
			}
			switch current := current.(type) {
			case *ast.AssignStmt:
				for _, left := range current.Lhs {
					if directObject(info, left) == object {
						initialized = true
					}
				}
			case *ast.ValueSpec:
				for _, name := range current.Names {
					if info.ObjectOf(name) == object {
						initialized = true
					}
				}
			}
			return !initialized
		},
	)
	return initialized
}

func writerMutationParameters(
	info *types.Info,
	function *ast.FuncType,
	body *ast.BlockStmt,
	functions map[*types.Func]*writerMutationSummaryNode,
	literals map[*ast.FuncLit]*writerMutationSummaryNode,
) map[int]struct{} {
	if info == nil || function == nil || function.Params == nil || body == nil {
		return nil
	}
	parameterIndexes := newWriterParameterAliasResolver(info, function, body)
	mutated := make(map[int]struct{})
	localFunctions := writerLocalFunctionLiterals(info, body)
	markExpression := func(expression ast.Node, position token.Pos) {
		ast.Inspect(
			expression,
			func(current ast.Node) bool {
				identifier, _ := current.(*ast.Ident)
				if identifier == nil {
					return current != nil
				}
				for parameter := range
					parameterIndexes.parameters(
						info.ObjectOf(identifier),
						position,
					) {
					mutated[parameter] = struct{}{}
				}
				return true
			},
		)
	}
	markDirectExpression := func(expression ast.Expr, position token.Pos) {
		for parameter := range
			parameterIndexes.parameters(directObject(info, expression), position) {
			mutated[parameter] = struct{}{}
		}
	}
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				markExpression(literal.Body, literal.Pos())
				return false
			}
			if call, _ := current.(*ast.CallExpr); call != nil {
				receiver := writerRebindReceiverExpression(info, call)
				for parameter := range
					parameterIndexes.parameters(
						directObject(info, receiver),
						call.Pos(),
					) {
					mutated[parameter] = struct{}{}
				}
				callee := functions[typeutil.StaticCallee(info, call)]
				if literal, _ := ast.Unparen(call.Fun).(*ast.FuncLit);
					literal != nil {
					callee = literals[literal]
				} else if literal := localFunctions[directObject(info, call.Fun)];
					literal != nil {
					callee = literals[literal]
				}
				for index, argument := range call.Args {
					if standardWriterOperation(info, call, index) ||
						callee != nil ||
						(receiver != nil &&
							writerSinkExpression(info, call) ==
								argument) {
						continue
					}
					markExpression(argument, call.Pos())
				}
			}
			if assertion, _ := current.(*ast.TypeAssertExpr); assertion != nil {
				for parameter := range
					parameterIndexes.parameters(
						directObject(info, assertion.X),
						assertion.Pos(),
					) {
					mutated[parameter] = struct{}{}
				}
			}
			switch node := current.(type) {
			case *ast.AssignStmt:
				for index, value := range node.Rhs {
					if len(node.Lhs) == len(node.Rhs) {
						target := directObject(info, node.Lhs[index])
						if target != nil && !packageScopeObject(target) {
							continue
						}
					}
					markDirectExpression(value, node.Pos())
				}
			case *ast.ReturnStmt:
				for _, result := range node.Results {
					markDirectExpression(result, node.Pos())
				}
			case *ast.SendStmt:
				markDirectExpression(node.Value, node.Pos())
			case *ast.CompositeLit:
				markExpression(node, node.Pos())
			case *ast.UnaryExpr:
				if node.Op == token.AND {
					markExpression(node.X, node.Pos())
				}
			}
			return true
		},
	)
	return mutated
}

type writerParameterAliasSet map[int]struct{}

type writerParameterAliasState map[types.Object]writerParameterAliasSet

type writerParameterAliasEvent struct {
	effect token.Pos
	node ast.Node
	target types.Object
	source types.Object
}

type writerParameterAliasResolver struct {
	initial writerParameterAliasState
	events []writerParameterAliasEvent
	eventsByTarget map[types.Object][]writerParameterAliasEvent
	memo map[writerParameterAliasQuery]writerParameterAliasSet
	parents map[ast.Node]ast.Node
	body *ast.BlockStmt
	loops []writerLoopBody
}

type writerParameterAliasQuery struct {
	object types.Object
	position token.Pos
}

type writerParameterAliasReachability uint8

const (
	writerParameterAliasNever writerParameterAliasReachability = iota
	writerParameterAliasMaybe
	writerParameterAliasAlways
)

func newWriterParameterAliasResolver(
	info *types.Info,
	function *ast.FuncType,
	body *ast.BlockStmt,
) writerParameterAliasResolver {
	resolver := writerParameterAliasResolver{
		initial: make(writerParameterAliasState),
		eventsByTarget: make(map[types.Object][]writerParameterAliasEvent),
		memo: make(map[writerParameterAliasQuery]writerParameterAliasSet),
		parents: make(map[ast.Node]ast.Node),
		body: body,
		loops: writerLoopBodies(body),
	}
	if info == nil || function == nil || function.Params == nil || body == nil {
		return resolver
	}
	index := 0
	for _, field := range function.Params.List {
		for _, name := range field.Names {
			if object := info.ObjectOf(name); object != nil {
				resolver.initial[object] = writerParameterAliasSet{index: {}}
			}
			index++
		}
		if len(field.Names) == 0 {
			index++
		}
	}
	var parent ast.Node
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				parent = resolver.parents[parent]
				return false
			}
			if parent != nil {
				resolver.parents[current] = parent
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			parent = current
			return true
		},
	)
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			switch current := current.(type) {
			case *ast.AssignStmt:
				resolver.addAssignmentEvents(
					info,
					current,
					current.Lhs,
					current.Rhs,
				)
			case *ast.ValueSpec:
				left := make([]ast.Expr, len(current.Names))
				for index, name := range current.Names {
					left[index] = name
				}
				resolver.addAssignmentEvents(info, current, left, current.Values)
			case *ast.RangeStmt:
				for _, expression := range []ast.Expr{current.Key, current.Value} {
					if target := directObject(info, expression); target != nil {
						resolver.events = append(
							resolver.events,
							writerParameterAliasEvent{
								effect: current.Body.Lbrace,
								node: current,
								target: target,
							},
						)
					}
				}
			case *ast.IncDecStmt:
				if target := directObject(info, current.X); target != nil {
					resolver.events = append(
						resolver.events,
						writerParameterAliasEvent{
							effect: current.End(),
							node: current,
							target: target,
						},
					)
				}
			case *ast.UnaryExpr:
				if current.Op == token.AND {
					if target := directObject(info, current.X); target != nil {
						resolver.events = append(
							resolver.events,
							writerParameterAliasEvent{
								effect: current.End(),
								node: current,
								target: target,
							},
						)
					}
				}
			}
			return true
		},
	)
	sort.SliceStable(
		resolver.events,
		func(left int, right int) bool {
			return resolver.events[left].effect < resolver.events[right].effect
		},
	)
	for _, event := range resolver.events {
		resolver.eventsByTarget[event.target] = append(
			resolver.eventsByTarget[event.target],
			event,
		)
	}
	return resolver
}

func (resolver *writerParameterAliasResolver) addAssignmentEvents(
	info *types.Info,
	node ast.Node,
	left []ast.Expr,
	right []ast.Expr,
) {
	if resolver == nil || info == nil || node == nil {
		return
	}
	for index, expression := range left {
		target := directObject(info, expression)
		if target == nil {
			continue
		}
		var source types.Object
		if len(left) == len(right) {
			source = directObject(info, right[index])
		}
		resolver.events = append(
			resolver.events,
			writerParameterAliasEvent{
				effect: node.End(),
				node: node,
				target: target,
				source: source,
			},
		)
	}
}

func (resolver *writerParameterAliasResolver) parameters(
	object types.Object,
	position token.Pos,
) writerParameterAliasSet {
	return resolver.parametersVisiting(
		writerParameterAliasQuery{object: object, position: position},
		make(map[writerParameterAliasQuery]struct{}),
	)
}

func (resolver *writerParameterAliasResolver) parametersVisiting(
	query writerParameterAliasQuery,
	visiting map[writerParameterAliasQuery]struct{},
) writerParameterAliasSet {
	if resolver == nil || query.object == nil || !query.position.IsValid() {
		return nil
	}
	if cached, found := resolver.memo[query]; found {
		return cached
	}
	if _, cyclic := visiting[query]; cyclic {
		return nil
	}
	visiting[query] = struct{}{}
	defer delete(visiting, query)
	parameters := cloneWriterParameterAliasSet(resolver.initial[query.object])
	for _, event := range resolver.eventsByTarget[query.object] {
		if !event.effect.IsValid() {
			continue
		}
		reachability := writerParameterAliasNever
		if event.effect < query.position {
			reachability = resolver.eventReachability(event.node, query.position)
		} else if resolver.eventReachesEarlierLoopPosition(event, query.position) {
			reachability = writerParameterAliasMaybe
		}
		if reachability == writerParameterAliasNever {
			continue
		}
		source := resolver.parametersVisiting(
			writerParameterAliasQuery{object: event.source, position: event.effect},
			visiting,
		)
		if reachability == writerParameterAliasMaybe {
			parameters = mergeWriterParameterAliasSets(parameters, source)
		} else {
			parameters = cloneWriterParameterAliasSet(source)
		}
	}
	resolver.memo[query] = parameters
	return parameters
}

func (resolver writerParameterAliasResolver) eventReachesEarlierLoopPosition(
	event writerParameterAliasEvent,
	position token.Pos,
) bool {
	for _, loop := range resolver.loops {
		if position < loop.start ||
			position >= loop.end ||
			event.effect < loop.start ||
			event.effect >= loop.end {
			continue
		}
		if loopBackedgeStructurallyReachable(loop.body, event.node.Pos()) {
			return true
		}
	}
	return false
}

func (resolver writerParameterAliasResolver) eventReachability(
	node ast.Node,
	position token.Pos,
) writerParameterAliasReachability {
	reachability := writerParameterAliasAlways
	child := node
	for parent := resolver.parents[child]; parent != nil; parent = resolver.parents[parent] {
		var branch ast.Node
		var control ast.Node
		switch parent := parent.(type) {
		case *ast.IfStmt:
			if child == parent.Body || child == parent.Else {
				branch, control = child, parent
			}
		case *ast.ForStmt:
			if child == parent.Body {
				branch, control = child, parent
			}
		case *ast.RangeStmt:
			if child == parent.Body {
				branch, control = child, parent
			}
		case *ast.CaseClause:
			branch = parent
			control = clauseControl(resolver.parents, parent)
		case *ast.CommClause:
			branch = parent
			control = clauseControl(resolver.parents, parent)
		}
		if branch != nil && !(branch.Pos() <= position && position < branch.End()) {
			if control != nil && control.Pos() <= position && position < control.End() {
				return writerParameterAliasNever
			}
			reachability = writerParameterAliasMaybe
		}
		if parent == resolver.body {
			break
		}
		child = parent
	}
	return reachability
}

func clauseControl(parents map[ast.Node]ast.Node, clause ast.Node) ast.Node {
	block, _ := parents[clause].(*ast.BlockStmt)
	if block == nil {
		return nil
	}
	return parents[block]
}

func cloneWriterParameterAliasSet(source writerParameterAliasSet) writerParameterAliasSet {
	if len(source) == 0 {
		return nil
	}
	clone := make(writerParameterAliasSet, len(source))
	for parameter := range source {
		clone[parameter] = struct{}{}
	}
	return clone
}

func mergeWriterParameterAliasSets(
	left writerParameterAliasSet,
	right writerParameterAliasSet,
) writerParameterAliasSet {
	merged := cloneWriterParameterAliasSet(left)
	for parameter := range right {
		if merged == nil {
			merged = make(writerParameterAliasSet)
		}
		merged[parameter] = struct{}{}
	}
	return merged
}

func writerLocalFunctionLiterals(
	info *types.Info,
	body *ast.BlockStmt,
) map[types.Object]*ast.FuncLit {
	literals := make(map[types.Object]*ast.FuncLit)
	if info == nil || body == nil {
		return literals
	}
	assignments := make(map[types.Object]int)
	bindings := make(map[types.Object]*ast.FuncLit)
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if current == nil {
				return false
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			var left []ast.Expr
			var right []ast.Expr
			switch current := current.(type) {
			case *ast.AssignStmt:
				left, right = current.Lhs, current.Rhs
			case *ast.ValueSpec:
				left = make([]ast.Expr, len(current.Names))
				for index, name := range current.Names {
					left[index] = name
				}
				right = current.Values
			case *ast.RangeStmt:
				for _, expression := range []ast.Expr{current.Key, current.Value} {
					if object := directObject(info, expression); object != nil {
						assignments[object]++
					}
				}
				return true
			case *ast.UnaryExpr:
				if current.Op == token.AND {
					if object := directObject(info, current.X); object != nil {
						assignments[object] += 2
					}
				}
				return true
			default:
				return true
			}
			for index, expression := range left {
				object := directObject(info, expression)
				if object == nil {
					continue
				}
				assignments[object]++
				if len(left) == len(right) {
					literal, _ := ast.Unparen(right[index]).(*ast.FuncLit)
					if literal != nil {
						bindings[object] = literal
					}
				}
			}
			return true
		},
	)
	for object, literal := range bindings {
		if assignments[object] == 1 {
			literals[object] = literal
		}
	}
	return literals
}

func writerRebindReceiverExpression(info *types.Info, call *ast.CallExpr) ast.Expr {
	if info == nil || call == nil {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	selection := info.Selections[selector]
	if selection == nil {
		return nil
	}
	function, _ := selection.Obj().(*types.Func)
	if !exactWriterRebind(function) {
		return nil
	}
	switch selection.Kind() {
	case types.MethodVal:
		return selector.X
	case types.MethodExpr:
		if len(call.Args) != 0 {
			return call.Args[0]
		}
	}
	return nil
}

func cloneWriterMutationParameters(source map[int]struct{}) map[int]struct{} {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[int]struct{}, len(source))
	for index := range source {
		clone[index] = struct{}{}
	}
	return clone
}

func (state *stableWriterState) closureCandidates(
	info *types.Info,
	expression ast.Expr,
) map[*ast.CallExpr]struct{} {
	if state == nil || info == nil || expression == nil {
		return nil
	}
	if literal, _ := ast.Unparen(expression).(*ast.FuncLit); literal != nil {
		captured := make(map[*ast.CallExpr]struct{})
		ast.Inspect(
			literal.Body,
			func(current ast.Node) bool {
				if current == nil {
					return false
				}
				if nested, _ := current.(*ast.FuncLit); nested != nil {
					return false
				}
				identifier, _ := current.(*ast.Ident)
				if identifier != nil {
					if candidate := state.candidates[info.ObjectOf(identifier)];
						candidate != nil {
						captured[candidate] = struct{}{}
					}
				}
				return true
			},
		)
		return captured
	}
	source := directObject(info, expression)
	if len(state.closures[source]) == 0 {
		return nil
	}
	captured := make(map[*ast.CallExpr]struct{}, len(state.closures[source]))
	for candidate := range state.closures[source] {
		captured[candidate] = struct{}{}
	}
	return captured
}

func (state *stableWriterState) closureMutationCandidates(
	info *types.Info,
	expression ast.Expr,
) map[*ast.CallExpr]struct{} {
	if state == nil || info == nil || expression == nil {
		return nil
	}
	if literal, _ := ast.Unparen(expression).(*ast.FuncLit); literal != nil {
		mutated := make(map[*ast.CallExpr]struct{})
		ast.Inspect(
			literal.Body,
			func(current ast.Node) bool {
				if current == nil {
					return false
				}
				if nested, _ := current.(*ast.FuncLit); nested != nil {
					return false
				}
				call, _ := current.(*ast.CallExpr)
				if candidate := writerRebindCandidate(info, call, state.candidates);
					candidate != nil {
					mutated[candidate] = struct{}{}
				}
				return true
			},
		)
		return mutated
	}
	source := directObject(info, expression)
	if len(state.mutationClosures[source]) == 0 {
		return nil
	}
	mutated := make(map[*ast.CallExpr]struct{}, len(state.mutationClosures[source]))
	for candidate := range state.mutationClosures[source] {
		mutated[candidate] = struct{}{}
	}
	return mutated
}

func (state *stableWriterState) invalidateClosure(info *types.Info, expression ast.Expr) {
	if state == nil || info == nil || expression == nil {
		return
	}
	ast.Inspect(
		expression,
		func(current ast.Node) bool {
			candidateExpression, _ := current.(ast.Expr)
			if candidateExpression == nil {
				return true
			}
			for candidate := range state.closureCandidates(info, candidateExpression) {
				state.invalidateCandidate(candidate, expression.Pos())
			}
			_, literal := ast.Unparen(candidateExpression).(*ast.FuncLit)
			return !literal
		},
	)
}

func packageScopeObject(object types.Object) bool {
	return object != nil && object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}

func (state *stableWriterState) registerCandidateDependencies(
	info *types.Info,
	candidate *ast.CallExpr,
) {
	state.registerCandidateDependenciesSeen(info, candidate, make(map[*ast.CallExpr]struct{}))
}

func (state *stableWriterState) registerCandidateDependenciesSeen(
	info *types.Info,
	candidate *ast.CallExpr,
	seen map[*ast.CallExpr]struct{},
) {
	if state == nil || info == nil || candidate == nil {
		return
	}
	if _, found := seen[candidate]; found {
		return
	}
	seen[candidate] = struct{}{}
	sink := writerSinkExpressionWithHelpers(info, candidate, state.helperSinks)
	if sink == nil {
		return
	}
	if inline, _ := ast.Unparen(sink).(*ast.CallExpr); inline != nil {
		if writerSinkExpressionWithHelpers(info, inline, state.helperSinks) == nil {
			return
		}
		state.registerCandidateDependenciesSeen(info, inline, seen)
		state.addCandidateDependent(inline, candidate)
		return
	}
	dependency := state.candidates[directObject(info, sink)]
	if dependency != nil && dependency != candidate {
		state.addCandidateDependent(dependency, candidate)
	}
}

func (state *stableWriterState) addCandidateDependent(
	dependency *ast.CallExpr,
	dependent *ast.CallExpr,
) {
	if state == nil || dependency == nil || dependent == nil || dependency == dependent {
		return
	}
	if state.dependents[dependency] == nil {
		state.dependents[dependency] = make(map[*ast.CallExpr]struct{})
	}
	state.dependents[dependency][dependent] = struct{}{}
	if _, invalid := state.invalidated[dependency]; invalid {
		state.invalidateCandidate(dependent, dependency.Pos())
	}
}

func (state *stableWriterState) invalidateCandidate(candidate *ast.CallExpr, position token.Pos) {
	if state == nil || candidate == nil {
		return
	}
	pending := []*ast.CallExpr{candidate}
	visited := make(map[*ast.CallExpr]struct{})
	for len(pending) != 0 {
		current := pending[len(pending) - 1]
		pending = pending[:len(pending) - 1]
		if _, found := visited[current]; found {
			continue
		}
		visited[current] = struct{}{}
		state.invalidated[current] = struct{}{}
		if position.IsValid() {
			state.invalidationPositions[current] = append(
				state.invalidationPositions[current],
				position,
			)
		}
		for dependent := range state.dependents[current] {
			pending = append(pending, dependent)
		}
	}
}

func (state *stableWriterState) invalidateCandidateReferences(info *types.Info, node ast.Node) {
	if state == nil || info == nil || node == nil {
		return
	}
	ast.Inspect(
		node,
		func(current ast.Node) bool {
			identifier, _ := current.(*ast.Ident)
			if identifier == nil {
				return true
			}
			if candidate := state.candidates[info.ObjectOf(identifier)];
				candidate != nil {
				state.invalidateCandidate(candidate, identifier.Pos())
			}
			return true
		},
	)
}

func standardWriterOperation(info *types.Info, call *ast.CallExpr, argument int) bool {
	if info == nil || call == nil {
		return false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil {
		return false
	}
	if sink, matched := writerConstructorSinkIndex(function); matched && argument == sink {
		return true
	}
	if argument == writerCallParameterOffset(info, call) {
		switch function.Pkg().Path() {
		case "html/template", "text/template":
			if function.Name() == "Execute" || function.Name() == "ExecuteTemplate" {
				return true
			}
		case "io":
			if (function.Name() == "Copy" || function.Name() == "CopyBuffer") &&
				len(call.Args) > argument + 1 {
				return writerCopySourceCannotObserveDestination(
					info.TypeOf(call.Args[argument + 1]),
				)
			}
		}
	}
	if argument != 0 {
		return false
	}
	switch function.Pkg().Path() {
	case "fmt":
		switch function.Name() {
		case "Fprint", "Fprintf", "Fprintln":
			for _, value := range call.Args[1:] {
				if !writerFormattingValueCannotCallBack(info.TypeOf(value)) {
					return false
				}
			}
			return true
		}
	case "io":
		return function.Name() == "WriteString"
	}
	return false
}

func writerFormattingValueCannotCallBack(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	if namedReceiver(type_, "time", "Time") {
		return true
	}
	if namedReceiver(type_, "io/fs", "FileMode") {
		return true
	}
	if _, basic := types.Unalias(type_).Underlying().(*types.Basic); !basic {
		return false
	}
	return types.NewMethodSet(type_).Len() == 0
}

func writerCopySourceCannotObserveDestination(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	if _, dynamic := types.Unalias(type_).Underlying().(*types.Interface); dynamic {
		return false
	}
	if types.NewMethodSet(type_).Lookup(nil, "WriteTo") == nil {
		return true
	}
	return namedReceiver(type_, "bytes", "Buffer") ||
		namedReceiver(type_, "bytes", "Reader") ||
		namedReceiver(type_, "strings", "Reader")
}

func writerArgumentCannotRebind(info *types.Info, call *ast.CallExpr, argument int) bool {
	if info == nil || call == nil || argument < 0 || argument >= len(call.Args) {
		return false
	}
	signature, _ := types.Unalias(info.TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || signature.Params() == nil {
		return false
	}
	parameter := argument - writerCallParameterOffset(info, call)
	if parameter < 0 {
		return false
	}
	parameterCount := signature.Params().Len()
	if parameterCount == 0 {
		return false
	}
	if parameter >= parameterCount {
		if !signature.Variadic() {
			return false
		}
		parameter = parameterCount - 1
	}
	parameterType := signature.Params().At(parameter).Type()
	if signature.Variadic() && parameter == parameterCount - 1 {
		slice, _ := types.Unalias(parameterType).(*types.Slice)
		if slice != nil {
			parameterType = slice.Elem()
		}
	}
	return namedReceiver(parameterType, "io", "Writer")
}

func (state *stableWriterState) writerArgumentCannotRebind(
	info *types.Info,
	call *ast.CallExpr,
	argument int,
) bool {
	if state == nil || !writerArgumentCannotRebind(info, call, argument) {
		return false
	}
	callee := typeutil.StaticCallee(info, call)
	if callee == nil {
		return false
	}
	mutated, local := state.mutationSummaries.functions[callee]
	if !local {
		return false
	}
	parameter := argument - writerCallParameterOffset(info, call)
	if parameter < 0 {
		return false
	}
	_, rebinds := mutated[parameter]
	return !rebinds
}

func (state *stableWriterState) harmlessWriterClosureArgument(
	info *types.Info,
	expression ast.Expr,
) bool {
	if state == nil || info == nil || expression == nil {
		return false
	}
	literal, _ := ast.Unparen(expression).(*ast.FuncLit)
	if literal == nil || literal.Body == nil {
		return false
	}
	captured := state.closureCandidates(info, literal)
	if len(captured) == 0 {
		return true
	}
	for candidate := range state.closureMutationCandidates(info, literal) {
		if _, relevant := captured[candidate]; relevant {
			return false
		}
	}
	parents := writerASTParents(literal.Body)
	for object, candidate := range state.candidates {
		if _, relevant := captured[candidate]; !relevant {
			continue
		}
		harmless := true
		ast.Inspect(
			literal.Body,
			func(node ast.Node) bool {
				if !harmless || node == nil {
					return harmless
				}
				identifier, _ := node.(*ast.Ident)
				if identifier == nil || info.ObjectOf(identifier) != object {
					return true
				}
				if !state.harmlessWriterClosureUse(info, identifier, parents) {
					harmless = false
				}
				return harmless
			},
		)
		if !harmless {
			return false
		}
	}
	return true
}

func writerASTParents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	stack := make([]ast.Node, 0)
	ast.Inspect(
		root,
		func(node ast.Node) bool {
			if node == nil {
				if len(stack) != 0 {
					stack = stack[:len(stack) - 1]
				}
				return false
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

func (state *stableWriterState) harmlessWriterClosureUse(
	info *types.Info,
	identifier *ast.Ident,
	parents map[ast.Node]ast.Node,
) bool {
	if info == nil || identifier == nil {
		return false
	}
	parent := parents[identifier]
	if call, _ := parent.(*ast.CallExpr); call != nil {
		for index, argument := range call.Args {
			if argument == identifier {
				return standardWriterOperation(info, call, index) ||
					state.writerArgumentCannotRebind(info, call, index)
			}
		}
		return false
	}
	selector, _ := parent.(*ast.SelectorExpr)
	if selector == nil || selector.X != identifier {
		return false
	}
	call, _ := parents[selector].(*ast.CallExpr)
	if call == nil || ast.Unparen(call.Fun) != selector {
		return false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	return function != nil && !exactWriterRebind(function)
}

func writerRebindCandidate(
	info *types.Info,
	call *ast.CallExpr,
	candidates map[types.Object]*ast.CallExpr,
) *ast.CallExpr {
	if info == nil || call == nil {
		return nil
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if candidate := writerRebindSelectorCandidate(info, selector, candidates);
		candidate != nil {
		return candidate
	}
	function := typeutil.StaticCallee(info, call)
	if !exactWriterRebind(function) || len(call.Args) == 0 {
		return nil
	}
	return candidates[directObject(info, call.Args[0])]
}

func writerRebindSelectorCandidate(
	info *types.Info,
	selector *ast.SelectorExpr,
	candidates map[types.Object]*ast.CallExpr,
) *ast.CallExpr {
	if info == nil || selector == nil {
		return nil
	}
	selection := info.Selections[selector]
	if selection == nil {
		return nil
	}
	function, _ := selection.Obj().(*types.Func)
	if !exactWriterRebind(function) {
		return nil
	}
	return candidates[directObject(info, selector.X)]
}

func exactWriterRebind(function *types.Func) bool {
	if function == nil || function.Pkg() == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	switch function.Pkg().Path() {
	case "bufio":
		return function.Name() == "Reset" &&
			namedReceiver(signature.Recv().Type(), "bufio", "Writer")
	case "compress/gzip":
		return function.Name() == "Reset" &&
			namedReceiver(signature.Recv().Type(), "compress/gzip", "Writer")
	case "text/tabwriter":
		return function.Name() == "Init" &&
			namedReceiver(signature.Recv().Type(), "text/tabwriter", "Writer")
	default:
		return false
	}
}

func discardedCall(node ast.Node) (*ast.CallExpr, bool) {
	switch statement := node.(type) {
	case *ast.ExprStmt:
		call, _ := ast.Unparen(statement.X).(*ast.CallExpr)
		return call, call != nil
	case *ast.AssignStmt:
		if len(statement.Lhs) != 1 ||
			len(statement.Rhs) != 1 ||
			!blankIdentifier(statement.Lhs[0]) {
			return nil, false
		}
		call, _ := ast.Unparen(statement.Rhs[0]).(*ast.CallExpr)
		return call, call != nil
	case *ast.GoStmt:
		return statement.Call, statement.Call != nil
	case *ast.DeferStmt:
		return statement.Call, statement.Call != nil
	default:
		return nil, false
	}
}

func (s writerFinalizerSpec) target() string {
	if s.constructor != "" {
		return fmt.Sprintf("%s.%s result %s", s.packagePath, s.constructor, s.methodName)
	}
	return fmt.Sprintf("%s.%s.%s", s.packagePath, s.typeName, s.methodName)
}

func writerFinalizer(ctx *TypesContext, call *ast.CallExpr) (writerFinalizerSpec, bool) {
	if ctx == nil || ctx.Info() == nil || call == nil {
		return writerFinalizerSpec{}, false
	}
	info := ctx.Info()
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return writerFinalizerSpec{}, false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return writerFinalizerSpec{}, false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil || function.Pkg() == nil {
		return writerFinalizerSpec{}, false
	}
	signature, _ := types.Unalias(function.Type()).(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return writerFinalizerSpec{}, false
	}
	for _, spec := range writerFinalizerSpecs {
		if function.Pkg().Path() == spec.packagePath &&
			function.Name() == spec.methodName &&
			namedReceiver(signature.Recv().Type(), spec.packagePath, spec.typeName) {
			return spec, true
		}
	}
	if spec, matched := acquiredEncoderFinalizer(info, ctx.Syntax(), call, selector); matched {
		return spec, true
	}
	return writerFinalizerSpec{}, false
}

func acquiredEncoderFinalizer(
	info *types.Info,
	syntax *ast.File,
	call *ast.CallExpr,
	selector *ast.SelectorExpr,
) (writerFinalizerSpec, bool) {
	if info == nil || syntax == nil || call == nil || selector == nil || len(call.Args) != 0 {
		return writerFinalizerSpec{}, false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return writerFinalizerSpec{}, false
	}
	method, _ := selection.Obj().(*types.Func)
	if method == nil || method.Name() != "Close" {
		return writerFinalizerSpec{}, false
	}
	if constructor, _ := ast.Unparen(selector.X).(*ast.CallExpr); constructor != nil {
		return writerEncoderConstructor(info, constructor)
	}
	receiver := directObject(info, selector.X)
	if receiver == nil {
		return writerFinalizerSpec{}, false
	}
	return stableWriterEncoderAcquisition(info, syntax, receiver, call)
}

func stableWriterEncoderAcquisition(
	info *types.Info,
	syntax *ast.File,
	receiver types.Object,
	finalizer ast.Node,
) (writerFinalizerSpec, bool) {
	var spec writerFinalizerSpec
	acquired := false
	stable := true
	ast.Inspect(
		syntax,
		func(node ast.Node) bool {
			if node == nil || !stable || node.Pos() >= finalizer.Pos() {
				return stable
			}
			switch node := node.(type) {
			case *ast.AssignStmt:
				if len(node.Lhs) != len(node.Rhs) {
					return true
				}
				for index, left := range node.Lhs {
					if directObject(info, left) != receiver {
						continue
					}
					identifier, _ := left.(*ast.Ident)
					constructor, _ := ast.Unparen(
						node.Rhs[index],
					).(*ast.CallExpr)
					candidate, matched := writerEncoderConstructor(
						info,
						constructor,
					)
					if !acquired &&
						identifier != nil &&
						info.Defs[identifier] == receiver &&
						matched {
						spec = candidate
						acquired = true
						continue
					}
					stable = false
					return false
				}
			case *ast.ValueSpec:
				if len(node.Names) != len(node.Values) {
					return true
				}
				for index, name := range node.Names {
					if info.Defs[name] != receiver {
						continue
					}
					constructor, _ := ast.Unparen(
						node.Values[index],
					).(*ast.CallExpr)
					candidate, matched := writerEncoderConstructor(
						info,
						constructor,
					)
					if acquired || !matched {
						stable = false
						return false
					}
					spec = candidate
					acquired = true
				}
			}
			return true
		},
	)
	return spec, acquired && stable
}

func writerEncoderConstructor(info *types.Info, call *ast.CallExpr) (writerFinalizerSpec, bool) {
	if info == nil || call == nil {
		return writerFinalizerSpec{}, false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil || function.Name() != "NewEncoder" {
		return writerFinalizerSpec{}, false
	}
	for _, spec := range writerEncoderSpecs {
		if function.Pkg().Path() == spec.packagePath {
			return spec, true
		}
	}
	return writerFinalizerSpec{}, false
}

func isWriterFinalizer(ctx *TypesContext, call *ast.CallExpr) bool {
	_, matched := writerFinalizer(ctx, call)
	return matched
}
