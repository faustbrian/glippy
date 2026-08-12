#!/bin/sh

set -eu

if ! command -v hyperfine >/dev/null 2>&1; then
	printf '%s\n' 'hyperfine is required' >&2
	exit 1
fi

budget_ms=${GOX_EDITOR_LATENCY_BUDGET_MS:-250}
case "$budget_ms" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GOX_EDITOR_LATENCY_BUDGET_MS must be a positive integer' >&2
		exit 1
		;;
esac

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
task_root=$(mktemp -d "${TMPDIR:-/tmp}/gox-editor-latency.XXXXXX")

cleanup() {
	find "$task_root" -mindepth 1 -delete
	rmdir "$task_root"
}
trap cleanup EXIT HUP INT TERM

env -u GOWORK GOCACHE="$task_root/cache" go -C "$repo_root" build \
	-o "$task_root/gox" ./cmd/gox

workload="$repo_root/benchmarks/testdata/workload/hostile.go"
hyperfine \
	--shell=none \
	--warmup 5 \
	--runs 20 \
	--export-csv "$task_root/results.csv" \
	--input "$workload" \
	"$task_root/gox fmt --stdin-filepath=$workload"

maximum_seconds=$(awk -F, 'NR == 2 { sub(/^.*,/, ""); print; exit }' \
	"$task_root/results.csv")
if [ -z "$maximum_seconds" ]; then
	printf '%s\n' 'hyperfine did not report a maximum latency' >&2
	exit 1
fi
if ! awk -v maximum="$maximum_seconds" -v budget="$budget_ms" \
	'BEGIN { exit !(maximum * 1000 <= budget) }'; then
	printf 'editor latency budget exceeded: %.3f ms > %s ms\n' \
		"$(awk -v maximum="$maximum_seconds" 'BEGIN { print maximum * 1000 }')" \
		"$budget_ms" >&2
	exit 1
fi
printf 'editor_latency_budget_ms,%s,maximum_ms,%.3f\n' "$budget_ms" \
	"$(awk -v maximum="$maximum_seconds" 'BEGIN { print maximum * 1000 }')"
