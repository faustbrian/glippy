package analysis

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/gox/internal/cache"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

func TestPackageAnalyzerCacheEntryRestoresDiagnosticsAndFacts(t *testing.T) {
	t.Parallel()

	rule, analyzer := packageAnalyzerCacheTestRule()
	firstTypes, _ := checkFactIdentityFixture(t)
	secondTypes, _ := checkFactIdentityFixture(t)
	path := packageAnalyzerCacheSource(t)
	firstSource := loadPackageAnalyzerCacheSource(t, path)
	secondSource := loadPackageAnalyzerCacheSource(t, path)
	firstPackage := &packages.Package{ID: "example.com/fixture", Types: firstTypes}
	secondPackage := &packages.Package{ID: "example.com/fixture", Types: secondTypes}
	facts := populatedFactSnapshotSet(t, analyzer, firstTypes, "cached")
	diagnostics := []rules.Diagnostic{{
		RuleID: rule.metadata.ID, Severity: rules.SeverityWarn,
		MessageKey: "cached", Message: "cached diagnostic", Path: path,
		Digest: firstSource.Digest(), Range: source.Range{Start: 0, End: 7},
		Related: []rules.Related{}, Notes: []string{"note"}, Fixes: []rules.Fix{},
	}}

	encoded, err := rule.encodePackageCacheEntry(firstPackage, diagnostics, facts)
	if err != nil {
		t.Fatal(err)
	}
	again, err := rule.encodePackageCacheEntry(firstPackage, diagnostics, facts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, again) {
		t.Fatal("package analyzer cache entry encoding is nondeterministic")
	}

	restoredFacts := newAnalyzerFactSet()
	restoredDiagnostics, err := rule.restorePackageCacheEntry(
		secondPackage,
		packageAnalyzerCacheSources(path, secondSource),
		map[string]string{path: secondPackage.ID},
		rules.SeverityWarn,
		restoredFacts,
		encoded,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restoredDiagnostics, diagnostics) {
		t.Fatalf("restored diagnostics = %#v, want %#v", restoredDiagnostics, diagnostics)
	}
	packageFact := new(snapshotFact)
	if !restoredFacts.importPackageFact(analyzer, secondTypes, packageFact) ||
		packageFact.Value != "cached:package" {
		t.Fatalf("restored package fact = %#v", packageFact)
	}
	for name, object := range persistentFixtureObjects(t, secondTypes) {
		fact := new(snapshotFact)
		if !restoredFacts.importObjectFact(analyzer, secondTypes, object, fact) ||
			fact.Value != "cached:"+name {
			t.Fatalf("restored object fact %q = %#v", name, fact)
		}
	}
}

func TestPackageAnalyzerCacheEntryRejectsStaleIdentityWithoutPartialRestore(t *testing.T) {
	t.Parallel()

	rule, analyzer := packageAnalyzerCacheTestRule()
	firstTypes, _ := checkFactIdentityFixture(t)
	secondTypes, _ := checkFactIdentityFixture(t)
	path := packageAnalyzerCacheSource(t)
	file := loadPackageAnalyzerCacheSource(t, path)
	pkg := &packages.Package{ID: "example.com/fixture", Types: firstTypes}
	facts := populatedFactSnapshotSet(t, analyzer, firstTypes, "cached")
	diagnostics := []rules.Diagnostic{{
		RuleID: rule.metadata.ID, Severity: rules.SeverityWarn,
		MessageKey: "cached", Message: "cached diagnostic", Path: path,
		Digest: file.Digest(), Range: source.Range{Start: 0, End: 7},
		Related: []rules.Related{}, Notes: []string{}, Fixes: []rules.Fix{},
	}}
	encoded, err := rule.encodePackageCacheEntry(pkg, diagnostics, facts)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*packageAnalyzerCacheEntry)
	}{
		{name: "schema", mutate: func(entry *packageAnalyzerCacheEntry) { entry.Version++ }},
		{name: "rule", mutate: func(entry *packageAnalyzerCacheEntry) { entry.RuleID = "other-rule" }},
		{name: "package ID", mutate: func(entry *packageAnalyzerCacheEntry) { entry.PackageID = "other-id" }},
		{name: "package path", mutate: func(entry *packageAnalyzerCacheEntry) { entry.PackagePath = "example.com/other" }},
		{name: "source digest", mutate: func(entry *packageAnalyzerCacheEntry) {
			entry.Diagnostics[0].Digest = "00"
		}},
		{name: "noncanonical source digest", mutate: func(entry *packageAnalyzerCacheEntry) {
			entry.Diagnostics[0].Digest = strings.ToUpper(entry.Diagnostics[0].Digest)
		}},
		{name: "diagnostic order", mutate: func(entry *packageAnalyzerCacheEntry) {
			entry.Diagnostics = append(entry.Diagnostics, entry.Diagnostics[0])
		}},
		{name: "fact analyzer", mutate: func(entry *packageAnalyzerCacheEntry) {
			entry.FactSnapshots[0].Analyzer = "other-analyzer"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var entry packageAnalyzerCacheEntry
			if err := json.Unmarshal(encoded, &entry); err != nil {
				t.Fatal(err)
			}
			test.mutate(&entry)
			corrupt, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			restored := newAnalyzerFactSet()
			if _, err := rule.restorePackageCacheEntry(
				&packages.Package{ID: pkg.ID, Types: secondTypes},
				packageAnalyzerCacheSources(path, file),
				map[string]string{path: pkg.ID},
				rules.SeverityWarn,
				restored,
				corrupt,
			); err == nil {
				t.Fatalf("restorePackageCacheEntry() accepted stale %s", test.name)
			}
			if restored.importPackageFact(analyzer, secondTypes, new(snapshotFact)) {
				t.Fatalf("stale %s partially restored package facts", test.name)
			}
		})
	}

	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, file),
		map[string]string{path: pkg.ID},
		rules.SeverityError,
		newAnalyzerFactSet(),
		encoded,
	); err == nil {
		t.Fatal("restorePackageCacheEntry() accepted a stale severity")
	}
	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, loadPackageAnalyzerCacheBytes(t, path, []byte("package changed\n"))),
		map[string]string{path: pkg.ID},
		rules.SeverityWarn,
		newAnalyzerFactSet(),
		encoded,
	); err == nil {
		t.Fatal("restorePackageCacheEntry() accepted stale source bytes")
	}
	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, file),
		map[string]string{path: "another-owner"},
		rules.SeverityWarn,
		newAnalyzerFactSet(),
		encoded,
	); err == nil {
		t.Fatal("restorePackageCacheEntry() accepted stale source ownership")
	}
	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, file),
		map[string]string{path: pkg.ID},
		rules.SeverityWarn,
		newAnalyzerFactSet(),
		append(bytes.Clone(encoded), '\n'),
	); err == nil {
		t.Fatal("restorePackageCacheEntry() accepted noncanonical bytes")
	}
}

