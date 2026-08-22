//go:build glippy_runtime_probes

package benchmarks_test

import (
	"crypto/sha256"
	"fmt"
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
				wantOutput: "GLIPPY_EDITOR_LATENCY_BUDGET_MS must be a positive integer",
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
					"GLIPPY_EDITOR_LATENCY_BUDGET_MS=" + test.budget,
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
case "$output" in
	*/process-group)
		printf '%s\n' '#!/bin/sh' 'exec "$@"' >"$output"
		chmod +x "$output"
		exit 0
		;;
esac
cat >"$output" <<'SCRIPT'
#!/bin/sh
case "${1:-}" in
	fmt)
		sleep "${FAKE_GLIPPY_FORMAT_SLEEP_SECONDS:-0}"
		;;
	lint)
		if [ "${2:-}" != "-Wsuspicious" ] || [ ! -e "$GOMODCACHE/downloaded" ]; then
			exit 2
		fi
		sleep "${FAKE_GLIPPY_TYPED_SLEEP_SECONDS:-0}"
		if [ -n "${FAKE_GLIPPY_TYPED_OUTPUT:-}" ]; then
			printf '%s\n' "$FAKE_GLIPPY_TYPED_OUTPUT"
		fi
		;;
	*) exit 2 ;;
esac
exit 1
SCRIPT
chmod +x "$output"
	`,
	)
	fakeTreeRSS := filepath.Join(toolDirectory, "tree-rss")
	writeExecutable(t, fakeTreeRSS, "#!/bin/sh\nprintf '%s\\n' 2048\n")
	formatRoot := t.TempDir()
	resolvedTypedRoot, err := filepath.EvalSymlinks(formatRoot)
	if err != nil {
		t.Fatal(err)
	}
	normalizedFingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte("<TYPED_ROOT>/file.go\n")))

	for _, test := range
		[]struct {
			name string
			memoryBudget string
			latencyBudget string
			typedMemoryBudget string
			typedLatencyBudget string
			formatSleep string
			typedSleep string
			typedOutput string
			typedOutputSHA256 string
			wantError bool
			wantOutput string
		}{
			{
				name: "inside budgets",
				memoryBudget: "999999999999",
				latencyBudget: "10",
				typedMemoryBudget: "999999999999",
				typedLatencyBudget: "10",
			},
			{
				name: "memory over budget",
				memoryBudget: "1",
				latencyBudget: "10",
				typedMemoryBudget: "999999999999",
				typedLatencyBudget: "10",
				wantError: true,
				wantOutput: "formatter-check peak RSS budget exceeded",
			},
			{
				name: "latency over budget",
				memoryBudget: "999999999999",
				latencyBudget: "1",
				typedMemoryBudget: "999999999999",
				typedLatencyBudget: "10",
				formatSleep: "2",
				wantError: true,
				wantOutput: "formatter-check elapsed-time budget exceeded",
			},
			{
				name: "typed memory over budget",
				memoryBudget: "999999999999",
				latencyBudget: "10",
				typedMemoryBudget: "1",
				typedLatencyBudget: "10",
				wantError: true,
				wantOutput: "typed-lint peak RSS budget exceeded",
			},
			{
				name: "typed latency over budget",
				memoryBudget: "999999999999",
				latencyBudget: "10",
				typedMemoryBudget: "999999999999",
				typedLatencyBudget: "1",
				typedSleep: "2",
				wantError: true,
				wantOutput: "typed-lint elapsed-time budget exceeded",
			},
			{
				name: "typed diagnostic fingerprint mismatch",
				memoryBudget: "999999999999",
				latencyBudget: "10",
				typedMemoryBudget: "999999999999",
				typedLatencyBudget: "10",
				typedOutput: "diagnostic",
				typedOutputSHA256: strings.Repeat("0", 64),
				wantError: true,
				wantOutput: "typed-lint diagnostic fingerprint changed",
			},
			{
				name: "typed diagnostic fingerprint matches",
				memoryBudget: "999999999999",
				latencyBudget: "10",
				typedMemoryBudget: "999999999999",
				typedLatencyBudget: "10",
				typedOutput: filepath.Join(resolvedTypedRoot, "file.go"),
				typedOutputSHA256: normalizedFingerprint,
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
					"GLIPPY_PEAK_RSS_RUNS=1",
					"GLIPPY_PEAK_RSS_FORMAT_ROOT=" + formatRoot,
					"GLIPPY_PEAK_RSS_TYPED_ROOT=" + formatRoot,
					"GLIPPY_PEAK_RSS_FORMAT_BUDGET_BYTES=" + test.memoryBudget,
					"GLIPPY_PEAK_RSS_FORMAT_BUDGET_SECONDS=" +
						test.latencyBudget,
					"GLIPPY_PEAK_RSS_TYPED_BUDGET_BYTES=" +
						test.typedMemoryBudget,
					"GLIPPY_PEAK_RSS_TYPED_BUDGET_SECONDS=" +
						test.typedLatencyBudget,
					"FAKE_GLIPPY_FORMAT_SLEEP_SECONDS=" + test.formatSleep,
					"FAKE_GLIPPY_TYPED_SLEEP_SECONDS=" + test.typedSleep,
					"FAKE_GLIPPY_TYPED_OUTPUT=" + test.typedOutput,
					"GLIPPY_PEAK_RSS_TYPED_OUTPUT_SHA256=" +
						test.typedOutputSHA256,
					"GLIPPY_PROCESS_TREE_RSS_COMMAND=" + fakeTreeRSS,
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
			printf '%s\n' "$FAKE_GLIPPY_REVISION"
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
			glippyRevision string
			wantError string
		}{
			{
				name: "Go host mismatch",
				goHostOS: "linux",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "arm64",
				corpusRevision: "abc123",
				glippyRevision: "glippy123",
				wantError: "release budget requires darwin/arm64; Go host is linux/arm64",
			},
			{
				name: "kernel architecture mismatch",
				goHostOS: "darwin",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "x86_64",
				corpusRevision: "abc123",
				glippyRevision: "glippy123",
				wantError: "release budget requires native darwin/arm64; kernel is darwin/amd64",
			},
			{
				name: "corpus revision mismatch",
				goHostOS: "darwin",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "arm64",
				corpusRevision: "different",
				glippyRevision: "glippy123",
				wantError: "formatter corpus revision is different; want abc123",
			},
			{
				name: "Glippy revision mismatch",
				goHostOS: "darwin",
				goHostArch: "arm64",
				unameOS: "Darwin",
				unameArch: "arm64",
				corpusRevision: "abc123",
				glippyRevision: "different",
				wantError: "Glippy revision is different; want glippy123",
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
					"GLIPPY_PEAK_RSS_FORMAT_ROOT=" + formatRoot,
					"GLIPPY_PEAK_RSS_FORMAT_REVISION=abc123",
					"GLIPPY_RELEASE_GLIPPY_REVISION=glippy123",
					"GLIPPY_RELEASE_EXPECTED_GOOS=darwin",
					"GLIPPY_RELEASE_EXPECTED_GOARCH=arm64",
					"FAKE_GO_HOST_OS=" + test.goHostOS,
					"FAKE_GO_HOST_ARCH=" + test.goHostArch,
					"FAKE_UNAME_OS=" + test.unameOS,
					"FAKE_UNAME_ARCH=" + test.unameArch,
					"FAKE_CORPUS_REVISION=" + test.corpusRevision,
					"FAKE_GLIPPY_REVISION=" + test.glippyRevision,
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
case "$output" in
	*/process-group)
		printf '%s\n' '#!/bin/sh' 'exec "$@"' >"$output"
		chmod +x "$output"
		exit 0
		;;
esac
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
	fakeTreeRSS := filepath.Join(toolDirectory, "tree-rss")
	writeExecutable(t, fakeTreeRSS, "#!/bin/sh\nprintf '%s\\n' 1024\n")

	command := exec.Command("sh", "peak-rss.sh")
	command.Dir = "."
	command.Env = append(
		os.Environ(),
		"PATH=" + toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GLIPPY_PEAK_RSS_RUNS=1",
		"GLIPPY_PEAK_RSS_FORMAT_ROOT=" + t.TempDir(),
		"GLIPPY_PEAK_RSS_FORMAT_BUDGET_BYTES=2147483648",
		"GLIPPY_PEAK_RSS_FORMAT_BUDGET_SECONDS=15",
		"GLIPPY_TIME_COMMAND=" + fakeTime,
		"GLIPPY_PROCESS_TREE_RSS_COMMAND=" + fakeTreeRSS,
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

func TestPeakRSSProbeDefaultsTypedMemoryBudgetToTwoGiB(t *testing.T) {
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
case "$output" in
	*/process-group)
		printf '%s\n' '#!/bin/sh' 'exec "$@"' >"$output"
		chmod +x "$output"
		exit 0
		;;
esac
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
printf '0.001 real 0.000 user 0.000 sys\n' >&2
printf '1024 maximum resident set size\n' >&2
for argument in "$@"; do
	if [ "$argument" = "true" ]; then
		exit 0
	fi
done
exit 1
`,
	)
	fakeTreeRSS := filepath.Join(toolDirectory, "tree-rss")
	writeExecutable(t, fakeTreeRSS, "#!/bin/sh\nset -eu\nprintf '%s\\n' 2147483649\n")

	command := exec.Command("sh", "peak-rss.sh")
	command.Dir = "."
	command.Env = append(
		os.Environ(),
		"PATH=" + toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GLIPPY_PEAK_RSS_RUNS=1",
		"GLIPPY_PEAK_RSS_FORMAT_ROOT=" + t.TempDir(),
		"GLIPPY_PEAK_RSS_FORMAT_BUDGET_BYTES=4294967296",
		"GLIPPY_TIME_COMMAND=" + fakeTime,
		"GLIPPY_PROCESS_TREE_RSS_COMMAND=" + fakeTreeRSS,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("peak-rss.sh succeeded, output = %q", output)
	}
	want := "typed-lint peak RSS budget exceeded: 2147483649 bytes > 2147483648 bytes"
	if !strings.Contains(string(output), want) {
		t.Fatalf("peak-rss.sh output = %q, want %q", output, want)
	}
}

