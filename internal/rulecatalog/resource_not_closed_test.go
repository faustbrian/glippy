package rulecatalog_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestResourceNotClosedReportsUnreleasedLocalClosers(t *testing.T) {
	t.Parallel()

	input := `package sample

import "os"

type customCommand struct{}

func (*customCommand) StdoutPipe() (*os.File, error) {
	return os.Open("input")
}

func bad() error {
	file, err := os.Open("input")
	if err != nil { return err }
	_ = file.Name()
	return nil
}

func badFallthrough() {
	file, _ := os.Open("input")
	_ = file.Name()
}

func badCustomPipe(command *customCommand) error {
	file, err := command.StdoutPipe()
	if err != nil { return err }
	_ = file.Name()
	return nil
}

func goodDefer() error {
	file, err := os.Open("input")
	if err != nil { return err }
	defer file.Close()
	return nil
}

func goodExplicit() error {
	file, err := os.Open("input")
	if err != nil { return err }
	return file.Close()
}

func partialClose(closeFile bool) error {
	file, err := os.Open("input")
	if err != nil { return err }
	if closeFile { return file.Close() }
	return nil
}

func completedBranches(closeFile bool) error {
	file, err := os.Open("input")
	if err != nil { return err }
	if closeFile { return file.Close() }
	consume(file)
	return nil
}

func overwritten() error {
	file, err := os.Open("input")
	if err != nil { return err }
	file, err = os.Open("replacement")
	if err != nil { return err }
	defer file.Close()
	return nil
}

func transfer() (*os.File, error) {
	file, err := os.Open("input")
	if err != nil { return nil, err }
	return file, nil
}

func pass() error {
	file, err := os.Open("input")
	if err != nil { return err }
	consume(file)
	return nil
}

func nilResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file == nil { return nil }
	return file.Close()
}

func reversedNilResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if nil == file { return nil }
	return file.Close()
}

func nilResultUnreleased() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file == nil { return nil }
	_ = file.Name()
	return nil
}

func nonNilResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file != nil { return file.Close() }
	return nil
}

func explicitElseResult() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file != nil { return file.Close() } else { return nil }
}

func nonNilResultUnreleased() error {
	file, err := os.Open("input")
	if err != nil { return err }
	if file != nil { _ = file.Name() }
	return nil
}

func consume(*os.File) {}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourcenotclosed\n\ngo 1.25.0\n",
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
				"resource-not-closed": rules.SeverityWarn,
			},
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 9 {
		t.Fatalf("resource-not-closed result = %#v", result)
	}
	expected := []struct {
		function string
		acquisition string
	}{
		{function: "func bad()", acquisition: "file, err := os.Open"},
		{function: "func badFallthrough()", acquisition: "file, _ := os.Open"},
		{function: "func badCustomPipe(", acquisition: "file, err := command.StdoutPipe"},
		{function: "func partialClose(", acquisition: "file, err := os.Open"},
		{function: "func completedBranches(", acquisition: "file, err := os.Open"},
		{function: "func overwritten()", acquisition: "file, err := os.Open"},
		{function: "func pass()", acquisition: "file, err := os.Open"},
		{function: "func nilResultUnreleased()", acquisition: "file, err := os.Open"},
		{function: "func nonNilResultUnreleased()", acquisition: "file, err := os.Open"},
	}
	expectedStarts := make(map[int]bool, len(expected))
	for index, location := range expected {
		functionStart := strings.Index(input, location.function)
		if functionStart < 0 {
			t.Fatalf("missing function %d", index)
		}
		relative := strings.Index(input[functionStart:], location.acquisition)
		if relative < 0 {
			t.Fatalf("missing acquisition %d", index)
		}
		expectedStarts[functionStart + relative] = true
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		start := diagnostic.Range.Start
		if diagnostic.RuleID != "resource-not-closed" ||
			diagnostic.Range.End != start + len("file") ||
			!expectedStarts[start] ||
			!strings.Contains(diagnostic.Message, "not closed") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("resource-not-closed diagnostic %d = %#v", index, diagnostic)
		}
		delete(expectedStarts, start)
	}
	if len(expectedStarts) != 0 {
		t.Fatalf("missing resource-not-closed ranges = %#v", expectedStarts)
	}
}

func TestResourceNotClosedAcceptsDogfoodOwnershipPatterns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourceownership\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"io"
	"os"
	"os/exec"
	"testing"
)

func methodDefer(root *os.Root) error {
	file, err := root.Open("input")
	if os.IsNotExist(err) { return err }
	if err != nil { return err }
	defer file.Close()
	_, err = file.Stat()
	return err
}

func cleanupCapture(t *testing.T) {
	store, err := os.OpenRoot(".")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = store.Close() })
}

func rejectedConstruction(t *testing.T) {
	store, err := os.OpenRoot("missing")
	if store != nil || err == nil { t.Fatal("constructor unexpectedly succeeded") }
}

func returnSuccessfulLoop(root *os.Root) (string, *os.File, error) {
	for range 10 {
		file, err := root.Open("input")
		if err == nil { return "input", file, nil }
		if !os.IsNotExist(err) { return "", nil, err }
	}
	return "", nil, os.ErrNotExist
}

func pipeTransfer() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	if err := consume(output); err != nil { _ = output.Close(); return err }
	return command.Wait()
}

func commandOwnedPipe() error {
	command := exec.Command("true")
	output, err := command.StdoutPipe()
	if err != nil { return err }
	if err := command.Start(); err != nil { return err }
	extractErr := consume(output)
	if extractErr != nil { _ = output.Close() }
	waitErr := command.Wait()
	if extractErr != nil { return extractErr }
	return waitErr
}

func consume(io.Reader) error { return nil }
`,
	)
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("resource-not-closed dogfood patterns = %#v", result)
	}
}

