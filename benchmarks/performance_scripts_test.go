package benchmarks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorLatencyProbeEnforcesMaximumRunBudget(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	writeExecutable(t, filepath.Join(toolDirectory, "go"), `#!/bin/sh
set -eu
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		output=$2
		break
	fi
	shift
done
test -n "$output"
printf '#!/bin/sh\nexit 0\n' >"$output"
chmod +x "$output"
`)
	writeExecutable(t, filepath.Join(toolDirectory, "hyperfine"), `#!/bin/sh
set -eu
csv=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--export-csv" ]; then
		csv=$2
		break
	fi
	shift
done
test -n "$csv"
printf '%s\n' 'command,mean,stddev,median,user,system,min,max' >"$csv"
printf 'fake,0,0,0,0,0,0,%s\n' "$FAKE_HYPERFINE_MAX_SECONDS" >>"$csv"
`)

	for _, test := range []struct {
		name       string
		budget     string
		maximum    string
		wantError  bool
		wantOutput string
	}{
		{name: "inside budget", budget: "250", maximum: "0.249"},
		{
			name:       "over budget",
			budget:     "250",
			maximum:    "0.251",
			wantError:  true,
			wantOutput: "editor latency budget exceeded",
		},
		{
			name:       "invalid budget",
			budget:     "0",
			maximum:    "0.001",
			wantError:  true,
			wantOutput: "GOX_EDITOR_LATENCY_BUDGET_MS must be a positive integer",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			command := exec.Command("sh", "editor-latency.sh")
			command.Dir = "."
			command.Env = append(os.Environ(),
				"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GOX_EDITOR_LATENCY_BUDGET_MS="+test.budget,
				"FAKE_HYPERFINE_MAX_SECONDS="+test.maximum,
			)
			output, err := command.CombinedOutput()
			if (err != nil) != test.wantError {
				t.Fatalf("editor-latency.sh error = %v, output = %q", err, output)
			}
			if test.wantOutput != "" && !strings.Contains(string(output), test.wantOutput) {
				t.Fatalf("editor-latency.sh output = %q, want %q", output, test.wantOutput)
			}
		})
	}
}

func TestPeakRSSProbeEnforcesFormatterBudgets(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	writeExecutable(t, filepath.Join(toolDirectory, "go"), `#!/bin/sh
set -eu
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		output=$2
		break
	fi
	shift
done
test -n "$output"
cat >"$output" <<'SCRIPT'
#!/bin/sh
sleep "${FAKE_GOX_SLEEP_SECONDS:-0}"
exit 1
SCRIPT
chmod +x "$output"
`)
	formatRoot := t.TempDir()

	for _, test := range []struct {
		name          string
		memoryBudget  string
		latencyBudget string
		sleep         string
		wantError     bool
		wantOutput    string
	}{
		{
			name:          "inside budgets",
			memoryBudget:  "999999999999",
			latencyBudget: "10",
		},
		{
			name:          "memory over budget",
			memoryBudget:  "1",
			latencyBudget: "10",
			wantError:     true,
			wantOutput:    "formatter-check peak RSS budget exceeded",
		},
		{
			name:          "latency over budget",
			memoryBudget:  "999999999999",
			latencyBudget: "1",
			sleep:         "2",
			wantError:     true,
			wantOutput:    "formatter-check elapsed-time budget exceeded",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			command := exec.Command("sh", "peak-rss.sh")
			command.Dir = "."
			command.Env = append(os.Environ(),
				"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GOX_PEAK_RSS_RUNS=1",
				"GOX_PEAK_RSS_FORMAT_ROOT="+formatRoot,
				"GOX_PEAK_RSS_FORMAT_BUDGET_BYTES="+test.memoryBudget,
				"GOX_PEAK_RSS_FORMAT_BUDGET_SECONDS="+test.latencyBudget,
				"FAKE_GOX_SLEEP_SECONDS="+test.sleep,
			)
			output, err := command.CombinedOutput()
			if (err != nil) != test.wantError {
				t.Fatalf("peak-rss.sh error = %v, output = %q", err, output)
			}
			if test.wantOutput != "" && !strings.Contains(string(output), test.wantOutput) {
				t.Fatalf("peak-rss.sh output = %q, want %q", output, test.wantOutput)
			}
		})
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