func TestPreparePackageCachePlanRejectsAmbientGoEnvironment(t *testing.T) {
	t.Parallel()

	store, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	options := &PackageCacheOptions{
		Store: store, ToolVersion: "v0.1.0", BuildGoVersion: "go1.26.5",
		SourceGoVersion: "1.26", Configuration: cache.DigestOf([]byte("configuration")),
		RuleOptions:   map[string]cache.Digest{"cache-rule": cache.DigestOf(nil)},
		FormatterMode: "gox-v1",
	}
	selection := []rules.Selection{{
		ID: "cache-rule", Severity: rules.SeverityWarn, Requirement: rules.RequireTypes,
	}}
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{name: "GOENV", env: []string{"CGO_ENABLED=0"}, want: "GOENV=off"},
		{name: "CGO", env: []string{"GOENV=off", "CGO_ENABLED=1"}, want: "CGO_ENABLED=0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := preparePackageCachePlan(options, selection, PackageLoadOptions{
				Env: test.env, GOOS: "linux", GOARCH: "amd64",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("preparePackageCachePlan() error = %v, want %q", err, test.want)
			}
		})
	}
}

func packageAnalyzerCacheTestRule() (*packageAnalyzerRule, *goanalysis.Analyzer) {
	analyzer := &goanalysis.Analyzer{
		Name: "cachefacts", FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	rule := &packageAnalyzerRule{
		metadata: rules.Metadata{ID: "cache-rule"},
		steps:    []packageAnalyzerStep{{original: analyzer, analyzer: *analyzer}},
	}
	return rule, analyzer
}

func packageAnalyzerCacheSource(t *testing.T) string {
	t.Helper()
	return writePackageCacheFile(t, t.TempDir(), "fixture.go", "package fixture\n")
}

func loadPackageAnalyzerCacheSource(t *testing.T, path string) *source.File {
	t.Helper()
	return loadPackageAnalyzerCacheBytes(t, path, []byte("package fixture\n"))
}

func loadPackageAnalyzerCacheBytes(t *testing.T, path string, contents []byte) *source.File {
	t.Helper()
	file, err := source.Load(path, contents)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func packageAnalyzerCacheSources(path string, file *source.File) PackageSourceSet {
	return PackageSourceSet{paths: []string{path}, files: map[string]*source.File{path: file}}
}

func FuzzDecodePackageAnalyzerCacheEntry(f *testing.F) {
	f.Add([]byte(`{"version":1,"rule":"r","packageId":"p","packagePath":"p","diagnostics":[],"factSnapshots":[]}`))
	f.Add([]byte("corrupt"))
	f.Fuzz(func(t *testing.T, encoded []byte) {
		entry, err := decodePackageAnalyzerCacheEntry(encoded)
		if err != nil {
			return
		}
		reencoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(reencoded, encoded) {
			t.Fatal("accepted package analyzer cache entry was not canonical")
		}
	})
}
