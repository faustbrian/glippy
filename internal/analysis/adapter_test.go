package analysis_test

import (
	"context"
	"errors"
	"flag"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	goanalysis "golang.org/x/tools/go/analysis"
	atomicanalyzer "golang.org/x/tools/go/analysis/passes/atomic"
	ctrlflowanalyzer "golang.org/x/tools/go/analysis/passes/ctrlflow"
	pkgfactanalyzer "golang.org/x/tools/go/analysis/passes/pkgfact"
	testsanalyzer "golang.org/x/tools/go/analysis/passes/tests"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/cache"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type adapterFact struct{}

func (*adapterFact) AFact() {}

type packageScopeFact struct {
	Value string
}

func (*packageScopeFact) AFact() {}

type nondeterministicPackageFact struct {
	encodings byte
}

func (*nondeterministicPackageFact) AFact() {}

func (f *nondeterministicPackageFact) GobEncode() ([]byte, error) {
	f.encodings++
	return []byte{f.encodings}, nil
}

func (*nondeterministicPackageFact) GobDecode([]byte) error { return nil }

func TestAdaptAnalyzerRunsOnAnIsolatedSyntaxViewAndMapsDiagnostics(t *testing.T) {
	t.Parallel()

	input := `package sample

//gox:ignore external-call -- accepted here
func suppressed() { target() }

func visible() { target() }
`
	file, err := source.Load("/project/source.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	upstream := &goanalysis.Analyzer{
		Name: "externalcall",
		Doc:  "reports target calls",
		URL:  "https://example.test/external-call",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Pkg == nil || pass.Pkg.Name() != "sample" || pass.TypesInfo != nil ||
				pass.TypesSizes != nil || len(pass.Files) != 1 || len(pass.ResultOf) != 0 {
				return nil, errors.New("adapter pass exposed an invalid syntax-only package")
			}
			path := pass.Fset.PositionFor(pass.Files[0].Pos(), false).Filename
			contents, err := pass.ReadFile(path)
			if err != nil || string(contents) != input {
				return nil, errors.New("adapter pass did not expose exact source bytes")
			}
			contents[0] = 'X'
			contents, err = pass.ReadFile(path)
			if err != nil || string(contents) != input {
				return nil, errors.New("adapter pass exposed mutable source bytes")
			}
			if _, err := pass.ReadFile("/project/other.go"); err == nil {
				return nil, errors.New("adapter pass read an undeclared file")
			}
			calls := make([]*ast.CallExpr, 0, 2)
			ast.Inspect(pass.Files[0], func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if ok {
					calls = append(calls, call)
				}
				return true
			})
			for index := len(calls) - 1; index >= 0; index-- {
				call := calls[index]
				pass.Report(goanalysis.Diagnostic{
					Pos:      call.Pos(),
					End:      call.End(),
					Category: "target-call",
					Message:  "target call requires review",
					Related: []goanalysis.RelatedInformation{{
						Pos:     call.Fun.Pos(),
						End:     call.Fun.End(),
						Message: "called function",
					}},
					SuggestedFixes: []goanalysis.SuggestedFix{{
						Message: "Replace target call",
						TextEdits: []goanalysis.TextEdit{{
							Pos:     call.Pos(),
							End:     call.End(),
							NewText: []byte("primary()"),
						}},
					}},
				})
			}
			return nil, nil
		},
	}
	metadata := analysisMetadata("external-call", rules.NodeFile, false)
	adapted, err := analysis.AdaptAnalyzer(upstream, analysis.AnalyzerAdapterOptions{
		Metadata: metadata,
		SuggestedFixes: []analysis.AnalyzerFixMapping{{
			Message:     "Replace target call",
			Name:        "replace-target",
			Description: "replace the target call",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Overrides: map[string]rules.Severity{
			"external-call": rules.SeverityError,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Suppressed) != 1 || len(result.Diagnostics) != 1 {
		t.Fatalf("adapter diagnostics = visible %#v, suppressed %#v", result.Diagnostics, result.Suppressed)
	}
	diagnostic := result.Diagnostics[0]
	visibleStart := strings.LastIndex(input, "target()")
	if diagnostic.RuleID != "external-call" || diagnostic.Severity != rules.SeverityError ||
		diagnostic.MessageKey != "target-call" || diagnostic.Range.Start != visibleStart ||
		len(diagnostic.Related) != 1 || diagnostic.Related[0].Message != "called function" ||
		diagnostic.Help != "https://example.test/external-call#target-call" ||
		len(diagnostic.Fixes) != 1 || diagnostic.Fixes[0].Name != "replace-target" ||
		diagnostic.Fixes[0].Safety != rules.FixSuggestion ||
		len(diagnostic.Fixes[0].Edits) != 1 || diagnostic.Fixes[0].Edits[0].NewText != "primary()" {
		t.Fatalf("adapter diagnostic = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerRunsTypedPackagesOncePerOwnedSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	testPath := filepath.Join(root, "adapter_test.go")
	input := `package adapter

import "testing"

func Testwrong(t *testing.T) {}
`
	writeTypesFixture(t, testPath, input)

	options := adapterOptions("test-name")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(testsanalyzer.Analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir:        root,
			Patterns:   []string{"."},
			Tests:      true,
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := make([]rules.Diagnostic, 0)
	for _, file := range result.Files {
		diagnostics = append(diagnostics, file.Diagnostics...)
	}
	if len(result.LoadDiagnostics) != 0 || len(result.SourceProblems) != 0 || len(diagnostics) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	diagnostic := diagnostics[0]
	start := strings.Index(input, "Testwrong")
	if diagnostic.RuleID != "test-name" || diagnostic.Path != testPath ||
		diagnostic.Range != (source.Range{Start: start, End: start + len("Testwrong")}) ||
		!strings.Contains(diagnostic.Message, "Testwrong has malformed name") {
		t.Fatalf("typed adapter diagnostic = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerRunsTypedPrerequisiteAnalyzers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	path := filepath.Join(root, "adapter.go")
	input := `package adapter

import "sync/atomic"

func increment(value *uint64) {
	*value = atomic.AddUint64(value, 1)
}
`
	writeTypesFixture(t, path, input)
	options := adapterOptions("atomic-assignment")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(atomicanalyzer.Analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	start := strings.Index(input, "*value =")
	if diagnostic.RuleID != "atomic-assignment" || diagnostic.Path != path ||
		diagnostic.Range.Start != start ||
		!strings.Contains(diagnostic.Message, "direct assignment to atomic value") {
		t.Fatalf("atomic diagnostic = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerStopsAfterCanceledTypedPrerequisite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	ctx, cancel := context.WithCancel(context.Background())
	prerequisite := &goanalysis.Analyzer{
		Name:       "cancelingprerequisite",
		Doc:        "cancels prerequisite execution",
		ResultType: reflect.TypeFor[string](),
		Run: func(*goanalysis.Pass) (any, error) {
			cancel()
			return "ready", nil
		},
	}
	rootRan := false
	upstream := &goanalysis.Analyzer{
		Name:     "aftercancellation",
		Doc:      "must not run after cancellation",
		Requires: []*goanalysis.Analyzer{prerequisite},
		Run: func(*goanalysis.Pass) (any, error) {
			rootRan = true
			return nil, nil
		},
	}
	options := adapterOptions("prerequisite-cancellation")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		ctx,
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if !errors.Is(err, context.Canceled) || rootRan {
		t.Fatalf("RunPackages() error = %v, root ran = %t", err, rootRan)
	}
}

func TestAdaptAnalyzerRunsSharedTypedPrerequisiteDAGOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	leafRuns := 0
	leaf := &goanalysis.Analyzer{
		Name:       "leafresult",
		Doc:        "provides a shared result",
		ResultType: reflect.TypeFor[string](),
		Run: func(*goanalysis.Pass) (any, error) {
			leafRuns++
			return "shared", nil
		},
	}
	newBranch := func(name string) *goanalysis.Analyzer {
		return &goanalysis.Analyzer{
			Name: name, Doc: "consumes the shared result", Requires: []*goanalysis.Analyzer{leaf},
			ResultType: reflect.TypeFor[int](),
			Run: func(pass *goanalysis.Pass) (any, error) {
				if pass.ResultOf[leaf] != "shared" {
					return nil, errors.New("branch omitted shared prerequisite result")
				}
				return len(name), nil
			},
		}
	}
	left := newBranch("leftbranch")
	right := newBranch("rightbranch")
	upstream := &goanalysis.Analyzer{
		Name: "dagroot", Doc: "consumes both branches",
		Requires:   []*goanalysis.Analyzer{right, left},
		ResultType: reflect.TypeFor[string](),
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.ResultOf[left] != len(left.Name) || pass.ResultOf[right] != len(right.Name) ||
				len(pass.ResultOf) != 2 {
				return nil, errors.New("root omitted direct prerequisite results")
			}
			pass.ReportRangef(pass.Files[0].Name, "prerequisite DAG")
			return "complete", nil
		},
	}
	options := adapterOptions("prerequisite-dag")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if leafRuns != 1 || len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("prerequisite DAG runs = %d, result = %#v", leafRuns, result)
	}
}

func TestAdaptAnalyzerRunsPackageFactsAcrossDependencies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "dep", "dep.go"),
		"package dep\nimport _ \"fmt\"\nconst _greeting_ = \"hello\"\n",
	)
	path := filepath.Join(root, "adapter.go")
	input := `package adapter

import _ "example.com/adapter/dep"

const _audience_ = "world"
`
	writeTypesFixture(t, path, input)
	options := adapterOptions("package-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(pkgfactanalyzer.Analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	importSpec := `_ "example.com/adapter/dep"`
	start := strings.Index(input, importSpec)
	if diagnostic.RuleID != "package-facts" || diagnostic.Path != path ||
		diagnostic.Range != (source.Range{Start: start, End: start + len(importSpec)}) ||
		diagnostic.Message != "greeting=\"hello\"" {
		t.Fatalf("package fact diagnostic = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerCachesPackageFactsAcrossIndependentLoads(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	dependencyPath := filepath.Join(root, "dep", "dep.go")
	writeTypesFixture(
		t,
		dependencyPath,
		"package dep\nconst _greeting_ = \"hello\"\n",
	)
	path := filepath.Join(root, "adapter.go")
	writeTypesFixture(t, path, "package adapter\nimport _ \"example.com/adapter/dep\"\nconst _audience_ = \"world\"\n")

	upstream := *pkgfactanalyzer.Analyzer
	runs := 0
	run := upstream.Run
	upstream.Run = func(pass *goanalysis.Pass) (any, error) {
		runs++
		return run(pass)
	}
	options := adapterOptions("cached-package-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(&upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	runOptions := analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache:  packageAnalyzerCacheOptions(store),
	}
	loadOptions := analysis.PackageLoadOptions{
		Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		Env:  append(os.Environ(), "CGO_ENABLED=0", "GOENV=off"),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
	}
	first, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	coldRuns := runs
	if coldRuns < 2 || len(first.Files) != 1 || len(first.Files[0].Diagnostics) != 1 ||
		first.Files[0].Diagnostics[0].Message != `greeting="hello"` {
		t.Fatalf("cold RunPackages() runs = %d, result = %#v", coldRuns, first)
	}
	second, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	if runs != coldRuns || !reflect.DeepEqual(second.Files, first.Files) {
		t.Fatalf("warm RunPackages() runs = %d, want %d; result = %#v", runs, coldRuns, second)
	}
	corruptPackageAnalyzerCache(t, cacheRoot)
	recovered, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	recoveredRuns := runs
	if recoveredRuns <= coldRuns || !reflect.DeepEqual(recovered.Files, first.Files) {
		t.Fatalf("recovered RunPackages() runs = %d, result = %#v", recoveredRuns, recovered)
	}
	if _, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions); err != nil {
		t.Fatal(err)
	}
	if runs != recoveredRuns {
		t.Fatalf("repaired warm RunPackages() runs = %d, want %d", runs, recoveredRuns)
	}

	writeTypesFixture(
		t,
		dependencyPath,
		"package dep\nconst _greeting_ = \"changed\"\n",
	)
	third, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	if runs <= recoveredRuns || len(third.Files) != 1 || len(third.Files[0].Diagnostics) != 1 ||
		third.Files[0].Diagnostics[0].Message != `greeting="changed"` {
		t.Fatalf("invalidated RunPackages() runs = %d, result = %#v", runs, third)
	}
}

func TestAdaptAnalyzerDoesNotCacheUnsupportedLocalObjectFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\nfunc run() { local := 1; _ = local }\n")
	runs := 0
	analyzer := &goanalysis.Analyzer{
		Name: "localfacts",
		Doc:  "exports an intentionally unpersistable local object fact",
		Run: func(pass *goanalysis.Pass) (any, error) {
			runs++
			for identifier, object := range pass.TypesInfo.Defs {
				if identifier.Name == "local" {
					pass.ExportObjectFact(object, &packageScopeFact{Value: "local"})
				}
			}
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(packageScopeFact)},
	}
	options := adapterOptions("uncacheable-local-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	store, err := cache.Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	runOptions := analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache:  packageAnalyzerCacheOptions(store),
	}
	loadOptions := packageAnalyzerCacheLoadOptions(root)
	for range 2 {
		result, err := analysis.RunPackages(
			context.Background(), registry, runOptions, loadOptions,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
			t.Fatalf("RunPackages() = %#v", result)
		}
	}
	if runs != 2 {
		t.Fatalf("uncacheable analyzer runs = %d, want 2", runs)
	}
}

func packageAnalyzerCacheOptions(store *cache.Store) *analysis.PackageCacheOptions {
	return &analysis.PackageCacheOptions{
		Store: store, ToolVersion: "v0.1.0", BuildGoVersion: runtime.Version(),
		SourceGoVersion: "1.26", Configuration: cache.DigestOf([]byte("configuration")),
		FormatterMode: "gox-v1",
	}
}

func packageAnalyzerCacheLoadOptions(root string) analysis.PackageLoadOptions {
	return analysis.PackageLoadOptions{
		Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		Env:  append(os.Environ(), "CGO_ENABLED=0", "GOENV=off"),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
	}
}

func corruptPackageAnalyzerCache(t *testing.T, root string) {
	t.Helper()
	corrupted := 0
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			corrupted++
			return os.WriteFile(path, []byte("corrupt"), 0o600)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if corrupted == 0 {
		t.Fatal("package analyzer cache did not contain an entry to corrupt")
	}
}

func TestAdaptAnalyzerRunsObjectFactsAcrossDependencies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "leaf", "leaf.go"),
		"package leaf\ntype T struct{}\nfunc (T) Die() { panic(\"stop\") }\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "dep", "dep.go"),
		"package dep\nimport \"example.com/adapter/leaf\"\ntype T = leaf.T\n",
	)
	path := filepath.Join(root, "adapter.go")
	input := `package adapter

import "example.com/adapter/dep"

func Run() { dep.T{}.Die() }
`
	writeTypesFixture(t, path, input)
	analyzer := &goanalysis.Analyzer{
		Name: "objectfactconsumer",
		Doc:  "reports control flow proven by an imported object fact",
		Run: func(pass *goanalysis.Pass) (any, error) {
			object, _ := pass.Pkg.Scope().Lookup("Run").(*types.Func)
			if object == nil {
				return nil, nil
			}
			cfgs := pass.ResultOf[ctrlflowanalyzer.Analyzer].(*ctrlflowanalyzer.CFGs)
			if !cfgs.NoReturn(object) {
				return nil, errors.New("Run was not proven non-returning")
			}
			for _, declaration := range pass.Files[0].Decls {
				if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "Run" {
					pass.ReportRangef(function.Name, "imported no-return fact")
				}
			}
			return nil, nil
		},
		Requires: []*goanalysis.Analyzer{ctrlflowanalyzer.Analyzer},
	}
	options := adapterOptions("object-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	start := strings.Index(input, "Run")
	if diagnostic.RuleID != "object-facts" || diagnostic.Path != path ||
		diagnostic.Range != (source.Range{Start: start, End: start + len("Run")}) ||
		diagnostic.Message != "imported no-return fact" {
		t.Fatalf("object fact diagnostic = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerScopesPackageFactsToEachRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "shared", "shared.go"),
		"package shared\nconst Value = 1\n",
	)
	for _, name := range []string{"left", "right"} {
		writeTypesFixture(
			t,
			filepath.Join(root, name, name+".go"),
			"package "+name+"\nimport _ \"example.com/adapter/shared\"\n",
		)
	}
	runs := make(map[string]int)
	analyzer := &goanalysis.Analyzer{
		Name: "scopedfacts",
		Doc:  "reports the package facts visible to each analyzed package",
		Run: func(pass *goanalysis.Pass) (any, error) {
			path := pass.Pkg.Path()
			runs[path]++
			pass.ExportPackageFact(&packageScopeFact{Value: path})
			visible := pass.AllPackageFacts()
			paths := make([]string, len(visible))
			for index, fact := range visible {
				paths[index] = fact.Package.Path()
			}
			pass.ReportRangef(pass.Files[0].Name, "%s", strings.Join(paths, ","))
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(packageScopeFact)},
	}
	options := adapterOptions("scoped-package-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"./left", "./right"}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if runs["example.com/adapter/shared"] != 1 {
		t.Fatalf("shared analyzer runs = %d, want 1", runs["example.com/adapter/shared"])
	}
	if len(result.Files) != 2 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	for _, file := range result.Files {
		if len(file.Diagnostics) != 1 {
			t.Fatalf("diagnostics for %q = %#v", file.Path, file.Diagnostics)
		}
		name := strings.TrimSuffix(filepath.Base(file.Path), ".go")
		want := "example.com/adapter/" + name + ",example.com/adapter/shared"
		if file.Diagnostics[0].Message != want {
			t.Fatalf("diagnostic for %q = %q, want %q", file.Path, file.Diagnostics[0].Message, want)
		}
	}
}

func TestAdaptAnalyzerCopiesExportedPackageFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "dep", "dep.go"),
		"package dep\nconst Value = 1\n",
	)
	path := filepath.Join(root, "adapter.go")
	writeTypesFixture(t, path, "package adapter\nimport _ \"example.com/adapter/dep\"\n")
	analyzer := &goanalysis.Analyzer{
		Name: "copyfacts",
		Doc:  "proves exported and imported package facts are independent copies",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Pkg.Path() == "example.com/adapter/dep" {
				fact := &packageScopeFact{Value: "exported"}
				pass.ExportPackageFact(fact)
				fact.Value = "mutated"
				return nil, nil
			}
			fact := &packageScopeFact{Value: "dirty destination"}
			if !pass.ImportPackageFact(pass.Pkg.Imports()[0], fact) {
				return nil, errors.New("dependency fact was not imported")
			}
			pass.ReportRangef(pass.Files[0].Name, "%s", fact.Value)
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(packageScopeFact)},
	}
	options := adapterOptions("copied-package-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].Message != "exported" {
		t.Fatalf("RunPackages() = %#v", result)
	}
}

func TestAdaptAnalyzerRejectsUndeclaredPackageFactType(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	analyzer := &goanalysis.Analyzer{
		Name: "undeclaredfact",
		Doc:  "attempts to export an undeclared package fact type",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.ExportPackageFact(new(packageScopeFact))
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(adapterFact)},
	}
	options := adapterOptions("undeclared-package-fact")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "did not declare fact type") {
		t.Fatalf("RunPackages() error = %v, want undeclared fact type", err)
	}
}

