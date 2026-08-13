package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	glippyreport "github.com/faustbrian/glippy/internal/report"
)

func TestRunExposesAndBaselinesVetCompatibilityPack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	writeCLIFixture(t, filepath.Join(root, "go.mod"), "module example.com/vetpackcli\n\ngo 1.25.0\n")
	writeCLIFixture(t, path, vetCompatibilityCLIInput)
	ruleIDs := []string{
		"append-no-values",
		"deferred-time-since",
		"invalid-slog-arguments",
		"invalid-struct-tag",
		"invalid-unmarshal-target",
		"nil-function-comparison",
		"oversized-shift",
		"printf-arguments",
		"standard-method-signature",
		"suspicious-string-conversion",
		"testing-goroutine-call",
		"unreachable-code",
		"unsafe-host-port",
		"unused-result",
		"waitgroup-misuse",
	}
	var configuration strings.Builder
	configuration.WriteString("version = 1\n[lint]\npresets = []\n[lint.rules]\n")
	for _, ruleID := range ruleIDs {
		configuration.WriteString(ruleID + " = \"warn\"\n")
	}
	writeCLIFixture(t, filepath.Join(root, ".glippy.toml"), configuration.String())

	runJSON := func() []byte {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{"lint", "--reporter=json", path},
			strings.NewReader(""),
			&stdout,
			&stderr,
		)
		if exitCode != ExitFindings || stderr.Len() != 0 {
			t.Fatalf("Run(lint vet compatibility) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
		}
		return bytes.Clone(stdout.Bytes())
	}
	firstJSON := runJSON()
	secondJSON := runJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("vet compatibility JSON output is not deterministic")
	}
	var result glippyreport.LintResult
	if err := json.Unmarshal(firstJSON, &result); err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(result.Diagnostics))
	for index, diagnostic := range result.Diagnostics {
		got[index] = diagnostic.RuleID
	}
	slices.Sort(got)
	if !reflect.DeepEqual(got, ruleIDs) {
		t.Fatalf("vet compatibility CLI diagnostics = %q, want %q", got, ruleIDs)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	baselinePath := filepath.Join(root, ".glippy-baseline.json")
	exitCode := Run(
		[]string{"lint", "--generate-baseline=.glippy-baseline.json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exitCode != ExitSuccess ||
		stdout.String() != "glippy lint: wrote baseline "+baselinePath+" (15 diagnostics)\n" ||
		stderr.Len() != 0 {
		t.Fatalf("Run(baseline vet compatibility) = exit %d, stdout %q, stderr %q", exitCode, stdout.String(), stderr.String())
	}
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ruleID := range ruleIDs {
		if !bytes.Contains(baseline, []byte(`"rule_id": "`+ruleID+`"`)) {
			t.Fatalf("vet compatibility baseline omits %s: %q", ruleID, baseline)
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
	if exitCode != ExitSuccess || stderr.Len() != 0 || !bytes.Equal(baseline, secondBaseline) {
		t.Fatalf("second vet compatibility baseline = exit %d, stderr %q, equal %t", exitCode, stderr.String(), bytes.Equal(baseline, secondBaseline))
	}

	for _, ruleID := range ruleIDs {
		stdout.Reset()
		stderr.Reset()
		exitCode = Run([]string{"explain", ruleID}, strings.NewReader(""), &stdout, &stderr)
		if exitCode != ExitSuccess ||
			stderr.Len() != 0 ||
			!strings.HasPrefix(stdout.String(), ruleID+"\n") ||
			!strings.Contains(stdout.String(), "analysis tier: types\n") {
			t.Fatalf("Run(explain %s) = exit %d, stdout %q, stderr %q", ruleID, exitCode, stdout.String(), stderr.String())
		}
	}
}

func TestRunAppliesUnambiguousVetCompatibilitySuggestions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ruleID string
		input  string
		want   string
	}{
		{
			name:   "printf format insertion",
			ruleID: "printf-arguments",
			input:  "package sample\n\nimport \"fmt\"\n\nfunc print(format string) {\n\tfmt.Printf(format)\n}\n",
			want:   "package sample\n\nimport \"fmt\"\n\nfunc print(format string) {\n\tfmt.Printf(\"%s\", format)\n}\n",
		},
		{
			name:   "unreachable removal",
			ruleID: "unreachable-code",
			input:  "package sample\n\nimport \"fmt\"\n\nfunc run() {\n\t_ = fmt.Sprint(\"kept\")\n\treturn\n\tfmt.Println(\"unreachable\")\n}\n",
			want:   "package sample\n\nimport \"fmt\"\n\nfunc run() {\n\t_ = fmt.Sprint(\"kept\")\n\treturn\n}\n",
		},
		{
			name:   "IPv6-safe address",
			ruleID: "unsafe-host-port",
			input:  "package sample\n\nimport (\n\t\"fmt\"\n\t\"net\"\n)\n\nfunc dial(host string) {\n\t_, _ = net.Dial(\"tcp\", fmt.Sprintf(\"%s:%d\", host, 80))\n\t_ = fmt.Sprintf(\"%s\", host)\n}\n",
			want:   "package sample\n\nimport (\n\t\"fmt\"\n\t\"net\"\n)\n\nfunc dial(host string) {\n\t_, _ = net.Dial(\"tcp\", net.JoinHostPort(host, \"80\"))\n\t_ = fmt.Sprintf(\"%s\", host)\n}\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "sample.go")
			writeCLIFixture(t, filepath.Join(root, "go.mod"), "module example.com/vetpackfix\n\ngo 1.25.0\n")
			writeCLIFixture(t, path, test.input)
			writeCLIFixture(
				t,
				filepath.Join(root, ".glippy.toml"),
				"version = 1\n[lint]\npresets = []\n[lint.rules]\n"+test.ruleID+" = \"warn\"\n",
			)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(
				[]string{"lint", "--fix-suggestions", path},
				strings.NewReader(""),
				&stdout,
				&stderr,
			)
			fixed, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 || string(fixed) != test.want {
				t.Fatalf("Run(fix %s) = exit %d, stdout %q, stderr %q, source %q", test.ruleID, exitCode, stdout.String(), stderr.String(), fixed)
			}

			stdout.Reset()
			stderr.Reset()
			exitCode = Run(
				[]string{"lint", "--fix-suggestions", path},
				strings.NewReader(""),
				&stdout,
				&stderr,
			)
			second, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 || !bytes.Equal(second, fixed) {
				t.Fatalf("second fix %s = exit %d, stdout %q, stderr %q, source %q", test.ruleID, exitCode, stdout.String(), stderr.String(), second)
			}
		})
	}
}

func writeCLIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const vetCompatibilityCLIInput = `package sample

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

type payload struct {
	Value string ` + "`json:\"value`" + `
}

type writer struct{}

func (writer) WriteTo(io.Writer) error { return nil }

func target() {}

func defects(data []byte, value uint8, number int, items []int, host string, start time.Time, t *testing.T) {
	var decoded payload
	json.Unmarshal(data, decoded)
	fmt.Printf("%d", "text")
	var group sync.WaitGroup
	go func() {
		group.Add(1)
		defer group.Done()
	}()
	group.Wait()
	go func() { t.Fatal("worker failed") }()
	_ = value << 8
	_ = string(number)
	if target == nil {}
	_ = append(items)
	slog.Info("message", "key")
	fmt.Sprintf("%s", "discarded")
	_, _ = net.Dial("tcp", fmt.Sprintf("%s:%d", host, 80))
	defer fmt.Println(time.Since(start))
	return
	fmt.Println("unreachable")
}
`
