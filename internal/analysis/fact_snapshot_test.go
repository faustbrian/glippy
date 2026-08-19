package analysis

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/token"
	"go/types"
	"testing"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

type snapshotFact struct {
	Value string
}

func (*snapshotFact) AFact() {}

type otherSnapshotFact struct{}

func (*otherSnapshotFact) AFact() {}

func TestPackageFactSnapshotRestoresAcrossIndependentTypeChecks(t *testing.T) {
	t.Parallel()

	analyzer := &goanalysis.Analyzer{
		Name: "persistent-facts",
		FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	first, _ := checkFactIdentityFixture(t)
	second, _ := checkFactIdentityFixture(t)
	firstFacts := populatedFactSnapshotSet(t, analyzer, first, "original")
	encoded, err := firstFacts.encodePackageFactSnapshot(analyzer, first)
	if err != nil {
		t.Fatal(err)
	}
	again, err := firstFacts.encodePackageFactSnapshot(analyzer, first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, again) {
		t.Fatal("fact snapshot encoding is nondeterministic")
	}

	restored := newAnalyzerFactSet()
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		encoded,
	);
		err != nil {
		t.Fatal(err)
	}
	packageFact := new(snapshotFact)
	if !restored.importPackageFact(analyzer, second, packageFact) ||
		packageFact.Value != "original:package" {
		t.Fatalf("restored package fact = %#v", packageFact)
	}
	for name, object := range persistentFixtureObjects(t, second) {
		fact := new(snapshotFact)
		if !restored.importObjectFact(analyzer, second, object, fact) {
			t.Fatalf("restored object fact %q was not found", name)
		}
		if want := "original:" + name; fact.Value != want {
			t.Fatalf("restored object fact %q = %q, want %q", name, fact.Value, want)
		}
	}
	reencoded, err := restored.encodePackageFactSnapshot(analyzer, second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reencoded, encoded) {
		t.Fatal("restored fact snapshot changed encoding")
	}
}

func TestPackageFactSnapshotsRestoreDependencyGraph(t *testing.T) {
	t.Parallel()

	analyzer := &goanalysis.Analyzer{
		Name: "persistent-facts",
		FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	firstDependency, firstRoot := factSnapshotPackages()
	first := newAnalyzerFactSet()
	if err := first.beginObjectFacts(analyzer, &packages.Package{Types: firstDependency});
		err != nil {
		t.Fatal(err)
	}
	first.exportPackageFact(
		analyzer,
		firstDependency,
		&snapshotFact{Value: "dependency package"},
	)
	first.exportObjectFact(
		analyzer,
		firstDependency,
		firstDependency.Scope().Lookup("Exported"),
		&snapshotFact{Value: "dependency object"},
	)
	if err := first.beginObjectFacts(analyzer, &packages.Package{Types: firstRoot});
		err != nil {
		t.Fatal(err)
	}
	dependencySnapshot, err := first.encodePackageFactSnapshot(analyzer, firstDependency)
	if err != nil {
		t.Fatal(err)
	}
	rootSnapshot, err := first.encodePackageFactSnapshot(analyzer, firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	decodedRoot, err := decodePackageFactSnapshot(rootSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(decodedRoot.PackageFacts) != 0 || len(decodedRoot.ObjectFacts) != 0 {
		t.Fatalf("root snapshot duplicated imported facts = %#v", decodedRoot)
	}

	secondDependency, secondRoot := factSnapshotPackages()
	restored := newAnalyzerFactSet()
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: secondDependency},
		dependencySnapshot,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: secondRoot},
		rootSnapshot,
	);
		err != nil {
		t.Fatal(err)
	}
	packageFact := new(snapshotFact)
	if !restored.importPackageFact(analyzer, secondDependency, packageFact) ||
		packageFact.Value != "dependency package" {
		t.Fatalf("restored dependency package fact = %#v", packageFact)
	}
	objectFact := new(snapshotFact)
	if !restored.importObjectFact(
		analyzer,
		secondRoot,
		secondDependency.Scope().Lookup("Exported"),
		objectFact,
	) ||
		objectFact.Value != "dependency object" {
		t.Fatalf("restored dependency object fact = %#v", objectFact)
	}
}

