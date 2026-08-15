package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestOverwrittenErrorReportsUnreadErrorValues(t *testing.T) {
	t.Parallel()

	input := `package sample

func first() (int, error) { return 1, nil }
func second() (int, error) { return 2, nil }
func observe(error) {}

func overwritten() {
	value, err := first()
	value, err = second()
	if err != nil { observe(err) }
	_ = value
}

func checked() {
	value, err := first()
	if err != nil { observe(err) }
	value, err = second()
	if err != nil { observe(err) }
	_ = value
}

func explicitlyIgnored() {
	value, err := first()
	_ = err
	value, err = second()
	if err != nil { observe(err) }
	_ = value
}

func explicitlyDeclaredIgnored() {
	value, err := first()
	var _ = err
	value, err = second()
	if err != nil { observe(err) }
	_ = value
}

func switchUse() {
	value, err := first()
	switch err { case nil: }
	value, err = second()
	if err != nil { observe(err) }
	_ = value
}

func assignedButNotOverwritten(err error) {
	err = func() error { return nil }()
}

func nestedFunction() {
	func() {
		value, nestedErr := first()
		value, nestedErr = second()
		if nestedErr != nil { observe(nestedErr) }
		_ = value
	}()
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/overwrittenerror\n\ngo 1.25.0\n",
	)
	path := filepath.Join(root, "sample.go")
	writeFixture(t, path, input)
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
				"overwritten-error": rules.SeverityWarn,
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
	if len(result.Files) != 1 ||
		result.Files[0].Path != path ||
		len(result.Files[0].Diagnostics) != 2 {
		t.Fatalf("overwritten-error result = %#v", result)
	}
	for index, target := range []string{"err := first()", "nestedErr := first()"} {
		diagnostic := result.Files[0].Diagnostics[index]
		wantStart := strings.Index(input, target)
		if wantStart < 0 {
			t.Fatalf("fixture does not contain %q", target)
		}
		identifier := strings.TrimSuffix(target, " := first()")
		if diagnostic.RuleID != "overwritten-error" ||
			diagnostic.Severity != rules.SeverityWarn ||
			diagnostic.Range.Start != wantStart ||
			diagnostic.Range.End != wantStart + len(identifier) ||
			diagnostic.MessageKey != "overwritten-error" ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("overwritten-error diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestOverwrittenErrorHandlesDeclarationsConcreteErrorsAndBranchJoins(t *testing.T) {
	t.Parallel()

	input := `package sample

type concreteError struct{}
func (*concreteError) Error() string { return "failure" }

type richError interface {
	error
	Code() int
}

func nextError() error { return nil }
func nextConcrete() *concreteError { return nil }
func nextRich() richError { return nil }
func observe(error) {}

func declaration() {
	var declaredErr error = nextError()
	declaredErr = nextError()
	observe(declaredErr)
}

func concrete() {
	concreteErr := nextConcrete()
	concreteErr = nextConcrete()
	observe(concreteErr)
}

func interfaceAssignment() {
	var interfaceErr error = nextConcrete()
	interfaceErr = nextConcrete()
	observe(interfaceErr)
}

func interfaceChange() {
	var richerErr error = nextRich()
	richerErr = nextRich()
	observe(richerErr)
}

func joined(condition bool) {
	joinedErr := nextError()
	if condition {
		joinedErr = nextError()
	}
	observe(joinedErr)
}

func overwrittenJoin(condition bool) {
	branchErr := nextError()
	if condition {
		branchErr = nextError()
	}
	branchErr = nextError()
	observe(branchErr)
}

func closureUse() {
	closureErr := nextError()
	func() { observe(closureErr) }()
	closureErr = nextError()
	observe(closureErr)
}
`
	result := runOverwrittenError(t, input, nil)
	assertRuleRanges(
		t,
		input,
		result,
		"overwritten-error",
		"overwritten-error",
		[]string{
			"declaredErr",
			"concreteErr",
			"interfaceErr",
			"richerErr",
			"branchErr",
			"branchErr",
		},
	)
}

func TestOverwrittenErrorMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("overwritten-error")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSSA ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!reflect.DeepEqual(
			metadata.Categories,
			[]rules.Category{rules.CategoryCorrectness, rules.CategorySuspicious},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("overwritten-error metadata = %#v, found = %v", metadata, found)
	}
}

func TestOverwrittenErrorHonorsSharedPoliciesAndSourceVersions(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/overwrittenerrorpolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

func suppressedFirst() error { return nil }
func suppressedSecond() error { return nil }

func suppressed() {
	//glippy:ignore overwritten-error -- the first failure is deliberately superseded
	err := suppressedFirst()
	err = suppressedSecond()
	_ = err
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

func generatedFirst() error { return nil }
func generatedSecond() error { return nil }

func generated() {
	err := generatedFirst()
	err = generatedSecond()
	_ = err
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

func first() error { return nil }
func second() error { return nil }

func invalid() {
	err := first()
	err = second()
	_ = err
	var broken string = 1
	_ = broken
}
`,
	)
	overrides := map[string]rules.Severity{"overwritten-error": rules.SeverityError}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: "go1.25",
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"./..."},
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 3 || len(result.LoadDiagnostics) == 0 {
		t.Fatalf("overwritten-error policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "overwritten-error" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed overwritten-error result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded overwritten-error result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection) != 0 {
		t.Fatalf("pre-minimum overwritten-error selection = %#v", selection)
	}
}

func runOverwrittenError(
	t *testing.T,
	input string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/overwrittenerrorfixture\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if overrides == nil {
		overrides = map[string]rules.Severity{"overwritten-error": rules.SeverityWarn}
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: overrides,
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

func BenchmarkOverwrittenErrorPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/overwrittenerrorbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString(
		"package sample\nfunc first() error { return nil }\nfunc second() error { return nil }\n",
	)
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() { err := first(); err = second(); _ = err }\n",
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
				"overwritten-error": rules.SeverityWarn,
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

func BenchmarkOverwrittenErrorLargeFunction(b *testing.B) {
	for _, assignments := range []int{100, 1000} {
		b.Run(
			fmt.Sprintf("assignments-%d", assignments),
			func(b *testing.B) {
				root := b.TempDir()
				writeFixture(
					b,
					filepath.Join(root, "go.mod"),
					"module example.com/overwrittenerrorlarge\n\ngo 1.25.0\n",
				)
				var input strings.Builder
				input.WriteString(
					"package sample\nfunc next() error { return nil }\nfunc run() {\nerr := next()\n",
				)
				for range assignments - 1 {
					input.WriteString("err = next()\n")
				}
				input.WriteString("_ = err\n}\n")
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
							"overwritten-error": rules.SeverityWarn,
						},
						SourceGoVersion: "go1.25",
					},
					analysis.PackageLoadOptions{
						Dir: root,
						Patterns: []string{"."},
						ModuleMode: analysis.ModuleReadonly,
					},
					assignments - 1,
				)
			},
		)
	}
}
