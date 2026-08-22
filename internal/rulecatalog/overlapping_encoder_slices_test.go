package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

func TestOverlappingEncoderSlicesReportsProvenOverlap(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
)

var initializerBuffer = make([]byte, 64)
var _ = hex.Encode(initializerBuffer, initializerBuffer)
var compositeBuffer = []byte{}
var _ = hex.Encode(compositeBuffer, compositeBuffer)
var nil = make([]byte, 64)
var _ = hex.Encode(nil, nil)

func packageEncoders(buffer []byte) {
	hex.Encode(buffer, buffer)
	ascii85.Encode(buffer[:], buffer[:])
}

func methodEncoders(buffer []byte) {
	base32.StdEncoding.Encode(buffer[1:], buffer[1:])
	base64.StdEncoding.Encode(buffer, buffer)
}

func methodExpression(encoding *base64.Encoding, buffer []byte) {
	(*base64.Encoding).Encode(encoding, buffer, buffer)
}

func boundMethodAlias(buffer []byte) {
	encode := base64.StdEncoding.Encode
	encode(buffer, buffer)
}

func resolvedAlias(buffer []byte) {
	encode := hex.Encode
	encode(buffer, buffer)
}

type namedBytes []byte

func convertedNamedSlice(buffer namedBytes) {
	hex.Encode([]byte(buffer), []byte(buffer))
}

func copiedAliases(buffer []byte) {
	destination := buffer
	source := buffer
	hex.Encode(destination, source)
}

func variableLowerBound(buffer []byte, offset int) {
	hex.Encode(buffer[offset:], buffer[offset:])
}

func equivalentPhi(buffer []byte, condition bool) {
	destination := buffer
	if condition {
		destination = buffer
	}
	hex.Encode(destination, buffer)
}

func convertedEquivalentPhi(buffer namedBytes, condition bool) {
	var destination []byte
	if condition {
		destination = []byte(buffer)
	} else {
		destination = []byte(buffer)
	}
	hex.Encode(destination, []byte(buffer))
}

