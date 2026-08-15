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

func TestInvalidStrconvArgumentReportsConstantContractViolations(t *testing.T) {
	t.Parallel()

	input := `package sample

import convert "strconv"

const badBase = 1

type localConverter struct{}

func (localConverter) FormatInt(int64, int) string { return "" }

func inspect(base int, bitSize int, format byte, local localConverter) {
	_, _ = convert.ParseComplex("0", 32)
	_, _ = convert.ParseFloat("0", 128)
	_, _ = convert.ParseInt("0", badBase, 0)
	_, _ = convert.ParseUint("0", 10, 65)
	_ = convert.FormatComplex(0, 'j', -1, 64)
	_ = convert.FormatComplex(0, 'g', -1, 32)
	_ = convert.FormatFloat(0, '?', -1, 64)
	_ = convert.FormatInt(0, 1)
	_ = convert.FormatUint(0, 37)
	_ = convert.AppendFloat(nil, 0, 'a', -1, 64)
	_ = convert.AppendInt(nil, 0, 0)
	_ = convert.AppendUint(nil, 0, 37)

	_, _ = convert.ParseComplex("0", 64)
	_, _ = convert.ParseComplex("0", 128)
	_, _ = convert.ParseFloat("0", 32)
	_, _ = convert.ParseFloat("0", 64)
	_, _ = convert.ParseInt("0", 0, 0)
	_, _ = convert.ParseInt("0", 2, 64)
	_, _ = convert.ParseUint("0", 36, 1)
	_ = convert.FormatComplex(0, 'x', -1, 128)
	_ = convert.FormatFloat(0, 'G', -1, 32)
	_ = convert.FormatInt(0, 2)
	_ = convert.FormatUint(0, 36)
	_ = convert.AppendFloat(nil, 0, 'f', -1, 64)
	_ = convert.AppendInt(nil, 0, 10)
	_ = convert.AppendUint(nil, 0, 16)
	_, _ = convert.ParseInt("0", base, bitSize)
	_ = convert.FormatFloat(0, format, -1, 64)
	_ = local.FormatInt(0, 1)
	dynamic := convert.FormatInt
	_ = dynamic(0, 1)
}
`
	result := runStandardLibraryArgumentRule(t, input, "invalid-strconv-argument")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 12 {
		t.Fatalf("invalid-strconv-argument result = %#v", result)
	}
	want := []struct {
		text string
		messageKey string
		message string
	}{
		{"32", "invalid-bit-size", "strconv.ParseComplex bit size must be 64 or 128"},
		{"128", "invalid-bit-size", "strconv.ParseFloat bit size must be 32 or 64"},
		{"badBase", "invalid-base", "strconv.ParseInt base must be 0 or between 2 and 36"},
		{"65", "invalid-bit-size", "strconv.ParseUint bit size must be between 0 and 64"},
		{
			"'j'",
			"invalid-format",
			"strconv.FormatComplex format must be one of b, e, E, f, g, G, x, or X",
		},
		{"32", "invalid-bit-size", "strconv.FormatComplex bit size must be 64 or 128"},
		{
			"'?'",
			"invalid-format",
			"strconv.FormatFloat format must be one of b, e, E, f, g, G, x, or X",
		},
		{"1", "invalid-base", "strconv.FormatInt base must be between 2 and 36"},
		{"37", "invalid-base", "strconv.FormatUint base must be between 2 and 36"},
		{
			"'a'",
			"invalid-format",
			"strconv.AppendFloat format must be one of b, e, E, f, g, G, x, or X",
		},
		{"0", "invalid-base", "strconv.AppendInt base must be between 2 and 36"},
		{"37", "invalid-base", "strconv.AppendUint base must be between 2 and 36"},
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		expected := want[index]
		if diagnostic.RuleID != "invalid-strconv-argument" ||
			diagnostic.MessageKey != expected.messageKey ||
			diagnostic.Message != expected.message ||
			diagnostic.Range.Start < 0 ||
			diagnostic.Range.End > len(input) ||
			input[diagnostic.Range.Start:diagnostic.Range.End] != expected.text ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestInvalidBinaryWriteReportsStaticallyUnsupportedData(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	bin "encoding/binary"
	"io"
)

type word int
type validPointer *uint32
type invalidRecord struct { Text string }
type invalidInterfaceRecord struct { Value any }
type validRecord struct { Flag bool; Values [2]uint32 }
type localBinary struct{}

func (localBinary) Write(io.Writer, bin.ByteOrder, any) error { return nil }

func inspect(writer io.Writer, order bin.ByteOrder, dynamic any, local localBinary) {
	var function func()
	var pointer **uint32
	var interfacePointer *any
	var namedPointer validPointer
	_ = bin.Write(writer, order, int(1))
	_ = bin.Write(writer, order, word(1))
	_ = bin.Write(writer, order, "text")
	_ = bin.Write(writer, order, []int{1})
	_ = bin.Write(writer, order, [1]uint{1})
	_ = bin.Write(writer, order, invalidRecord{})
	_ = bin.Write(writer, order, function)
	_ = bin.Write(writer, order, pointer)
	_ = bin.Write(writer, order, []any{1})
	_ = bin.Write(writer, order, invalidInterfaceRecord{})
	_ = bin.Write(writer, order, interfacePointer)

	_ = bin.Write(writer, order, uint32(1))
	_ = bin.Write(writer, order, []byte("text"))
	_ = bin.Write(writer, order, [1]complex128{1})
	_ = bin.Write(writer, order, validRecord{})
	_ = bin.Write(writer, order, &validRecord{})
	_ = bin.Write(writer, order, namedPointer)
	_ = bin.Write(writer, order, dynamic)
	_ = local.Write(writer, order, "text")
	write := bin.Write
	_ = write(writer, order, "text")
}
`
	result := runStandardLibraryArgumentRule(t, input, "invalid-binary-write")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 11 {
		t.Fatalf("invalid-binary-write result = %#v", result)
	}
	wantText := []string{
		"int(1)",
		"word(1)",
		`"text"`,
		"[]int{1}",
		"[1]uint{1}",
		"invalidRecord{}",
		"function",
		"pointer",
		"[]any{1}",
		"invalidInterfaceRecord{}",
		"interfacePointer",
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "invalid-binary-write" ||
			diagnostic.MessageKey != "variable-size-data" ||
			diagnostic.Message !=
				"binary.Write cannot encode a value whose type has variable size" ||
			diagnostic.Range.Start < 0 ||
			diagnostic.Range.End > len(input) ||
			input[diagnostic.Range.Start:diagnostic.Range.End] != wantText[index] ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
	}
}

func TestStandardLibraryArgumentRuleMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"invalid-binary-write", "invalid-strconv-argument"} {
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
				"invalid-binary-write": rules.SeverityWarn,
				"invalid-strconv-argument": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil || len(older) != 0 {
		t.Fatalf("go1.24 standard-library argument selection = %#v, %v", older, err)
	}
}

func TestStandardLibraryArgumentRulesHonorSharedSourcePolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/standard-library-argument-policy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample
import (
	"encoding/binary"
	"io"
	"strconv"
)
func suppressed(writer io.Writer) {
	//glippy:ignore invalid-binary-write -- compatibility fixture
	_ = binary.Write(writer, binary.LittleEndian, "text")
	//glippy:ignore invalid-strconv-argument -- compatibility fixture
	_ = strconv.FormatInt(0, 0)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample
import (
	"encoding/binary"
	"io"
	"strconv"
)
func generated(writer io.Writer) {
	_ = binary.Write(writer, binary.LittleEndian, "text")
	_ = strconv.FormatInt(0, 0)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid
import (
	"encoding/binary"
	"io"
	"strconv"
)
func invalid(writer io.Writer) {
	var text string = 1
	_ = text
	_ = binary.Write(writer, binary.LittleEndian, "text")
	_ = strconv.FormatInt(0, 0)
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
				"invalid-binary-write": rules.SeverityWarn,
				"invalid-strconv-argument": rules.SeverityWarn,
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
		t.Fatalf("standard-library argument policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 2 ||
				file.Suppressed[0].Diagnostic.RuleID != "invalid-binary-write" ||
				file.Suppressed[1].Diagnostic.RuleID != "invalid-strconv-argument" {
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

func runStandardLibraryArgumentRule(t *testing.T, input string, id string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/standard-library-arguments\n\ngo 1.25.0\n",
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

func BenchmarkInvalidStrconvArgument(b *testing.B) {
	benchmarkStandardLibraryArgumentRule(
		b,
		"invalid-strconv-argument",
		"strconv",
		`_ = strconv.FormatInt(0, 0)`,
	)
}

func BenchmarkInvalidBinaryWrite(b *testing.B) {
	benchmarkStandardLibraryArgumentRule(
		b,
		"invalid-binary-write",
		"encoding/binary",
		`_ = binary.Write(io.Discard, binary.LittleEndian, "text")`,
	)
}

func benchmarkStandardLibraryArgumentRule(
	b *testing.B,
	id string,
	packagePath string,
	statement string,
) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/standard-library-argument-benchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	if packagePath == "encoding/binary" {
		input.WriteString("package sample\nimport (\n\t\"encoding/binary\"\n\t\"io\"\n)\n")
	} else {
		fmt.Fprintf(&input, "package sample\nimport %q\n", packagePath)
	}
	for index := range 100 {
		fmt.Fprintf(&input, "func inspect%d() { %s }\n", index, statement)
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
