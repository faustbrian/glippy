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

func TestHTTPResponseBodyNotClosedReportsOpenNormalReturns(t *testing.T) {
	t.Parallel()

	input := `package sample

import (
	"errors"
	"io"
	"net/http"
	"net/url"
)

func missing(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil { return err }
	_ = response.StatusCode
	return nil
}
func earlyStatus() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	if response.StatusCode != http.StatusOK { return errors.New("status") }
	defer response.Body.Close()
	return nil
}

func readFailure() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_, err = io.ReadAll(response.Body)
	if err != nil { return err }
	return response.Body.Close()
}

func overwritten(client *http.Client) error {
	response, err := client.Get("https://example.com/first")
	if err != nil { return err }
	response, err = client.PostForm("https://example.com/second", url.Values{})
	if err != nil { return err }
	defer response.Body.Close()
	return nil
}

func readWithoutClose() error {
	response, err := http.Post("https://example.com", "text/plain", nil)
	if err != nil { return err }
	_, _ = io.ReadAll(response.Body)
	return nil
}

func deferred() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	defer response.Body.Close()
	return nil
}

func completedBranches(closeNow bool) error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	if closeNow { return response.Body.Close() }
	defer response.Body.Close()
	return nil
}

func transferResponse() (*http.Response, error) {
	response, err := http.Get("https://example.com")
	if err != nil { return nil, err }
	return response, nil
}

func passResponse() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	takeResponse(response)
	return nil
}

func transferBody() (io.ReadCloser, error) {
	response, err := http.Get("https://example.com")
	if err != nil { return nil, err }
	return response.Body, nil
}

func passBody() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	takeBody(response.Body)
	return nil
}

func passBodyVariadic() error {
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	takeBodies(response.Body)
	return nil
}

func takeResponse(*http.Response) {}
func takeBody(io.ReadCloser) {}
func takeBodies(...io.ReadCloser) {}
`
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/httpresponsebody\n\ngo 1.25.0\n",
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
				"http-response-body-not-closed": rules.SeverityWarn,
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
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 7 {
		t.Fatalf("http-response-body-not-closed result = %#v", result)
	}
	expectedFunctions := []string{
		"func missing(",
		"func earlyStatus()",
		"func readFailure()",
		"func overwritten(",
		"func readWithoutClose()",
		"func passBody()",
		"func passBodyVariadic()",
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		functionStart := strings.Index(input, expectedFunctions[index])
		if functionStart < 0 {
			t.Fatalf("missing function %d", index)
		}
		relative := strings.Index(input[functionStart:], "response, err")
		if relative < 0 {
			t.Fatalf("missing acquisition %d", index)
		}
		start := functionStart + relative
		if diagnostic.RuleID != "http-response-body-not-closed" ||
			diagnostic.Range.Start != start ||
			diagnostic.Range.End != start + len("response") ||
			!strings.Contains(diagnostic.Message, "body is not closed") ||
			len(diagnostic.Fixes) != 0 {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestHTTPResponseBodyNotClosedRecognizesSupportedNetHTTPCalls(t *testing.T) {
	t.Parallel()

	input := `package sample
import (
	"net/http"
	"net/url"
)
func packageGet() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.StatusCode; return nil }
func packageHead() error { response, err := http.Head("https://example.com"); if err != nil { return err }; _ = response.StatusCode; return nil }
func packagePost() error { response, err := http.Post("https://example.com", "text/plain", nil); if err != nil { return err }; _ = response.StatusCode; return nil }
func packagePostForm() error { response, err := http.PostForm("https://example.com", url.Values{}); if err != nil { return err }; _ = response.StatusCode; return nil }
func clientDo(client *http.Client, request *http.Request) error { response, err := client.Do(request); if err != nil { return err }; _ = response.StatusCode; return nil }
func clientGet(client *http.Client) error { response, err := client.Get("https://example.com"); if err != nil { return err }; _ = response.StatusCode; return nil }
func clientHead(client *http.Client) error { response, err := client.Head("https://example.com"); if err != nil { return err }; _ = response.StatusCode; return nil }
func clientPost(client *http.Client) error { response, err := client.Post("https://example.com", "text/plain", nil); if err != nil { return err }; _ = response.StatusCode; return nil }
func clientPostForm(client *http.Client) error { response, err := client.PostForm("https://example.com", url.Values{}); if err != nil { return err }; _ = response.StatusCode; return nil }
type wrapper struct{}
func (*wrapper) Get(string) (*http.Response, error) { return nil, nil }
func lookalike(client *wrapper) error { response, err := client.Get("https://example.com"); if err != nil { return err }; _ = response.StatusCode; return nil }
func noncanonicalGuard() error { response, err := http.Get("https://example.com"); if err == nil { defer response.Body.Close(); return nil }; return err }
`
	result := runHTTPResponseBodyNotClosed(t, input, "go1.25")
	if len(result.Files) != 1 || len(result.Files[0].Diagnostics) != 9 {
		t.Fatalf("supported net/http calls result = %#v", result)
	}
	for index, diagnostic := range result.Files[0].Diagnostics {
		if diagnostic.RuleID != "http-response-body-not-closed" ||
			diagnostic.Range.End - diagnostic.Range.Start != len("response") {
			t.Fatalf("diagnostic %d = %#v", index, diagnostic)
		}
	}
}

func TestHTTPResponseBodyNotClosedMetadataAndEligibility(t *testing.T) {
	t.Parallel()

	registry, err := rulecatalog.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	metadata, found := registry.Metadata("http-response-body-not-closed")
	if !found ||
		metadata.DefaultSeverity != rules.SeverityWarn ||
		!reflect.DeepEqual(metadata.Presets, []rules.Preset{rules.PresetSuspicious}) ||
		metadata.Requirement != rules.RequireControlFlow ||
		len(metadata.NodeInterests) != 0 ||
		metadata.RunOnGenerated ||
		metadata.RunDespiteTypeErrors ||
		len(metadata.Fixes) != 0 ||
		metadata.MinimumGoVersion != "1.25" {
		t.Fatalf(
			"http-response-body-not-closed metadata = %#v, found = %v",
			metadata,
			found,
		)
	}

	suppressed := runHTTPResponseBodyNotClosed(
		t,
		`package sample
import "net/http"
func run() error {
	//glippy:ignore http-response-body-not-closed -- caller owns this response
	response, err := http.Get("https://example.com")
	if err != nil { return err }
	_ = response.StatusCode
	return nil
}
`,
		"go1.25",
	)
	if len(suppressed.Files) != 1 ||
		len(suppressed.Files[0].Diagnostics) != 0 ||
		len(suppressed.Files[0].Suppressed) != 1 ||
		suppressed.Files[0].Suppressed[0].Diagnostic.RuleID !=
			"http-response-body-not-closed" ||
		suppressed.Files[0].Suppressed[0].Diagnostic.Severity != rules.SeverityError {
		t.Fatalf("suppressed result = %#v", suppressed)
	}

	for name, input := range
		map[string]string{
			"generated": `// Code generated by test. DO NOT EDIT.
package sample
import "net/http"
func run() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.StatusCode; return nil }
`,
			"type-error": `package sample
import "net/http"
func run() error { response, err := http.Get("https://example.com"); if err != nil { return err }; _ = response.StatusCode; undefined(); return nil }
`,
		} {
		result := runHTTPResponseBodyNotClosed(t, input, "go1.25")
		if len(result.Files) != 1 ||
			len(result.Files[0].Diagnostics) != 0 ||
			len(result.Files[0].Suppressed) != 0 {
			t.Fatalf("%s result = %#v", name, result)
		}
		if name == "type-error" && len(result.LoadDiagnostics) == 0 {
			t.Fatalf("type-error result has no load diagnostics: %#v", result)
		}
	}

	older, err := registry.ResolveOptions(
		rules.ResolveOptions{
			Presets: []rules.Preset{},
			Overrides: map[string]rules.Severity{
				"http-response-body-not-closed": rules.SeverityError,
			},
			SourceGoVersion: "go1.24",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 {
		t.Fatalf("go1.24 result = %#v", older)
	}
}

func BenchmarkHTTPResponseBodyNotClosedPackageAnalysis(b *testing.B) {
	var input strings.Builder
	input.WriteString("package sample\nimport \"net/http\"\n")
	for index := 0; index < 100; index++ {
		fmt.Fprintf(
			&input,
			"func run%d() error { response, err := http.Get(\"https://example.com\"); if err != nil { return err }; _ = response.StatusCode; return nil }\n",
			index,
		)
	}
	root := b.TempDir()
	writeFixture(
		b,
		filepath.Join(root, "go.mod"),
		"module example.com/httpresponsebodybenchmark\n\ngo 1.25.0\n",
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
				"http-response-body-not-closed": rules.SeverityWarn,
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

func runHTTPResponseBodyNotClosed(
	t *testing.T,
	input string,
	goVersion string,
) analysis.PackageResult {
	t.Helper()
	root := t.TempDir()
	writeFixture(
		t,
		filepath.Join(root, "go.mod"),
		"module example.com/httpresponsebodypolicy\n\ngo 1.25.0\n",
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
				"http-response-body-not-closed": rules.SeverityError,
			},
			SourceGoVersion: goVersion,
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