func deferredAndAsynchronous(buffer []byte) {
	defer hex.Encode(buffer, buffer)
	go hex.Encode(buffer, buffer)
}
`
	result := runOverlappingEncoderSlices(t, input)
	want := []struct {
		call string
		destination string
	}{
		{"hex.Encode(initializerBuffer, initializerBuffer)", "initializerBuffer"},
		{"hex.Encode(compositeBuffer, compositeBuffer)", "compositeBuffer"},
		{"hex.Encode(nil, nil)", "nil"},
		{"hex.Encode(buffer, buffer)", "buffer"},
		{"ascii85.Encode(buffer[:], buffer[:])", "buffer[:]"},
		{"base32.StdEncoding.Encode(buffer[1:], buffer[1:])", "buffer[1:]"},
		{"base64.StdEncoding.Encode(buffer, buffer)", "buffer"},
		{"(*base64.Encoding).Encode(encoding, buffer, buffer)", "buffer"},
		{"encode(buffer, buffer)", "buffer"},
		{"encode(buffer, buffer)", "buffer"},
		{"hex.Encode([]byte(buffer), []byte(buffer))", "[]byte(buffer)"},
		{"hex.Encode(destination, source)", "destination"},
		{"hex.Encode(buffer[offset:], buffer[offset:])", "buffer[offset:]"},
		{"hex.Encode(destination, buffer)", "destination"},
		{"hex.Encode(destination, []byte(buffer))", "destination"},
		{"hex.Encode(buffer, buffer)", "buffer"},
		{"hex.Encode(buffer, buffer)", "buffer"},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf(
			"result = %#v, want %d overlapping-encoder-slices diagnostics",
			result,
			len(want),
		)
	}
	searchStart := 0
	for index, expected := range want {
		callOffset := strings.Index(input[searchStart:], expected.call)
		if callOffset < 0 {
			t.Fatalf("input does not contain %q after %d", expected.call, searchStart)
		}
		callOffset += searchStart
		offset := strings.Index(expected.call, expected.destination)
		if offset < 0 {
			t.Fatalf("call does not contain destination %q", expected.destination)
		}
		offset += callOffset
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "overlapping-encoder-slices" ||
			diagnostic.MessageKey != "overlapping-encoder-slices" ||
			diagnostic.Message != "encoder destination and source slices overlap" ||
			diagnostic.Help !=
				"use a destination backed by different memory from the source" ||
			diagnostic.Range !=
				(source.Range{
					Start: offset,
					End: offset + len(expected.destination),
				}) ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
		searchStart = callOffset + len(expected.call)
	}
}

func TestOverlappingEncoderSlicesExcludesUnprovenAndSafeBuffers(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"encoding/base64"
	"encoding/hex"
)

var nilBuffer []byte
var _ = hex.Encode(nilBuffer, nilBuffer)
var explicitNilBuffer []byte = nil
var _ = hex.Encode(explicitNilBuffer, explicitNilBuffer)
var convertedNilBuffer = []byte(nil)
var _ = hex.Encode(convertedNilBuffer, convertedNilBuffer)
var aliasedNilBuffer = nilBuffer
var _ = hex.Encode(aliasedNilBuffer, aliasedNilBuffer)

type encoder struct{}

func (encoder) Encode([]byte, []byte) {}

func unproven(destination, source []byte) {
	hex.Encode(destination, source)
	base64.StdEncoding.Encode(destination, source)
}

func differentOffsets(buffer []byte) {
	hex.Encode(buffer[1:], buffer[2:])
}

func nilSource(destination []byte) {
	hex.Encode(destination, nil)
}

func definitelyNilVariable() {
	var buffer []byte
	hex.Encode(buffer, buffer)
}

func unresolved(encode func([]byte, []byte), buffer []byte) {
	encode(buffer, buffer)
}

func lookalike(buffer []byte) {
	encoder{}.Encode(buffer, buffer)
}

func safeAppend(buffer []byte) {
	_ = hex.AppendEncode(buffer, buffer)
}

func decoder(buffer []byte) {
	_, _ = hex.Decode(buffer, buffer)
}
`
	result := runOverlappingEncoderSlices(t, input)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestOverlappingEncoderSlicesOwnsPackageInitializerDiagnosticsByFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/overlappingencoderinitializers\n\ngo 1.25.0\n",
	)
	first := `package sample

import "encoding/hex"

var buffer = make([]byte, 64)
var zeroBuffer []byte
var encoded = hex.Encode(buffer, buffer)
`
	writeFixture(t, filepath.Join(root, "first.go"), first)
	writeFixture(
		t,
		filepath.Join(root, "second.go"),
		`package sample

import "encoding/hex"

var unrelated = hex.Encode(zeroBuffer, zeroBuffer)
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
				"overlapping-encoder-slices": rules.SeverityWarn,
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
	if len(result.Files) != 2 {
		t.Fatalf("initializer ownership result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "first.go":
			start := strings.Index(first, "buffer, buffer")
			if len(file.Diagnostics) != 1 ||
				file.Diagnostics[0].Range !=
					(source.Range{Start: start, End: start + len("buffer")}) {
				t.Fatalf("first initializer result = %#v", file)
			}
		case "second.go":
			if len(file.Diagnostics) != 0 {
				t.Fatalf("second initializer result = %#v", file)
			}
		default:
			t.Fatalf("unexpected initializer file %q", file.Path)
		}
	}
}

func TestOverlappingEncoderSlicesMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("overlapping-encoder-slices")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!slices.Equal(metadata.Presets, []rules.Preset{rules.PresetCorrectness}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireSSA ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		!slices.Equal(
			metadata.Categories,
			[]rules.Category{rules.CategoryCorrectness, rules.CategorySafety},
		) ||
		len(metadata.Fixes) != 0 ||
		len(metadata.Examples) == 0 ||
		len(metadata.KnownLimitations) == 0 {
		t.Fatalf("overlapping-encoder-slices metadata = %#v, found = %v", metadata, found)
	}
}

func TestOverlappingEncoderSlicesHonorsSharedPolicies(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/overlappingencoderslicespolicy\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "suppressed.go"),
		`package sample

import "encoding/hex"

func suppressed(buffer []byte) {
	//glippy:ignore overlapping-encoder-slices -- legacy in-place buffer migration
	hex.Encode(buffer, buffer)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "generated.go"),
		`// Code generated by fixture. DO NOT EDIT.
package sample

import "encoding/hex"

func generated(buffer []byte) {
	hex.Encode(buffer, buffer)
}
`,
	)
	writeFixture(
		t,
		filepath.Join(root, "invalid", "invalid.go"),
		`package invalid

import "encoding/hex"

func invalid(buffer []byte) {
	var broken string = 1
	_ = broken
	hex.Encode(buffer, buffer)
}
`,
	)
	result, err := analysis.RunPackages(
		context.Background(),
		registry,
		analysis.RunOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"overlapping-encoder-slices": rules.SeverityError,
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
		t.Fatalf("overlapping-encoder-slices policy result = %#v", result)
	}
	for _, file := range result.Files {
		switch filepath.Base(file.Path) {
		case "suppressed.go":
			if len(file.Diagnostics) != 0 ||
				len(file.Suppressed) != 1 ||
				file.Suppressed[0].Diagnostic.RuleID !=
					"overlapping-encoder-slices" ||
				file.Suppressed[0].Diagnostic.Severity != rules.SeverityError {
				t.Fatalf("suppressed overlap result = %#v", file)
			}
		case "generated.go", "invalid.go":
			if len(file.Diagnostics) != 0 || len(file.Suppressed) != 0 {
				t.Fatalf("excluded overlap result = %#v", file)
			}
		default:
			t.Fatalf("unexpected policy file %q", file.Path)
		}
	}
	selection, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{rules.PresetCorrectness},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range selection {
		if selected.ID == "overlapping-encoder-slices" {
			t.Fatalf(
				"pre-minimum overlapping-encoder-slices selection = %#v",
				selection,
			)
		}
	}
}

func runOverlappingEncoderSlices(t *testing.T, input string) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/overlappingencoderslices\n\ngo 1.25.0\n",
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
				"overlapping-encoder-slices": rules.SeverityWarn,
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
	return result
}

func BenchmarkOverlappingEncoderSlicesPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/overlappingencoderslicesbenchmark\n\ngo 1.25.0\n",
	)
	var input strings.Builder
	input.WriteString("package sample\nimport \"encoding/hex\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d(buffer []byte) { hex.Encode(buffer, buffer) }\n",
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
				"overlapping-encoder-slices": rules.SeverityWarn,
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
