package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
)

type nilContextRule struct{}

type timeDurationUnitRule struct{}

type invalidRandomBoundRule struct{}

// NewNilContextRule constructs the nil context argument rule for product
// registry composition.
func NewNilContextRule() Rule {
	return nilContextRule{}
}

// NewTimeDurationUnitRule constructs the bare duration literal rule for
// product registry composition.
func NewTimeDurationUnitRule() Rule {
	return timeDurationUnitRule{}
}

// NewInvalidRandomBoundRule constructs the constant random-bound rule for
// product registry composition.
func NewInvalidRandomBoundRule() Rule {
	return invalidRandomBoundRule{}
}

func (nilContextRule) Metadata() Metadata {
	includeTestsDefault := BooleanOption(false)
	return Metadata{
		ID: "nil-context",
		Summary: "detects nil passed where a context.Context is required",
		Documentation: "The context package contract requires callers to pass context.TODO when no context is available instead of passing nil. A nil context can panic in standard-library APIs and violates the propagation contract expected by Go code.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		Options: []OptionMetadata{
			{
				Name: "include-tests",
				Summary: "report nil contexts in files whose base name ends in _test.go",
				Kind: OptionBoolean,
				Default: &includeTestsDefault,
			},
		},
		KnownLimitations: []string{
			"The initial rule reports the predeclared nil identifier passed directly to a context.Context parameter.",
			"Nil contexts hidden behind interface values, variables, or helper returns require value-flow analysis and are not inferred.",
			"Test files are excluded by default because invalid-input contract tests deliberately pass nil; include-tests enables them.",
		},
		Examples: []Example{
			{
				Title: "Use context.TODO when no context is available",
				Incorrect: "client.Do(nil, request)",
				Correct: "client.Do(context.TODO(), request)",
			},
		},
	}
}

func (nilContextRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("nil-context requires a call expression")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("nil-context requires complete type information")
	}
	includeTests, found := ctx.BooleanOption("include-tests")
	if !found {
		return nil, fmt.Errorf("nil-context requires the include-tests option")
	}
	if !includeTests && strings.HasSuffix(filepath.Base(ctx.File().Path()), "_test.go") {
		return nil, nil
	}
	signature, _ := types.Unalias(ctx.Info().TypeOf(call.Fun)).(*types.Signature)
	if signature == nil || signature.Params() == nil {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for index, argument := range call.Args {
		parameterIndex := index
		if signature.Variadic() && parameterIndex >= signature.Params().Len() - 1 {
			parameterIndex = signature.Params().Len() - 1
		}
		if parameterIndex < 0 || parameterIndex >= signature.Params().Len() {
			continue
		}
		parameterType := signature.Params().At(parameterIndex).Type()
		if signature.Variadic() && parameterIndex == signature.Params().Len() - 1 {
			variadic, _ := types.Unalias(parameterType).(*types.Slice)
			if variadic == nil {
				continue
			}
			parameterType = variadic.Elem()
		}
		if !isStandardContext(parameterType) {
			continue
		}
		identifier, _ := ast.Unparen(argument).(*ast.Ident)
		if identifier == nil || identifier.Name != "nil" {
			continue
		}
		range_, err := ctx.Range(argument)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "nil-context",
				Message: "nil must not be passed as a context.Context",
				Range: range_,
				Help: "pass context.TODO() when no suitable context is available",
			},
		)
	}
	return findings, nil
}

func isStandardContext(type_ types.Type) bool {
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "context" &&
		named.Obj().Name() == "Context"
}

func (timeDurationUnitRule) Metadata() Metadata {
	return Metadata{
		ID: "time-duration-unit",
		Summary: "detects nonzero bare integer durations passed to waiting APIs",
		Documentation: "Duration arguments are nanoseconds. A nonzero bare integer passed to time.Sleep, time.After, timer, ticker, or AfterFunc APIs is usually a missing multiplication by time.Millisecond, time.Second, or another explicit unit.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Only direct nonzero integer literals, optionally parenthesized or signed, are reported.",
			"Named constants and explicit time.Duration conversions are treated as deliberate even when their numeric value is small.",
		},
		Examples: []Example{
			{
				Title: "Spell the duration unit",
				Incorrect: "time.Sleep(5)",
				Correct: "time.Sleep(5 * time.Second)",
			},
		},
	}
}

func (timeDurationUnitRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("time-duration-unit requires a call expression")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("time-duration-unit requires complete type information")
	}
	index, recognized := timeDurationParameter(ctx.Info(), call)
	if !recognized ||
		index >= len(call.Args) ||
		!bareNonzeroInteger(ctx.Info(), call.Args[index]) {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[index])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "time-duration-unit",
			Message: "bare integer duration is interpreted as nanoseconds",
			Range: range_,
			Help: "multiply by an explicit time unit",
		},
	}, nil
}

