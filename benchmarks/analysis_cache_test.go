package benchmarks_test

import (
	"context"
	"go/ast"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	goanalysis "golang.org/x/tools/go/analysis"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/cache"
	"github.com/faustbrian/gox/internal/rules"
)

type benchmarkPackageFact struct {
	PackagePath string
}

func (*benchmarkPackageFact) AFact() {}

type nativeCacheBenchmarkCounters struct {
	types atomic.Int64
	packageWide atomic.Int64
	cfg atomic.Int64
	ssa atomic.Int64
}

type nativeCacheBenchmarkCounts struct {
	types int64
	packageWide int64
	cfg int64
	ssa int64
}

type nativeCacheBenchmarkTypesRule struct {
	metadata rules.Metadata
	counters *nativeCacheBenchmarkCounters
}

func (r nativeCacheBenchmarkTypesRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r nativeCacheBenchmarkTypesRule) RunTypes(
	*rules.TypesContext,
	ast.Node,
) ([]rules.Finding, error) {
	r.counters.types.Add(1)
	return nil, nil
}

type nativeCacheBenchmarkPackageRule struct {
	metadata rules.Metadata
	counters *nativeCacheBenchmarkCounters
}

func (r nativeCacheBenchmarkPackageRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r nativeCacheBenchmarkPackageRule) RunPackage(
	*rules.PackageContext,
) ([]rules.PackageFinding, error) {
	r.counters.packageWide.Add(1)
	return nil, nil
}

type nativeCacheBenchmarkControlFlowRule struct {
	metadata rules.Metadata
	counters *nativeCacheBenchmarkCounters
}

func (r nativeCacheBenchmarkControlFlowRule) Metadata() rules.Metadata {
	return r.metadata
}

func (r nativeCacheBenchmarkControlFlowRule) RunControlFlow(
	*rules.ControlFlowContext,
) ([]rules.Finding, error) {
	r.counters.cfg.Add(1)
	return nil, nil
}

type nativeCacheBenchmarkSSARule struct {
	metadata rules.Metadata
	counters *nativeCacheBenchmarkCounters
}

func (r nativeCacheBenchmarkSSARule) Metadata() rules.Metadata {
	return r.metadata
}

func (r nativeCacheBenchmarkSSARule) RunSSA(*rules.SSAContext) ([]rules.Finding, error) {
	r.counters.ssa.Add(1)
	return nil, nil
}

func BenchmarkPackageAnalyzerFactCache(b *testing.B) {
	directory, err := filepath.Abs("testdata/workload")
	if err != nil {
		b.Fatal(err)
	}
	loadOptions := analysis.PackageLoadOptions{
		Dir: directory,
		Patterns: []string{"."},
		ModuleMode: analysis.ModuleReadonly,
		Env: replaceEnvironment(
			os.Environ(),
			map[string]string{"CGO_ENABLED": "0", "GOENV": "off", "GOWORK": "off"},
		),
		GOOS: runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}

	b.Run(
		"cold-result-cache",
		func(b *testing.B) {
			registry, runs := benchmarkPackageFactRegistry(b)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				b.StopTimer()
				root, err := os.MkdirTemp("", "gox-package-fact-cache-cold-")
				if err != nil {
					b.Fatal(err)
				}
				store, err := cache.Open(root)
				if err != nil {
					_ = os.RemoveAll(root)
					b.Fatal(err)
				}
				options := benchmarkPackageCacheRunOptions(store)
				b.StartTimer()

				result, runErr := analysis.RunPackages(
					context.Background(),
					registry,
					options,
					loadOptions,
				)

				b.StopTimer()
				closeErr := store.Close()
				cleanupErr := os.RemoveAll(root)
				b.StartTimer()
				validatePackageCacheBenchmarkResult(b, result, runErr)
				if closeErr != nil {
					b.Fatal(closeErr)
				}
				if cleanupErr != nil {
					b.Fatal(cleanupErr)
				}
			}
			b.ReportMetric(float64(runs.Load()) / float64(b.N), "analyzer-runs/op")
		},
	)

	b.Run(
		"warm-result-cache",
		func(b *testing.B) {
			registry, runs := benchmarkPackageFactRegistry(b)
			store, err := cache.Open(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(
				func() {
					if err := store.Close(); err != nil {
						b.Error(err)
					}
				},
			)
			options := benchmarkPackageCacheRunOptions(store)
			result, err := analysis.RunPackages(
				context.Background(),
				registry,
				options,
				loadOptions,
			)
			validatePackageCacheBenchmarkResult(b, result, err)
			coldRuns := runs.Load()
			if coldRuns == 0 {
				b.Fatal("cache population did not execute the analyzer")
			}
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				result, err := analysis.RunPackages(
					context.Background(),
					registry,
					options,
					loadOptions,
				)
				validatePackageCacheBenchmarkResult(b, result, err)
			}
			if runs.Load() != coldRuns {
				b.Fatalf(
					"warm result cache reran analyzer %d times",
					runs.Load() - coldRuns,
				)
			}
			b.ReportMetric(0, "analyzer-runs/op")
		},
	)
}

