package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

const packageFactMetadataLoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedForTest |
	packages.NeedModule

func (r *packageAnalyzerRule) prepareDependencyFactCandidates(
	ctx context.Context,
	dependencies []string,
	retainedSources PackageSourceSet,
	options PackageLoadOptions,
	cachePlan *packageCachePlan,
	store *retainedPackageFactStore,
) ([]string, error) {
	if r == nil || r.dependencyFactFilter == nil || len(dependencies) == 0 {
		return slices.Clone(dependencies), nil
	}
	buildFlags, err := packageBuildFlags(options)
	if err != nil {
		return nil, err
	}
	started := beginStatisticsMeasurement(statisticsFromContext(ctx))
	loaded, err := packages.Load(
		&packages.Config{
			Context: ctx,
			Mode: packageFactMetadataLoadMode,
			Dir: options.Dir,
			Env: packageLoadEnvironment(options),
			BuildFlags: buildFlags,
			Tests: false,
			Overlay: cloneOverlay(options.Overlay),
		},
		dependencies...,
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, fmt.Errorf("load package fact candidate metadata: %w", err)
	}
	if err := validatePackageGraphLimit(loaded, resolvedMaxPackages(options)); err != nil {
		return nil, err
	}
	byPath := make(map[string]*packages.Package, len(loaded))
	for _, pkg := range loaded {
		if pkg == nil || pkg.ID == "" || pkg.PkgPath == "" {
			return nil, fmt.Errorf(
				"package fact candidate metadata contains an unidentified package",
			)
		}
		if _, duplicate := byPath[pkg.PkgPath]; duplicate {
			return nil, fmt.Errorf(
				"package fact candidate metadata repeats path %q",
				pkg.PkgPath,
			)
		}
		byPath[pkg.PkgPath] = pkg
	}
	limits, err := resolvePackageResourceLimits(options)
	if err != nil {
		return nil, err
	}
	statisticPaths := make([]string, 0)
	candidates := make([]string, 0, len(dependencies))
	for _, path := range dependencies {
		pkg := byPath[path]
		if pkg == nil {
			return nil, fmt.Errorf("package fact candidate metadata omitted %q", path)
		}
		files, components, mayExport, err := r.inspectDependencyFactCandidate(
			pkg,
			retainedSources,
			options,
			limits,
		)
		if err != nil {
			return nil, err
		}
		statisticPaths = append(statisticPaths, files...)
		if mayExport || len(pkg.Errors) > 0 {
			candidates = append(candidates, path)
			continue
		}
		if err := r.retainEmptyDependencyFacts(pkg, components, options, cachePlan, store);
			err != nil {
			return nil, err
		}
	}
	sort.Strings(statisticPaths)
	statisticPaths = slices.Compact(statisticPaths)
	statisticsFromContext(
		ctx,
	).recordPackageLoad(
		started,
		PackageLoadResult{
			Requirement: rules.RequireTypes,
			Packages: loaded,
			Sources: PackageSourceSet{paths: statisticPaths},
		},
	)
	return candidates, nil
}

func resolvedMaxPackages(options PackageLoadOptions) int {
	if options.MaxPackages > 0 {
		return options.MaxPackages
	}
	return DefaultMaxPackages
}

