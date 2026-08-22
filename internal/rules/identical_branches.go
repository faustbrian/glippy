package rules

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"

	"github.com/faustbrian/glippy/internal/source"
)

type identicalBranchesRule struct{}

func (identicalBranchesRule) Metadata() Metadata {
	return Metadata{
		ID: "identical-branches",
		Summary: "detects identical if and else branches",
		Documentation: "An if statement whose two direct branches perform the same work usually contains a copied branch that was not updated or a condition that no longer selects behavior. The condition may still have effects, so the rule diagnoses the duplication without offering an automatic rewrite.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious, PresetPedantic},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeIfStmt},
		Categories: []Category{CategorySuspicious, CategoryMaintainability},
		KnownLimitations: []string{
			"Only direct if/else block pairs are compared; else-if chains are left to condition-specific rules.",
			"Statements that differ syntactically but are semantically equivalent are not compared.",
			"Commented statements are excluded because comments may document an intentional distinction between otherwise identical branches.",
		},
		Examples: []Example{
			{
				Title: "Keep distinct branch behavior",
				Incorrect: "if ready { run() } else { run() }",
				Correct: "if ready { run() } else { wait() }",
			},
		},
	}
}

func (identicalBranchesRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.IfStmt)
	if !ok {
		return nil, fmt.Errorf("identical-branches requires an if statement")
	}
	alternative, ok := statement.Else.(*ast.BlockStmt)
	if !ok {
		return nil, nil
	}
	statementRange, err := ctx.Range(statement)
	if err != nil {
		return nil, err
	}
	if rangeContainsComment(ctx.File().Comments(), statementRange) {
		return nil, nil
	}
	first, err := branchFingerprint(statement.Body)
	if err != nil {
		return nil, err
	}
	second, err := branchFingerprint(alternative)
	if err != nil {
		return nil, err
	}
	if first != second {
		return nil, nil
	}
	primary, err := ctx.Range(alternative)
	if err != nil {
		return nil, err
	}
	related, err := ctx.Range(statement.Body)
	if err != nil {
		return nil, err
	}
	return []Finding{
		{
			MessageKey: "identical-else-branch",
			Message: "this else branch is identical to the preceding if branch",
			Range: primary,
			Related: []Related{{Range: related, Message: "matching if branch"}},
			Help: "remove the unnecessary condition or restore the intended branch behavior",
		},
	}, nil
}

func branchFingerprint(block *ast.BlockStmt) (string, error) {
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), block); err != nil {
		return "", fmt.Errorf("render branch fingerprint: %w", err)
	}
	return output.String(), nil
}

func rangeContainsComment(comments []source.Comment, target source.Range) bool {
	for _, comment := range comments {
		if comment.Range.Start >= target.Start && comment.Range.End <= target.End {
			return true
		}
	}
	return false
}