func BenchmarkNativeAnalysisResultCache(b *testing.B) {
	directory, err := filepath.Abs("testdata/workload")
	if err != nil {
		b.Fatal(err)
	}
	loadOptions := analysis.PackageLoadOptions{
		Dir: directory,
		Patterns: []string{"."},
		ModuleMode: analysis.ModuleReadonly,
		Env: replaceEnvironment(
			os.Environ(),
			map[string]string{"CGO_ENABLED": "0", "GOENV": "off", "GOWORK": "off"},
		),
		GOOS: runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}

	for _, requirement := range
		[]rules.Requirement{
			rules.RequireTypes,
			rules.RequireControlFlow,
			rules.RequireSSA,
		} {
		b.Run(
			nativeCacheBenchmarkTierName(requirement),
			func(b *testing.B) {
				b.Run(
					"cold-result-cache",
					func(b *testing.B) {
						registry, counters := nativeCacheBenchmarkRegistry(
							b,
							requirement,
						)
						b.ReportAllocs()
						b.ResetTimer()

						for b.Loop() {
							b.StopTimer()
							root, err := os.MkdirTemp(
								"",
								"gox-native-cache-cold-",
							)
							if err != nil {
								b.Fatal(err)
							}
							store, err := cache.Open(root)
							if err != nil {
								_ = os.RemoveAll(root)
								b.Fatal(err)
							}
							options := benchmarkPackageCacheRunOptions(
								store,
							)
							b.StartTimer()

							result, runErr := analysis.RunPackages(
								context.Background(),
								registry,
								options,
								loadOptions,
							)

							b.StopTimer()
							closeErr := store.Close()
							cleanupErr := os.RemoveAll(root)
							validateNativeCacheBenchmarkResult(
								b,
								result,
								runErr,
								requirement,
							)
							if closeErr != nil {
								b.Fatal(closeErr)
							}
							if cleanupErr != nil {
								b.Fatal(cleanupErr)
							}
							b.StartTimer()
						}
						cold := counters.snapshot()
						validateNativeCacheBenchmarkCallbacks(
							b,
							cold,
							requirement,
						)
						reportNativeCacheBenchmarkMetrics(
							b,
							cold,
							float64(b.N),
						)
					},
				)

				b.Run(
					"warm-result-cache",
					func(b *testing.B) {
						registry, counters := nativeCacheBenchmarkRegistry(
							b,
							requirement,
						)
						store, err := cache.Open(b.TempDir())
						if err != nil {
							b.Fatal(err)
						}
						b.Cleanup(
							func() {
								if err := store.Close();
									err != nil {
									b.Error(err)
								}
							},
						)
						options := benchmarkPackageCacheRunOptions(store)
						result, err := analysis.RunPackages(
							context.Background(),
							registry,
							options,
							loadOptions,
						)
						validateNativeCacheBenchmarkResult(
							b,
							result,
							err,
							requirement,
						)
						cold := counters.snapshot()
						validateNativeCacheBenchmarkCallbacks(
							b,
							cold,
							requirement,
						)
						b.ReportAllocs()
						b.ResetTimer()

						for b.Loop() {
							result, err := analysis.RunPackages(
								context.Background(),
								registry,
								options,
								loadOptions,
							)
							validateNativeCacheBenchmarkResult(
								b,
								result,
								err,
								requirement,
							)
						}
						warm := counters.snapshot().subtract(cold)
						if warm.total() != 0 {
							b.Fatalf(
								"warm result cache reran native callbacks: %#v",
								warm,
							)
						}
						reportNativeCacheBenchmarkMetrics(
							b,
							warm,
							float64(b.N),
						)
					},
				)
			},
		)
	}
}

