package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"sort"

	"github.com/faustbrian/gox/internal/rules"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type activeSSARule struct {
	rule rules.SSARule
	metadata rules.Metadata
	severity rules.Severity
	options rules.OptionSet
}

// RunSSA executes selected SSA-tier rules once per source function through one
// program shared by all eligible rules and selected packages.
func RunSSA(
	ctx context.Context,
	loaded PackageLoadResult,
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Diagnostic, error) {
	if ctx == nil {
		return nil, fmt.Errorf("SSA analysis requires a context")
	}
	if registry == nil {
		return nil, fmt.Errorf("SSA analysis requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	activeRules, err := prepareSSARules(registry, selection)
	if err != nil {
		return nil, err
	}
	if len(activeRules) == 0 {
		return []rules.Diagnostic{}, nil
	}
	if loaded.Requirement < rules.RequireSSA {
		return nil, fmt.Errorf("SSA analysis requires an SSA-tier package load")
	}
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return nil, err
	}
	files, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return nil, err
	}
	ssaInputs := eligibleSSAPackages(packages_, files, activeRules)
	if len(ssaInputs) == 0 {
		return []rules.Diagnostic{}, nil
	}
	if err := validateSharedFileSet(ssaInputs); err != nil {
		return nil, err
	}
	program, ssaPackages := ssautil.Packages(ssaInputs, ssa.BuilderMode(0))
	program.Build()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ssaByPackage := make(map[*packages.Package]*ssa.Package, len(ssaInputs))
	for index, pkg := range ssaInputs {
		ssaByPackage[pkg] = ssaPackages[index]
	}
	functionsByPackage := make(map[*ssa.Package]map[ast.Node]*ssa.Function)
	diagnostics := make([]rules.Diagnostic, 0)
	for _, work := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pkg, file := work.package_, work.file
		if pkg.IllTyped ||
			(file.source.Metadata().Generated &&
				!anySSARuleRunsGenerated(activeRules)) {
			continue
		}
		ssaPackage := ssaByPackage[pkg]
		if ssaPackage == nil {
			return nil, fmt.Errorf("well-typed package %q has no SSA package", pkg.ID)
		}
		functionMap, found := functionsByPackage[ssaPackage]
		if !found {
			functionMap = sourceSSAFunctions(program, ssaPackage, pkg)
			functionsByPackage[ssaPackage] = functionMap
		}
		typesContexts := make(map[string]*rules.TypesContext, len(activeRules))
		for _, active := range activeRules {
			typesContexts[active.metadata.ID] = rules.NewTypesContext(
				file.source,
				pkg.Fset,
				pkg.ID,
				pkg.Types,
				pkg.TypesInfo,
				pkg.IllTyped,
				active.options,
			)
		}
		for _, sourceFunction := range functionBodies(file.syntax) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			function := functionMap[sourceFunction.function]
			if function == nil {
				return nil, fmt.Errorf(
					"package %q source function has no SSA function",
					pkg.ID,
				)
			}
			for _, active := range activeRules {
				if file.source.Metadata().Generated &&
					!active.metadata.RunOnGenerated {
					continue
				}
				ruleContext := rules.NewSSAContext(
					typesContexts[active.metadata.ID],
					program,
					ssaPackage,
					function,
					sourceFunction.function,
				)
				findings, err := active.rule.RunSSA(ruleContext)
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
						return nil, fmt.Errorf(
							"%s: %w",
							active.metadata.ID,
							err,
						)
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}
	}
	return OrderDiagnostics(diagnostics), nil
}

func prepareSSARules(
	registry *rules.Registry,
	selection []rules.Selection,
) ([]activeSSARule, error) {
	ordered := slices.Clone(selection)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].ID < ordered[right].ID
		},
	)
	activeRules := make([]activeSSARule, 0, len(ordered))
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, fmt.Errorf("selected rule %q more than once", selected.ID)
		}
		previousID = selected.ID
		if selected.Severity != rules.SeverityWarn &&
			selected.Severity != rules.SeverityError {
			return nil, fmt.Errorf(
				"selected rule %q has invalid severity %q",
				selected.ID,
				selected.Severity,
			)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, fmt.Errorf(
				"selected rule %q requirement does not match registry",
				selected.ID,
			)
		}
		if metadata.Requirement != rules.RequireSSA {
			return nil, fmt.Errorf(
				"selected rule %q requires %s; SSA runner requires SSA rules",
				selected.ID,
				metadata.Requirement,
			)
		}
		ssaRule, found := nativeRule.(rules.SSARule)
		if !found {
			return nil, fmt.Errorf(
				"selected rule %q does not implement SSA execution",
				selected.ID,
			)
		}
		if implementsNonSSAExecution(nativeRule) {
			return nil, fmt.Errorf(
				"selected rule %q implements ambiguous SSA execution",
				selected.ID,
			)
		}
		activeRules = append(
			activeRules,
			activeSSARule{
				rule: ssaRule,
				metadata: metadata,
				severity: selected.Severity,
				options: selected.Options,
			},
		)
	}
	return activeRules, nil
}

func implementsNonSSAExecution(rule rules.Rule) bool {
	_, syntax := rule.(rules.SyntaxRule)
	_, syntaxFile := rule.(rules.SyntaxFileRule)
	_, typed := rule.(rules.TypesRule)
	_, controlFlow := rule.(rules.ControlFlowRule)
	return syntax || syntaxFile || typed || controlFlow
}

func anySSARuleRunsGenerated(activeRules []activeSSARule) bool {
	for _, active := range activeRules {
		if active.metadata.RunOnGenerated {
			return true
		}
	}
	return false
}

func eligibleSSAPackages(
	packages_ []*packages.Package,
	files []typedPackageFile,
	activeRules []activeSSARule,
) []*packages.Package {
	eligible := make(map[*packages.Package]bool, len(packages_))
	runsGenerated := anySSARuleRunsGenerated(activeRules)
	for _, work := range files {
		if !work.package_.IllTyped &&
			(!work.file.source.Metadata().Generated || runsGenerated) {
			eligible[work.package_] = true
		}
	}
	result := make([]*packages.Package, 0, len(eligible))
	for _, pkg := range packages_ {
		if eligible[pkg] {
			result = append(result, pkg)
		}
	}
	return result
}

func validateSharedFileSet(packages_ []*packages.Package) error {
	if len(packages_) == 0 {
		return nil
	}
	fileSet := packages_[0].Fset
	for _, pkg := range packages_[1:] {
		if pkg.Fset != fileSet {
			return fmt.Errorf("typed packages do not share one file set")
		}
	}
	return nil
}

func sourceSSAFunctions(
	program *ssa.Program,
	ssaPackage *ssa.Package,
	pkg *packages.Package,
) map[ast.Node]*ssa.Function {
	functions := make(map[ast.Node]*ssa.Function)
	var add func(*ssa.Function)
	add = func(function *ssa.Function) {
		if function == nil {
			return
		}
		if syntax := function.Syntax(); syntax != nil {
			functions[syntax] = function
		}
		for _, anonymous := range function.AnonFuncs {
			add(anonymous)
		}
	}
	add(ssaPackage.Func("init"))
	for _, file := range pkg.Syntax {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pkg.TypesInfo.Defs[function.Name].(*types.Func)
			if ok {
				add(program.FuncValue(object))
			}
		}
	}
	return functions
}