func TestAdaptAnalyzerRejectsNondeterministicallyEncodedPackageFact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	analyzer := &goanalysis.Analyzer{
		Name: "nondeterministicfact",
		Doc:  "attempts to export a nondeterministically encoded package fact",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.ExportPackageFact(new(nondeterministicPackageFact))
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(nondeterministicPackageFact)},
	}
	options := adapterOptions("nondeterministic-package-fact")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "encoding is nondeterministic") {
		t.Fatalf("RunPackages() error = %v, want nondeterministic fact refusal", err)
	}
}

func TestAdaptAnalyzerRejectsNilPackageFactImport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	analyzer := &goanalysis.Analyzer{
		Name: "nilpackagefact",
		Doc:  "attempts to import a fact for a nil package",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.ImportPackageFact(nil, new(adapterFact))
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(adapterFact)},
	}
	options := adapterOptions("nil-package-fact")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "package fact import requires a package") {
		t.Fatalf("RunPackages() error = %v, want nil package refusal", err)
	}
}

func TestAdaptAnalyzerStopsPackageFactGraphAfterCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "dep", "dep.go"), "package dep\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "adapter.go"),
		"package adapter\nimport _ \"example.com/adapter/dep\"\n",
	)
	ctx, cancel := context.WithCancel(context.Background())
	rootRuns := 0
	analyzer := &goanalysis.Analyzer{
		Name: "cancelfacts",
		Doc:  "cancels after dependency fact execution",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.ExportPackageFact(new(adapterFact))
			if pass.Pkg.Path() == "example.com/adapter/dep" {
				cancel()
			} else {
				rootRuns++
			}
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(adapterFact)},
	}
	options := adapterOptions("cancel-package-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		ctx,
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if !errors.Is(err, context.Canceled) || rootRuns != 0 {
		t.Fatalf("RunPackages() error = %v, root runs = %d", err, rootRuns)
	}
}

