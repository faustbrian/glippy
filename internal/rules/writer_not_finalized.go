package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/types/typeutil"
)

type writerNotFinalizedRule struct{}

type writerLifecycleSpec struct {
	packagePath string
	constructors map[string]int
	sinkArgument int
	useMethods map[string]struct{}
	finalizer string
}

type writerLifecycleCandidate struct {
	identifier *ast.Ident
	object types.Object
	sink types.Object
	sinkConstructor token.Pos
	sinkStart token.Pos
	spec writerLifecycleSpec
	start obligationStart
}

type writerLifecycleState uint8

const (
	writerLifecycleUnused writerLifecycleState = iota
	writerLifecycleUsed
)

type writerLifecycleWork struct {
	block *cfg.Block
	offset int
	state writerLifecycleState
	errorState writerNamedErrorState
}

type writerNodeTransition struct {
	used bool
	terminal obligationEffect
}

type writerNamedErrorAnalysis struct {
	result types.Object
	snapshot stateTransitionSnapshot[[]writerNamedErrorState]
	complete bool
}

type writerNamedErrorState struct {
	nilness NilState
	escaped bool
}

func (s writerNamedErrorState) provenNil() bool {
	return !s.escaped && s.nilness == NilStateNil
}

var requiredWriterLifecycleSpecs = []writerLifecycleSpec{
	{
		packagePath: "archive/tar",
		constructors: map[string]int{"NewWriter": 0},
		sinkArgument: 0,
		useMethods: map[string]struct{}{"AddFS": {}, "Write": {}, "WriteHeader": {}},
		finalizer: "Close",
	},
	{
		packagePath: "compress/gzip",
		constructors: map[string]int{"NewWriter": 0, "NewWriterLevel": 0},
		sinkArgument: 0,
		useMethods: map[string]struct{}{"Flush": {}, "Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "encoding/ascii85",
		constructors: map[string]int{"NewEncoder": 0},
		sinkArgument: 0,
		useMethods: map[string]struct{}{"Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "encoding/base32",
		constructors: map[string]int{"NewEncoder": 0},
		sinkArgument: 1,
		useMethods: map[string]struct{}{"Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "encoding/base64",
		constructors: map[string]int{"NewEncoder": 0},
		sinkArgument: 1,
		useMethods: map[string]struct{}{"Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "mime/multipart",
		constructors: map[string]int{"NewWriter": 0},
		sinkArgument: 0,
		useMethods: map[string]struct{}{
			"CreateFormField": {},
			"CreateFormFile": {},
			"CreatePart": {},
			"WriteField": {},
		},
		finalizer: "Close",
	},
}

// NewWriterNotFinalizedRule constructs the required writer-finalization rule.
func NewWriterNotFinalizedRule() Rule {
	return writerNotFinalizedRule{}
}

func (writerNotFinalizedRule) Metadata() Metadata {
	return Metadata{
		ID: "writer-not-finalized",
		Summary: "detects successful output paths that omit required writer finalization",
		Documentation: "Archive, compression, encoded, and multipart writers can buffer bytes or require trailing framing that is emitted only by Close. This rule follows exact standard-library writer acquisitions and reports a used writer when a successfully returning path neither finalizes nor transfers it.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireControlFlow,
		RequiresEffectFacts: true,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The exact contract covers direct local assignment results and initialized variable declarations from archive/tar.NewWriter, compress/gzip.NewWriter or NewWriterLevel, encoding/ascii85.NewEncoder, encoding/base32.NewEncoder, encoding/base64.NewEncoder, and mime/multipart.NewWriter.",
			"Only exact output-producing receiver methods establish use; construction and configuration alone do not require finalization.",
			"A function with one named error result may classify a bare return or that exact result as successful only while CFG dataflow proves nil from zero initialization, direct nil or self assignment, or an exact == nil or != nil edge. An exact statically resolved delegated call is successful only when every explicit normal return of the selected local helper or bounded delegation chain proves that error result nil. Compound conditions, address escape, closure capture, multiple errors, dynamic calls, recursive delegation, typed nil errors, and unknown joins remain conservative.",
			"Aliases, fields, containers, closures, method values, asynchronous calls, and transfers stop analysis because exact ownership or execution order is unavailable.",
			"A proven non-nil error passed to an exact io.PipeWriter.CloseWithError aborts the output path without requiring writer finalization. Proof is limited to exact non-nil constructors or a direct enclosing nil or io.EOF guard without intervening reassignment.",
			"One constructor call must supply the declaration or assignment values; parallel multi-expression acquisitions remain outside the direct mapping contract.",
			"No fix is offered because correct finalization and error joining depend on the surrounding return contract.",
		},
		Examples: []Example{
			{
				Title: "Finalize compressed output before returning success",
				Incorrect: "writer := gzip.NewWriter(output)\n_, _ = writer.Write(data)\nreturn nil",
				Correct: "writer := gzip.NewWriter(output)\nif _, err := writer.Write(data); err != nil { return err }\nreturn writer.Close()",
			},
		},
	}
}

func (writerNotFinalizedRule) RunControlFlow(ctx *ControlFlowContext) ([]Finding, error) {
	if ctx == nil || ctx.Body() == nil || ctx.Graph() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"writer-not-finalized requires a complete control-flow context",
		)
	}
	candidates := writerLifecycleCandidates(ctx)
	if len(candidates) == 0 {
		return []Finding{}, nil
	}
	findings := make([]Finding, 0)
	errorAnalysis := newWriterNamedErrorAnalysis(ctx)
	for _, candidate := range candidates {
		if !writerLifecycleNeedsFinalization(ctx, candidate, errorAnalysis) {
			continue
		}
		range_, err := ctx.Range(candidate.identifier)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "writer-not-finalized",
				Message: "used writer is not finalized on every successful return path",
				Range: range_,
				Help: "observe and propagate the finalizer error before reporting successful output",
			},
		)
	}
	return findings, nil
}

