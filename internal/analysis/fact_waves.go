package analysis

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"slices"
	"sort"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/rules"
	"golang.org/x/tools/go/packages"
)

const packageFactWaveSize = 8

type packageFactPlan struct {
	dependencies []string
	roots []string
	owners map[string]string
}

func preparePackageFactPlan(loaded PackageLoadResult) (packageFactPlan, error) {
	dependencies, roots, err := packageFactSchedule(loaded.Packages)
	if err != nil {
		return packageFactPlan{}, err
	}
	packages_, err := canonicalPackages(loaded.Packages)
	if err != nil {
		return packageFactPlan{}, err
	}
	ownedFiles, err := canonicalTypedFiles(packages_, loaded.Sources)
	if err != nil {
		return packageFactPlan{}, err
	}
	owners := make(map[string]string, len(ownedFiles))
	for _, owned := range ownedFiles {
		owners[owned.file.path] = owned.package_.ID
	}
	return packageFactPlan{dependencies: dependencies, roots: roots, owners: owners}, nil
}

func packageFactWaves(dependencies []string) [][]string {
	result := make(
		[][]string,
		0,
		(len(dependencies) + packageFactWaveSize - 1) / packageFactWaveSize,
	)
	for start := 0; start < len(dependencies); start += packageFactWaveSize {
		end := min(start + packageFactWaveSize, len(dependencies))
		result = append(result, slices.Clone(dependencies[start:end]))
	}
	return result
}

func packageFactWaveLoadOptions(
	options PackageLoadOptions,
	retained PackageSourceSet,
) (PackageLoadOptions, packageResourceLimits, error) {
	limits, err := resolvePackageResourceLimits(options)
	if err != nil {
		return PackageLoadOptions{}, packageResourceLimits{}, err
	}
	files, bytes_ := packageSourceUsage(retained)
	if files > limits.maxSourceFiles || bytes_ > limits.maxSourceBytes {
		return PackageLoadOptions{}, packageResourceLimits{}, fmt.Errorf(
			"retained package roots exceed %d-file/%d-byte source limits",
			limits.maxSourceFiles,
			limits.maxSourceBytes,
		)
	}
	result := clonePackageLoadOptions(options)
	result.MaxSourceFiles = max(1, limits.maxSourceFiles - files)
	result.MaxSourceBytes = max(1, limits.maxSourceBytes - bytes_)
	return result, limits, nil
}

func validatePackageFactWaveSources(
	retained PackageSourceSet,
	wave PackageSourceSet,
	limits packageResourceLimits,
) error {
	retainedFiles, retainedBytes := packageSourceUsage(retained)
	waveFiles, waveBytes := packageSourceUsage(wave)
	if retainedFiles + waveFiles > limits.maxSourceFiles {
		return fmt.Errorf(
			"package fact source set exceeds %d-file limit after retained roots",
			limits.maxSourceFiles,
		)
	}
	if waveBytes > limits.maxSourceBytes - retainedBytes {
		return fmt.Errorf(
			"package fact source set exceeds %d-byte limit after retained roots",
			limits.maxSourceBytes,
		)
	}
	return nil
}

func packageSourceUsage(sources PackageSourceSet) (int, int64) {
	bytes_ := int64(0)
	for _, file := range sources.files {
		bytes_ += file.ByteSize()
	}
	return len(sources.files), bytes_
}

func loadPackageFactRoots(
	ctx context.Context,
	options PackageLoadOptions,
	retainedSources PackageSourceSet,
) (PackageLoadResult, PackageLoadOptions, error) {
	rootOptions := clonePackageLoadOptions(options)
	rootOptions.Requirement = rules.RequireTypes
	rootOptions.LoadDependencySyntax = false
	rootOptions.LoadEffectFacts = false
	rootOptions.compactDependencySource = false
	started := beginStatisticsMeasurement(statisticsFromContext(ctx))
	loaded, err := LoadPackages(ctx, rootOptions)
	if err != nil {
		return PackageLoadResult{}, PackageLoadOptions{}, fmt.Errorf(
			"reload package fact reporter roots: %w",
			err,
		)
	}
	statisticsFromContext(ctx).recordPackageLoad(started, loaded)
	if err := validateReloadedPackageFactSources(retainedSources, loaded.Sources); err != nil {
		return PackageLoadResult{}, PackageLoadOptions{}, err
	}
	return loaded, rootOptions, nil
}