func TestImportablePackageFactSnapshotSkipsUnavailableObjects(t *testing.T) {
	t.Parallel()

	analyzer := &goanalysis.Analyzer{
		Name: "persistent-facts",
		FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	complete, _ := checkFactIdentityFixture(t)
	encoded, err := populatedFactSnapshotSet(
		t,
		analyzer,
		complete,
		"complete",
	).encodeImportablePackageFactSnapshot(analyzer, complete)
	if err != nil {
		t.Fatal(err)
	}
	partial := types.NewPackage(complete.Path(), complete.Name())
	partial.Scope().Insert(types.NewVar(token.NoPos, partial, "Exported", types.Typ[types.Int]))
	partial.MarkComplete()
	strict := newAnalyzerFactSet()
	if err := strict.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: partial},
		encoded,
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted an incomplete package scope")
	}
	restored := newAnalyzerFactSet()
	if err := restored.restoreImportablePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: partial},
		encoded,
	);
		err != nil {
		t.Fatal(err)
	}
	fact := new(snapshotFact)
	if !restored.importObjectFact(
		analyzer,
		partial,
		partial.Scope().Lookup("Exported"),
		fact,
	) ||
		fact.Value != "complete:exported variable" {
		t.Fatalf("restored available object fact = %#v", fact)
	}
	if facts := restored.allObjectFacts(analyzer, partial); len(facts) != 1 {
		t.Fatalf("restored available object facts = %#v", facts)
	}
}

func TestPackageFactSnapshotRejectsUnstableObjects(t *testing.T) {
	t.Parallel()

	analyzer := &goanalysis.Analyzer{
		Name: "persistent-facts",
		FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	pkg, definitions := checkFactIdentityFixture(t)
	facts := newAnalyzerFactSet()
	if err := facts.beginObjectFacts(analyzer, &packages.Package{Types: pkg}); err != nil {
		t.Fatal(err)
	}
	facts.exportObjectFact(analyzer, pkg, definitions["local"], &snapshotFact{Value: "local"})
	if _, err := facts.encodePackageFactSnapshot(analyzer, pkg); err == nil {
		t.Fatal("encodePackageFactSnapshot() accepted a local object fact")
	}
	duplicateTypes := &goanalysis.Analyzer{
		Name: "duplicate-facts",
		FactTypes: []goanalysis.Fact{new(snapshotFact), new(snapshotFact)},
	}
	duplicateFacts := populatedFactSnapshotSet(t, duplicateTypes, pkg, "duplicate")
	if _, err := duplicateFacts.encodePackageFactSnapshot(duplicateTypes, pkg); err == nil {
		t.Fatal("encodePackageFactSnapshot() accepted duplicate fact types")
	}
}

func TestPackageFactSnapshotRestoreIsValidatedAndTransactional(t *testing.T) {
	t.Parallel()

	analyzer := &goanalysis.Analyzer{
		Name: "persistent-facts",
		FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	first, _ := checkFactIdentityFixture(t)
	second, _ := checkFactIdentityFixture(t)
	encoded, err := populatedFactSnapshotSet(
		t,
		analyzer,
		first,
		"first",
	).encodePackageFactSnapshot(analyzer, first)
	if err != nil {
		t.Fatal(err)
	}
	restored := newAnalyzerFactSet()
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		append(bytes.Clone(encoded), '\n'),
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted a noncanonical snapshot")
	}
	if restored.importPackageFact(analyzer, second, new(snapshotFact)) {
		t.Fatal("invalid snapshot partially restored a package fact")
	}
	var corrupt packageFactSnapshot
	if err := json.Unmarshal(encoded, &corrupt); err != nil {
		t.Fatal(err)
	}
	corrupt.PackageFacts[0].Value = []byte("corrupt")
	corruptBytes, err := json.Marshal(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		corruptBytes,
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted corrupt fact bytes")
	}
	stale := corrupt
	if err := json.Unmarshal(encoded, &stale); err != nil {
		t.Fatal(err)
	}
	stale.ObjectFacts[0].Object.ObjectPath = "missing"
	staleBytes, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		staleBytes,
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted a stale object path")
	}
	unordered := stale
	if err := json.Unmarshal(encoded, &unordered); err != nil {
		t.Fatal(err)
	}
	unordered.ObjectFacts[0], unordered.ObjectFacts[1] = unordered.ObjectFacts[1], unordered.ObjectFacts[0]
	unorderedBytes, err := json.Marshal(unordered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePackageFactSnapshot(unorderedBytes); err == nil {
		t.Fatal("decodePackageFactSnapshot() accepted unordered object facts")
	}
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		unorderedBytes,
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted unordered object facts")
	}
	duplicateOrder := stale
	if err := json.Unmarshal(encoded, &duplicateOrder); err != nil {
		t.Fatal(err)
	}
	duplicateOrder.ObjectFacts[1].Order = duplicateOrder.ObjectFacts[0].Order
	duplicateOrderBytes, err := json.Marshal(duplicateOrder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePackageFactSnapshot(duplicateOrderBytes); err == nil {
		t.Fatal("decodePackageFactSnapshot() accepted duplicate object fact order")
	}
	outOfRangeOrder := stale
	if err := json.Unmarshal(encoded, &outOfRangeOrder); err != nil {
		t.Fatal(err)
	}
	outOfRangeOrder.ObjectFacts[0].Order = len(outOfRangeOrder.ObjectFacts)
	outOfRangeOrderBytes, err := json.Marshal(outOfRangeOrder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePackageFactSnapshot(outOfRangeOrderBytes); err == nil {
		t.Fatal("decodePackageFactSnapshot() accepted out-of-range object fact order")
	}

	wrongAnalyzer := &goanalysis.Analyzer{
		Name: analyzer.Name,
		FactTypes: []goanalysis.Fact{new(otherSnapshotFact)},
	}
	if err := restored.restorePackageFactSnapshot(
		wrongAnalyzer,
		&packages.Package{Types: second},
		encoded,
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted an undeclared fact type")
	}
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: types.NewPackage("example.com/other", "other")},
		encoded,
	);
		err == nil {
		t.Fatal("restorePackageFactSnapshot() accepted a different package")
	}

	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		encoded,
	);
		err != nil {
		t.Fatal(err)
	}
	conflicting, err := populatedFactSnapshotSet(
		t,
		analyzer,
		first,
		"second",
	).encodePackageFactSnapshot(analyzer, first)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.restorePackageFactSnapshot(
		analyzer,
		&packages.Package{Types: second},
		conflicting,
	);
		!errors.Is(err, errFactSnapshotConflict) {
		t.Fatalf("conflicting restore error = %v", err)
	}
	packageFact := new(snapshotFact)
	if !restored.importPackageFact(analyzer, second, packageFact) ||
		packageFact.Value != "first:package" {
		t.Fatalf("conflicting restore changed package fact = %#v", packageFact)
	}
}