func TestPeakRSSProbeFailsWhenProcessSnapshotFailsAsCommandExits(t *testing.T) {
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
case "$output" in
	*/process-group)
		printf '%s\n' '#!/bin/sh' 'exec "$@"' >"$output"
		chmod +x "$output"
		exit 0
		;;
esac
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
printf '0.001 real 0.000 user 0.000 sys\n' >&2
printf '1024 maximum resident set size\n' >&2
for argument in "$@"; do
	if [ "$argument" = "true" ]; then
		exit 0
	fi
done
exit 1
`,
	)
	fakeTreeRSS := filepath.Join(toolDirectory, "tree-rss")
	writeExecutable(
		t,
		fakeTreeRSS,
		`#!/bin/sh
set -eu
if [ ! -e "$FAKE_TREE_STATE" ]; then
	touch "$FAKE_TREE_STATE"
	printf '%s\n' 1024
	exit 0
fi
while kill -0 "$1" 2>/dev/null; do sleep 0.001; done
exit 3
`,
	)
	fakeTreeState := filepath.Join(toolDirectory, "tree-state")

	command := exec.Command("sh", "peak-rss.sh")
	command.Dir = "."
	command.Env = append(
		os.Environ(),
		"PATH=" + toolDirectory + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GLIPPY_PEAK_RSS_RUNS=1",
		"GLIPPY_PEAK_RSS_FORMAT_ROOT=" + t.TempDir(),
		"GLIPPY_TIME_COMMAND=" + fakeTime,
		"GLIPPY_PROCESS_TREE_RSS_COMMAND=" + fakeTreeRSS,
		"FAKE_TREE_STATE=" + fakeTreeState,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("peak-rss.sh succeeded, output = %q", output)
	}
	if want := "process-tree RSS sampling failed"; !strings.Contains(string(output), want) {
		t.Fatalf("peak-rss.sh output = %q, want %q", output, want)
	}
}

func TestPeakRSSProbeRejectsMalformedTimeEvidence(t *testing.T) {
	t.Parallel()

	for _, test := range
		[]struct {
			name string
			mode string
			elapsed string
			peak string
		}{
			{
				name: "darwin elapsed",
				mode: "darwin",
				elapsed: "not-a-duration",
				peak: "1024",
			},
			{name: "darwin peak", mode: "darwin", elapsed: "0.001", peak: "not-bytes"},
			{
				name: "gnu elapsed",
				mode: "gnu",
				elapsed: "0:not-a-duration",
				peak: "1024",
			},
			{name: "gnu peak", mode: "gnu", elapsed: "0:00.001", peak: "not-kbytes"},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
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
	if [ "$1" = "-o" ]; then output=$2; break; fi
	shift
done
test -n "$output"
case "$output" in
	*/process-group)
		printf '%s\n' '#!/bin/sh' 'exec "$@"' >"$output"
		chmod +x "$output"
		exit 0
		;;
esac
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
if [ "$FAKE_TIME_MODE" = gnu ]; then
	if [ "$1" = -l ]; then exit 0; fi
	printf 'Elapsed (wall clock) time (h:mm:ss or m:ss): %s\n' \
		"$FAKE_TIME_ELAPSED_SECONDS" >&2
	printf 'Maximum resident set size (kbytes): %s\n' "$FAKE_TIME_PEAK_BYTES" >&2
else
	printf '%s real 0.000 user 0.000 sys\n' "$FAKE_TIME_ELAPSED_SECONDS" >&2
	printf '%s maximum resident set size\n' "$FAKE_TIME_PEAK_BYTES" >&2
fi
for argument in "$@"; do
	if [ "$argument" = "true" ]; then exit 0; fi
done
exit 1
`,
				)
				fakeTreeRSS := filepath.Join(toolDirectory, "tree-rss")
				writeExecutable(t, fakeTreeRSS, "#!/bin/sh\nprintf '%s\\n' 1024\n")
				command := exec.Command("sh", "peak-rss.sh")
				command.Dir = "."
				command.Env = append(
					os.Environ(),
					"PATH=" +
						toolDirectory +
						string(os.PathListSeparator) +
						os.Getenv("PATH"),
					"GLIPPY_PEAK_RSS_RUNS=1",
					"GLIPPY_PEAK_RSS_FORMAT_ROOT=" + t.TempDir(),
					"GLIPPY_TIME_COMMAND=" + fakeTime,
					"GLIPPY_PROCESS_TREE_RSS_COMMAND=" + fakeTreeRSS,
					"FAKE_TIME_MODE=" + test.mode,
					"FAKE_TIME_ELAPSED_SECONDS=" + test.elapsed,
					"FAKE_TIME_PEAK_BYTES=" + test.peak,
				)
				output, err := command.CombinedOutput()
				if err == nil {
					t.Fatalf("peak-rss.sh succeeded, output = %q", output)
				}
				if want := "failed to parse elapsed time or peak RSS";
					!strings.Contains(string(output), want) {
					t.Fatalf("peak-rss.sh output = %q, want %q", output, want)
				}
			},
		)
	}
}