func TestResourceNotClosedUsesGuaranteedCleanupManagedResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/cleanupmanaged\n\ngo 1.26.0\n",
	)
	input := `package sample

import (
	"os"
	"testing"
)

func directManaged(t *testing.T) *os.File {
	file, _ := os.Open("input")
	_ = file.Name()
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func helperManaged(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { closeFile(file) })
	return file
}

func closeFile(file *os.File) { _ = file.Close() }

func observationOnly(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Name() })
	return file
}

func conditionallyManaged(t *testing.T, closeFile bool) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() {
		if closeFile { _ = file.Close() }
	})
	return file
}

func conditionallyRegistered(t *testing.T, register bool) *os.File {
	file, _ := os.Open("input")
	if register { t.Cleanup(func() { _ = file.Close() }) }
	return file
}

func asynchronouslyManaged(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { go file.Close() })
	return file
}

func nestedOnly(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = func() error { return file.Close() } })
	return file
}

func replacedAfterRegistration(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Close() })
	file, _ = os.Open("replacement")
	return file
}

func aliasedBeforeCleanup(t *testing.T) *os.File {
	file, _ := os.Open("input")
	alias := file
	_ = alias
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func replacedDuringCleanup(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() {
		file, _ = os.Open("replacement")
		_ = file.Close()
	})
	return file
}

func replaceFile(file **os.File) { *file, _ = os.Open("replacement") }

func escapedDuringCleanup(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() {
		replaceFile(&file)
		_ = file.Close()
	})
	return file
}

func copiedTestHandle(t testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func use(t *testing.T) {
	direct := directManaged(t)
	_ = direct.Name()
	helper := helperManaged(t)
	_ = helper.Name()
	observed := observationOnly(t)
	_ = observed.Name()
	conditional := conditionallyManaged(t, false)
	_ = conditional.Name()
	registered := conditionallyRegistered(t, false)
	_ = registered.Name()
	asynchronous := asynchronouslyManaged(t)
	_ = asynchronous.Name()
	nested := nestedOnly(t)
	_ = nested.Name()
	replaced := replacedAfterRegistration(t)
	_ = replaced.Name()
	aliased := aliasedBeforeCleanup(t)
	_ = aliased.Name()
	callbackReplaced := replacedDuringCleanup(t)
	_ = callbackReplaced.Name()
	callbackEscaped := escapedDuringCleanup(t)
	_ = callbackEscaped.Name()
	copied := copiedTestHandle(*t)
	_ = copied.Name()
}
`
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 10 {
		t.Fatalf("cleanup-managed result diagnostics = %#v", result)
	}
	want := map[int]string{
		strings.Index(input, "observed := observationOnly"): "observed",
		strings.Index(input, "conditional := conditionallyManaged"): "conditional",
		strings.Index(input, "registered := conditionallyRegistered"): "registered",
		strings.Index(input, "asynchronous := asynchronouslyManaged"): "asynchronous",
		strings.Index(input, "nested := nestedOnly"): "nested",
		strings.Index(input, "replaced := replacedAfterRegistration"): "replaced",
		strings.Index(input, "aliased := aliasedBeforeCleanup"): "aliased",
		strings.Index(
			input,
			"callbackReplaced := replacedDuringCleanup",
		): "callbackReplaced",
		strings.Index(input, "callbackEscaped := escapedDuringCleanup"): "callbackEscaped",
		strings.Index(input, "copied := copiedTestHandle"): "copied",
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		name, found := want[diagnostic.Range.Start]
		if !found || diagnostic.Range.End != diagnostic.Range.Start + len(name) {
			t.Fatalf("cleanup-managed result diagnostic = %#v", diagnostic)
		}
		delete(want, diagnostic.Range.Start)
	}
	if len(want) != 0 {
		t.Fatalf("missing cleanup-managed result diagnostics = %#v", want)
	}
}

