package rules

import (
	"fmt"
	"go/ast"
)

type deprecatedIOUtilRule struct{}

// NewDeprecatedIOUtilRule constructs the io/ioutil migration rule.
func NewDeprecatedIOUtilRule() Rule {
	return deprecatedIOUtilRule{}
}

func (deprecatedIOUtilRule) Metadata() Metadata {
	return Metadata{
		ID: "deprecated-ioutil",
		Summary: "detects uses of deprecated io/ioutil APIs",
		Documentation: "The io/ioutil package was deprecated in Go 1.16 after its APIs moved to io and os. Keeping the old import obscures the supported API surface and delays straightforward toolchain migrations. This migration rule reports exact typed io/ioutil references without guessing import edits.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetMigration},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeFile},
		Categories: []Category{CategoryMigration, CategoryMaintainability},
		KnownLimitations: []string{
			"Only exact objects imported from io/ioutil are reported; references copied through variables or wrappers are not followed.",
			"No fix is offered because replacing the selector also requires coordinated import ownership and may conflict with an existing io or os import alias.",
			"The migration group requires explicit targeting; the rule can also be enabled by exact ID.",
		},
		Examples: []Example{
			{
				Title: "Use the current standard-library API",
				Incorrect: "data, err := ioutil.ReadFile(path)",
				Correct: "data, err := os.ReadFile(path)",
			},
		},
	}
}

func (deprecatedIOUtilRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	file, ok := node.(*ast.File)
	if !ok {
		return nil, fmt.Errorf("deprecated-ioutil requires a file")
	}
	if ctx == nil || ctx.Info() == nil || ctx.Syntax() != file {
		return nil, fmt.Errorf("deprecated-ioutil requires complete type information")
	}
	selectors := make(map[*ast.Ident]*ast.SelectorExpr)
	ast.Inspect(
		file,
		func(current ast.Node) bool {
			selector, selectorNode := current.(*ast.SelectorExpr)
			if selectorNode && selector.Sel != nil {
				selectors[selector.Sel] = selector
			}
			return true
		},
	)
	findings := make([]Finding, 0)
	var runErr error
	ast.Inspect(
		file,
		func(current ast.Node) bool {
			if runErr != nil {
				return false
			}
			identifier, identifierNode := current.(*ast.Ident)
			if !identifierNode {
				return true
			}
			object := ctx.Info().Uses[identifier]
			if object == nil ||
				object.Pkg() == nil ||
				object.Pkg().Path() != "io/ioutil" ||
				!deprecatedIOUtilName(object.Name()) {
				return true
			}
			var target ast.Node = identifier
			if selector := selectors[identifier]; selector != nil {
				target = selector
			}
			range_, err := ctx.Range(target)
			if err != nil {
				runErr = err
				return false
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "deprecated-ioutil-api",
					Message: "io/ioutil is deprecated; use the corresponding io or os API",
					Range: range_,
					Help: "replace this API and its import with the io or os equivalent",
				},
			)
			return true
		},
	)
	return findings, runErr
}

func deprecatedIOUtilName(name string) bool {
	switch name {
	case "Discard",
		"NopCloser",
		"ReadAll",
		"ReadDir",
		"ReadFile",
		"TempDir",
		"TempFile",
		"WriteFile":
		return true
	default:
		return false
	}
}
