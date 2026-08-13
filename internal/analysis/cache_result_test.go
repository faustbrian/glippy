package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"reflect"
	"strings"
	"testing"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/faustbrian/gox/internal/cache"
	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type nativeCacheTestRule struct {
	metadata rules.Metadata
}

type nativeCachePackageRule struct {
	metadata rules.Metadata
}

func (r nativeCacheTestRule) Metadata() rules.Metadata {
	return r.metadata
}

func (nativeCacheTestRule) RunTypes(*rules.TypesContext, ast.Node) ([]rules.Finding, error) {
	return nil, nil
}

func (r nativeCachePackageRule) Metadata() rules.Metadata {
	return r.metadata
}

func (nativeCachePackageRule) RunPackage(*rules.PackageContext) ([]rules.PackageFinding, error) {
	return nil, nil
}

func TestNativeRuleSnapshotsBindDependencySyntaxRequirement(t *testing.T) {
	t.Parallel()

	metadata := rules.Metadata{
		ID: "dependency-cache",
		Summary: "inspects dependency syntax",
		Documentation: "Full dependency-aware package rule documentation.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement: rules.RequireTypes,
		RequiresDependencySyntax: true,
		Categories: []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}
	registry, err := rules.NewRegistry(nativeCachePackageRule{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := nativeRuleSnapshots(
		registry,
		[]rules.Selection{
			{
				ID: metadata.ID,
				Severity: rules.SeverityWarn,
				Requirement: rules.RequireTypes,
			},
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || !snapshots[0].RequiresDependencySyntax {
		t.Fatalf("native rule snapshots = %#v", snapshots)
	}
}

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
	diagnostics := []rules.Diagnostic{
		{
			RuleID: rule.metadata.ID,
			Severity: rules.SeverityWarn,
			MessageKey: "cached",
			Message: "cached diagnostic",
			Path: path,
			Digest: firstSource.Digest(),
			Range: source.Range{Start: 0, End: 7},
			Related: []rules.Related{},
			Notes: []string{"note"},
			Fixes: []rules.Fix{},
		},
	}

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
			fact.Value != "cached:" + name {
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
	diagnostics := []rules.Diagnostic{
		{
			RuleID: rule.metadata.ID,
			Severity: rules.SeverityWarn,
			MessageKey: "cached",
			Message: "cached diagnostic",
			Path: path,
			Digest: file.Digest(),
			Range: source.Range{Start: 0, End: 7},
			Related: []rules.Related{},
			Notes: []string{},
			Fixes: []rules.Fix{},
		},
	}
	encoded, err := rule.encodePackageCacheEntry(pkg, diagnostics, facts)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		mutate func(*packageAnalyzerCacheEntry)
	}{
		{
			name: "schema",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.Version++
			},
		},
		{
			name: "rule",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.RuleID = "other-rule"
			},
		},
		{
			name: "package ID",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.PackageID = "other-id"
			},
		},
		{
			name: "package path",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.PackagePath = "example.com/other"
			},
		},
		{
			name: "source digest",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.Diagnostics[0].Digest = "00"
			},
		},
		{
			name: "noncanonical source digest",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.Diagnostics[0].Digest = strings.ToUpper(
					entry.Diagnostics[0].Digest,
				)
			},
		},
		{
			name: "diagnostic order",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.Diagnostics = append(entry.Diagnostics, entry.Diagnostics[0])
			},
		},
		{
			name: "fact analyzer",
			mutate: func(entry *packageAnalyzerCacheEntry) {
				entry.FactSnapshots[0].Analyzer = "other-analyzer"
			},
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
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
				);
					err == nil {
					t.Fatalf(
						"restorePackageCacheEntry() accepted stale %s",
						test.name,
					)
				}
				if restored.importPackageFact(
					analyzer,
					secondTypes,
					new(snapshotFact),
				) {
					t.Fatalf(
						"stale %s partially restored package facts",
						test.name,
					)
				}
			},
		)
	}

	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, file),
		map[string]string{path: pkg.ID},
		rules.SeverityError,
		newAnalyzerFactSet(),
		encoded,
	);
		err == nil {
		t.Fatal("restorePackageCacheEntry() accepted a stale severity")
	}
	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(
			path,
			loadPackageAnalyzerCacheBytes(t, path, []byte("package changed\n")),
		),
		map[string]string{path: pkg.ID},
		rules.SeverityWarn,
		newAnalyzerFactSet(),
		encoded,
	);
		err == nil {
		t.Fatal("restorePackageCacheEntry() accepted stale source bytes")
	}
	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, file),
		map[string]string{path: "another-owner"},
		rules.SeverityWarn,
		newAnalyzerFactSet(),
		encoded,
	);
		err == nil {
		t.Fatal("restorePackageCacheEntry() accepted stale source ownership")
	}
	if _, err := rule.restorePackageCacheEntry(
		&packages.Package{ID: pkg.ID, Types: secondTypes},
		packageAnalyzerCacheSources(path, file),
		map[string]string{path: pkg.ID},
		rules.SeverityWarn,
		newAnalyzerFactSet(),
		append(bytes.Clone(encoded), '\n'),
	);
		err == nil {
		t.Fatal("restorePackageCacheEntry() accepted noncanonical bytes")
	}
}

