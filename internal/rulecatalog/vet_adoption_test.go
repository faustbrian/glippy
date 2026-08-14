package rulecatalog_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestInvalidBuildConstraintReportsMisplacedConstraint(t *testing.T) {
	t.Parallel()

	input := "package sample\n\n//go:build linux\n\nfunc run() {}\n"
	result := runAdoptionSyntaxRule(t, "invalid-build-constraint", input)
	assertSingleSyntaxDiagnostic(
		t,
		result,
		input,
		"invalid-build-constraint",
		"//go:build linux",
	)
}

func TestInvalidDirectiveReportsInvalidGoDebugPackage(t *testing.T) {
	t.Parallel()

	input := "//go:debug panicnil=1\npackage sample\n\nfunc run() {}\n"
	result := runAdoptionSyntaxRule(t, "invalid-directive", input)
	assertSingleSyntaxDiagnostic(t, result, input, "invalid-directive", "//go:debug")
}

func TestInvalidTestSignatureReportsGenericTestFunction(t *testing.T) {
	t.Parallel()

	input := "package sample\n\nimport \"testing\"\n\nfunc TestGeneric[T any](t *testing.T) {}\n"
	result := runAdoptionVetRule(
		t,
		"invalid-test-signature",
		"go1.25",
		map[string]string{"sample_test.go": input},
	)
	assertSingleRuleDiagnostic(t, result, "invalid-test-signature", "[T any")
}

func TestUnbufferedSignalChannelReportsAndSuggestsBuffer(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"os"
	"os/signal"
)

func run() {
	interrupts := make(chan os.Signal)
	signal.Notify(interrupts, os.Interrupt)
}
`
	result := runAdoptionVetRule(
		t,
		"unbuffered-signal-channel",
		"go1.25",
		map[string]string{"sample.go": input},
	)
	diagnostic := assertSingleRuleDiagnostic(
		t,
		result,
		"unbuffered-signal-channel",
		"signal.Notify(interrupts, os.Interrupt)",
	)
	if len(diagnostic.Fixes) != 1 ||
		diagnostic.Fixes[0].Name != "buffer-signal-channel" ||
		diagnostic.Fixes[0].Safety != rules.FixSuggestion ||
		len(diagnostic.Fixes[0].Edits) != 1 ||
		string(diagnostic.Fixes[0].Edits[0].NewText) != "make(chan os.Signal, 1)" {
		t.Fatalf("unbuffered signal fix = %#v", diagnostic.Fixes)
	}
}

func TestStandardLibraryVersionReportsSymbolNewerThanModule(t *testing.T) {
	t.Parallel()

	input := `package sample

import "bytes"

