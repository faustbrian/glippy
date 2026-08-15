package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type overwrittenErrorRule struct{}

// NewOverwrittenErrorRule constructs the unread error-value rule for product
// registry composition.
func NewOverwrittenErrorRule() Rule {
	return overwrittenErrorRule{}
}

func (overwrittenErrorRule) Metadata() Metadata {
	return Metadata{
		ID: "overwritten-error",
		Summary: "detects error values overwritten before they are observed",
		Documentation: "An error overwritten before it is checked, returned, logged, or explicitly discarded loses the failure from the earlier operation. The rule follows SSA values through branch joins and tuple extraction, then reports only result components assignable to Go's built-in error interface.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSSA,
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Only assignments and initialized variable declarations with direct identifier destinations are considered; fields, indexes, dereferences, range variables, and incoming parameter values are excluded.",
			"An explicitly blank assignment or variable declaration such as _ = err counts as intentional observation and is not reported.",
			"Address-taken values, including some captured and named-result variables, may be represented through memory and are conservatively excluded when SSA cannot identify the assigned value precisely.",
			"Generated files and packages with type errors are excluded.",
		},
		Examples: []Example{
			{
				Title: "Check each operation's error before reusing it",
				Incorrect: "value, err := first()\nvalue, err = second()\nif err != nil { return err }",
				Correct: "value, err := first()\nif err != nil { return err }\nvalue, err = second()",
			},
		},
	}
}

func (overwrittenErrorRule) RequiresSSADebug() {}

func (overwrittenErrorRule) RunSSA(ctx *SSAContext) ([]Finding, error) {
	if ctx == nil ||
		ctx.Function() == nil ||
		ctx.Syntax() == nil ||
		ctx.Info() == nil ||
		ctx.Package() == nil {
		return nil, fmt.Errorf("overwritten-error requires a complete SSA context")
	}
	errorObject := types.Universe.Lookup("error")
	if errorObject == nil {
		return nil, fmt.Errorf("overwritten-error cannot resolve the built-in error type")
	}
	errorType := errorObject.Type()
	expressionValues := overwrittenErrorExpressionValues(ctx.Function())
	explicitUses := overwrittenErrorExplicitUses(ctx, expressionValues)
	switchUses := overwrittenErrorSwitchUses(ctx, expressionValues)
	lastDefinitions := overwrittenErrorLastDefinitions(ctx)
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
				assigned, runErr = overwrittenErrorAssignments(
					ctx,
					expressionValues,
					node.Lhs,
					node.Rhs,
					errorType,
					explicitUses,
					switchUses,
					lastDefinitions,
				)
				findings = append(findings, assigned...)
			case *ast.ValueSpec:
				lhs := make([]ast.Expr, len(node.Names))
				for index := range node.Names {
					lhs[index] = node.Names[index]
				}
				var assigned []Finding
				assigned, runErr = overwrittenErrorAssignments(
					ctx,
					expressionValues,
					lhs,
					node.Values,
					errorType,
					explicitUses,
					switchUses,
					lastDefinitions,
				)
				findings = append(findings, assigned...)
			}
		},
	)
	if runErr != nil {
		return nil, runErr
	}
	return findings, nil
}

func overwrittenErrorAssignments(
	ctx *SSAContext,
	expressionValues map[ast.Expr]ssa.Value,
	lhs []ast.Expr,
	rhs []ast.Expr,
	errorType types.Type,
	explicitUses map[ssa.Value]struct{},
	switchUses map[ssa.Value]struct{},
	lastDefinitions map[types.Object]token.Pos,
) ([]Finding, error) {
	if len(lhs) == 0 || len(rhs) == 0 {
		return nil, nil
	}
	values := make([]ssa.Value, len(lhs))
	if len(rhs) == 1 && len(lhs) > 1 {
		tuple := expressionValues[ast.Unparen(rhs[0])]
		if tuple == nil || tuple.Referrers() == nil {
			return nil, nil
		}
		for _, referrer := range *tuple.Referrers() {
			extract, ok := referrer.(*ssa.Extract)
			if ok && extract.Index >= 0 && extract.Index < len(values) {
				values[extract.Index] = extract
			}
		}
	} else if len(lhs) == len(rhs) {
		for index := range rhs {
			values[index] = expressionValues[ast.Unparen(rhs[index])]
		}
	} else {
		return nil, nil
	}
	findings := make([]Finding, 0)
	for index, destination := range lhs {
		identifier, ok := destination.(*ast.Ident)
		if !ok || identifier.Name == "_" {
			continue
		}
		object := ctx.Info().ObjectOf(identifier)
		if object == nil ||
			lastDefinitions[object] <= identifier.Pos() ||
			values[index] == nil ||
			!types.AssignableTo(values[index].Type(), errorType) ||
			overwrittenErrorValueUsed(values[index], explicitUses, switchUses, nil) {
			continue
		}
		range_, err := ctx.Range(identifier)
		if err != nil {
			return nil, err
		}
		findings = append(
			findings,
			Finding{
				MessageKey: "overwritten-error",
				Message: fmt.Sprintf(
					"this value of %s is overwritten before its error is observed",
					identifier.Name,
				),
				Range: range_,
				Help: "check, return, log, or explicitly discard the error before assigning another value",
			},
		)
	}
	return findings, nil
}