func TestResourceNotClosedUsesImportedCleanupManagedResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/importedcleanup\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "helper", "helper.go"),
		`package helper

import (
	"os"
	"testing"
)

func Open(t *testing.T) *os.File {
	file, _ := os.Open("input")
	t.Cleanup(func() { closeFile(file) })
	return file
}

func closeFile(file *os.File) { _ = file.Close() }

type Resource struct{}

func (*Resource) Close() error { return nil }

func (resource *Resource) Shutdown() error { return resource.Close() }

func OpenViaReceiver(t *testing.T) *Resource {
	resource := &Resource{}
	t.Cleanup(func() { _ = resource.Shutdown() })
	return resource
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"testing"

	"example.com/importedcleanup/helper"
)

func use(t *testing.T) {
	file := helper.Open(t)
	_ = file.Name()
	resource := helper.OpenViaReceiver(t)
	_ = resource
}
`,
	)
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("imported cleanup-managed result diagnostics = %#v", result)
	}
}

func TestResourceNotClosedRequiresGuaranteedReceiverCleanupEffects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/receivereffects\n\ngo 1.26.0\n",
	)
	input := `package sample

import (
	"context"
	"testing"
)

type resource struct{}

func (*resource) Close() error { return nil }

type innerResource struct{}

func (*innerResource) Close() error { return nil }

func (value *innerResource) Shutdown(context.Context) error { return value.Close() }

type promotedResource struct{ *innerResource }

func (*promotedResource) Close() error { return nil }

func (value *resource) Shutdown(context.Context) error { return value.Close() }

func (value *resource) DelegatedShutdown(ctx context.Context) error {
	return value.Shutdown(ctx)
}

func closeThroughReceiver(value *resource) {
	_ = value.Shutdown(context.Background())
}

func (value *resource) ConditionalShutdown(closeNow bool) {
	if closeNow { _ = value.Close() }
}

func (value *resource) AsynchronousShutdown() { go value.Close() }

func (value *resource) AliasedShutdown() {
	alias := value
	_ = alias.Close()
}

func (value *resource) ReassignedShutdown() {
	value = &resource{}
	_ = value.Close()
}

func replaceResource(value **resource) { *value = &resource{} }

func (value *resource) EscapedShutdown() {
	replaceResource(&value)
	_ = value.Close()
}

func (*resource) Observe() {}

func (value *resource) EarlyReturnShutdown(skip bool) {
	if skip { return }
	_ = value.Close()
}

type dynamicResource interface {
	Close() error
	Shutdown(context.Context) error
}

func directManaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { _ = value.Shutdown(context.Background()) })
	return value
}

func delegatedManaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { _ = value.DelegatedShutdown(context.Background()) })
	return value
}

func methodExpressionManaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { _ = (*resource).Shutdown(value, context.Background()) })
	return value
}

func parameterDelegatedManaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { closeThroughReceiver(value) })
	return value
}

func conditionalUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.ConditionalShutdown(false) })
	return value
}

func asynchronousUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.AsynchronousShutdown() })
	return value
}

func aliasedUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.AliasedShutdown() })
	return value
}

func reassignedUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.ReassignedShutdown() })
	return value
}

func escapedUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.EscapedShutdown() })
	return value
}

func observedUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.Observe() })
	return value
}

func earlyReturnUnmanaged(t *testing.T) *resource {
	value := &resource{}
	t.Cleanup(func() { value.EarlyReturnShutdown(true) })
	return value
}

func dynamicUnmanaged(t *testing.T) dynamicResource {
	var value dynamicResource = &resource{}
	t.Cleanup(func() { _ = value.Shutdown(context.Background()) })
	return value
}

func promotedUnmanaged(t *testing.T) *promotedResource {
	value := &promotedResource{innerResource: &innerResource{}}
	t.Cleanup(func() { _ = value.Shutdown(context.Background()) })
	return value
}

func use(t *testing.T) {
	direct := directManaged(t)
	_ = direct
	delegated := delegatedManaged(t)
	_ = delegated
	methodExpression := methodExpressionManaged(t)
	_ = methodExpression
	parameterDelegated := parameterDelegatedManaged(t)
	_ = parameterDelegated
	conditional := conditionalUnmanaged(t)
	_ = conditional
	asynchronous := asynchronousUnmanaged(t)
	_ = asynchronous
	aliased := aliasedUnmanaged(t)
	_ = aliased
	reassigned := reassignedUnmanaged(t)
	_ = reassigned
	escaped := escapedUnmanaged(t)
	_ = escaped
	observed := observedUnmanaged(t)
	_ = observed
	earlyReturn := earlyReturnUnmanaged(t)
	_ = earlyReturn
	dynamic := dynamicUnmanaged(t)
	_ = dynamic
	promoted := promotedUnmanaged(t)
	_ = promoted
}
`
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 9 {
		t.Fatalf("receiver cleanup diagnostics = %#v", result)
	}
	want := map[int]string{
		strings.Index(input, "conditional := conditionalUnmanaged"): "conditional",
		strings.Index(input, "asynchronous := asynchronousUnmanaged"): "asynchronous",
		strings.Index(input, "aliased := aliasedUnmanaged"): "aliased",
		strings.Index(input, "reassigned := reassignedUnmanaged"): "reassigned",
		strings.Index(input, "escaped := escapedUnmanaged"): "escaped",
		strings.Index(input, "observed := observedUnmanaged"): "observed",
		strings.Index(input, "earlyReturn := earlyReturnUnmanaged"): "earlyReturn",
		strings.Index(input, "dynamic := dynamicUnmanaged"): "dynamic",
		strings.Index(input, "promoted := promotedUnmanaged"): "promoted",
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		name, found := want[diagnostic.Range.Start]
		if !found || diagnostic.Range.End != diagnostic.Range.Start + len(name) {
			t.Fatalf("receiver cleanup diagnostic = %#v", diagnostic)
		}
		delete(want, diagnostic.Range.Start)
	}
	if len(want) != 0 {
		t.Fatalf("missing receiver cleanup diagnostics = %#v", want)
	}
}

