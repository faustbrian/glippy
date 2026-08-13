package analysis

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/types"
	"io"
	"reflect"
	"sort"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/glippy/internal/cache"
)

const factSnapshotVersion = 1

var errFactSnapshotConflict = errors.New("fact snapshot conflicts with existing facts")

type factTypeIdentity struct {
	PackagePath string `json:"package"`
	Name string `json:"name"`
}

type persistedFact struct {
	Type factTypeIdentity `json:"type"`
	Value []byte `json:"value"`
}

type persistedObjectFact struct {
	Object factObjectIdentity `json:"object"`
	Type factTypeIdentity `json:"type"`
	Value []byte `json:"value"`
}

type packageFactSnapshot struct {
	Version int `json:"version"`
	Analyzer string `json:"analyzer"`
	PackagePath string `json:"package"`
	PackageFacts []persistedFact `json:"packageFacts"`
	ObjectFacts []persistedObjectFact `json:"objectFacts"`
}

func (s *analyzerFactSet) encodePackageFactSnapshot(
	analyzer *goanalysis.Analyzer,
	current *types.Package,
) ([]byte, error) {
	if s == nil || current == nil || current.Path() == "" {
		return nil, fmt.Errorf("encode fact snapshot requires a fact set and package")
	}
	typesByIdentity, err := declaredFactTypeIdentities(analyzer)
	if err != nil {
		return nil, err
	}
	identitiesByType := make(map[reflect.Type]factTypeIdentity, len(typesByIdentity))
	for identity, type_ := range typesByIdentity {
		identitiesByType[type_] = identity
	}
	snapshot := packageFactSnapshot{
		Version: factSnapshotVersion,
		Analyzer: analyzer.Name,
		PackagePath: current.Path(),
		PackageFacts: []persistedFact{},
		ObjectFacts: []persistedObjectFact{},
	}
	for key, encoded := range s.packageValues {
		if key.analyzer != analyzer || key.package_ != current {
			continue
		}
		identity, found := identitiesByType[key.type_]
		if !found {
			return nil, fmt.Errorf(
				"snapshot package fact has an undeclared type %v",
				key.type_,
			)
		}
		snapshot.PackageFacts = append(
			snapshot.PackageFacts,
			persistedFact{Type: identity, Value: bytes.Clone(encoded)},
		)
	}
	view, found := s.objectViews[objectFactViewKey{analyzer: analyzer, package_: current}]
	if !found {
		return nil, fmt.Errorf("snapshot object fact view was not initialized")
	}
	encoder := newFactObjectEncoder()
	for key, encoded := range view.values {
		if key.object.Pkg() != current {
			continue
		}
		object, err := encoder.Identity(key.object)
		if err != nil {
			return nil, fmt.Errorf(
				"snapshot object fact %s: %w",
				key.object.Name(),
				err,
			)
		}
		identity, found := identitiesByType[key.type_]
		if !found {
			return nil, fmt.Errorf(
				"snapshot object fact has an undeclared type %v",
				key.type_,
			)
		}
		snapshot.ObjectFacts = append(
			snapshot.ObjectFacts,
			persistedObjectFact{
				Object: object,
				Type: identity,
				Value: bytes.Clone(encoded),
			},
		)
	}
	sort.Slice(
		snapshot.PackageFacts,
		func(left, right int) bool {
			return lessFactTypeIdentity(
				snapshot.PackageFacts[left].Type,
				snapshot.PackageFacts[right].Type,
			)
		},
	)
	sort.Slice(
		snapshot.ObjectFacts,
		func(left, right int) bool {
			return lessPersistedObjectFact(
				snapshot.ObjectFacts[left],
				snapshot.ObjectFacts[right],
			)
		},
	)
	if err := validateFactSnapshotOrder(snapshot); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode fact snapshot: %w", err)
	}
	if len(encoded) > cache.MaxEntrySize {
		return nil, fmt.Errorf(
			"fact snapshot is %d bytes; maximum is %d",
			len(encoded),
			cache.MaxEntrySize,
		)
	}
	return encoded, nil
}