func overwrittenErrorExpressionValues(function *ssa.Function) map[ast.Expr]ssa.Value {
	values := make(map[ast.Expr]ssa.Value)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			reference, ok := instruction.(*ssa.DebugRef)
			if !ok || reference.IsAddr || reference.Expr == nil || reference.X == nil {
				continue
			}
			expression := ast.Unparen(reference.Expr)
			if _, found := values[expression]; !found {
				values[expression] = reference.X
			}
		}
	}
	return values
}

func overwrittenErrorLastDefinitions(ctx *SSAContext) map[types.Object]token.Pos {
	definitions := make(map[types.Object]token.Pos)
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			var identifiers []*ast.Ident
			switch node := node.(type) {
			case *ast.AssignStmt:
				for _, destination := range node.Lhs {
					identifier, ok := destination.(*ast.Ident)
					if ok {
						identifiers = append(identifiers, identifier)
					}
				}
			case *ast.ValueSpec:
				identifiers = append(identifiers, node.Names...)
			}
			for _, identifier := range identifiers {
				if identifier.Name == "_" {
					continue
				}
				object := ctx.Info().ObjectOf(identifier)
				if object != nil && identifier.Pos() > definitions[object] {
					definitions[object] = identifier.Pos()
				}
			}
		},
	)
	return definitions
}

func overwrittenErrorValueUsed(
	value ssa.Value,
	explicitUses map[ssa.Value]struct{},
	switchUses map[ssa.Value]struct{},
	seen map[ssa.Value]struct{},
) bool {
	if _, found := explicitUses[value]; found {
		return true
	}
	if _, found := switchUses[value]; found {
		return true
	}
	if seen != nil {
		if _, found := seen[value]; found {
			return false
		}
	}
	referrers := value.Referrers()
	if referrers == nil {
		return true
	}
	for _, referrer := range *referrers {
		switch referrer := referrer.(type) {
		case *ssa.DebugRef:
		case *ssa.Phi, *ssa.MakeInterface, *ssa.ChangeInterface:
			forwarded := referrer.(ssa.Value)
			if seen == nil {
				seen = make(map[ssa.Value]struct{})
			}
			seen[value] = struct{}{}
			if overwrittenErrorValueUsed(forwarded, explicitUses, switchUses, seen) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func overwrittenErrorExplicitUses(
	ctx *SSAContext,
	expressionValues map[ast.Expr]ssa.Value,
) map[ssa.Value]struct{} {
	uses := make(map[ssa.Value]struct{})
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			var lhs []ast.Expr
			var rhs []ast.Expr
			switch node := node.(type) {
			case *ast.AssignStmt:
				lhs = node.Lhs
				rhs = node.Rhs
			case *ast.ValueSpec:
				lhs = make([]ast.Expr, len(node.Names))
				for index := range node.Names {
					lhs[index] = node.Names[index]
				}
				rhs = node.Values
			}
			if len(lhs) == 0 || len(lhs) != len(rhs) {
				return
			}
			for index, destination := range lhs {
				identifier, blank := destination.(*ast.Ident)
				if !blank || identifier.Name != "_" {
					continue
				}
				value := expressionValues[ast.Unparen(rhs[index])]
				if value != nil {
					uses[value] = struct{}{}
				}
			}
		},
	)
	return uses
}

func overwrittenErrorSwitchUses(
	ctx *SSAContext,
	expressionValues map[ast.Expr]ssa.Value,
) map[ssa.Value]struct{} {
	uses := make(map[ssa.Value]struct{})
	inspectOwnedFunction(
		ctx.Syntax(),
		func(node ast.Node) {
			switchStatement, ok := node.(*ast.SwitchStmt)
			if !ok || switchStatement.Tag == nil {
				return
			}
			value := expressionValues[ast.Unparen(switchStatement.Tag)]
			if value != nil {
				uses[value] = struct{}{}
			}
		},
	)
	return uses
}

func inspectOwnedFunction(root ast.Node, visit func(ast.Node)) {
	ast.Inspect(
		root,
		func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if function, nested := node.(*ast.FuncLit); nested && function != root {
				return false
			}
			visit(node)
			return true
		},
	)
}
