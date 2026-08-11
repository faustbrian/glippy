package analysis

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"sort"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/gox/internal/rules"
)

type packageFactKey struct {
	analyzer *goanalysis.Analyzer
	package_ *types.Package
	type_    reflect.Type
}

type objectFactViewKey struct {
	analyzer *goanalysis.Analyzer
	package_ *types.Package
}

type objectFactKey struct {
	object types.Object
	type_  reflect.Type
}

type objectFactView struct {
	values map[objectFactKey][]byte
}

type analyzerFactSet struct {
	packageValues map[packageFactKey][]byte
	objectViews   map[objectFactViewKey]*objectFactView
	packages      map[*types.Package]*packages.Package
}

func newAnalyzerFactSet() *analyzerFactSet {
	return &analyzerFactSet{
		packageValues: make(map[packageFactKey][]byte),
		objectViews:   make(map[objectFactViewKey]*objectFactView),
		packages:      make(map[*types.Package]*packages.Package),
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
	roots []*packages.Package,
	sources PackageSourceSet,
	owners map[string]string,
	metadata rules.Metadata,
	severity rules.Severity,
) ([]rules.Diagnostic, error) {
	facts := newAnalyzerFactSet()
	state := make(map[*packages.Package]uint8)
	results := make(map[*packages.Package][]rules.Diagnostic)
	var execute func(*packages.Package) error
	execute = func(pkg *packages.Package) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch state[pkg] {
		case 1:
			return fmt.Errorf("package fact graph contains an import cycle at %q", pkg.ID)
		case 2:
			return nil
		}
		state[pkg] = 1
		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)
		for _, path := range imports {
			if err := execute(pkg.Imports[path]); err != nil {
				return err
			}
		}
		if pkg.IllTyped {
			for _, step := range r.steps {
				if !step.original.RunDespiteErrors {
					return fmt.Errorf(
						"analyzer %q cannot produce facts for ill-typed package %q",
						step.original.Name,
						pkg.ID,
					)
				}
			}
		}
		files, err := packageSyntaxFiles(pkg, sources)
		if err != nil {
			return err
		}
		produced, err := r.runPackage(ctx, pkg, files, owners, severity, facts)
		if err != nil {
			return err
		}
		results[pkg] = produced
		state[pkg] = 2
		return nil
	}
	diagnostics := make([]rules.Diagnostic, 0)
	for _, root := range roots {
		files, err := packageSyntaxFiles(root, sources)
		if err != nil {
			return nil, err
		}
		if root.IllTyped && !metadata.RunDespiteTypeErrors {
			continue
		}
		if !packageAnalyzerOwnsEligibleFile(root.ID, files, owners, metadata) {
			continue
		}
		if err := execute(root); err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, results[root]...)
	}
	return diagnostics, nil
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
	encoded, found := s.packageValues[packageFactKey{analyzer: analyzer, package_: package_, type_: type_}]
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
	visible := make(map[*types.Package]struct{}, len(current.Imports())+1)
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
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].package_.Path() != keys[right].package_.Path() {
			return keys[left].package_.Path() < keys[right].package_.Path()
		}
		return lessFactType(keys[left].type_, keys[right].type_)
	})
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
	view := &objectFactView{values: make(map[objectFactKey][]byte)}
	imports := append([]*types.Package(nil), pkg.Types.Imports()...)
	sort.Slice(imports, func(left, right int) bool {
		return imports[left].Path() < imports[right].Path()
	})
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
			if previous, duplicate := view.values[factKey]; duplicate && !bytes.Equal(previous, encoded) {
				return fmt.Errorf(
					"analyzer %q inherited conflicting object fact %T for %s",
					analyzer.Name,
					reflect.New(factKey.type_.Elem()).Interface(),
					types.ObjectString(factKey.object, packagePath),
				)
			}
			view.values[factKey] = encoded
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
		panic(fmt.Sprintf(
			"analyzer %q cannot export object fact for %s outside package %q",
			analyzer.Name,
			types.ObjectString(object, packagePath),
			current.Path(),
		))
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
	view.values[objectFactKey{object: object, type_: type_}] = encoded
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
	sort.Slice(keys, func(left, right int) bool {
		return s.lessObjectFact(view, keys[left], keys[right])
	})
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
