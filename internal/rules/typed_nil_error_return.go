package rules

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type typedNilErrorReturnRule struct{}

// NewTypedNilErrorReturnRule constructs the definite typed-nil error return
// rule for product registry composition.
func NewTypedNilErrorReturnRule() Rule {
	return typedNilErrorReturnRule{}
}

func (typedNilErrorReturnRule) Metadata() Metadata {
	return Metadata{
		ID: "typed-nil-error-return",
		Summary: "detects definitely nil concrete values returned as errors",
		Documentation: "Converting a nil concrete value to an error interface produces a non-nil interface because its dynamic type remains present. The rule reports explicit return operands whose SSA value is definitely nil and whose concrete type is converted to an error interface result.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only explicit one-to-one return operands are considered; bare returns and tuple-returning calls are excluded.",
			"Values are reported only when SSA proves every incoming value nil; values that may be nil are conservatively excluded.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Return a nil error interface directly",
				Incorrect: "var problem *Problem\nreturn problem",
				Correct: "if problem != nil { return problem }\nreturn nil",
			},
		},
	}
}

func (typedNilErrorReturnRule) RequiresSSADebug() {}

func (typedNilErrorReturnRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("typed-nil-error-return requires a complete SSA context")
	}
	errorObject := types.Universe.Lookup("error")
	if errorObject == nil {
		return nil, fmt.Errorf(
			"typed-nil-error-return cannot resolve the built-in error type",
		)
	}
	errorInterface, ok := errorObject.Type().Underlying().(*types.Interface)
	if !ok {
		return nil, fmt.Errorf("typed-nil-error-return resolved a non-interface error type")
	}
	results := ctx.Function().Signature.Results()
	if results == nil || results.Len() == 0 {
		return nil, nil
	}
	expressionValues := overwrittenErrorExpressionValues(ctx.Function())
	findings := make([]Finding, 0)
	var runErr error
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			if runErr != nil {
				return
			}
			returned, ok := node.(*ast.ReturnStmt)
			if !ok || len(returned.Results) != results.Len() {
				return
			}
			for index, expression := range returned.Results {
				resultType := results.At(index).Type()
				if !types.IsInterface(resultType) ||
					!types.Implements(resultType, errorInterface) {
					continue
				}
				expression = ast.Unparen(expression)
				expressionType := ctx.Info().TypeOf(expression)
				if expressionType == nil ||
					types.IsInterface(expressionType) ||
					isUntypedNil(expressionType) ||
					!types.AssignableTo(expressionType, resultType) ||
					!definitelyNilSSAValue(expressionValues[expression], nil) {
					continue
				}
				range_, err := ctx.Range(expression)
				if err != nil {
					runErr = err
					return
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "typed-nil-error-return",
						Message: "this definitely nil concrete error becomes a non-nil error interface",
						Range: range_,
						Help: "return an untyped nil error interface on the success path",
					},
				)
			}
		},
	)
	if runErr != nil {
		return nil, runErr
	}
	return findings, nil
}

func isUntypedNil(valueType types.Type) bool {
	basic, ok := valueType.(*types.Basic)
	return ok && basic.Kind() == types.UntypedNil
}

func definitelyNilSSAValue(value ssa.Value, seen map[ssa.Value]struct{}) bool {
	if value == nil {
		return false
	}
	if seen != nil {
		if _, found := seen[value]; found {
			return true
		}
	} else {
		seen = make(map[ssa.Value]struct{})
	}
	seen[value] = struct{}{}
	switch value := value.(type) {
	case *ssa.Const:
		return value.IsNil()
	case *ssa.MakeInterface:
		return definitelyNilSSAValue(value.X, seen)
	case *ssa.ChangeInterface:
		return definitelyNilSSAValue(value.X, seen)
	case *ssa.ChangeType:
		return definitelyNilSSAValue(value.X, seen)
	case *ssa.Convert:
		return definitelyNilSSAValue(value.X, seen)
	case *ssa.Phi:
		if len(value.Edges) == 0 {
			return false
		}
		for _, edge := range value.Edges {
			if !definitelyNilSSAValue(edge, seen) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
