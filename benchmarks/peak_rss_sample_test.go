package benchmarks_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPeakRSSSampleSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  string
		current string
		peak    string
		want    string
		wantErr string
	}{
		{
			name:    "continues with a larger current sample",
			status:  "0",
			current: "8192",
			peak:    "4096",
			want:    "continue 8192\n",
		},
		{
			name:    "retains the peak through zero until parent completion",
			status:  "0",
			current: "0",
			peak:    "4096",
			want:    "continue 4096\n",
		},
		{
			name:   "completes when the root leaves the snapshot",
			status: "5",
			peak:   "4096",
			want:   "complete 4096\n",
		},
		{
			name:   "records a missed tree for an unobserved short process",
			status: "5",
			peak:   "0",
			want:   "complete missed\n",
		},
		{
			name:    "waits for parent completion before a short-process fallback",
			status:  "0",
			current: "0",
			peak:    "0",
			want:    "continue 0\n",
		},
		{
			name:    "rejects a failed snapshot",
			status:  "3",
			current: "",
			peak:    "4096",
			wantErr: "process-tree RSS sampling failed",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				command := exec.Command(
					"sh",
					"peak-rss-sample.sh",
					test.status,
					test.current,
					test.peak,
				)
				command.Dir = "."
				output, err := command.CombinedOutput()
				if test.wantErr == "" {
					if err != nil {
						t.Fatalf(
							"peak-rss-sample.sh error = %v, output = %q",
							err,
							output,
						)
					}
					if string(output) != test.want {
						t.Fatalf(
							"peak-rss-sample.sh output = %q, want %q",
							output,
							test.want,
						)
					}
					return
				}
				if err == nil {
					t.Fatalf(
						"peak-rss-sample.sh succeeded, output = %q",
						output,
					)
				}
				if !strings.Contains(string(output), test.wantErr) {
					t.Fatalf(
						"peak-rss-sample.sh output = %q, want %q",
						output,
						test.wantErr,
					)
				}
			},
		)
	}
}