func validateReloadedPackageFactSources(
	retained PackageSourceSet,
	reloaded PackageSourceSet,
) error {
	if len(retained.paths) != len(reloaded.paths) {
		return fmt.Errorf(
			"reloaded package fact roots contain %d sources, want %d",
			len(reloaded.paths),
			len(retained.paths),
		)
	}
	for index, path := range retained.paths {
		if reloaded.paths[index] != path {
			return fmt.Errorf(
				"reloaded package fact root source %d is %q, want %q",
				index,
				reloaded.paths[index],
				path,
			)
		}
		retainedFile := retained.files[path]
		reloadedFile := reloaded.files[path]
		if retainedFile == nil ||
			reloadedFile == nil ||
			retainedFile.Digest() != reloadedFile.Digest() {
			return fmt.Errorf("reloaded package fact root source %q changed", path)
		}
	}
	return nil
}

func (r *packageAnalyzerRule) runPackageFactWave(
	ctx context.Context,
	dependencyWave []string,
	retainedSources PackageSourceSet,
	loadOptions PackageLoadOptions,
	cachePlan *packageCachePlan,
	severity rules.Severity,
	store *retainedPackageFactStore,
) error {
	waveOptions, sourceLimits, err := packageFactWaveLoadOptions(loadOptions, retainedSources)
	if err != nil {
		return err
	}
	waveOptions.Patterns = dependencyWave
	waveOptions.Requirement = rules.RequireTypes
	waveOptions.Tests = false
	waveOptions.LoadDependencySyntax = false
	waveOptions.LoadEffectFacts = false
	waveOptions.compactDependencySource = false
	started := beginStatisticsMeasurement(statisticsFromContext(ctx))
	wave, err := LoadPackages(ctx, waveOptions)
	if err != nil {
		return fmt.Errorf(
			"load package fact wave after retained roots within %d-file/%d-byte source limits: %w",
			sourceLimits.maxSourceFiles,
			sourceLimits.maxSourceBytes,
			err,
		)
	}
	if err := validatePackageFactWaveSources(retainedSources, wave.Sources, sourceLimits);
		err != nil {
		return err
	}
	statisticsFromContext(ctx).recordPackageLoad(started, wave)
	byPath, err := packagesByTypePath(wave.Packages)
	if err != nil {
		return err
	}
	for _, path := range dependencyWave {
		pkg := byPath[path]
		if pkg == nil {
			return fmt.Errorf("package fact wave omitted %q", path)
		}
		if _, err := r.runRetainedFactPackage(
			ctx,
			pkg,
			wave,
			waveOptions,
			cachePlan,
			map[string]string{},
			severity,
			store,
		);
			err != nil {
			return err
		}
	}
	return nil
}

type retainedPackageFact struct {
	id string
	path string
	snapshots [][]byte
	cacheKey cache.Key
	cacheable bool
}

type retainedPackageFactStore struct {
	byID map[string]*retainedPackageFact
	byPath map[string][]*retainedPackageFact
}

func newRetainedPackageFactStore() *retainedPackageFactStore {
	return &retainedPackageFactStore{
		byID: make(map[string]*retainedPackageFact),
		byPath: make(map[string][]*retainedPackageFact),
	}
}

func (s *retainedPackageFactStore) put(entry *retainedPackageFact) error {
	if s == nil || entry == nil || entry.id == "" || entry.path == "" {
		return fmt.Errorf("retained package facts require an identified package")
	}
	if _, found := s.byID[entry.id]; found {
		return fmt.Errorf("retained package facts contain duplicate ID %q", entry.id)
	}
	s.byID[entry.id] = entry
	s.byPath[entry.path] = append(s.byPath[entry.path], entry)
	return nil
}

func (s *retainedPackageFactStore) lookup(id, path string) (*retainedPackageFact, error) {
	if s == nil {
		return nil, nil
	}
	if id != "" {
		if entry := s.byID[id]; entry != nil {
			if path != "" && entry.path != path {
				return nil, fmt.Errorf(
					"package fact ID %q has path %q, want %q",
					id,
					entry.path,
					path,
				)
			}
			return entry, nil
		}
	}
	entries := s.byPath[path]
	if len(entries) == 0 {
		return nil, nil
	}
	if len(entries) == 1 {
		return entries[0], nil
	}
	for _, entry := range entries {
		if entry.id == path {
			return entry, nil
		}
	}
	return nil, fmt.Errorf(
		"package fact path %q is ambiguous across %d package IDs",
		path,
		len(entries),
	)
}