func timeDurationParameter(info *types.Info, call *ast.CallExpr) (int, bool) {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return 0, false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != "time" {
		return 0, false
	}
	switch function.Name() {
	case "Sleep", "After", "NewTimer", "NewTicker", "Tick", "AfterFunc":
		return 0, true
	default:
		return 0, false
	}
}

func bareNonzeroInteger(info *types.Info, expression ast.Expr) bool {
	expression = ast.Unparen(expression)
	switch current := expression.(type) {
	case *ast.BasicLit:
		if current.Kind != token.INT {
			return false
		}
	case *ast.UnaryExpr:
		if current.Op != token.ADD && current.Op != token.SUB {
			return false
		}
		literal, _ := ast.Unparen(current.X).(*ast.BasicLit)
		if literal == nil || literal.Kind != token.INT {
			return false
		}
	default:
		return false
	}
	value := info.Types[expression].Value
	return value != nil && value.Kind() == constant.Int && constant.Sign(value) != 0
}

func (invalidRandomBoundRule) Metadata() Metadata {
	return Metadata{
		ID: "invalid-random-bound",
		Summary: "detects random bounds that panic or always return zero",
		Documentation: "Bounded math/rand and math/rand/v2 APIs generate values in the half-open interval [0,n). A constant nonpositive bound panics, while a bound of one can only produce zero and usually indicates an off-by-one error or an ineffective attempt to choose between alternatives.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only direct calls to exact math/rand and math/rand/v2 package functions or Rand methods are recognized; function values and interface dispatch remain conservative.",
			"Only compile-time integer bounds less than or equal to one are reported; value flow through variables is not inferred.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Use the exclusive upper bound",
				Incorrect: "choice := rand.Intn(1)",
				Correct: "choice := rand.Intn(2)",
			},
		},
	}
}

func (invalidRandomBoundRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"invalid-random-bound requires a call expression and type information",
		)
	}
	if len(call.Args) != 1 || !isBoundedRandomCall(ctx.Info(), call) {
		return nil, nil
	}
	argument := ast.Unparen(call.Args[0])
	value := ctx.Info().Types[argument].Value
	if value == nil || value.Kind() != constant.Int {
		return nil, nil
	}
	sign := constant.Sign(value)
	if sign > 0 && !constant.Compare(value, token.EQL, constant.MakeInt64(1)) {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	if sign <= 0 {
		return []Finding{
			{
				MessageKey: "nonpositive",
				Message: "random call with a nonpositive bound panics",
				Range: range_,
				Help: "pass a positive exclusive upper bound",
			},
		}, nil
	}
	return []Finding{
		{
			MessageKey: "constant-result",
			Message: "random call with bound 1 always returns zero",
			Range: range_,
			Help: "increase the exclusive upper bound to include another result",
		},
	}, nil
}

func isBoundedRandomCall(info *types.Info, call *ast.CallExpr) bool {
	selector := directCallSelector(call.Fun)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return false
	}
	switch function.Pkg().Path() {
	case "math/rand":
		switch function.Name() {
		case "Intn", "Int31n", "Int63n":
			return signature.Recv() == nil ||
				isNamedReceiver(signature.Recv().Type(), "math/rand", "Rand")
		}
	case "math/rand/v2":
		switch function.Name() {
		case "N":
			return signature.Recv() == nil
		case "IntN", "Int32N", "Int64N", "UintN", "Uint32N", "Uint64N":
			return signature.Recv() == nil ||
				isNamedReceiver(signature.Recv().Type(), "math/rand/v2", "Rand")
		}
	}
	return false
}

func directCallSelector(expression ast.Expr) *ast.SelectorExpr {
	switch expression := ast.Unparen(expression).(type) {
	case *ast.SelectorExpr:
		return expression
	case *ast.IndexExpr:
		selector, _ := ast.Unparen(expression.X).(*ast.SelectorExpr)
		return selector
	case *ast.IndexListExpr:
		selector, _ := ast.Unparen(expression.X).(*ast.SelectorExpr)
		return selector
	default:
		return nil
	}
}

func isNamedReceiver(type_ types.Type, packagePath string, name string) bool {
	type_ = types.Unalias(type_)
	if pointer, ok := type_.(*types.Pointer); ok {
		type_ = types.Unalias(pointer.Elem())
	}
	named, _ := type_.(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == packagePath &&
		named.Obj().Name() == name
}
