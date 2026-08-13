package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
)

type unnecessaryConversionRule struct{}

type unnecessarySprintfRule struct{}

// NewUnnecessaryConversionRule constructs the identity-conversion rule for
// product registry composition.
func NewUnnecessaryConversionRule() Rule {
	return unnecessaryConversionRule{}
}

// NewUnnecessarySprintfRule constructs the direct string-representation rule
// for product registry composition.
func NewUnnecessarySprintfRule() Rule {
	return unnecessarySprintfRule{}
}

func (unnecessaryConversionRule) Metadata() Metadata {
	return Metadata{
		ID: "unnecessary-conversion",
		Summary: "detects conversions whose input already has the target type",
		Documentation: "Converting a value to the exact type it already has cannot change its representation or method set. The extra conversion obscures the expression and can survive after a refactor has made an earlier type boundary redundant.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryStyle, CategoryMaintainability},
		KnownLimitations: []string{
			"Only conversions whose source and target types are identical under go/types are reported.",
			"Conversions between distinct defined types and underlying types remain visible because they establish a real type boundary.",
			"Compile-time constant conversions remain visible because they can document an intentional type boundary.",
			"No fix is offered until parent precedence and comments inside conversion delimiters have a dedicated source-preservation proof.",
		},
		Examples: []Example{
			{
				Title: "Use the value directly",
				Incorrect: "text := string(value) // value is already string",
				Correct: "text := value",
			},
		},
	}
}

func (unnecessaryConversionRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("unnecessary-conversion requires a call expression")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("unnecessary-conversion requires complete type information")
	}
	if len(call.Args) != 1 || !ctx.Info().Types[call.Fun].IsType() {
		return nil, nil
	}
	target := ctx.Info().TypeOf(call.Fun)
	source := ctx.Info().TypeOf(call.Args[0])
	if target == nil ||
		source == nil ||
		ctx.Info().Types[call.Args[0]].Value != nil ||
		!types.Identical(target, source) {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "identity-conversion",
			Message: "conversion is unnecessary because the value already has the target type",
			Range: range_,
			Help: "use the value directly",
		},
	}, nil
}

func (unnecessarySprintfRule) Metadata() Metadata {
	return Metadata{
		ID: "unnecessary-sprintf",
		Summary: "detects fmt.Sprintf calls that only reproduce a string value",
		Documentation: "fmt.Sprintf with the exact format %s is unnecessary when its only argument is already a string, has a string underlying type, or is a byte slice. Direct use or conversion is clearer and avoids the formatting machinery.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryStyle, CategoryPerformance},
		KnownLimitations: []string{
			"Only the standard library fmt.Sprintf function with one exact compile-time %s directive and one argument is checked.",
			"Stringer, Formatter, interface, type-parameter, rune-slice, and wider formatting cases are excluded because their output contracts can differ.",
			"No fix is offered until argument comments and parent-expression precedence have a dedicated source-preservation proof.",
		},
		Examples: []Example{
			{
				Title: "Use an existing string directly",
				Incorrect: `text := fmt.Sprintf("%s", value)`,
				Correct: "text := value",
			},
		},
	}
}

func (unnecessarySprintfRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("unnecessary-sprintf requires a call expression")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("unnecessary-sprintf requires complete type information")
	}
	if len(call.Args) != 2 || !isStandardFunction(ctx.Info(), call.Fun, "fmt", "Sprintf") {
		return nil, nil
	}
	format := ctx.Info().Types[call.Args[0]].Value
	if format == nil ||
		format.Kind() != constant.String ||
		constant.StringVal(format) != "%s" ||
		!directStringRepresentation(ctx.Info().TypeOf(call.Args[1])) {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "direct-string-representation",
			Message: "fmt.Sprintf is unnecessary for this string representation",
			Range: range_,
			Help: "use the string directly or convert the value with string",
		},
	}, nil
}

func isStandardFunction(info *types.Info, expression ast.Expr, packagePath, name string) bool {
	if info == nil {
		return false
	}
	expression = ast.Unparen(expression)
	var object types.Object
	switch current := expression.(type) {
	case *ast.Ident:
		object = info.ObjectOf(current)
	case *ast.SelectorExpr:
		object = info.ObjectOf(current.Sel)
	default:
		return false
	}
	function, _ := object.(*types.Func)
	return function != nil &&
		function.Pkg() != nil &&
		function.Pkg().Path() == packagePath &&
		function.Name() == name
}

func directStringRepresentation(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	unaliased := types.Unalias(type_)
	if _, parameter := unaliased.(*types.TypeParam); parameter {
		return false
	}
	if basic, ok := unaliased.Underlying().(*types.Basic); ok {
		return basic.Kind() == types.String
	}
	slice, _ := unaliased.Underlying().(*types.Slice)
	if slice == nil {
		return false
	}
	element, _ := types.Unalias(slice.Elem()).Underlying().(*types.Basic)
	return element != nil && element.Kind() == types.Uint8
}