func packageFactSchedule(roots []*packages.Package) ([]string, []string, error) {
	rootPaths := make(map[string]struct{})
	rootIDsByPath := make(map[string][]string)
	nodes := make(map[string]map[string]struct{})
	var collect func(*types.Package)
	collect = func(pkg *types.Package) {
		if pkg == nil || pkg.Path() == "" || pkg.Path() == "C" {
			return
		}
		if _, collected := nodes[pkg.Path()]; collected {
			return
		}
		nodes[pkg.Path()] = make(map[string]struct{})
		for _, imported := range pkg.Imports() {
			if imported == nil || imported.Path() == "" || imported.Path() == "C" {
				continue
			}
			if imported.Path() != pkg.Path() {
				nodes[pkg.Path()][imported.Path()] = struct{}{}
			}
			collect(imported)
		}
	}
	for _, root := range roots {
		if root == nil || root.ID == "" || root.Types == nil || root.Types.Path() == "" {
			return nil, nil, fmt.Errorf(
				"package fact schedule contains an unidentified root",
			)
		}
		rootPaths[root.Types.Path()] = struct{}{}
		rootIDsByPath[root.Types.Path()] = append(rootIDsByPath[root.Types.Path()], root.ID)
		collect(root.Types)
	}
	state := make(map[string]uint8)
	order := make([]string, 0, len(nodes))
	var visit func(string) error
	visit = func(path string) error {
		switch state[path] {
		case 1:
			return fmt.Errorf("package fact graph contains an import cycle at %q", path)
		case 2:
			return nil
		}
		state[path] = 1
		imports := make([]string, 0, len(nodes[path]))
		for imported := range nodes[path] {
			imports = append(imports, imported)
		}
		sort.Strings(imports)
		for _, imported := range imports {
			if err := visit(imported); err != nil {
				return err
			}
		}
		state[path] = 2
		order = append(order, path)
		return nil
	}
	paths := make([]string, 0, len(nodes))
	for path := range nodes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := visit(path); err != nil {
			return nil, nil, err
		}
	}
	dependencies := make([]string, 0, len(order))
	rootOrder := make([]string, 0, len(roots))
	for _, path := range order {
		if _, root := rootPaths[path]; !root {
			dependencies = append(dependencies, path)
			continue
		}
		ids := rootIDsByPath[path]
		sort.Strings(ids)
		rootOrder = append(rootOrder, ids...)
	}
	return dependencies, rootOrder, nil
}

func packagesByTypePath(packages_ []*packages.Package) (map[string]*packages.Package, error) {
	result := make(map[string]*packages.Package, len(packages_))
	for _, pkg := range packages_ {
		if pkg == nil || pkg.Types == nil || pkg.Types.Path() == "" {
			return nil, fmt.Errorf(
				"package fact wave contains an unidentified typed package",
			)
		}
		path := pkg.Types.Path()
		if _, duplicate := result[path]; duplicate {
			return nil, fmt.Errorf("package fact wave contains duplicate path %q", path)
		}
		result[path] = pkg
	}
	return result, nil
}

