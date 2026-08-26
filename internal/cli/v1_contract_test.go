package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV1TextContractsMatchApprovedGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "help", args: []string{"--help"}, golden: "help.txt"},
		{name: "rules", args: []string{"rules"}, golden: "rules.txt"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			want, err := os.ReadFile(filepath.Join(
				"..", "..", "testdata", "contracts", "v1", test.golden,
			))
			if err != nil {
				t.Fatal(err)
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(test.args, bytes.NewReader(nil), &stdout, &stderr)
			if exitCode != ExitSuccess || stderr.Len() != 0 {
				t.Fatalf(
					"Run(%v) exit = %d, stderr = %q",
					test.args,
					exitCode,
					stderr.String(),
				)
			}
			if !bytes.Equal(stdout.Bytes(), want) {
				t.Fatalf("Run(%v) output does not match %s", test.args, test.golden)
			}
		})
	}
}

func TestV1ConfigurationProfilesMatchApprovedGolden(t *testing.T) {
	t.Parallel()

	contractRoot := filepath.Join("..", "..", "testdata", "contracts", "v1")
	want, err := os.ReadFile(filepath.Join(contractRoot, "profiles.txt"))
	if err != nil {
		t.Fatal(err)
	}
	configurationRoot := filepath.Join(contractRoot, "config")
	absoluteRoot, err := filepath.Abs(configurationRoot)
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	for _, profile := range []string{"default", "recommended", "strict", "pedantic"} {
		got.WriteString("## " + profile + "\n")
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exitCode := Run(
			[]string{
				"config",
				"show",
				"--config",
				filepath.Join(configurationRoot, profile+".toml"),
				configurationRoot,
			},
			bytes.NewReader(nil),
			&stdout,
			&stderr,
		)
		if exitCode != ExitSuccess || stderr.Len() != 0 {
			t.Fatalf(
				"config show %s exit = %d, stderr = %q",
				profile,
				exitCode,
				stderr.String(),
			)
		}
		got.WriteString(strings.ReplaceAll(stdout.String(), absoluteRoot, "<ROOT>"))
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("curated profile output does not match profiles.txt")
	}
}