func populatedFactSnapshotSet(
	t *testing.T,
	analyzer *goanalysis.Analyzer,
	pkg *types.Package,
	prefix string,
) *analyzerFactSet {
	t.Helper()
	facts := newAnalyzerFactSet()
	if err := facts.beginObjectFacts(analyzer, &packages.Package{Types: pkg}); err != nil {
		t.Fatal(err)
	}
	facts.exportPackageFact(analyzer, pkg, &snapshotFact{Value: prefix + ":package"})
	for name, object := range persistentFixtureObjects(t, pkg) {
		facts.exportObjectFact(
			analyzer,
			pkg,
			object,
			&snapshotFact{Value: prefix + ":" + name},
		)
	}
	return facts
}

func factSnapshotPackages() (*types.Package, *types.Package) {
	dependency := types.NewPackage("example.com/dependency", "dependency")
	dependency.Scope().Insert(
		types.NewVar(token.NoPos, dependency, "Exported", types.Typ[types.Int]),
	)
	dependency.MarkComplete()
	root := types.NewPackage("example.com/root", "root")
	root.SetImports([]*types.Package{dependency})
	root.MarkComplete()
	return dependency, root
}

func FuzzDecodePackageFactSnapshot(f *testing.F) {
	f.Add(
		[]byte(
			`{"version":2,"analyzer":"a","package":"p","packageFacts":[],"objectFacts":[]}`,
		),
	)
	f.Add([]byte("corrupt"))
	f.Fuzz(
		func(t *testing.T, encoded []byte) {
			snapshot, err := decodePackageFactSnapshot(encoded)
			if err != nil {
				return
			}
			reencoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(reencoded, encoded) {
				t.Fatal("accepted fact snapshot was not canonical")
			}
		},
	)
}
