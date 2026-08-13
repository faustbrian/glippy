package analysis

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/gox/internal/cache"
)

type packageCacheKeyInput struct {
	Namespace string
	ToolVersion string
	BuildGoVersion string
	SourceGoVersion string
	Configuration cache.Digest
	Rules []cache.RuleInput
	CGOEnabled bool
	FormatterMode string
	LoadOptions PackageLoadOptions
	Loaded PackageLoadResult
	Facts map[string]cache.Digest
}

type packageCacheComponentKey struct {
	kind cache.ComponentKind
	identity string
}

func buildPackageCacheKey(input packageCacheKeyInput) (cache.Key, error) {
	if err := validatePackageCacheLoadIdentity(input.LoadOptions, input.CGOEnabled);
		err != nil {
		return cache.Key{}, err
	}
	if input.Loaded.Requirement != input.LoadOptions.Requirement {
		return cache.Key{}, fmt.Errorf(
			"package cache load requirement does not match its request",
		)
	}
	if len(input.Loaded.Diagnostics) != 0 || len(input.Loaded.Sources.problems) != 0 {
		return cache.Key{}, fmt.Errorf("package cache identity requires an error-free load")
	}
	if input.Facts == nil {
		return cache.Key{}, fmt.Errorf(
			"package cache identity requires explicit imported fact digests",
		)
	}
	if _, err := packageBuildFlags(input.LoadOptions); err != nil {
		return cache.Key{}, err
	}
	packages_, err := reachablePackageGraph(input.Loaded.Packages)
	if err != nil {
		return cache.Key{}, err
	}
	for _, pkg := range packages_ {
		if pkg.IllTyped {
			return cache.Key{}, fmt.Errorf(
				"package cache identity refuses ill-typed package %q",
				pkg.ID,
			)
		}
	}
	components := make(map[packageCacheComponentKey]cache.Component)
	add := func(component cache.Component) error {
		key := packageCacheComponentKey{kind: component.Kind, identity: component.Identity}
		if previous, found := components[key]; found {
			if previous.Digest != component.Digest {
				return fmt.Errorf(
					"package cache component %s %q has conflicting digests",
					component.Kind,
					component.Identity,
				)
			}
			return nil
		}
		components[key] = component
		return nil
	}
	if err := addPackageSourceComponents(add, input.Loaded.Sources); err != nil {
		return cache.Key{}, err
	}
	if err := addPackageSelectionComponents(add, input.LoadOptions, packages_); err != nil {
		return cache.Key{}, err
	}
	if err := addPackageEnvironmentComponent(add, input.LoadOptions); err != nil {
		return cache.Key{}, err
	}
	if err := addPackageOverlayComponents(add, input.LoadOptions.Overlay); err != nil {
		return cache.Key{}, err
	}
	if err := addPackageModuleComponents(add, input.LoadOptions, packages_); err != nil {
		return cache.Key{}, err
	}
	if err := addPackageExportComponents(
		add,
		input.Loaded.Sources,
		input.Loaded.Packages,
		packages_,
		input.LoadOptions.LoadDependencySyntax,
	);
		err != nil {
		return cache.Key{}, err
	}
	factNames := make([]string, 0, len(input.Facts))
	for name := range input.Facts {
		factNames = append(factNames, name)
	}
	sort.Strings(factNames)
	for _, name := range factNames {
		if strings.TrimSpace(name) == "" || input.Facts[name] == (cache.Digest{}) {
			return cache.Key{}, fmt.Errorf(
				"package cache imported fact identity is incomplete",
			)
		}
		if err := add(
			cache.Component{
				Kind: cache.ComponentFact,
				Identity: name,
				Digest: input.Facts[name],
			},
		);
			err != nil {
			return cache.Key{}, err
		}
	}
	ordered := make([]cache.Component, 0, len(components))
	for _, component := range components {
		ordered = append(ordered, component)
	}
	return cache.BuildKey(
		cache.KeyInput{
			Namespace: input.Namespace,
			ToolVersion: input.ToolVersion,
			BuildGoVersion: input.BuildGoVersion,
			SourceGoVersion: input.SourceGoVersion,
			Configuration: input.Configuration,
			Rules: slices.Clone(input.Rules),
			BuildTags: slices.Clone(input.LoadOptions.BuildTags),
			GOOS: input.LoadOptions.GOOS,
			GOARCH: input.LoadOptions.GOARCH,
			CGOEnabled: input.CGOEnabled,
			FormatterMode: input.FormatterMode,
			Components: ordered,
		},
	)
}

