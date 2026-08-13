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

func TestHTTPCanonicalHeaderKeyReportsDirectNoncanonicalAccess(t *testing.T) {
	t.Parallel()

	input := `package sample

import "net/http"

const lowerContentType = "content-type"

func bad(header http.Header, request *http.Request) {
	_ = header["content-type"]
	header["x-request-id"] = []string{"one"}
	_ = request.Header[lowerContentType]
}

func good(header http.Header, ordinary map[string][]string, key string) {
	_ = header["Content-Type"]
	header.Set("content-type", "text/plain")
	_ = header[key]
	_ = ordinary["content-type"]
	_ = header["bad key"]
}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/httpheaderkey\n\ngo 1.25.0\n",
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
				"http-canonical-header-key": rules.SeverityWarn,
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
	want := []struct {
		text string
		prefix string
	}{
		{text: `"content-type"`, prefix: `_ = header[`},
		{text: `"x-request-id"`, prefix: `header[`},
		{text: "lowerContentType", prefix: `_ = request.Header[`},
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(want) {
		t.Fatalf("http-canonical-header-key result = %#v", result)
	}
	searchFrom := 0
	for index, diagnostic := range result.Files[0].Diagnostics {
		relativePrefix := strings.Index(input[searchFrom:], want[index].prefix)
		if relativePrefix < 0 {
			t.Fatalf("missing diagnostic prefix %q", want[index].prefix)
		}
		searchFrom += relativePrefix + len(want[index].prefix)
		relative := strings.Index(input[searchFrom:], want[index].text)
		if relative < 0 {
			t.Fatalf("missing diagnostic text %q", want[index].text)
		}
		start := searchFrom + relative
		if diagnostic.RuleID != "http-canonical-header-key" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want[index].text) ||
			!strings.Contains(diagnostic.Message, "canonical") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic[%d] = %#v", index, diagnostic)
		}
		searchFrom = start + len(want[index].text)
	}
}

func TestHTTPCanonicalHeaderKeyMetadata(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("http-canonical-header-key")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.MinimumGoVersion != "1.25" ||
		metadata.Requirement != rules.RequireTypes ||
		!reflect.DeepEqual(metadata.NodeInterests, []rules.NodeKind{rules.NodeIndexExpr}) ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 {
		t.Fatalf("http-canonical-header-key metadata = %#v, found = %v", metadata, found)
	}
}

func TestHTTPCanonicalHeaderKeyHonorsSuppression(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/httpheaderkeysuppression\n\ngo 1.25.0\n",
	)
	writeFixture(
		t,
		filepath.Join(root, "sample.go"),
		`package sample
import "net/http"
func read(header http.Header) []string {
	//glippy:ignore http-canonical-header-key -- upstream preserves raw casing
	return header["content-type"]
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
				"http-canonical-header-key": rules.SeverityWarn,
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
		len(result.Files[0].Diagnostics) != 0 ||
		len(result.Files[0].Suppressed) != 1 ||
		result.Files[0].Suppressed[0].Diagnostic.RuleID != "http-canonical-header-key" {
		t.Fatalf("http-canonical-header-key suppression result = %#v", result)
	}
}

func BenchmarkHTTPCanonicalHeaderKeyPackageAnalysis(b *testing.B) {
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/httpheaderkeybenchmark\n\ngo 1.25.0\n",
	)
	writeFixture(
		b,
		filepath.Join(root, "sample.go"),
		"package sample\nimport \"net/http\"\nfunc read(header http.Header) []string { return header[\"content-type\"] }\n",
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
				"http-canonical-header-key": rules.SeverityWarn,
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