func writerLifecycleCandidates(ctx *ControlFlowContext) []writerLifecycleCandidate {
	result := make([]writerLifecycleCandidate, 0)
	ast.Inspect(
		ctx.Body(),
		func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			candidate, found := writerLifecycleCandidateAt(ctx, node)
			if found {
				result = append(result, candidate)
			}
			return true
		},
	)
	return result
}

func writerLifecycleCandidateAt(
	ctx *ControlFlowContext,
	node ast.Node,
) (writerLifecycleCandidate, bool) {
	if ctx == nil || ctx.Info() == nil || ctx.Graph() == nil || node == nil {
		return writerLifecycleCandidate{}, false
	}
	var (
		identifier *ast.Ident
		call *ast.CallExpr
		spec writerLifecycleSpec
	)
	switch node := node.(type) {
	case *ast.AssignStmt:
		if len(node.Rhs) != 1 {
			return writerLifecycleCandidate{}, false
		}
		call, _ = ast.Unparen(node.Rhs[0]).(*ast.CallExpr)
		candidate, resultIndex, matched := writerLifecycleConstructor(ctx.Info(), call)
		if !matched || resultIndex >= len(node.Lhs) {
			return writerLifecycleCandidate{}, false
		}
		spec = candidate
		identifier, _ = node.Lhs[resultIndex].(*ast.Ident)
	case *ast.ValueSpec:
		if len(node.Values) != 1 {
			return writerLifecycleCandidate{}, false
		}
		call, _ = ast.Unparen(node.Values[0]).(*ast.CallExpr)
		candidate, resultIndex, matched := writerLifecycleConstructor(ctx.Info(), call)
		if !matched || resultIndex >= len(node.Names) {
			return writerLifecycleCandidate{}, false
		}
		spec = candidate
		identifier = node.Names[resultIndex]
	default:
		return writerLifecycleCandidate{}, false
	}
	if identifier == nil || identifier.Name == "_" {
		return writerLifecycleCandidate{}, false
	}
	object := ctx.Info().ObjectOf(identifier)
	if spec.sinkArgument < 0 || spec.sinkArgument >= len(call.Args) {
		return writerLifecycleCandidate{}, false
	}
	sink := directObject(ctx.Info(), call.Args[spec.sinkArgument])
	start, found := obligationStartAfter(ctx.Graph(), node)
	if object == nil || !found {
		return writerLifecycleCandidate{}, false
	}
	return writerLifecycleCandidate{
		identifier: identifier,
		object: object,
		sink: sink,
		sinkConstructor: call.Pos(),
		sinkStart: call.End(),
		spec: spec,
		start: start,
	}, true
}

