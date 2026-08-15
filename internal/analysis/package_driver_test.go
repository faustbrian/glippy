package analysis_test

import (
	"context"
	"errors"
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestRunPackagesFiltersAndReseversDiagnosticsByPathPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	productionPath := filepath.Join(root, "project.go")
	testPath := filepath.Join(root, "project_test.go")
	writeTypesFixture(t, productionPath, "package project\nfunc production() {}\n")
	writeTypesFixture(t, testPath, "package project\nfunc testHelper() {}\n")
	rule := typesRule{
		metadata: typesMetadata("file-rule", rules.NodeFile),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			range_, rangeErr := ctx.Range(node.(*ast.File).Name)
			if rangeErr != nil {
				return nil, rangeErr
			}
			return []rules.Finding{
				{
					MessageKey: "file",
					Message: "file requires review",
					Range: range_,
				},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"file-rule": rules.SeverityWarn},
			PathRoot: root,
			PathOverrides: []config.LintOverride{
				{
					Paths: []string{"project.go"},
					Rules: map[string]config.Severity{
						"file-rule": config.SeverityError,
					},
				},
				{
					Paths: []string{"**/*_test.go"},
					Rules: map[string]config.Severity{
						"file-rule": config.SeverityOff,
					},
				},
			},
		},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}, Tests: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]analysis.Result, len(result.Files))
	for _, file := range result.Files {
		byPath[file.Path] = file
	}
	production := byPath[productionPath]
	if len(production.Diagnostics) != 1 ||
		production.Diagnostics[0].Severity != rules.SeverityError ||
		len(production.Selection) != 1 ||
		production.Selection[0].Severity != rules.SeverityError {
		t.Fatalf("production result = %#v", production)
	}
	test := byPath[testPath]
	if len(test.Diagnostics) != 0 || len(test.Selection) != 0 {
		t.Fatalf("test result = %#v, want path-disabled rule", test)
	}
}

func TestRunPackagesRejectsOversizedOverlayBeforeCloningOrCachePlanning(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates one source-size boundary buffer")
	}
	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "project.go")
	writeTypesFixture(t, path, "package project\n")
	registry, err := rules.NewRegistry(
		typesRule{
			metadata: typesMetadata("typed-overlay", rules.NodeFile),
			run: func(*rules.TypesContext, ast.Node) ([]rules.Finding, error) {
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Preset: rules.PresetCorrectness,
			Cache: &analysis.PackageCacheOptions{},
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Overlay: map[string][]byte{path: make([]byte, source.MaxFileSize + 1)},
		},
	)
	if len(result.Files) != 0 || !errors.Is(err, source.ErrTooLarge) {
		t.Fatalf(
			"RunPackages() returned files=%d, error=%v, want ErrTooLarge before cache validation",
			len(result.Files),
			err,
		)
	}
}

