package rulecatalog_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestExportedAPIDocumentationReportsMissingAndMalformedComments(t *testing.T) {
	t.Parallel()

	input := `package sample

// Documented performs documented work.
func Documented() {}

// performs work.
func Wrong() {}

func Missing() {}

// describes a type.
type WrongType struct{}

type hidden struct {
	MissingHiddenField int
}

func (hidden) MissingMethod() {}

// A Widget represents configurable behavior.
type Widget struct {
	// Name identifies the widget.
	Name string
	MissingField int
	// contains an optional value.
	WrongField bool
	TrailingField string // TrailingField is documented on the same line.
	FirstField, SecondField int // Paired fields share documentation.
	hidden int
}

// Service executes work.
type Service interface {
	// Run executes one operation.
	Run()
	MissingMember()
	// executes another operation.
	WrongMember()
	TrailingMember() // TrailingMember is documented on the same line.
	hidden()
}

// Domain types share one grouped contract.
type (
	GroupedA struct{}
	GroupedB struct{}
)

// State values share one grouped contract.
const (
	GroupedOne = 1
	GroupedTwo = 2
)

// indicates a state.
const WrongConstant = 0

const (
	// Ready indicates readiness.
	Ready = 1
	MissingValue = 2
)

// Defaults are paired values.
var First, Second = 1, 2

// stores a value.
var WrongVariable = 1

var MissingVariable = 1

//go:nosplit
func DirectiveOnly() {}
`
	result := runExportedAPIDocumentation(t, "sample.go", input, nil)
	want := []struct {
		name string
		key string
		related int
	}{
		{name: "Wrong", key: "noncanonical-function-documentation", related: 1},
		{name: "Missing", key: "missing-function-documentation"},
		{name: "WrongType", key: "noncanonical-type-documentation", related: 1},
		{name: "MissingHiddenField", key: "missing-field-documentation"},
		{name: "MissingMethod", key: "missing-method-documentation"},
		{name: "MissingField", key: "missing-field-documentation"},
		{name: "WrongField", key: "noncanonical-field-documentation", related: 1},
		{name: "MissingMember", key: "missing-interface-method-documentation"},
		{
			name: "WrongMember",
			key: "noncanonical-interface-method-documentation",
			related: 1,
		},
		{name: "WrongConstant", key: "noncanonical-constant-documentation", related: 1},
		{name: "MissingValue", key: "missing-constant-documentation"},
		{name: "WrongVariable", key: "noncanonical-variable-documentation", related: 1},
		{name: "MissingVariable", key: "missing-variable-documentation"},
		{name: "DirectiveOnly", key: "missing-function-documentation"},
	}
	if len(result.Diagnostics) != len(want) {
		t.Fatalf("exported-api-documentation diagnostics = %#v", result.Diagnostics)
	}
	for index, expected := range want {
		diagnostic := result.Diagnostics[index]
		if diagnostic.RuleID != "exported-api-documentation" ||
			diagnostic.MessageKey != expected.key ||
			string(input[diagnostic.Range.Start:diagnostic.Range.End]) !=
				expected.name ||
			len(diagnostic.Related) != expected.related ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestExportedAPIDocumentationOptionsNarrowPolicy(t *testing.T) {
	t.Parallel()

	input := `package sample

// does work.
func Wrong() {}

func Missing() {}

type hidden struct{}
func (hidden) MissingMethod() {}

// Container stores values.
type Container struct {
	MissingField int
	// stores another value.
	WrongField int
}

// Service executes work.
type Service interface {
	MissingMember()
	// performs work.
	WrongMember()
}
`
	withoutMembers := runExportedAPIDocumentation(
		t,
		"sample.go",
		input,
		map[string]bool{"include-members": false},
	)
	if got := diagnosticNames(input, withoutMembers);
		!reflect.DeepEqual(got, []string{"Wrong", "Missing", "MissingMethod"}) {
		t.Fatalf("without members = %#v", got)
	}
	withoutNamePolicy := runExportedAPIDocumentation(
		t,
		"sample.go",
		input,
		map[string]bool{"require-name-prefix": false},
	)
	if got := diagnosticNames(input, withoutNamePolicy);
		!reflect.DeepEqual(
			got,
			[]string{"Missing", "MissingMethod", "MissingField", "MissingMember"},
		) {
		t.Fatalf("without name-prefix policy = %#v", got)
	}
}

func TestExportedAPIDocumentationHonorsFileAndSuppressionPolicy(t *testing.T) {
	t.Parallel()

	suppressed := runExportedAPIDocumentation(
		t,
		"sample.go",
		`package sample

//glippy:ignore exported-api-documentation -- compatibility symbol is self-evident
func Legacy() {}
`,
		nil,
	)
	if len(suppressed.Diagnostics) != 0 || len(suppressed.Suppressed) != 1 {
		t.Fatalf("suppressed exported API result = %#v", suppressed)
	}

	testFile := `package sample
type MissingTestAPI struct{}
`
	withoutTests := runExportedAPIDocumentation(t, "sample_test.go", testFile, nil)
	if len(withoutTests.Diagnostics) != 0 {
		t.Fatalf("default test-file result = %#v", withoutTests)
	}
	withTests := runExportedAPIDocumentation(
		t,
		"sample_test.go",
		testFile,
		map[string]bool{"include-tests": true},
	)
	if got := diagnosticNames(testFile, withTests);
		!reflect.DeepEqual(got, []string{"MissingTestAPI"}) {
		t.Fatalf("included test-file result = %#v", got)
	}

	mainFile := `package main
func ExportedCommandHelper() {}
`
	withoutMain := runExportedAPIDocumentation(t, "main.go", mainFile, nil)
	if len(withoutMain.Diagnostics) != 0 {
		t.Fatalf("default main-package result = %#v", withoutMain)
	}
	withMain := runExportedAPIDocumentation(
		t,
		"main.go",
		mainFile,
		map[string]bool{"include-main": true},
	)
	if got := diagnosticNames(mainFile, withMain);
		!reflect.DeepEqual(got, []string{"ExportedCommandHelper"}) {
		t.Fatalf("included main-package result = %#v", got)
	}

	generated := runExportedAPIDocumentation(
		t,
		"generated.go",
		"// Code generated by fixture. DO NOT EDIT.\npackage sample\ntype MissingGeneratedAPI struct{}\n",
		nil,
	)
	if len(generated.Diagnostics) != 0 {
		t.Fatalf("generated-file result = %#v", generated)
	}
}

func TestExportedAPIDocumentationMetadataSupportsRestrictionAndExactOptIn(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("exported-api-documentation")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetRestriction}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSyntax ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeFile}) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("exported-api-documentation metadata = %#v, found = %v", metadata, found)
	}
	optionNames := make([]string, 0, len(metadata.Options))
	for _, option := range metadata.Options {
		optionNames = append(optionNames, option.Name)
	}
	if !reflect.DeepEqual(
		optionNames,
		[]string{"include-tests", "include-main", "include-members", "require-name-prefix"},
	) {
		t.Fatalf("exported-api-documentation options = %#v", metadata.Options)
	}
	selection, resolveErr := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"exported-api-documentation": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if resolveErr != nil || len(selection) != 0 {
		t.Fatalf("old-version selection = %#v, err = %v", selection, resolveErr)
	}
}

