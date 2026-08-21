package analysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	goversion "go/version"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

const (
	maximumPackageSessionEntries = 8
	maximumPackageSessionBytes int64 = 128 << 20
	packageSessionSourceFactor int64 = 16
	packageSessionPackageWeight int64 = 64 << 10
)

// PackageSessionStatistics reports how a persistent typed-analysis session
// satisfied package loading requests.
type PackageSessionStatistics struct {
	FullLoads uint64
	IncrementalLoads uint64
	ImportLoads uint64
}

type packageSessionImportLoader func(
	context.Context,
	PackageLoadOptions,
	[]string,
) (PackageLoadResult, error)

type packageSessionKey [sha256.Size]byte

type packageSessionVariant struct {
	root packages.Package
	availableImports map[string]*packages.Package
	rootFiles map[string]struct{}
}

type packageSessionEntry struct {
	variants []packageSessionVariant
	effectFacts *nativeEffectFacts
	rootFiles map[string]struct{}
	rootBuildConstraints map[string][]packageSessionBuildConstraint
	rootIgnoredInputs map[string]packageSessionFileIdentity
	dependencyFiles map[string]struct{}
	dependencyDirs map[string]struct{}
	mutableDependencyFiles map[string]struct{}
	mutableDependencyBuildConstraints map[string][]packageSessionBuildConstraint
	dependencyInputs map[string]packageSessionFileIdentity
	dependencyNames map[string][]string
	directoryNames map[string][]string
	controlFiles map[string]packageSessionFileIdentity
	accountedBytes int64
	used uint64
}

type packageSessionFileIdentity struct {
	digest source.Digest
	exists bool
}

type packageSessionBuildConstraint struct {
	offset int
	raw string
}

// PackageSession owns bounded typed state that may be reused by successive
// editor snapshots. It retains compact dependency type graphs, never
// dependency syntax or type-value maps.
type PackageSession struct {
	mu sync.Mutex
	entries map[packageSessionKey]packageSessionEntry
	clock uint64
	generation uint64
	statistics PackageSessionStatistics
	loadPackages func(context.Context, PackageLoadOptions) (PackageLoadResult, error)
	loadImports packageSessionImportLoader
}

// NewPackageSession creates one empty persistent typed-analysis session.
func NewPackageSession() *PackageSession {
	return &PackageSession{entries: make(map[packageSessionKey]packageSessionEntry)}
}

// Statistics returns one immutable session snapshot.
func (s *PackageSession) Statistics() PackageSessionStatistics {
	if s == nil {
		return PackageSessionStatistics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statistics
}

// InvalidateAll drops every retained typed graph. Editor file notifications
// use this conservative boundary when disk or project metadata may have
// changed outside an exact document overlay.
func (s *PackageSession) InvalidateAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[packageSessionKey]packageSessionEntry)
	s.generation++
}

func (s *PackageSession) load(
	ctx context.Context,
	sourceGoVersion string,
	options PackageLoadOptions,
) (PackageLoadResult, error) {
	if s == nil {
		return LoadPackages(ctx, options)
	}
	key, err := packageSessionIdentity(sourceGoVersion, options)
	if err != nil {
		return PackageLoadResult{}, err
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return PackageLoadResult{}, err
	}
	entry, found := s.entries[key]
	generation := s.generation
	loadPackages := s.loadPackages
	loadImports := s.loadImports
	s.mu.Unlock()
	if found {
		if loadImports == nil {
			loadImports = loadPackageSessionImports
		}
		resolveImports := func(
			ctx context.Context,
			options PackageLoadOptions,
			paths []string,
		) (PackageLoadResult, error) {
			s.mu.Lock()
			s.statistics.ImportLoads++
			s.mu.Unlock()
			selected := packageSessionImportLoadOptions(options, paths)
			return loadImports(ctx, selected, paths)
		}
		loaded, reusable, reloadErr := entry.reload(
			ctx,
			sourceGoVersion,
			options,
			resolveImports,
		)
		if reloadErr != nil {
			return PackageLoadResult{}, reloadErr
		}
		if reusable {
			s.mu.Lock()
			if s.generation != generation {
				generation = s.generation
				loadPackages = s.loadPackages
				s.mu.Unlock()
			} else {
				s.mu.Unlock()
				retained, retain := newPackageSessionEntry(loaded, options)
				if !retain {
					retained, retain = entry.refreshed(loaded, options.Dir)
				}
				s.mu.Lock()
				if s.generation == generation {
					s.statistics.IncrementalLoads++
					s.store(key, retained, retain)
					s.mu.Unlock()
					return loaded, nil
				}
				generation = s.generation
				loadPackages = s.loadPackages
				s.mu.Unlock()
			}
		}
	}
	s.mu.Lock()
	s.statistics.FullLoads++
	s.mu.Unlock()
	if loadPackages == nil {
		loadPackages = LoadPackages
	}
	loaded, err := loadPackages(ctx, options)
	if err != nil {
		s.mu.Lock()
		if s.generation == generation {
			delete(s.entries, key)
		}
		s.mu.Unlock()
		return PackageLoadResult{}, err
	}
	retained, retain := newPackageSessionEntry(loaded, options)
	s.mu.Lock()
	if s.generation == generation {
		s.store(key, retained, retain)
	}
	s.mu.Unlock()
	return loaded, nil
}

func (s *PackageSession) store(key packageSessionKey, entry packageSessionEntry, retain bool) {
	if !retain {
		delete(s.entries, key)
		return
	}
	s.clock++
	entry.used = s.clock
	s.entries[key] = entry
	s.bound()
}

func (s *PackageSession) bound() {
	type candidate struct {
		key packageSessionKey
		entry packageSessionEntry
	}
	candidates := make([]candidate, 0, len(s.entries))
	for key, entry := range s.entries {
		candidates = append(candidates, candidate{key: key, entry: entry})
	}
	sort.Slice(
		candidates,
		func(left, right int) bool {
			if candidates[left].entry.used != candidates[right].entry.used {
				return candidates[left].entry.used > candidates[right].entry.used
			}
			return bytes.Compare(candidates[left].key[:], candidates[right].key[:]) < 0
		},
	)
	bounded := make(map[packageSessionKey]packageSessionEntry)
	var retained int64
	for _, candidate := range candidates {
		if len(bounded) == maximumPackageSessionEntries {
			break
		}
		weight := candidate.entry.accountedBytes
		if weight > maximumPackageSessionBytes ||
			retained > maximumPackageSessionBytes - weight {
			continue
		}
		bounded[candidate.key] = candidate.entry
		retained += weight
	}
	s.entries = bounded
}

