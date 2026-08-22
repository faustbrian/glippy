package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type overlappingEncoderSlicesRule struct{}

// NewOverlappingEncoderSlicesRule constructs the encoder buffer overlap rule
// for product registry composition.
func NewOverlappingEncoderSlicesRule() Rule {
	return overlappingEncoderSlicesRule{}
}

func (overlappingEncoderSlicesRule) Metadata() Metadata {
	return Metadata{
		ID: "overlapping-encoder-slices",
		Summary: "detects overlapping destination and source encoder buffers",
		Documentation: "Expanding byte encoders write multiple destination bytes while continuing to read the source. When destination and source share the same starting storage, earlier writes can overwrite source bytes before the encoder reads them and silently corrupt the result.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only exact static calls to encoding/ascii85.Encode, encoding/hex.Encode, and the Encode methods of encoding/base32.Encoding and encoding/base64.Encoding are recognized.",
			"The rule reports identical typed variable references, identical normalized SSA slice values, and slices proven to share one base and lower bound; aliases or overlapping ranges with different unproven offsets remain conservative.",
			"Package-initializer variable fallback is limited to same-file declarations initialized directly by make or a composite literal; aliases, unknown initializers, and cross-file declarations remain conservative unless SSA independently proves overlap.",
			"No automatic fix is offered because allocating, reusing, or relocating the destination depends on ownership and capacity requirements.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Encode into separate destination storage",
				Incorrect: "hex.Encode(buffer, buffer)",
				Correct: "hex.Encode(destination, source)",
			},
		},
	}
}

func (overlappingEncoderSlicesRule) RunsOnSSAInitializers() {}

func (overlappingEncoderSlicesRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil {
		return nil, fmt.Errorf("overlapping-encoder-slices requires a complete SSA context")
	}
	calls := encoderSyntaxCalls(ctx.Syntax())
	findings := make([]Finding, 0)
	for _, block := range ctx.Function().Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(ssa.CallInstruction)
			if !ok {
				continue
			}
			destination, source, recognized := encoderSliceArguments(call.Common())
			if !recognized {
				continue
			}
			callExpression := calls[call.Common().Pos()]
			if callExpression == nil {
				if _, initializer := ctx.Syntax().(*ast.File); initializer {
					continue
				}
				return nil, fmt.Errorf(
					"cannot map proven encoder overlap call to source",
				)
			}
			if len(callExpression.Args) < 2 {
				return nil, fmt.Errorf(
					"proven encoder overlap call has fewer than two source arguments",
				)
			}
			destinationExpression := callExpression.Args[len(callExpression.Args) - 2]
			sourceExpression := callExpression.Args[len(callExpression.Args) - 1]
			if !encoderSlicesOverlap(destination, source) {
				if encoderSliceDefinitelyNil(destination) ||
					encoderSliceDefinitelyNil(source) ||
					!encoderSameVariable(
						ctx,
						destinationExpression,
						sourceExpression,
					) {
					continue
				}
			}
			range_, err := ctx.Range(destinationExpression)
			if err != nil {
				return nil, err
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "overlapping-encoder-slices",
					Message: "encoder destination and source slices overlap",
					Range: range_,
					Help: "use a destination backed by different memory from the source",
				},
			)
		}
	}
	return findings, nil
}

func encoderSameVariable(ctx *SSAContext, destination, source ast.Expr) bool {
	if ctx == nil || ctx.Info() == nil {
		return false
	}
	info := ctx.Info()
	destinationIdentifier, destinationOK := ast.Unparen(destination).(*ast.Ident)
	sourceIdentifier, sourceOK := ast.Unparen(source).(*ast.Ident)
	if !destinationOK || !sourceOK {
		return false
	}
	destinationObject := info.ObjectOf(destinationIdentifier)
	if destinationObject == nil || destinationObject != info.ObjectOf(sourceIdentifier) {
		return false
	}
	file, initializer := ctx.Syntax().(*ast.File)
	if !initializer {
		return true
	}
	found, canOverlap := encoderPackageVariableCanOverlap(file, info, destinationObject)
	return found && canOverlap
}

func encoderPackageVariableCanOverlap(
	file *ast.File,
	info *types.Info,
	object types.Object,
) (bool, bool) {
	if file == nil || info == nil || object == nil {
		return false, false
	}
	for _, declaration := range file.Decls {
		variables, ok := declaration.(*ast.GenDecl)
		if !ok || variables.Tok != token.VAR {
			continue
		}
		for _, specification := range variables.Specs {
			values, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range values.Names {
				if info.Defs[name] != object {
					continue
				}
				if len(values.Values) != len(values.Names) {
					return true, false
				}
				return true, encoderSourceExpressionDefinitelyNonNil(
					info,
					values.Values[index],
				)
			}
		}
	}
	return false, false
}