func TestAdaptAnalyzerRejectsIllTypedFactDependencyWithoutOptIn(t *testing.T) {
	t.Parallel()

	producer := &goanalysis.Analyzer{
		Name:      "factproducer",
		Doc:       "produces facts only for well-typed packages",
		Run:       func(*goanalysis.Pass) (any, error) { return nil, nil },
		FactTypes: []goanalysis.Fact{new(adapterFact)},
	}
	analyzer := &goanalysis.Analyzer{
		Name:             "factconsumer",
		Doc:              "requires facts while admitting type errors",
		RunDespiteErrors: true,
		Run:              func(*goanalysis.Pass) (any, error) { return nil, nil },
		Requires:         []*goanalysis.Analyzer{producer},
	}
	options := adapterOptions("ill-typed-fact-dependency")
	options.Metadata.Requirement = rules.RequireTypes
	options.Metadata.RunDespiteTypeErrors = true
	options.ReadOnlyAudited = true
	_, err := analysis.AdaptAnalyzer(analyzer, options)
	if err == nil || !strings.Contains(err.Error(),
		`native type-error policy exceeds analyzer "factproducer" contract`) {
		t.Fatalf("AdaptAnalyzer() error = %v, want prerequisite type-error refusal", err)
	}
}

func TestAdaptAnalyzerCopiesAndScopesObjectFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "dep", "dep.go"),
		"package dep\nfunc Second() {}\nfunc First() {}\nfunc hidden() {}\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "adapter.go"),
		"package adapter\nimport _ \"example.com/adapter/dep\"\n",
	)
	analyzer := &goanalysis.Analyzer{
		Name: "scopedobjectfacts",
		Doc:  "reports copied object facts visible to each package",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Pkg.Path() == "example.com/adapter/dep" {
				for _, name := range []string{"Second", "First", "hidden"} {
					fact := &packageScopeFact{Value: name}
					pass.ExportObjectFact(pass.Pkg.Scope().Lookup(name), fact)
					fact.Value = "mutated"
				}
				pass.ReportRangef(pass.Files[0].Name, "dependency diagnostic")
				return nil, nil
			}
			facts := pass.AllObjectFacts()
			values := make([]string, len(facts))
			for index, item := range facts {
				fact := &packageScopeFact{Value: "dirty destination"}
				if !pass.ImportObjectFact(item.Object, fact) {
					return nil, errors.New("enumerated object fact could not be imported")
				}
				values[index] = fact.Value
			}
			pass.ReportRangef(pass.Files[0].Name, "%s", strings.Join(values, ","))
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(packageScopeFact)},
	}
	options := adapterOptions("scoped-object-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].Message != "Second,First" {
		t.Fatalf("RunPackages() = %#v", result)
	}
}

