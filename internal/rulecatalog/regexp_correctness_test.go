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

func TestInvalidRegexpReportsConstantPatternsForExactStandardAPIs(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	re "regexp"
	"strings"
)

const invalid = "["

type localCompiler struct{}

func (localCompiler) Compile(string) {}

func inspect(pattern string, local localCompiler) {
	_, _ = re.Compile("[")
	_, _ = re.CompilePOSIX(invalid)
	_ = re.MustCompile("[")
	_ = re.MustCompilePOSIX(invalid)
	_, _ = re.Match("[", nil)
	_, _ = re.MatchReader(invalid, strings.NewReader(""))
	_, _ = re.MatchString("[", "")

	_, _ = re.Compile("a+")
	_, _ = re.Compile(pattern)
	local.Compile("[")
	dynamic := re.Compile
	_, _ = dynamic("[")
}
`
	result := runRegexpRule(t, input, "invalid-regexp")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 7 {
		t.Fatalf("invalid-regexp result = %#v", result)
	}
	wantText := []string{`"["`, "invalid", `"["`, "invalid", `"["`, "invalid", `"["`}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "invalid-regexp" ||
			diagnostic.MessageKey != "invalid-pattern" ||
			diagnostic.Message !=
				"invalid constant regular expression: missing closing ]" ||
			diagnostic.Range.Start < 0 ||
			diagnostic.Range.End > len(input) ||
			input[diagnostic.Range.Start:diagnostic.Range.End] != wantText[index] ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestInvalidRegexpUsesPOSIXSyntaxForPOSIXCompilers(t *testing.T) {
	t.Parallel()

	input := `package sample
import re "regexp"
func inspect() {
	_, _ = re.Compile(` +
		"`\\d`" +
		`)
	_, _ = re.CompilePOSIX(` +
		"`\\d`" +
		`)
}
`
	result := runRegexpRule(t, input, "invalid-regexp")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("invalid-regexp POSIX result = %#v", result)
	}
	diagnostic := result.Files[0].Diagnostics[0]
	if diagnostic.Message != "invalid constant regular expression: invalid escape sequence" ||
		input[diagnostic.Range.Start:diagnostic.Range.End] != "`\\d`" {
		t.Fatalf("invalid-regexp POSIX diagnostic = %#v", diagnostic)
	}
}

func TestInvalidRegexpBoundsConstantPatternValidation(t *testing.T) {
	t.Parallel()

	pattern := strings.Repeat("a", 64 << 10) + "["
	input := "package sample\nimport \"regexp\"\nconst pattern = `" +
		pattern +
		"`\nfunc inspect() { _, _ = regexp.Compile(pattern) }\n"
	result := runRegexpRule(t, input, "invalid-regexp")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("oversized invalid-regexp result = %#v", result)
	}
}