func TestResourceNotClosedUsesGuaranteedDirectReceiverEffects(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}

func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }
func (value *resource) Shutdown() error { return value.Close() }
func (value *resource) MaybeShutdown(closeNow bool) error {
	if closeNow { return value.Close() }
	return nil
}

type dynamicResource interface {
	Close() error
	Shutdown() error
}

type innerResource struct{}
func (*innerResource) Shutdown() error { return nil }
type promotedResource struct{ *innerResource }
func (*promotedResource) Close() error { return nil }
func openPromoted() (*promotedResource, error) {
	return &promotedResource{innerResource: &innerResource{}}, nil
}

func direct() error {
	value, err := open()
	if err != nil { return err }
	return value.Shutdown()
}

func methodExpression() error {
	value, err := open()
	if err != nil { return err }
	return (*resource).Shutdown(value)
}

func conditional(closeNow bool) error {
	value, err := open()
	if err != nil { return err }
	return value.MaybeShutdown(closeNow)
}

func dynamic() error {
	var value dynamicResource
	var err error
	value, err = open()
	if err != nil { return err }
	return value.Shutdown()
}

func promoted() error {
	value, err := openPromoted()
	if err != nil { return err }
	return value.Shutdown()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/directreceivereffects\n\ngo 1.26.0\n",
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 3 {
		t.Fatalf("direct receiver effect diagnostics = %#v", result)
	}
	want := map[int]string{
		strings.Index(
			input,
			"value, err := open()\n\tif err != nil { return err }\n\treturn value.MaybeShutdown",
		): "value",
		strings.Index(
			input,
			"value, err = open()\n\tif err != nil { return err }\n\treturn value.Shutdown",
		): "value",
		strings.Index(input, "value, err := openPromoted()"): "value",
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		name, found := want[diagnostic.Range.Start]
		if !found || diagnostic.Range.End != diagnostic.Range.Start + len(name) {
			t.Fatalf("direct receiver effect diagnostic = %#v", diagnostic)
		}
		delete(want, diagnostic.Range.Start)
	}
	if len(want) != 0 {
		t.Fatalf("missing direct receiver effect diagnostics = %#v", want)
	}
}

