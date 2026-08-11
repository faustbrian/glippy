package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"sort"

	"github.com/faustbrian/gox/internal/rules"
	"golang.org/x/tools/go/cfg"
)

type activeControlFlowRule struct {
	rule     rules.ControlFlowRule
	metadata rules.Metadata
	severity rules.Severity
}

type functionBody struct {
	function ast.Node
	body     *ast.BlockStmt
}

// RunControlFlow executes selected CFG-tier rules once per function over one
// graph shared by all eligible rules.
func RunControlFlow(
	ctx context.Context,
	loaded PackageLoadResult,
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Diagnostic, error) {
	if ctx == nil {
		return nil, fmt.Errorf("control-flow analysis requires a context")
	}
	if registry == nil {
		return nil, fmt.Errorf("control-flow analysis requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	activeRules, err := prepareControlFlowRules(registry, selection)
	if err != nil {
		return nil, err
	}
	if len(activeRules) == 0 {
		return []rules.Diagnostic{}, nil
	}
	if loaded.Requirement < rules.RequireControlFlow {
		return nil, fmt.Errorf("control-flow analysis requires a CFG-tier package load")
	}
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return nil, err
	}
	files, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return nil, err
	}

	diagnostics := make([]rules.Diagnostic, 0)
	for _, work := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pkg, file := work.package_, work.file
		eligible := eligibleControlFlowRules(
			activeRules,
			file.source.Metadata().Generated,
			pkg.IllTyped,
		)
		if len(eligible) == 0 {
			continue
		}
		typesContext := rules.NewTypesContext(
			file.source,
			pkg.Fset,
			pkg.ID,
			pkg.Types,
			pkg.TypesInfo,
			pkg.IllTyped,
		)
		for _, function := range functionBodies(file.syntax) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			graph := cfg.New(function.body, mayReturn(pkg.TypesInfo))
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ruleContext := rules.NewControlFlowContext(
				typesContext,
				function.function,
				function.body,
				graph,
			)
			for _, active := range eligible {
				findings, err := active.rule.RunControlFlow(ruleContext)
				if contextErr := ctx.Err(); contextErr != nil {
					return nil, contextErr
				}
				if err != nil {
					return nil, fmt.Errorf("%s: %w", active.metadata.ID, err)
				}
				for _, finding := range findings {
					diagnostic, err := diagnosticForFinding(
						file.source,
						active.metadata,
						active.severity,
						finding,
					)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", active.metadata.ID, err)
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}
	}
	return OrderDiagnostics(diagnostics), nil
}

func prepareControlFlowRules(
	registry *rules.Registry,
	selection []rules.Selection,
) ([]activeControlFlowRule, error) {
	ordered := slices.Clone(selection)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].ID < ordered[right].ID })
	activeRules := make([]activeControlFlowRule, 0, len(ordered))
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, fmt.Errorf("selected rule %q more than once", selected.ID)
		}
		previousID = selected.ID
		if selected.Severity != rules.SeverityWarn && selected.Severity != rules.SeverityError {
			return nil, fmt.Errorf("selected rule %q has invalid severity %q", selected.ID, selected.Severity)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, fmt.Errorf("selected rule %q requirement does not match registry", selected.ID)
		}
		if metadata.Requirement != rules.RequireControlFlow {
			return nil, fmt.Errorf(
				"selected rule %q requires %s; control-flow runner requires control-flow rules",
				selected.ID,
				metadata.Requirement,
			)
		}
		controlFlowRule, found := nativeRule.(rules.ControlFlowRule)
		if !found {
			return nil, fmt.Errorf("selected rule %q does not implement control-flow execution", selected.ID)
		}
		if implementsOtherExecution(nativeRule) {
			return nil, fmt.Errorf("selected rule %q implements ambiguous control-flow execution", selected.ID)
		}
		activeRules = append(activeRules, activeControlFlowRule{
			rule: controlFlowRule, metadata: metadata, severity: selected.Severity,
		})
	}
	return activeRules, nil
}

func implementsOtherExecution(rule rules.Rule) bool {
	_, syntax := rule.(rules.SyntaxRule)
	_, syntaxFile := rule.(rules.SyntaxFileRule)
	_, typed := rule.(rules.TypesRule)
	return syntax || syntaxFile || typed
}

func eligibleControlFlowRules(
	activeRules []activeControlFlowRule,
	generated bool,
	illTyped bool,
) []activeControlFlowRule {
	eligible := make([]activeControlFlowRule, 0, len(activeRules))
	for _, active := range activeRules {
		if generated && !active.metadata.RunOnGenerated {
			continue
		}
		if illTyped && !active.metadata.RunDespiteTypeErrors {
			continue
		}
		eligible = append(eligible, active)
	}
	return eligible
}

func functionBodies(file *ast.File) []functionBody {
	functions := make([]functionBody, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncDecl:
			if node.Body != nil {
				functions = append(functions, functionBody{function: node, body: node.Body})
			}
		case *ast.FuncLit:
			if node.Body != nil {
				functions = append(functions, functionBody{function: node, body: node.Body})
			}
		}
		return true
	})
	return functions
}

func mayReturn(info *types.Info) func(*ast.CallExpr) bool {
	panicObject := types.Universe.Lookup("panic")
	return func(call *ast.CallExpr) bool {
		function := call.Fun
		for {
			parenthesized, ok := function.(*ast.ParenExpr)
			if !ok {
				break
			}
			function = parenthesized.X
		}
		identifier, ok := function.(*ast.Ident)
		return !ok || info.Uses[identifier] != panicObject
	}
}