func (r *packageAnalyzerRule) runRetainedFactPackage(
	ctx context.Context,
	pkg *packages.Package,
	loaded PackageLoadResult,
	loadOptions PackageLoadOptions,
	cachePlan *packageCachePlan,
	owners map[string]string,
	severity rules.Severity,
	store *retainedPackageFactStore,
) ([]rules.Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pkg.IllTyped {
		for _, step := range r.steps {
			if !step.original.RunDespiteErrors {
				return nil, fmt.Errorf(
					"analyzer %q cannot produce facts for ill-typed package %q",
					step.original.Name,
					pkg.ID,
				)
			}
		}
	}
	facts := newAnalyzerFactSet()
	if err := r.restoreImportedFactClosure(pkg, facts, store); err != nil {
		return nil, err
	}
	files, err := packageSyntaxFiles(pkg, loaded.Sources)
	if err != nil {
		return nil, err
	}
	dependencyKeys, dependenciesCacheable, err := retainedDependencyCacheKeys(pkg, store)
	if err != nil {
		return nil, err
	}
	baseKey, baseCacheable := r.packageCacheBaseKey(loaded, loadOptions, cachePlan)
	key, packageCacheable := r.packageCacheKey(
		pkg,
		cachePlan,
		loadOptions,
		baseKey,
		baseCacheable && dependenciesCacheable,
		dependencyKeys,
	)
	statistics := statisticsFromContext(ctx)
	invalidHit := false
	var produced []rules.Diagnostic
	if packageCacheable {
		encoded, found, err := cachePlan.options.Store.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("read package analyzer cache: %w", err)
		}
		if found {
			produced, err = r.restorePackageCacheEntry(
				pkg,
				loaded.Sources,
				owners,
				severity,
				facts,
				encoded,
			)
			if err == nil {
				statistics.recordCacheLookup(true, false)
			} else {
				invalidHit = true
				statistics.recordCacheLookup(false, true)
			}
		} else {
			statistics.recordCacheLookup(false, false)
		}
	}
	if produced == nil {
		produced, err = r.runPackage(ctx, pkg, files, owners, severity, facts)
		if err != nil {
			return nil, err
		}
		if packageCacheable {
			encoded, encodeErr := r.encodePackageCacheEntry(pkg, produced, facts)
			if encodeErr == nil {
				if err := cachePlan.options.Store.Put(ctx, key, encoded);
					err != nil {
					if !invalidHit || !errors.Is(err, cache.ErrConflict) {
						return nil, fmt.Errorf(
							"write package analyzer cache: %w",
							err,
						)
					}
				} else {
					statistics.recordCacheWrite()
				}
			}
		}
	}
	snapshots := make([][]byte, len(r.steps))
	for index, step := range r.steps {
		snapshot, err := facts.encodeImportablePackageFactSnapshot(step.original, pkg.Types)
		if err != nil {
			return nil, fmt.Errorf(
				"retain analyzer %q facts: %w",
				step.original.Name,
				err,
			)
		}
		snapshots[index] = snapshot
	}
	if err := store.put(
		&retainedPackageFact{
			id: pkg.ID,
			path: pkg.Types.Path(),
			snapshots: snapshots,
			cacheKey: key,
			cacheable: packageCacheable,
		},
	);
		err != nil {
		return nil, err
	}
	return produced, nil
}

func retainedDependencyCacheKeys(
	pkg *packages.Package,
	store *retainedPackageFactStore,
) (map[string]cache.Key, bool, error) {
	result := make(map[string]cache.Key)
	for _, imported := range pkg.Types.Imports() {
		entry, err := store.lookup(importedPackageID(pkg, imported.Path()), imported.Path())
		if err != nil {
			return nil, false, err
		}
		if entry == nil || !entry.cacheable {
			return result, false, nil
		}
		result[imported.Path()] = entry.cacheKey
	}
	return result, true, nil
}

func (r *packageAnalyzerRule) restoreImportedFactClosure(
	pkg *packages.Package,
	facts *analyzerFactSet,
	store *retainedPackageFactStore,
) error {
	visited := make(map[*types.Package]struct{})
	var restore func(*types.Package, string) error
	restore = func(current *types.Package, id string) error {
		if current == nil || current.Path() == "" {
			return nil
		}
		if _, found := visited[current]; found {
			return nil
		}
		visited[current] = struct{}{}
		imports := slices.Clone(current.Imports())
		sort.Slice(
			imports,
			func(left, right int) bool {
				return imports[left].Path() < imports[right].Path()
			},
		)
		for _, imported := range imports {
			if err := restore(imported, ""); err != nil {
				return err
			}
		}
		entry, err := store.lookup(id, current.Path())
		if err != nil {
			return err
		}
		wrapper := &packages.Package{ID: id, PkgPath: current.Path(), Types: current}
		for index, step := range r.steps {
			if entry == nil {
				if err := facts.beginObjectFacts(step.original, wrapper);
					err != nil {
					return err
				}
				continue
			}
			if index >= len(entry.snapshots) {
				return fmt.Errorf(
					"package fact snapshot plan for %q is incomplete",
					current.Path(),
				)
			}
			if err := facts.restoreImportablePackageFactSnapshot(
				step.original,
				wrapper,
				entry.snapshots[index],
			);
				err != nil {
				return fmt.Errorf(
					"restore imported facts for %q: %w",
					current.Path(),
					err,
				)
			}
		}
		return nil
	}
	imports := slices.Clone(pkg.Types.Imports())
	sort.Slice(
		imports,
		func(left, right int) bool {
			return imports[left].Path() < imports[right].Path()
		},
	)
	for _, imported := range imports {
		if err := restore(imported, importedPackageID(pkg, imported.Path())); err != nil {
			return err
		}
	}
	return nil
}

func importedPackageID(pkg *packages.Package, path string) string {
	if pkg != nil {
		if imported := pkg.Imports[path]; imported != nil {
			return imported.ID
		}
	}
	return ""
}
