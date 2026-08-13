package rules

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"net/http"
)

type httpCanonicalHeaderKeyRule struct{}

// NewHTTPCanonicalHeaderKeyRule constructs the direct http.Header access rule
// for product registry composition.
func NewHTTPCanonicalHeaderKeyRule() Rule {
	return httpCanonicalHeaderKeyRule{}
}

func (httpCanonicalHeaderKeyRule) Metadata() Metadata {
	return Metadata{
		ID: "http-canonical-header-key",
		Summary: "detects noncanonical keys used in direct http.Header access",
		Documentation: "The methods on http.Header canonicalize header names, but direct map access does not. Reading or writing a constant noncanonical key can therefore miss an existing value or create a second entry whose spelling disagrees with values managed through Header.Get, Set, Add, or Del.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeIndexExpr},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Only compile-time string keys used through direct indexing of the standard library http.Header type are checked.",
			"Dynamic keys and values deliberately stored with noncanonical spelling require application-specific ownership knowledge and are not inferred.",
			"No fix is offered because changing a direct map key can change behavior when the map intentionally contains noncanonical entries.",
		},
		Examples: []Example{
			{
				Title: "Use canonical spelling for direct map access",
				Incorrect: `value := header["content-type"]`,
				Correct: `value := header["Content-Type"]`,
			},
		},
	}
}

func (httpCanonicalHeaderKeyRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	index, ok := node.(*ast.IndexExpr)
	if !ok {
		return nil, fmt.Errorf("http-canonical-header-key requires an index expression")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"http-canonical-header-key requires complete type information",
		)
	}
	if !isHTTPHeader(ctx.Info().TypeOf(index.X)) {
		return nil, nil
	}
	value := ctx.Info().Types[index.Index].Value
	if value == nil || value.Kind() != constant.String {
		return nil, nil
	}
	key := constant.StringVal(value)
	canonical := http.CanonicalHeaderKey(key)
	if canonical == key {
		return nil, nil
	}
	range_, err := ctx.Range(index.Index)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "noncanonical-header-key",
			Message: fmt.Sprintf(
				"direct http.Header access uses noncanonical key %q; canonical spelling is %q",
				key,
				canonical,
			),
			Range: range_,
			Help: "use the canonical key or an http.Header method that canonicalizes it",
		},
	}, nil
}

func isHTTPHeader(type_ types.Type) bool {
	named, _ := types.Unalias(type_).(*types.Named)
	return named != nil &&
		named.Obj() != nil &&
		named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "net/http" &&
		named.Obj().Name() == "Header"
}
