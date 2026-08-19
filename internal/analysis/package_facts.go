package analysis

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"runtime/debug"
	"sort"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/glippy/internal/cache"
	"github.com/faustbrian/glippy/internal/rules"
)

type packageFactKey struct {
	analyzer *goanalysis.Analyzer
	package_ *types.Package
	type_ reflect.Type
}

type objectFactViewKey struct {
	analyzer *goanalysis.Analyzer
	package_ *types.Package
}

type objectFactKey struct {
	object types.Object
	type_ reflect.Type
}

type objectFactView struct {
	values map[objectFactKey][]byte
	order map[objectFactKey]int
}

type analyzerFactSet struct {
	packageValues map[packageFactKey][]byte
	objectViews map[objectFactViewKey]*objectFactView
	packages map[*types.Package]*packages.Package
}

func newAnalyzerFactSet() *analyzerFactSet {
	return &analyzerFactSet{
		packageValues: make(map[packageFactKey][]byte),
		objectViews: make(map[objectFactViewKey]*objectFactView),
		packages: make(map[*types.Package]*packages.Package),
	}
}

func (r *packageAnalyzerRule) usesFacts() bool {
	for _, step := range r.steps {
		if len(step.original.FactTypes) != 0 {
			return true
		}
	}
	return false
}

func (r *packageAnalyzerRule) runPackageFactGraph(
	ctx context.Context,
	plan packageFactPlan,
	retainedSources PackageSourceSet,
	loadOptions PackageLoadOptions,
	cachePlan *packageCachePlan,
	metadata rules.Metadata,
	severity rules.Severity,
) ([]rules.Diagnostic, error) {
	store := newRetainedPackageFactStore()
	dependencies, err := r.prepareDependencyFactCandidates(
		ctx,
		plan.dependencies,
		retainedSources,
		loadOptions,
		cachePlan,
		store,
	)
	if err != nil {
		return nil, err
	}
	for _, dependencyWave := range packageFactWaves(dependencies) {
		if err := r.runPackageFactWave(
			ctx,
			dependencyWave,
			retainedSources,
			loadOptions,
			cachePlan,
			severity,
			store,
		);
			err != nil {
			return nil, err
		}
		debug.FreeOSMemory()
	}
	loaded, rootOptions, err := loadPackageFactRoots(ctx, loadOptions, retainedSources)
	if err != nil {
		return nil, err
	}
	rootsByID := make(map[string]*packages.Package, len(loaded.Packages))
	for _, root := range loaded.Packages {
		if root != nil {
			rootsByID[root.ID] = root
		}
	}
	diagnostics := make([]rules.Diagnostic, 0)
	for _, id := range plan.roots {
		root := rootsByID[id]
		if root == nil {
			return nil, fmt.Errorf("package fact schedule omitted root %q", id)
		}
		files, err := packageSyntaxFiles(root, loaded.Sources)
		if err != nil {
			return nil, err
		}
		if root.IllTyped && !metadata.RunDespiteTypeErrors {
			continue
		}
		if !packageAnalyzerOwnsEligibleFile(root.ID, files, plan.owners, metadata) {
			continue
		}
		produced, err := r.runRetainedFactPackage(
			ctx,
			root,
			loaded,
			rootOptions,
			cachePlan,
			plan.owners,
			severity,
			store,
		)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, produced...)
	}
	return diagnostics, nil
}

func (r *packageAnalyzerRule) packageCacheKey(
	pkg *packages.Package,
	plan *packageCachePlan,
	loadOptions PackageLoadOptions,
	base cache.Key,
	baseCacheable bool,
	dependencyKeys map[string]cache.Key,
) (cache.Key, bool) {
	if plan == nil || !baseCacheable {
		return cache.Key{}, false
	}
	components := make([]cache.Component, 0, len(pkg.Imports) + 1)
	components = append(
		components,
		cache.Component{
			Kind: cache.ComponentBuildSelection,
			Identity: "analysis-input-manifest",
			Digest: cache.Digest(base),
		},
	)
	imports := make([]string, 0, len(pkg.Types.Imports()))
	for _, imported := range pkg.Types.Imports() {
		imports = append(imports, imported.Path())
	}
	sort.Strings(imports)
	for _, path := range imports {
		key, found := dependencyKeys[path]
		if !found {
			return cache.Key{}, false
		}
		components = append(
			components,
			cache.Component{
				Kind: cache.ComponentFact,
				Identity: path,
				Digest: cache.Digest(key),
			},
		)
	}
	key, err := cache.BuildKey(
		cache.KeyInput{
			Namespace: "typed-analyzer-v2:" + r.metadata.ID + ":" + pkg.ID,
			ToolVersion: plan.options.ToolVersion,
			BuildGoVersion: plan.options.BuildGoVersion,
			SourceGoVersion: plan.options.SourceGoVersion,
			Configuration: plan.options.Configuration,
			Rules: plan.rules,
			BuildTags: loadOptions.BuildTags,
			GOOS: loadOptions.GOOS,
			GOARCH: loadOptions.GOARCH,
			CGOEnabled: plan.options.CGOEnabled,
			FormatterMode: plan.options.FormatterMode,
			Components: components,
		},
	)
	if err != nil {
		return cache.Key{}, false
	}
	return key, true
}

