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

func TestShadowedErrorReportsStaleErrorAfterLeavingLoop(t *testing.T) {
	t.Parallel()

	input := `package sample

type concreteError struct{}
func (*concreteError) Error() string { return "failure" }

func next() (int, error) { return 0, nil }
func nextConcrete() *concreteError { return nil }
func nextError() error { return nil }

func shortDeclaration() error {
	var err error
	for {
		_, err := next()
		if err != nil { break }
		break
	}
	return err
}

`
	result := runShadowedError(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("shadowed-error result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	needle := "_, err := next()"
	start := strings.Index(input, needle) + strings.Index(needle, "err")
	if diagnostic.RuleID != "shadowed-error" ||
		diagnostic.MessageKey != "shadowed-error" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len("err") ||
		len(diagnostic.Fixes) != 0 {
		t.Fatalf("shadowed-error diagnostic = %#v", diagnostic)
	}
}

func TestShadowedErrorExcludesNonErrorsInconsequentialAndNestedDeclarations(t *testing.T) {
	t.Parallel()

	input := `package sample

type concreteError struct{}
func (*concreteError) Error() string { return "failure" }

func next() (int, error) { return 0, nil }
func nextConcrete() *concreteError { return nil }
func nextError() error { return nil }

func nonError() int {
	value := 1
	if value > 0 { value := 2; _ = value }
	return value
}

func outerNotUsedAfterward() {
	var err error
	if true { _, err := next(); _ = err }
	_ = err
}

func differentErrorTypes() error {
	var err error
	if true { err := nextConcrete(); _ = err }
	return err
}

func sameTypeNestedDeclaration() error {
	var err error
	if true { err := nextError(); _ = err }
	return err
}

func uninitializedInnerError() error {
	var err error
	for {
		var err error
		if err != nil { break }
		break
	}
	return err
}

func handledLocally() error {
	_, err := next()
	if err != nil { return err }
	if err := nextError(); err != nil { return err }
	return err
}

func validationSequence() error {
	_, err := next()
	if err != nil { return err }
	if err := nextError(); err != nil { return err }
	_, err = next()
	return err
}

func outerReassignedAfterLoop() error {
	var err error
	for {
		_, err := next()
		if err != nil { break }
		break
	}
	err = nextError()
	return err
}

func switchBreakDoesNotLeaveLoop() error {
	var err error
	for {
		_, err := next()
		if err != nil {
			switch {
			default:
				break
			}
		}
		break
	}
	return err
}

type analyzer struct{}
func makeAnalyzer() (*analyzer, error) { return nil, nil }
func validateAnalyzer(*analyzer) error { return nil }
func adaptedAnalyzer() (*analyzer, error) {
	analyzer, err := makeAnalyzer()
	if err != nil { return nil, err }
	if err := validateAnalyzer(analyzer); err != nil { return nil, err }
	comparison, err := makeAnalyzer()
	if err != nil { return nil, err }
	return comparison, nil
}

func nestedFunction() error {
	var err error
	func() { _, err := next(); _ = err }()
	return err
}

func idiomaticRedeclaration(err error) error {
	if true { err := err; _ = err }
	return err
}
`
	result := runShadowedError(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("shadowed-error exclusions = %#v", result)
	}
}

func TestShadowedErrorReportsDeferredUpdateToShadowedNamedResult(t *testing.T) {
	t.Parallel()

	input := `package sample

type resource struct{}
func open() (*resource, error) { return &resource{}, nil }
func (*resource) Close() error { return nil }

func namedResult() (err error) {
	if true {
		resource, err := open()
		if err != nil { return err }
		defer func() {
			if closeErr := resource.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}()
	}
	return nil
}
`
	result := runShadowedError(t, input, nil)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("shadowed-error named result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	needle := "resource, err := open()"
	start := strings.Index(input, needle) + strings.Index(needle, "err")
	if diagnostic.Range.Start != start || diagnostic.Range.End != start + len("err") {
		t.Fatalf("shadowed-error named-result diagnostic = %#v", diagnostic)
	}
}

func TestShadowedErrorMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("shadowed-error")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeFile}) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!reflect.DeepEqual(
			metadata.Categories,
			[]rules.Category{rules.CategoryCorrectness, rules.CategorySuspicious},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("shadowed-error metadata = %#v, found = %v", metadata, found)
	}
}

func TestShadowedErrorHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/shadowederrorpolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

func suppressedNext() error { return nil }
func suppressed() error {
	var err error
	for {
		//glippy:ignore shadowed-error -- compatibility requires the inner operation
		_, err := 0, suppressedNext()
		if err != nil { break }
		break
	}
	return err
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

func generatedNext() error { return nil }
func generated() error {
	var err error
	for {
		_, err := 0, generatedNext()
		if err != nil { break }
		break
	}
	return err
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

func invalidNext() error { return nil }
func invalid() error {
	var err error
	for {
		_, err := 0, invalidNext()
		if err != nil { break }
		break
	}
	var broken string = 1
	_ = broken
	return err
}
`,
	)
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{"shadowed-error": rules.SeverityError},
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
		t.Fatalf("shadowed-error policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID != "shadowed-error" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed shadowed-error result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded shadowed-error result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
}

func runShadowedError(
	t *testing.T,
	input string,
	overrides map[string]rules.Severity,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/shadowederrorfixture\n\ngo 1.25.0\n",
	)
	writeFixture(t, filepath.Join(root, "sample.go"), input)
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if overrides == nil {
		overrides = map[string]rules.Severity{"shadowed-error": rules.SeverityWarn}
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
	if len(result.LoadDiagnostics) != 0 {
		t.Fatalf("shadowed-error fixture load diagnostics = %#v", result.LoadDiagnostics)
	}
	return result
}

func BenchmarkShadowedErrorPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/shadowederrorbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nfunc next() error { return nil }\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() error { var err error; for { _, err := 0, next(); if err != nil { break }; break }; return err }\n",
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
			Overrides: map[string]rules.Severity{"shadowed-error": rules.SeverityWarn},
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

func BenchmarkShadowedErrorLargeFunction(b *testing.B) {
	for _, loops := range []int{100, 1000} {
		b.Run(
			fmt.Sprintf("loops-%d", loops),
			func(b *testing.B) {
				root := b.TempDir()
				writeFixture(
					b,
					filepath.Join(root, "go.mod"),
					"module example.com/shadowederrorlarge\n\ngo 1.25.0\n",
				)
				var input strings.Builder
				input.WriteString(
					"package sample\nfunc next() error { return nil }\nfunc run() error {\nvar err error\n",
				)
				for range loops {
					input.WriteString(
						"for { _, err := 0, next(); if err != nil { break }; break }\n",
					)
				}
				input.WriteString("return err\n}\n")
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
							"shadowed-error": rules.SeverityWarn,
						},
						SourceGoVersion: "go1.25",
					},
					analysis.PackageLoadOptions{
						Dir: root,
						Patterns: []string{"."},
						ModuleMode: analysis.ModuleReadonly,
					},
					loops,
				)
			},
		)
	}
}