func TestRunPackagesCombinesSyntaxAndTypesBeforeSuppressing(t *testing.T) {
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
		`//glippy:ignore-file cfg-function -- reviewed file
//glippy:ignore-file ssa-function -- reviewed file
package project

//glippy:ignore typed-call -- reviewed here
func suppressed() { target() }
func visible() { target() }
func target() {}
`,
	)

	syntax := syntaxRule{
		metadata: analysisMetadata("syntax-call", rules.NodeCallExpr, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "syntax-call", Message: "syntax call", Range: range_},
			}, nil
		},
	}
	typed := typesRule{
		metadata: typesMetadata("typed-call", rules.NodeCallExpr),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			call := node.(*ast.CallExpr)
			if ctx.Info().TypeOf(call) == nil {
				t.Fatal("typed runner did not share type information")
			}
			range_, err := ctx.Range(call)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "typed-call", Message: "typed call", Range: range_},
			}, nil
		},
	}
	controlFlow := controlFlowRule{
		metadata: controlFlowMetadata("cfg-function"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{
					MessageKey: "cfg-function",
					Message: "cfg function",
					Range: range_,
				},
			}, nil
		},
	}
	ssaRule := ssaRule{
		metadata: ssaMetadata("ssa-function"),
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Syntax())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{
					MessageKey: "ssa-function",
					Message: "SSA function",
					Range: range_,
				},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(typed, syntax, controlFlow, ssaRule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSelection := []rules.Selection{
		{
			ID: "cfg-function",
			Severity: rules.SeverityWarn,
			Requirement: rules.RequireControlFlow,
		},
		{ID: "ssa-function", Severity: rules.SeverityWarn, Requirement: rules.RequireSSA},
		{ID: "syntax-call", Severity: rules.SeverityWarn, Requirement: rules.RequireSyntax},
		{ID: "typed-call", Severity: rules.SeverityWarn, Requirement: rules.RequireTypes},
	}
	if result.Requirement != rules.RequireSSA ||
		!reflect.DeepEqual(result.Selection, wantSelection) {
		t.Fatalf("RunPackages() plan = %s, %#v", result.Requirement, result.Selection)
	}
	if len(result.LoadDiagnostics) != 0 || len(result.Files) != 1 {
		t.Fatalf("RunPackages() = %#v", result)
	}
	captured, found := result.Sources.Lookup(path)
	if !found || captured.Digest() != result.Files[0].Digest {
		t.Fatalf("RunPackages() sources = %#v, %t", captured, found)
	}
	fileResult := result.Files[0]
	if fileResult.Path != path ||
		fileResult.Requirement != rules.RequireSSA ||
		!reflect.DeepEqual(fileResult.Selection, wantSelection) {
		t.Fatalf("RunPackages() file = %#v", fileResult)
	}
	if len(fileResult.Diagnostics) != 3 ||
		fileResult.Diagnostics[0].RuleID != "syntax-call" ||
		fileResult.Diagnostics[1].RuleID != "syntax-call" ||
		fileResult.Diagnostics[2].RuleID != "typed-call" {
		t.Fatalf("RunPackages() diagnostics = %#v", fileResult.Diagnostics)
	}
	if len(fileResult.Suppressed) != 7 ||
		fileResult.Suppressed[0].Diagnostic.RuleID != "cfg-function" ||
		fileResult.Suppressed[1].Diagnostic.RuleID != "ssa-function" ||
		fileResult.Suppressed[2].Diagnostic.RuleID != "typed-call" ||
		fileResult.Suppressed[3].Diagnostic.RuleID != "cfg-function" ||
		fileResult.Suppressed[4].Diagnostic.RuleID != "ssa-function" ||
		fileResult.Suppressed[5].Diagnostic.RuleID != "cfg-function" ||
		fileResult.Suppressed[6].Diagnostic.RuleID != "ssa-function" ||
		len(fileResult.UnusedSuppressions) != 0 ||
		len(fileResult.SuppressionProblems) != 0 {
		t.Fatalf("RunPackages() suppressions = %#v", fileResult)
	}
}

func TestRunPackagesComposesPresetGroupsAndEscalatesWarnings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/presets\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "presets.go"),
		"package presets\nfunc run(){target()}\nfunc target(){}\n",
	)
	metadata := typesMetadata("typed-suspicious", rules.NodeCallExpr)
	metadata.Presets = []rules.Preset{rules.PresetSuspicious}
	registry, err := rules.NewRegistry(
		typesRule{
			metadata: metadata,
			run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
				range_, rangeErr := ctx.Range(node)
				if rangeErr != nil {
					return nil, rangeErr
				}
				return []rules.Finding{
					{
						MessageKey: "typed-suspicious",
						Message: "typed suspicious call",
						Range: range_,
					},
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{rules.PresetCorrectness, rules.PresetSuspicious},
			WarningsAsErrors: true,
		},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Selection) != 1 ||
		result.Selection[0].ID != "typed-suspicious" ||
		result.Selection[0].Severity != rules.SeverityError ||
		len(result.Files) != 1 ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].Severity != rules.SeverityError {
		t.Fatalf("RunPackages() = %#v", result)
	}
}

