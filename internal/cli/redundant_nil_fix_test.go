package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunLintAppliesRedundantNilCheckSafeFixIdempotently(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	configurationPath := filepath.Join(root, ".glippy.toml")
	write := func(path string, contents []byte) {
		t.Helper()
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/redundantnilfix\n\ngo 1.26.0\n"),
	)
	write(
		configurationPath,
		[]byte(
			"version = 1\n[lint]\npresets = []\n[lint.rules]\nredundant-nil-check = \"warn\"\n",
		),
	)
	write(
		path,
		[]byte(
			"package sample\nfunc run(values []string) { if values != nil && len(values) > 0 { println() } }\n",
		),
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
	want := "package sample\n\nfunc run(values []string) {\n\tif len(values) > 0 {\n\t\tprintln()\n\t}\n}\n"
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