func TestAdaptAnalyzerOrdersIndistinguishableSyntheticObjectFacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	analyzer := &goanalysis.Analyzer{
		Name: "syntheticobjectfacts",
		Doc:  "reports a deterministic order for synthetic object facts",
		Run: func(pass *goanalysis.Pass) (any, error) {
			signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
			for _, value := range []string{"second", "first"} {
				object := types.NewFunc(token.NoPos, pass.Pkg, "Synthetic", signature)
				pass.ExportObjectFact(object, &packageScopeFact{Value: value})
			}
			facts := pass.AllObjectFacts()
			values := make([]string, len(facts))
			for index, item := range facts {
				values[index] = item.Fact.(*packageScopeFact).Value
			}
			pass.ReportRangef(pass.Files[0].Name, "%s", strings.Join(values, ","))
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(packageScopeFact)},
	}
	options := adapterOptions("synthetic-object-facts")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(analyzer, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].Message != "first,second" {
		t.Fatalf("RunPackages() = %#v", result)
	}
}

func TestAdaptAnalyzerRejectsInvalidObjectFactOperations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "dep", "dep.go"), "package dep\nconst Value = 1\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "adapter.go"),
		"package adapter\nimport _ \"example.com/adapter/dep\"\n",
	)
	tests := []struct {
		name      string
		run       func(*goanalysis.Pass)
		wantError string
	}{
		{
			name: "nil import",
			run: func(pass *goanalysis.Pass) {
				pass.ImportObjectFact(nil, new(adapterFact))
			},
			wantError: "object fact import requires an object",
		},
		{
			name: "nil export",
			run: func(pass *goanalysis.Pass) {
				pass.ExportObjectFact(nil, new(adapterFact))
			},
			wantError: "object fact export requires an object",
		},
		{
			name: "foreign export",
			run: func(pass *goanalysis.Pass) {
				if len(pass.Pkg.Imports()) != 0 {
					pass.ExportObjectFact(
						pass.Pkg.Imports()[0].Scope().Lookup("Value"),
						new(adapterFact),
					)
				}
			},
			wantError: "cannot export object fact",
		},
		{
			name: "undeclared export",
			run: func(pass *goanalysis.Pass) {
				if object := pass.Pkg.Scope().Lookup("Value"); object != nil {
					pass.ExportObjectFact(object, new(packageScopeFact))
				}
			},
			wantError: "did not declare fact type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := adapterOptions("invalid-object-facts-" + strings.ReplaceAll(test.name, " ", "-"))
			options.Metadata.Requirement = rules.RequireTypes
			options.ReadOnlyAudited = true
			adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
				Name: "invalidobjectfacts" + strings.ReplaceAll(test.name, " ", ""),
				Doc:  "attempts an invalid object fact operation",
				Run: func(pass *goanalysis.Pass) (any, error) {
					test.run(pass)
					return nil, nil
				},
				FactTypes: []goanalysis.Fact{new(adapterFact)},
			}, options)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := rules.NewRegistry(adapted)
			if err != nil {
				t.Fatal(err)
			}
			_, err = analysis.RunPackages(
				context.Background(),
				registry,
				analysis.RunOptions{Preset: rules.PresetCorrectness},
				analysis.PackageLoadOptions{
					Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("RunPackages() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestAdaptAnalyzerRejectsTypedPrerequisiteRuntimeViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    any
		report    bool
		wantError string
	}{
		{name: "result type", result: 42, wantError: "returned result type int"},
		{name: "diagnostic", result: "ready", report: true, wantError: "reported diagnostics"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
			writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
			prerequisite := &goanalysis.Analyzer{
				Name: "invalidprerequisite", Doc: "violates the prerequisite contract",
				ResultType: reflect.TypeFor[string](),
				Run: func(pass *goanalysis.Pass) (any, error) {
					if test.report {
						pass.ReportRangef(pass.Files[0].Name, "unexpected prerequisite diagnostic")
					}
					return test.result, nil
				},
			}
			upstream := &goanalysis.Analyzer{
				Name: "invalidroot", Doc: "depends on an invalid prerequisite",
				Requires: []*goanalysis.Analyzer{prerequisite},
				Run:      func(*goanalysis.Pass) (any, error) { return nil, nil },
			}
			options := adapterOptions("invalid-prerequisite")
			options.Metadata.Requirement = rules.RequireTypes
			options.ReadOnlyAudited = true
			adapted, err := analysis.AdaptAnalyzer(upstream, options)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := rules.NewRegistry(adapted)
			if err != nil {
				t.Fatal(err)
			}
			_, err = analysis.RunPackages(
				context.Background(),
				registry,
				analysis.RunOptions{Preset: rules.PresetCorrectness},
				analysis.PackageLoadOptions{
					Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("RunPackages() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestAdaptAnalyzerOwnsEachPhysicalTestVariantSourceOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	paths := []string{
		filepath.Join(root, "adapter.go"),
		filepath.Join(root, "adapter_test.go"),
		filepath.Join(root, "external_test.go"),
	}
	writeTypesFixture(t, paths[0], "package adapter\nconst Value = 1\n")
	writeTypesFixture(t, paths[1], `package adapter

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(m.Run()) }
`)
	writeTypesFixture(t, paths[2], `package adapter_test

import (
	"testing"

	"example.com/adapter"
)

func TestValue(t *testing.T) {
	if adapter.Value != 1 { t.Fatal(adapter.Value) }
}
`)
	upstream := &goanalysis.Analyzer{
		Name: "variantownership",
		Doc:  "reports every package source",
		Run: func(pass *goanalysis.Pass) (any, error) {
			for _, file := range pass.Files {
				pass.ReportRangef(file.Name, "package source")
			}
			return nil, nil
		},
	}
	options := adapterOptions("variant-ownership")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, Tests: true, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	gotPaths := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		if len(file.Diagnostics) != 1 {
			t.Fatalf("variant source diagnostics = %#v", result)
		}
		gotPaths = append(gotPaths, file.Path)
	}
	if !reflect.DeepEqual(gotPaths, paths) {
		t.Fatalf("variant source paths = %#v, want %#v", gotPaths, paths)
	}
}

func TestAdaptAnalyzerTypedPassUsesLoadOwnedPackageData(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	path := filepath.Join(root, "adapter.go")
	writeTypesFixture(t, path, "package adapter\nconst Value = 1\n")
	overlay := []byte("package adapter\nconst Value = 2\n")
	upstream := &goanalysis.Analyzer{
		Name: "typedpass",
		Doc:  "checks the typed package pass",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Pkg == nil || pass.Pkg.Path() != "example.com/adapter" ||
				pass.TypesInfo == nil || pass.TypesSizes == nil || len(pass.Files) != 1 ||
				len(pass.ResultOf) != 0 || pass.Module == nil || pass.Module.Path != "example.com/adapter" ||
				len(pass.TypeErrors) != 0 || len(pass.OtherFiles) != 0 || len(pass.IgnoredFiles) != 0 {
				return nil, errors.New("typed adapter pass omitted or invented package data")
			}
			filename := pass.Fset.PositionFor(pass.Files[0].Pos(), false).Filename
			contents, err := pass.ReadFile(filename)
			if err != nil || string(contents) != string(overlay) {
				return nil, errors.New("typed adapter pass did not expose overlay bytes")
			}
			contents[0] = 'X'
			contents, err = pass.ReadFile(filename)
			if err != nil || string(contents) != string(overlay) {
				return nil, errors.New("typed adapter pass exposed mutable source bytes")
			}
			if _, err := pass.ReadFile(filepath.Join(root, "outside.go")); err == nil {
				return nil, errors.New("typed adapter pass read an undeclared file")
			}
			pass.ReportRangef(pass.Files[0].Name, "typed package")
			return nil, nil
		},
	}
	options := adapterOptions("typed-pass")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
			Overlay: map[string][]byte{path: overlay},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	loadedSource, found := result.Sources.Lookup(path)
	if !found || result.Files[0].Digest != loadedSource.Digest() ||
		result.Files[0].Diagnostics[0].Digest != loadedSource.Digest() {
		t.Fatalf("typed adapter source identity = %#v, found %t", result.Files[0], found)
	}
}

func TestAdaptAnalyzerRunsTypedPackagesAfterNativeSSA(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\nfunc run() {}\n")
	order := make([]string, 0, 2)
	native := ssaRule{
		metadata: ssaMetadata("native-ssa"),
		run: func(*rules.SSAContext) ([]rules.Finding, error) {
			order = append(order, "native")
			return nil, nil
		},
	}
	upstream := &goanalysis.Analyzer{
		Name: "afterssa",
		Doc:  "records package adapter ordering",
		Run: func(*goanalysis.Pass) (any, error) {
			order = append(order, "adapted")
			return nil, nil
		},
	}
	options := adapterOptions("adapted-package")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted, native)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"native", "adapted"}) {
		t.Fatalf("analysis order = %#v", order)
	}
}

