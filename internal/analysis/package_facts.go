package analysis

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
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

type packageFactSet struct {
	values map[packageFactKey][]byte
}

func newPackageFactSet() *packageFactSet {
	return &packageFactSet{values: make(map[packageFactKey][]byte)}
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
	facts := newPackageFactSet()
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

func (s *packageFactSet) importPackageFact(
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
	encoded, found := s.values[packageFactKey{analyzer: analyzer, package_: package_, type_: type_}]
	if !found {
		return false
	}
	value := reflect.ValueOf(fact)
	value.Elem().Set(reflect.Zero(value.Elem().Type()))
	if err := gob.NewDecoder(bytes.NewReader(encoded)).Decode(fact); err != nil {
		panic(fmt.Errorf("decode package fact %T: %w", fact, err))
	}
	return true
}

func (s *packageFactSet) exportPackageFact(
	analyzer *goanalysis.Analyzer,
	package_ *types.Package,
	fact goanalysis.Fact,
) {
	type_, err := declaredFactType(analyzer, fact)
	if err != nil {
		panic(err)
	}
	encoded, err := encodePackageFact(fact)
	if err != nil {
		panic(err)
	}
	s.values[packageFactKey{analyzer: analyzer, package_: package_, type_: type_}] = encoded
}

func (s *packageFactSet) allPackageFacts(
	analyzer *goanalysis.Analyzer,
	current *types.Package,
) []goanalysis.PackageFact {
	visible := make(map[*types.Package]struct{}, len(current.Imports())+1)
	visible[current] = struct{}{}
	for _, imported := range current.Imports() {
		visible[imported] = struct{}{}
	}
	keys := make([]packageFactKey, 0)
	for key := range s.values {
		if _, found := visible[key.package_]; key.analyzer == analyzer && found {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].package_.Path() != keys[right].package_.Path() {
			return keys[left].package_.Path() < keys[right].package_.Path()
		}
		if keys[left].type_.PkgPath() != keys[right].type_.PkgPath() {
			return keys[left].type_.PkgPath() < keys[right].type_.PkgPath()
		}
		if keys[left].type_.Elem().PkgPath() != keys[right].type_.Elem().PkgPath() {
			return keys[left].type_.Elem().PkgPath() < keys[right].type_.Elem().PkgPath()
		}
		return keys[left].type_.String() < keys[right].type_.String()
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

func declaredFactType(analyzer *goanalysis.Analyzer, fact goanalysis.Fact) (reflect.Type, error) {
	if analyzer == nil || fact == nil {
		return nil, fmt.Errorf("package fact requires an analyzer and non-nil fact")
	}
	type_ := reflect.TypeOf(fact)
	value := reflect.ValueOf(fact)
	if type_.Kind() != reflect.Pointer || value.IsNil() {
		return nil, fmt.Errorf("package fact %T must be a non-nil pointer", fact)
	}
	for _, declared := range analyzer.FactTypes {
		if reflect.TypeOf(declared) == type_ {
			return type_, nil
		}
	}
	return nil, fmt.Errorf("analyzer %q did not declare package fact type %T", analyzer.Name, fact)
}

func encodePackageFact(fact goanalysis.Fact) ([]byte, error) {
	encode := func() ([]byte, error) {
		var output bytes.Buffer
		if err := gob.NewEncoder(&output).Encode(fact); err != nil {
			return nil, fmt.Errorf("encode package fact %T: %w", fact, err)
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
		return nil, fmt.Errorf("package fact %T encoding is nondeterministic", fact)
	}
	return first, nil
}
