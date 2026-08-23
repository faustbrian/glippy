package analysis_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/ssa"
)

type ssaRule struct {
	metadata rules.Metadata
	run func(*rules.SSAContext) ([]rules.Finding, error)
}

type ambiguousSSARule struct {
	ssaRule
}

type debugSSARule struct {
	ssaRule
}

type initializerSSARule struct {
	ssaRule
}

func (debugSSARule) RequiresSSADebug() {}

func (initializerSSARule) RunsOnSSAInitializers() {}

func (r ambiguousSSARule) RunControlFlow(*rules.ControlFlowContext) ([]rules.Finding, error) {
	return nil, nil
}

func (r ssaRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r ssaRule) RunSSA(ctx *rules.SSAContext) ([]rules.Finding, error) {
	return r.run(ctx)
}

func TestRunSSASharesProgramAndVisitsSourceFunctionsCanonically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "project.go")
	writeTypesFixture(
		t,
		path,
		`package project

var initializer = func() { target() }

func init() { target() }

func outer(value bool) {
	if value { target() }
	_ = func() { target() }
}

func target() {}

type owner struct{}
func (owner) method() {}
`,
	)
	writeTypesFixture(t, filepath.Join(root, "package.go"), "package project\n")
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	programs := make(map[int][]*ssa.Program)
	functions := make(map[int][]*ssa.Function)
	var visits []string
	newRule := func(id string) ssaRule {
		return ssaRule{
			metadata: ssaMetadata(id),
			run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
				if ctx.PackageSyntax().Len() != 2 {
					t.Fatalf("SSA package syntax = %#v", ctx)
				}
				if ctx.File().Path() != path ||
					ctx.PackageID() != "example.com/project" ||
					ctx.Package().Path() != "example.com/project" ||
					ctx.Info() == nil ||
					ctx.Program() == nil ||
					ctx.SSAPackage() == nil ||
					ctx.Function() == nil ||
					ctx.Syntax() == nil ||
					ctx.IllTyped() {
					t.Fatalf("SSA context = %#v", ctx)
				}
				range_, err := ctx.Range(ctx.Syntax())
				if err != nil {
					return nil, err
				}
				programs[range_.Start] = append(
					programs[range_.Start],
					ctx.Program(),
				)
				functions[range_.Start] = append(
					functions[range_.Start],
					ctx.Function(),
				)
				visits = append(visits, id + ":" + functionKind(ctx.Syntax()))
				return []rules.Finding{
					{MessageKey: id, Message: id, Range: range_},
				}, nil
			},
		}
	}
	registry, err := rules.NewRegistry(newRule("z-ssa"), newRule("a-ssa"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunSSA(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}

	wantVisits := []string{
		"a-ssa:literal",
		"z-ssa:literal",
		"a-ssa:declaration",
		"z-ssa:declaration",
		"a-ssa:declaration",
		"z-ssa:declaration",
		"a-ssa:literal",
		"z-ssa:literal",
		"a-ssa:declaration",
		"z-ssa:declaration",
		"a-ssa:declaration",
		"z-ssa:declaration",
	}
	if !reflect.DeepEqual(visits, wantVisits) {
		t.Fatalf("SSA visits = %#v, want %#v", visits, wantVisits)
	}
	if len(programs) != 6 || len(functions) != 6 || len(diagnostics) != 12 {
		t.Fatalf(
			"RunSSA() = programs %#v, functions %#v, diagnostics %#v",
			programs,
			functions,
			diagnostics,
		)
	}
	var sharedProgram *ssa.Program
	for start, values := range programs {
		if len(values) != 2 || values[0] != values[1] {
			t.Fatalf("function at %d received programs %#v", start, values)
		}
		if sharedProgram == nil {
			sharedProgram = values[0]
		} else if values[0] != sharedProgram {
			t.Fatalf("function at %d received a different SSA program", start)
		}
	}
	for start, values := range functions {
		if len(values) != 2 || values[0] != values[1] {
			t.Fatalf("function at %d received SSA functions %#v", start, values)
		}
	}
}

func TestRunSSAInvokesInitializerRulesPerPhysicalFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "first.go"),
		"package project\n\nvar value = build()\n\nfunc build() int { return 1 }\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "second.go"),
		"package project\n\nfunc use() int { return value }\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	initializerVisits := 0
	ordinaryInitializerVisits := 0
	newRule := func(id string, visits *int) ssaRule {
		return ssaRule{
			metadata: ssaMetadata(id),
			run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
				if _, initializer := ctx.Syntax().(*ast.File); initializer {
					(*visits)++
					if ctx.Function().Synthetic != "package initializer" {
						t.Fatalf(
							"initializer SSA function = %#v",
							ctx.Function(),
						)
					}
				}
				return nil, nil
			},
		}
	}
	initializer := initializerSSARule{ssaRule: newRule("initializer-ssa", &initializerVisits)}
	ordinary := newRule("ordinary-ssa", &ordinaryInitializerVisits)
	registry, err := rules.NewRegistry(initializer, ordinary)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSSA(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if initializerVisits != 2 || ordinaryInitializerVisits != 0 {
		t.Fatalf(
			"initializer visits = %d, ordinary visits = %d",
			initializerVisits,
			ordinaryInitializerVisits,
		)
	}
}