func TestAdaptAnalyzerRejectsCrossFilePackageDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "a.go"), "package adapter\nconst A = 1\n")
	writeTypesFixture(t, filepath.Join(root, "b.go"), "package adapter\nconst B = 2\n")
	upstream := &goanalysis.Analyzer{
		Name: "crossfile",
		Doc:  "reports cross-file related information",
		Run: func(pass *goanalysis.Pass) (any, error) {
			files := make(map[string]*ast.File, len(pass.Files))
			for _, file := range pass.Files {
				files[pass.Fset.PositionFor(file.Pos(), false).Filename] = file
			}
			first := files[filepath.Join(root, "a.go")]
			second := files[filepath.Join(root, "b.go")]
			pass.Report(goanalysis.Diagnostic{
				Pos: first.Name.Pos(), End: first.Name.End(), Message: "cross-file diagnostic",
				Related: []goanalysis.RelatedInformation{{
					Pos: second.Name.Pos(), End: second.Name.End(), Message: "other file",
				}},
			})
			return nil, nil
		},
	}
	options := adapterOptions("cross-file")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "related range 0 belongs to another source file") {
		t.Fatalf("RunPackages() error = %v", err)
	}
}

func TestAdaptAnalyzerRejectsCrossFilePackageFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "a.go"), "package adapter\nconst A = 1\n")
	writeTypesFixture(t, filepath.Join(root, "b.go"), "package adapter\nconst B = 2\n")
	upstream := &goanalysis.Analyzer{
		Name: "crossfilefix",
		Doc:  "offers a cross-file package fix",
		Run: func(pass *goanalysis.Pass) (any, error) {
			files := make(map[string]*ast.File, len(pass.Files))
			for _, file := range pass.Files {
				files[pass.Fset.PositionFor(file.Pos(), false).Filename] = file
			}
			first := files[filepath.Join(root, "a.go")]
			second := files[filepath.Join(root, "b.go")]
			pass.Report(goanalysis.Diagnostic{
				Pos: first.Name.Pos(), End: first.Name.End(), Message: "cross-file fix",
				SuggestedFixes: []goanalysis.SuggestedFix{{
					Message: "Replace other file",
					TextEdits: []goanalysis.TextEdit{{
						Pos: second.Name.Pos(), End: second.Name.End(), NewText: []byte("changed"),
					}},
				}},
			})
			return nil, nil
		},
	}
	options := adapterOptions("cross-file-fix")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
		Message: "Replace other file", Name: "replace-other", Description: "replace other file",
	}}
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "edit 0 belongs to another source file") {
		t.Fatalf("RunPackages() error = %v", err)
	}
}

