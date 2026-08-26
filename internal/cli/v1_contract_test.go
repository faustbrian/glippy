package cli

import (
	"bytes"
	"os"
	"path/filepath"
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