func packageSessionIdentity(
	sourceGoVersion string,
	options PackageLoadOptions,
) (packageSessionKey, error) {
	tags := slices.Clone(options.BuildTags)
	slices.Sort(tags)
	tags = slices.Compact(tags)
	type identity struct {
		Dir string
		Patterns []string
		Requirement uint8
		Tests bool
		LoadDependencySyntax bool
		LoadEffectFacts bool
		BuildTags []string
		ModuleMode ModuleMode
		Environment []string
		AllowNetwork bool
		GOOS string
		GOARCH string
		MaxPackages int
		MaxSourceFiles int
		MaxSourceBytes int64
		Contracts []byte
		SourceGoVersion string
	}
	encoded, err := json.Marshal(
		identity{
			Dir: options.Dir,
			Patterns: slices.Clone(options.Patterns),
			Requirement: uint8(options.Requirement),
			Tests: options.Tests,
			LoadDependencySyntax: options.LoadDependencySyntax,
			LoadEffectFacts: options.LoadEffectFacts,
			BuildTags: tags,
			ModuleMode: options.ModuleMode,
			Environment: packageLoadEnvironment(options),
			AllowNetwork: options.AllowNetwork,
			GOOS: options.GOOS,
			GOARCH: options.GOARCH,
			MaxPackages: options.MaxPackages,
			MaxSourceFiles: options.MaxSourceFiles,
			MaxSourceBytes: options.MaxSourceBytes,
			Contracts: options.Contracts.CanonicalBytes(),
			SourceGoVersion: sourceGoVersion,
		},
	)
	if err != nil {
		return packageSessionKey{}, fmt.Errorf("encode package session identity: %w", err)
	}
	return packageSessionKey(sha256.Sum256(encoded)), nil
}

func newPackageSessionEntry(
	loaded PackageLoadResult,
	options PackageLoadOptions,
) (packageSessionEntry, bool) {
	if len(loaded.Diagnostics) != 0 || len(loaded.Sources.problems) != 0 {
		return packageSessionEntry{}, false
	}
	roots, ok := packageSessionRootFamily(loaded.Packages)
	if !ok {
		return packageSessionEntry{}, false
	}
	rootFiles := make(map[string]struct{})
	allRootFiles := make(map[string]struct{})
	rootIgnoredFiles := make(map[string]struct{})
	rootDirectories := make(map[string]struct{})
	loadedNames := make(map[string]map[string]struct{})
	familyDirectory := ""
	for _, root := range roots {
		if !validPackageSessionRoot(root) {
			return packageSessionEntry{}, false
		}
		if familyDirectory == "" {
			familyDirectory = root.Dir
		} else if root.Dir != familyDirectory {
			return packageSessionEntry{}, false
		}
		variantFiles := make(map[string]struct{}, len(root.GoFiles))
		for _, path := range root.GoFiles {
			path = filepath.Clean(path)
			if !filepath.IsAbs(path) || filepath.Dir(path) != root.Dir {
				return packageSessionEntry{}, false
			}
			variantFiles[path] = struct{}{}
			rootFiles[path] = struct{}{}
			allRootFiles[path] = struct{}{}
		}
		for _, path := range root.IgnoredFiles {
			path = filepath.Clean(path)
			if !filepath.IsAbs(path) || filepath.Dir(path) != root.Dir {
				return packageSessionEntry{}, false
			}
			rootIgnoredFiles[path] = struct{}{}
			allRootFiles[path] = struct{}{}
		}
		if len(root.CompiledGoFiles) == 0 {
			return packageSessionEntry{}, false
		}
		for _, path := range root.CompiledGoFiles {
			if _, found := variantFiles[filepath.Clean(path)]; !found {
				return packageSessionEntry{}, false
			}
		}
		rootDirectories[root.Dir] = struct{}{}
		names := loadedNames[root.Dir]
		if names == nil {
			names = make(map[string]struct{})
			loadedNames[root.Dir] = names
		}
		for _, name := range packageSessionLoadedGoNames(root) {
			names[name] = struct{}{}
		}
	}
	if !validPackageSessionVariantImports(roots) {
		return packageSessionEntry{}, false
	}
	for path := range rootFiles {
		delete(rootIgnoredFiles, path)
	}
	buildConstraints := make(map[string][]packageSessionBuildConstraint, len(rootFiles))
	var accounted int64
	for path := range rootFiles {
		file, found := loaded.Sources.Lookup(path)
		if !found || file == nil || !file.CanFormat() {
			return packageSessionEntry{}, false
		}
		buildConstraints[path] = packageSessionBuildConstraints(file)
		size := file.ByteSize()
		if size > (maximumPackageSessionBytes - accounted) / packageSessionSourceFactor {
			return packageSessionEntry{}, false
		}
		accounted += size * packageSessionSourceFactor
	}
	variants := make([]packageSessionVariant, 0, len(roots))
	dependencyFiles := make(map[string]struct{})
	dependencyDirs := make(map[string]struct{})
	mutableDependencyFiles := make(map[string]struct{})
	mutableDependencyIgnoredFiles := make(map[string]struct{})
	mutableDependencyDirs := make(map[string][]string)
	packageCount := 0
	for _, root := range roots {
		compactRoot, variantDependencyFiles, variantDependencyDirs, variantMutableFiles, variantMutableIgnoredFiles, variantMutableDirs, variantPackageCount, ok := compactPackageSessionRoot(
				root,
				options.Dir,
			)
		if !ok {
			return packageSessionEntry{}, false
		}
		availableImports, ok := packageSessionAvailableImports(&compactRoot)
		if !ok {
			return packageSessionEntry{}, false
		}
		variantFiles := make(map[string]struct{}, len(root.GoFiles))
		for _, path := range root.GoFiles {
			variantFiles[filepath.Clean(path)] = struct{}{}
		}
		variants = append(
			variants,
			packageSessionVariant{
				root: compactRoot,
				availableImports: availableImports,
				rootFiles: variantFiles,
			},
		)
		mergePackageSessionSet(dependencyFiles, variantDependencyFiles)
		mergePackageSessionSet(dependencyDirs, variantDependencyDirs)
		mergePackageSessionSet(mutableDependencyFiles, variantMutableFiles)
		mergePackageSessionSet(mutableDependencyIgnoredFiles, variantMutableIgnoredFiles)
		if !mergePackageSessionNames(mutableDependencyDirs, variantMutableDirs) {
			return packageSessionEntry{}, false
		}
		packageCount += variantPackageCount
	}
	if int64(packageCount) >
		(maximumPackageSessionBytes - accounted) / packageSessionPackageWeight {
		return packageSessionEntry{}, false
	}
	accounted += int64(packageCount) * packageSessionPackageWeight
	for path := range allRootFiles {
		delete(dependencyFiles, path)
		delete(mutableDependencyFiles, path)
		delete(mutableDependencyIgnoredFiles, path)
	}
	for directory := range rootDirectories {
		names := loadedNames[directory]
		for _, name := range mutableDependencyDirs[directory] {
			names[name] = struct{}{}
		}
		delete(dependencyDirs, directory)
		delete(mutableDependencyDirs, directory)
	}
	directoryNames := make(map[string][]string, len(rootDirectories))
	controlFiles := make(map[string]packageSessionFileIdentity)
	for directory := range rootDirectories {
		current, err := packageSessionGoNames(directory)
		if err != nil || !equalPackageSessionNameSet(current, loadedNames[directory]) {
			return packageSessionEntry{}, false
		}
		directoryNames[directory] = current
		controls, err := capturePackageSessionControlFiles(options.Dir, directory)
		if err != nil || !mergePackageSessionFileIdentities(controlFiles, controls) {
			return packageSessionEntry{}, false
		}
	}
	ignoredByDirectory := make(map[string][]string)
	for path := range rootIgnoredFiles {
		directory := filepath.Dir(path)
		ignoredByDirectory[directory] = append(ignoredByDirectory[directory], path)
	}
	rootIgnoredInputs := make(map[string]packageSessionFileIdentity)
	for directory, paths := range ignoredByDirectory {
		captured, err := capturePackageSessionFiles(directory, paths)
		if err != nil || !mergePackageSessionFileIdentities(rootIgnoredInputs, captured) {
			return packageSessionEntry{}, false
		}
	}
	dependencyInputs, dependencyNames, err := capturePackageSessionDependencies(
		loaded.Sources,
		mutableDependencyFiles,
		mutableDependencyIgnoredFiles,
		mutableDependencyDirs,
		options.Overlay,
	)
	if err != nil {
		return packageSessionEntry{}, false
	}
	mutableDependencySources := make(map[string]struct{})
	mutableDependencyBuildConstraints := make(map[string][]packageSessionBuildConstraint)
	for path := range mutableDependencyFiles {
		if filepath.Ext(path) != ".go" {
			continue
		}
		file, found := loaded.Sources.Lookup(path)
		if !found || file == nil || !file.CanFormat() {
			return packageSessionEntry{}, false
		}
		mutableDependencySources[path] = struct{}{}
		mutableDependencyBuildConstraints[path] = packageSessionBuildConstraints(file)
	}
	return packageSessionEntry{
		variants: variants,
		effectFacts: cloneNativeEffectFacts(loaded.effectFacts),
		rootFiles: rootFiles,
		rootBuildConstraints: buildConstraints,
		rootIgnoredInputs: rootIgnoredInputs,
		dependencyFiles: dependencyFiles,
		dependencyDirs: dependencyDirs,
		mutableDependencyFiles: mutableDependencySources,
		mutableDependencyBuildConstraints: mutableDependencyBuildConstraints,
		dependencyInputs: dependencyInputs,
		dependencyNames: dependencyNames,
		directoryNames: directoryNames,
		controlFiles: controlFiles,
		accountedBytes: accounted,
	}, true
}