func TestAdaptAnalyzerHonorsGeneratedPackagePolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "generated.go"),
		"// Code generated by test. DO NOT EDIT.\npackage adapter\n",
	)
	var skippedRuns, enabledRuns int
	newAdapter := func(id string, runOnGenerated bool, runs *int) rules.Rule {
		upstream := &goanalysis.Analyzer{
			Name: strings.ReplaceAll(id, "-", ""),
			Doc:  "records generated package scheduling",
			Run: func(pass *goanalysis.Pass) (any, error) {
				(*runs)++
				pass.ReportRangef(pass.Files[0].Name, "generated package")
				return nil, nil
			},
		}
		options := adapterOptions(id)
		options.Metadata.Requirement = rules.RequireTypes
		options.Metadata.RunOnGenerated = runOnGenerated
		options.ReadOnlyAudited = true
		adapted, err := analysis.AdaptAnalyzer(upstream, options)
		if err != nil {
			t.Fatal(err)
		}
		return adapted
	}
	registry, err := rules.NewRegistry(
		newAdapter("generated-package-disabled", false, &skippedRuns),
		newAdapter("generated-package-enabled", true, &enabledRuns),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if skippedRuns != 0 || enabledRuns != 1 || len(result.Files) != 1 ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "generated-package-enabled" {
		t.Fatalf(
			"generated package result = disabled %d, enabled %d, %#v",
			skippedRuns,
			enabledRuns,
			result,
		)
	}
}

func TestAdaptAnalyzerHonorsTypedPackageErrorPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\nvar Value = missing\n")
	var skippedRuns, enabledRuns int
	newAdapter := func(id string, runDespiteErrors bool, runs *int) rules.Rule {
		upstream := &goanalysis.Analyzer{
			Name:             strings.ReplaceAll(id, "-", ""),
			Doc:              "records ill-typed package scheduling",
			RunDespiteErrors: runDespiteErrors,
			Run: func(pass *goanalysis.Pass) (any, error) {
				(*runs)++
				if len(pass.TypeErrors) == 0 {
					return nil, errors.New("ill-typed adapter pass omitted type errors")
				}
				pass.ReportRangef(pass.Files[0].Name, "ill-typed package")
				return nil, nil
			},
		}
		options := adapterOptions(id)
		options.Metadata.Requirement = rules.RequireTypes
		options.Metadata.RunDespiteTypeErrors = runDespiteErrors
		options.ReadOnlyAudited = true
		adapted, err := analysis.AdaptAnalyzer(upstream, options)
		if err != nil {
			t.Fatal(err)
		}
		return adapted
	}
	registry, err := rules.NewRegistry(
		newAdapter("typed-errors-disabled", false, &skippedRuns),
		newAdapter("typed-errors-enabled", true, &enabledRuns),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if skippedRuns != 0 || enabledRuns != 1 || len(result.LoadDiagnostics) == 0 ||
		len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "typed-errors-enabled" {
		t.Fatalf(
			"ill-typed package result = disabled %d, enabled %d, %#v",
			skippedRuns,
			enabledRuns,
			result,
		)
	}
}

func TestAdaptAnalyzerPreservesCancellationAfterTypedPackageRun(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	writeTypesFixture(t, filepath.Join(root, "adapter.go"), "package adapter\n")
	ctx, cancel := context.WithCancel(context.Background())
	upstream := &goanalysis.Analyzer{
		Name: "typedcancel",
		Doc:  "cancels after typed package execution",
		Run: func(pass *goanalysis.Pass) (any, error) {
			cancel()
			pass.ReportRangef(pass.Files[0].Name, "diagnostic after cancellation")
			return nil, nil
		},
	}
	options := adapterOptions("typed-cancel")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = analysis.RunPackages(
		ctx,
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunPackages() error = %v, want context cancellation", err)
	}
}

func TestAdaptAnalyzerMapsTypedPackageSuggestedFixes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(t, filepath.Join(root, "go.mod"), "module example.com/adapter\n\ngo 1.26.0\n")
	path := filepath.Join(root, "adapter.go")
	input := "package adapter\nconst Value = 1\n"
	writeTypesFixture(t, path, input)
	upstream := &goanalysis.Analyzer{
		Name: "typedfix",
		Doc:  "offers a same-file typed package fix",
		Run: func(pass *goanalysis.Pass) (any, error) {
			declaration := pass.Files[0].Decls[0].(*ast.GenDecl)
			value := declaration.Specs[0].(*ast.ValueSpec).Values[0]
			pass.Report(goanalysis.Diagnostic{
				Pos: value.Pos(), End: value.End(), Message: "replace the value",
				SuggestedFixes: []goanalysis.SuggestedFix{{
					Message: "Replace value",
					TextEdits: []goanalysis.TextEdit{{
						Pos: value.Pos(), End: value.End(), NewText: []byte("2"),
					}},
				}},
			})
			return nil, nil
		},
	}
	options := adapterOptions("typed-fix")
	options.Metadata.Requirement = rules.RequireTypes
	options.ReadOnlyAudited = true
	options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
		Message: "Replace value", Name: "replace-value", Description: "replace the value",
		Safety: rules.FixSafe, Audited: true,
	}}
	adapted, err := analysis.AdaptAnalyzer(upstream, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root, Patterns: []string{"."}, ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	start := strings.LastIndex(input, "1")
	if diagnostic.Path != path || len(diagnostic.Fixes) != 1 ||
		diagnostic.Fixes[0].Name != "replace-value" ||
		diagnostic.Fixes[0].Safety != rules.FixSafe ||
		len(diagnostic.Fixes[0].Edits) != 1 ||
		diagnostic.Fixes[0].Edits[0] != (rules.Edit{
			Range: source.Range{Start: start, End: start + 1}, NewText: "2",
		}) {
		t.Fatalf("typed package fix = %#v", diagnostic)
	}
}

func TestAdaptAnalyzerMapsExplicitlyAuditedSafeFixes(t *testing.T) {
	t.Parallel()

	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "safe",
		Doc:  "offers an audited safe fix",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.Report(goanalysis.Diagnostic{
				Pos:     pass.Files[0].Name.Pos(),
				End:     pass.Files[0].Name.End(),
				Message: "package name spelling can be preserved",
				SuggestedFixes: []goanalysis.SuggestedFix{{
					Message: "Preserve package name",
					TextEdits: []goanalysis.TextEdit{{
						Pos: pass.Files[0].Name.Pos(), End: pass.Files[0].Name.End(), NewText: []byte("sample"),
					}},
				}},
			})
			return nil, nil
		},
	}, analysis.AnalyzerAdapterOptions{
		Metadata: analysisMetadata("safe-analyzer", rules.NodeFile, false),
		SuggestedFixes: []analysis.AnalyzerFixMapping{{
			Message: "Preserve package name", Name: "preserve-package",
			Description: "preserve the package name spelling", Safety: rules.FixSafe, Audited: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := adapted.Metadata()
	if len(metadata.Fixes) != 1 || metadata.Fixes[0].Safety != rules.FixSafe {
		t.Fatalf("adapted fix metadata = %#v", metadata.Fixes)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || len(result.Diagnostics[0].Fixes) != 1 ||
		result.Diagnostics[0].Fixes[0].Safety != rules.FixSafe {
		t.Fatalf("adapted diagnostics = %#v", result.Diagnostics)
	}
}

func TestAdaptAnalyzerIsolatesAnalyzerMutationsFromOtherAdapters(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\nfunc run() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	mutator, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "mutator",
		Doc:  "mutates its isolated syntax view",
		Run: func(pass *goanalysis.Pass) (any, error) {
			pass.Files[0].Name.Name = "changed"
			return nil, nil
		},
	}, analysis.AnalyzerAdapterOptions{Metadata: analysisMetadata("a-mutator", rules.NodeFile, false)})
	if err != nil {
		t.Fatal(err)
	}
	observer, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "observer",
		Doc:  "observes the original isolated syntax view",
		Run: func(pass *goanalysis.Pass) (any, error) {
			if pass.Files[0].Name.Name != "sample" {
				return nil, errors.New("another analyzer mutated the syntax view")
			}
			pass.ReportRangef(pass.Files[0].Name, "package name remained isolated")
			return nil, nil
		},
	}, analysis.AnalyzerAdapterOptions{Metadata: analysisMetadata("b-observer", rules.NodeFile, false)})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(observer, mutator)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleID != "b-observer" {
		t.Fatalf("isolated adapter diagnostics = %#v", result.Diagnostics)
	}
}