func (r *packageAnalyzerRule) packageCacheBaseKey(
	loaded PackageLoadResult,
	loadOptions PackageLoadOptions,
	plan *packageCachePlan,
) (cache.Key, bool) {
	if plan == nil {
		return cache.Key{}, false
	}
	key, err := buildPackageCacheKey(
		packageCacheKeyInput{
			Namespace: "typed-analyzer-input-v2:" +
				r.metadata.ID +
				":" +
				r.dependencyFactFilterIdentity(),
			ToolVersion: plan.options.ToolVersion,
			BuildGoVersion: plan.options.BuildGoVersion,
			SourceGoVersion: plan.options.SourceGoVersion,
			Configuration: plan.options.Configuration,
			Rules: plan.rules,
			CGOEnabled: plan.options.CGOEnabled,
			FormatterMode: plan.options.FormatterMode,
			LoadOptions: loadOptions,
			Loaded: loaded,
			Facts: map[string]cache.Digest{},
		},
	)
	if err != nil {
		return cache.Key{}, false
	}
	return key, true
}

func (r *packageAnalyzerRule) dependencyFactFilterIdentity() string {
	if r == nil || r.dependencyFactFilter == nil {
		return "unfiltered"
	}
	return r.dependencyFactFilter.Identity
}

func (s *analyzerFactSet) importPackageFact(
	analyzer *goanalysis.Analyzer,
	package_ *types.Package,
	fact goanalysis.Fact,
) bool {
	if package_ == nil {
		panic("package fact import requires a package")
	}
	type_, err := declaredFactType(analyzer, fact)
	if err != nil {
		panic(err)
	}
	encoded, found := s.packageValues[packageFactKey{
		analyzer: analyzer,
		package_: package_,
		type_: type_,
	}]
	if !found {
		return false
	}
	if err := decodeFact(encoded, fact); err != nil {
		panic(fmt.Errorf("decode package fact %T: %w", fact, err))
	}
	return true
}

func (s *analyzerFactSet) exportPackageFact(
	analyzer *goanalysis.Analyzer,
	package_ *types.Package,
	fact goanalysis.Fact,
) {
	type_, err := declaredFactType(analyzer, fact)
	if err != nil {
		panic(err)
	}
	encoded, err := encodeFact(fact)
	if err != nil {
		panic(err)
	}
	s.packageValues[packageFactKey{analyzer: analyzer, package_: package_, type_: type_}] = encoded
}

func (s *analyzerFactSet) allPackageFacts(
	analyzer *goanalysis.Analyzer,
	current *types.Package,
) []goanalysis.PackageFact {
	visible := make(map[*types.Package]struct{}, len(current.Imports()) + 1)
	visible[current] = struct{}{}
	for _, imported := range current.Imports() {
		visible[imported] = struct{}{}
	}
	keys := make([]packageFactKey, 0)
	for key := range s.packageValues {
		if _, found := visible[key.package_]; key.analyzer == analyzer && found {
			keys = append(keys, key)
		}
	}
	sort.Slice(
		keys,
		func(left, right int) bool {
			if keys[left].package_.Path() != keys[right].package_.Path() {
				return keys[left].package_.Path() < keys[right].package_.Path()
			}
			return lessFactType(keys[left].type_, keys[right].type_)
		},
	)
	result := make([]goanalysis.PackageFact, len(keys))
	for index, key := range keys {
		fact := reflect.New(key.type_.Elem()).Interface().(goanalysis.Fact)
		if !s.importPackageFact(analyzer, key.package_, fact) {
			panic("package fact disappeared during enumeration")
		}
		result[index] = goanalysis.PackageFact{Package: key.package_, Fact: fact}
	}
	return result
}

