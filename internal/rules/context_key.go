package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type contextKeyRule struct{}

func (contextKeyRule) Metadata() Metadata {
	return Metadata{
		ID: "context-key",
		Summary: "detects unsafe keys passed to context.WithValue",
		Documentation: "A context key must be comparable and should use a package-specific defined type to avoid collisions. Built-in types and anonymous empty structs can collide across packages, while nil or non-comparable keys cause context.WithValue to panic.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Interface-typed keys and type parameters whose type sets are not proven entirely non-comparable are not reported because their dynamic value is not proven by the types tier.",
			"The rule recognizes only the standard library context.WithValue function by object identity.",
		},
		Examples: []Example{
			{
				Title: "Use a package-specific context key type",
				Incorrect: `context.WithValue(ctx, "request-id", requestID)`,
				Correct: `type requestIDKey struct{}
context.WithValue(ctx, requestIDKey{}, requestID)`,
			},
		},
	}
}

func (contextKeyRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("context-key requires a call expression")
	}
	if len(call.Args) != 3 || !isContextWithValue(ctx.Info(), call.Fun) {
		return nil, nil
	}
	key := call.Args[1]
	type_ := ctx.Info().TypeOf(key)
	if type_ == nil {
		return nil, fmt.Errorf("context-key could not resolve the key type")
	}
	messageKey, message := contextKeyDiagnostic(type_, ctx.Package())
	if identifier, ok := key.(*ast.Ident); ok && identifier.Name == "nil" {
		messageKey = "nil"
		message = "context.WithValue key is nil and will panic"
	}
	if messageKey == "" {
		return nil, nil
	}
	range_, err := ctx.Range(key)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: messageKey,
			Message: message,
			Range: range_,
			Help: "use a comparable package-specific defined key type",
		},
	}, nil
}

func isContextWithValue(info *types.Info, expression ast.Expr) bool {
	if info == nil {
		return false
	}
	expression = ast.Unparen(expression)
	var object types.Object
	switch expression := expression.(type) {
	case *ast.Ident:
		object = info.Uses[expression]
	case *ast.SelectorExpr:
		object = info.Uses[expression.Sel]
	default:
		return false
	}
	function, ok := object.(*types.Func)
	return ok &&
		function.Pkg() != nil &&
		function.Pkg().Path() == "context" &&
		function.Name() == "WithValue"
}

func contextKeyDiagnostic(type_ types.Type, package_ *types.Package) (string, string) {
	unaliased := types.Unalias(type_)
	if basic, ok := unaliased.(*types.Basic); ok {
		if basic.Kind() == types.UntypedNil {
			return "nil", "context.WithValue key is nil and will panic"
		}
		return "built-in", fmt.Sprintf(
			"context.WithValue key has built-in type %s and may collide across packages",
			types.TypeString(type_, types.RelativeTo(package_)),
		)
	}
	if structure, ok := unaliased.(*types.Struct); ok && structure.NumFields() == 0 {
		return "empty-struct", "context.WithValue key uses an anonymous empty struct and may collide across packages"
	}
	if typeParameter, ok := unaliased.(*types.TypeParam); ok {
		if !typeParameterAlwaysNonComparable(typeParameter) {
			return "", ""
		}
		return "not-comparable", fmt.Sprintf(
			"context.WithValue key has non-comparable type %s and will panic",
			types.TypeString(type_, types.RelativeTo(package_)),
		)
	}
	if !types.Comparable(type_) {
		return "not-comparable", fmt.Sprintf(
			"context.WithValue key has non-comparable type %s and will panic",
			types.TypeString(type_, types.RelativeTo(package_)),
		)
	}
	return "", ""
}

func typeParameterAlwaysNonComparable(typeParameter *types.TypeParam) bool {
	nonComparable, restricted, known := structuralNonComparability(
		typeParameter.Constraint(),
		make(map[types.Type]bool),
	)
	return known && restricted && nonComparable
}

func structuralNonComparability(
	type_ types.Type,
	visiting map[types.Type]bool,
) (nonComparable bool, restricted bool, known bool) {
	type_ = types.Unalias(type_)
	if visiting[type_] {
		return false, false, false
	}
	visiting[type_] = true
	defer delete(visiting, type_)

	switch underlying := type_.Underlying().(type) {
	case *types.Interface:
		if underlying.IsComparable() {
			return false, true, true
		}
		var result bool
		found := false
		for embedded := range underlying.EmbeddedTypes() {
			nonComparable, restricted, known := structuralNonComparability(
				embedded,
				visiting,
			)
			if !known {
				return false, false, false
			}
			if !restricted {
				continue
			}
			if found {
				return false, true, false
			}
			found = true
			result = nonComparable
		}
		return result, found, true
	case *types.Union:
		if underlying.Len() == 0 {
			return false, false, false
		}
		allNonComparable := true
		for term := range underlying.Terms() {
			nonComparable, restricted, known := structuralNonComparability(
				term.Type(),
				visiting,
			)
			if !known || !restricted {
				return false, false, false
			}
			allNonComparable = allNonComparable && nonComparable
		}
		return allNonComparable, true, true
	default:
		return !types.Comparable(type_), true, true
	}
}
