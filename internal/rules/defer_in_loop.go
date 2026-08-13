package rules

import (
	"fmt"
	"go/ast"
	"go/token"
)

type deferInLoopRule struct{}

// NewDeferInLoopRule constructs the iteration-scoped defer rule for product
// registry composition.
func NewDeferInLoopRule() Rule {
	return deferInLoopRule{}
}

func (deferInLoopRule) Metadata() Metadata {
	return Metadata{
		ID: "defer-in-loop",
		Summary: "detects defers accumulated across loop iterations",
		Documentation: "A defer inside a finite or condition-controlled loop runs when the surrounding function returns, not when the current iteration ends. Repeated iterations can retain resources and defer cleanup much longer than intended. Conditionless loops remain covered by the more precise defer-in-infinite-loop control-flow rule.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetSuspicious},
		MinimumGoVersion: "1.25",
		Requirement: RequireSyntax,
		NodeInterests: []NodeKind{NodeForStmt, NodeRangeStmt},
		Categories: []Category{CategorySafety, CategorySuspicious},
		KnownLimitations: []string{
			"Conditionless for loops are delegated to defer-in-infinite-loop to avoid duplicate diagnostics.",
			"The rule does not infer a statically single-iteration loop or a deliberate bounded accumulation policy.",
			"Defers inside nested function literals are scoped to that function and are excluded.",
		},
		Examples: []Example{
			{
				Title: "Use an iteration helper for scoped cleanup",
				Incorrect: "for _, path := range paths { file := open(path); defer file.Close() }",
				Correct: "for _, path := range paths { process(path) }",
			},
		},
	}
}

func (deferInLoopRule) RunSyntax(ctx *Context, node ast.Node) ([]Finding, error) {
	if ctx == nil {
		return nil, fmt.Errorf("defer-in-loop requires a syntax context")
	}
	var body *ast.BlockStmt
	switch loop := node.(type) {
	case *ast.ForStmt:
		if loop.Cond == nil {
			return nil, nil
		}
		body = loop.Body
	case *ast.RangeStmt:
		body = loop.Body
	default:
		return nil, fmt.Errorf("defer-in-loop requires a for or range statement")
	}
	findings := make([]Finding, 0)
	var rangeErr error
	root := ast.Node(body)
	ast.Inspect(
		body,
		func(current ast.Node) bool {
			if rangeErr != nil {
				return false
			}
			if current == nil {
				return true
			}
			if current != root {
				switch current.(type) {
				case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
					return false
				}
			}
			statement, ok := current.(*ast.DeferStmt)
			if !ok {
				return true
			}
			range_, err := ctx.PositionRange(
				statement.Defer,
				statement.Defer + token.Pos(len("defer")),
			)
			if err != nil {
				rangeErr = err
				return false
			}
			findings = append(
				findings,
				Finding{
					MessageKey: "defer-in-loop",
					Message: "this defer runs when the surrounding function returns, not after the iteration",
					Range: range_,
					Help: "move one iteration into a helper function or perform cleanup explicitly",
				},
			)
			return true
		},
	)
	return findings, rangeErr
}