func (entry packageSessionEntry) refreshed(
	loaded PackageLoadResult,
	projectRoot string,
) (packageSessionEntry, bool) {
	if len(loaded.Diagnostics) != 0 || len(loaded.Sources.problems) != 0 {
		return packageSessionEntry{}, false
	}
	roots, ok := packageSessionRootFamily(loaded.Packages)
	if !ok || len(roots) != len(entry.variants) {
		return packageSessionEntry{}, false
	}
	variants := make([]packageSessionVariant, 0, len(roots))
	dependencyFiles := make(map[string]struct{})
	dependencyDirs := make(map[string]struct{})
	packageCount := 0
	for index, root := range roots {
		previous := entry.variants[index]
		if !validPackageSessionRoot(root) ||
			root.ID != previous.root.ID ||
			root.PkgPath != previous.root.PkgPath ||
			root.Name != previous.root.Name ||
			root.ForTest != previous.root.ForTest ||
			root.Dir != previous.root.Dir ||
			len(root.CompiledGoFiles) == 0 {
			return packageSessionEntry{}, false
		}
		rootFiles := make(map[string]struct{}, len(root.GoFiles))
		for _, path := range root.GoFiles {
			rootFiles[filepath.Clean(path)] = struct{}{}
		}
		if !equalPackageSessionSet(rootFiles, previous.rootFiles) {
			return packageSessionEntry{}, false
		}
		for _, path := range root.CompiledGoFiles {
			if _, found := rootFiles[filepath.Clean(path)]; !found {
				return packageSessionEntry{}, false
			}
		}
		compactRoot, variantDependencyFiles, variantDependencyDirs, _, _, _, variantPackageCount, ok := compactPackageSessionRoot(
				root,
				projectRoot,
			)
		if !ok {
			return packageSessionEntry{}, false
		}
		availableImports, ok := packageSessionAvailableImports(&compactRoot)
		if !ok {
			return packageSessionEntry{}, false
		}
		variants = append(
			variants,
			packageSessionVariant{
				root: compactRoot,
				availableImports: availableImports,
				rootFiles: rootFiles,
			},
		)
		mergePackageSessionSet(dependencyFiles, variantDependencyFiles)
		mergePackageSessionSet(dependencyDirs, variantDependencyDirs)
		packageCount += variantPackageCount
	}
	if !validPackageSessionVariantImports(roots) {
		return packageSessionEntry{}, false
	}
	for path := range entry.rootFiles {
		delete(dependencyFiles, path)
	}
	for directory := range entry.directoryNames {
		delete(dependencyDirs, directory)
	}
	if !equalPackageSessionSet(dependencyFiles, entry.dependencyFiles) ||
		!equalPackageSessionSet(dependencyDirs, entry.dependencyDirs) {
		return packageSessionEntry{}, false
	}
	var accounted int64
	for path := range entry.rootFiles {
		file, found := loaded.Sources.Lookup(path)
		if !found || file == nil || !file.CanFormat() {
			return packageSessionEntry{}, false
		}
		size := file.ByteSize()
		if size > (maximumPackageSessionBytes - accounted) / packageSessionSourceFactor {
			return packageSessionEntry{}, false
		}
		accounted += size * packageSessionSourceFactor
	}
	if int64(packageCount) >
		(maximumPackageSessionBytes - accounted) / packageSessionPackageWeight {
		return packageSessionEntry{}, false
	}
	accounted += int64(packageCount) * packageSessionPackageWeight
	entry.variants = variants
	entry.effectFacts = cloneNativeEffectFacts(loaded.effectFacts)
	entry.accountedBytes = accounted
	return entry, true
}

func validPackageSessionRoot(root *packages.Package) bool {
	return root != nil &&
		!root.IllTyped &&
		root.ID != "" &&
		root.Name != "" &&
		root.PkgPath != "" &&
		root.Dir != "" &&
		filepath.IsAbs(root.Dir) &&
		filepath.Clean(root.Dir) == root.Dir &&
		root.Types != nil &&
		root.TypesInfo != nil &&
		root.TypesSizes != nil &&
		root.Fset != nil &&
		len(root.Errors) == 0 &&
		len(root.TypeErrors) == 0
}

func equalPackageSessionSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if _, found := right[value]; !found {
			return false
		}
	}
	return true
}

func packageSessionRootFamily(roots []*packages.Package) ([]*packages.Package, bool) {
	if len(roots) == 0 || len(roots) > 3 {
		return nil, false
	}
	testPath, baseName, ok := packageSessionRootIdentity(roots[0])
	if !ok {
		return nil, false
	}
	result := slices.Clone(roots)
	seenRanks := make(map[int]struct{})
	seenIDs := make(map[string]struct{})
	for _, root := range result {
		rank := packageSessionRootRank(testPath, baseName, root)
		if rank < 0 || root.ID == "" {
			return nil, false
		}
		if _, duplicate := seenRanks[rank]; duplicate {
			return nil, false
		}
		seenRanks[rank] = struct{}{}
		if _, duplicate := seenIDs[root.ID]; duplicate {
			return nil, false
		}
		seenIDs[root.ID] = struct{}{}
	}
	sort.Slice(
		result,
		func(left, right int) bool {
			leftRank := packageSessionRootRank(testPath, baseName, result[left])
			rightRank := packageSessionRootRank(testPath, baseName, result[right])
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			return result[left].ID < result[right].ID
		},
	)
	return result, true
}

func validPackageSessionVariantImports(roots []*packages.Package) bool {
	if len(roots) < 2 {
		return true
	}
	testPath, baseName, ok := packageSessionRootIdentity(roots[0])
	if !ok {
		return false
	}
	rootIDs := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		rootIDs[root.ID] = struct{}{}
	}
	for _, root := range roots {
		if packageSessionRootRank(testPath, baseName, root) != 2 {
			continue
		}
		imported := root.Imports[testPath]
		if imported == nil {
			continue
		}
		if _, selected := rootIDs[imported.ID]; !selected {
			return false
		}
	}
	return true
}