func BenchmarkExportedAPIDocumentationSyntax(b *testing.B) {
	file, err := source.Load(
		"sample.go",
		[]byte("package sample\ntype Missing struct { Field int }\nfunc Exported() {}\n"),
	)
	if err != nil {
		b.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, runErr := analysis.Run(
			context.Background(),
			file,
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{
					"exported-api-documentation": rules.SeverityWarn,
				},
				SourceGoVersion: "go1.26",
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Diagnostics) != 3 {
			b.Fatalf("benchmark result = %#v", result)
		}
	}
}

func runExportedAPIDocumentation(
	t testing.TB,
	path string,
	input string,
	options map[string]bool,
) analysis.Result {
	t.Helper()
	file, err := source.Load(path, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	optionValues := make(map[string]rules.OptionValue, len(options))
	for name, value := range options {
		optionValues[name] = rules.BooleanOption(value)
	}
	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"exported-api-documentation": rules.SeverityWarn,
			},
			RuleOptions: map[string]rules.OptionSet{
				"exported-api-documentation": rules.NewOptionSet(optionValues),
			},
			SourceGoVersion: "go1.26",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func diagnosticNames(input string, result analysis.Result) []string {
	names := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		names = append(names, input[diagnostic.Range.Start:diagnostic.Range.End])
	}
	return names
}