func TestRunSSABoundsProgramsByPackageWave(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	packageNames := make([]string, 65)
	for index := range packageNames {
		packageNames[index] = fmt.Sprintf("p%02d", index)
		packageName := packageNames[index]
		directory := filepath.Join(root, packageName)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTypesFixture(
			t,
			filepath.Join(directory, packageName + ".go"),
			"package " + packageName + "\n\nfunc first() {}\nfunc second() {}\n",
		)
	}
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	programs := make(map[string]*ssa.Program)
	visits := make(map[string]int)
	rule := ssaRule{
		metadata: ssaMetadata("package-scoped-ssa"),
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			path := ctx.Package().Path()
			if ctx.Program().Package(ctx.Package()) != ctx.SSAPackage() {
				t.Fatalf("package %q is not owned by its SSA program", path)
			}
			if previous := programs[path];
				previous != nil && previous != ctx.Program() {
				t.Fatalf("package %q received multiple SSA programs", path)
			}
			programs[path] = ctx.Program()
			visits[path]++
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSSA(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if len(programs) != len(packageNames) {
		t.Fatalf("SSA programs = %#v, want %d packages", programs, len(packageNames))
	}
	first := programs["example.com/project/p00"]
	last := programs["example.com/project/p64"]
	if first == nil || last == nil || first == last {
		t.Fatalf("SSA programs = %#v, want a new program after one bounded wave", programs)
	}
	for _, packageName := range packageNames[:64] {
		path := "example.com/project/" + packageName
		if programs[path] != first {
			t.Fatalf(
				"package %q received program %p, want %p",
				path,
				programs[path],
				first,
			)
		}
	}
	for path, count := range visits {
		if count != 2 {
			t.Fatalf("package %q visits = %d, want 2", path, count)
		}
	}
}

func TestRunSSABuildsReturnStatesOnlyWhenEffectFactsAreLoaded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		`package project

type Value struct{}
func lookup() (*Value, error) { return &Value{}, nil }
func inspect() { _, _ = lookup() }
`,
	)
	metadata := ssaMetadata("return-state-demand")
	var summary rules.ReturnStateSummary
	var resultState rules.NilState
	rule := ssaRule{
		metadata: metadata,
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			if ctx.Function().Name() != "inspect" {
				return nil, nil
			}
			for _, block := range ctx.Function().Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(*ssa.Call)
					if !ok || call.Call.StaticCallee() == nil {
						continue
					}
					function, _ := call.Call.StaticCallee().Object().(*types.Func)
					summary = ctx.ReturnState(function, 0, 1)
					resultState = ctx.ResultState(function, 1)
				}
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := func(loadEffects bool) (rules.ReturnStateSummary, rules.NilState) {
		t.Helper()
		summary = rules.ReturnStateSummary{}
		resultState = rules.NilStateUnknown
		loaded, err := analysis.LoadPackages(
			context.Background(),
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				Requirement: rules.RequireSSA,
				LoadEffectFacts: loadEffects,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := analysis.RunSSA(context.Background(), loaded, registry, selection);
			err != nil {
			t.Fatal(err)
		}
		return summary, resultState
	}
	if got, state := run(false);
		got != (rules.ReturnStateSummary{}) || state != rules.NilStateUnknown {
		t.Fatalf("return state without effect requirement = %#v", got)
	}
	if got, state := run(true);
		got.WhenErrorNil != rules.NilStateNonNil || state != rules.NilStateNil {
		t.Fatalf("return facts with effect requirement = %#v, result %v", got, state)
	}
}

func TestRunSSAEnablesExpressionMappingsOnlyForDebugRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/debugssa\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "debug.go"),
		"package debugssa\nfunc source() int { return 1 }\nfunc run() { value := source(); _ = value }\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	checkMapping := func(ctx *rules.SSAContext) bool {
		if ctx.Function().Name() != "run" {
			return false
		}
		var mapped bool
		ast.Inspect(
			ctx.Syntax(),
			func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				value, _ := ctx.Function().ValueForExpr(call)
				mapped = value != nil
				return false
			},
		)
		return mapped
	}
	ordinaryMapped := false
	ordinary := ssaRule{
		metadata: ssaMetadata("ordinary-ssa"),
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			ordinaryMapped = ordinaryMapped || checkMapping(ctx)
			return nil, nil
		},
	}
	ordinaryRegistry, err := rules.NewRegistry(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	ordinarySelection, err := ordinaryRegistry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSSA(
		context.Background(),
		loaded,
		ordinaryRegistry,
		ordinarySelection,
	);
		err != nil {
		t.Fatal(err)
	}
	if ordinaryMapped {
		t.Fatal("ordinary SSA rule unexpectedly received debug expression mappings")
	}
	debugMapped := false
	debug := debugSSARule{
		ssaRule{
			metadata: ssaMetadata("debug-ssa"),
			run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
				debugMapped = debugMapped || checkMapping(ctx)
				return nil, nil
			},
		},
	}
	debugRegistry, err := rules.NewRegistry(debug)
	if err != nil {
		t.Fatal(err)
	}
	debugSelection, err := debugRegistry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSSA(context.Background(), loaded, debugRegistry, debugSelection);
		err != nil {
		t.Fatal(err)
	}
	if !debugMapped {
		t.Fatal("debug SSA rule did not receive expression mappings")
	}
}