func packageSessionRootIdentity(root *packages.Package) (string, string, bool) {
	if root == nil || root.PkgPath == "" || root.Name == "" {
		return "", "", false
	}
	if root.ForTest == "" {
		return root.PkgPath, root.Name, true
	}
	if root.PkgPath == root.ForTest {
		return root.ForTest, root.Name, true
	}
	if root.PkgPath == root.ForTest + "_test" &&
		strings.HasSuffix(root.Name, "_test") &&
		len(root.Name) > len("_test") {
		return root.ForTest, strings.TrimSuffix(root.Name, "_test"), true
	}
	return "", "", false
}

func packageSessionRootRank(testPath, baseName string, root *packages.Package) int {
	if root == nil {
		return -1
	}
	if root.ForTest == "" && root.PkgPath == testPath && root.Name == baseName {
		return 0
	}
	if root.ForTest != testPath {
		return -1
	}
	if root.PkgPath == testPath && root.Name == baseName {
		return 1
	}
	if root.PkgPath == testPath + "_test" && root.Name == baseName + "_test" {
		return 2
	}
	return -1
}

func mergePackageSessionSet(target, source_ map[string]struct{}) {
	for value := range source_ {
		target[value] = struct{}{}
	}
}

func mergePackageSessionNames(target, source_ map[string][]string) bool {
	for directory, names := range source_ {
		if previous, found := target[directory]; found && !slices.Equal(previous, names) {
			return false
		}
		target[directory] = slices.Clone(names)
	}
	return true
}

func mergePackageSessionFileIdentities(
	target map[string]packageSessionFileIdentity,
	source_ map[string]packageSessionFileIdentity,
) bool {
	for path, identity := range source_ {
		if previous, found := target[path]; found && previous != identity {
			return false
		}
		target[path] = identity
	}
	return true
}

func equalPackageSessionNameSet(names []string, expected map[string]struct{}) bool {
	if len(names) != len(expected) {
		return false
	}
	for _, name := range names {
		if _, found := expected[name]; !found {
			return false
		}
	}
	return true
}

func compactPackageSessionRoot(
	root *packages.Package,
	projectRoot string,
) (
	packages.Package,
	map[string]struct{},
	map[string]struct{},
	map[string]struct{},
	map[string]struct{},
	map[string][]string,
	int,
	bool,
) {
	cloned := make(map[*packages.Package]*packages.Package)
	dependencyFiles := make(map[string]struct{})
	dependencyDirs := make(map[string]struct{})
	mutableDependencyFiles := make(map[string]struct{})
	mutableDependencyIgnoredFiles := make(map[string]struct{})
	mutableDependencyDirs := make(map[string][]string)
	var cloneDependency func(*packages.Package) (*packages.Package, bool)
	cloneDependency = func(pkg *packages.Package) (*packages.Package, bool) {
		if pkg == nil || pkg.ID == "" || pkg.PkgPath == "" || pkg.Types == nil {
			return nil, false
		}
		if existing := cloned[pkg]; existing != nil {
			return existing, true
		}
		copy_ := *pkg
		copy_.Errors = nil
		copy_.TypeErrors = nil
		copy_.Syntax = nil
		copy_.TypesInfo = nil
		copy_.Fset = nil
		copy_.Imports = make(map[string]*packages.Package, len(pkg.Imports))
		cloned[pkg] = &copy_
		mutable := packageSessionDependencyIsMutable(projectRoot, pkg)
		goFiles := make(map[string]struct{}, len(pkg.GoFiles))
		for _, path := range pkg.GoFiles {
			path = filepath.Clean(path)
			goFiles[path] = struct{}{}
			dependencyFiles[path] = struct{}{}
			if mutable {
				mutableDependencyFiles[path] = struct{}{}
			}
		}
		if mutable {
			for _, path := range pkg.IgnoredFiles {
				path = filepath.Clean(path)
				if !filepath.IsAbs(path) || filepath.Dir(path) != pkg.Dir {
					return nil, false
				}
				mutableDependencyIgnoredFiles[path] = struct{}{}
			}
		}
		if mutable {
			if pkg.Dir == "" ||
				!filepath.IsAbs(pkg.Dir) ||
				filepath.Clean(pkg.Dir) != pkg.Dir {
				return nil, false
			}
			if pkg.Module != nil && pkg.Module.GoMod != "" {
				goMod := filepath.Clean(pkg.Module.GoMod)
				if !filepath.IsAbs(goMod) {
					return nil, false
				}
				mutableDependencyFiles[goMod] = struct{}{}
				mutableDependencyFiles[filepath.Join(
					filepath.Dir(goMod),
					"go.sum",
				)] = struct{}{}
			}
			for _, path := range pkg.CompiledGoFiles {
				if _, sourceBacked := goFiles[filepath.Clean(path)]; !sourceBacked {
					return nil, false
				}
			}
		}
		if pkg.Dir != "" {
			directory := filepath.Clean(pkg.Dir)
			dependencyDirs[directory] = struct{}{}
			if mutable {
				expected := packageSessionLoadedGoNames(pkg)
				if previous, found := mutableDependencyDirs[directory];
					found && !slices.Equal(previous, expected) {
					return nil, false
				}
				mutableDependencyDirs[directory] = expected
			}
		}
		paths := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			imported, ok := cloneDependency(pkg.Imports[path])
			if !ok {
				return nil, false
			}
			copy_.Imports[path] = imported
		}
		return &copy_, true
	}
	copy_ := *root
	copy_.Errors = nil
	copy_.TypeErrors = nil
	copy_.Types = nil
	copy_.Fset = nil
	copy_.IllTyped = false
	copy_.Syntax = nil
	copy_.TypesInfo = nil
	copy_.Imports = make(map[string]*packages.Package, len(root.Imports))
	paths := make([]string, 0, len(root.Imports))
	for path := range root.Imports {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		imported, ok := cloneDependency(root.Imports[path])
		if !ok {
			return packages.Package{}, nil, nil, nil, nil, nil, 0, false
		}
		copy_.Imports[path] = imported
	}
	return copy_, dependencyFiles, dependencyDirs, mutableDependencyFiles, mutableDependencyIgnoredFiles, mutableDependencyDirs, len(
		cloned,
	) +
		1, true
}

func packageSessionDependencyIsMutable(root string, pkg *packages.Package) bool {
	if pkg == nil {
		return false
	}
	if packageSessionPathWithin(root, pkg.Dir) {
		return true
	}
	module := pkg.Module
	if module == nil {
		return false
	}
	if module.Main {
		return true
	}
	return module.Replace != nil && module.Replace.Dir != "" && module.Replace.Version == ""
}

func packageSessionAvailableImports(root *packages.Package) (map[string]*packages.Package, bool) {
	if root == nil || root.PkgPath == "" {
		return nil, false
	}
	result := make(map[string]*packages.Package)
	seen := make(map[*packages.Package]struct{})
	stack := []*packages.Package{root}
	for len(stack) > 0 {
		pkg := stack[len(stack) - 1]
		stack = stack[:len(stack) - 1]
		if pkg == nil {
			return nil, false
		}
		if _, found := seen[pkg]; found {
			continue
		}
		seen[pkg] = struct{}{}
		paths := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			imported := pkg.Imports[path]
			if imported == nil || imported.Types == nil || imported.PkgPath == "" {
				return nil, false
			}
			if pkg == root || packageSessionImportAllowed(root.PkgPath, path) {
				if previous := result[path];
					previous != nil && previous.Types != imported.Types {
					return nil, false
				}
				result[path] = imported
			}
			stack = append(stack, imported)
		}
	}
	return result, true
}

