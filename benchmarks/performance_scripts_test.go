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
	writeExecutable(
		t,
		filepath.Join(toolDirectory, "go"),
		`#!/bin/sh
set -eu
output=
arguments=$*
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		output=$2
		break
	fi
	shift
done
test -n "$output"
if [ ! -e "$GOMODCACHE/read-only" ]; then
	mkdir -p "$GOMODCACHE/read-only"
	printf '%s\n' 'module cache entry' >"$GOMODCACHE/read-only/file"
	chmod -R a-w "$GOMODCACHE"
fi
case "$arguments" in
	*./benchmarks/cmd/editor-latency*)
		cat >"$output" <<'SCRIPT'
#!/bin/sh
set -eu
budget=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--budget-ms" ]; then
		budget=$2
		break
	fi
	shift
done
test -n "$budget"
printf 'maximum_ms,%s.000\n' "$FAKE_EDITOR_MAXIMUM_MS"
if [ "$FAKE_EDITOR_MAXIMUM_MS" -gt "$budget" ]; then
	printf 'editor latency budget exceeded: %s.000 ms > %s ms\n' \
		"$FAKE_EDITOR_MAXIMUM_MS" "$budget" >&2
	exit 1
fi
SCRIPT
		;;
	*)
		printf '#!/bin/sh\nexit 0\n' >"$output"
		;;
esac
chmod +x "$output"
`,
	)

	for _, test := range
		[]struct {
			name string
			budget string
			maximumMS string
			wantError bool
			wantOutput string
		}{
			{name: "inside budget", budget: "250", maximumMS: "249"},
			{
				name: "over budget",
				budget: "250",
				maximumMS: "251",
				wantError: true,
				wantOutput: "editor latency budget exceeded",
			},
			{
				name: "invalid budget",
				budget: "0",
				maximumMS: "1",
				wantError: true,
				wantOutput: "GOX_EDITOR_LATENCY_BUDGET_MS must be a positive integer",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				command := exec.Command("sh", "editor-latency.sh")
				command.Dir = "."
				command.Env = append(
					os.Environ(),
					"PATH=" +
						toolDirectory +
						string(os.PathListSeparator) +
						os.Getenv("PATH"),
					"GOX_EDITOR_LATENCY_BUDGET_MS=" + test.budget,
					"FAKE_EDITOR_MAXIMUM_MS=" + test.maximumMS,
				)
				output, err := command.CombinedOutput()
				if (err != nil) != test.wantError {
					t.Fatalf(
						"editor-latency.sh error = %v, output = %q",
						err,
						output,
					)
				}
				if test.wantOutput != "" &&
					!strings.Contains(string(output), test.wantOutput) {
					t.Fatalf(
						"editor-latency.sh output = %q, want %q",
						output,
						test.wantOutput,
					)
				}
			},
		)
	}
}

func TestPeakRSSProbeEnforcesFormatterBudgets(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	writeExecutable(
		t,
		filepath.Join(toolDirectory, "go"),
		`#!/bin/sh
set -eu
case "$*" in
	*' mod download')
		mkdir -p "$GOMODCACHE"
		printf '%s\n' 'ready' >"$GOMODCACHE/downloaded"
		exit 0
		;;
esac
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
if [ "${1:-}" = "check" ] && [ ! -e "$GOMODCACHE/downloaded" ]; then
	exit 2
fi
sleep "${FAKE_GOX_SLEEP_SECONDS:-0}"
exit 1
SCRIPT
chmod +x "$output"
	`,
	)
	formatRoot := t.TempDir()

	for _, test := range
		[]struct {
			name string
			memoryBudget string
			latencyBudget string
			sleep string
			wantError bool
			wantOutput string
		}{
			{name: "inside budgets", memoryBudget: "999999999999", latencyBudget: "10"},
			{
				name: "memory over budget",
				memoryBudget: "1",
				latencyBudget: "10",
				wantError: true,
				wantOutput: "formatter-check peak RSS budget exceeded",
			},
			{
				name: "latency over budget",
				memoryBudget: "999999999999",
				latencyBudget: "1",
				sleep: "2",
				wantError: true,
				wantOutput: "formatter-check elapsed-time budget exceeded",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				command := exec.Command("sh", "peak-rss.sh")
				command.Dir = "."
				command.Env = append(
					os.Environ(),
					"PATH=" +
						toolDirectory +
						string(os.PathListSeparator) +
						os.Getenv("PATH"),
					"GOX_PEAK_RSS_RUNS=1",
					"GOX_PEAK_RSS_FORMAT_ROOT=" + formatRoot,
					"GOX_PEAK_RSS_FORMAT_BUDGET_BYTES=" + test.memoryBudget,
					"GOX_PEAK_RSS_FORMAT_BUDGET_SECONDS=" + test.latencyBudget,
					"FAKE_GOX_SLEEP_SECONDS=" + test.sleep,
				)
				output, err := command.CombinedOutput()
				if (err != nil) != test.wantError {
					t.Fatalf("peak-rss.sh error = %v, output = %q", err, output)
				}
				if test.wantOutput != "" &&
					!strings.Contains(string(output), test.wantOutput) {
					t.Fatalf(
						"peak-rss.sh output = %q, want %q",
						output,
						test.wantOutput,
					)
				}
			},
		)
	}
}