func validatePackageCacheLoadIdentity(options PackageLoadOptions, cgoEnabled bool) error {
	if options.Env == nil {
		return fmt.Errorf("package cache identity requires an explicit environment")
	}
	if options.GOOS == "" || options.GOARCH == "" {
		return fmt.Errorf("package cache identity requires explicit GOOS and GOARCH")
	}
	environment := packageLoadEnvironment(options)
	if environmentValue(environment, "GOENV") != "off" {
		return fmt.Errorf("package cache identity requires GOENV=off")
	}
	wantCGO := "0"
	if cgoEnabled {
		wantCGO = "1"
	}
	if environmentValue(environment, "CGO_ENABLED") != wantCGO {
		return fmt.Errorf("package cache identity requires CGO_ENABLED=%s", wantCGO)
	}
	return nil
}

func reachablePackageGraph(roots []*packages.Package) ([]*packages.Package, error) {
	byID := make(map[string]*packages.Package)
	state := make(map[*packages.Package]uint8)
	var visit func(*packages.Package) error
	visit = func(pkg *packages.Package) error {
		if pkg == nil || pkg.ID == "" {
			return fmt.Errorf(
				"package cache graph contains a nil or unidentified package",
			)
		}
		if state[pkg] == 1 {
			return fmt.Errorf(
				"package cache graph contains an import cycle at %q",
				pkg.ID,
			)
		}
		if state[pkg] == 2 {
			return nil
		}
		if previous, found := byID[pkg.ID]; found && previous != pkg {
			return fmt.Errorf(
				"package cache graph has conflicting package ID %q",
				pkg.ID,
			)
		}
		byID[pkg.ID] = pkg
		state[pkg] = 1
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)
		for _, path := range imports {
			if err := visit(pkg.Imports[path]); err != nil {
				return err
			}
		}
		state[pkg] = 2
		return nil
	}
	for _, root := range roots {
		if err := visit(root); err != nil {
			return nil, err
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*packages.Package, len(ids))
	for index, id := range ids {
		result[index] = byID[id]
	}
	return result, nil
}

func addPackageSourceComponents(add func(cache.Component) error, sources PackageSourceSet) error {
	if len(sources.paths) == 0 || len(sources.paths) != len(sources.files) {
		return fmt.Errorf("package cache source set is incomplete")
	}
	previous := ""
	for _, path := range sources.paths {
		if path <= previous || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("package cache source paths are not canonical")
		}
		previous = path
		file, found := sources.Lookup(path)
		if !found {
			return fmt.Errorf("package cache source %q is missing", path)
		}
		if file.Path() != path {
			return fmt.Errorf(
				"package cache source %q has identity %q",
				path,
				file.Path(),
			)
		}
		if err := add(
			cache.Component{
				Kind: cache.ComponentSource,
				Identity: path,
				Digest: cache.Digest(file.Digest()),
			},
		);
			err != nil {
			return err
		}
	}
	return nil
}