func packageSessionImportAllowed(rootPath, importPath string) bool {
	if rootPath == "" || importPath == "" || importPath == rootPath {
		return false
	}
	if importPath == "vendor" ||
		strings.HasPrefix(importPath, "vendor/") ||
		strings.Contains(importPath, "/vendor/") ||
		strings.HasSuffix(importPath, "/vendor") {
		return false
	}
	if strings.HasPrefix(importPath, "internal/") || importPath == "internal" {
		return false
	}
	marker := "/internal/"
	index := strings.LastIndex(importPath, marker)
	if index < 0 && strings.HasSuffix(importPath, "/internal") {
		index = len(importPath) - len("/internal")
	}
	if index < 0 {
		return true
	}
	parent := importPath[:index]
	return rootPath == parent || strings.HasPrefix(rootPath, parent + "/")
}

func loadPackageSessionImports(
	ctx context.Context,
	options PackageLoadOptions,
	_ []string,
) (PackageLoadResult, error) {
	return LoadPackages(ctx, options)
}

func packageSessionImportLoadOptions(
	options PackageLoadOptions,
	paths []string,
) PackageLoadOptions {
	selected := clonePackageLoadOptions(options)
	exact := slices.Clone(paths)
	slices.Sort(exact)
	exact = slices.Compact(exact)
	selected.Patterns = make([]string, len(exact))
	for index, path := range exact {
		selected.Patterns[index] = "pattern=" + path
	}
	selected.Requirement = rules.RequireTypes
	selected.Tests = false
	selected.LoadDependencySyntax = false
	selected.LoadEffectFacts = false
	return selected
}

func packageSessionResolvedImports(
	loaded PackageLoadResult,
	paths []string,
) (map[string]*packages.Package, bool) {
	if len(loaded.Diagnostics) != 0 || len(loaded.Sources.problems) != 0 {
		return nil, false
	}
	resolved := make(map[string]*packages.Package, len(loaded.Packages))
	for _, pkg := range loaded.Packages {
		if !validPackageSessionRoot(pkg) || pkg.ForTest != "" {
			return nil, false
		}
		if previous := resolved[pkg.PkgPath]; previous != nil && previous.ID != pkg.ID {
			return nil, false
		}
		resolved[pkg.PkgPath] = pkg
	}
	for _, path := range paths {
		if resolved[path] == nil {
			return nil, false
		}
	}
	return resolved, true
}

