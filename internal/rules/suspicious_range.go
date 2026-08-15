package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

type suspiciousRangeRule struct{}

// NewSuspiciousRangeRule constructs the copied-range-value mutation rule for
// product registry composition.
func NewSuspiciousRangeRule() Rule {
	return suspiciousRangeRule{}
}

func (suspiciousRangeRule) Metadata() Metadata {
	return Metadata{
		ID: "suspicious-range",
		Summary: "detects mutations made only to a copied range value",
		Documentation: "Range values are copies. Mutating a field or element reached only through a non-pointer struct or array range variable does not update the slice, array, or map element that produced it and is usually an ineffective update.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeRangeStmt},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"The rule reports assignments and increments rooted in the exact range value object and ignores nested function literals.",
			"Paths that cross a pointer, slice, map, interface, or channel are excluded because mutation can reach shared state.",
			"A mutation followed by any later use of the range value is not reported because it can be intentional local computation, projection, or write-back.",
		},
		Examples: []Example{
			{
				Title: "Mutate a slice element through its index",
				Incorrect: "for _, value := range values { value.ready = true }",
				Correct: "for index := range values { values[index].ready = true }",
			},
		},
	}
}

func (suspiciousRangeRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	loop, ok := node.(*ast.RangeStmt)
	if !ok {
		return nil, fmt.Errorf("suspicious-range requires a range statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("suspicious-range requires complete type information")
	}
	identifier, _ := loop.Value.(*ast.Ident)
	if identifier == nil || identifier.Name == "_" {
		return nil, nil
	}
	object := ctx.Info().ObjectOf(identifier)
	if object == nil || !copiedAggregateType(object.Type()) {
		return nil, nil
	}
	findings := make([]Finding, 0)
	var rangeErr error
	ast.Inspect(
		loop.Body,
		func(current ast.Node) bool {
			if rangeErr != nil {
				return false
			}
			if current == nil {
				return true
			}
			if _, nested := current.(*ast.FuncLit); nested {
				return false
			}
			var targets []ast.Expr
			var mutationEnd token.Pos
			switch statement := current.(type) {
			case *ast.AssignStmt:
				targets = statement.Lhs
				mutationEnd = statement.End()
			case *ast.IncDecStmt:
				targets = []ast.Expr{statement.X}
				mutationEnd = statement.End()
			default:
				return true
			}
			for _, target := range targets {
				if !mutationStaysOnRangeCopy(ctx.Info(), target, object) {
					continue
				}
				if rangeValueUsedAfter(ctx.Info(), loop.Body, object, mutationEnd) {
					continue
				}
				range_, err := ctx.Range(target)
				if err != nil {
					rangeErr = err
					return false
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "range-value-copy-mutation",
						Message: "this mutation changes only the range value copy",
						Range: range_,
						Help: "range over indexes or store the modified value back into the collection",
					},
				)
			}
			return true
		},
	)
	return findings, rangeErr
}

func rangeValueUsedAfter(
	info *types.Info,
	body *ast.BlockStmt,
	object types.Object,
	position token.Pos,
) bool {
	if info == nil || body == nil || object == nil || !position.IsValid() {
		return false
	}
	for identifier, used := range info.Uses {
		if used == object && identifier.Pos() > position && identifier.Pos() < body.End() {
			return true
		}
	}
	return false
}

func copiedAggregateType(type_ types.Type) bool {
	switch types.Unalias(type_).Underlying().(type) {
	case *types.Struct, *types.Array:
		return true
	default:
		return false
	}
}

func mutationStaysOnRangeCopy(info *types.Info, expression ast.Expr, object types.Object) bool {
	expression = ast.Unparen(expression)
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		return copyMutationPath(info, expression.X, object)
	case *ast.IndexExpr:
		return copyMutationPath(info, expression.X, object)
	default:
		return false
	}
}

func copyMutationPath(info *types.Info, expression ast.Expr, object types.Object) bool {
	expression = ast.Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		return info.ObjectOf(identifier) == object
	}
	switch types.Unalias(info.TypeOf(expression)).Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Chan:
		return false
	}
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		return copyMutationPath(info, expression.X, object)
	case *ast.IndexExpr:
		return copyMutationPath(info, expression.X, object)
	default:
		return false
	}
}
