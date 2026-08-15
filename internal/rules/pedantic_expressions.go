package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
)

type unnecessaryConversionRule struct{}

type unnecessarySprintfRule struct{}

const (
	removeUnnecessaryConversionFix = "remove-unnecessary-conversion"
	replaceUnnecessarySprintfFix = "replace-unnecessary-sprintf"
)

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
		Fixes: []FixMetadata{
			{
				Name: removeUnnecessaryConversionFix,
				Description: "replace the identity conversion with its value",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"Only conversions whose source and target types are identical under go/types are reported.",
			"Conversions between distinct defined types and underlying types remain visible because they establish a real type boundary.",
			"Compile-time constant conversions remain visible because they can document an intentional type boundary.",
			"The suggestion retains grouping for non-primary operands and is withheld when conversion-delimiter comments would be lost.",
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
	finding := Finding{
		MessageKey: "identity-conversion",
		Message: "conversion is unnecessary because the value already has the target type",
		Range: range_,
		Help: "use the value directly",
	}
	argumentRange, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRange(ctx.File().Comments(), range_, argumentRange) {
		finding.WithheldFixes = []WithheldFix{
			{
				Name: removeUnnecessaryConversionFix,
				Reason: FixWithheldComments,
				Message: "removing this conversion would remove comments",
			},
		}
		return []Finding{finding}, nil
	}
	replacement, found := ctx.File().Slice(argumentRange)
	if !found {
		return nil, fmt.Errorf(
			"unnecessary conversion argument has an invalid source range",
		)
	}
	finding.Fixes = []Fix{
		{
			Name: removeUnnecessaryConversionFix,
			Safety: FixSuggestion,
			Edits: []Edit{
				{
					Range: range_,
					NewText: directExpressionReplacement(
						call.Args[0],
						replacement,
					),
				},
			},
		},
	}
	return []Finding{finding}, nil
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
		Fixes: []FixMetadata{
			{
				Name: replaceUnnecessarySprintfFix,
				Description: "replace fmt.Sprintf with the direct string representation",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"Only the standard library fmt.Sprintf function with one exact compile-time %s directive and one argument is checked.",
			"Values implementing fmt.Stringer, fmt.Formatter, or error, along with interface, type-parameter, rune-slice, and wider formatting cases, are excluded because their output contracts can differ.",
			"The suggestion preserves the result's predeclared string type and is withheld when format-call comments would be lost.",
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
		!directStringRepresentation(ctx.Info().TypeOf(call.Args[1])) ||
		hasCustomStringFormatting(ctx.Package(), ctx.Info().TypeOf(call.Args[1])) {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "direct-string-representation",
		Message: "fmt.Sprintf is unnecessary for this string representation",
		Range: range_,
		Help: "use the string directly or convert the value with string",
	}
	argumentRange, err := ctx.Range(call.Args[1])
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRange(ctx.File().Comments(), range_, argumentRange) {
		finding.WithheldFixes = []WithheldFix{
			{
				Name: replaceUnnecessarySprintfFix,
				Reason: FixWithheldComments,
				Message: "replacing this formatting call would remove comments",
			},
		}
		return []Finding{finding}, nil
	}
	argumentSource, found := ctx.File().Slice(argumentRange)
	if !found {
		return nil, fmt.Errorf("unnecessary sprintf argument has an invalid source range")
	}
	replacement := "string(" + argumentSource + ")"
	if types.Identical(
		types.Unalias(ctx.Info().TypeOf(call.Args[1])),
		types.Typ[types.String],
	) {
		replacement = directExpressionReplacement(call.Args[1], argumentSource)
	}
	finding.Fixes = []Fix{
		{
			Name: replaceUnnecessarySprintfFix,
			Safety: FixSuggestion,
			Edits: []Edit{{Range: range_, NewText: replacement}},
		},
	}
	return []Finding{finding}, nil
}

func directExpressionReplacement(expression ast.Expr, text string) string {
	switch ast.Unparen(expression).(type) {
	case *ast.Ident,
		*ast.BasicLit,
		*ast.FuncLit,
		*ast.CompositeLit,
		*ast.SelectorExpr,
		*ast.IndexExpr,
		*ast.IndexListExpr,
		*ast.SliceExpr,
		*ast.TypeAssertExpr,
		*ast.CallExpr,
		*ast.StarExpr,
		*ast.UnaryExpr:
		return text
	default:
		return "(" + text + ")"
	}
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

func hasCustomStringFormatting(package_ *types.Package, type_ types.Type) bool {
	if package_ == nil || type_ == nil {
		return true
	}
	if errorType, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface);
		ok && types.Implements(type_, errorType) {
		return true
	}
	fmtPackage := package_
	if package_.Path() != "fmt" {
		fmtPackage = nil
		for _, imported := range package_.Imports() {
			if imported.Path() == "fmt" {
				fmtPackage = imported
				break
			}
		}
	}
	if fmtPackage == nil {
		return true
	}
	for _, name := range []string{"Formatter", "Stringer"} {
		object := fmtPackage.Scope().Lookup(name)
		if object == nil {
			return true
		}
		interface_, ok := types.Unalias(object.Type()).Underlying().(*types.Interface)
		if !ok || types.Implements(type_, interface_) {
			return true
		}
	}
	return false
}
