package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

const useNetIPEqualFix = "use-net-ip-equal"

type netIPBytesEqualRule struct{}

// NewNetIPBytesEqualRule constructs the net.IP comparison rule for product
// registry composition.
func NewNetIPBytesEqualRule() Rule {
	return netIPBytesEqualRule{}
}

func (netIPBytesEqualRule) Metadata() Metadata {
	return Metadata{
		ID: "net-ip-bytes-equal",
		Summary: "detects representation-sensitive net.IP comparisons",
		Documentation: "A net.IP value may represent the same IPv4 address with either 4 or 16 bytes. bytes.Equal compares only the underlying bytes, while net.IP.Equal accounts for both valid representations.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeCallExpr},
		Categories: []Category{CategoryCorrectness},
		Fixes: []FixMetadata{
			{
				Name: useNetIPEqualFix,
				Description: "compare addresses with net.IP.Equal",
				Safety: FixSuggestion,
			},
		},
		KnownLimitations: []string{
			"Only direct bytes.Equal calls whose two arguments have the exact net.IP type are recognized; function values remain conservative.",
			"Named byte-slice wrappers and interface values are excluded because net.IP semantics are not proven.",
			"The suggestion is withheld when replacing the call would remove a comment.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Compare IP addresses semantically",
				Incorrect: "same := bytes.Equal(first, second)",
				Correct: "same := first.Equal(second)",
			},
		},
	}
}

func (netIPBytesEqualRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	call, ok := node.(*ast.CallExpr)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"net-ip-bytes-equal requires a call expression and type information",
		)
	}
	if len(call.Args) != 2 {
		return nil, nil
	}
	function := directStandardFunction(ctx.Info(), call.Fun, "bytes")
	if function == nil ||
		function.Name() != "Equal" ||
		!isExactNetIP(ctx.Info().TypeOf(call.Args[0])) ||
		!isExactNetIP(ctx.Info().TypeOf(call.Args[1])) {
		return nil, nil
	}
	range_, err := ctx.Range(call)
	if err != nil {
		return nil, err
	}
	finding := Finding{
		MessageKey: "representation-sensitive-comparison",
		Message: "bytes.Equal does not account for equivalent net.IP representations",
		Range: range_,
		Help: "use net.IP.Equal to compare IP addresses",
	}
	firstRange, err := ctx.Range(call.Args[0])
	if err != nil {
		return nil, err
	}
	secondRange, err := ctx.Range(call.Args[1])
	if err != nil {
		return nil, err
	}
	if commentsOutsideRetainedRanges(ctx.File().Comments(), range_, firstRange, secondRange) {
		finding.WithheldFixes = []WithheldFix{
			{
				Name: useNetIPEqualFix,
				Reason: FixWithheldComments,
				Message: "replacing this comparison would remove comments",
			},
		}
		return []Finding{finding}, nil
	}
	firstSource, found := ctx.File().Slice(firstRange)
	if !found {
		return nil, fmt.Errorf("net.IP equality receiver has an invalid source range")
	}
	secondSource, found := ctx.File().Slice(secondRange)
	if !found {
		return nil, fmt.Errorf("net.IP equality argument has an invalid source range")
	}
	finding.Fixes = []Fix{
		{
			Name: useNetIPEqualFix,
			Safety: FixSuggestion,
			Edits: []Edit{
				{
					Range: range_,
					NewText: netIPReceiverSource(call.Args[0], firstSource) +
						".Equal(" +
						secondSource +
						")",
				},
			},
		},
	}
	return []Finding{finding}, nil
}

func netIPReceiverSource(expression ast.Expr, text string) string {
	switch ast.Unparen(expression).(type) {
	case *ast.Ident,
		*ast.SelectorExpr,
		*ast.IndexExpr,
		*ast.IndexListExpr,
		*ast.SliceExpr,
		*ast.TypeAssertExpr,
		*ast.CallExpr:
		return text
	default:
		return "(" + text + ")"
	}
}

func isExactNetIP(type_ types.Type) bool {
	if type_ == nil {
		return false
	}
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "net" &&
		named.Obj().Name() == "IP"
}
