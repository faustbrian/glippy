package rules

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

type ineffectiveAssignmentRule struct{}

// NewIneffectiveAssignmentRule constructs the unread assigned-value rule for
// product registry composition.
func NewIneffectiveAssignmentRule() Rule {
	return ineffectiveAssignmentRule{}
}

func (ineffectiveAssignmentRule) Metadata() Metadata {
	return Metadata{
		ID:               "ineffective-assignment",
		Summary:          "detects assigned values that are never read",
		Documentation:    "An assigned value that is never read can indicate a forgotten consumer, a stale computation, or an ineffective state update. The rule preserves right-hand-side effects, follows SSA values through branch joins, and reports only direct identifier destinations whose assigned value has no observable use.",
		DefaultSeverity:  SeverityWarn,
		Presets:          []Preset{PresetNursery},
		MinimumGoVersion: "1.25",
		Requirement:      RequireSSA,
		Categories:       []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Only assignments to direct identifiers are considered; fields, indexes, dereferences, range variables, standalone var declarations, and incoming parameter values are excluded.",
			"Constant SSA values are excluded because the compiler and SSA builder can merge them independently of the source assignment.",
			"Address-taken values may be represented through memory and are conservatively excluded when SSA cannot identify the assigned value precisely.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title:     "Consume the value produced by an assignment",
				Incorrect: "name := input\nuse(name)\nname = normalize(input)\nreturn input",
				Correct:   "name := input\nuse(name)\nname = normalize(input)\nreturn name",
			},
		},
	}
}

func (ineffectiveAssignmentRule) RequiresSSADebug() {}

func (ineffectiveAssignmentRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil || ctx.Function() == nil || ctx.Syntax() == nil {
		return nil, fmt.Errorf("ineffective-assignment requires a complete SSA context")
	}
	expressionValues := overwrittenErrorExpressionValues(ctx.Function())
	expressionValueSets := ineffectiveAssignmentExpressionValues(ctx.Function())
	explicitUses := overwrittenErrorExplicitUses(ctx, expressionValues)
	switchUses := overwrittenErrorSwitchUses(ctx, expressionValues)
	findings := make([]Finding, 0)
	var runErr error
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			if runErr != nil {
				return
			}
			switch node := node.(type) {
			case *ast.AssignStmt:
				var assigned []Finding
				assigned, runErr = ineffectiveAssignmentFindings(
					ctx,
					expressionValues,
					expressionValueSets,
					explicitUses,
					switchUses,
					node,
				)
				findings = append(findings, assigned...)
			case *ast.IncDecStmt:
				var assigned *Finding
				assigned, runErr = ineffectiveIncrementFinding(
					ctx,
					expressionValueSets,
					explicitUses,
					switchUses,
					node,
				)
				if assigned != nil {
					findings = append(findings, *assigned)
				}
			}
		},
	)
	if runErr != nil {
		return nil, runErr
	}
	return findings, nil
}

func ineffectiveAssignmentFindings(
	ctx *SSAContext,
	expressionValues map[ast.Expr]ssa.Value,
	expressionValueSets map[ast.Expr][]ssa.Value,
	explicitUses map[ssa.Value]struct{},
	switchUses map[ssa.Value]struct{},
	assignment *ast.AssignStmt,
) ([]Finding, error) {
	values := ineffectiveAssignedValues(expressionValues, assignment)
	findings := make([]Finding, 0)
	for index, destination := range assignment.Lhs {
		identifier, ok := destination.(*ast.Ident)
		if !ok || identifier.Name == "_" || index >= len(values) {
			continue
		}
		value := values[index]
		if assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE {
			value = ineffectiveUnreadValue(
				expressionValueSets[ast.Unparen(destination)],
				explicitUses,
				switchUses,
			)
		}
		if value == nil {
			continue
		}
		if _, constant := value.(*ssa.Const); constant ||
			overwrittenErrorValueUsed(value, explicitUses, switchUses, nil) {
			continue
		}
		finding, err := ineffectiveAssignmentFinding(ctx, identifier)
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func ineffectiveAssignedValues(
	expressionValues map[ast.Expr]ssa.Value,
	assignment *ast.AssignStmt,
) []ssa.Value {
	values := make([]ssa.Value, len(assignment.Lhs))
	if len(assignment.Rhs) == 1 && len(assignment.Lhs) > 1 {
		tuple := expressionValues[ast.Unparen(assignment.Rhs[0])]
		if tuple == nil || tuple.Referrers() == nil {
			return values
		}
		for _, referrer := range *tuple.Referrers() {
			extract, ok := referrer.(*ssa.Extract)
			if ok && extract.Index >= 0 && extract.Index < len(values) {
				values[extract.Index] = extract
			}
		}
		return values
	}
	if len(assignment.Lhs) != len(assignment.Rhs) {
		return values
	}
	for index := range assignment.Rhs {
		values[index] = expressionValues[ast.Unparen(assignment.Rhs[index])]
		if values[index] == nil && assignment.Tok != token.ASSIGN {
			values[index] = expressionValues[ast.Unparen(assignment.Lhs[index])]
		}
	}
	return values
}

func ineffectiveIncrementFinding(
	ctx *SSAContext,
	expressionValueSets map[ast.Expr][]ssa.Value,
	explicitUses map[ssa.Value]struct{},
	switchUses map[ssa.Value]struct{},
	increment *ast.IncDecStmt,
) (*Finding, error) {
	identifier, ok := increment.X.(*ast.Ident)
	if !ok {
		return nil, nil
	}
	value := ineffectiveUnreadValue(
		expressionValueSets[ast.Unparen(increment.X)],
		explicitUses,
		switchUses,
	)
	if value == nil {
		return nil, nil
	}
	finding, err := ineffectiveAssignmentFinding(ctx, identifier)
	if err != nil {
		return nil, err
	}
	return &finding, nil
}

func ineffectiveAssignmentExpressionValues(function *ssa.Function) map[ast.Expr][]ssa.Value {
	values := make(map[ast.Expr][]ssa.Value)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			reference, ok := instruction.(*ssa.DebugRef)
			if !ok || reference.IsAddr || reference.Expr == nil || reference.X == nil {
				continue
			}
			expression := ast.Unparen(reference.Expr)
			values[expression] = append(values[expression], reference.X)
		}
	}
	return values
}

func ineffectiveUnreadValue(
	values []ssa.Value,
	explicitUses map[ssa.Value]struct{},
	switchUses map[ssa.Value]struct{},
) ssa.Value {
	for _, value := range values {
		if _, constant := value.(*ssa.Const); constant ||
			overwrittenErrorValueUsed(value, explicitUses, switchUses, nil) {
			continue
		}
		return value
	}
	return nil
}

func ineffectiveAssignmentFinding(ctx *SSAContext, identifier *ast.Ident) (Finding, error) {
	range_, err := ctx.Range(identifier)
	if err != nil {
		return Finding{}, err
	}
	return Finding{
		MessageKey: "ineffective-assignment",
		Message:    fmt.Sprintf("this value of %s is never used", identifier.Name),
		Range:      range_,
		Help:       "use the assigned value or remove the ineffective destination while preserving right-hand-side effects",
	}, nil
}