func TestAdaptAnalyzerIsolatesAnalyzerDescriptorMutationsBetweenRuns(t *testing.T) {
	t.Parallel()

	runs := 0
	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "descriptor",
		Doc:  "mutates the pass analyzer descriptor",
		Run: func(pass *goanalysis.Pass) (any, error) {
			runs++
			pass.Analyzer.Run = func(*goanalysis.Pass) (any, error) {
				return nil, errors.New("mutated analyzer descriptor escaped its run")
			}
			return nil, nil
		},
	}, adapterOptions("descriptor-analyzer"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
			Preset: rules.PresetCorrectness,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if runs != 2 {
		t.Fatalf("original analyzer runs = %d, want 2", runs)
	}
}

func TestAdaptAnalyzerHonorsGeneratedFilePolicy(t *testing.T) {
	t.Parallel()

	var skippedRuns, enabledRuns int
	newAdapter := func(id string, generated bool, runs *int) rules.Rule {
		adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
			Name: strings.ReplaceAll(id, "-", ""),
			Doc:  "records generated-file scheduling",
			Run: func(*goanalysis.Pass) (any, error) {
				(*runs)++
				return nil, nil
			},
		}, analysis.AnalyzerAdapterOptions{
			Metadata: analysisMetadata(id, rules.NodeFile, generated),
		})
		if err != nil {
			t.Fatal(err)
		}
		return adapted
	}
	registry, err := rules.NewRegistry(
		newAdapter("generated-disabled", false, &skippedRuns),
		newAdapter("generated-enabled", true, &enabledRuns),
	)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load(
		"/project/generated.go",
		[]byte("// Code generated by test. DO NOT EDIT.\npackage generated\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	}); err != nil {
		t.Fatal(err)
	}
	if skippedRuns != 0 || enabledRuns != 1 {
		t.Fatalf("generated analyzer runs = disabled %d, enabled %d", skippedRuns, enabledRuns)
	}
}

func TestAdaptAnalyzerDiagnosticsShareNativeDeterministicOrdering(t *testing.T) {
	t.Parallel()

	file, err := source.Load("/project/source.go", []byte("package sample\nfunc run() { target() }\n"))
	if err != nil {
		t.Fatal(err)
	}
	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "adapted",
		Doc:  "reports the target call",
		Run: func(pass *goanalysis.Pass) (any, error) {
			ast.Inspect(pass.Files[0], func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					pass.ReportRangef(call, "adapted diagnostic")
				}
				return true
			})
			return nil, nil
		},
	}, adapterOptions("z-adapted"))
	if err != nil {
		t.Fatal(err)
	}
	native := syntaxRule{
		metadata: analysisMetadata("a-native", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			sourceRange, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{{
				MessageKey: "native", Message: "native diagnostic", Range: sourceRange,
			}}, nil
		},
	}
	registry, err := rules.NewRegistry(adapted, native)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("mixed diagnostics = %#v", result.Diagnostics)
	}
	if got, want := []string{result.Diagnostics[0].RuleID, result.Diagnostics[1].RuleID},
		[]string{"a-native", "z-adapted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed diagnostic order = %#v, want %#v", got, want)
	}
}

