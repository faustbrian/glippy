package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type failedTypeAssertionValueRule struct{}

// NewFailedTypeAssertionValueRule constructs the failed type-assertion value
// rule for product registry composition.
func NewFailedTypeAssertionValueRule() Rule {
	return failedTypeAssertionValueRule{}
}

func (failedTypeAssertionValueRule) Metadata() Metadata {
	return Metadata{
		ID: "failed-type-assertion-value",
		Summary: "detects reads of a shadowed zero value after a failed type assertion",
		Documentation: "A short type assertion such as `if value, ok := value.(T); ok` shadows the original value throughout both branches. In the else branch the new value is the zero value of T, not the interface value that failed the assertion. Reading it can silently discard the original value or substitute nil, zero, or an empty value in error handling and fallback paths.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"Only short assertions of the exact form `if value, ok := value.(T); ok` are recognized; renamed results, assignments, type switches, and compound conditions remain conservative.",
			"Only else-branch reads that SSA proves still refer to the failed assertion result report; reassignment, address-taking, closure capture, and ambiguous joins are excluded.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Keep the original value available to the failure branch",
				Incorrect: "if value, ok := value.(string); ok {\n\treturn value\n} else {\n\treturn fmt.Sprintf(\"unexpected %T\", value)\n}",
				Correct: "if text, ok := value.(string); ok {\n\treturn text\n} else {\n\treturn fmt.Sprintf(\"unexpected %T\", value)\n}",
			},
		},
	}
}

func (failedTypeAssertionValueRule) RequiresSSADebug() {}

func (failedTypeAssertionValueRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil || ctx.Info() == nil {
		return nil, fmt.Errorf(
			"failed-type-assertion-value requires a complete SSA context",
		)
	}
	findings := make([]Finding, 0)
	var runErr error
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			if runErr != nil {
				return
			}
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return
			}
			var current []Finding
			current, runErr = failedTypeAssertionElseReads(ctx, statement)
			findings = append(findings, current...)
		},
	)
	if runErr != nil {
		return nil, runErr
	}
	return findings, nil
}

func failedTypeAssertionElseReads(ctx *SSAContext, statement *ast.IfStmt) ([]Finding, error) {
	assignment, _ := statement.Init.(*ast.AssignStmt)
	condition, _ := ast.Unparen(statement.Cond).(*ast.Ident)
	if assignment == nil ||
		assignment.Tok != token.DEFINE ||
		len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 ||
		condition == nil ||
		statement.Else == nil {
		return nil, nil
	}
	shadow, _ := assignment.Lhs[0].(*ast.Ident)
	okIdentifier, _ := assignment.Lhs[1].(*ast.Ident)
	assertion, _ := ast.Unparen(assignment.Rhs[0]).(*ast.TypeAssertExpr)
	if shadow == nil || okIdentifier == nil || assertion == nil {
		return nil, nil
	}
	asserted, _ := ast.Unparen(assertion.X).(*ast.Ident)
	if asserted == nil || shadow.Name != asserted.Name {
		return nil, nil
	}
	info := ctx.Info()
	shadowObject, _ := info.Defs[shadow].(*types.Var)
	okObject, _ := info.Defs[okIdentifier].(*types.Var)
	assertedObject := info.ObjectOf(asserted)
	if shadowObject == nil ||
		okObject == nil ||
		assertedObject == nil ||
		assertedObject == shadowObject ||
		info.ObjectOf(condition) != okObject {
		return nil, nil
	}
	assertionValue, _ := ctx.Function().ValueForExpr(assertion)
	shadowValue := failedTypeAssertionResult(assertionValue)
	if shadowValue == nil {
		return nil, nil
	}
	shadowRange, err := ctx.Range(shadow)
	if err != nil {
		return nil, err
	}
	assertedRange, err := ctx.Range(asserted)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0)
	var runErr error
	ast.Inspect(
		statement.Else,
		func(node ast.Node) bool {
			if node == nil || runErr != nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok || info.ObjectOf(identifier) != shadowObject {
				return true
			}
			value, isAddress := ctx.Function().ValueForExpr(identifier)
			if isAddress || value == nil || value != shadowValue {
				return true
			}
			range_, err := ctx.Range(identifier)
			if err != nil {
				runErr = err
				return false
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "failed-type-assertion-value",
					Message: fmt.Sprintf(
						"%s is the zero value from a failed type assertion, not the original value",
						identifier.Name,
					),
					Range: range_,
					Related: []Related{
						{
							Range: shadowRange,
							Message: "this assertion result shadows the original value",
						},
						{
							Range: assertedRange,
							Message: "this is the original value being asserted",
						},
					},
					Help: "rename the assertion result or read the original value in the else branch",
				},
			)
			return true
		},
	)
	if runErr != nil {
		return nil, runErr
	}
	return findings, nil
}

func failedTypeAssertionResult(tuple ssa.Value) ssa.Value {
	if tuple == nil || tuple.Referrers() == nil {
		return nil
	}
	for _, referrer := range *tuple.Referrers() {
		extract, ok := referrer.(*ssa.Extract)
		if ok && extract.Index == 0 {
			return extract
		}
	}
	return nil
}
