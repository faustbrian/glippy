package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type nonSliceSortRule struct{}

// NewNonSliceSortRule constructs the sort slice-argument rule for product
// registry composition.
func NewNonSliceSortRule() Rule {
	return nonSliceSortRule{}
}

func (nonSliceSortRule) Metadata() Metadata {
	return Metadata{
		ID: "non-slice-sort",
		Summary: "detects non-slice values passed to sort slice APIs",
		Documentation: "sort.Slice, sort.SliceStable, and sort.SliceIsSorted accept any value at compile time but panic when its dynamic value is not a slice. Statically non-slice arguments can therefore be rejected before execution.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only direct calls to sort.Slice, sort.SliceStable, and sort.SliceIsSorted are recognized; function values remain conservative.",
			"Interface and type-parameter arguments remain conservative because their runtime value may be a slice.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Pass the slice rather than its pointer",
				Incorrect: "sort.Slice(&values, less)",
				Correct: "sort.Slice(values, less)",
			},
		},
	}
}

func (nonSliceSortRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"non-slice-sort requires a call expression and type information",
		)
	}
	function := directStandardFunction(ctx.Info(), call.Fun, "sort")
	if function == nil ||
		(function.Name() != "Slice" &&
			function.Name() != "SliceStable" &&
			function.Name() != "SliceIsSorted") ||
		len(call.Args) == 0 {
		return nil, nil
	}
	argumentType := ctx.Info().TypeOf(call.Args[0])
	if argumentType == nil {
		return nil, nil
	}
	unaliased := types.Unalias(argumentType)
	if _, unknown := unaliased.(*types.TypeParam); unknown {
		return nil, nil
	}
	switch unaliased.Underlying().(type) {
	case *types.Slice, *types.Interface:
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "non-slice-argument",
			Message: "sort requires a slice argument",
			Range: range_,
			Help: "pass a slice value directly",
		},
	}, nil
}
