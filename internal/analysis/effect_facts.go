package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/faustbrian/glippy/internal/cache"
	"golang.org/x/tools/go/packages"
)

const nativeEffectFactSchemaVersion = 1

// nativeEffectFacts contains conservative, versioned semantic summaries whose
// stable identities survive independent package loads. The first schema
// exports only proven no-return functions; later effect kinds must advance the
// schema whenever their encoded meaning changes.
type nativeEffectFacts struct {
	noReturns map[string]struct{}
}

func newNativeEffectFacts() *nativeEffectFacts {
	return &nativeEffectFacts{noReturns: make(map[string]struct{})}
}

func (f *nativeEffectFacts) noReturn(function *types.Func) bool {
	if f == nil {
		return false
	}
	_, found := f.noReturns[stableFunctionIdentity(function)]
	return found
}

func (f *nativeEffectFacts) addNoReturns(analysis *noReturnAnalysis) {
	if f == nil || analysis == nil {
		return
	}
	for function, definition := range analysis.definitions {
		if definition != nil && definition.noReturn {
			if identity := stableFunctionIdentity(function); identity != "" {
				f.noReturns[identity] = struct{}{}
			}
		}
	}
}

func (f *nativeEffectFacts) digest() cache.Digest {
	digest := sha256.New()
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], nativeEffectFactSchemaVersion)
	_, _ = digest.Write(version[:])
	identities := make([]string, 0)
	if f != nil {
		identities = make([]string, 0, len(f.noReturns))
		for identity := range f.noReturns {
			identities = append(identities, identity)
		}
	}
	sort.Strings(identities)
	for _, identity := range identities {
		binary.BigEndian.PutUint64(version[:], uint64(len(identity)))
		_, _ = digest.Write(version[:])
		_, _ = digest.Write([]byte(identity))
	}
	var result cache.Digest
	copy(result[:], digest.Sum(nil))
	return result
}

func stableFunctionIdentity(function *types.Func) string {
	if function == nil || function.Pkg() == nil {
		return ""
	}
	return types.ObjectString(
		function,
		func(package_ *types.Package) string {
			if package_ == nil {
				return ""
			}
			return package_.Path()
		},
	)
}

func loadNativeEffectFacts(
	ctx context.Context,
	options PackageLoadOptions,
	roots []*packages.Package,
	rootSources PackageSourceSet,
) (*nativeEffectFacts, error) {
	facts := newNativeEffectFacts()
	prefixes := effectModulePrefixes(roots)
	if len(prefixes) == 0 {
		return facts, nil
	}
	seen := make(map[string]struct{})
	for _, root := range roots {
		if root != nil && root.PkgPath != "" {
			seen[root.PkgPath] = struct{}{}
		}
	}
	current := localEffectImports(roots, prefixes, seen)
	layers := make([][]*packages.Package, 0)
	maximumPackages := options.MaxPackages
	if maximumPackages == 0 {
		maximumPackages = DefaultMaxPackages
	}
	maximumSourceFiles := options.MaxSourceFiles
	if maximumSourceFiles == 0 {
		maximumSourceFiles = DefaultMaxSourceFiles
	}
	maximumSourceBytes := options.MaxSourceBytes
	if maximumSourceBytes == 0 {
		maximumSourceBytes = DefaultMaxSourceBytes
	}
	loadedCount := len(roots)
	loadedSources := make(map[string]struct{})
	var loadedBytes int64
	for _, path := range rootSources.Paths() {
		file, found := rootSources.Lookup(path)
		if !found {
			return nil, fmt.Errorf("native effect root source %q is missing", path)
		}
		loadedSources[path] = struct{}{}
		loadedBytes += int64(len(file.Bytes()))
	}
	for len(current) != 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		layerOptions := clonePackageLoadOptions(options)
		layerOptions.Patterns = current
		layerOptions.Tests = false
		layerOptions.LoadDependencySyntax = false
		layerOptions.LoadEffectFacts = false
		loaded, err := LoadPackages(ctx, layerOptions)
		if err != nil {
			return nil, fmt.Errorf("load native effect inputs: %w", err)
		}
		loadedCount += len(loaded.Packages)
		if loadedCount > maximumPackages {
			return nil, fmt.Errorf(
				"native effect graph exceeds %d-package limit",
				maximumPackages,
			)
		}
		for _, path := range loaded.Sources.Paths() {
			if _, found := loadedSources[path]; found {
				continue
			}
			file, found := loaded.Sources.Lookup(path)
			if !found {
				return nil, fmt.Errorf("native effect source %q is missing", path)
			}
			loadedSources[path] = struct{}{}
			loadedBytes += int64(len(file.Bytes()))
			if len(loadedSources) > maximumSourceFiles {
				return nil, fmt.Errorf(
					"native effect source set exceeds %d-file limit",
					maximumSourceFiles,
				)
			}
			if loadedBytes > maximumSourceBytes {
				return nil, fmt.Errorf(
					"native effect source set exceeds %d-byte limit",
					maximumSourceBytes,
				)
			}
		}
		layers = append(layers, loaded.Packages)
		current = localEffectImports(loaded.Packages, prefixes, seen)
	}
	for index := len(layers) - 1; index >= 0; index-- {
		analysis := newNoReturnAnalysis(ctx, layers[index], facts)
		analysis.buildAll()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		facts.addNoReturns(analysis)
	}
	return facts, nil
}

func effectModulePrefixes(roots []*packages.Package) []string {
	prefixes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, root := range roots {
		if root == nil || root.Module == nil || root.Module.Path == "" {
			continue
		}
		if _, found := seen[root.Module.Path]; found {
			continue
		}
		seen[root.Module.Path] = struct{}{}
		prefixes = append(prefixes, root.Module.Path)
	}
	sort.Strings(prefixes)
	return prefixes
}

func localEffectImports(
	packages_ []*packages.Package,
	prefixes []string,
	seen map[string]struct{},
) []string {
	imports := make(map[string]struct{})
	for _, pkg := range packages_ {
		if pkg == nil || pkg.Types == nil {
			continue
		}
		for _, imported := range pkg.Types.Imports() {
			path := imported.Path()
			if !effectPathWithinModules(path, prefixes) {
				continue
			}
			if _, found := seen[path]; found {
				continue
			}
			seen[path] = struct{}{}
			imports[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(imports))
	for path := range imports {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func effectPathWithinModules(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix + "/") {
			return true
		}
	}
	return false
}