func (entry packageSessionEntry) reload(
	ctx context.Context,
	sourceGoVersion string,
	options PackageLoadOptions,
	loadImports packageSessionImportLoader,
) (PackageLoadResult, bool, error) {
	if options.LoadDependencySyntax {
		return PackageLoadResult{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return PackageLoadResult{}, false, err
	}
	currentControls := make(map[string]packageSessionFileIdentity)
	for directory, expected := range entry.directoryNames {
		currentNames, err := packageSessionGoNames(directory)
		if err != nil || !slices.Equal(currentNames, expected) {
			return PackageLoadResult{}, false, nil
		}
		controls, err := capturePackageSessionControlFiles(options.Dir, directory)
		if err != nil || !mergePackageSessionFileIdentities(currentControls, controls) {
			return PackageLoadResult{}, false, nil
		}
	}
	if !equalPackageSessionControlFiles(currentControls, entry.controlFiles) {
		return PackageLoadResult{}, false, nil
	}
	dependencySources, dependencyBytes, changedDependencies, current, dependencySourceErr := entry.loadDependencySources(
		ctx,
		options,
	)
	if dependencySourceErr != nil {
		return PackageLoadResult{}, false, dependencySourceErr
	}
	if !current {
		return PackageLoadResult{}, false, nil
	}
	if !packageSessionDependenciesCurrent(entry.rootIgnoredInputs, nil) {
		return PackageLoadResult{}, false, nil
	}
	for path := range options.Overlay {
		if _, dependency := entry.dependencyFiles[path]; dependency {
			if _, mutable := entry.mutableDependencyFiles[path]; !mutable {
				return PackageLoadResult{}, false, nil
			}
			continue
		}
		if _, dependency := entry.dependencyDirs[filepath.Dir(path)]; dependency {
			return PackageLoadResult{}, false, nil
		}
		if _, rootDirectory := entry.directoryNames[filepath.Dir(path)]; rootDirectory {
			if _, rootFile := entry.rootFiles[path]; !rootFile {
				return PackageLoadResult{}, false, nil
			}
		}
	}
	files := make(map[string]*source.File, len(entry.rootFiles))
	bytesByPath := make(map[string][]byte, len(entry.rootFiles))
	paths := make([]string, 0, len(entry.rootFiles))
	for path := range entry.rootFiles {
		if err := ctx.Err(); err != nil {
			return PackageLoadResult{}, false, err
		}
		input, overlaid := options.Overlay[path]
		if !overlaid {
			var readErr error
			input, readErr = source.ReadFile(path)
			if readErr != nil {
				return PackageLoadResult{}, false, nil
			}
		}
		physical, sourceErr := source.Load(path, input)
		if sourceErr != nil || physical == nil || !physical.CanFormat() {
			return PackageLoadResult{}, false, nil
		}
		if !slices.Equal(
			packageSessionBuildConstraints(physical),
			entry.rootBuildConstraints[path],
		) {
			return PackageLoadResult{}, false, nil
		}
		files[path] = physical
		bytesByPath[path] = physical.Bytes()
		paths = append(paths, path)
	}
	slices.Sort(paths)
	variants := slices.Clone(entry.variants)
	missingByVariant := make([][]string, len(variants))
	missingSet := make(map[string]struct{})
	for index := range variants {
		missing, valid := variants[index].missingImports(bytesByPath)
		if !valid {
			return PackageLoadResult{}, false, nil
		}
		missingByVariant[index] = missing
		for _, path := range missing {
			missingSet[path] = struct{}{}
		}
	}
	sources := PackageSourceSet{paths: paths, files: files}
	mergedSources, mergeErr := MergePackageSourceSets(sources, dependencySources)
	if mergeErr != nil {
		return PackageLoadResult{}, false, nil
	}
	sources = mergedSources
	if len(missingSet) > 0 {
		if loadImports == nil {
			return PackageLoadResult{}, false, nil
		}
		missing := make([]string, 0, len(missingSet))
		for path := range missingSet {
			missing = append(missing, path)
		}
		slices.Sort(missing)
		loadedImports, importErr := loadImports(ctx, options, missing)
		if contextErr := ctx.Err(); contextErr != nil {
			return PackageLoadResult{}, false, contextErr
		}
		if importErr != nil {
			return PackageLoadResult{}, false, nil
		}
		resolved, valid := packageSessionResolvedImports(loadedImports, missing)
		if !valid {
			return PackageLoadResult{}, false, nil
		}
		merged, mergeErr := MergePackageSourceSets(sources, loadedImports.Sources)
		if mergeErr != nil {
			return PackageLoadResult{}, false, nil
		}
		sources = merged
		for index, variantMissing := range missingByVariant {
			if len(variantMissing) == 0 {
				continue
			}
			available := make(
				map[string]*packages.Package,
				len(variants[index].availableImports) + len(variantMissing),
			)
			for path, imported := range variants[index].availableImports {
				available[path] = imported
			}
			for _, path := range variantMissing {
				available[path] = resolved[path]
			}
			variants[index].availableImports = available
		}
	}
	withinLimits, limitErr := packageSessionSourcesWithinLimits(sources, options)
	if limitErr != nil {
		return PackageLoadResult{}, false, limitErr
	}
	if !withinLimits {
		return PackageLoadResult{}, false, nil
	}
	freshByID, reusable, dependencyErr := reloadPackageSessionDependencies(
		ctx,
		sourceGoVersion,
		options.Dir,
		variants,
		dependencyBytes,
		changedDependencies,
		options.LoadEffectFacts,
	)
	if dependencyErr != nil || !reusable {
		return PackageLoadResult{}, reusable, dependencyErr
	}
	packages_ := make([]*packages.Package, 0, len(entry.variants))
	for _, variant := range variants {
		root, reusable, err := variant.reload(ctx, sourceGoVersion, bytesByPath, freshByID)
		if err != nil || !reusable {
			return PackageLoadResult{}, reusable, err
		}
		freshByID[root.ID] = root
		packages_ = append(packages_, root)
	}
	effectFacts := cloneNativeEffectFacts(entry.effectFacts)
	if options.LoadEffectFacts && len(changedDependencies) > 0 {
		var effectErr error
		effectFacts, effectErr = rebuildPackageSessionEffectFacts(
			ctx,
			options,
			packages_,
			freshByID,
		)
		if effectErr != nil {
			return PackageLoadResult{}, false, effectErr
		}
	}
	return PackageLoadResult{
		Requirement: options.Requirement,
		Packages: packages_,
		Diagnostics: []PackageDiagnostic{},
		Sources: sources,
		effectFacts: effectFacts,
	}, true, nil
}

func packageSessionSourcesWithinLimits(
	sources PackageSourceSet,
	options PackageLoadOptions,
) (bool, error) {
	limits, err := resolvePackageResourceLimits(options)
	if err != nil {
		return false, err
	}
	paths := sources.Paths()
	if len(paths) > limits.maxSourceFiles {
		return false, nil
	}
	var bytes int64
	for _, path := range paths {
		file, found := sources.Lookup(path)
		if !found || file == nil {
			return false, nil
		}
		if file.ByteSize() > limits.maxSourceBytes - bytes {
			return false, nil
		}
		bytes += file.ByteSize()
	}
	return true, nil
}

func (entry packageSessionEntry) loadDependencySources(
	ctx context.Context,
	options PackageLoadOptions,
) (PackageSourceSet, map[string][]byte, map[string]struct{}, bool, error) {
	if !packageSessionDependencyNamesCurrent(entry.dependencyNames) {
		return PackageSourceSet{}, nil, nil, false, nil
	}
	paths := make([]string, 0, len(entry.mutableDependencyFiles))
	for path := range entry.mutableDependencyFiles {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	files := make(map[string]*source.File, len(paths))
	bytesByPath := make(map[string][]byte, len(paths))
	changed := make(map[string]struct{})
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return PackageSourceSet{}, nil, nil, false, err
		}
		input, overlaid := options.Overlay[path]
		if !overlaid {
			var err error
			input, err = source.ReadFile(path)
			if err != nil {
				return PackageSourceSet{}, nil, nil, false, nil
			}
		}
		physical, err := source.Load(path, input)
		if err != nil || physical == nil || !physical.CanFormat() {
			return PackageSourceSet{}, nil, nil, false, nil
		}
		if !slices.Equal(
			packageSessionBuildConstraints(physical),
			entry.mutableDependencyBuildConstraints[path],
		) {
			return PackageSourceSet{}, nil, nil, false, nil
		}
		identity := entry.dependencyInputs[path]
		if !identity.exists || physical.Digest() != identity.digest {
			changed[path] = struct{}{}
		}
		files[path] = physical
		bytesByPath[path] = physical.Bytes()
	}
	for path, expected := range entry.dependencyInputs {
		if _, active := entry.mutableDependencyFiles[path]; active {
			continue
		}
		input, err := source.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			if expected.exists {
				return PackageSourceSet{}, nil, nil, false, nil
			}
			continue
		}
		if err != nil ||
			!expected.exists ||
			source.Digest(sha256.Sum256(input)) != expected.digest {
			return PackageSourceSet{}, nil, nil, false, nil
		}
	}
	return PackageSourceSet{paths: paths, files: files}, bytesByPath, changed, true, nil
}

func packageSessionDependencyNamesCurrent(directories map[string][]string) bool {
	for directory, expected := range directories {
		current, err := packageSessionGoNames(directory)
		if err != nil || !slices.Equal(current, expected) {
			return false
		}
	}
	return true
}

func reloadPackageSessionDependencies(
	ctx context.Context,
	sourceGoVersion string,
	projectRoot string,
	variants []packageSessionVariant,
	bytesByPath map[string][]byte,
	changedPaths map[string]struct{},
	refreshAllMutable bool,
) (map[string]*packages.Package, bool, error) {
	freshByID := make(map[string]*packages.Package)
	if len(changedPaths) == 0 {
		return freshByID, true, nil
	}
	directlyChanged := make(map[string]struct{})
	seen := make(map[string]struct{})
	stack := make([]*packages.Package, 0)
	for _, variant := range variants {
		for _, imported := range variant.root.Imports {
			stack = append(stack, imported)
		}
	}
	for len(stack) > 0 {
		pkg := stack[len(stack) - 1]
		stack = stack[:len(stack) - 1]
		if pkg == nil || pkg.ID == "" {
			return nil, false, nil
		}
		if _, visited := seen[pkg.ID]; visited {
			continue
		}
		seen[pkg.ID] = struct{}{}
		for _, path := range pkg.GoFiles {
			if _, changed := changedPaths[filepath.Clean(path)]; changed {
				directlyChanged[pkg.ID] = struct{}{}
			}
		}
		if refreshAllMutable && packageSessionDependencyIsMutable(projectRoot, pkg) {
			directlyChanged[pkg.ID] = struct{}{}
		}
		for _, imported := range pkg.Imports {
			stack = append(stack, imported)
		}
	}
	if len(directlyChanged) == 0 {
		return nil, false, nil
	}
	type visitState uint8
	const (
		visitActive visitState = iota + 1
		visitComplete
	)
	states := make(map[string]visitState)
	impacted := make(map[string]bool)
	var refresh func(*packages.Package) (bool, bool, error)
	refresh = func(pkg *packages.Package) (bool, bool, error) {
		if pkg == nil || pkg.ID == "" {
			return false, false, nil
		}
		switch states[pkg.ID] {
		case visitActive:
			return false, false, nil
		case visitComplete:
			return impacted[pkg.ID], true, nil
		}
		states[pkg.ID] = visitActive
		changed := false
		if _, direct := directlyChanged[pkg.ID]; direct {
			changed = true
		}
		paths := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			importChanged, valid, err := refresh(pkg.Imports[path])
			if err != nil || !valid {
				return false, valid, err
			}
			changed = changed || importChanged
		}
		if changed {
			if !packageSessionDependencyIsMutable(projectRoot, pkg) {
				return false, false, nil
			}
			fresh, valid, err := reloadPackageSessionPackage(
				ctx,
				sourceGoVersion,
				pkg,
				bytesByPath,
				freshByID,
			)
			if err != nil || !valid {
				return false, valid, err
			}
			freshByID[pkg.ID] = fresh
		}
		states[pkg.ID] = visitComplete
		impacted[pkg.ID] = changed
		return changed, true, nil
	}
	for _, variant := range variants {
		paths := make([]string, 0, len(variant.root.Imports))
		for path := range variant.root.Imports {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			_, valid, err := refresh(variant.root.Imports[path])
			if err != nil || !valid {
				return nil, valid, err
			}
		}
	}
	return freshByID, true, nil
}

