package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

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
		KnownLimitations: []string{
			"Only direct bytes.Equal calls whose two arguments have the exact net.IP type are recognized; function values remain conservative.",
			"Named byte-slice wrappers and interface values are excluded because net.IP semantics are not proven.",
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
	return []Finding{
		{
			MessageKey: "representation-sensitive-comparison",
			Message: "bytes.Equal does not account for equivalent net.IP representations",
			Range: range_,
			Help: "use net.IP.Equal to compare IP addresses",
		},
	}, nil
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
