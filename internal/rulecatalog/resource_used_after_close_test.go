package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/config"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestResourceUsedAfterCloseReportsDefinitelyClosedOperations(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func openWithCallback(any) (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func (*resource) Write([]byte) (int, error) { return 0, nil }
func (*resource) ReadByte() (byte, error) { return 0, nil }
func (*resource) Sync() error { return nil }
func (*resource) Stat() error { return nil }
var escapedResource *resource

func direct() error {
	value, err := open()
	if err != nil { return err }
	if err := value.Close(); err != nil { return err }
	_, err = value.Read(nil)
	return err
}

func bothBranches(closeLeft bool) error {
	value, err := open()
	if err != nil { return err }
	if closeLeft {
		_ = value.Close()
	} else {
		_ = value.Close()
	}
	_, err = value.Write(nil)
	return err
}

func closeAfterUnknown() error {
	value, err := open()
	if err != nil { return err }
	escapedResource = value
	_ = value.Close()
	return value.Sync()
}

func afterDeferredHelper() error {
	value, err := open()
	if err != nil { return err }
	closeDeferred(value)
	_, err = value.ReadByte()
	return err
}

func callbackOwned() error {
	var value *resource
	value, err := openWithCallback(func() { _ = value.Close() })
	if err != nil { return err }
	_ = value.Close()
	return value.Stat()
}

func closeDeferred(value *resource) { defer value.Close() }
`
	result := runResourceUsedAfterClose(t, input, contracts.Set{})
	want := []struct {
		operation string
		close string
	}{
		{operation: "value.Read(nil)", close: "value.Close()"},
		{operation: "value.Write(nil)", close: ""},
		{operation: "value.Sync()", close: "value.Close()"},
		{operation: "value.ReadByte()", close: "closeDeferred(value)"},
		{operation: "value.Stat()", close: "value.Close()"},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("resource-used-after-close result = %#v", result)
	}
	for index, expected := range want {
		diagnostic := result.Files[0].Diagnostics[index]
		start := nthIndex(input, expected.operation, 0)
		if diagnostic.RuleID != "resource-used-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(expected.operation) ||
			!strings.Contains(diagnostic.Message, "after it is closed") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		if expected.close == "" {
			if len(diagnostic.Related) != 0 {
				t.Fatalf("diagnostic[%d] related = %#v", index, diagnostic.Related)
			}
			continue
		}
		closeStart := strings.LastIndex(input[:start], expected.close)
		if len(diagnostic.Related) != 1 ||
			diagnostic.Related[0].Range.Start != closeStart ||
			diagnostic.Related[0].Range.End != closeStart + len(expected.close) {
			t.Fatalf("diagnostic[%d] related = %#v", index, diagnostic.Related)
		}
	}
}

func TestResourceUsedAfterCloseTracksCleanupManagedResultAcquisitions(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"os"
	"testing"
)

func open(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func delegated(t *testing.T) *os.File { return open(t) }

func use(t *testing.T) {
	file := delegated(t)
	_ = file.Close()
	_, _ = file.Read(nil)
}
`
	result := runResourceUsedAfterClose(t, input, contracts.Set{})
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("cleanup-managed resource-used-after-close result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	start := strings.Index(input, "file.Read(nil)")
	if diagnostic.RuleID != "resource-used-after-close" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len("file.Read(nil)") {
		t.Fatalf("cleanup-managed resource-used-after-close diagnostic = %#v", diagnostic)
	}
}

func TestResourceUsedAfterCloseRemainsConservative(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func (*resource) Reset() {}
func (*resource) Name() string { return "resource" }

func conditional(closeNow bool) error {
	value, err := open()
	if err != nil { return err }
	if closeNow { _ = value.Close() }
	_, err = value.Read(nil)
	return err
}

func deferred() error {
	value, err := open()
	if err != nil { return err }
	defer value.Close()
	_, err = value.Read(nil)
	return err
}

func asynchronous() error {
	value, err := open()
	if err != nil { return err }
	go value.Close()
	_, err = value.Read(nil)
	return err
}

func asynchronousHelper() error {
	value, err := open()
	if err != nil { return err }
	closeAsync(value)
	_, err = value.Read(nil)
	return err
}

func closeAsync(value *resource) { go value.Close() }

func mixedHelper(transfer bool) error {
	value, err := open()
	if err != nil { return err }
	_ = closeOrTransfer(value, transfer)
	_, err = value.Read(nil)
	return err
}

func closeOrTransfer(value *resource, transfer bool) *resource {
	if transfer {
		return value
	}
	_ = value.Close()
	return nil
}

func escaped() error {
	value, err := open()
	if err != nil { return err }
	consume(value)
	_, err = value.Read(nil)
	return err
}

func aliased() error {
	value, err := open()
	if err != nil { return err }
	alias := value
	_ = alias.Close()
	_, err = value.Read(nil)
	return err
}

func reset() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Close()
	value.Reset()
	_, err = value.Read(nil)
	return err
}