func rebuildPackageSessionEffectFacts(
	ctx context.Context,
	options PackageLoadOptions,
	roots []*packages.Package,
	freshByID map[string]*packages.Package,
) (*nativeEffectFacts, error) {
	facts := newNativeEffectFacts()
	resolved, err := contracts.Resolve(options.Contracts, effectTypePackages(roots))
	if err != nil {
		return nil, fmt.Errorf("resolve incremental project semantic contracts: %w", err)
	}
	facts.addContracts(resolved)
	prefixes := effectModulePrefixes(roots)
	selected := make([]*packages.Package, 0, len(freshByID))
	seen := make(map[string]struct{})
	rootIDs := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root != nil {
			rootIDs[root.ID] = struct{}{}
		}
	}
	for id, pkg := range freshByID {
		if pkg == nil || pkg.ID != id || !effectPathWithinModules(pkg.PkgPath, prefixes) {
			continue
		}
		if _, root := rootIDs[pkg.ID]; root {
			continue
		}
		if _, duplicate := seen[pkg.ID]; duplicate {
			continue
		}
		seen[pkg.ID] = struct{}{}
		selected = append(selected, pkg)
	}
	slices.SortFunc(
		selected,
		func(left, right *packages.Package) int {
			if compared := strings.Compare(left.PkgPath, right.PkgPath); compared != 0 {
				return compared
			}
			return strings.Compare(left.ID, right.ID)
		},
	)
	if len(selected) == 0 {
		return facts, nil
	}
	noReturns := newNoReturnAnalysis(ctx, selected, facts)
	noReturns.buildAll()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts.addNoReturns(noReturns)
	parameterEffects := newParameterEffectAnalysis(ctx, selected, facts, noReturns)
	parameterEffects.buildAll()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts.addParameterEffects(parameterEffects)
	managedResults := newManagedResultAnalysis(ctx, selected, facts, noReturns)
	managedResults.buildAll()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts.addCleanupManagedResults(managedResults)
	returnStates := newReturnStateAnalysis(ctx, selected, facts, noReturns)
	returnStates.buildAll()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts.addReturnStates(returnStates)
	facts.addResultStates(returnStates)
	return facts, nil
}

func reloadPackageSessionPackage(
	ctx context.Context,
	sourceGoVersion string,
	pkg *packages.Package,
	bytesByPath map[string][]byte,
	freshByID map[string]*packages.Package,
) (*packages.Package, bool, error) {
	if pkg == nil || pkg.TypesSizes == nil || len(pkg.CompiledGoFiles) == 0 {
		return nil, false, nil
	}
	fileSet := token.NewFileSet()
	syntax := make([]*ast.File, 0, len(pkg.CompiledGoFiles))
	usedImports := make(map[string]*packages.Package)
	importer := packageSessionImporter{packages: pkg.Imports, freshByID: freshByID}
	for _, path := range pkg.CompiledGoFiles {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		input, found := bytesByPath[filepath.Clean(path)]
		if !found {
			return nil, false, nil
		}
		parsed, err := parser.ParseFile(
			fileSet,
			path,
			input,
			parser.ParseComments | parser.SkipObjectResolution,
		)
		if err != nil ||
			parsed == nil ||
			parsed.Name == nil ||
			parsed.Name.Name != pkg.Name {
			return nil, false, nil
		}
		for _, specification := range parsed.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err != nil || importPath == "C" {
				return nil, false, nil
			}
			imported := importer.packageFor(importPath)
			if imported == nil || imported.Types == nil {
				return nil, false, nil
			}
			usedImports[importPath] = imported
		}
		syntax = append(syntax, parsed)
	}
	information := newPackageSessionTypesInfo()
	configuration := types.Config{
		Importer: importer,
		Sizes: pkg.TypesSizes,
		GoVersion: packageSessionGoVersion(sourceGoVersion),
	}
	checked, typeErr := configuration.Check(pkg.PkgPath, fileSet, syntax, information)
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if typeErr != nil || checked == nil {
		return nil, false, nil
	}
	fresh := *pkg
	fresh.Types = checked
	fresh.Fset = fileSet
	fresh.Syntax = syntax
	fresh.TypesInfo = information
	fresh.Imports = usedImports
	fresh.Errors = nil
	fresh.TypeErrors = nil
	fresh.IllTyped = false
	return &fresh, true, nil
}

func newPackageSessionTypesInfo() *types.Info {
	return &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Instances: make(map[*ast.Ident]types.Instance),
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
		Implicits: make(map[ast.Node]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		FileVersions: make(map[*ast.File]string),
	}
}

func (variant packageSessionVariant) missingImports(
	bytesByPath map[string][]byte,
) ([]string, bool) {
	missing := make([]string, 0)
	seen := make(map[string]struct{})
	for _, path := range variant.root.CompiledGoFiles {
		input, found := bytesByPath[path]
		if !found {
			return nil, false
		}
		parsed, err := parser.ParseFile(
			token.NewFileSet(),
			path,
			input,
			parser.ImportsOnly | parser.SkipObjectResolution,
		)
		if err != nil ||
			parsed == nil ||
			parsed.Name == nil ||
			parsed.Name.Name != variant.root.Name {
			return nil, false
		}
		for _, specification := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(specification.Path.Value)
			if unquoteErr != nil || importPath == "C" {
				return nil, false
			}
			if variant.availableImports[importPath] != nil {
				continue
			}
			if !packageSessionImportAllowed(variant.root.PkgPath, importPath) {
				return nil, false
			}
			if _, found := seen[importPath]; found {
				continue
			}
			seen[importPath] = struct{}{}
			missing = append(missing, importPath)
		}
	}
	slices.Sort(missing)
	return missing, true
}