func addPackageSelectionComponents(
	add func(cache.Component) error,
	options PackageLoadOptions,
	packages_ []*packages.Package,
) error {
	patterns := slices.Clone(options.Patterns)
	sort.Strings(patterns)
	patterns = slices.Compact(patterns)
	tags := slices.Clone(options.BuildTags)
	sort.Strings(tags)
	tags = slices.Compact(tags)
	moduleMode := options.ModuleMode
	if moduleMode == "" {
		moduleMode = ModuleReadonly
	}
	request := struct {
		Dir string
		Patterns []string
		Requirement string
		Tests bool
		LoadDependencySyntax bool
		BuildTags []string
		ModuleMode ModuleMode
		AllowNetwork bool
	}{
		Dir: options.Dir,
		Patterns: patterns,
		Requirement: options.Requirement.String(),
		Tests: options.Tests,
		LoadDependencySyntax: options.LoadDependencySyntax,
		BuildTags: tags,
		ModuleMode: moduleMode,
		AllowNetwork: options.AllowNetwork,
	}
	digest, err := digestPackageCacheJSON(request)
	if err != nil {
		return err
	}
	if err := add(
		cache.Component{
			Kind: cache.ComponentBuildSelection,
			Identity: "request",
			Digest: digest,
		},
	);
		err != nil {
		return err
	}
	for _, pkg := range packages_ {
		imports := make([]string, 0, len(pkg.Imports))
		for path, imported := range pkg.Imports {
			if imported == nil {
				return fmt.Errorf(
					"package cache package %q has nil import %q",
					pkg.ID,
					path,
				)
			}
			imports = append(imports, path + "=" + imported.ID)
		}
		sort.Strings(imports)
		selection := struct {
			ID, Name, PkgPath, ForTest string
			GoFiles, CompiledGoFiles []string
			OtherFiles, EmbedFiles []string
			EmbedPatterns, IgnoredFiles []string
			Imports []string
		}{
			ID: pkg.ID,
			Name: pkg.Name,
			PkgPath: pkg.PkgPath,
			ForTest: pkg.ForTest,
			GoFiles: sortedStrings(pkg.GoFiles),
			CompiledGoFiles: sortedStrings(pkg.CompiledGoFiles),
			OtherFiles: sortedStrings(pkg.OtherFiles),
			EmbedFiles: sortedStrings(pkg.EmbedFiles),
			EmbedPatterns: sortedStrings(pkg.EmbedPatterns),
			IgnoredFiles: sortedStrings(pkg.IgnoredFiles),
			Imports: imports,
		}
		digest, err := digestPackageCacheJSON(selection)
		if err != nil {
			return err
		}
		if err := add(
			cache.Component{
				Kind: cache.ComponentBuildSelection,
				Identity: "package:" + pkg.ID,
				Digest: digest,
			},
		);
			err != nil {
			return err
		}
	}
	return nil
}

func addPackageEnvironmentComponent(
	add func(cache.Component) error,
	options PackageLoadOptions,
) error {
	environment := packageCacheEnvironment(options)
	digest, err := digestPackageCacheJSON(environment)
	if err != nil {
		return err
	}
	return add(
		cache.Component{
			Kind: cache.ComponentEnvironment,
			Identity: "go/packages",
			Digest: digest,
		},
	)
}

func packageCacheEnvironment(options PackageLoadOptions) []string {
	selected := map[string]struct{}{
		"CC": {},
		"CXX": {},
		"FC": {},
		"PATH": {},
		"PKG_CONFIG": {},
		"CGO_ENABLED": {},
		"CGO_CFLAGS": {},
		"CGO_CPPFLAGS": {},
		"CGO_CXXFLAGS": {},
		"CGO_FFLAGS": {},
		"CGO_LDFLAGS": {},
		"GO111MODULE": {},
		"GO386": {},
		"GOAMD64": {},
		"GOARCH": {},
		"GOARM": {},
		"GOARM64": {},
		"GOENV": {},
		"GOEXPERIMENT": {},
		"GOFLAGS": {},
		"GOINSECURE": {},
		"GOMIPS": {},
		"GOMIPS64": {},
		"GONOPROXY": {},
		"GONOSUMDB": {},
		"GOPPC64": {},
		"GOPRIVATE": {},
		"GOPROXY": {},
		"GORISCV64": {},
		"GOSUMDB": {},
		"GOTOOLCHAIN": {},
		"GOVCS": {},
		"GOWASM": {},
		"GOWORK": {},
	}
	environment := packageLoadEnvironment(options)
	result := make([]string, 0, len(selected))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if _, retained := selected[name]; found && retained {
			result = append(result, entry)
		}
	}
	return result
}

func addPackageOverlayComponents(add func(cache.Component) error, overlay map[string][]byte) error {
	paths := make([]string, 0, len(overlay))
	for path := range overlay {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("package cache overlay path %q is not canonical", path)
		}
		if err := add(
			cache.Component{
				Kind: cache.ComponentOverlay,
				Identity: path,
				Digest: cache.DigestOf(overlay[path]),
			},
		);
			err != nil {
			return err
		}
	}
	return nil
}

