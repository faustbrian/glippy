package benchmarks_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPeakLatencySelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arguments []string
		want string
		wantError string
	}{
		{
			name: "accepts one shared-runner outlier",
			arguments: []string{
				"formatter-check",
				"120",
				"92.050",
				"106.320",
				"87.910",
				"90.340",
				"167.530",
			},
			want: "106.320\n",
		},
		{
			name: "rejects sustained regression",
			arguments: []string{
				"formatter-check",
				"120",
				"100.000",
				"121.000",
				"124.000",
				"130.000",
				"180.000",
			},
			wantError: "formatter-check sustained elapsed-time budget exceeded: " +
				"130.000 seconds > 120 seconds",
		},
		{
			name: "rejects one extreme outlier",
			arguments: []string{
				"formatter-check",
				"120",
				"90.000",
				"91.000",
				"92.000",
				"93.000",
				"241.000",
			},
			wantError: "formatter-check hard elapsed-time budget exceeded: " +
				"241.000 seconds > 240.000 seconds",
		},
		{
			name: "enforces one-sample campaigns",
			arguments: []string{"typed-lint", "15", "15.500"},
			wantError: "typed-lint sustained elapsed-time budget exceeded: " +
				"15.500 seconds > 15 seconds",
		},
		{
			name: "rejects malformed evidence",
			arguments: []string{"formatter-check", "120", "not-seconds"},
			wantError: "latency samples must be non-negative decimal seconds",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				command := exec.Command(
					"sh",
					append(
						[]string{"peak-latency-select.sh"},
						test.arguments...,
					)...,
				)
				output, err := command.CombinedOutput()
				if test.wantError == "" {
					if err != nil {
						t.Fatalf(
							"peak-latency-select.sh error = %v, output = %q",
							err,
							output,
						)
					}
					if string(output) != test.want {
						t.Fatalf(
							"peak-latency-select.sh output = %q, want %q",
							output,
							test.want,
						)
					}
					return
				}
				if err == nil {
					t.Fatalf(
						"peak-latency-select.sh succeeded, output = %q",
						output,
					)
				}
				if !strings.Contains(string(output), test.wantError) {
					t.Fatalf(
						"peak-latency-select.sh output = %q, want %q",
						output,
						test.wantError,
					)
				}
			},
		)
	}
}

func TestPeakLatencySelectionUsesPortableDecimalOutput(t *testing.T) {
	localeOutput, err := exec.Command("locale", "-a").Output()
	if err != nil {
		t.Skipf("list locales: %v", err)
	}

	var decimalCommaLocale string
	for _, available := range strings.Fields(string(localeOutput)) {
		for _, candidate := range
			[]string{"de_DE.UTF-8", "de_DE.utf8", "fr_FR.UTF-8", "fr_FR.utf8"} {
			if strings.EqualFold(available, candidate) {
				decimalCommaLocale = available
				break
			}
		}
		if decimalCommaLocale != "" {
			break
		}
	}
	if decimalCommaLocale == "" {
		t.Skip("no decimal-comma locale is available")
	}

	command := exec.Command(
		"sh",
		"peak-latency-select.sh",
		"formatter-check",
		"120",
		"92.050",
		"106.320",
		"87.910",
		"90.340",
		"167.530",
	)
	command.Env = append(os.Environ(), "LC_ALL=" + decimalCommaLocale)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("peak-latency-select.sh error = %v, output = %q", err, output)
	}
	if string(output) != "106.320\n" {
		t.Fatalf("peak-latency-select.sh output = %q, want %q", output, "106.320\n")
	}
}