func writerLifecycleConstructor(
	info *types.Info,
	call *ast.CallExpr,
) (writerLifecycleSpec, int, bool) {
	if info == nil || call == nil {
		return writerLifecycleSpec{}, 0, false
	}
	function := typeutil.StaticCallee(info, call)
	if function == nil || function.Pkg() == nil {
		return writerLifecycleSpec{}, 0, false
	}
	for _, spec := range requiredWriterLifecycleSpecs {
		result, found := spec.constructors[function.Name()]
		if found && function.Pkg().Path() == spec.packagePath {
			return spec, result, true
		}
	}
	return writerLifecycleSpec{}, 0, false
}

func writerLifecycleNeedsFinalization(
	ctx *ControlFlowContext,
	candidate writerLifecycleCandidate,
	errorAnalysis writerNamedErrorAnalysis,
) bool {
	start := candidate.start
	if start.block == nil {
		return false
	}
	errorResult := errorAnalysis.result
	errorState := errorAnalysis.stateAt(ctx, start)
	work := []writerLifecycleWork{
		{block: start.block, offset: start.offset, errorState: errorState},
	}
	seen := make(map[writerLifecycleWork]struct{})
	var parents map[ast.Node]ast.Node
	if candidate.sink != nil && namedReceiver(candidate.sink.Type(), "io", "PipeWriter") {
		parents = writerPackageASTParents(ctx.PackageSyntax(), ctx.Body())
	}
	for len(work) > 0 {
		current := work[len(work) - 1]
		work = work[:len(work) - 1]
		if current.block == nil || !current.block.Live {
			continue
		}
		if _, found := seen[current]; found {
			continue
		}
		seen[current] = struct{}{}
		state := current.state
		errorState := current.errorState
		terminal := obligationOpen
		nodes := current.block.Nodes
		for _, node := range nodes[boundedNodeOffset(current.offset, nodes):] {
			errorState = writerNamedErrorStateAfter(
				ctx.Info(),
				node,
				errorResult,
				errorState,
			)
			transition := writerLifecycleNodeTransition(
				ctx.Info(),
				node,
				candidate,
				parents,
			)
			if transition.used {
				state = writerLifecycleUsed
			}
			if transition.terminal != obligationOpen {
				terminal = transition.terminal
				break
			}
		}
		switch terminal {
		case obligationCompleted, obligationTransferred:
			continue
		case obligationLost:
			if state == writerLifecycleUsed {
				return true
			}
			continue
		}
		if returned := current.block.Return();
			returned != nil &&
				state == writerLifecycleUsed &&
				writerReturnIsSuccessful(ctx, returned, errorResult, errorState) {
			return true
		}
		for _, successor := range current.block.Succs {
			successorErrorState := writerNamedErrorStateOnEdge(
				ctx.Info(),
				current.block,
				successor,
				errorResult,
				errorState,
			)
			work = append(
				work,
				writerLifecycleWork{
					block: successor,
					state: state,
					errorState: successorErrorState,
				},
			)
		}
	}
	return false
}

func boundedNodeOffset(offset int, nodes []ast.Node) int {
	if offset < 0 {
		return 0
	}
	if offset > len(nodes) {
		return len(nodes)
	}
	return offset
}

func writerLifecycleNodeTransition(
	info *types.Info,
	node ast.Node,
	candidate writerLifecycleCandidate,
	parents map[ast.Node]ast.Node,
) writerNodeTransition {
	transition := writerNodeTransition{}
	if statement, asynchronous := node.(*ast.GoStmt);
		asynchronous && expressionUsesObject(info, statement.Call, candidate.object) {
		transition.terminal = obligationTransferred
		return transition
	}
	assignment, _ := node.(*ast.AssignStmt)
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if transition.terminal != obligationOpen {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				if expressionUsesObject(info, literal.Body, candidate.object) {
					transition.terminal = obligationTransferred
				}
				return false
			}
			switch current := current.(type) {
			case *ast.ReturnStmt:
				for _, expression := range current.Results {
					if directObject(info, expression) == candidate.object ||
						methodValueUsesObject(
							info,
							expression,
							candidate.object,
						) {
						transition.terminal = obligationTransferred
						return false
					}
				}
			case *ast.SendStmt:
				if directObject(info, current.Value) == candidate.object {
					transition.terminal = obligationTransferred
					return false
				}
			case *ast.CompositeLit:
				for _, element := range current.Elts {
					if expressionUsesObject(info, element, candidate.object) {
						transition.terminal = obligationTransferred
						return false
					}
				}
			}
			call, _ := current.(*ast.CallExpr)
			if call == nil {
				return true
			}
			method, receiverArgument, matched := writerLifecycleMethod(
				info,
				call,
				candidate.object,
			)
			if matched {
				if method == candidate.spec.finalizer {
					transition.terminal = obligationCompleted
					return false
				}
				if _, used := candidate.spec.useMethods[method]; used {
					transition.used = true
				}
			}
			if writerLifecycleAbortCall(info, call, candidate, parents) {
				transition.terminal = obligationCompleted
				return false
			}
			for index, argument := range call.Args {
				if index == receiverArgument {
					continue
				}
				if directObject(info, argument) == candidate.object ||
					methodValueUsesObject(info, argument, candidate.object) {
					transition.terminal = obligationTransferred
					return false
				}
			}
			return true
		},
	)
	if transition.terminal != obligationOpen || assignment == nil {
		return transition
	}
	for index, expression := range assignment.Rhs {
		if directObject(info, expression) != candidate.object &&
			!methodValueUsesObject(info, expression, candidate.object) {
			continue
		}
		if len(assignment.Rhs) != len(assignment.Lhs) {
			transition.terminal = obligationTransferred
			return transition
		}
		target := assignment.Lhs[index]
		identifier, blank := target.(*ast.Ident)
		if blank && identifier.Name == "_" {
			continue
		}
		if directObject(info, target) != candidate.object {
			transition.terminal = obligationTransferred
			return transition
		}
	}
	if assignmentReplacesObject(info, assignment, candidate.object, nil) {
		transition.terminal = obligationLost
	}
	return transition
}

