package benchmarks_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPeakRSSSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		reported string
		sampled string
		want string
		wantErr string
	}{
		{
			name: "uses reported peak when short process has no tree sample",
			reported: "4096",
			sampled: "missed",
			want: "4096\n",
		},
		{
			name: "uses larger process tree peak",
			reported: "4096",
			sampled: "8192",
			want: "8192\n",
		},
		{
			name: "keeps larger reported peak",
			reported: "8192",
			sampled: "4096",
			want: "8192\n",
		},
		{
			name: "rejects malformed reported peak",
			reported: "invalid",
			wantErr: "reported peak RSS must be a positive integer",
		},
		{
			name: "rejects zero reported peak",
			reported: "0",
			wantErr: "reported peak RSS must be a positive integer",
		},
		{
			name: "rejects zero reported peak with leading zero",
			reported: "00",
			sampled: "missed",
			wantErr: "reported peak RSS must be a positive integer",
		},
		{
			name: "rejects malformed process tree peak",
			reported: "4096",
			sampled: "invalid",
			wantErr: "process-tree peak RSS must be a positive integer",
		},
		{
			name: "rejects zero process tree peak",
			reported: "4096",
			sampled: "0",
			wantErr: "process-tree peak RSS must be a positive integer",
		},
		{
			name: "rejects zero process tree peak with leading zero",
			reported: "4096",
			sampled: "00",
			wantErr: "process-tree peak RSS must be a positive integer",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				command := exec.Command(
					"sh",
					"peak-rss-select.sh",
					test.reported,
					test.sampled,
				)
				command.Dir = "."
				output, err := command.CombinedOutput()
				if test.wantErr == "" {
					if err != nil {
						t.Fatalf(
							"peak-rss-select.sh error = %v, output = %q",
							err,
							output,
						)
					}
					if string(output) != test.want {
						t.Fatalf(
							"peak-rss-select.sh output = %q, want %q",
							output,
							test.want,
						)
					}
					return
				}
				if err == nil {
					t.Fatalf(
						"peak-rss-select.sh succeeded, output = %q",
						output,
					)
				}
				if !strings.Contains(string(output), test.wantErr) {
					t.Fatalf(
						"peak-rss-select.sh output = %q, want %q",
						output,
						test.wantErr,
					)
				}
			},
		)
	}
}