func resetHelper() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Close()
	reinitialize(value)
	_, err = value.Read(nil)
	return err
}

func reinitialize(value *resource) { value.Reset() }

func observed() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Close()
	_ = value.Name()
	_, err = value.Read(nil)
	return err
}

func reacquired() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Close()
	value, err = open()
	if err != nil { return err }
	_, err = value.Read(nil)
	return err
}

func methodValue() error {
	value, err := open()
	if err != nil { return err }
	closeValue := value.Close
	_ = closeValue
	_, err = value.Read(nil)
	return err
}

func consume(*resource) {}

type noErrorCloser struct{}
func openNoError() *noErrorCloser { return &noErrorCloser{} }
func (*noErrorCloser) Close() {}
func (*noErrorCloser) Read([]byte) (int, error) { return 0, nil }

func excludedCloseShape() error {
	value := openNoError()
	value.Close()
	_, err := value.Read(nil)
	return err
}
`
	result := runResourceUsedAfterClose(t, input, contracts.Set{})
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("conservative resource-used-after-close result = %#v", result)
	}
}

func TestResourceUsedAfterCloseConsumesProjectCloseContract(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func finish(*resource) {}

func run() error {
	value, err := open()
	if err != nil { return err }
	finish(value)
	_, err = value.Read(nil)
	return err
}
`
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/resourceuse.finish"
closes = [0]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := runResourceUsedAfterClose(t, input, set)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("contract resource-used-after-close result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	wantStart := strings.LastIndex(input, "value.Read(nil)")
	if diagnostic.RuleID != "resource-used-after-close" ||
		diagnostic.Range.Start != wantStart ||
		diagnostic.Range.End != wantStart + len("value.Read(nil)") {
		t.Fatalf("contract diagnostic = %#v", diagnostic)
	}
}

func TestResourceUsedAfterCloseConsumesGuaranteedReceiverEffects(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func (value *resource) Shutdown() error { return value.Close() }
func (value *resource) MaybeShutdown(closeNow bool) error {
	if closeNow { return value.Close() }
	return nil
}

func direct() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Shutdown()
	_, err = value.Read(nil)
	return err
}

func methodExpression() error {
	value, err := open()
	if err != nil { return err }
	_ = (*resource).Shutdown(value)
	_, err = value.Read(nil)
	return err
}

func conditional(closeNow bool) error {
	value, err := open()
	if err != nil { return err }
	_ = value.MaybeShutdown(closeNow)
	_, err = value.Read(nil)
	return err
}
`
	result := runResourceUsedAfterClose(t, input, contracts.Set{})
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("receiver effect resource-used-after-close result = %#v", result)
	}
	want := []string{"value.Read(nil)", "value.Read(nil)"}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relative := strings.Index(input[searchFrom:], want[index])
		if relative < 0 {
			t.Fatalf("missing receiver effect operation %d", index)
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "resource-used-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index]) ||
			len(diagnostic.Related) != 1 {
			t.Fatalf("receiver effect diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index])
	}
}

func TestResourceUsedAfterCloseExcludesSourceProvenNoOpClosers(t *testing.T) {
	t.Parallel()

	input := `package sample

type inertCloser struct { closeErr error }

func (closer *inertCloser) Close() error { return closer.closeErr }
func (*inertCloser) Read() {}
func openInertCloser() *inertCloser { return &inertCloser{} }

type statefulCloser struct { closed bool }

func (closer *statefulCloser) Close() error { closer.closed = true; return nil }
func (*statefulCloser) Read() {}
func openStatefulCloser() *statefulCloser { return &statefulCloser{} }

func inert() {
	closer := openInertCloser()
	_ = closer.Close()
	closer.Read()
}

func stateful() {
	closer := openStatefulCloser()
	_ = closer.Close()
	closer.Read()
}
`
	result := runResourceUsedAfterClose(t, input, contracts.Set{})
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("no-op closer use diagnostics = %#v", result)
	}
	start := strings.LastIndex(input, "closer.Read()")
	diagnostic := result.Files[0].Diagnostics[0]
	if diagnostic.Range.Start != start || diagnostic.Range.End != start + len("closer.Read()") {
		t.Fatalf("stateful closer diagnostic = %#v", diagnostic)
	}
}

func TestResourceUsedAfterCloseRequiresNurseryOrExplicitEnablement(t *testing.T) {
	t.Parallel()

	input := `package sample

type readCloser interface {
	Close() error
	Read([]byte) (int, error)
}

type contractReader struct { closed bool }

func open() readCloser { return &contractReader{} }
func (reader *contractReader) Close() error { reader.closed = true; return nil }
func (reader *contractReader) Read([]byte) (int, error) {
	if reader.closed { return 0, nil }
	return 1, nil
}