func nativeCacheBenchmarkRegistry(
	b *testing.B,
	requirement rules.Requirement,
) (*rules.Registry, *nativeCacheBenchmarkCounters) {
	b.Helper()
	counters := new(nativeCacheBenchmarkCounters)
	nativeRules := []rules.Rule{
		nativeCacheBenchmarkTypesRule{
			metadata: nativeCacheBenchmarkMetadata(
				"benchmark-native-types",
				rules.RequireTypes,
				[]rules.NodeKind{rules.NodeCallExpr},
			),
			counters: counters,
		},
		nativeCacheBenchmarkPackageRule{
			metadata: nativeCacheBenchmarkMetadata(
				"benchmark-native-package",
				rules.RequireTypes,
				nil,
			),
			counters: counters,
		},
	}
	if requirement >= rules.RequireControlFlow {
		nativeRules = append(
			nativeRules,
			nativeCacheBenchmarkControlFlowRule{
				metadata: nativeCacheBenchmarkMetadata(
					"benchmark-native-cfg",
					rules.RequireControlFlow,
					nil,
				),
				counters: counters,
			},
		)
	}
	if requirement >= rules.RequireSSA {
		nativeRules = append(
			nativeRules,
			nativeCacheBenchmarkSSARule{
				metadata: nativeCacheBenchmarkMetadata(
					"benchmark-native-ssa",
					rules.RequireSSA,
					nil,
				),
				counters: counters,
			},
		)
	}
	registry, err := rules.NewRegistry(nativeRules...)
	if err != nil {
		b.Fatal(err)
	}
	return registry, counters
}

func nativeCacheBenchmarkMetadata(
	id string,
	requirement rules.Requirement,
	interests []rules.NodeKind,
) rules.Metadata {
	return rules.Metadata{
		ID: id,
		Summary: "Benchmark native analysis result persistence.",
		Documentation: "Measures cold population and independent-load native result restoration.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.26",
		Requirement: requirement,
		NodeInterests: interests,
		RunOnGenerated: true,
		Categories: []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{
			{
				Title: "Benchmark fixture",
				Incorrect: "package benchmark\n",
				Correct: "package benchmark\n",
			},
		},
	}
}

func nativeCacheBenchmarkTierName(requirement rules.Requirement) string {
	switch requirement {
	case rules.RequireTypes:
		return "types"
	case rules.RequireControlFlow:
		return "control-flow"
	case rules.RequireSSA:
		return "ssa"
	default:
		return requirement.String()
	}
}

func (c *nativeCacheBenchmarkCounters) snapshot() nativeCacheBenchmarkCounts {
	return nativeCacheBenchmarkCounts{
		types: c.types.Load(),
		packageWide: c.packageWide.Load(),
		cfg: c.cfg.Load(),
		ssa: c.ssa.Load(),
	}
}

func (c nativeCacheBenchmarkCounts) subtract(
	other nativeCacheBenchmarkCounts,
) nativeCacheBenchmarkCounts {
	return nativeCacheBenchmarkCounts{
		types: c.types - other.types,
		packageWide: c.packageWide - other.packageWide,
		cfg: c.cfg - other.cfg,
		ssa: c.ssa - other.ssa,
	}
}

func (c nativeCacheBenchmarkCounts) total() int64 {
	return c.types + c.packageWide + c.cfg + c.ssa
}