func (variant packageSessionVariant) reload(
	ctx context.Context,
	sourceGoVersion string,
	bytesByPath map[string][]byte,
	freshByID map[string]*packages.Package,
) (*packages.Package, bool, error) {
	fileSet := token.NewFileSet()
	syntax := make([]*ast.File, 0, len(variant.root.CompiledGoFiles))
	usedImports := make(map[string]*packages.Package)
	importer := packageSessionImporter{packages: variant.availableImports, freshByID: freshByID}
	for _, path := range variant.root.CompiledGoFiles {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		input, found := bytesByPath[path]
		if !found {
			return nil, false, nil
		}
		parsed, parseErr := parser.ParseFile(
			fileSet,
			path,
			input,
			parser.ParseComments | parser.SkipObjectResolution,
		)
		if parseErr != nil ||
			parsed == nil ||
			parsed.Name == nil ||
			parsed.Name.Name != variant.root.Name {
			return nil, false, nil
		}
		for _, specification := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(specification.Path.Value)
			if unquoteErr != nil || importPath == "C" {
				return nil, false, nil
			}
			imported := importer.packageFor(importPath)
			if imported == nil || imported.Types == nil {
				return nil, false, nil
			}
			usedImports[importPath] = imported
		}
		syntax = append(syntax, parsed)
	}
	information := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Instances: make(map[*ast.Ident]types.Instance),
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
		Implicits: make(map[ast.Node]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		FileVersions: make(map[*ast.File]string),
	}
	configuration := types.Config{
		Importer: importer,
		Sizes: variant.root.TypesSizes,
		GoVersion: packageSessionGoVersion(sourceGoVersion),
	}
	checked, typeErr := configuration.Check(variant.root.PkgPath, fileSet, syntax, information)
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if typeErr != nil || checked == nil {
		return nil, false, nil
	}
	root := variant.root
	root.Types = checked
	root.Fset = fileSet
	root.Syntax = syntax
	root.TypesInfo = information
	root.Imports = usedImports
	root.Errors = nil
	root.TypeErrors = nil
	root.IllTyped = false
	return &root, true, nil
}

func packageSessionBuildConstraints(file *source.File) []packageSessionBuildConstraint {
	if file == nil {
		return nil
	}
	result := make([]packageSessionBuildConstraint, 0)
	for _, directive := range file.Directives() {
		if directive.Kind == source.DirectiveBuildConstraint {
			result = append(
				result,
				packageSessionBuildConstraint{
					offset: directive.Range.Start,
					raw: directive.Raw,
				},
			)
		}
	}
	return result
}

type packageSessionImporter struct {
	packages map[string]*packages.Package
	freshByID map[string]*packages.Package
}

func (i packageSessionImporter) packageFor(path string) *packages.Package {
	imported := i.packages[path]
	if imported == nil {
		return nil
	}
	if fresh := i.freshByID[imported.ID]; fresh != nil {
		return fresh
	}
	return imported
}

func (i packageSessionImporter) Import(path string) (*types.Package, error) {
	return i.ImportFrom(path, "", 0)
}

func (i packageSessionImporter) ImportFrom(
	path string,
	directory string,
	mode types.ImportMode,
) (*types.Package, error) {
	if imported := i.packageFor(path); imported != nil && imported.Types != nil {
		return imported.Types, nil
	}
	return nil, fmt.Errorf("incremental package import %q is unavailable", path)
}

var _ types.ImporterFrom = packageSessionImporter{}

func packageSessionGoVersion(value string) string {
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "go") {
		value = "go" + value
	}
	return goversion.Lang(value)
}

func packageSessionGoNames(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".go" {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

func packageSessionLoadedGoNames(pkg *packages.Package) []string {
	if pkg == nil {
		return nil
	}
	names := make([]string, 0, len(pkg.GoFiles) + len(pkg.IgnoredFiles))
	for _, path := range append(slices.Clone(pkg.GoFiles), pkg.IgnoredFiles...) {
		if filepath.Ext(path) == ".go" {
			names = append(names, filepath.Base(path))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func capturePackageSessionControlFiles(
	projectRoot string,
	packageDirectory string,
) (map[string]packageSessionFileIdentity, error) {
	paths := []string{
		filepath.Join(projectRoot, "go.mod"),
		filepath.Join(projectRoot, "go.sum"),
		filepath.Join(projectRoot, "go.work"),
		filepath.Join(projectRoot, "go.work.sum"),
	}
	moduleRoot := packageDirectory
	for {
		_, err := source.ReadFile(filepath.Join(moduleRoot, "go.mod"))
		if err == nil {
			paths = append(
				paths,
				filepath.Join(moduleRoot, "go.mod"),
				filepath.Join(moduleRoot, "go.sum"),
			)
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(moduleRoot)
		if parent == moduleRoot {
			break
		}
		moduleRoot = parent
	}
	result := make(map[string]packageSessionFileIdentity)
	for _, path := range paths {
		if _, duplicate := result[path]; duplicate {
			continue
		}
		contents, err := source.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			result[path] = packageSessionFileIdentity{}
			continue
		}
		if err != nil {
			return nil, err
		}
		result[path] = packageSessionFileIdentity{
			digest: source.Digest(sha256.Sum256(contents)),
			exists: true,
		}
	}
	return result, nil
}

func equalPackageSessionControlFiles(left, right map[string]packageSessionFileIdentity) bool {
	if len(left) != len(right) {
		return false
	}
	for path, identity := range left {
		if right[path] != identity {
			return false
		}
	}
	return true
}

func capturePackageSessionDependencies(
	sources PackageSourceSet,
	files map[string]struct{},
	ignoredFiles map[string]struct{},
	directories map[string][]string,
	overlay map[string][]byte,
) (map[string]packageSessionFileIdentity, map[string][]string, error) {
	inputs := make(map[string]packageSessionFileIdentity)
	for path := range files {
		contents, overlaid := overlay[path]
		if !overlaid {
			var err error
			contents, err = source.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) && filepath.Base(path) == "go.sum" {
				inputs[path] = packageSessionFileIdentity{}
				continue
			}
			if err != nil {
				return nil, nil, err
			}
		}
		digest := source.Digest(sha256.Sum256(contents))
		file, found := sources.Lookup(path)
		if filepath.Ext(path) == ".go" && !found {
			return nil, nil, fmt.Errorf(
				"mutable dependency source %q was not captured by the package load",
				path,
			)
		}
		if found && file.Digest() != digest {
			return nil, nil, fmt.Errorf(
				"mutable dependency %q changed during package load",
				path,
			)
		}
		inputs[path] = packageSessionFileIdentity{digest: digest, exists: true}
	}
	for path := range ignoredFiles {
		contents, err := source.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		inputs[path] = packageSessionFileIdentity{
			digest: source.Digest(sha256.Sum256(contents)),
			exists: true,
		}
	}
	names := make(map[string][]string)
	for directory, expected := range directories {
		current, err := packageSessionGoNames(directory)
		if err != nil {
			return nil, nil, err
		}
		if !slices.Equal(current, expected) {
			return nil, nil, fmt.Errorf(
				"mutable dependency Go-file membership changed during package load",
			)
		}
		names[directory] = current
	}
	return inputs, names, nil
}

func capturePackageSessionFiles(
	directory string,
	paths []string,
) (map[string]packageSessionFileIdentity, error) {
	result := make(map[string]packageSessionFileIdentity, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) || filepath.Dir(path) != directory {
			return nil, fmt.Errorf(
				"package session file %q is outside %q",
				path,
				directory,
			)
		}
		contents, err := source.ReadFile(path)
		if err != nil {
			return nil, err
		}
		result[path] = packageSessionFileIdentity{
			digest: source.Digest(sha256.Sum256(contents)),
			exists: true,
		}
	}
	return result, nil
}

func packageSessionDependenciesCurrent(
	inputs map[string]packageSessionFileIdentity,
	directories map[string][]string,
) bool {
	for path, expected := range inputs {
		contents, err := source.ReadFile(path)
		if err != nil ||
			!expected.exists ||
			source.Digest(sha256.Sum256(contents)) != expected.digest {
			return false
		}
	}
	for directory, expected := range directories {
		current, err := packageSessionGoNames(directory)
		if err != nil || !slices.Equal(current, expected) {
			return false
		}
	}
	return true
}

func packageSessionPathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".." + string(filepath.Separator))
}