func (s *analyzerFactSet) beginObjectFacts(
	analyzer *goanalysis.Analyzer,
	pkg *packages.Package,
) error {
	if analyzer == nil || pkg == nil || pkg.Types == nil {
		return fmt.Errorf("object facts require an analyzer and typed package")
	}
	key := objectFactViewKey{analyzer: analyzer, package_: pkg.Types}
	if _, found := s.objectViews[key]; found {
		return nil
	}
	s.packages[pkg.Types] = pkg
	view := &objectFactView{
		values: make(map[objectFactKey][]byte),
		order: make(map[objectFactKey]int),
	}
	imports := append([]*types.Package(nil), pkg.Types.Imports()...)
	sort.Slice(
		imports,
		func(left, right int) bool {
			return imports[left].Path() < imports[right].Path()
		},
	)
	for _, imported := range imports {
		dependency, found := s.objectViews[objectFactViewKey{
			analyzer: analyzer,
			package_: imported,
		}]
		if !found {
			return fmt.Errorf(
				"analyzer %q object facts for dependency %q did not run",
				analyzer.Name,
				imported.Path(),
			)
		}
		for factKey, encoded := range dependency.values {
			if !objectFactExportedFrom(factKey.object, imported) {
				continue
			}
			if previous, duplicate := view.values[factKey];
				duplicate && !bytes.Equal(previous, encoded) {
				return fmt.Errorf(
					"analyzer %q inherited conflicting object fact %T for %s",
					analyzer.Name,
					reflect.New(factKey.type_.Elem()).Interface(),
					types.ObjectString(factKey.object, packagePath),
				)
			}
			view.values[factKey] = encoded
			if ordinal, ordered := dependency.order[factKey]; ordered {
				if previous, duplicate := view.order[factKey];
					duplicate && previous != ordinal {
					return fmt.Errorf(
						"analyzer %q inherited conflicting object fact order for %s",
						analyzer.Name,
						types.ObjectString(factKey.object, packagePath),
					)
				}
				view.order[factKey] = ordinal
			}
		}
	}
	s.objectViews[key] = view
	return nil
}

func (s *analyzerFactSet) importObjectFact(
	analyzer *goanalysis.Analyzer,
	current *types.Package,
	object types.Object,
	fact goanalysis.Fact,
) bool {
	if object == nil {
		panic("object fact import requires an object")
	}
	type_, err := declaredFactType(analyzer, fact)
	if err != nil {
		panic(err)
	}
	view, found := s.objectViews[objectFactViewKey{analyzer: analyzer, package_: current}]
	if !found {
		panic("object fact view was not initialized")
	}
	encoded, found := view.values[objectFactKey{object: object, type_: type_}]
	if !found {
		return false
	}
	if err := decodeFact(encoded, fact); err != nil {
		panic(fmt.Errorf("decode object fact %T: %w", fact, err))
	}
	return true
}

func (s *analyzerFactSet) exportObjectFact(
	analyzer *goanalysis.Analyzer,
	current *types.Package,
	object types.Object,
	fact goanalysis.Fact,
) {
	if object == nil {
		panic("object fact export requires an object")
	}
	if object.Pkg() != current {
		panic(
			fmt.Sprintf(
				"analyzer %q cannot export object fact for %s outside package %q",
				analyzer.Name,
				types.ObjectString(object, packagePath),
				current.Path(),
			),
		)
	}
	type_, err := declaredFactType(analyzer, fact)
	if err != nil {
		panic(err)
	}
	encoded, err := encodeFact(fact)
	if err != nil {
		panic(err)
	}
	view, found := s.objectViews[objectFactViewKey{analyzer: analyzer, package_: current}]
	if !found {
		panic("object fact view was not initialized")
	}
	key := objectFactKey{object: object, type_: type_}
	view.values[key] = encoded
	delete(view.order, key)
}

func (s *analyzerFactSet) allObjectFacts(
	analyzer *goanalysis.Analyzer,
	current *types.Package,
) []goanalysis.ObjectFact {
	view, found := s.objectViews[objectFactViewKey{analyzer: analyzer, package_: current}]
	if !found {
		panic("object fact view was not initialized")
	}
	keys := make([]objectFactKey, 0, len(view.values))
	for key := range view.values {
		keys = append(keys, key)
	}
	sort.Slice(
		keys,
		func(left, right int) bool {
			return s.lessObjectFact(view, keys[left], keys[right])
		},
	)
	result := make([]goanalysis.ObjectFact, len(keys))
	for index, key := range keys {
		fact := reflect.New(key.type_.Elem()).Interface().(goanalysis.Fact)
		if err := decodeFact(view.values[key], fact); err != nil {
			panic(fmt.Errorf("decode object fact %T: %w", fact, err))
		}
		result[index] = goanalysis.ObjectFact{Object: key.object, Fact: fact}
	}
	return result
}