func TestRunPackagesRoutesTypedOptionsAcrossEveryTier(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/options\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "options.go"),
		"package options\nfunc run(){target()}\nfunc target(){}\n",
	)
	optionMetadata := []rules.OptionMetadata{
		{
			Name: "enabled",
			Summary: "enable the rule",
			Kind: rules.OptionBoolean,
			Required: true,
		},
	}
	assertEnabled := func(name string, enabled bool, found bool) {
		t.Helper()
		if !found || !enabled {
			t.Fatalf("%s option enabled = %t, %t", name, enabled, found)
		}
	}
	visits := make(map[string]int)
	syntaxMetadata := analysisMetadata("syntax-options", rules.NodeCallExpr, false)
	syntaxMetadata.Options = optionMetadata
	typedMetadata := typesMetadata("types-options", rules.NodeCallExpr)
	typedMetadata.Options = optionMetadata
	cfgMetadata := controlFlowMetadata("cfg-options")
	cfgMetadata.Options = optionMetadata
	ssaMetadata := ssaMetadata("ssa-options")
	ssaMetadata.Options = optionMetadata
	registry, err := rules.NewRegistry(
		syntaxRule{
			metadata: syntaxMetadata,
			run: func(ctx *rules.Context, _ ast.Node) ([]rules.Finding, error) {
				enabled, found := ctx.BooleanOption("enabled")
				assertEnabled("syntax", enabled, found)
				visits["syntax"]++
				return nil, nil
			},
		},
		typesRule{
			metadata: typedMetadata,
			run: func(ctx *rules.TypesContext, _ ast.Node) ([]rules.Finding, error) {
				enabled, found := ctx.BooleanOption("enabled")
				assertEnabled("types", enabled, found)
				visits["types"]++
				return nil, nil
			},
		},
		controlFlowRule{
			metadata: cfgMetadata,
			run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
				enabled, found := ctx.BooleanOption("enabled")
				assertEnabled("CFG", enabled, found)
				visits["cfg"]++
				return nil, nil
			},
		},
		ssaRule{
			metadata: ssaMetadata,
			run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
				enabled, found := ctx.BooleanOption("enabled")
				assertEnabled("SSA", enabled, found)
				visits["ssa"]++
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	options := make(map[string]rules.OptionSet)
	for _, id := range
		[]string{"syntax-options", "types-options", "cfg-options", "ssa-options"} {
		options[id] = rules.NewOptionSet(
			map[string]rules.OptionValue{"enabled": rules.BooleanOption(true)},
		)
	}
	if _, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness, RuleOptions: options},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	);
		err != nil {
		t.Fatal(err)
	}
	for _, tier := range []string{"syntax", "types", "cfg", "ssa"} {
		if visits[tier] == 0 {
			t.Fatalf("%s rule was not visited", tier)
		}
	}
}