func TestZeroRegexpMatchLimitReportsExactFindAllMethods(t *testing.T) {
	t.Parallel()

	input := `package sample

import re "regexp"

const zero = 0

type localRegexp struct{}

func (localRegexp) FindAll([]byte, int) [][]byte { return nil }

func inspect(pattern *re.Regexp, local localRegexp, count int) {
	_ = pattern.FindAll(nil, 0)
	_ = pattern.FindAllIndex(nil, zero)
	_ = pattern.FindAllString("", 1 - 1)
	_ = pattern.FindAllStringIndex("", 0)
	_ = pattern.FindAllStringSubmatch("", zero)
	_ = pattern.FindAllStringSubmatchIndex("", 0)
	_ = pattern.FindAllSubmatch(nil, zero)
	_ = pattern.FindAllSubmatchIndex(nil, 0)

	_ = pattern.FindAll(nil, -1)
	_ = pattern.FindAll(nil, 1)
	_ = pattern.FindAll(nil, count)
	_ = local.FindAll(nil, 0)
	dynamic := pattern.FindAll
	_ = dynamic(nil, 0)
}
`
	result := runRegexpRule(t, input, "zero-regexp-match-limit")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 8 {
		t.Fatalf("zero-regexp-match-limit result = %#v", result)
	}
	wantText := []string{"0", "zero", "1 - 1", "0", "zero", "0", "zero", "0"}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "zero-regexp-match-limit" ||
			diagnostic.MessageKey != "zero-limit" ||
			diagnostic.Message !=
				"regexp FindAll limit zero always returns no matches" ||
			diagnostic.Range.Start < 0 ||
			diagnostic.Range.End > len(input) ||
			input[diagnostic.Range.Start:diagnostic.Range.End] != wantText[index] ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestRegexpCorrectnessMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"invalid-regexp", "zero-regexp-match-limit"} {
		metadata, found := registry.Metadata(id)
		if !found ||
			metadata.DefaultSeverity != rules.SeverityWarn ||
			!reflect.DeepEqual(
				metadata.Presets,
				[]rules.Preset{rules.PresetCorrectness},
			) ||
			metadata.MinimumGoVersion != "1.25" ||
			metadata.Requirement != rules.RequireTypes ||
			!reflect.DeepEqual(
				metadata.NodeInterests,
				[]rules.NodeKind{rules.NodeCallExpr},
			) ||
			metadata.RunOnGenerated ||
			metadata.RunDespiteTypeErrors ||
			len(metadata.Fixes) != 0 {
			t.Fatalf("%s metadata = %#v, found = %v", id, metadata, found)
		}
	}
	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"invalid-regexp": rules.SeverityWarn,
				"zero-regexp-match-limit": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil || len(older) != 0 {
		t.Fatalf("go1.24 regexp correctness selection = %#v, %v", older, err)
	}
}

func TestRegexpCorrectnessHonorsSharedSourcePolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/regexp-policy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample
import "regexp"
func suppressed(pattern *regexp.Regexp) {
	//glippy:ignore invalid-regexp -- intentional invalid-pattern fixture
	_, _ = regexp.Compile("[")
	//glippy:ignore zero-regexp-match-limit -- intentional empty result
	_ = pattern.FindAllString("", 0)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample
import "regexp"
func generated(pattern *regexp.Regexp) {
	_, _ = regexp.Compile("[")
	_ = pattern.FindAllString("", 0)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid
import "regexp"
func invalid(pattern *regexp.Regexp) {
	var text string = 1
	_ = text
	_, _ = regexp.Compile("[")
	_ = pattern.FindAllString("", 0)
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
				"invalid-regexp": rules.SeverityWarn,
				"zero-regexp-match-limit": rules.SeverityWarn,
			},
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
		t.Fatalf("regexp correctness policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 2 ||
				file.Suppressed[0].Diagnostic.RuleID != "invalid-regexp" ||
				file.Suppressed[1].Diagnostic.RuleID != "zero-regexp-match-limit" {
				t.Fatalf("suppressed result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy path %q", file.Path)
		}
	}
}

func runRegexpRule(t *testing.T, input string, id string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/regexp-correctness\n\ngo 1.25.0\n",
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
			Overrides: map[string]rules.Severity{id: rules.SeverityWarn},
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

func BenchmarkInvalidRegexp(b *testing.B) {
	benchmarkRegexpRule(b, "invalid-regexp", "_, _ = regexp.Compile(\"[\")")
}

func BenchmarkZeroRegexpMatchLimit(b *testing.B) {
	benchmarkRegexpRule(b, "zero-regexp-match-limit", "_ = pattern.FindAllString(\"value\", 0)")
}

func benchmarkRegexpRule(b *testing.B, id string, statement string) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/regexp-benchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"regexp\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func inspect%d(pattern *regexp.Regexp) { %s }\n",
			index,
			statement,
		)
	}
	writeFixture(b, filepath.Join(root, "sample.go"), input.String())
	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, runErr := analysis.RunPackages(
			context.Background(),
			registry,
			analysis.RunOptions{
				Presets: []rules.Preset{},
				Overrides: map[string]rules.Severity{id: rules.SeverityWarn},
				SourceGoVersion: "go1.25",
			},
			analysis.PackageLoadOptions{
				Dir: root,
				Patterns: []string{"."},
				ModuleMode: analysis.ModuleReadonly,
			},
		)
		if runErr != nil {
			b.Fatal(runErr)
		}
		if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 100 {
			b.Fatalf("%s benchmark result = %#v", id, result)
		}
	}
}
