package benchmarks_test

import (
	"context"
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

func BenchmarkPackageAnalyzerFactCache(b *testing.B) {
	directory, err := filepath.Abs("testdata/workload")
	if err != nil {
		b.Fatal(err)
	}
	loadOptions := analysis.PackageLoadOptions{
		Dir:        directory,
		Patterns:   []string{"."},
		ModuleMode: analysis.ModuleReadonly,
		Env: replaceEnvironment(os.Environ(), map[string]string{
			"CGO_ENABLED": "0",
			"GOENV":       "off",
			"GOWORK":      "off",
		}),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
	}

	b.Run("cold-result-cache", func(b *testing.B) {
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
				context.Background(), registry, options, loadOptions,
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
		b.ReportMetric(float64(runs.Load())/float64(b.N), "analyzer-runs/op")
	})

	b.Run("warm-result-cache", func(b *testing.B) {
		registry, runs := benchmarkPackageFactRegistry(b)
		store, err := cache.Open(b.TempDir())
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if err := store.Close(); err != nil {
				b.Error(err)
			}
		})
		options := benchmarkPackageCacheRunOptions(store)
		result, err := analysis.RunPackages(
			context.Background(), registry, options, loadOptions,
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
				context.Background(), registry, options, loadOptions,
			)
			validatePackageCacheBenchmarkResult(b, result, err)
		}
		if runs.Load() != coldRuns {
			b.Fatalf("warm result cache reran analyzer %d times", runs.Load()-coldRuns)
		}
		b.ReportMetric(0, "analyzer-runs/op")
	})
}

func benchmarkPackageFactRegistry(b *testing.B) (*rules.Registry, *atomic.Int64) {
	b.Helper()
	runs := new(atomic.Int64)
	analyzer := &goanalysis.Analyzer{
		Name: "benchmarkfacts",
		Doc:  "exports one deterministic package fact for cache measurement",
		Run: func(pass *goanalysis.Pass) (any, error) {
			runs.Add(1)
			pass.ExportPackageFact(&benchmarkPackageFact{PackagePath: pass.Pkg.Path()})
			return nil, nil
		},
		FactTypes: []goanalysis.Fact{new(benchmarkPackageFact)},
	}
	adapted, err := analysis.AdaptAnalyzer(analyzer, analysis.AnalyzerAdapterOptions{
		Metadata: rules.Metadata{
			ID: "benchmark-package-facts", Summary: "Benchmark package fact persistence.",
			Documentation:   "Measures cold population and independent-load cache restoration.",
			DefaultSeverity: rules.SeverityWarn, Presets: []rules.Preset{rules.PresetCorrectness},
			MinimumGoVersion: "1.26", Requirement: rules.RequireTypes,
			NodeInterests: []rules.NodeKind{rules.NodeFile}, RunOnGenerated: true,
			Categories: []rules.Category{rules.CategoryCorrectness},
			Examples: []rules.Example{{
				Title: "Benchmark fixture", Incorrect: "package benchmark\n", Correct: "package benchmark\n",
			}},
		},
		ReadOnlyAudited: true,
	})
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
			Store: store, ToolVersion: "benchmark-v1", BuildGoVersion: runtime.Version(),
			SourceGoVersion: "1.26", Configuration: cache.DigestOf([]byte("benchmark-configuration")),
			RuleOptions: map[string]cache.Digest{
				"benchmark-package-facts": cache.DigestOf(nil),
			},
			FormatterMode: "gox-v1",
		},
	}
}

func validatePackageCacheBenchmarkResult(
	b *testing.B,
	result analysis.PackageResult,
	err error,
) {
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