func TestRunPackagesCachesNativeTypedTiersAcrossIndependentLoads(t *testing.T) {
	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/nativecache\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "native.go")
	writeTypesFixture(t, path, "package nativecache\nfunc run(){target()}\nfunc target(){}\n")

	runs := map[string]int{}
	packageRule := packageRule{
		metadata: packageMetadata("cached-package"),
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			runs["package"]++
			for _, file := range ctx.Files() {
				if !file.Target() {
					continue
				}
				range_, err := file.Range(file.Syntax().Name)
				if err != nil {
					return nil, err
				}
				return []rules.PackageFinding{
					{
						File: file,
						Finding: rules.Finding{
							MessageKey: "package",
							Message: "package",
							Range: range_,
						},
					},
				}, nil
			}
			return nil, nil
		},
	}
	typedMetadata := typesMetadata("cached-types", rules.NodeCallExpr)
	typedMetadata.Fixes = []rules.FixMetadata{
		{
			Name: "cached-rewrite",
			Description: "rewrite cached source",
			Safety: rules.FixSafe,
		},
	}
	typedRule := typesRule{
		metadata: typedMetadata,
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			runs["types"]++
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{
					MessageKey: "types",
					Message: "types",
					Range: range_,
					WithheldFixes: []rules.WithheldFix{
						{
							Name: "cached-rewrite",
							Reason: rules.FixWithheldComments,
							Message: "cached rewrite would remove comments",
						},
					},
				},
			}, nil
		},
	}
	controlFlowRule := controlFlowRule{
		metadata: controlFlowMetadata("cached-cfg"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			runs["cfg"]++
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "cfg", Message: "cfg", Range: range_},
			}, nil
		},
	}
	ssaRule := ssaRule{
		metadata: ssaMetadata("cached-ssa"),
		run: func(ctx *rules.SSAContext) ([]rules.Finding, error) {
			runs["ssa"]++
			range_, err := ctx.Range(ctx.Syntax())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "ssa", Message: "ssa", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(packageRule, typedRule, controlFlowRule, ssaRule)
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	runOptions := analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache: packageAnalyzerCacheOptions(store),
	}
	loadOptions := packageAnalyzerCacheLoadOptions(root)

	first, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	coldRuns := maps.Clone(runs)
	for _, tier := range []string{"package", "types", "cfg", "ssa"} {
		if coldRuns[tier] == 0 {
			t.Fatalf("cold %s callbacks = %d", tier, coldRuns[tier])
		}
	}
	second, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runs, coldRuns) || !reflect.DeepEqual(second.Files, first.Files) {
		t.Fatalf(
			"warm native callbacks = %#v, want %#v; result = %#v",
			runs,
			coldRuns,
			second,
		)
	}

	corruptPackageAnalyzerCache(t, cacheRoot)
	recovered, err := analysis.RunPackages(
		context.Background(),
		registry,
		runOptions,
		loadOptions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(runs, coldRuns) || !reflect.DeepEqual(recovered.Files, first.Files) {
		t.Fatalf("recovered native callbacks = %#v; result = %#v", runs, recovered)
	}
	recoveredRuns := maps.Clone(runs)
	if _, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions);
		err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runs, recoveredRuns) {
		t.Fatalf("repaired warm native callbacks = %#v, want %#v", runs, recoveredRuns)
	}

	writeTypesFixture(t, path, "package nativecache\n\nfunc run(){target()}\nfunc target(){}\n")
	invalidated, err := analysis.RunPackages(
		context.Background(),
		registry,
		runOptions,
		loadOptions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(runs, recoveredRuns) ||
		reflect.DeepEqual(invalidated.Files, first.Files) {
		t.Fatalf(
			"source-invalidated native callbacks = %#v; result = %#v",
			runs,
			invalidated,
		)
	}
}

func TestRunPackagesStatisticsExplainTiersDependenciesEffectsAndCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/statistics\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "dependency", "dependency.go"),
		"package dependency\nfunc Stop() {}\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "statistics.go"),
		"package statistics\nimport \"example.com/statistics/dependency\"\nfunc run() { dependency.Stop() }\n",
	)
	dependencyMetadata := packageMetadata("dependency-statistics-rule")
	dependencyMetadata.RequiresDependencySyntax = true
	effectMetadata := controlFlowMetadata("effect-statistics-rule")
	effectMetadata.RequiresEffectFacts = true
	registry, err := rules.NewRegistry(
		packageRule{
			metadata: dependencyMetadata,
			run: func(*rules.PackageContext) ([]rules.PackageFinding, error) {
				return nil, nil
			},
		},
		controlFlowRule{
			metadata: effectMetadata,
			run: func(*rules.ControlFlowContext) ([]rules.Finding, error) {
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	store, err := cache.Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	loadOptions := packageAnalyzerCacheLoadOptions(root)
	coldStatistics := analysis.NewStatistics()
	runOptions := analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache: packageAnalyzerCacheOptions(store),
		Statistics: coldStatistics,
	}
	if _, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions);
		err != nil {
		t.Fatal(err)
	}
	cold := coldStatistics.Snapshot()
	if !reflect.DeepEqual(
		cold.DependencySyntaxReasons,
		[]string{"dependency-statistics-rule"},
	) ||
		!reflect.DeepEqual(cold.EffectFactReasons, []string{"effect-statistics-rule"}) ||
		cold.MaximumRequirement != rules.RequireControlFlow ||
		cold.PackageLoads.Calls != 1 ||
		cold.Packages == 0 ||
		cold.Files == 0 ||
		len(cold.Tiers) != 2 ||
		cold.Tiers[0].Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(
			cold.Tiers[0].Reasons,
			[]string{"dependency-statistics-rule", "effect-statistics-rule"},
		) ||
		cold.Tiers[1].Requirement != rules.RequireControlFlow ||
		!reflect.DeepEqual(cold.Tiers[1].Reasons, []string{"effect-statistics-rule"}) ||
		cold.Cache.Lookups != 1 ||
		cold.Cache.Misses != 1 ||
		cold.Cache.Writes != 1 ||
		len(cold.Rules) != 2 ||
		cold.Rules[0].ID != "dependency-statistics-rule" ||
		cold.Rules[0].Metric.Calls == 0 ||
		cold.Rules[1].ID != "effect-statistics-rule" ||
		cold.Rules[1].Metric.Calls == 0 {
		t.Fatalf("cold statistics = %#v", cold)
	}

	warmStatistics := analysis.NewStatistics()
	runOptions.Statistics = warmStatistics
	if _, err := analysis.RunPackages(context.Background(), registry, runOptions, loadOptions);
		err != nil {
		t.Fatal(err)
	}
	warm := warmStatistics.Snapshot()
	if warm.Cache.Lookups != 1 ||
		warm.Cache.Hits != 1 ||
		warm.Cache.Misses != 0 ||
		warm.Cache.Writes != 0 ||
		len(warm.Rules) != 0 {
		t.Fatalf("warm statistics = %#v", warm)
	}
}

