package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunLintAppliesRedundantTypeSafeFixIdempotently(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	configurationPath := filepath.Join(root, ".glippy.toml")
	write := func(path string, contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "go.mod"), "module example.com/redundanttypefix\n\ngo 1.26.0\n")
	write(
		configurationPath,
		"version = 1\n[lint]\npresets = []\n[lint.rules]\nredundant-type-declaration = \"warn\"\n",
	)
	write(
		path,
		"package sample\nfunc run() { var retries int = configured(); println(retries) }\nfunc configured() int { return 3 }\n",
	)

	run := func() {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{"lint", "--fix", "--config=" + configurationPath, path},
			failingReader{},
			&stdout,
			&stderr,
		)
		if exitCode != ExitSuccess || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf(
				"Run(lint --fix) = exit %d, stdout %q, stderr %q",
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	}

	run()
	want := "package sample\n\nfunc run() {\n\tvar retries = configured()\n\tprintln(retries)\n}\n\nfunc configured() int {\n\treturn 3\n}\n"
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("fixed source = %q, error = %v", got, err)
	}
	run()
	second, err := os.ReadFile(path)
	if err != nil || string(second) != want {
		t.Fatalf("second fixed source = %q, error = %v", second, err)
	}
}