func addPackageModuleComponents(
	add func(cache.Component) error,
	options PackageLoadOptions,
	packages_ []*packages.Package,
) error {
	seen := make(map[*packages.Module]struct{})
	var addModule func(*packages.Module) error
	addModule = func(module *packages.Module) error {
		if module == nil {
			return nil
		}
		if _, found := seen[module]; found {
			return nil
		}
		seen[module] = struct{}{}
		metadata := struct {
			Path, Version, Time, Dir, GoMod, GoVersion, Error string
			Main, Indirect bool
		}{
			Path: module.Path,
			Version: module.Version,
			Dir: module.Dir,
			GoMod: module.GoMod,
			GoVersion: module.GoVersion,
			Main: module.Main,
			Indirect: module.Indirect,
		}
		if module.Time != nil {
			metadata.Time = module.Time.UTC().Format("2006-01-02T15:04:05.999999999Z")
		}
		if module.Error != nil {
			metadata.Error = module.Error.Err
		}
		digest, err := digestPackageCacheJSON(metadata)
		if err != nil {
			return err
		}
		if err := add(
			cache.Component{
				Kind: cache.ComponentModule,
				Identity: "metadata:" + module.Path + "@" + module.Version,
				Digest: digest,
			},
		);
			err != nil {
			return err
		}
		if module.GoMod != "" {
			if err := addRequiredPackageCacheFile(
				add,
				cache.ComponentModule,
				module.GoMod,
			);
				err != nil {
				return err
			}
			if err := addOptionalPackageCacheFile(
				add,
				cache.ComponentModule,
				filepath.Join(filepath.Dir(module.GoMod), "go.sum"),
			);
				err != nil {
				return err
			}
		}
		return addModule(module.Replace)
	}
	for _, pkg := range packages_ {
		if err := addModule(pkg.Module); err != nil {
			return err
		}
	}
	environment := packageLoadEnvironment(options)
	goWork := environmentValue(environment, "GOWORK")
	workspace := ""
	switch goWork {
	case "off":
	case "", "auto":
		workspace = findPackageCacheWorkspace(options.Dir)
	default:
		workspace = goWork
	}
	selection, err := digestPackageCacheJSON(
		struct {
			Setting, Path string
		}{Setting: goWork, Path: workspace},
	)
	if err != nil {
		return err
	}
	if err := add(
		cache.Component{
			Kind: cache.ComponentWorkspace,
			Identity: "selection",
			Digest: selection,
		},
	);
		err != nil {
		return err
	}
	if workspace != "" {
		if !filepath.IsAbs(workspace) || filepath.Clean(workspace) != workspace {
			return fmt.Errorf(
				"package cache workspace path %q is not canonical",
				workspace,
			)
		}
		if err := addRequiredPackageCacheFile(add, cache.ComponentWorkspace, workspace);
			err != nil {
			return err
		}
		if err := addOptionalPackageCacheFile(
			add,
			cache.ComponentWorkspace,
			filepath.Join(filepath.Dir(workspace), "go.work.sum"),
		);
			err != nil {
			return err
		}
	}
	if options.ModuleMode == ModuleVendor {
		vendorRoot := ""
		if workspace != "" {
			vendorRoot = filepath.Dir(workspace)
		} else {
			for _, pkg := range packages_ {
				if pkg.Module == nil || !pkg.Module.Main || pkg.Module.GoMod == "" {
					continue
				}
				candidate := filepath.Dir(pkg.Module.GoMod)
				if vendorRoot != "" && vendorRoot != candidate {
					return fmt.Errorf(
						"package cache graph has multiple main module roots",
					)
				}
				vendorRoot = candidate
			}
		}
		if vendorRoot == "" {
			return fmt.Errorf(
				"package cache vendor mode has no workspace or main module root",
			)
		}
		vendor := filepath.Join(vendorRoot, "vendor", "modules.txt")
		if err := addRequiredPackageCacheFile(add, cache.ComponentModule, vendor);
			err != nil {
			return err
		}
	}
	return nil
}