func TestRunPackagesRejectsCachedNativeDiagnosticAfterGeneratedPolicyChanges(t *testing.T) {
	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/generatedcache\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "generated.go"),
		"// Code generated by fixture. DO NOT EDIT.\npackage generatedcache\nfunc run(){target()}\nfunc target(){}\n",
	)

	runs := 0
	metadata := typesMetadata("generated-policy", rules.NodeCallExpr)
	metadata.RunOnGenerated = true
	rule := func(metadata rules.Metadata) typesRule {
		return typesRule{
			metadata: metadata,
			run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
				runs++
				range_, err := ctx.Range(node)
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{
						MessageKey: "generated",
						Message: "generated",
						Range: range_,
					},
				}, nil
			},
		}
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	runOptions := analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache: packageAnalyzerCacheOptions(store),
	}
	loadOptions := packageAnalyzerCacheLoadOptions(root)
	firstRegistry, err := rules.NewRegistry(rule(metadata))
	if err != nil {
		t.Fatal(err)
	}
	first, err := analysis.RunPackages(
		context.Background(),
		firstRegistry,
		runOptions,
		loadOptions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 || len(first.Files) != 1 || len(first.Files[0].Diagnostics) != 1 {
		t.Fatalf("generated-enabled native result = %#v, runs = %d", first, runs)
	}

	metadata.RunOnGenerated = false
	secondRegistry, err := rules.NewRegistry(rule(metadata))
	if err != nil {
		t.Fatal(err)
	}
	second, err := analysis.RunPackages(
		context.Background(),
		secondRegistry,
		runOptions,
		loadOptions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 || len(second.Files) != 1 || len(second.Files[0].Diagnostics) != 0 {
		t.Fatalf("generated-disabled cached native result = %#v, runs = %d", second, runs)
	}
}

func TestRunPackagesRejectsCachedEmptyNativeResultAfterGeneratedPolicyChanges(t *testing.T) {
	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/emptygeneratedcache\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "generated.go"),
		"// Code generated by fixture. DO NOT EDIT.\npackage emptygeneratedcache\nfunc run(){target()}\nfunc target(){}\n",
	)

	runs := 0
	metadata := typesMetadata("empty-generated-policy", rules.NodeCallExpr)
	rule := func(metadata rules.Metadata) typesRule {
		return typesRule{
			metadata: metadata,
			run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
				runs++
				range_, err := ctx.Range(node)
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{
						MessageKey: "generated",
						Message: "generated",
						Range: range_,
					},
				}, nil
			},
		}
	}
	store, err := cache.Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	runOptions := analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache: packageAnalyzerCacheOptions(store),
	}
	loadOptions := packageAnalyzerCacheLoadOptions(root)
	firstRegistry, err := rules.NewRegistry(rule(metadata))
	if err != nil {
		t.Fatal(err)
	}
	first, err := analysis.RunPackages(
		context.Background(),
		firstRegistry,
		runOptions,
		loadOptions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runs != 0 || len(first.Files) != 1 || len(first.Files[0].Diagnostics) != 0 {
		t.Fatalf("generated-disabled native result = %#v, runs = %d", first, runs)
	}

	metadata.RunOnGenerated = true
	secondRegistry, err := rules.NewRegistry(rule(metadata))
	if err != nil {
		t.Fatal(err)
	}
	second, err := analysis.RunPackages(
		context.Background(),
		secondRegistry,
		runOptions,
		loadOptions,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 || len(second.Files) != 1 || len(second.Files[0].Diagnostics) != 1 {
		t.Fatalf("generated-enabled cached native result = %#v, runs = %d", second, runs)
	}
}

func TestRunPackagesRequiresTypesBeforeLoading(t *testing.T) {
	t.Parallel()

	syntax := syntaxRule{
		metadata: analysisMetadata("syntax-only", rules.NodeCallExpr, false),
		run: func(*rules.Context, ast.Node) ([]rules.Finding, error) {
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(syntax)
	if err != nil {
		t.Fatal(err)
	}

	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"requires at least one types-tier, CFG-tier, or SSA-tier rule",
		) {
		t.Fatalf("RunPackages() error = %v", err)
	}
}

func TestRunPackagesRetainsLoadErrorsAndValidPartialResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	validPath := filepath.Join(root, "a_valid.go")
	writeTypesFixture(
		t,
		validPath,
		"package project\nfunc run() { target() }\nfunc target() {}\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "z_invalid.go"),
		"package project\nfunc broken( {\n",
	)

	metadata := typesMetadata("partial-types", rules.NodeCallExpr)
	metadata.RunDespiteTypeErrors = true
	typed := typesRule{
		metadata: metadata,
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "partial", Message: "partial", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(typed)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.LoadDiagnostics) == 0 ||
		len(result.SourceProblems) != 1 ||
		result.SourceProblems[0].Path != filepath.Join(root, "z_invalid.go") ||
		len(result.Files) != 1 ||
		result.Files[0].Path != validPath ||
		len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("RunPackages() partial result = %#v", result)
	}
}