func run(buffer *bytes.Buffer) {
	_, _ = buffer.Peek(1)
}
`
	result := runAdoptionVetRule(
		t,
		"standard-library-version",
		"go1.25",
		map[string]string{"sample.go": input},
	)
	diagnostic := assertSingleRuleDiagnostic(t, result, "standard-library-version", "Peek")
	if !strings.Contains(diagnostic.Message, "requires go1.26 or later") {
		t.Fatalf("standard library version message = %q", diagnostic.Message)
	}
}

func TestAdoptionVetRulesExcludeNearbyValidCode(t *testing.T) {
	t.Parallel()

	syntaxCases := []struct {
		ruleID string
		input string
	}{
		{
			ruleID: "invalid-build-constraint",
			input: "//go:build linux\n\npackage sample\n\nfunc run() {}\n",
		},
		{
			ruleID: "invalid-directive",
			input: "//go:debug panicnil=1\npackage main\n\nfunc main() {}\n",
		},
	}
	for _, test := range syntaxCases {
		result := runAdoptionSyntaxRule(t, test.ruleID, test.input)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("valid %s result = %#v", test.ruleID, result)
		}
	}

	packageCases := []struct {
		ruleID string
		name string
		input string
	}{
		{
			ruleID: "invalid-test-signature",
			name: "sample_test.go",
			input: "package sample\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {}\n",
		},
		{
			ruleID: "unbuffered-signal-channel",
			name: "sample.go",
			input: "package sample\n\nimport (\"os\"; \"os/signal\")\n" +
				"func run() { signals := make(chan os.Signal, 1); signal.Notify(signals, os.Interrupt) }\n",
		},
		{
			ruleID: "standard-library-version",
			name: "sample.go",
			input: "package sample\n\nimport \"bytes\"\n\nfunc run(value []byte) { _ = bytes.Clone(value) }\n",
		},
	}
	for _, test := range packageCases {
		result := runAdoptionVetRule(
			t,
			test.ruleID,
			"go1.25",
			map[string]string{test.name: test.input},
		)
		if countPackageDiagnostics(result) != 0 {
			t.Fatalf("valid %s result = %#v", test.ruleID, result)
		}
	}
}

func TestAdoptionVetPackMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id string
		presets []rules.Preset
		requirement rules.Requirement
		runDespiteError bool
		fix string
	}{
		{
			"invalid-build-constraint",
			[]rules.Preset{rules.PresetCorrectness},
			rules.RequireSyntax,
			false,
			"",
		},
		{
			"invalid-directive",
			[]rules.Preset{rules.PresetCorrectness},
			rules.RequireSyntax,
			false,
			"",
		},
		{
			"invalid-test-signature",
			[]rules.Preset{rules.PresetCorrectness},
			rules.RequireTypes,
			false,
			"",
		},
		{
			"unbuffered-signal-channel",
			[]rules.Preset{rules.PresetCorrectness},
			rules.RequireTypes,
			false,
			"buffer-signal-channel",
		},
		{
			"standard-library-version",
			[]rules.Preset{rules.PresetCorrectness, rules.PresetMigration},
			rules.RequireTypes,
			true,
			"",
		},
	}
	for _, test := range tests {
		metadata, found := registry.Metadata(test.id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(metadata.Presets, test.presets) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != test.requirement ||
			!reflect.DeepEqual(
				metadata.NodeInterests,
				[]rules.NodeKind{rules.NodeFile},
			) ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors != test.runDespiteError {
			t.Fatalf("%s metadata = %#v, found = %v", test.id, metadata, found)
		}
		if test.fix == "" {
			if len(metadata.Fixes) != 0 {
				t.Fatalf("%s fixes = %#v", test.id, metadata.Fixes)
			}
			continue
		}
		if len(metadata.Fixes) != 1 ||
			metadata.Fixes[0].Name != test.fix ||
			metadata.Fixes[0].Safety != rules.FixSuggestion {
			t.Fatalf("%s fixes = %#v", test.id, metadata.Fixes)
		}
	}
}

func runAdoptionVetRule(
	t *testing.T,
	ruleID string,
	goVersion string,
	files map[string]string,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/vetadoption\n\ngo " +
			strings.TrimPrefix(goVersion, "go") +
			".0\n",
	)
	for name, content := range files {
		writeFixture(t, filepath.Join(root, name), content)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
			SourceGoVersion: goVersion,
		},
		analysis.PackageLoadOptions{
			Dir: root,
			Patterns: []string{"."},
			Tests: true,
			ModuleMode: analysis.ModuleReadonly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runAdoptionSyntaxRule(t *testing.T, ruleID, input string) analysis.Result {
	t.Helper()
	file, err := source.Load("sample.go", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := analysis.Run(
		context.Background(),
		file,
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{ruleID: rules.SeverityWarn},
			SourceGoVersion: "go1.25",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertSingleSyntaxDiagnostic(
	t *testing.T,
	result analysis.Result,
	input string,
	ruleID string,
	wantText string,
) {
	t.Helper()
	if len(result.Diagnostics) != 1 {
		t.Fatalf("%s result = %#v", ruleID, result)
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.RuleID != ruleID {
		t.Fatalf("diagnostic rule = %q, want %q", diagnostic.RuleID, ruleID)
	}
	if wantStart := strings.Index(input, wantText); diagnostic.Range.Start != wantStart {
		t.Fatalf(
			"%s diagnostic start = %d, want %d for %q",
			ruleID,
			diagnostic.Range.Start,
			wantStart,
			wantText,
		)
	}
}

func assertSingleRuleDiagnostic(
	t *testing.T,
	result analysis.PackageResult,
	ruleID string,
	wantText string,
) rules.Diagnostic {
	t.Helper()
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("%s result = %#v", ruleID, result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	if diagnostic.RuleID != ruleID {
		t.Fatalf("diagnostic rule = %q, want %q", diagnostic.RuleID, ruleID)
	}
	input, err := filepath.Abs(result.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content[diagnostic.Range.Start:diagnostic.Range.End]);
		!strings.Contains(got, wantText) {
		t.Fatalf("%s diagnostic range text = %q, want containing %q", ruleID, got, wantText)
	}
	return diagnostic
}
