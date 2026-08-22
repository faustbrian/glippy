package rules

import (
	"fmt"
	"go/ast"
)

type busySelectLoopRule struct{}

// NewBusySelectLoopRule constructs the empty-default for-select rule for
// product registry composition.
func NewBusySelectLoopRule() Rule {
	return busySelectLoopRule{}
}

func (busySelectLoopRule) Metadata() Metadata {
	return Metadata{
		ID: "busy-select-loop",
		Summary: "detects conditionless for-select loops with an empty default",
		Documentation: "A conditionless for loop whose only statement is a select with an empty default never blocks when no communication is ready. It immediately begins the next iteration and can consume an entire CPU core while doing no work.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeForStmt},
		Categories: []Category{CategoryCorrectness, CategorySuspicious},
		KnownLimitations: []string{
			"Only a conditionless for loop whose body consists solely of the select statement is reported; loops with conditions, init or post statements, or surrounding work remain outside this exact busy-spin boundary.",
			"An intentionally busy-polling loop is valid Go and requires a narrow suppression.",
			"No fix is offered because removing the default changes blocking behavior and may also move or discard comments.",
			"Generated files are excluded.",
		},
		Examples: []Example{
			{
				Title: "Let the select block until communication is ready",
				Incorrect: `for {
	select {
	case value := <-updates:
		consume(value)
	default:
	}
}`,
				Correct: `for {
	select {
	case value := <-updates:
		consume(value)
	}
}`,
			},
		},
	}
}

func (busySelectLoopRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	loop, ok := node.(*ast.ForStmt)
	if !ok || ctx == nil {
		return nil, fmt.Errorf("busy-select-loop requires a for statement")
	}
	if loop.Init != nil ||
		loop.Cond != nil ||
		loop.Post != nil ||
		loop.Body == nil ||
		len(loop.Body.List) != 1 {
		return nil, nil
	}
	selection, ok := loop.Body.List[0].(*ast.SelectStmt)
	if !ok || selection.Body == nil {
		return nil, nil
	}
	for _, statement := range selection.Body.List {
		clause, ok := statement.(*ast.CommClause)
		if !ok || clause.Comm != nil || !emptySelectDefaultBody(clause.Body) {
			continue
		}
		range_, err := ctx.PositionRange(clause.Case, clause.Colon + 1)
		if err != nil {
			return nil, err
		}
		return []Finding{
			{
				MessageKey: "busy-select-loop",
				Message: "empty default in this for-select loop causes busy spinning",
				Range: range_,
				Help: "remove the empty default so the select blocks, or suppress an intentional busy-poll loop",
			},
		}, nil
	}
	return nil, nil
}

func emptySelectDefaultBody(statements []ast.Stmt) bool {
	for _, statement := range statements {
		if _, empty := statement.(*ast.EmptyStmt); !empty {
			return false
		}
	}
	return true
}
