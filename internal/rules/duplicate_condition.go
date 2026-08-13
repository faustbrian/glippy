package rules

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"

	"github.com/faustbrian/gox/internal/source"
)

type duplicateConditionRule struct{}

func (duplicateConditionRule) Metadata() Metadata {
	return Metadata{
		ID: "duplicate-condition",
		Summary: "detects repeated conditions in an if/else-if chain",
		Documentation: "Repeated side-effect-free conditions make a later branch unreachable and commonly indicate a copied condition that was not updated. Chains with initializers or conditions whose evaluation may have effects are ignored because changing those branches requires more context.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeIfStmt},
		Categories: []Category{CategoryCorrectness},
		KnownLimitations: []string{
			"Syntactically different expressions are not compared for semantic equivalence.",
			"Calls, channel receives, address operations, and chains with initializers are excluded conservatively.",
		},
		Examples: []Example{
			{
				Title: "Repeated branch condition",
				Incorrect: "if ready { use() } else if ready { retry() }",
				Correct: "if ready { use() } else if retryable { retry() }",
			},
		},
	}
}

func (duplicateConditionRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	statement, ok := node.(*ast.IfStmt)
	if !ok {
		return nil, fmt.Errorf("duplicate-condition requires an if statement")
	}
	if previous, found := ctx.PreviousSignificantToken(statement.If);
		found && previous.Kind == token.ELSE {
		return nil, nil
	}

	chain := make([]*ast.IfStmt, 0, 2)
	for current := statement; current != nil; {
		if current.Init != nil || !conditionIsSideEffectFree(current.Cond) {
			return nil, nil
		}
		chain = append(chain, current)
		next, _ := current.Else.(*ast.IfStmt)
		current = next
	}

	firstRanges := make(map[string]source.Range, len(chain))
	reported := make(map[string]struct{}, len(chain))
	findings := make([]Finding, 0)
	for _, current := range chain {
		fingerprint, err := conditionFingerprint(current.Cond)
		if err != nil {
			return nil, err
		}
		conditionRange, err := ctx.Range(current.Cond)
		if err != nil {
			return nil, err
		}
		firstRange, duplicate := firstRanges[fingerprint]
		if !duplicate {
			firstRanges[fingerprint] = conditionRange
			continue
		}
		if _, alreadyReported := reported[fingerprint]; alreadyReported {
			continue
		}
		reported[fingerprint] = struct{}{}
		findings = append(
			findings,
			Finding{
				MessageKey: "duplicate-condition",
				Message: "condition occurs more than once in this if/else-if chain",
				Range: conditionRange,
				Related: []Related{
					{
						Range: firstRange,
						Message: "first occurrence of this condition",
					},
				},
				Help: "change the repeated condition or remove the unreachable branch",
			},
		)
	}
	return findings, nil
}

func conditionIsSideEffectFree(expression ast.Expr) bool {
	safe := true
	ast.Inspect(
		expression,
		func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				safe = false
				return false
			case *ast.UnaryExpr:
				if current.Op == token.ARROW || current.Op == token.AND {
					safe = false
					return false
				}
			}
			return safe
		},
	)
	return safe
}

func conditionFingerprint(expression ast.Expr) (string, error) {
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), expression); err != nil {
		return "", fmt.Errorf("render condition fingerprint: %w", err)
	}
	return output.String(), nil
}