func encoderSourceExpressionDefinitelyNonNil(info *types.Info, expression ast.Expr) bool {
	expression = ast.Unparen(expression)
	if _, composite := expression.(*ast.CompositeLit); composite {
		return true
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	function, ok := ast.Unparen(call.Fun).(*ast.Ident)
	return ok && info.ObjectOf(function) == types.Universe.Lookup("make")
}

func encoderSyntaxCalls(root ast.Node) map[token.Pos]*ast.CallExpr {
	result := make(map[token.Pos]*ast.CallExpr)
	inspectOwnedFunction(
		root,
		func(node ast.Node) {
			call, ok := node.(*ast.CallExpr)
			if ok {
				result[call.Lparen] = call
			}
		},
	)
	return result
}

func encoderSliceArguments(call *ssa.CallCommon) (ssa.Value, ssa.Value, bool) {
	if call == nil || call.StaticCallee() == nil {
		return nil, nil, false
	}
	function, _ := call.StaticCallee().Object().(*types.Func)
	if function == nil || function.Pkg() == nil || function.Name() != "Encode" {
		return nil, nil, false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return nil, nil, false
	}
	switch function.Pkg().Path() {
	case "encoding/ascii85", "encoding/hex":
		if signature.Recv() != nil || len(call.Args) != 2 {
			return nil, nil, false
		}
		return call.Args[0], call.Args[1], true
	case "encoding/base32", "encoding/base64":
		if !encoderReceiver(signature.Recv(), function.Pkg().Path()) {
			return nil, nil, false
		}
		switch len(call.Args) {
		case 2:
			return call.Args[0], call.Args[1], true
		case 3:
			return call.Args[1], call.Args[2], true
		default:
			return nil, nil, false
		}
	default:
		return nil, nil, false
	}
}

func encoderReceiver(receiver *types.Var, packagePath string) bool {
	if receiver == nil {
		return false
	}
	type_ := receiver.Type()
	if pointer, ok := type_.(*types.Pointer); ok {
		type_ = pointer.Elem()
	}
	named, ok := type_.(*types.Named)
	return ok &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == packagePath &&
		named.Obj().Name() == "Encoding"
}

func encoderSlicesOverlap(destination, source ssa.Value) bool {
	destination = encoderSliceValue(destination)
	source = encoderSliceValue(source)
	if destination == nil ||
		source == nil ||
		encoderNilSlice(destination) ||
		encoderNilSlice(source) {
		return false
	}
	if destination == source {
		return true
	}
	destinationSlice, destinationOK := destination.(*ssa.Slice)
	sourceSlice, sourceOK := source.(*ssa.Slice)
	if !destinationOK ||
		!sourceOK ||
		encoderSliceValue(destinationSlice.X) != encoderSliceValue(sourceSlice.X) {
		return false
	}
	return encoderSliceBoundsEqual(destinationSlice.Low, sourceSlice.Low)
}

func encoderSliceValue(value ssa.Value) ssa.Value {
	seen := make(map[ssa.Value]struct{})
	var result ssa.Value
	var visit func(ssa.Value) bool
	visit = func(current ssa.Value) bool {
		if current == nil {
			return false
		}
		if _, found := seen[current]; found {
			return true
		}
		seen[current] = struct{}{}
		switch current := current.(type) {
		case *ssa.ChangeType:
			return visit(current.X)
		case *ssa.Phi:
			for _, edge := range current.Edges {
				if !visit(edge) {
					return false
				}
			}
			return true
		default:
			if result == nil {
				result = current
				return true
			}
			return result == current
		}
	}
	if !visit(value) {
		return nil
	}
	return result
}

func encoderNilSlice(value ssa.Value) bool {
	constant, ok := value.(*ssa.Const)
	return ok && constant.IsNil()
}

func encoderSliceDefinitelyNil(value ssa.Value) bool {
	value = encoderSliceValue(value)
	return value != nil && encoderNilSlice(value)
}

func encoderSliceBoundsEqual(left, right ssa.Value) bool {
	left = encoderSliceValue(left)
	right = encoderSliceValue(right)
	if left == right {
		return true
	}
	leftConstant, leftOK := left.(*ssa.Const)
	rightConstant, rightOK := right.(*ssa.Const)
	return leftOK &&
		rightOK &&
		leftConstant.Value != nil &&
		rightConstant.Value != nil &&
		constant.Compare(leftConstant.Value, token.EQL, rightConstant.Value)
}