func (s *analyzerFactSet) lessObjectFact(
	view *objectFactView,
	left objectFactKey,
	right objectFactKey,
) bool {
	leftPackage, rightPackage := "", ""
	if left.object.Pkg() != nil {
		leftPackage = left.object.Pkg().Path()
	}
	if right.object.Pkg() != nil {
		rightPackage = right.object.Pkg().Path()
	}
	if leftPackage != rightPackage {
		return leftPackage < rightPackage
	}
	leftOrder, leftOrdered := view.order[left]
	rightOrder, rightOrdered := view.order[right]
	if leftOrdered && rightOrdered && leftOrder != rightOrder {
		return leftOrder < rightOrder
	}
	leftPosition := s.objectFactPosition(left.object)
	rightPosition := s.objectFactPosition(right.object)
	if leftPosition.Filename != rightPosition.Filename {
		return leftPosition.Filename < rightPosition.Filename
	}
	if leftPosition.Offset != rightPosition.Offset {
		return leftPosition.Offset < rightPosition.Offset
	}
	leftObject := types.ObjectString(left.object, packagePath)
	rightObject := types.ObjectString(right.object, packagePath)
	if leftObject != rightObject {
		return leftObject < rightObject
	}
	if left.type_ != right.type_ {
		return lessFactType(left.type_, right.type_)
	}
	return bytes.Compare(view.values[left], view.values[right]) < 0
}

func (s *analyzerFactSet) objectFactPosition(object types.Object) token.Position {
	if object.Pkg() == nil {
		return token.Position{}
	}
	pkg := s.packages[object.Pkg()]
	if pkg == nil || pkg.Fset == nil {
		return token.Position{}
	}
	return pkg.Fset.PositionFor(object.Pos(), false)
}

func objectFactExportedFrom(object types.Object, pkg *types.Package) bool {
	switch object := object.(type) {
	case *types.Func:
		return object.Exported() && object.Pkg() == pkg || object.Signature().Recv() != nil
	case *types.Var:
		return object.IsField() || object.Pkg() == pkg
	case *types.TypeName, *types.Const:
		return true
	default:
		return false
	}
}

func packagePath(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Path()
}

func declaredFactType(analyzer *goanalysis.Analyzer, fact goanalysis.Fact) (reflect.Type, error) {
	if analyzer == nil || fact == nil {
		return nil, fmt.Errorf("analysis fact requires an analyzer and non-nil fact")
	}
	type_ := reflect.TypeOf(fact)
	value := reflect.ValueOf(fact)
	if type_.Kind() != reflect.Pointer || value.IsNil() {
		return nil, fmt.Errorf("analysis fact %T must be a non-nil pointer", fact)
	}
	for _, declared := range analyzer.FactTypes {
		if reflect.TypeOf(declared) == type_ {
			return type_, nil
		}
	}
	return nil, fmt.Errorf("analyzer %q did not declare fact type %T", analyzer.Name, fact)
}

func lessFactType(left, right reflect.Type) bool {
	if left.PkgPath() != right.PkgPath() {
		return left.PkgPath() < right.PkgPath()
	}
	if left.Elem().PkgPath() != right.Elem().PkgPath() {
		return left.Elem().PkgPath() < right.Elem().PkgPath()
	}
	return left.String() < right.String()
}

func encodeFact(fact goanalysis.Fact) ([]byte, error) {
	encode := func() ([]byte, error) {
		var output bytes.Buffer
		if err := gob.NewEncoder(&output).Encode(fact); err != nil {
			return nil, fmt.Errorf("encode analysis fact %T: %w", fact, err)
		}
		return output.Bytes(), nil
	}
	first, err := encode()
	if err != nil {
		return nil, err
	}
	second, err := encode()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, fmt.Errorf("analysis fact %T encoding is nondeterministic", fact)
	}
	return first, nil
}

func decodeFact(encoded []byte, fact goanalysis.Fact) error {
	value := reflect.ValueOf(fact)
	value.Elem().Set(reflect.Zero(value.Elem().Type()))
	return gob.NewDecoder(bytes.NewReader(encoded)).Decode(fact)
}