func addPackageExportComponents(
	add func(cache.Component) error,
	sources PackageSourceSet,
	roots []*packages.Package,
	packages_ []*packages.Package,
	loadDependencySyntax bool,
) error {
	requireSource := make(map[string]struct{}, len(packages_))
	for _, root := range roots {
		if root == nil {
			return fmt.Errorf("package cache roots contain a nil package")
		}
		requireSource[root.ID] = struct{}{}
	}
	if loadDependencySyntax {
		for _, pkg := range packages_ {
			requireSource[pkg.ID] = struct{}{}
		}
	}
	for _, pkg := range packages_ {
		if pkg.ID == "unsafe" && pkg.PkgPath == "unsafe" && len(pkg.CompiledGoFiles) == 0 {
			if err := add(
				cache.Component{
					Kind: cache.ComponentDependencyExport,
					Identity: "intrinsic:unsafe",
					Digest: cache.DigestOf(
						[]byte("go-toolchain-intrinsic-unsafe-v1"),
					),
				},
			);
				err != nil {
				return err
			}
			continue
		}
		if _, required := requireSource[pkg.ID]; required {
			if len(pkg.CompiledGoFiles) == 0 {
				return fmt.Errorf(
					"package cache package %q has no compiled source",
					pkg.ID,
				)
			}
			for _, path := range pkg.CompiledGoFiles {
				if _, found := sources.Lookup(path); !found {
					return fmt.Errorf(
						"package cache package %q source %q was not captured",
						pkg.ID,
						path,
					)
				}
			}
		}
		if pkg.ExportFile != "" {
			digest, err := digestPackageCacheFile(pkg.ExportFile)
			if err != nil {
				return err
			}
			if err := add(
				cache.Component{
					Kind: cache.ComponentDependencyExport,
					Identity: "package:" + pkg.ID,
					Digest: digest,
				},
			);
				err != nil {
				return err
			}
			continue
		}
		if _, required := requireSource[pkg.ID]; required {
			if len(pkg.CompiledGoFiles) != 0 {
				continue
			}
		}
		if len(pkg.CompiledGoFiles) == 0 {
			return fmt.Errorf(
				"package cache package %q has no source or export evidence",
				pkg.ID,
			)
		}
		for _, path := range pkg.CompiledGoFiles {
			if _, found := sources.Lookup(path); !found {
				return fmt.Errorf(
					"package cache package %q source %q was not captured",
					pkg.ID,
					path,
				)
			}
		}
	}
	return nil
}

func addRequiredPackageCacheFile(
	add func(cache.Component) error,
	kind cache.ComponentKind,
	path string,
) error {
	digest, err := digestPackageCacheFile(path)
	if err != nil {
		return err
	}
	return add(cache.Component{Kind: kind, Identity: path, Digest: digest})
}

func addOptionalPackageCacheFile(
	add func(cache.Component) error,
	kind cache.ComponentKind,
	path string,
) error {
	digest, err := digestPackageCacheFile(path)
	if errors.Is(err, os.ErrNotExist) {
		digest = cache.DigestOf([]byte("gox-cache-file-missing-v1"))
		err = nil
	}
	if err != nil {
		return err
	}
	return add(cache.Component{Kind: kind, Identity: path, Digest: digest})
}

func digestPackageCacheFile(path string) (cache.Digest, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return cache.Digest{}, fmt.Errorf(
			"package cache file path %q is not canonical",
			path,
		)
	}
	file, err := os.Open(path)
	if err != nil {
		return cache.Digest{}, fmt.Errorf("open package cache input %q: %w", path, err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return cache.Digest{}, fmt.Errorf("digest package cache input %q: %w", path, err)
	}
	var result cache.Digest
	copy(result[:], digest.Sum(nil))
	return result, nil
}

func digestPackageCacheJSON(value any) (cache.Digest, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return cache.Digest{}, fmt.Errorf("encode package cache identity: %w", err)
	}
	return cache.DigestOf(encoded), nil
}

func sortedStrings(values []string) []string {
	result := slices.Clone(values)
	sort.Strings(result)
	return result
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func findPackageCacheWorkspace(directory string) string {
	for current := directory; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "go.work")
		if information, err := os.Stat(candidate);
			err == nil && information.Mode().IsRegular() {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}
