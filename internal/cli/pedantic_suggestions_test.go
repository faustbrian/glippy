package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAppliesPedanticSuggestionsAndReachesFixedPoint(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/pedanticsuggestions\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			`version = 1
[lint]
presets = []
[lint.rules]
needless-blank-identifier = "warn"
time-since = "warn"
time-until = "warn"
unnecessary-conversion = "warn"
unnecessary-sprintf = "warn"
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(
		path,
		[]byte(
			`package sample

import (
	"fmt"
	"time"
)

type Name string

func run(text string, name Name, values []int, start, deadline time.Time) {
	fmt.Println(text)
	_ = string(text)
	_ = fmt.Sprintf("%s", name)
	for _, _ = range values {}
	_ = time.Now().Sub(start)
	_ = deadline.Sub(time.Now())
}
`,
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exit != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"first suggestion run = exit %d, stdout %q, stderr %q",
			exit,
			stdout.String(),
			stderr.String(),
		)
	}
	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"_ = text",
		"_ = string(name)",
		"for range values",
		"_ = time.Since(start)",
		"_ = time.Until(deadline)",
	}
	for _, text := range want {
		if !bytes.Contains(formatted, []byte(text)) {
			t.Fatalf("fixed source omits %q: %s", text, formatted)
		}
	}
	for _, old := range
		[]string{
			"string(text)",
			"fmt.Sprintf",
			"_, _ = range",
			"time.Now().Sub",
			"Sub(time.Now())",
		} {
		if bytes.Contains(formatted, []byte(old)) {
			t.Fatalf("fixed source retains %q: %s", old, formatted)
		}
	}

	stdout.Reset()
	stderr.Reset()
	exit = Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exit != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf(
			"second suggestion run = exit %d, stdout %q, stderr %q",
			exit,
			stdout.String(),
			stderr.String(),
		)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(formatted, second) {
		t.Fatalf("second suggestion run changed source:\n%s", second)
	}
}

func TestRunAppliesPedanticSuggestionAndRemovesItsUnusedImport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/pedanticvalidation\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npresets = []\n[lint.rules]\nunnecessary-sprintf = \"warn\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		"package sample\n\nimport \"fmt\"\n\nfunc run(text string) string { return fmt.Sprintf(\"%s\", text) }\n",
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exit != ExitSuccess ||
		stderr.Len() != 0 ||
		stdout.Len() != 0 ||
		bytes.Contains(got, []byte(`import "fmt"`)) ||
		bytes.Contains(got, []byte("fmt.Sprintf")) ||
		!bytes.Contains(got, []byte("return text")) {
		t.Fatalf(
			"import-aware suggestion = exit %d, stdout %q, stderr %q, source %q",
			exit,
			stdout.String(),
			stderr.String(),
			got,
		)
	}
}

func TestRunAppliesUnnecessaryFormatSuggestionWhenImportRemains(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/unnecessaryformatfix\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npresets = []\n[lint.rules]\nunnecessary-format = \"warn\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		"package sample\n\nimport \"fmt\"\n\nfunc run() string { fmt.Println(\"keep import\"); return fmt.Sprintf(\"literal\") }\n",
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if exit != ExitSuccess || stderr.Len() != 0 {
		t.Fatalf(
			"first unnecessary-format suggestion run = exit %d, stdout %q, stderr %q",
			exit,
			stdout.String(),
			stderr.String(),
		)
	}
	fixed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(fixed, []byte(`return "literal"`)) ||
		bytes.Contains(fixed, []byte("fmt.Sprintf")) ||
		!bytes.Contains(fixed, []byte("fmt.Println")) {
		t.Fatalf("fixed source = %s", fixed)
	}

	stdout.Reset()
	stderr.Reset()
	exit = Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exit != ExitSuccess ||
		stdout.Len() != 0 ||
		stderr.Len() != 0 ||
		!bytes.Equal(fixed, second) {
		t.Fatalf(
			"second unnecessary-format suggestion run = exit %d, stdout %q, stderr %q, source %q",
			exit,
			stdout.String(),
			stderr.String(),
			second,
		)
	}
}

func TestRunAppliesUnnecessaryFormatSuggestionAndRemovesItsUnusedImport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/unnecessaryformatvalidation\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npresets = []\n[lint.rules]\nunnecessary-format = \"warn\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		"package sample\n\nimport \"fmt\"\n\nfunc run() string { return fmt.Sprintf(\"literal\") }\n",
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run(
		[]string{"lint", "--fix-suggestions", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if exit != ExitSuccess ||
		stderr.Len() != 0 ||
		stdout.Len() != 0 ||
		bytes.Contains(got, []byte(`import "fmt"`)) ||
		bytes.Contains(got, []byte("fmt.Sprintf")) ||
		!bytes.Contains(got, []byte(`return "literal"`)) {
		t.Fatalf(
			"import-aware unnecessary-format suggestion = exit %d, stdout %q, stderr %q, source %q",
			exit,
			stdout.String(),
			stderr.String(),
			got,
		)
	}
}

func TestRunImportAwareSuggestionPreservesRemainingImportComments(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/importcomments\n\ngo 1.25.0\n"),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".glippy.toml"),
		[]byte(
			"version = 1\n[lint]\npresets = []\n[lint.rules]\nunnecessary-sprintf = \"warn\"\n",
		),
		0o600,
	);
		err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.go")
	input := []byte(
		`package sample

import (
	// retained dependency
	"os"
	"fmt" // formatting only
)

func run(text string) string {
	_ = os.ErrInvalid
	return fmt.Sprintf("%s", text)
}
`,
	)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Run(
		[]string{"lint", "--fix-suggestions", "--reporter=json", path},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Summary struct {
			ImportChanges int `json:"import_changes"`
		} `json:"summary"`
		ImportChanges []struct {
			Action string `json:"action"`
			ImportPath string `json:"import_path"`
			ImportName string `json:"import_name"`
		} `json:"import_changes"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if exit != ExitSuccess ||
		stderr.Len() != 0 ||
		plan.Summary.ImportChanges != 1 ||
		len(plan.ImportChanges) != 1 ||
		plan.ImportChanges[0].Action != "remove" ||
		plan.ImportChanges[0].ImportPath != "fmt" ||
		plan.ImportChanges[0].ImportName != "fmt" ||
		bytes.Contains(got, []byte(`"fmt"`)) ||
		bytes.Contains(got, []byte("formatting only")) ||
		!bytes.Contains(got, []byte("retained dependency")) ||
		!bytes.Contains(got, []byte(`"os"`)) ||
		!bytes.Contains(got, []byte("return text")) {
		t.Fatalf(
			"comment-preserving import fix = exit %d, stdout %q, stderr %q, source %q",
			exit,
			stdout.String(),
			stderr.String(),
			got,
		)
	}
}