func (s *analyzerFactSet) restorePackageFactSnapshot(
	analyzer *goanalysis.Analyzer,
	pkg *packages.Package,
	encoded []byte,
) error {
	if s == nil || pkg == nil || pkg.Types == nil || pkg.Types.Path() == "" {
		return fmt.Errorf("restore fact snapshot requires a fact set and typed package")
	}
	typesByIdentity, err := declaredFactTypeIdentities(analyzer)
	if err != nil {
		return err
	}
	snapshot, err := decodePackageFactSnapshot(encoded)
	if err != nil {
		return err
	}
	if snapshot.Analyzer != analyzer.Name {
		return fmt.Errorf(
			"fact snapshot analyzer %q does not match %q",
			snapshot.Analyzer,
			analyzer.Name,
		)
	}
	if snapshot.PackagePath != pkg.Types.Path() {
		return fmt.Errorf(
			"fact snapshot package %q does not match %q",
			snapshot.PackagePath,
			pkg.Types.Path(),
		)
	}
	if err := validateFactSnapshotOrder(snapshot); err != nil {
		return err
	}
	packageValues := make(map[packageFactKey][]byte, len(snapshot.PackageFacts))
	for _, persisted := range snapshot.PackageFacts {
		type_, found := typesByIdentity[persisted.Type]
		if !found {
			return fmt.Errorf(
				"fact snapshot contains undeclared type %s",
				persisted.Type,
			)
		}
		if err := validatePersistedFact(type_, persisted.Value); err != nil {
			return err
		}
		packageValues[packageFactKey{
			analyzer: analyzer,
			package_: pkg.Types,
			type_: type_,
		}] = bytes.Clone(persisted.Value)
	}
	objectValues := make(map[objectFactKey][]byte, len(snapshot.ObjectFacts))
	resolver, err := newFactObjectResolver(pkg.Types)
	if err != nil {
		return err
	}
	for _, persisted := range snapshot.ObjectFacts {
		if persisted.Object.PackagePath != pkg.Types.Path() {
			return fmt.Errorf(
				"fact snapshot object package %q does not match %q",
				persisted.Object.PackagePath,
				pkg.Types.Path(),
			)
		}
		object, err := resolver.Resolve(persisted.Object)
		if err != nil {
			return err
		}
		type_, found := typesByIdentity[persisted.Type]
		if !found {
			return fmt.Errorf(
				"fact snapshot contains undeclared type %s",
				persisted.Type,
			)
		}
		if err := validatePersistedFact(type_, persisted.Value); err != nil {
			return err
		}
		objectValues[objectFactKey{object: object, type_: type_}] = bytes.Clone(
			persisted.Value,
		)
	}
	for key, value := range packageValues {
		if existing, found := s.packageValues[key]; found && !bytes.Equal(existing, value) {
			return fmt.Errorf(
				"%w for package fact %s",
				errFactSnapshotConflict,
				key.type_,
			)
		}
	}
	if err := s.beginObjectFacts(analyzer, pkg); err != nil {
		return err
	}
	view := s.objectViews[objectFactViewKey{analyzer: analyzer, package_: pkg.Types}]
	for key, value := range objectValues {
		if existing, found := view.values[key]; found && !bytes.Equal(existing, value) {
			return fmt.Errorf(
				"%w for object fact %s %s",
				errFactSnapshotConflict,
				key.object.Name(),
				key.type_,
			)
		}
	}
	for key, value := range packageValues {
		s.packageValues[key] = value
	}
	for key, value := range objectValues {
		view.values[key] = value
	}
	return nil
}