func TestResourceNotClosedUsesReachableWorkspaceModuleReceiverEffects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	app := filepath.Join(root, "app")
	helper := filepath.Join(root, "helper")
	writeFixture(
		t,
		filepath.Join(root, "go.work"),
		"go 1.26.0\n\nuse (\n\t./app\n\t./helper\n)\n",
	)
	writeFixture(
		t,
		filepath.Join(app, "go.mod"),
		"module example.com/app\n\ngo 1.26.0\n\nrequire example.com/helper v0.0.0\n",
	)
	writeFixture(t, filepath.Join(helper, "go.mod"), "module example.com/helper\n\ngo 1.26.0\n")
	writeFixture(
		t,
		filepath.Join(helper, "helper.go"),
		`package helper

type Resource struct{}

func (*Resource) Close() error { return nil }

func (resource *Resource) Shutdown() error { return resource.Close() }
`,
	)
	writeFixture(
		t,
		filepath.Join(app, "sample.go"),
		`package app

import (
	"testing"

	"example.com/helper"
)

func open(t *testing.T) *helper.Resource {
	resource := &helper.Resource{}
	t.Cleanup(func() { _ = resource.Shutdown() })
	return resource
}

func use(t *testing.T) {
	resource := open(t)
	_ = resource
}
`,
	)
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
		},
		analysis.PackageLoadOptions{
			Dir: app,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("workspace receiver cleanup diagnostics = %#v", result)
	}
}

func TestResourceNotClosedMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("resource-not-closed")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		len(metadata.NodeInterests) != 0 ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("resource-not-closed metadata = %#v, found = %v", metadata, found)
	}
}

func TestResourceNotClosedDelegatesSpecializedWriterLifecycles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourcewriterownership\n\ngo 1.26.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"mime/multipart"
)

func archive(output io.Writer, input fs.FS) error {
	writer := tar.NewWriter(output)
	if err := writer.AddFS(input); err != nil {
		return err
	}
	return nil
}

func encode(output io.Writer) error {
	writer := gzip.NewWriter(output)
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func encodeLevel(output io.Writer) error {
	writer, err := gzip.NewWriterLevel(output, gzip.BestSpeed)
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte("payload")); err != nil {
		return err
	}
	return nil
}

func validateBoundary(output io.Writer) error {
	writer := multipart.NewWriter(output)
	return writer.SetBoundary("fixture-boundary")
}
`,
	)
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("generic writer lifecycle diagnostics = %#v", result)
	}
}

func TestResourceNotClosedTransfersConstructorCallbackCaptures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/resourceconstructorcapture\n\ngo 1.26.0\n",
	)
	input := `package sample

type resource struct{}
type callbacks struct{ run func() }

func (*resource) Close() error { return nil }
func open(any) (*resource, error) { return &resource{}, nil }

func captured() error {
	var value *resource
	value, err := open(func() { _ = value.Close() })
	if err != nil {
		return err
	}
	return nil
}

func unrelatedCapture() error {
	other := &resource{}
	value, err := open(func() { _ = other.Close() })
	if err != nil {
		return err
	}
	_ = value
	return nil
}

func indirectCapture() error {
	var value *resource
	hooks := callbacks{run: func() { _ = value.Close() }}
	value, err := open(hooks)
	if err != nil {
		return err
	}
	return nil
}

func replacedCapture() error {
	var value *resource
	hooks := callbacks{run: func() { _ = value.Close() }}
	hooks = callbacks{}
	value, err := open(hooks)
	if err != nil {
		return err
	}
	_ = value
	return nil
}

func conditionalCapture(enabled bool) error {
	var value *resource
	hooks := callbacks{}
	if enabled {
		hooks = callbacks{run: func() { _ = value.Close() }}
	}
	value, err := open(hooks)
	if err != nil {
		return err
	}
	_ = value
	return nil
}
`
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.26",
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 3 {
		t.Fatalf("constructor callback capture diagnostics = %#v", result)
	}
	wantOffsets := make(map[int]struct{})
	for _, function := range
		[]string{"unrelatedCapture", "replacedCapture", "conditionalCapture"} {
		functionOffset := strings.Index(input, "func " + function)
		if functionOffset < 0 {
			t.Fatalf("missing fixture function %q", function)
		}
		valueOffset := strings.Index(input[functionOffset:], "value, err := open")
		if valueOffset < 0 {
			t.Fatalf("missing acquisition in fixture function %q", function)
		}
		wantOffsets[functionOffset + valueOffset] = struct{}{}
	}
	for _, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "resource-not-closed" {
			t.Fatalf("constructor callback diagnostic = %#v", diagnostic)
		}
		if _, expected := wantOffsets[diagnostic.Range.Start]; !expected {
			t.Fatalf("unexpected constructor callback diagnostic = %#v", diagnostic)
		}
		delete(wantOffsets, diagnostic.Range.Start)
	}
	if len(wantOffsets) != 0 {
		t.Fatalf("missing constructor callback diagnostic offsets = %#v", wantOffsets)
	}
}

func BenchmarkResourceNotClosedPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/resourcenotclosedbenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nimport \"os\"\nfunc run() error { file, err := os.Open(\"input\"); if err != nil { return err }; _ = file.Name(); return nil }\n",
	)
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
				"resource-not-closed": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			ModuleMode: analysis.ModuleReadonly,
		},
		1,
	)
}