func TestRunPackagesKeepsCgoSourceSyntaxOnlyAndNeverTargetsGeneratedCacheFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is not a supported Glippy runtime")
	}

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	path := filepath.Join(root, "cgo.go")
	writeTypesFixture(
		t,
		path,
		`package project

/*
int answer(void) { return 42; }
*/
import "C"

func Answer() int { return int(C.answer()) }
`,
	)

	syntax := syntaxRule{
		metadata: analysisMetadata("syntax-function", rules.NodeFuncDecl, false),
		run: func(ctx *rules.Context, node ast.Node) ([]rules.Finding, error) {
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "syntax", Message: "syntax", Range: range_},
			}, nil
		},
	}
	typed := typesRule{
		metadata: typesMetadata("typed-function", rules.NodeFuncDecl),
		run: func(ctx *rules.TypesContext, node ast.Node) ([]rules.Finding, error) {
			range_, err := ctx.Range(node)
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "typed", Message: "typed", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(syntax, typed)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Env: append(
				os.Environ(),
				"GOENV=off",
				"GOTOOLCHAIN=local",
				"CGO_ENABLED=1",
				"GOCACHE=" + t.TempDir(),
				"GOMODCACHE=" + t.TempDir(),
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 1 ||
		result.Files[0].Diagnostics[0].RuleID != "syntax-function" {
		t.Fatalf("RunPackages() cgo files = %#v", result.Files)
	}
	if len(result.LoadDiagnostics) != 1 ||
		result.LoadDiagnostics[0].Position != path ||
		!strings.Contains(
			result.LoadDiagnostics[0].Message,
			"typed analysis is unavailable for cgo source",
		) {
		t.Fatalf("RunPackages() cgo load diagnostics = %#v", result.LoadDiagnostics)
	}
}

func TestRunPackagesRunsControlFlowRulesBeforeSuppressing(t *testing.T) {
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
		"//glippy:ignore-file cfg-rule -- reviewed file\npackage project\nfunc run() {}\n",
	)
	rule := controlFlowRule{
		metadata: controlFlowMetadata("cfg-rule"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "cfg", Message: "cfg", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Requirement != rules.RequireControlFlow ||
		len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 0 ||
		len(result.Files[0].Suppressed) != 1 ||
		result.Files[0].Suppressed[0].Diagnostic.RuleID != "cfg-rule" {
		t.Fatalf("RunPackages() CFG result = %#v", result)
	}
}

func TestRunPackagesRunsPackageWideRulesBeforeSuppressing(t *testing.T) {
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
		"//glippy:ignore-file package-rule -- reviewed package\npackage project\nfunc run() {}\n",
	)
	rule := packageRule{
		metadata: packageMetadata("package-rule"),
		run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
			for _, file := range ctx.Files() {
				if !file.Target() {
					continue
				}
				range_, err := file.Range(file.Syntax().Name)
				if err != nil {
					return nil, err
				}
				return []rules.PackageFinding{
					{
						File: file,
						Finding: rules.Finding{
							MessageKey: "package",
							Message: "package",
							Range: range_,
						},
					},
				}, nil
			}
			return nil, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Requirement != rules.RequireTypes ||
		len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 0 ||
		len(result.Files[0].Suppressed) != 1 ||
		result.Files[0].Suppressed[0].Diagnostic.RuleID != "package-rule" {
		t.Fatalf("RunPackages() package-wide result = %#v", result)
	}
}

func TestRunPackagesLoadsDependencySyntaxOnlyForDeclaredNativeRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "dependency", "dependency.go"),
		"package dependency\nconst Value = 1\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nimport \"example.com/project/dependency\"\nconst Value = dependency.Value\n",
	)

	run := func(requiresDependencies bool, callerRequestsDependencies bool) int {
		t.Helper()
		metadata := packageMetadata("package-dependencies")
		metadata.RequiresDependencySyntax = requiresDependencies
		count := -1
		registry, err := rules.NewRegistry(
			packageRule{
				metadata: metadata,
				run: func(
					ctx *rules.PackageContext,
				) ([]rules.PackageFinding, error) {
					count = len(ctx.Dependencies())
					return nil, nil
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{Preset: rules.PresetCorrectness},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				LoadDependencySyntax: callerRequestsDependencies,
			},
		);
			err != nil {
			t.Fatal(err)
		}
		return count
	}
	if got := run(true, false); got != 1 {
		t.Fatalf("declared dependency count = %d, want 1", got)
	}
	if got := run(false, true); got != 0 {
		t.Fatalf("undeclared dependency count = %d, want 0", got)
	}
}

func TestRunPackagesInvalidatesNativeCacheWhenDependencySyntaxChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/project\n\ngo 1.26.0\n",
	)
	dependencyPath := filepath.Join(root, "dependency", "dependency.go")
	writeTypesFixture(t, dependencyPath, "package dependency\nconst Value = 1\n")
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		"package project\nimport \"example.com/project/dependency\"\nconst Value = dependency.Value\n",
	)

	metadata := packageMetadata("dependency-cache")
	metadata.RequiresDependencySyntax = true
	runs := 0
	registry, err := rules.NewRegistry(
		packageRule{
			metadata: metadata,
			run: func(ctx *rules.PackageContext) ([]rules.PackageFinding, error) {
				runs++
				if len(ctx.Dependencies()) != 1 {
					t.Fatalf(
						"dependency count = %d, want 1",
						len(ctx.Dependencies()),
					)
				}
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	run := func() {
		t.Helper()
		if _, err := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Preset: rules.PresetCorrectness,
				Cache: packageAnalyzerCacheOptions(store),
			},
			packageAnalyzerCacheLoadOptions(root),
		);
			err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	if runs != 1 {
		t.Fatalf("callbacks after warm run = %d, want 1", runs)
	}
	writeTypesFixture(t, dependencyPath, "package dependency\nconst Value = 2\n")
	run()
	if runs != 2 {
		t.Fatalf("callbacks after dependency change = %d, want 2", runs)
	}
}

func TestRunPackagesInvalidatesNativeCacheWhenParameterEffectsChange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTypesFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/effectcache\n\ngo 1.26.0\n",
	)
	dependencyPath := filepath.Join(root, "helper", "helper.go")
	writeTypesFixture(
		t,
		dependencyPath,
		"package helper\n\nimport \"io\"\n\nfunc Apply(io.Closer) {}\n",
	)
	writeTypesFixture(
		t,
		filepath.Join(root, "project.go"),
		`package project

import (
	"io"
	"example.com/effectcache/helper"
)

func run(closer io.Closer) { helper.Apply(closer) }
`,
	)

	metadata := controlFlowMetadata("parameter-effect-cache")
	metadata.RequiresEffectFacts = true
	runs := 0
	registry, err := rules.NewRegistry(
		controlFlowRule{
			metadata: metadata,
			run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
				declaration, ok := ctx.Function().(*ast.FuncDecl)
				if !ok || declaration.Name.Name != "run" {
					return nil, nil
				}
				runs++
				var summary rules.ParameterEffectSummary
				ast.Inspect(
					declaration.Body,
					func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
						if ok {
							summary = ctx.ParameterEffect(call, 0)
						}
						return true
					},
				)
				if !summary.Known || summary.Always {
					return nil, nil
				}
				range_, err := ctx.Range(declaration.Name)
				if err != nil {
					return nil, err
				}
				return []rules.Finding{
					{
						MessageKey: "borrowed",
						Message: "helper borrows the parameter",
						Range: range_,
					},
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	store, err := cache.Open(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	run := func() analysis.PackageResult {
		t.Helper()
		result, err := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Preset: rules.PresetCorrectness,
				Cache: packageAnalyzerCacheOptions(store),
			},
			packageAnalyzerCacheLoadOptions(root),
		)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := run()
	second := run()
	if runs != 1 ||
		len(first.Files) != 1 ||
		len(first.Files[0].Diagnostics) != 1 ||
		!reflect.DeepEqual(second.Files, first.Files) {
		t.Fatalf(
			"warm parameter effect cache runs = %d; results = %#v, %#v",
			runs,
			first,
			second,
		)
	}
	writeTypesFixture(
		t,
		dependencyPath,
		"package helper\n\nimport \"io\"\n\nfunc Apply(closer io.Closer) { _ = closer.Close() }\n",
	)
	changed := run()
	if runs != 2 || len(changed.Files) != 1 || len(changed.Files[0].Diagnostics) != 0 {
		t.Fatalf("changed parameter effect cache runs = %d; result = %#v", runs, changed)
	}
}

func TestRunPackagesRejectsExpiredSuppressions(t *testing.T) {
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
		"//glippy:ignore-file cfg-rule -- expires=2026-08-11 temporary waiver\npackage project\nfunc run() {}\n",
	)
	rule := controlFlowRule{
		metadata: controlFlowMetadata("cfg-rule"),
		run: func(ctx *rules.ControlFlowContext) ([]rules.Finding, error) {
			range_, err := ctx.Range(ctx.Function())
			if err != nil {
				return nil, err
			}
			return []rules.Finding{
				{MessageKey: "cfg", Message: "cfg", Range: range_},
			}, nil
		},
	}
	registry, err := rules.NewRegistry(rule)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Preset: rules.PresetCorrectness,
			SuppressionExpiryCutoff: "2026-08-11",
		},
		analysis.PackageLoadOptions{Dir: root, Patterns: []string{"."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 1 ||
		len(result.Files[0].Suppressed) != 0 ||
		len(result.Files[0].SuppressionProblems) != 1 ||
		result.Files[0].SuppressionProblems[0].Kind != "expired" {
		t.Fatalf("RunPackages() expired suppression result = %#v", result)
	}
}

func TestRunPackagesRejectsUnimplementedCheapTiersBeforeLoading(t *testing.T) {
	t.Parallel()

	lexicalMetadata := analysisMetadata("lexical-rule", rules.NodeFile, false)
	lexicalMetadata.Requirement = rules.RequireLexical
	typedMetadata := typesMetadata("typed-rule", rules.NodeCallExpr)
	registry, err := rules.NewRegistry(
		metadataRuleAdapter{metadata: lexicalMetadata},
		typesRule{
			metadata: typedMetadata,
			run: func(*rules.TypesContext, ast.Node) ([]rules.Finding, error) {
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{Preset: rules.PresetCorrectness},
		analysis.PackageLoadOptions{},
	)
	if err == nil ||
		!strings.Contains(err.Error(), `selected rule "lexical-rule"`) ||
		!strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("RunPackages() error = %v", err)
	}
}
