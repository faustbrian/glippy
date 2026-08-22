package rules

import (
	"fmt"
	"go/ast"
	"go/types"
)

type deferredFunctionNotCalledRule struct{}

// NewDeferredFunctionNotCalledRule constructs the deferred returned-function
// rule for product registry composition.
func NewDeferredFunctionNotCalledRule() Rule {
	return deferredFunctionNotCalledRule{}
}

func (deferredFunctionNotCalledRule) Metadata() Metadata {
	return Metadata{
		ID: "deferred-function-not-called",
		Summary: "detects deferred calls whose returned function is never invoked",
		Documentation: "A common setup or tracing helper performs work immediately and returns a cleanup function. Writing defer setup() defers the setup call itself and discards the returned cleanup function when the surrounding function exits. The intended form is usually defer setup()().",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeDeferStmt},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Any function-valued result is reported, including named function types and functions requiring arguments; Glippy does not guess how the returned function should be invoked.",
			"Deferring a call for its own side effects while intentionally discarding a returned function is valid Go and requires a narrow suppression.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Invoke the cleanup function returned by the setup helper",
				Incorrect: "defer beginTrace()",
				Correct: "defer beginTrace()()",
			},
		},
	}
}

func (deferredFunctionNotCalledRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.DeferStmt)
	if !ok || ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"deferred-function-not-called requires a defer statement and type information",
		)
	}
	resultType := ctx.Info().TypeOf(statement.Call)
	if resultType == nil {
		return nil, nil
	}
	if _, ok := resultType.Underlying().(*types.Signature); !ok {
		return nil, nil
	}
	range_, err := ctx.Range(statement.Call)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "deferred-function-not-called",
			Message: "deferred call returns a function that is never invoked",
			Range: range_,
			Help: "invoke the returned function in the defer statement or suppress the intentional discard",
		},
	}, nil
}