func TestProcessTreeRSSSumsRootAndEveryDescendant(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	fakePS := filepath.Join(toolDirectory, "ps")
	writeExecutable(
		t,
		fakePS,
		`#!/bin/sh
set -eu
printf '%s\n' \
  '100 1 100' \
  '200 100 200' \
  '300 200 300' \
  '400 1 400'
`,
	)
	command := exec.Command("sh", "process-tree-rss.sh", "100")
	command.Dir = "."
	command.Env = append(os.Environ(), "GLIPPY_PS_COMMAND=" + fakePS)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("process-tree-rss.sh error = %v, output = %q", err, output)
	}
	if got, want := strings.TrimSpace(string(output)), "614400"; got != want {
		t.Fatalf("process tree RSS = %s, want %s", got, want)
	}
}

func TestProcessTreeRSSFailsWhenProcessSnapshotIsUnavailable(t *testing.T) {
	t.Parallel()

	toolDirectory := t.TempDir()
	emptyPS := filepath.Join(toolDirectory, "empty-ps")
	writeExecutable(t, emptyPS, "#!/bin/sh\nexit 0\n")
	missingRootPS := filepath.Join(toolDirectory, "missing-root-ps")
	writeExecutable(t, missingRootPS, "#!/bin/sh\nprintf '%s\\n' '200 1 128'\n")
	malformedPS := filepath.Join(toolDirectory, "malformed-ps")
	writeExecutable(t, malformedPS, "#!/bin/sh\nprintf '%s\\n' '100 1 invalid'\n")
	extraFieldPS := filepath.Join(toolDirectory, "extra-field-ps")
	writeExecutable(t, extraFieldPS, "#!/bin/sh\nprintf '%s\\n' '100 1 128 trailing'\n")
	for _, test := range
		[]struct {
			name string
			command string
			status int
		}{
			{name: "command failure", command: "/usr/bin/false", status: 3},
			{name: "empty snapshot", command: emptyPS, status: 5},
			{name: "missing root", command: missingRootPS, status: 5},
			{name: "malformed snapshot", command: malformedPS, status: 4},
			{name: "extra snapshot field", command: extraFieldPS, status: 4},
		} {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				command := exec.Command("sh", "process-tree-rss.sh", "100")
				command.Dir = "."
				command.Env = append(
					os.Environ(),
					"GLIPPY_PS_COMMAND=" + test.command,
				)
				output, err := command.CombinedOutput()
				if err == nil {
					t.Fatalf(
						"process-tree-rss.sh succeeded, output = %q",
						output,
					)
				}
				exitError, ok := err.(*exec.ExitError)
				if !ok || exitError.ExitCode() != test.status {
					t.Fatalf(
						"process-tree-rss.sh error = %v, want exit %d; output = %q",
						err,
						test.status,
						output,
					)
				}
			},
		)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
