package rules

import (
	"fmt"
	"go/ast"
	"go/token"
)

type ineffectiveBreakRule struct{}

func (ineffectiveBreakRule) Metadata() Metadata {
	return Metadata{
		ID:               "ineffective-break",
		Summary:          "detects unlabeled breaks that cannot exit the surrounding loop",
		Documentation:    "An unlabeled break at the end of a switch case or select clause exits only that switch or select. When the construct is directly inside a loop, the break has no control-flow effect and commonly means the author intended to leave the outer loop.",
		DefaultSeverity:  SeverityWarn,
		Presets:          []Preset{PresetCorrectness},
		MinimumGoVersion: "1.26",
		Requirement:      RequireSyntax,
		NodeInterests:    []NodeKind{NodeForStmt, NodeRangeStmt},
		Categories:       []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Only switches and selects that are direct statements of the loop body are inspected.",
			"A final if statement is inspected one level deep; more deeply nested terminal branches are not unfolded.",
			"Type switches are not inspected until their distinct syntax boundary has dedicated fixtures.",
		},
		Examples: []Example{{
			Title: "Break out of the surrounding loop",
			Incorrect: `for {
	select {
	case <-done:
		break
	}
}`,
			Correct: `loop:
for {
	select {
	case <-done:
		break loop
	}
}`,
		}},
	}
}

func (ineffectiveBreakRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	body, ok := loopBody(node)
	if !ok {
		return nil, fmt.Errorf("ineffective-break requires a for or range statement")
	}
	findings := make([]Finding, 0)
	for _, statement := range body.List {
		for _, clause := range switchOrSelectClauses(statement) {
			for _, branch := range ineffectiveTerminalBreaks(clause) {
				range_, err := ctx.Range(branch)
				if err != nil {
					return nil, err
				}
				findings = append(findings, Finding{
					MessageKey: "ineffective-break",
					Message:    "unlabeled break exits only the enclosing switch or select",
					Range:      range_,
					Help:       "label the surrounding loop and break to that label, or remove the ineffective break",
				})
			}
		}
	}
	return findings, nil
}

func loopBody(node ast.Node) (*ast.BlockStmt, bool) {
	switch statement := node.(type) {
	case *ast.ForStmt:
		return statement.Body, true
	case *ast.RangeStmt:
		return statement.Body, true
	default:
		return nil, false
	}
}

func switchOrSelectClauses(statement ast.Stmt) [][]ast.Stmt {
	clauses := make([][]ast.Stmt, 0)
	switch statement := statement.(type) {
	case *ast.SwitchStmt:
		for _, item := range statement.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if ok {
				clauses = append(clauses, clause.Body)
			}
		}
	case *ast.SelectStmt:
		for _, item := range statement.Body.List {
			clause, ok := item.(*ast.CommClause)
			if ok {
				clauses = append(clauses, clause.Body)
			}
		}
	}
	return clauses
}

func ineffectiveTerminalBreaks(statements []ast.Stmt) []*ast.BranchStmt {
	if len(statements) == 0 {
		return nil
	}
	last := statements[len(statements)-1]
	if branch, ineffective := ineffectiveBreak(last); ineffective {
		return []*ast.BranchStmt{branch}
	}
	conditional, ok := last.(*ast.IfStmt)
	if !ok {
		return nil
	}
	result := ineffectiveTerminalBreaks(conditional.Body.List)
	if alternative, ok := conditional.Else.(*ast.BlockStmt); ok {
		result = append(result, ineffectiveTerminalBreaks(alternative.List)...)
	}
	return result
}

func ineffectiveBreak(statement ast.Stmt) (*ast.BranchStmt, bool) {
	branch, ok := statement.(*ast.BranchStmt)
	return branch, ok && branch.Tok == token.BREAK && branch.Label == nil
}
