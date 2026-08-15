package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

type execPipeRunRule struct{}

// NewExecPipeRunRule constructs the os/exec output-pipe Run misuse rule.
func NewExecPipeRunRule() Rule {
	return execPipeRunRule{}
}

func (execPipeRunRule) Metadata() Metadata {
	return Metadata{
		ID: "exec-pipe-run",
		Summary: "detects Cmd.Run after StdoutPipe or StderrPipe",
		Documentation: "The os/exec contract requires callers of Cmd.StdoutPipe and Cmd.StderrPipe to start the command, finish reading the pipe, and then call Wait. Cmd.Run combines Start and Wait, so it may close the pipe before the caller finishes reading and is explicitly documented as incorrect with these output-pipe methods.",
		DefaultSeverity: SeverityWarn,
		Presets: []Preset{PresetCorrectness},
		MinimumGoVersion: "1.25",
		Requirement: RequireTypes,
		NodeInterests: []NodeKind{NodeBlockStmt},
		Categories: []Category{CategoryCorrectness, CategorySafety},
		KnownLimitations: []string{
			"The rule tracks one direct local Cmd identifier within a lexical block; fields, aliases, helper calls, and cross-block ownership require deeper value flow.",
			"Reassigning the tracked Cmd variable clears the local pipe state conservatively.",
		},
		Examples: []Example{
			{
				Title: "Read the pipe between Start and Wait",
				Incorrect: "stdout, _ := command.StdoutPipe()\n_ = command.Run()\n_ = stdout",
				Correct: "stdout, _ := command.StdoutPipe()\n_ = command.Start()\n_, _ = io.ReadAll(stdout)\n_ = command.Wait()",
			},
		},
	}
}

func (execPipeRunRule) RunTypes(ctx *TypesContext, node ast.Node) ([]Finding, error) {
	block, ok := node.(*ast.BlockStmt)
	if !ok {
		return nil, fmt.Errorf("exec-pipe-run requires a block statement")
	}
	if ctx == nil || ctx.Info() == nil {
		return nil, fmt.Errorf("exec-pipe-run requires complete type information")
	}
	piped := make(map[types.Object]bool)
	findings := make([]Finding, 0)
	for _, statement := range block.List {
		for object := range assignedDirectObjects(ctx.Info(), statement) {
			delete(piped, object)
		}
		for _, call := range directStatementCalls(statement) {
			object, method := execCmdMethod(ctx.Info(), call)
			if object == nil {
				continue
			}
			switch method {
			case "StdoutPipe", "StderrPipe":
				piped[object] = true
			case "Run":
				if !piped[object] {
					continue
				}
				range_, err := ctx.Range(call)
				if err != nil {
					return nil, err
				}
				findings = append(
					findings,
					Finding{
						MessageKey: "run-after-output-pipe",
						Message: "Cmd.Run may close the output pipe before reads complete",
						Range: range_,
						Help: "call Start, finish reading the pipe, and then call Wait",
					},
				)
			}
		}
	}
	return findings, nil
}

func execCmdMethod(info *types.Info, call *ast.CallExpr) (types.Object, string) {
	if info == nil || call == nil {
		return nil, ""
	}
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if selector == nil {
		return nil, ""
	}
	function, _ := info.ObjectOf(selector.Sel).(*types.Func)
	selection := info.Selections[selector]
	if function == nil ||
		function.Pkg() == nil ||
		function.Pkg().Path() != "os/exec" ||
		selection == nil ||
		namedTypeName(selection.Recv()) != "Cmd" {
		return nil, ""
	}
	return directObject(info, selector.X), function.Name()
}

func assignedDirectObjects(info *types.Info, statement ast.Stmt) map[types.Object]bool {
	result := make(map[types.Object]bool)
	if info == nil || statement == nil {
		return result
	}
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			if node == nil {
				return true
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if _, nested := node.(*ast.BlockStmt); nested {
				return false
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok ||
				(assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE) {
				return true
			}
			for _, expression := range assignment.Lhs {
				if object := directObject(info, expression); object != nil {
					result[object] = true
				}
			}
			return true
		},
	)
	return result
}

func directStatementCalls(statement ast.Stmt) []*ast.CallExpr {
	result := make([]*ast.CallExpr, 0)
	ast.Inspect(
		statement,
		func(node ast.Node) bool {
			if node == nil {
				return true
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if _, nested := node.(*ast.BlockStmt); nested {
				return false
			}
			if call, ok := node.(*ast.CallExpr); ok {
				result = append(result, call)
			}
			return true
		},
	)
	return result
}