func TestAdaptAnalyzerPreservesCancellationObservedDuringAnalyzerRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
		Name: "canceling",
		Doc:  "observes cancellation after analyzer execution",
		Run: func(pass *goanalysis.Pass) (any, error) {
			cancel()
			pass.ReportRangef(pass.Files[0].Name, "diagnostic after cancellation")
			return nil, nil
		},
	}, adapterOptions("canceling-analyzer"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("/project/source.go", []byte("package sample\n"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := analysis.Run(ctx, file, registry, analysis.RunOptions{
		Preset: rules.PresetCorrectness,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
}

func TestAdaptAnalyzerRejectsUnsupportedAnalyzerContracts(t *testing.T) {
	t.Parallel()

	validRun := func(*goanalysis.Pass) (any, error) { return nil, nil }
	prerequisite := &goanalysis.Analyzer{Name: "prerequisite", Doc: "prerequisite", Run: validRun}
	flagged := &goanalysis.Analyzer{Name: "flagged", Doc: "flagged", Run: validRun}
	flagged.Flags.Init("flagged", flag.ContinueOnError)
	flagged.Flags.Bool("enabled", false, "enable analysis")
	tests := []struct {
		name      string
		analyzer  *goanalysis.Analyzer
		options   analysis.AnalyzerAdapterOptions
		wantError string
	}{
		{name: "nil", analyzer: nil, options: adapterOptions("nil-analyzer"), wantError: "nil analyzer"},
		{
			name: "prerequisites",
			analyzer: &goanalysis.Analyzer{
				Name: "requires", Doc: "requires another analyzer", Run: validRun,
				Requires: []*goanalysis.Analyzer{prerequisite},
			},
			options: adapterOptions("requires-analyzer"), wantError: "prerequisite",
		},
		{
			name: "typed prerequisite flags",
			analyzer: &goanalysis.Analyzer{
				Name: "requiresflags", Doc: "requires flags", Run: validRun,
				Requires: []*goanalysis.Analyzer{flagged},
			},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("requires-flags")
				options.Metadata.Requirement = rules.RequireTypes
				options.ReadOnlyAudited = true
				return options
			}(),
			wantError: "flagged\" flags",
		},
		{
			name: "facts",
			analyzer: &goanalysis.Analyzer{
				Name: "facts", Doc: "uses facts", Run: validRun, FactTypes: []goanalysis.Fact{new(adapterFact)},
			},
			options: adapterOptions("facts-analyzer"), wantError: "facts",
		},
		{
			name: "result",
			analyzer: &goanalysis.Analyzer{
				Name: "result", Doc: "returns a result", Run: validRun, ResultType: reflect.TypeFor[string](),
			},
			options: adapterOptions("result-analyzer"), wantError: "result",
		},
		{name: "flags", analyzer: flagged, options: adapterOptions("flagged-analyzer"), wantError: "flags"},
		{
			name:     "unbound native options",
			analyzer: &goanalysis.Analyzer{Name: "options", Doc: "declares no flags", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("option-analyzer")
				options.Metadata.Options = []rules.OptionMetadata{{
					Name: "enabled", Summary: "enable analysis", Kind: rules.OptionBoolean, Required: true,
				}}
				return options
			}(),
			wantError: "options require analyzer flag bindings",
		},
		{
			name:     "predeclared native fixes",
			analyzer: &goanalysis.Analyzer{Name: "fixes", Doc: "declares fixes twice", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("fix-metadata")
				options.Metadata.Fixes = []rules.FixMetadata{{
					Name: "rewrite", Description: "rewrite source", Safety: rules.FixSuggestion,
				}}
				return options
			}(),
			wantError: "fix metadata",
		},
		{
			name:     "unaudited safe fix",
			analyzer: &goanalysis.Analyzer{Name: "safe", Doc: "offers a safe fix", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("safe-analyzer")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
					Message: "Rewrite", Name: "rewrite", Description: "rewrite source", Safety: rules.FixSafe,
				}}
				return options
			}(),
			wantError: "audit",
		},
		{
			name:     "audit on suggestion",
			analyzer: &goanalysis.Analyzer{Name: "suggestion", Doc: "offers a suggestion", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("suggestion-analyzer")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
					Message: "Rewrite", Name: "rewrite", Description: "rewrite source", Audited: true,
				}}
				return options
			}(),
			wantError: "audit applies only",
		},
		{
			name:     "incomplete fix mapping",
			analyzer: &goanalysis.Analyzer{Name: "incomplete", Doc: "offers an incomplete fix", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("incomplete-analyzer")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{Message: "Rewrite"}}
				return options
			}(),
			wantError: "incomplete",
		},
		{
			name:     "duplicate fix message",
			analyzer: &goanalysis.Analyzer{Name: "duplicatemessage", Doc: "duplicates a fix message", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("duplicate-message")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{
					{Message: "Rewrite", Name: "rewrite-one", Description: "rewrite source one"},
					{Message: "Rewrite", Name: "rewrite-two", Description: "rewrite source two"},
				}
				return options
			}(),
			wantError: "duplicate suggested-fix message",
		},
		{
			name:     "duplicate native fix name",
			analyzer: &goanalysis.Analyzer{Name: "duplicatename", Doc: "duplicates a native fix name", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("duplicate-name")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{
					{Message: "Rewrite one", Name: "rewrite", Description: "rewrite source one"},
					{Message: "Rewrite two", Name: "rewrite", Description: "rewrite source two"},
				}
				return options
			}(),
			wantError: "duplicate native fix name",
		},
		{
			name:     "invalid fix safety",
			analyzer: &goanalysis.Analyzer{Name: "invalidsafety", Doc: "offers an invalid fix safety", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("invalid-safety")
				options.SuggestedFixes = []analysis.AnalyzerFixMapping{{
					Message: "Rewrite", Name: "rewrite", Description: "rewrite source", Safety: "trusted",
				}}
				return options
			}(),
			wantError: "invalid fix safety",
		},
		{
			name:     "typed metadata without read-only audit",
			analyzer: &goanalysis.Analyzer{Name: "typed", Doc: "declares typed metadata", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("typed-analyzer")
				options.Metadata.Requirement = rules.RequireTypes
				return options
			}(),
			wantError: "read-only analyzer audit",
		},
		{
			name: "typed metadata exceeds type-error contract",
			analyzer: &goanalysis.Analyzer{
				Name: "typederrors", Doc: "rejects type errors", Run: validRun,
			},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("typed-errors")
				options.Metadata.Requirement = rules.RequireTypes
				options.Metadata.RunDespiteTypeErrors = true
				options.ReadOnlyAudited = true
				return options
			}(),
			wantError: "type-error policy exceeds",
		},
		{
			name: "typed prerequisite rejects type errors",
			analyzer: &goanalysis.Analyzer{
				Name: "typedrooterrors", Doc: "runs despite type errors", Run: validRun,
				RunDespiteErrors: true,
				Requires: []*goanalysis.Analyzer{{
					Name: "typedprerequisiteerrors", Doc: "rejects type errors", Run: validRun,
				}},
			},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("typed-root-errors")
				options.Metadata.Requirement = rules.RequireTypes
				options.Metadata.RunDespiteTypeErrors = true
				options.ReadOnlyAudited = true
				return options
			}(),
			wantError: "typedprerequisiteerrors\" contract",
		},
		{
			name:     "unsupported tier metadata",
			analyzer: &goanalysis.Analyzer{Name: "ssa", Doc: "declares SSA metadata", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("ssa-analyzer")
				options.Metadata.Requirement = rules.RequireSSA
				return options
			}(),
			wantError: "syntax or types requirement",
		},
		{
			name:     "node metadata",
			analyzer: &goanalysis.Analyzer{Name: "node", Doc: "declares node metadata", Run: validRun},
			options: func() analysis.AnalyzerAdapterOptions {
				options := adapterOptions("node-analyzer")
				options.Metadata.NodeInterests = []rules.NodeKind{rules.NodeCallExpr}
				return options
			}(),
			wantError: "only file interest",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := analysis.AdaptAnalyzer(test.analyzer, test.options); err == nil ||
				!strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("AdaptAnalyzer() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestAdaptAnalyzerRejectsUnsupportedRuntimeResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		analyzerURL string
		run         func(*goanalysis.Pass) (any, error)
		wantError   string
	}{
		{
			name: "panic",
			run: func(*goanalysis.Pass) (any, error) {
				panic("boom")
			},
			wantError: "panicked",
		},
		{
			name: "undeclared suggested fix",
			run: func(pass *goanalysis.Pass) (any, error) {
				pass.Report(goanalysis.Diagnostic{
					Pos: pass.Files[0].Name.Pos(), Message: "problem",
					SuggestedFixes: []goanalysis.SuggestedFix{{Message: "Unknown fix"}},
				})
				return nil, nil
			},
			wantError: "undeclared suggested fix",
		},
		{
			name: "foreign position",
			run: func(pass *goanalysis.Pass) (any, error) {
				other := pass.Fset.AddFile("other.go", -1, 4)
				other.SetLinesForContent([]byte("bad\n"))
				pass.Reportf(other.Pos(0), "foreign diagnostic")
				return nil, nil
			},
			wantError: "outside the adapted source",
		},
		{
			name: "unexpected result",
			run: func(*goanalysis.Pass) (any, error) {
				return "result", nil
			},
			wantError: "unexpected result",
		},
		{
			name:        "invalid analyzer URL",
			analyzerURL: ":not a URL",
			run: func(pass *goanalysis.Pass) (any, error) {
				pass.Report(goanalysis.Diagnostic{
					Pos: pass.Files[0].Name.Pos(), Message: "problem", URL: "#relative",
				})
				return nil, nil
			},
			wantError: "invalid analyzer URL",
		},
		{
			name:        "invalid diagnostic URL",
			analyzerURL: "https://example.test/analyzer",
			run: func(pass *goanalysis.Pass) (any, error) {
				pass.Report(goanalysis.Diagnostic{
					Pos: pass.Files[0].Name.Pos(), Message: "problem", URL: ":not a URL",
				})
				return nil, nil
			},
			wantError: "invalid diagnostic URL",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapted, err := analysis.AdaptAnalyzer(&goanalysis.Analyzer{
				Name: "runtime", Doc: "exercises runtime validation", URL: test.analyzerURL, Run: test.run,
			}, adapterOptions("runtime-analyzer"))
			if err != nil {
				t.Fatal(err)
			}
			registry, err := rules.NewRegistry(adapted)
			if err != nil {
				t.Fatal(err)
			}
			file, err := source.Load("/project/source.go", []byte("package sample\n"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := analysis.Run(context.Background(), file, registry, analysis.RunOptions{
				Preset: rules.PresetCorrectness,
			}); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func adapterOptions(id string) analysis.AnalyzerAdapterOptions {
	return analysis.AnalyzerAdapterOptions{Metadata: analysisMetadata(id, rules.NodeFile, false)}
}