func TestPeakRSSProbeRejectsMismatchedReleaseEnvironmentAndCorpus(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	writeExecutable(
		t,
		filepath.Join(toolDirectory, "go"),
		`#!/bin/sh
set -eu
if [ "$#" -eq 3 ] && [ "$1" = "env" ] && [ "$2" = "GOHOSTOS" ] && [ "$3" = "GOHOSTARCH" ]; then
	printf '%s\n%s\n' "$FAKE_GO_HOST_OS" "$FAKE_GO_HOST_ARCH"
	exit 0
fi
exit 99
`,
	)
	writeExecutable(
		t,
		filepath.Join(toolDirectory, "uname"),
		`#!/bin/sh
set -eu
case "$1" in
	-s) printf '%s\n' "$FAKE_UNAME_OS" ;;
	-m) printf '%s\n' "$FAKE_UNAME_ARCH" ;;
	*) exit 99 ;;
esac
`,
	)
	writeExecutable(
		t,
		filepath.Join(toolDirectory, "git"),
		`#!/bin/sh
set -eu
case "$*" in
	*'rev-parse HEAD')
		if [ "$2" = "$FAKE_FORMAT_ROOT" ]; then
			printf '%s\n' "$FAKE_CORPUS_REVISION"
		else
			printf '%s\n' "$FAKE_GOX_REVISION"
		fi
		;;
	*'status --porcelain=v1 --untracked-files=all') ;;
	*) exit 99 ;;
esac
	`,
	)
	formatRoot := t.TempDir()
	resolvedFormatRoot, err := filepath.EvalSymlinks(formatRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range
		[]struct {
			name string
			goHostOS string
			goHostArch string
			unameOS string
			unameArch string
			corpusRevision string
			goxRevision string
			wantError string
		}{
			{
				name: "Go host mismatch",
				goHostOS: "linux",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "arm64",
				corpusRevision: "abc123",
				goxRevision: "gox123",
				wantError: "release budget requires darwin/arm64; Go host is linux/arm64",
			},
			{
				name: "kernel architecture mismatch",
				goHostOS: "darwin",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "x86_64",
				corpusRevision: "abc123",
				goxRevision: "gox123",
				wantError: "release budget requires native darwin/arm64; kernel is darwin/amd64",
			},
			{
				name: "corpus revision mismatch",
				goHostOS: "darwin",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "arm64",
				corpusRevision: "different",
				goxRevision: "gox123",
				wantError: "formatter corpus revision is different; want abc123",
			},
			{
				name: "Gox revision mismatch",
				goHostOS: "darwin",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "arm64",
				corpusRevision: "abc123",
				goxRevision: "different",
				wantError: "Gox revision is different; want gox123",
			},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				command := exec.Command("sh", "peak-rss.sh")
				command.Dir = "."
				command.Env = append(
					os.Environ(),
					"PATH=" +
						toolDirectory +
						string(os.PathListSeparator) +
						os.Getenv("PATH"),
					"GOX_PEAK_RSS_FORMAT_ROOT=" + formatRoot,
					"GOX_PEAK_RSS_FORMAT_REVISION=abc123",
					"GOX_RELEASE_GOX_REVISION=gox123",
					"GOX_RELEASE_EXPECTED_GOOS=darwin",
					"GOX_RELEASE_EXPECTED_GOARCH=arm64",
					"FAKE_GO_HOST_OS=" + test.goHostOS,
					"FAKE_GO_HOST_ARCH=" + test.goHostArch,
					"FAKE_UNAME_OS=" + test.unameOS,
					"FAKE_UNAME_ARCH=" + test.unameArch,
					"FAKE_CORPUS_REVISION=" + test.corpusRevision,
					"FAKE_GOX_REVISION=" + test.goxRevision,
					"FAKE_FORMAT_ROOT=" + resolvedFormatRoot,
				)
				output, err := command.CombinedOutput()
				if err == nil {
					t.Fatalf("peak-rss.sh succeeded, output = %q", output)
				}
				if !strings.Contains(string(output), test.wantError) {
					t.Fatalf(
						"peak-rss.sh output = %q, want %q",
						output,
						test.wantError,
					)
				}
			},
		)
	}
}

func TestPeakRSSProbeUsesPreciseElapsedTime(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	writeExecutable(
		t,
		filepath.Join(toolDirectory, "go"),
		`#!/bin/sh
set -eu
case "$*" in
	*' mod download') exit 0 ;;
esac
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		output=$2
		break
	fi
	shift
done
test -n "$output"
printf '#!/bin/sh\nexit 1\n' >"$output"
chmod +x "$output"
`,
	)
	fakeTime := filepath.Join(toolDirectory, "time")
	writeExecutable(
		t,
		fakeTime,
		`#!/bin/sh
set -eu
printf '%s real 0.000 user 0.000 sys\n' "$FAKE_TIME_ELAPSED_SECONDS" >&2
printf '%s maximum resident set size\n' "$FAKE_TIME_PEAK_BYTES" >&2
for argument in "$@"; do
	if [ "$argument" = "true" ]; then
		exit 0
	fi
done
exit 1
`,
	)

	command := exec.Command("sh", "peak-rss.sh")
	command.Dir = "."
	command.Env = append(
		os.Environ(),
		"PATH=" + toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GOX_PEAK_RSS_RUNS=1",
		"GOX_PEAK_RSS_FORMAT_ROOT=" + t.TempDir(),
		"GOX_PEAK_RSS_FORMAT_BUDGET_BYTES=2147483648",
		"GOX_PEAK_RSS_FORMAT_BUDGET_SECONDS=15",
		"GOX_TIME_COMMAND=" + fakeTime,
		"FAKE_TIME_ELAPSED_SECONDS=15.500",
		"FAKE_TIME_PEAK_BYTES=1024",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("peak-rss.sh succeeded, output = %q", output)
	}
	want := "formatter-check elapsed-time budget exceeded: 15.500 seconds > 15 seconds"
	if !strings.Contains(string(output), want) {
		t.Fatalf("peak-rss.sh output = %q, want %q", output, want)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