func writerLifecycleAbortCall(
	info *types.Info,
	call *ast.CallExpr,
	candidate writerLifecycleCandidate,
	parents map[ast.Node]ast.Node,
) bool {
	if info == nil ||
		call == nil ||
		candidate.sink == nil ||
		!namedReceiver(candidate.sink.Type(), "io", "PipeWriter") {
		return false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	selection := info.Selections[selector]
	if selector == nil || selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "io" ||
		function.Name() != "CloseWithError" ||
		!namedReceiver(selection.Recv(), "io", "PipeWriter") {
		return false
	}
	receiver, _ := staticReceiverArgument(info, call)
	argument := writerCallParameterOffset(info, call)
	if receiver != candidate.sink || argument < 0 || argument >= len(call.Args) {
		return false
	}
	return !writerObjectReassignedBetween(
		info,
		writerEnclosingFunctionBody(call, parents),
		candidate.sink,
		candidate.sinkStart,
		call.Pos(),
	) &&
		!writerObjectExposedBetween(
			info,
			writerEnclosingFunctionBody(call, parents),
			candidate.sink,
			token.NoPos,
			candidate.sinkConstructor,
		) &&
		!writerObjectExposedBeforeEnclosingClosures(info, call, candidate.sink, parents) &&
		writerAbortErrorProvenNonNil(info, call.Args[argument], call, parents)
}

func writerObjectExposedBeforeEnclosingClosures(
	info *types.Info,
	node ast.Node,
	object types.Object,
	parents map[ast.Node]ast.Node,
) bool {
	if info == nil || node == nil || object == nil {
		return true
	}
	for ancestor := parents[node]; ancestor != nil; ancestor = parents[ancestor] {
		literal, _ := ancestor.(*ast.FuncLit)
		if literal == nil {
			continue
		}
		outerBody := writerEnclosingFunctionBody(parents[literal], parents)
		if outerBody != nil &&
			writerObjectExposedBetween(
				info,
				outerBody,
				object,
				token.NoPos,
				literal.Pos(),
			) {
			return true
		}
	}
	return false
}

func writerPackageASTParents(files *PackageSyntax, fallback ast.Node) map[ast.Node]ast.Node {
	if files != nil && fallback != nil {
		fileIndex := -1
		for index := 0; index < files.Len(); index++ {
			file := files.At(index)
			if file != nil &&
				fallback.Pos() >= file.Pos() &&
				fallback.End() <= file.End() {
				fileIndex = index
				break
			}
		}
		if fileIndex < 0 {
			return writerASTParents(fallback)
		}
		value := files.memoized(
			fmt.Sprintf("writer-not-finalized/ast-parents-v1/%d", fileIndex),
			func() any {
				return writerASTParents(files.At(fileIndex))
			},
		)
		if parents, _ := value.(map[ast.Node]ast.Node); len(parents) != 0 {
			return parents
		}
	}
	return writerASTParents(fallback)
}

func writerObjectExposedBetween(
	info *types.Info,
	root ast.Node,
	object types.Object,
	start token.Pos,
	end token.Pos,
) bool {
	if info == nil || root == nil || object == nil || !end.IsValid() {
		return true
	}
	exposed := false
	ast.Inspect(
		root,
		func(node ast.Node) bool {
			if exposed || node == nil {
				return !exposed
			}
			if node.Pos() <= start || node.Pos() >= end {
				return node.Pos() < end
			}
			switch node := node.(type) {
			case *ast.UnaryExpr:
				if node.Op == token.AND &&
					expressionUsesObject(info, node.X, object) {
					exposed = true
				}
			case *ast.AssignStmt:
				for index, value := range node.Rhs {
					if value.Pos() >= end ||
						!expressionUsesObject(info, value, object) {
						continue
					}
					if len(node.Lhs) != len(node.Rhs) ||
						directObject(info, node.Lhs[index]) != object {
						exposed = true
						break
					}
				}
			case *ast.FuncLit:
				if expressionUsesObject(info, node.Body, object) {
					exposed = true
					return false
				}
			case *ast.CallExpr:
				for _, argument := range node.Args {
					if expressionUsesObject(info, argument, object) {
						exposed = true
						break
					}
				}
			case *ast.CompositeLit:
				if expressionUsesObject(info, node, object) {
					exposed = true
				}
			case *ast.ReturnStmt:
				for _, result := range node.Results {
					if expressionUsesObject(info, result, object) {
						exposed = true
						break
					}
				}
			case *ast.SendStmt:
				if expressionUsesObject(info, node.Value, object) {
					exposed = true
				}
			}
			return !exposed
		},
	)
	return exposed
}

func writerEnclosingFunctionBody(node ast.Node, parents map[ast.Node]ast.Node) *ast.BlockStmt {
	for ancestor := node; ancestor != nil; ancestor = parents[ancestor] {
		switch ancestor := ancestor.(type) {
		case *ast.FuncLit:
			return ancestor.Body
		case *ast.FuncDecl:
			return ancestor.Body
		case *ast.BlockStmt:
			if parents[ancestor] == nil {
				return ancestor
			}
		}
	}
	return nil
}

func writerAbortErrorProvenNonNil(
	info *types.Info,
	expression ast.Expr,
	use ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	var function *types.Func
	if call != nil {
		function = typeutil.StaticCallee(info, call)
	}
	if function != nil && function.Pkg() != nil {
		switch function.Pkg().Path() {
		case "fmt":
			if function.Name() == "Errorf" {
				return true
			}
		case "errors":
			if function.Name() == "New" {
				return true
			}
		case "github.com/pkg/errors":
			switch function.Name() {
			case "Errorf":
				return true
			case "Wrap", "Wrapf":
				if len(call.Args) != 0 {
					expression = call.Args[0]
				}
			}
		}
	}
	object := directObject(info, expression)
	return object != nil && writerErrorGuardProvesNonNil(info, object, use, parents)
}

func writerErrorGuardProvesNonNil(
	info *types.Info,
	object types.Object,
	use ast.Node,
	parents map[ast.Node]ast.Node,
) bool {
	if info == nil || object == nil || use == nil {
		return false
	}
	for ancestor := parents[use]; ancestor != nil; ancestor = parents[ancestor] {
		guard, _ := ancestor.(*ast.IfStmt)
		if guard == nil ||
			guard.Body == nil ||
			use.Pos() <= guard.Body.Lbrace ||
			use.Pos() >= guard.Body.Rbrace ||
			!writerErrorConditionProvesNonNil(info, guard.Cond, object) {
			continue
		}
		functionBody := writerEnclosingFunctionBody(use, parents)
		if writerObjectExposedBetween(
			info,
			functionBody,
			object,
			token.NoPos,
			guard.Pos(),
		) ||
			writerObjectExposedBetween(info, guard, object, guard.Pos(), use.Pos()) ||
			writerObjectExposedBeforeEnclosingClosures(info, use, object, parents) ||
			writerBlockMayMutateObjectBefore(info, guard.Body, object, use.Pos()) {
			return false
		}
		return true
	}
	return false
}

func writerBlockMayMutateObjectBefore(
	info *types.Info,
	block *ast.BlockStmt,
	object types.Object,
	boundary token.Pos,
) bool {
	if info == nil || block == nil || object == nil || !boundary.IsValid() {
		return false
	}
	for _, statement := range block.List {
		if statement.Pos() >= boundary {
			break
		}
		if callStatement, _ := statement.(*ast.ExprStmt); callStatement != nil {
			call, _ := ast.Unparen(callStatement.X).(*ast.CallExpr)
			if callMayMutateWriterObject(info, call, object) {
				return true
			}
		}
		if writerObjectReassignedBetween(
			info,
			statement,
			object,
			statement.Pos() - 1,
			boundary,
		) {
			return true
		}
		if nested, _ := statement.(*ast.BlockStmt);
			nested != nil &&
				writerBlockMayMutateObjectBefore(info, nested, object, boundary) {
			return true
		}
	}
	return false
}

func writerErrorConditionProvesNonNil(
	info *types.Info,
	condition ast.Expr,
	object types.Object,
) bool {
	comparison, _ := ast.Unparen(condition).(*ast.BinaryExpr)
	if info == nil || comparison == nil || object == nil {
		return false
	}
	var other ast.Expr
	switch {
	case directObject(info, comparison.X) == object:
		other = comparison.Y
	case directObject(info, comparison.Y) == object:
		other = comparison.X
	default:
		return false
	}
	if comparison.Op == token.NEQ && isNilExpression(info, other) {
		return true
	}
	return comparison.Op == token.EQL && exactIONonNilError(other, info)
}

func exactIONonNilError(expression ast.Expr, info *types.Info) bool {
	object := directObject(info, expression)
	if selector, _ := ast.Unparen(expression).(*ast.SelectorExpr); selector != nil {
		object = info.ObjectOf(selector.Sel)
	}
	variable, _ := object.(*types.Var)
	return variable != nil &&
		variable.Pkg() != nil &&
		variable.Pkg().Path() == "io" &&
		variable.Name() == "EOF"
}

func writerObjectReassignedBetween(
	info *types.Info,
	root ast.Node,
	object types.Object,
	start token.Pos,
	end token.Pos,
) bool {
	reassigned := false
	ast.Inspect(
		root,
		func(node ast.Node) bool {
			if reassigned || node == nil {
				return !reassigned
			}
			if node.Pos() <= start || node.Pos() >= end {
				return node.Pos() < end
			}
			unary, addressed := node.(*ast.UnaryExpr)
			if addressed &&
				unary.Op == token.AND &&
				expressionUsesObject(info, unary.X, object) {
				reassigned = true
				return false
			}
			call, called := node.(*ast.CallExpr)
			if called && callMayMutateWriterObject(info, call, object) {
				reassigned = true
				return false
			}
			if assignmentReplacesObject(info, node, object, nil) {
				reassigned = true
				return false
			}
			if ranged, _ := node.(*ast.RangeStmt);
				ranged != nil &&
					(directObject(info, ranged.Key) == object ||
						directObject(info, ranged.Value) == object) {
				reassigned = true
				return false
			}
			return true
		},
	)
	return reassigned
}

func callMayMutateWriterObject(info *types.Info, call *ast.CallExpr, object types.Object) bool {
	if info == nil || call == nil || object == nil {
		return false
	}
	if identifier, _ := ast.Unparen(call.Fun).(*ast.Ident); identifier != nil {
		builtin, _ := info.ObjectOf(identifier).(*types.Builtin)
		if builtin != nil && builtin.Name() == "clear" {
			for _, argument := range call.Args {
				if expressionUsesObject(info, argument, object) {
					return true
				}
			}
		}
	}
	for _, argument := range call.Args {
		if expressionUsesObject(info, argument, object) {
			return true
		}
		unary, _ := ast.Unparen(argument).(*ast.UnaryExpr)
		if unary != nil &&
			unary.Op == token.AND &&
			expressionUsesObject(info, unary.X, object) {
			return true
		}
	}
	return false
}

func writerLifecycleMethod(
	info *types.Info,
	call *ast.CallExpr,
	object types.Object,
) (string, int, bool) {
	if info == nil || call == nil || object == nil {
		return "", -1, false
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return "", -1, false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return "", -1, false
	}
	function, _ := selection.Obj().(*types.Func)
	if function == nil {
		return "", -1, false
	}
	receiver, argument := staticReceiverArgument(info, call)
	return function.Name(), argument, receiver == object
}

func writerReturnIsSuccessful(
	ctx *ControlFlowContext,
	returned *ast.ReturnStmt,
	namedError types.Object,
	errorState writerNamedErrorState,
) bool {
	if ctx == nil || returned == nil || ctx.Info() == nil {
		return false
	}
	signature := controlFlowFunctionSignature(ctx.Info(), ctx.Function())
	if signature == nil {
		return false
	}
	if signature.Results() == nil || signature.Results().Len() == 0 {
		return true
	}
	errorIndex := -1
	for index := range signature.Results().Len() {
		if !isBuiltinErrorType(signature.Results().At(index).Type()) {
			continue
		}
		if errorIndex >= 0 {
			return false
		}
		errorIndex = index
	}
	if errorIndex < 0 {
		return true
	}
	if len(returned.Results) == 0 {
		return namedError != nil && errorState.provenNil()
	}
	if delegatedErrorResultIsNil(ctx, returned, signature, errorIndex) {
		return true
	}
	if len(returned.Results) != signature.Results().Len() {
		return false
	}
	errorExpression := returned.Results[errorIndex]
	if isNilExpression(ctx.Info(), errorExpression) {
		return namedError == nil || errorState.provenNil()
	}
	return namedError != nil &&
		errorState.provenNil() &&
		directObject(ctx.Info(), errorExpression) == namedError
}

func delegatedErrorResultIsNil(
	ctx *ControlFlowContext,
	returned *ast.ReturnStmt,
	signature *types.Signature,
	errorIndex int,
) bool {
	if ctx == nil ||
		returned == nil ||
		signature == nil ||
		signature.Results() == nil ||
		errorIndex < 0 {
		return false
	}
	resultIndex := 0
	var expression ast.Expr
	switch {
	case len(returned.Results) == 1:
		expression = returned.Results[0]
		resultIndex = errorIndex
	case len(returned.Results) == signature.Results().Len():
		expression = returned.Results[errorIndex]
	default:
		return false
	}
	call, _ := ast.Unparen(expression).(*ast.CallExpr)
	if call == nil {
		return false
	}
	callee := typeutil.StaticCallee(ctx.Info(), call)
	if callee == nil {
		return false
	}
	calleeSignature, _ := types.Unalias(callee.Type()).(*types.Signature)
	if calleeSignature == nil ||
		calleeSignature.Results() == nil ||
		resultIndex >= calleeSignature.Results().Len() {
		return false
	}
	if len(returned.Results) == 1 &&
		calleeSignature.Results().Len() != signature.Results().Len() {
		return false
	}
	return isBuiltinErrorType(calleeSignature.Results().At(resultIndex).Type()) &&
		ctx.ResultState(call, resultIndex) == NilStateNil
}

func writerNamedErrorResult(ctx *ControlFlowContext) types.Object {
	if ctx == nil || ctx.Info() == nil {
		return nil
	}
	signature := controlFlowFunctionSignature(ctx.Info(), ctx.Function())
	if signature == nil || signature.Results() == nil {
		return nil
	}
	var result *types.Var
	for index := range signature.Results().Len() {
		candidate := signature.Results().At(index)
		if !isBuiltinErrorType(candidate.Type()) {
			continue
		}
		if result != nil {
			return nil
		}
		result = candidate
	}
	if result == nil || result.Name() == "" {
		return nil
	}
	return result
}

func writerNamedErrorStateAfter(
	info *types.Info,
	node ast.Node,
	result types.Object,
	state writerNamedErrorState,
) writerNamedErrorState {
	if info == nil || node == nil || result == nil || state.escaped {
		return state
	}
	escaped := false
	ast.PreorderStack(
		node,
		nil,
		func(current ast.Node, _ []ast.Node) bool {
			if escaped {
				return false
			}
			if literal, nested := current.(*ast.FuncLit); nested {
				escaped = expressionUsesObject(info, literal.Body, result)
				return false
			}
			unary, addressed := current.(*ast.UnaryExpr)
			if addressed &&
				unary.Op.String() == "&" &&
				directObject(info, unary.X) == result {
				escaped = true
				return false
			}
			return true
		},
	)
	if escaped {
		return writerNamedErrorState{nilness: NilStateUnknown, escaped: true}
	}
	if ranged, ok := node.(*ast.RangeStmt);
		ok &&
			(directObject(info, ranged.Key) == result ||
				directObject(info, ranged.Value) == result) {
		state.nilness = NilStateUnknown
		return state
	}
	assignment, ok := node.(*ast.AssignStmt)
	if !ok {
		return state
	}
	for index, target := range assignment.Lhs {
		if directObject(info, target) != result {
			continue
		}
		if len(assignment.Lhs) != len(assignment.Rhs) || index >= len(assignment.Rhs) {
			state.nilness = NilStateUnknown
			return state
		}
		value := assignment.Rhs[index]
		if isNilExpression(info, value) {
			state.nilness = NilStateNil
			continue
		}
		if directObject(info, value) != result {
			state.nilness = NilStateUnknown
		}
	}
	return state
}

func writerNamedErrorStateOnEdge(
	info *types.Info,
	from *cfg.Block,
	to *cfg.Block,
	result types.Object,
	state writerNamedErrorState,
) writerNamedErrorState {
	if state.escaped {
		return state
	}
	nilness, proven := exactObjectNilStateOnEdge(info, from, to, result)
	if proven {
		state.nilness = nilness
	}
	return state
}

func newWriterNamedErrorAnalysis(ctx *ControlFlowContext) writerNamedErrorAnalysis {
	if ctx == nil || ctx.Graph() == nil {
		return writerNamedErrorAnalysis{}
	}
	result := writerNamedErrorResult(ctx)
	if result == nil {
		return writerNamedErrorAnalysis{}
	}
	changeBound := len(ctx.Graph().Blocks) * 3
	if changeBound <= 0 || changeBound > maxStateTransitionChanges {
		changeBound = maxStateTransitionChanges
	}
	analysis := writerNamedErrorAnalysis{result: result}
	analysis.snapshot, analysis.complete = runStateTransitions(
		ctx.Graph(),
		stateTransitionModel[[]writerNamedErrorState]{
			Initial: []writerNamedErrorState{{nilness: NilStateNil}},
			Clone: func(state []writerNamedErrorState) []writerNamedErrorState {
				if len(state) != 1 {
					return []writerNamedErrorState{{nilness: NilStateUnknown}}
				}
				return []writerNamedErrorState{state[0]}
			},
			Merge: func(
				current *[]writerNamedErrorState,
				incoming []writerNamedErrorState,
			) bool {
				if current == nil || len(*current) != 1 || len(incoming) != 1 {
					return false
				}
				merged := (*current)[0]
				merged.escaped = merged.escaped || incoming[0].escaped
				if merged.nilness != incoming[0].nilness {
					merged.nilness = NilStateUnknown
				}
				if merged == (*current)[0] {
					return false
				}
				(*current)[0] = merged
				return true
			},
			Transfer: func(state []writerNamedErrorState, node ast.Node) bool {
				if len(state) == 1 {
					state[0] = writerNamedErrorStateAfter(
						ctx.Info(),
						node,
						result,
						state[0],
					)
				}
				return true
			},
			Edge: func(
				state []writerNamedErrorState,
				from *cfg.Block,
				to *cfg.Block,
			) bool {
				if len(state) == 1 {
					state[0] = writerNamedErrorStateOnEdge(
						ctx.Info(),
						from,
						to,
						result,
						state[0],
					)
				}
				return true
			},
			MaxChanges: changeBound,
		},
	)
	return analysis
}

func (a writerNamedErrorAnalysis) stateAt(
	ctx *ControlFlowContext,
	start obligationStart,
) writerNamedErrorState {
	if ctx == nil ||
		a.result == nil ||
		!a.complete ||
		start.block == nil ||
		start.block.Index < 0 ||
		int(start.block.Index) >= len(a.snapshot.entries) ||
		!a.snapshot.present[start.block.Index] ||
		len(a.snapshot.entries[start.block.Index]) != 1 {
		return writerNamedErrorState{nilness: NilStateUnknown}
	}
	state := a.snapshot.entries[start.block.Index][0]
	nodes := start.block.Nodes
	for _, node := range nodes[:boundedNodeOffset(start.offset, nodes)] {
		state = writerNamedErrorStateAfter(ctx.Info(), node, a.result, state)
	}
	return state
}

func controlFlowFunctionSignature(info *types.Info, function ast.Node) *types.Signature {
	if info == nil || function == nil {
		return nil
	}
	switch function := function.(type) {
	case *ast.FuncDecl:
		if function.Name == nil {
			return nil
		}
		object, _ := info.Defs[function.Name].(*types.Func)
		if object == nil {
			return nil
		}
		signature, _ := types.Unalias(object.Type()).(*types.Signature)
		return signature
	case *ast.FuncLit:
		signature, _ := types.Unalias(info.TypeOf(function)).(*types.Signature)
		return signature
	default:
		return nil
	}
}
