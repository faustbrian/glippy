package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

const (
	maximumSSAPackagesPerProgram = 64
	maximumSSAPackageWaveSourceBytes int64 = 8 << 20
)

type activeSSARule struct {
	rule rules.SSARule
	metadata rules.Metadata
	severity rules.Severity
	options rules.OptionSet
}

// RunSSA executes selected SSA-tier rules once per source function through one
// bounded package-wave program shared by all eligible rules.
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
	statistics := statisticsFromContext(ctx)
	tierStarted := beginStatisticsMeasurement(statistics)
	defer statistics.recordTier(rules.RequireSSA, tierStarted)
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
	noReturns := newNoReturnAnalysis(ctx, ssaInputs, loaded.effectFacts)
	effects := cloneNativeEffectFacts(loaded.effectFacts)
	if loaded.effectFacts != nil {
		returnStates := newReturnStateAnalysis(ctx, ssaInputs)
		returnStates.buildAll()
		effects.addReturnStates(returnStates)
	}
	noReturns.buildAll()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	diagnostics := make([]rules.Diagnostic, 0)
	mode := ssaMode(activeRules)
	noReturn := noReturns.predicate()
	filesByPackage := make(map[*packages.Package][]typedPackageFile, len(ssaInputs))
	for _, file := range files {
		filesByPackage[file.package_] = append(filesByPackage[file.package_], file)
	}
	sourceBytesByPackage, err := ssaPackageSourceBytes(ssaInputs, loaded.Sources)
	if err != nil {
		return nil, err
	}
	for _, wave := range ssaPackageWaves(ssaInputs, sourceBytesByPackage) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		produced, err := runSSAPackageWave(
			ctx,
			wave,
			filesByPackage,
			activeRules,
			mode,
			noReturn,
			effects,
		)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, produced...)
	}
	return OrderDiagnostics(diagnostics), nil
}

func ssaPackageSourceBytes(
	packages_ []*packages.Package,
	sources PackageSourceSet,
) (map[*packages.Package]int64, error) {
	result := make(map[*packages.Package]int64, len(packages_))
	for _, pkg := range packages_ {
		seen := make(map[string]struct{}, len(pkg.CompiledGoFiles))
		for _, path := range pkg.CompiledGoFiles {
			if _, found := seen[path]; found {
				continue
			}
			seen[path] = struct{}{}
			file, found := sources.Lookup(path)
			if !found || file == nil {
				return nil, fmt.Errorf(
					"SSA package %q compiled source %q is unavailable",
					pkg.ID,
					path,
				)
			}
			result[pkg] += file.ByteSize()
		}
	}
	return result, nil
}

func ssaPackageWaves(
	packages_ []*packages.Package,
	sourceBytes map[*packages.Package]int64,
) [][]*packages.Package {
	waves := make([][]*packages.Package, 0)
	wave := make([]*packages.Package, 0, min(len(packages_), maximumSSAPackagesPerProgram))
	var waveBytes int64
	flush := func() {
		if len(wave) == 0 {
			return
		}
		waves = append(waves, slices.Clone(wave))
		wave = wave[:0]
		waveBytes = 0
	}
	for _, pkg := range packages_ {
		packageBytes := sourceBytes[pkg]
		if len(wave) > 0 &&
			(len(wave) >= maximumSSAPackagesPerProgram ||
				waveBytes > maximumSSAPackageWaveSourceBytes - packageBytes) {
			flush()
		}
		wave = append(wave, pkg)
		waveBytes += packageBytes
	}
	flush()
	return waves
}

func runSSAPackageWave(
	ctx context.Context,
	packages_ []*packages.Package,
	filesByPackage map[*packages.Package][]typedPackageFile,
	activeRules []activeSSARule,
	mode ssa.BuilderMode,
	noReturn func(*types.Func) bool,
	effects *nativeEffectFacts,
) ([]rules.Diagnostic, error) {
	program, ssaPackages := ssautil.Packages(packages_, mode)
	if len(ssaPackages) != len(packages_) {
		return nil, fmt.Errorf(
			"SSA package wave returned %d packages, want %d",
			len(ssaPackages),
			len(packages_),
		)
	}
	program.SetNoReturn(noReturn)
	program.Build()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	diagnostics := make([]rules.Diagnostic, 0)
	for index, pkg := range packages_ {
		ssaPackage := ssaPackages[index]
		if ssaPackage == nil {
			return nil, fmt.Errorf("well-typed package %q has no SSA package", pkg.ID)
		}
		produced, err := runSSAProgramPackage(
			ctx,
			pkg,
			ssaPackage,
			program,
			filesByPackage[pkg],
			activeRules,
			effects,
		)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, produced...)
	}
	return diagnostics, nil
}

func runSSAProgramPackage(
	ctx context.Context,
	pkg *packages.Package,
	ssaPackage *ssa.Package,
	program *ssa.Program,
	files []typedPackageFile,
	activeRules []activeSSARule,
	effects *nativeEffectFacts,
) ([]rules.Diagnostic, error) {
	functionMap := sourceSSAFunctions(program, ssaPackage, pkg)
	statistics := statisticsFromContext(ctx)
	diagnostics := make([]rules.Diagnostic, 0)
	for _, work := range files {
		file := work.file
		if pkg.IllTyped ||
			(file.source.Metadata().Generated &&
				!anySSARuleRunsGenerated(activeRules)) {
			continue
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
					effects,
				)
				ruleStarted := beginStatisticsMeasurement(statistics)
				findings, err := active.rule.RunSSA(ruleContext)
				statistics.recordRule(
					active.metadata.ID,
					active.metadata.Requirement,
					ruleStarted,
				)
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
	return diagnostics, nil
}

func ssaMode(activeRules []activeSSARule) ssa.BuilderMode {
	for _, active := range activeRules {
		_, ok := active.rule.(rules.SSADebugRule)
		if ok {
			return ssa.GlobalDebug
		}
	}
	return ssa.BuilderMode(0)
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
