package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	glippyreport "github.com/faustbrian/glippy/internal/report"
)

func TestRunExposesAndBaselinesPedanticCatalog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/pedanticcli\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	input := `package sample

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Named demonstrates receiver naming.
type Named struct{}

// First uses the canonical receiver name.
func (n *Named) First() {}
// Second uses the canonical receiver name.
func (n *Named) Second() {}
// Third deliberately uses another receiver name.
func (named *Named) Third() {}

// Form demonstrates receiver forms.
type Form struct{}

// First uses a pointer receiver.
func (f *Form) First() {}
// Second uses a pointer receiver.
func (f *Form) Second() {}
// Third deliberately uses a value receiver.
func (f Form) Third() {}

func run(text string) string {
	values := []string{text}
	other := "value"
	low, high := 1, 2
	var buffer bytes.Buffer
	var explicit int = len(text)
	if low < high { low = high }
	if text == "placeholder" {}
	_ = string(text)
	_ = fmt.Sprintf("%s", text)
	if text == "" { return "" } else { return text }
	for _, _ = range values {}
	_ = func(value string) string { return strings.TrimSpace(value) }
	if values != nil && len(values) > 0 { /* condition example */ }
	_ = time.Now().Sub(time.Time{})
	_ = time.Time{}.Sub(time.Now())
	_ = string(buffer.Bytes())
	_ = fmt.Sprintf("literal")
	_ = strings.ToLower(text) == strings.ToLower(other)
	return text + string(rune(explicit + low))
}
`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte("version = 1\n[lint]\npresets = [\"pedantic\"]\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	want := []string{
		"inconsistent-receiver-name",
		"mixed-receiver-type",
		"redundant-type-declaration",
		"manual-min-max",
		"empty-branch",
		"unnecessary-conversion",
		"unnecessary-sprintf",
		"redundant-else",
		"needless-blank-identifier",
		"redundant-closure",
		"redundant-nil-check",
		"time-since",
		"time-until",
		"buffer-string-conversion",
		"unnecessary-format",
		"inefficient-string-comparison",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		[]string{"lint", "--reporter=json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitFindings || stderr.Len() != 0 {
		t.Fatalf(
			"Run(lint pedantic catalog) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(result.Diagnostics))
	for index, diagnostic := range result.Diagnostics {
		got[index] = diagnostic.RuleID
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pedantic diagnostics = %q, want %q", got, want)
	}

	stdout.Reset()
	baselinePath := filepath.Join(root, ".glippy-baseline.json")
	exitCode = Run(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess ||
		stdout.String() !=
			"glippy lint: wrote baseline " + baselinePath + " (16 diagnostics)\n" ||
		stderr.Len() != 0 {
		t.Fatalf(
			"Run(baseline pedantic catalog) = exit %d, stdout %q, stderr %q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ruleID := range want {
		if !bytes.Contains(baseline, []byte(`"rule_id": "` + ruleID + `"`)) {
			t.Fatalf("pedantic baseline omits %s: %q", ruleID, baseline)
		}
	}
	stdout.Reset()
	stderr.Reset()
	exitCode = Run(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	secondBaseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != ExitSuccess || stderr.Len() != 0 || !bytes.Equal(secondBaseline, baseline) {
		t.Fatalf(
			"second pedantic baseline = exit %d, stderr %q, equal %t",
			exitCode,
			stderr.String(),
			bytes.Equal(secondBaseline, baseline),
		)
	}

	for _, ruleID := range want {
		stdout.Reset()
		stderr.Reset()
		exitCode = Run([]string{"explain", ruleID}, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitSuccess ||
			stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), ruleID + "\n") ||
			!strings.Contains(stdout.String(), "presets: pedantic") {
			t.Fatalf(
				"Run(explain %s) = exit %d, stdout %q, stderr %q",
				ruleID,
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}
