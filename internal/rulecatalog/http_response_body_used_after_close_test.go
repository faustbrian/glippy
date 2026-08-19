package rulecatalog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/contracts"
	"github.com/faustbrian/glippy/internal/rulecatalog"
	"github.com/faustbrian/glippy/internal/rules"
)

func TestHTTPResponseBodyUsedAfterCloseReportsProvenClosedOperations(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io"
	"net/http"
)

func directRead() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	_, err = response.Body.Read(nil)
	return err
}

func standardReader() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	_, err = io.ReadAll(response.Body)
	return err
}

func repeatedClose() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	return response.Body.Close()
}
`
	result := runHTTPResponseBodyUsedAfterClose(t, input, "go1.25")
	wants := []string{
		"response.Body.Read(nil)",
		"io.ReadAll(response.Body)",
		"response.Body.Close()",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(wants) {
		t.Fatalf("HTTP response body use-after-close result = %#v", result)
	}
	searchFrom := 0
	for index, want := range wants {
		start := strings.Index(input[searchFrom:], want)
		if start < 0 {
			t.Fatalf("missing operation %q", want)
		}
		start += searchFrom
		if index == 2 {
			second := strings.Index(input[start + len(want):], want)
			if second < 0 {
				t.Fatalf("missing repeated close %q", want)
			}
			start += len(want) + second
		}
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "http-response-body-used-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want) ||
			!strings.Contains(diagnostic.Message, "after it is closed") ||
			len(diagnostic.Related) != 1 ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("HTTP response body diagnostic %d = %#v", index, diagnostic)
		}
		searchFrom = start + len(want)
	}
}

func TestHTTPResponseBodyUsedAfterCloseTracksBranchesAndFailsClosed(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io"
	"net/http"
)

func closedBranches(closeFirst bool) error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	if closeFirst { _ = response.Body.Close() } else { _ = response.Body.Close() }
	_, err = response.Body.Read(nil)
	return err
}

func conditionalClose(closeNow bool) error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	if closeNow { _ = response.Body.Close() }
	_, err = response.Body.Read(nil)
	return err
}

func deferredClose() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	defer response.Body.Close()
	_, err = response.Body.Read(nil)
	return err
}

func asynchronousClose() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	go response.Body.Close()
	_, err = response.Body.Read(nil)
	return err
}

func unknownHelperAfterClose(consume func(io.Reader)) error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	consume(response.Body)
	_, err = response.Body.Read(nil)
	return err
}

func exactCloseAfterUnknown(consume func(io.Reader)) error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	consume(response.Body)
	_ = response.Body.Close()
	_, err = response.Body.Read(nil)
	return err
}

func aliasAfterClose() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	body := response.Body
	_ = body
	_, err = response.Body.Read(nil)
	return err
}

func multipleCallsInNode() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = consumeErrors(response.Body.Close(), response.Body.Close())
	_, err = response.Body.Read(nil)
	return err
}

func consumeErrors(error, error) error { return nil }
`
	result := runHTTPResponseBodyUsedAfterClose(t, input, "go1.25")
	wants := []string{"response.Body.Read(nil)", "response.Body.Read(nil)"}
	functions := []string{"func closedBranches(", "func exactCloseAfterUnknown("}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(wants) {
		t.Fatalf("HTTP response body branch state result = %#v", result)
	}
	for index, want := range wants {
		functionStart := strings.Index(input, functions[index])
		start := functionStart + strings.Index(input[functionStart:], want)
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "http-response-body-used-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(want) {
			t.Fatalf("HTTP response body branch diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestHTTPResponseBodyUsedAfterCloseConsumesProvenEffects(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"io"
	"net/http"
)

func closeBody(body io.ReadCloser) { _ = body.Close() }
func borrowBody(io.Reader) {}
func handoffBody(body io.ReadCloser) { go body.Close() }

func helperClose() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	closeBody(response.Body)
	_, err = response.Body.Read(nil)
	return err
}

func provenBorrow() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	borrowBody(response.Body)
	_, err = response.Body.Read(nil)
	return err
}

func transferred() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	handoffBody(response.Body)
	_, err = response.Body.Read(nil)
	return err
}

func clientCall(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil { return err }
	_ = response.Body.Close()
	_, err = io.ReadAll(response.Body)
	return err
}
`
	result := runHTTPResponseBodyUsedAfterClose(t, input, "go1.25")
	functions := []string{"func helperClose(", "func provenBorrow(", "func clientCall("}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(functions) {
		t.Fatalf("HTTP response body effect result = %#v", result)
	}
	for index, function := range functions {
		functionStart := strings.Index(input, function)
		operation := "response.Body.Read(nil)"
		if index == 2 {
			operation = "io.ReadAll(response.Body)"
		}
		start := functionStart + strings.Index(input[functionStart:], operation)
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.RuleID != "http-response-body-used-after-close" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len(operation) {
			t.Fatalf("HTTP response body effect diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestHTTPResponseBodyUsedAfterCloseRecognizesImmediateIOConsumers(t *testing.T) {
	t.Parallel()

	input := `package sample