func verifyClosedState() {
	reader := open()
	_ = reader.Close()
	_, _ = reader.Read(nil)
}
`
	tests := []struct {
		name string
		configuration string
		wantDiagnostics int
	}{
		{name: "default", configuration: "version = 1\n"},
		{
			name: "recommended",
			configuration: "version = 1\n[lint]\nprofile = \"recommended\"\n",
		},
		{name: "strict", configuration: "version = 1\n[lint]\nprofile = \"strict\"\n"},
		{name: "pedantic", configuration: "version = 1\n[lint]\nprofile = \"pedantic\"\n"},
		{
			name: "nursery",
			configuration: "version = 1\n[lint]\npresets = [\"nursery\"]\n" +
				"[lint.rules]\nself-assignment = \"warn\"\n",
			wantDiagnostics: 1,
		},
		{
			name: "explicit",
			configuration: "version = 1\n[lint]\npresets = []\n" +
				"[lint.rules]\nresource-used-after-close = \"warn\"\n",
			wantDiagnostics: 1,
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				result := runResourceUsedAfterClosePolicy(
					t,
					input,
					test.configuration,
				)
				if len(result.Files) != 1 {
					t.Fatalf(
						"%s profile files = %d, want 1",
						test.name,
						len(result.Files),
					)
				}
				if got := len(result.Files[0].Diagnostics);
					got != test.wantDiagnostics {
					t.Fatalf(
						"%s profile diagnostics = %d, want %d",
						test.name,
						got,
						test.wantDiagnostics,
					)
				}
			},
		)
	}
}

func TestResourceUsedAfterCloseStopsTrackingAfterProjectTransfer(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func handoff(*resource) {}

func run() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Close()
	handoff(value)
	_, err = value.Read(nil)
	return err
}
`
	set, err := contracts.ParseFiles(
		[]contracts.File{
			{
				Path: "contracts.toml",
				Bytes: []byte(
					`version = 1
[[functions]]
symbol = "example.com/resourceuse.handoff"
takes-ownership = [0]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := runResourceUsedAfterClose(t, input, set)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("transferred resource-used-after-close result = %#v", result)
	}
}

func TestResourceUsedAfterCloseMetadataAndEligibility(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("resource-used-after-close")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetNursery}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		!metadata.RequiresEffectFacts ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf("resource-used-after-close metadata = %#v, found = %v", metadata, found)
	}

	suppressed := runResourceUsedAfterClose(
		t,
		`package sample
type resource struct{}
func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func run() error {
	value, err := open()
	if err != nil { return err }
	_ = value.Close()
	//glippy:ignore resource-used-after-close -- compatibility probe
	_, err = value.Read(nil)
	return err
}
`,
		contracts.Set{},
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed resource-used-after-close result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
type resource struct{}
func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func run() { value, _ := open(); _ = value.Close(); _, _ = value.Read(nil) }
`,
			"type-error": `package sample
type resource struct{}
func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }
func run() { value, _ := open(); missing(); _ = value.Close(); _, _ = value.Read(nil) }
`,
		} {
		result := runResourceUsedAfterClose(t, input, contracts.Set{})
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
			t.Fatalf("%s resource-used-after-close result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf("type-error result has no load diagnostics: %#v", result)
		}
	}

	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"resource-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 resource-used-after-close selection = %#v", older)
	}
}

func BenchmarkResourceUsedAfterClosePackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/resourceusebenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString(
		"package sample\ntype resource struct{}\nfunc open() (*resource, error) { return &resource{}, nil }\nfunc (*resource) Close() error { return nil }\nfunc (*resource) Read([]byte) (int, error) { return 0, nil }\n",
	)
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() error { value, err := open(); if err != nil { return err }; _ = value.Close(); _, err = value.Read(nil); return err }\n",
			index,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	benchmarkPackageRuns(
		b,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"resource-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		100,
	)
}

func runResourceUsedAfterClose(
	t testing.TB,
	input string,
	contractSet contracts.Set,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourceuse\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"resource-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
			Contracts: contractSet,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runResourceUsedAfterClosePolicy(
	t testing.TB,
	input string,
	configuration string,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourceusepolicy\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	configured, err := config.Parse(
		filepath.Join(root, ".glippy.toml"),
		[]byte(configuration),
		config.ParseOptions{
			KnownRules: registry.IDs(),
			RuleOptions: registry.OptionSchemas(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Profile: configured.Lint.Profile,
			ProfileRules: configured.Lint.ProfileRules,
			Presets: configured.Lint.Presets,
			Overrides: configured.Lint.Rules,
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestResourceUsedAfterCloseTracksInitializedLocalDeclarations(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (*resource) Read([]byte) (int, error) { return 0, nil }

func use() error {
	var value, err = open()
	if err != nil { return err }
	_ = value.Close()
	_, err = value.Read(nil)
	return err
}
`
	result := runResourceUsedAfterClose(t, input, contracts.Set{})
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("initialized declaration result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	operation := "value.Read(nil)"
	start := strings.Index(input, operation)
	if diagnostic.RuleID != "resource-used-after-close" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len(operation) {
		t.Fatalf("initialized declaration diagnostic = %#v", diagnostic)
	}
}