func (r *packageAnalyzerRule) inspectDependencyFactCandidate(
	pkg *packages.Package,
	retainedSources PackageSourceSet,
	options PackageLoadOptions,
	limits packageResourceLimits,
) ([]string, []cache.Component, bool, error) {
	paths := slices.Clone(pkg.CompiledGoFiles)
	sort.Strings(paths)
	paths = slices.Compact(paths)
	retainedFiles, retainedBytes := packageSourceUsage(retainedSources)
	if retainedFiles + len(paths) > limits.maxSourceFiles {
		return nil, nil, false, fmt.Errorf(
			"package fact candidate source set exceeds %d-file limit after retained roots",
			limits.maxSourceFiles,
		)
	}
	components := make([]cache.Component, 0, len(paths) + 1)
	sources := make([]AnalyzerDependencyFactSource, 0, len(paths))
	components = append(
		components,
		cache.Component{
			Kind: cache.ComponentBuildSelection,
			Identity: "dependency-fact-filter",
			Digest: cache.DigestOf([]byte(r.dependencyFactFilter.Identity)),
		},
	)
	totalBytes := retainedBytes
	for _, path := range paths {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) || path == "" {
			return nil, nil, false, fmt.Errorf(
				"package fact candidate %q has invalid source path %q",
				pkg.PkgPath,
				path,
			)
		}
		input, found := options.Overlay[path]
		if found {
			input = slices.Clone(input)
		} else {
			var err error
			input, err = os.ReadFile(path)
			if err != nil {
				return nil, nil, false, fmt.Errorf(
					"read package fact candidate source %q: %w",
					path,
					err,
				)
			}
		}
		if err := source.ValidateSize(int64(len(input))); err != nil {
			return nil, nil, false, fmt.Errorf(
				"package fact candidate source %q: %w",
				path,
				err,
			)
		}
		totalBytes += int64(len(input))
		if totalBytes > limits.maxSourceBytes {
			return nil, nil, false, fmt.Errorf(
				"package fact candidate source set exceeds %d-byte limit after retained roots",
				limits.maxSourceBytes,
			)
		}
		sources = append(sources, AnalyzerDependencyFactSource{Path: path, Bytes: input})
		components = append(
			components,
			cache.Component{
				Kind: cache.ComponentSource,
				Identity: path,
				Digest: cache.DigestOf(input),
			},
		)
	}
	mayExport, err := callDependencyFactFilter(r.dependencyFactFilter, sources)
	if err != nil {
		return nil, nil, false, err
	}
	return paths, components, mayExport, nil
}

func callDependencyFactFilter(
	filter *AnalyzerDependencyFactFilter,
	sources []AnalyzerDependencyFactSource,
) (result bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"dependency fact filter %q panicked: %v",
				filter.Identity,
				recovered,
			)
		}
	}()
	return filter.PackageMayExport(sources)
}

func (r *packageAnalyzerRule) retainEmptyDependencyFacts(
	pkg *packages.Package,
	components []cache.Component,
	options PackageLoadOptions,
	cachePlan *packageCachePlan,
	store *retainedPackageFactStore,
) error {
	snapshots := make([][]byte, len(r.steps))
	for index, step := range r.steps {
		snapshot, err := encodeEmptyPackageFactSnapshot(step.original, pkg.PkgPath)
		if err != nil {
			return fmt.Errorf(
				"retain empty analyzer %q facts: %w",
				step.original.Name,
				err,
			)
		}
		snapshots[index] = snapshot
	}
	key, cacheable, err := r.emptyDependencyFactKey(pkg, components, options, cachePlan)
	if err != nil {
		return err
	}
	return store.put(
		&retainedPackageFact{
			id: pkg.ID,
			path: pkg.PkgPath,
			snapshots: snapshots,
			cacheKey: key,
			cacheable: cacheable,
		},
	)
}

func (r *packageAnalyzerRule) emptyDependencyFactKey(
	pkg *packages.Package,
	components []cache.Component,
	options PackageLoadOptions,
	plan *packageCachePlan,
) (cache.Key, bool, error) {
	if plan == nil {
		return cache.Key{}, false, nil
	}
	components = append(
		slices.Clone(components),
		cache.Component{
			Kind: cache.ComponentBuildSelection,
			Identity: "package",
			Digest: cache.DigestOf([]byte(pkg.ID + "\x00" + pkg.PkgPath)),
		},
	)
	key, err := cache.BuildKey(
		cache.KeyInput{
			Namespace: "typed-analyzer-empty-facts-v1:" + r.metadata.ID,
			ToolVersion: plan.options.ToolVersion,
			BuildGoVersion: plan.options.BuildGoVersion,
			SourceGoVersion: plan.options.SourceGoVersion,
			Configuration: plan.options.Configuration,
			Rules: slices.Clone(plan.rules),
			BuildTags: slices.Clone(options.BuildTags),
			GOOS: options.GOOS,
			GOARCH: options.GOARCH,
			CGOEnabled: plan.options.CGOEnabled,
			FormatterMode: plan.options.FormatterMode,
			Components: components,
		},
	)
	if err != nil {
		return cache.Key{}, false, fmt.Errorf("build empty dependency fact key: %w", err)
	}
	return key, true, nil
}