func TestPreparePackageCachePlanRejectsAmbientGoEnvironment(t *testing.T) {
	t.Parallel()

	store, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	options := &PackageCacheOptions{
		Store: store,
		ToolVersion: "v0.1.0",
		BuildGoVersion: "go1.26.5",
		SourceGoVersion: "1.26",
		Configuration: cache.DigestOf([]byte("configuration")),
		FormatterMode: "gox-v1",
	}
	selection := []rules.Selection{
		{ID: "cache-rule", Severity: rules.SeverityWarn, Requirement: rules.RequireTypes},
	}
	tests := []struct {
		name string
		env []string
		want string
	}{
		{name: "GOENV", env: []string{"CGO_ENABLED=0"}, want: "GOENV=off"},
		{name: "CGO", env: []string{"GOENV=off", "CGO_ENABLED=1"}, want: "CGO_ENABLED=0"},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := preparePackageCachePlan(
					options,
					selection,
					PackageLoadOptions{
						Env: test.env,
						GOOS: "linux",
						GOARCH: "amd64",
					},
				)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"preparePackageCachePlan() error = %v, want %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestPreparePackageCachePlanDerivesResolvedRuleOptions(t *testing.T) {
	t.Parallel()

	store, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(
		func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		},
	)
	options := &PackageCacheOptions{
		Store: store,
		ToolVersion: "v0.1.0",
		BuildGoVersion: "go1.26.5",
		SourceGoVersion: "1.26",
		Configuration: cache.DigestOf([]byte("configuration")),
		FormatterMode: "gox-v1",
	}
	loadOptions := PackageLoadOptions{
		Env: []string{"GOENV=off", "CGO_ENABLED=0"},
		GOOS: "linux",
		GOARCH: "amd64",
	}
	selection := func(enabled bool) []rules.Selection {
		return []rules.Selection{
			{
				ID: "cache-rule",
				Severity: rules.SeverityWarn,
				Requirement: rules.RequireTypes,
				Options: rules.NewOptionSet(
					map[string]rules.OptionValue{
						"enabled": rules.BooleanOption(enabled),
					},
				),
			},
		}
	}
	first, err := preparePackageCachePlan(options, selection(false), loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparePackageCachePlan(options, selection(true), loadOptions)
	if err != nil {
		t.Fatal(err)
	}
	if first.rules[0].Options == second.rules[0].Options {
		t.Fatal("resolved rule option values produced one cache identity")
	}
}

func TestRestoreNativePackageCacheEntryRejectsChangedPackageOwnership(t *testing.T) {
	root := t.TempDir()
	writePackageCacheFile(t, root, "go.mod", "module example.com/nativeowner\n\ngo 1.26.0\n")
	firstPath := writePackageCacheFile(t, root, "first/first.go", "package first\n")
	secondPath := writePackageCacheFile(t, root, "second/second.go", "package second\n")
	loaded, err := LoadPackages(
		context.Background(),
		PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			Requirement: rules.RequireTypes,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	metadata := rules.Metadata{
		ID: "native-owner",
		Summary: "reports typed syntax",
		Documentation: "Full typed rule documentation.",
		DefaultSeverity: rules.SeverityWarn,
		Presets: []rules.Preset{rules.PresetCorrectness},
		MinimumGoVersion: "1.22",
		Requirement: rules.RequireTypes,
		NodeInterests: []rules.NodeKind{rules.NodeFile},
		Categories: []rules.Category{rules.CategoryCorrectness},
		Examples: []rules.Example{{Incorrect: "bad()", Correct: "good()"}},
	}
	registry, err := rules.NewRegistry(nativeCacheTestRule{metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	selection := []rules.Selection{
		{ID: metadata.ID, Severity: rules.SeverityWarn, Requirement: rules.RequireTypes},
	}
	snapshots, err := nativeRuleSnapshots(registry, selection, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].RequiresDependencySyntax {
		t.Fatalf("native rule snapshots = %#v", snapshots)
	}
	file, found := loaded.Sources.Lookup(firstPath)
	if !found {
		t.Fatalf("loaded source %q is missing", firstPath)
	}
	range_, found := file.TokenRangeAtOffset(len("package "))
	if !found {
		t.Fatal("package name token is missing")
	}
	diagnostic, err := diagnosticForFinding(
		file,
		metadata,
		rules.SeverityWarn,
		rules.Finding{MessageKey: "owner", Message: "owner", Range: range_},
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeNativePackageCacheEntry(
		selection,
		snapshots,
		loaded,
		[]rules.Diagnostic{diagnostic},
	)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := decodeNativePackageCacheEntry(encoded)
	if err != nil {
		t.Fatal(err)
	}
	missingRule := entry
	missingRule.Rules = []nativeRuleSnapshot{}
	missingRuleBytes, err := json.Marshal(missingRule)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreNativePackageCacheEntry(
		registry,
		selection,
		snapshots,
		loaded,
		missingRuleBytes,
	);
		err == nil || !strings.Contains(err.Error(), "rule metadata is stale") {
		t.Fatalf("restoreNativePackageCacheEntry() rule-set error = %v", err)
	}
	owners, err := nativeSourceOwners(loaded)
	if err != nil {
		t.Fatal(err)
	}
	secondOwner, found := owners[secondPath]
	if !found || secondOwner == entry.Diagnostics[0].PackageID {
		t.Fatalf("package owners = %#v", owners)
	}
	entry.Diagnostics[0].PackageID = secondOwner
	corrupt, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreNativePackageCacheEntry(
		registry,
		selection,
		snapshots,
		loaded,
		corrupt,
	);
		err == nil || !strings.Contains(err.Error(), "source is not owned") {
		t.Fatalf("restoreNativePackageCacheEntry() ownership error = %v", err)
	}
}

func packageAnalyzerCacheTestRule() (*packageAnalyzerRule, *goanalysis.Analyzer) {
	analyzer := &goanalysis.Analyzer{
		Name: "cachefacts",
		FactTypes: []goanalysis.Fact{new(snapshotFact)},
	}
	rule := &packageAnalyzerRule{
		metadata: rules.Metadata{ID: "cache-rule"},
		steps: []packageAnalyzerStep{{original: analyzer, analyzer: *analyzer}},
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
	f.Add(
		[]byte(
			`{"version":1,"rule":"r","packageId":"p","packagePath":"p","diagnostics":[],"factSnapshots":[]}`,
		),
	)
	f.Add([]byte("corrupt"))
	f.Fuzz(
		func(t *testing.T, encoded []byte) {
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
		},
	)
}

func FuzzDecodeNativePackageCacheEntry(f *testing.F) {
	f.Add([]byte(`{"version":1,"requirement":2,"rules":[],"diagnostics":[]}`))
	f.Add([]byte("corrupt"))
	f.Fuzz(
		func(t *testing.T, encoded []byte) {
			entry, err := decodeNativePackageCacheEntry(encoded)
			if err != nil {
				return
			}
			reencoded, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(reencoded, encoded) {
				t.Fatal("accepted native analysis cache entry was not canonical")
			}
		},
	)
}
