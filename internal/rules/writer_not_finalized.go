package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/types/typeutil"
)

type writerNotFinalizedRule struct{}

type writerLifecycleSpec struct {
	packagePath string
	constructors map[string]int
	useMethods map[string]struct{}
	finalizer string
}

type writerLifecycleCandidate struct {
	identifier *ast.Ident
	object types.Object
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
	errorState NilState
}

type writerNodeTransition struct {
	used bool
	terminal obligationEffect
}

type writerNamedErrorAnalysis struct {
	result types.Object
	snapshot stateTransitionSnapshot[[]NilState]
	complete bool
}

var requiredWriterLifecycleSpecs = []writerLifecycleSpec{
	{
		packagePath: "archive/tar",
		constructors: map[string]int{"NewWriter": 0},
		useMethods: map[string]struct{}{"AddFS": {}, "Write": {}, "WriteHeader": {}},
		finalizer: "Close",
	},
	{
		packagePath: "compress/gzip",
		constructors: map[string]int{"NewWriter": 0, "NewWriterLevel": 0},
		useMethods: map[string]struct{}{"Flush": {}, "Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "encoding/ascii85",
		constructors: map[string]int{"NewEncoder": 0},
		useMethods: map[string]struct{}{"Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "encoding/base32",
		constructors: map[string]int{"NewEncoder": 0},
		useMethods: map[string]struct{}{"Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "encoding/base64",
		constructors: map[string]int{"NewEncoder": 0},
		useMethods: map[string]struct{}{"Write": {}},
		finalizer: "Close",
	},
	{
		packagePath: "mime/multipart",
		constructors: map[string]int{"NewWriter": 0},
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
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The exact contract covers direct local assignment results and initialized variable declarations from archive/tar.NewWriter, compress/gzip.NewWriter or NewWriterLevel, encoding/ascii85.NewEncoder, encoding/base32.NewEncoder, encoding/base64.NewEncoder, and mime/multipart.NewWriter.",
			"Only exact output-producing receiver methods establish use; construction and configuration alone do not require finalization.",
			"A function with one named error result may classify a bare return or that exact result as successful only while CFG dataflow proves the result retains its nil zero value through direct nil or self assignments; other assignments, address escape, closure capture, multiple error results, delegated values, and unknown joins remain conservative.",
			"Aliases, fields, containers, closures, method values, asynchronous calls, and transfers stop analysis because exact ownership or execution order is unavailable.",
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
	start, found := obligationStartAfter(ctx.Graph(), node)
	if object == nil || !found {
		return writerLifecycleCandidate{}, false
	}
	return writerLifecycleCandidate{
		identifier: identifier,
		object: object,
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
			transition := writerLifecycleNodeTransition(ctx.Info(), node, candidate)
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
			work = append(
				work,
				writerLifecycleWork{
					block: successor,
					state: state,
					errorState: errorState,
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
	errorState NilState,
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
		return namedError != nil && errorState == NilStateNil
	}
	if len(returned.Results) != signature.Results().Len() {
		return false
	}
	errorExpression := returned.Results[errorIndex]
	if isNilExpression(ctx.Info(), errorExpression) {
		return namedError == nil || errorState == NilStateNil
	}
	return namedError != nil &&
		errorState == NilStateNil &&
		directObject(ctx.Info(), errorExpression) == namedError
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
	state NilState,
) NilState {
	if info == nil || node == nil || result == nil || state != NilStateNil {
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
		return NilStateUnknown
	}
	if ranged, ok := node.(*ast.RangeStmt);
		ok &&
			(directObject(info, ranged.Key) == result ||
				directObject(info, ranged.Value) == result) {
		return NilStateUnknown
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
			return NilStateUnknown
		}
		value := assignment.Rhs[index]
		if !isNilExpression(info, value) && directObject(info, value) != result {
			return NilStateUnknown
		}
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
		stateTransitionModel[[]NilState]{
			Initial: []NilState{NilStateNil},
			Clone: func(state []NilState) []NilState {
				if len(state) != 1 {
					return []NilState{NilStateUnknown}
				}
				return []NilState{state[0]}
			},
			Merge: func(current *[]NilState, incoming []NilState) bool {
				if current == nil || len(*current) != 1 || len(incoming) != 1 {
					return false
				}
				if (*current)[0] == incoming[0] ||
					(*current)[0] == NilStateUnknown {
					return false
				}
				(*current)[0] = NilStateUnknown
				return true
			},
			Transfer: func(state []NilState, node ast.Node) bool {
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
			MaxChanges: changeBound,
		},
	)
	return analysis
}

func (a writerNamedErrorAnalysis) stateAt(ctx *ControlFlowContext, start obligationStart) NilState {
	if ctx == nil ||
		a.result == nil ||
		!a.complete ||
		start.block == nil ||
		start.block.Index < 0 ||
		int(start.block.Index) >= len(a.snapshot.entries) ||
		!a.snapshot.present[start.block.Index] ||
		len(a.snapshot.entries[start.block.Index]) != 1 {
		return NilStateUnknown
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