import (
	"io"
	"net/http"
)
func readAll() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.ReadAll(response.Body); return err }
func readAtLeast() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.ReadAtLeast(response.Body, make([]byte, 1), 1); return err }
func readFull() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.ReadFull(response.Body, make([]byte, 1)); return err }
func copyBody() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.Copy(io.Discard, response.Body); return err }
func copyN() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.CopyN(io.Discard, response.Body, 1); return err }
func copyBuffer() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.CopyBuffer(io.Discard, response.Body, nil); return err }
func zeroReadAtLeast() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.ReadAtLeast(response.Body, nil, 0); return err }
func emptyReadFull() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.ReadFull(response.Body, nil); return err }
func zeroCopyN() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = io.CopyN(io.Discard, response.Body, 0); return err }
func constructorOnly() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _ = io.LimitReader(response.Body, 1); return nil }
`
	result := runHTTPResponseBodyUsedAfterClose(t, input, "go1.25")
	wants := []string{
		"io.ReadAll(response.Body)",
		"io.ReadAtLeast(response.Body, make([]byte, 1), 1)",
		"io.ReadFull(response.Body, make([]byte, 1))",
		"io.Copy(io.Discard, response.Body)",
		"io.CopyN(io.Discard, response.Body, 1)",
		"io.CopyBuffer(io.Discard, response.Body, nil)",
	}
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != len(wants) {
		t.Fatalf("HTTP response body io consumer result = %#v", result)
	}
	for index, want := range wants {
		start := strings.Index(input, want)
		diagnostic := result.Files[0].Diagnostics[index]
		if diagnostic.Range.Start != start || diagnostic.Range.End != start + len(want) {
			t.Fatalf("HTTP response body io diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestHTTPResponseBodyUsedAfterCloseConsumesProjectCloseContract(t *testing.T) {
	t.Parallel()

	input := `package sample
import (
	"io"
	"net/http"
)
func finish(io.ReadCloser) {}
func run() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	finish(response.Body)
	_, err = response.Body.Read(nil)
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
symbol = "example.com/httpresponsebodystate.finish"
closes = [0]
`,
				),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := runHTTPResponseBodyUsedAfterCloseWithContracts(t, input, "go1.25", set)
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 1 {
		t.Fatalf("contract HTTP response body state result = %#v", result)
	}
	want := "response.Body.Read(nil)"
	start := strings.Index(input, want)
	diagnostic := result.Files[0].Diagnostics[0]
	if diagnostic.RuleID != "http-response-body-used-after-close" ||
		diagnostic.Range.Start != start ||
		diagnostic.Range.End != start + len(want) {
		t.Fatalf("contract HTTP response body diagnostic = %#v", diagnostic)
	}
}

func TestHTTPResponseBodyUsedAfterCloseMetadataAndEligibility(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("http-response-body-used-after-close")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		!metadata.RequiresEffectFacts ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf(
			"http-response-body-used-after-close metadata = %#v, found = %v",
			metadata,
			found,
		)
	}

	suppressed := runHTTPResponseBodyUsedAfterClose(
		t,
		`package sample
import "net/http"
func run() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.Body.Close()
	//glippy:ignore http-response-body-used-after-close -- compatibility probe
	_, err = response.Body.Read(nil)
	return err
}
`,
		"go1.25",
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 {
		t.Fatalf("suppressed HTTP response body state result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
import "net/http"
func run() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); _, err = response.Body.Read(nil); return err }
`,
			"type-error": `package sample
import "net/http"
func run() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.Body.Close(); undefined(); _, err = response.Body.Read(nil); return err }
`,
		} {
		result := runHTTPResponseBodyUsedAfterClose(t, input, "go1.25")
		if len(result.Files) != 1 ||
			len(result.Files[0].Diagnostics) != 0 ||
			len(result.Files[0].Suppressed) != 0 {
			t.Fatalf("%s HTTP response body state result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf("type-error result has no load diagnostics: %#v", result)
		}
	}

	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"http-response-body-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 HTTP response body state selection = %#v", older)
	}
}

func BenchmarkHTTPResponseBodyUsedAfterClosePackageAnalysis(b *testing.B) {
	var input strings.Builder
	input.WriteString("package sample\nimport \"net/http\"\n")
	for index := range 100 {
		fmt.Fprintf(
			&input,
			"func run%d() error { response, err := http.Get(\"https://example.com\"); if err != nil { return err }; _ = response.Body.Close(); _, err = response.Body.Read(nil); return err }\n",
			index,
		)
	}
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/httpresponsebodystatebenchmark\n\ngo 1.25.0\n",
	)
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
				"http-response-body-used-after-close": rules.SeverityWarn,
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

func runHTTPResponseBodyUsedAfterClose(
	t testing.TB,
	input string,
	goVersion string,
) analysis.PackageResult {
	t.Helper()
	return runHTTPResponseBodyUsedAfterCloseWithContracts(t, input, goVersion, contracts.Set{})
}

func runHTTPResponseBodyUsedAfterCloseWithContracts(
	t testing.TB,
	input string,
	goVersion string,
	contractSet contracts.Set,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/httpresponsebodystate\n\ngo 1.25.0\n",
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
				"http-response-body-used-after-close": rules.SeverityWarn,
			},
			SourceGoVersion: goVersion,
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
