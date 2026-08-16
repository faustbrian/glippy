package analysis_test

import (
	"context"
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestRunControlFlowUsesConfiguredProjectContracts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, root + "/go.mod", "module example.com/project\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		root + "/project.go",
		`package project

type Resource struct{}

func Stop() {}
func Consume(*Resource) {}
func Wait() {}
func Open(*Resource) (*Resource, error) { return nil, nil }

func inspect(seed *Resource) {
	Consume(seed)
	Wait()
	value, err := Open(seed)
	_, _ = value, err
	Stop()
	println("unreachable")
}
`,
	)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project.Stop"
noreturn = true

[[functions]]
symbol = "example.com/project.Consume"
closes = [0]
takes-ownership = [0]

[[functions]]
symbol = "example.com/project.Wait"
blocking = true

[[functions]]
symbol = "example.com/project.Open"
must-use = [0, 1]
returns-alias = [{ result = 0, argument = 0 }]
nil-error = [{ value = 0, error = 1, when-error-nil = "non-nil", when-error-non-nil = "nil" }]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireControlFlow,
			LoadEffectFacts: true,
			Contracts: set,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	type observation struct {
		mayReturn bool
		parameter rules.ParameterEffectSummary
		blocking bool
		mustUse bool
		alias bool
		returnState rules.ReturnStateSummary
	}
	observed := make(map[string]observation)
	metadata := controlFlowMetadata("configured-contracts")
	metadata.RequiresEffectFacts = true
	rule := controlFlowRule{
		metadata: metadata,
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			declaration, ok := ctx.Function().(*ast.FuncDecl)
			if !ok || declaration.Name.Name != "inspect" {
				return nil, nil
			}
			ast.Inspect(
				declaration.Body,
				func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					identifier, _ := call.Fun.(*ast.Ident)
					if identifier == nil {
						return true
					}
					observed[identifier.Name] = observation{
						mayReturn: ctx.CallMayReturn(call),
						parameter: ctx.ParameterEffect(call, 0),
						blocking: ctx.Blocking(call),
						mustUse: ctx.MustUse(call, 0),
						alias: ctx.ReturnAliasesArgument(call, 0, 0),
						returnState: ctx.ReturnState(call, 0, 1),
					}
					return true
				},
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
	if _, err := analysis.RunControlFlow(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if observed["Stop"].mayReturn {
		t.Fatal("configured noreturn contract did not change shared reachability")
	}
	consume := observed["Consume"].parameter
	if !consume.GuaranteesAny(rules.ParameterEffectClose | rules.ParameterEffectTransfer) {
		t.Fatalf("configured parameter effects = %#v", consume)
	}
	if !observed["Wait"].blocking {
		t.Fatal("configured blocking contract was unavailable")
	}
	open := observed["Open"]
	if !open.mustUse ||
		!open.alias ||
		open.returnState.WhenErrorNil != rules.NilStateNonNil ||
		open.returnState.WhenErrorNonNil != rules.NilStateNil {
		t.Fatalf("configured Open effects = %#v", open)
	}
}

func TestLoadPackagesValidatesContractsOnlyForEffectConsumers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, root + "/go.mod", "module example.com/project\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		root + "/project.go",
		"package project\nfunc Open() error { return nil }\n",
	)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/project.Open"
must-use = [1]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	base := analysis.PackageLoadOptions{
		Dir: root,
		Patterns: []string{"."},
		Requirement: rules.RequireTypes,
		Contracts: set,
	}
	if _, err := analysis.LoadPackages(context.Background(), base); err != nil {
		t.Fatalf("LoadPackages() without effect consumer = %v", err)
	}
	base.LoadEffectFacts = true
	if _, err := analysis.LoadPackages(context.Background(), base);
		err == nil || !strings.Contains(err.Error(), "result index 1") {
		t.Fatalf("LoadPackages() contract error = %v, want result validation", err)
	}
}

func TestConfiguredExternalContractDoesNotLoadDependencySource(t *testing.T) {
	t.Parallel()

	dependency := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(dependency, "go.mod"),
		"module example.com/dependency\n\ngo 1.26.0\n",
	)
	dependencySource := filepath.Join(dependency, "stop.go")
	writeTypesFixture(t, dependencySource, "package dependency\nfunc Stop() {}\n")
	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n\nrequire example.com/dependency v0.0.0\nreplace example.com/dependency => " +
			dependency +
			"\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nimport \"example.com/dependency\"\nfunc run() { dependency.Stop(); println(\"unreachable\") }\n",
	)
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					"version = 1\n[[functions]]\nsymbol = \"example.com/dependency.Stop\"\nnoreturn = true\n",
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := analysis.LoadPackages(
		context.Background(),
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Requirement: rules.RequireControlFlow,
			LoadEffectFacts: true,
			Contracts: set,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range loaded.Sources.Paths() {
		if path == dependencySource {
			t.Fatal("configured external contract loaded dependency source")
		}
	}
	mayReturn := true
	metadata := controlFlowMetadata("external-contract")
	metadata.RequiresEffectFacts = true
	registry, err := rules.NewRegistry(
		controlFlowRule{
			metadata: metadata,
			run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
				declaration, ok := ctx.Function().(*ast.FuncDecl)
				if !ok || declaration.Name.Name != "run" {
					return nil, nil
				}
				statement, _ := declaration.Body.List[0].(*ast.ExprStmt)
				call, _ := statement.X.(*ast.CallExpr)
				mayReturn = ctx.CallMayReturn(call)
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.Resolve(rules.PresetCorrectness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunControlFlow(context.Background(), loaded, registry, selection);
		err != nil {
		t.Fatal(err)
	}
	if mayReturn {
		t.Fatal("external noreturn contract was not available from type information")
	}
}
