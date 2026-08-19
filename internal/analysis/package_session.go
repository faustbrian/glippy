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
}

type packageSessionKey [sha256.Size]byte

type packageSessionEntry struct {
	root packages.Package
	availableImports map[string]*packages.Package
	effectFacts *nativeEffectFacts
	rootFiles map[string]struct{}
	rootBuildConstraints map[string][]packageSessionBuildConstraint
	rootIgnoredInputs map[string]packageSessionFileIdentity
	dependencyFiles map[string]struct{}
	dependencyDirs map[string]struct{}
	dependencyInputs map[string]packageSessionFileIdentity
	dependencyNames map[string][]string
	directoryNames []string
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
	s.mu.Unlock()
	if found {
		loaded, reusable, reloadErr := entry.reload(ctx, sourceGoVersion, options)
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
	if len(loaded.Packages) != 1 ||
		len(loaded.Diagnostics) != 0 ||
		len(loaded.Sources.problems) != 0 {
		return packageSessionEntry{}, false
	}
	root := loaded.Packages[0]
	if root == nil ||
		root.ForTest != "" ||
		root.IllTyped ||
		root.Types == nil ||
		root.TypesInfo == nil ||
		root.TypesSizes == nil ||
		root.Fset == nil ||
		len(root.Errors) != 0 ||
		len(root.TypeErrors) != 0 {
		return packageSessionEntry{}, false
	}
	rootFiles := make(map[string]struct{}, len(root.GoFiles))
	compiled := make(map[string]struct{}, len(root.CompiledGoFiles))
	for _, path := range root.GoFiles {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) || filepath.Dir(path) != root.Dir {
			return packageSessionEntry{}, false
		}
		rootFiles[path] = struct{}{}
	}
	for _, path := range root.CompiledGoFiles {
		path = filepath.Clean(path)
		if _, found := rootFiles[path]; !found {
			return packageSessionEntry{}, false
		}
		compiled[path] = struct{}{}
	}
	if len(compiled) == 0 {
		return packageSessionEntry{}, false
	}
	compactRoot, dependencyFiles, dependencyDirs, mutableDependencyFiles, mutableDependencyIgnoredFiles, mutableDependencyDirs, packageCount, ok := compactPackageSessionRoot(
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
	if int64(packageCount) >
		(maximumPackageSessionBytes - accounted) / packageSessionPackageWeight {
		return packageSessionEntry{}, false
	}
	accounted += int64(packageCount) * packageSessionPackageWeight
	directoryNames, err := packageSessionGoNames(root.Dir)
	if err != nil || !slices.Equal(directoryNames, packageSessionLoadedGoNames(root)) {
		return packageSessionEntry{}, false
	}
	controlFiles, err := capturePackageSessionControlFiles(options.Dir, root.Dir)
	if err != nil {
		return packageSessionEntry{}, false
	}
	rootIgnoredInputs, err := capturePackageSessionFiles(root.Dir, root.IgnoredFiles)
	if err != nil {
		return packageSessionEntry{}, false
	}
	dependencyInputs, dependencyNames, err := capturePackageSessionDependencies(
		loaded.Sources,
		mutableDependencyFiles,
		mutableDependencyIgnoredFiles,
		mutableDependencyDirs,
	)
	if err != nil {
		return packageSessionEntry{}, false
	}
	return packageSessionEntry{
		root: compactRoot,
		availableImports: availableImports,
		effectFacts: cloneNativeEffectFacts(loaded.effectFacts),
		rootFiles: rootFiles,
		rootBuildConstraints: buildConstraints,
		rootIgnoredInputs: rootIgnoredInputs,
		dependencyFiles: dependencyFiles,
		dependencyDirs: dependencyDirs,
		dependencyInputs: dependencyInputs,
		dependencyNames: dependencyNames,
		directoryNames: directoryNames,
		controlFiles: controlFiles,
		accountedBytes: accounted,
	}, true
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

func (entry packageSessionEntry) reload(
	ctx context.Context,
	sourceGoVersion string,
	options PackageLoadOptions,
) (PackageLoadResult, bool, error) {
	if options.LoadDependencySyntax {
		return PackageLoadResult{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return PackageLoadResult{}, false, err
	}
	currentNames, err := packageSessionGoNames(entry.root.Dir)
	if err != nil || !slices.Equal(currentNames, entry.directoryNames) {
		return PackageLoadResult{}, false, nil
	}
	currentControls, err := capturePackageSessionControlFiles(options.Dir, entry.root.Dir)
	if err != nil || !equalPackageSessionControlFiles(currentControls, entry.controlFiles) {
		return PackageLoadResult{}, false, nil
	}
	if !packageSessionDependenciesCurrent(entry.dependencyInputs, entry.dependencyNames) {
		return PackageLoadResult{}, false, nil
	}
	if !packageSessionDependenciesCurrent(entry.rootIgnoredInputs, nil) {
		return PackageLoadResult{}, false, nil
	}
	for path := range options.Overlay {
		if _, dependency := entry.dependencyFiles[path]; dependency {
			return PackageLoadResult{}, false, nil
		}
		if _, dependency := entry.dependencyDirs[filepath.Dir(path)]; dependency {
			return PackageLoadResult{}, false, nil
		}
		if filepath.Dir(path) == entry.root.Dir {
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
			input, err = source.ReadFile(path)
			if err != nil {
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
	fileSet := token.NewFileSet()
	syntax := make([]*ast.File, 0, len(entry.root.CompiledGoFiles))
	usedImports := make(map[string]*packages.Package)
	for _, path := range entry.root.CompiledGoFiles {
		if err := ctx.Err(); err != nil {
			return PackageLoadResult{}, false, err
		}
		input, found := bytesByPath[path]
		if !found {
			return PackageLoadResult{}, false, nil
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
			parsed.Name.Name != entry.root.Name {
			return PackageLoadResult{}, false, nil
		}
		for _, specification := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(specification.Path.Value)
			if unquoteErr != nil || importPath == "C" {
				return PackageLoadResult{}, false, nil
			}
			imported := entry.availableImports[importPath]
			if imported == nil || imported.Types == nil {
				return PackageLoadResult{}, false, nil
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
		Importer: packageSessionImporter{packages: entry.availableImports},
		Sizes: entry.root.TypesSizes,
		GoVersion: packageSessionGoVersion(sourceGoVersion),
	}
	checked, typeErr := configuration.Check(entry.root.PkgPath, fileSet, syntax, information)
	if err := ctx.Err(); err != nil {
		return PackageLoadResult{}, false, err
	}
	if typeErr != nil || checked == nil {
		return PackageLoadResult{}, false, nil
	}
	root := entry.root
	root.Types = checked
	root.Fset = fileSet
	root.Syntax = syntax
	root.TypesInfo = information
	root.Imports = usedImports
	root.Errors = nil
	root.TypeErrors = nil
	root.IllTyped = false
	return PackageLoadResult{
		Requirement: options.Requirement,
		Packages: []*packages.Package{&root},
		Diagnostics: []PackageDiagnostic{},
		Sources: PackageSourceSet{paths: paths, files: files},
		effectFacts: cloneNativeEffectFacts(entry.effectFacts),
	}, true, nil
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
}

func (i packageSessionImporter) Import(path string) (*types.Package, error) {
	return i.ImportFrom(path, "", 0)
}

func (i packageSessionImporter) ImportFrom(
	path string,
	directory string,
	mode types.ImportMode,
) (*types.Package, error) {
	if imported := i.packages[path]; imported != nil && imported.Types != nil {
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
) (map[string]packageSessionFileIdentity, map[string][]string, error) {
	inputs := make(map[string]packageSessionFileIdentity)
	for path := range files {
		contents, err := source.ReadFile(path)
		if err != nil {
			return nil, nil, err
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