func validateNativeCacheBenchmarkCallbacks(
	b *testing.B,
	counts nativeCacheBenchmarkCounts,
	requirement rules.Requirement,
) {
	b.Helper()
	if counts.types == 0 ||
		counts.packageWide == 0 ||
		(requirement >= rules.RequireControlFlow) != (counts.cfg > 0) ||
		(requirement >= rules.RequireSSA) != (counts.ssa > 0) {
		b.Fatalf("cold %s callback counts = %#v", requirement, counts)
	}
}

func reportNativeCacheBenchmarkMetrics(
	b *testing.B,
	counts nativeCacheBenchmarkCounts,
	operations float64,
) {
	b.Helper()
	b.ReportMetric(float64(counts.types) / operations, "types-callbacks/op")
	b.ReportMetric(float64(counts.packageWide) / operations, "package-callbacks/op")
	b.ReportMetric(float64(counts.cfg) / operations, "cfg-callbacks/op")
	b.ReportMetric(float64(counts.ssa) / operations, "ssa-callbacks/op")
}

func validateNativeCacheBenchmarkResult(
	b *testing.B,
	result analysis.PackageResult,
	err error,
	requirement rules.Requirement,
) {
	b.Helper()
	validatePackageCacheBenchmarkResult(b, result, err)
	if result.Requirement != requirement {
		b.Fatalf(
			"native cache benchmark requirement = %s, want %s",
			result.Requirement,
			requirement,
		)
	}
}

func benchmarkPackageFactRegistry(b *testing.B) (*rules.Registry, *atomic.Int64) {
	b.Helper()
	runs := new(atomic.Int64)
	analyzer := &goanalysis.Analyzer{
		Name: "benchmarkfacts",
		Doc: "exports one deterministic package fact for cache measurement",
		Run: func(pass *goanalysis.Pass) (any, error) {
			runs.Add(1)
			pass.ExportPackageFact(&benchmarkPackageFact{PackagePath: pass.Pkg.Path()})
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(benchmarkPackageFact)},
	}
	adapted, err := analysis.AdaptAnalyzer(
		analyzer,
		analysis.AnalyzerAdapterOptions{
			Metadata: rules.Metadata{
				ID: "benchmark-package-facts",
				Summary: "Benchmark package fact persistence.",
				Documentation: "Measures cold population and independent-load cache restoration.",
				DefaultSeverity: rules.SeverityWarn,
				Presets: []rules.Preset{rules.PresetCorrectness},
				MinimumGoVersion: "1.26",
				Requirement: rules.RequireTypes,
				NodeInterests: []rules.NodeKind{rules.NodeFile},
				RunOnGenerated: true,
				Categories: []rules.Category{rules.CategoryCorrectness},
				Examples: []rules.Example{
					{
						Title: "Benchmark fixture",
						Incorrect: "package benchmark\n",
						Correct: "package benchmark\n",
					},
				},
			},
			ReadOnlyAudited: true,
		},
	)
	if err != nil {
		b.Fatal(err)
	}
	registry, err := rules.NewRegistry(adapted)
	if err != nil {
		b.Fatal(err)
	}
	return registry, runs
}

func benchmarkPackageCacheRunOptions(store *cache.Store) analysis.RunOptions {
	return analysis.RunOptions{
		Preset: rules.PresetCorrectness,
		Cache: &analysis.PackageCacheOptions{
			Store: store,
			ToolVersion: "benchmark-v1",
			BuildGoVersion: runtime.Version(),
			SourceGoVersion: "1.26",
			Configuration: cache.DigestOf([]byte("benchmark-configuration")),
			FormatterMode: "gox-v1",
		},
	}
}

func validatePackageCacheBenchmarkResult(b *testing.B, result analysis.PackageResult, err error) {
	b.Helper()
	if err != nil {
		b.Fatal(err)
	}
	if len(result.LoadDiagnostics) != 0 || len(result.SourceProblems) != 0 {
		b.Fatalf("package cache benchmark reported load problems: %#v", result)
	}
	for _, file := range result.Files {
		if len(file.Diagnostics) != 0 || len(file.SuppressionProblems) != 0 {
			b.Fatalf("package cache benchmark reported findings: %#v", result)
		}
	}
}
