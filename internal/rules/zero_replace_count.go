package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
)

type zeroReplaceCountRule struct{}

// NewZeroReplaceCountRule constructs the standard replacement-count rule for
// product registry composition.
func NewZeroReplaceCountRule() Rule {
	return zeroReplaceCountRule{}
}

func (zeroReplaceCountRule) Metadata() Metadata {
	return Metadata{
		ID: "zero-replace-count",
		Summary: "detects replacement calls whose zero count replaces nothing",
		Documentation: "strings.Replace and bytes.Replace interpret their final argument as the maximum number of replacements. A compile-time zero requests no replacements, so the call cannot perform the apparent substitution.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only direct calls to exact strings.Replace and bytes.Replace package functions are recognized; function values remain conservative.",
			"Only compile-time integer zero counts are reported; value flow through variables is not inferred.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Request the intended number of replacements",
				Incorrect: `path = strings.Replace(path, "\\", "/", 0)`,
				Correct: `path = strings.ReplaceAll(path, "\\", "/")`,
			},
		},
	}
}

func (zeroReplaceCountRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"zero-replace-count requires a call expression and type information",
		)
	}
	if len(call.Args) != 4 || !isStandardReplaceCall(ctx.Info(), call) {
		return nil, nil
	}
	count := ast.Unparen(call.Args[3])
	value := ctx.Info().Types[count].Value
	if value == nil || value.Kind() != constant.Int || constant.Sign(value) != 0 {
		return nil, nil
	}
	range_, err := ctx.Range(call.Args[3])
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "zero-count",
			Message: "replacement count zero replaces no occurrences",
			Range: range_,
			Help: "pass a positive count, a negative count for all occurrences, or use ReplaceAll",
		},
	}, nil
}

func isStandardReplaceCall(info *types.Info, call *ast.CallExpr) bool {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return false
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	if function == nil || function.Pkg() == nil || function.Name() != "Replace" {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() != nil {
		return false
	}
	return function.Pkg().Path() == "strings" || function.Pkg().Path() == "bytes"
}