func TestRunSSASkipsIllTypedPackagesAndHonorsGeneratedPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	validPath := filepath.Join(root, "valid", "valid.go")
	generatedPath := filepath.Join(root, "generated", "generated.go")
	writeTypesFixture(t, validPath, "package valid\nfunc run() {}\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		"package invalid\nfunc run() { _ = missing }\n",
	)
	writeTypesFixture(
		t,
		generatedPath,
		"// Code generated by test. DO NOT EDIT.\npackage generated\nfunc run() {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./valid", "./invalid", "./generated"},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Diagnostics) == 0 {
		t.Fatal("SSA eligibility fixture did not retain the type error")
	}
	newRule := func(id string, generated bool) ssaRule {
		metadata := ssaMetadata(id)
		metadata.RunOnGenerated = generated
		return ssaRule{
			metadata: metadata,
			run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
				range_, err := ctx.Range(ctx.Syntax())
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{MessageKey: id, Message: id, Range: range_},
				}, nil
			},
		}
	}
	registry, err := rules.NewRegistry(
		newRule("ordinary-ssa", false),
		newRule("generated-ssa", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := analysis.RunSSA(context.Background(), loaded, registry, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 3 ||
		diagnostics[0].Path != generatedPath ||
		diagnostics[0].RuleID != "generated-ssa" ||
		diagnostics[1].Path != validPath ||
		diagnostics[1].RuleID != "generated-ssa" ||
		diagnostics[2].RuleID != "ordinary-ssa" {
		t.Fatalf("RunSSA() eligibility = %#v", diagnostics)
	}
}

func TestRunSSAPreservesCancellationAfterRuleExecution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(t, filepath.Join(root, "project.go"), "package project\nfunc run() {}\n")
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rule := ssaRule{
		metadata: ssaMetadata("cancel-ssa"),
		run: func(ruleContext *rules.SSAContext) ([]rules.Finding, error) {
			cancel()
			range_, err := ruleContext.Range(ruleContext.Syntax())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "canceled", Message: "canceled", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics, err := analysis.RunSSA(ctx, loaded, registry, selection);
		!errors.Is(err, context.Canceled) || diagnostics != nil {
		t.Fatalf("RunSSA() = %#v, %v", diagnostics, err)
	}
}

func TestRunSSARejectsInvalidExecutionContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(t, filepath.Join(root, "project.go"), "package project\nfunc run() {}\n")
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireSSA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	rule := ssaRule{
		metadata: ssaMetadata("valid-ssa"),
		run: func(*rules.SSAContext) ([]rules.Finding, error) {
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSSA(nil, loaded, registry, selection); err == nil {
		t.Fatal("RunSSA() accepted nil context")
	}
	typesLoad := loaded
	typesLoad.Requirement = rules.RequireTypes
	if _, err := analysis.RunSSA(context.Background(), typesLoad, registry, selection);
		err == nil || !strings.Contains(err.Error(), "SSA-tier package load") {
		t.Fatalf("RunSSA() types-load error = %v", err)
	}
	invalidSeverity := append([]rules.Selection(nil), selection...)
	invalidSeverity[0].Severity = "fatal"
	if _, err := analysis.RunSSA(context.Background(), loaded, registry, invalidSeverity);
		err == nil || !strings.Contains(err.Error(), "invalid severity") {
		t.Fatalf("RunSSA() invalid severity error = %v", err)
	}
	missingRegistry, err := rules.NewRegistry(
		metadataRuleAdapter{metadata: ssaMetadata("missing-ssa-execution")},
	)
	if err != nil {
		t.Fatal(err)
	}
	missingSelection, _ := missingRegistry.Resolve(rules.PresetCorrectness, nil)
	if _, err := analysis.RunSSA(
		context.Background(),
		loaded,
		missingRegistry,
		missingSelection,
	);
		err == nil || !strings.Contains(err.Error(), "does not implement SSA execution") {
		t.Fatalf("RunSSA() missing execution error = %v", err)
	}
	ambiguousRegistry, err := rules.NewRegistry(
		ambiguousSSARule{
			ssaRule{
				metadata: ssaMetadata("ambiguous-ssa"),
				run: func(*rules.SSAContext) ([]rules.Finding, error) {
					return nil, nil
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ambiguousSelection, _ := ambiguousRegistry.Resolve(rules.PresetCorrectness, nil)
	if _, err := analysis.RunSSA(
		context.Background(),
		loaded,
		ambiguousRegistry,
		ambiguousSelection,
	);
		err == nil || !strings.Contains(err.Error(), "ambiguous SSA execution") {
		t.Fatalf("RunSSA() ambiguous execution error = %v", err)
	}
}

func TestRunSSAAnalyzesEachPhysicalFunctionOnceAcrossTestVariants(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	projectPath := filepath.Join(root, "project.go")
	testPath := filepath.Join(root, "project_test.go")
	writeTypesFixture(t, projectPath, "package project\nfunc run() {}\n")
	writeTypesFixture(
		t,
		testPath,
		"package project\nimport \"testing\"\nfunc TestRun(t *testing.T) {}\n",
	)
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireSSA,
			Tests: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	owners := make(map[string][]string)
	rule := ssaRule{
		metadata: ssaMetadata("variant-ssa"),
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			owners[ctx.File().Path()] = append(
				owners[ctx.File().Path()],
				ctx.PackageID(),
			)
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunSSA(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if got := owners[projectPath]; !reflect.DeepEqual(got, []string{"example.com/project"}) {
		t.Fatalf("production owners = %#v", got)
	}
	if got := owners[testPath];
		len(got) != 1 || !strings.Contains(got[0], "[example.com/project.test]") {
		t.Fatalf("test owners = %#v", got)
	}
}

func ssaMetadata(id string) rules.Metadata {
	return rules.Metadata{
		ID: id,
		Summary: "reports SSA",
		Documentation: "Full SSA rule documentation.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement: rules.RequireSSA,
		Categories: []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}
}