func decodePackageFactSnapshot(encoded []byte) (packageFactSnapshot, error) {
	if len(encoded) == 0 || len(encoded) > cache.MaxEntrySize {
		return packageFactSnapshot{}, fmt.Errorf(
			"fact snapshot size %d is outside 1..%d",
			len(encoded),
			cache.MaxEntrySize,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var snapshot packageFactSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return packageFactSnapshot{}, fmt.Errorf("decode fact snapshot: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return packageFactSnapshot{}, fmt.Errorf("decode fact snapshot trailing data")
	}
	canonical, err := json.Marshal(snapshot)
	if err != nil {
		return packageFactSnapshot{}, fmt.Errorf("re-encode fact snapshot: %w", err)
	}
	if !bytes.Equal(canonical, encoded) {
		return packageFactSnapshot{}, fmt.Errorf("fact snapshot is not canonically encoded")
	}
	if snapshot.Version != factSnapshotVersion {
		return packageFactSnapshot{}, fmt.Errorf(
			"fact snapshot version %d is unsupported",
			snapshot.Version,
		)
	}
	if snapshot.Analyzer == "" ||
		snapshot.PackagePath == "" ||
		snapshot.PackageFacts == nil ||
		snapshot.ObjectFacts == nil {
		return packageFactSnapshot{}, fmt.Errorf("fact snapshot identity is incomplete")
	}
	if err := validateFactSnapshotOrder(snapshot); err != nil {
		return packageFactSnapshot{}, err
	}
	return snapshot, nil
}

func declaredFactTypeIdentities(
	analyzer *goanalysis.Analyzer,
) (map[factTypeIdentity]reflect.Type, error) {
	if analyzer == nil || analyzer.Name == "" {
		return nil, fmt.Errorf("fact snapshot requires a named analyzer")
	}
	result := make(map[factTypeIdentity]reflect.Type, len(analyzer.FactTypes))
	for _, declared := range analyzer.FactTypes {
		type_ := reflect.TypeOf(declared)
		if type_ == nil ||
			type_.Kind() != reflect.Pointer ||
			type_.Elem().Name() == "" ||
			type_.Elem().PkgPath() == "" {
			return nil, fmt.Errorf(
				"analyzer %q has an unstable fact type %T",
				analyzer.Name,
				declared,
			)
		}
		identity := factTypeIdentity{
			PackagePath: type_.Elem().PkgPath(),
			Name: type_.Elem().Name(),
		}
		if _, duplicate := result[identity]; duplicate {
			return nil, fmt.Errorf(
				"analyzer %q has duplicate fact identity %s",
				analyzer.Name,
				identity,
			)
		}
		result[identity] = type_
	}
	return result, nil
}

func validatePersistedFact(type_ reflect.Type, encoded []byte) error {
	fact := reflect.New(type_.Elem()).Interface().(goanalysis.Fact)
	if err := decodeFact(encoded, fact); err != nil {
		return fmt.Errorf("decode persisted fact %s: %w", type_, err)
	}
	canonical, err := encodeFact(fact)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, encoded) {
		return fmt.Errorf("persisted fact %s is not canonically encoded", type_)
	}
	return nil
}

func validateFactSnapshotOrder(snapshot packageFactSnapshot) error {
	for index := 1; index < len(snapshot.PackageFacts); index++ {
		if !lessFactTypeIdentity(
			snapshot.PackageFacts[index - 1].Type,
			snapshot.PackageFacts[index].Type,
		) {
			return fmt.Errorf("fact snapshot package facts are duplicated or unordered")
		}
	}
	for index := 1; index < len(snapshot.ObjectFacts); index++ {
		if !lessPersistedObjectFact(
			snapshot.ObjectFacts[index - 1],
			snapshot.ObjectFacts[index],
		) {
			return fmt.Errorf("fact snapshot object facts are duplicated or unordered")
		}
	}
	return nil
}

func lessFactTypeIdentity(left, right factTypeIdentity) bool {
	if left.PackagePath != right.PackagePath {
		return left.PackagePath < right.PackagePath
	}
	return left.Name < right.Name
}

func lessPersistedObjectFact(left, right persistedObjectFact) bool {
	if left.Object.PackagePath != right.Object.PackagePath {
		return left.Object.PackagePath < right.Object.PackagePath
	}
	if left.Object.ObjectPath != right.Object.ObjectPath {
		return left.Object.ObjectPath < right.Object.ObjectPath
	}
	return lessFactTypeIdentity(left.Type, right.Type)
}

func (i factTypeIdentity) String() string {
	return i.PackagePath + "." + i.Name
}
