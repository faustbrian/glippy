package rules

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/faustbrian/glippy/internal/source"
)

type redundantElseRule struct{}

// NewRedundantElseRule constructs the terminating-branch structure rule for
// product registry composition.
func NewRedundantElseRule() Rule {
	return redundantElseRule{}
}

func (redundantElseRule) Metadata() Metadata {
	return Metadata{
		ID: "redundant-else",
		Summary: "detects else blocks after a branch that always terminates",
		Documentation: "An else block is structurally unnecessary when the matching if branch always returns or transfers control. Moving the alternative after the if reduces nesting while preserving which path executes.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeIfStmt},
		Categories: []Category{CategoryComplexity, CategoryStyle},
		KnownLimitations: []string{
			"If statements with initializers are excluded because the else branch may depend on initializer scope.",
			"Termination is proven only from return, branch statements, nested blocks, or if statements whose two branches terminate.",
			"No fix is offered because removing else braces must preserve comments and surrounding statement ownership.",
		},
		Examples: []Example{
			{
				Title: "Continue after the terminating branch",
				Incorrect: "if err != nil { return err } else { use(value) }",
				Correct: "if err != nil { return err }\nuse(value)",
			},
		},
	}
}

func (redundantElseRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.IfStmt)
	if !ok {
		return nil, fmt.Errorf("redundant-else requires an if statement")
	}
	if statement.Init != nil || statement.Else == nil || !blockTerminates(statement.Body) {
		return nil, nil
	}
	alternative, directBlock := statement.Else.(*ast.BlockStmt)
	if !directBlock {
		return nil, nil
	}
	range_, err := elseKeywordRange(ctx, statement.Body, alternative)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "terminating-if-branch",
			Message: "else is unnecessary because the if branch always terminates",
			Range: range_,
			Help: "move the else body after the if statement",
		},
	}, nil
}

func elseKeywordRange(
	ctx *Context,
	body *ast.BlockStmt,
	alternative *ast.BlockStmt,
) (source.Range, error) {
	bodyRange, err := ctx.Range(body)
	if err != nil {
		return source.Range{}, err
	}
	alternativeRange, err := ctx.Range(alternative)
	if err != nil {
		return source.Range{}, err
	}
	for _, item := range ctx.File().Tokens() {
		if item.Kind == token.ELSE &&
			item.Range.Start >= bodyRange.End &&
			item.Range.End <= alternativeRange.Start {
			return item.Range, nil
		}
	}
	return source.Range{}, fmt.Errorf("redundant-else could not locate the else keyword")
}

func blockTerminates(block *ast.BlockStmt) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	return statementTerminates(block.List[len(block.List) - 1])
}

func statementTerminates(statement ast.Stmt) bool {
	switch current := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return current.Tok == token.BREAK ||
			current.Tok == token.CONTINUE ||
			current.Tok == token.GOTO
	case *ast.BlockStmt:
		return blockTerminates(current)
	case *ast.IfStmt:
		alternative, _ := current.Else.(*ast.BlockStmt)
		return alternative != nil &&
			blockTerminates(current.Body) &&
			blockTerminates(alternative)
	default:
		return false
	}
}
