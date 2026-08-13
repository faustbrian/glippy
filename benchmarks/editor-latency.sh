#!/bin/sh

set -eu

budget_ms=${GLIPPY_EDITOR_LATENCY_BUDGET_MS:-250}
case "$budget_ms" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GLIPPY_EDITOR_LATENCY_BUDGET_MS must be a positive integer' >&2
		exit 1
		;;
esac

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
task_root=$(mktemp -d "${TMPDIR:-/tmp}/glippy-editor-latency.XXXXXX")

cleanup() {
	trap - EXIT HUP INT TERM
	if [ -d "$task_root/modcache" ] &&
		! GOMODCACHE="$task_root/modcache" go clean -modcache >/dev/null 2>&1; then
		chmod -R u+w "$task_root/modcache"
	fi
	find "$task_root" -mindepth 1 -delete
	rmdir "$task_root"
}
trap cleanup EXIT HUP INT TERM

env -u GOWORK GOCACHE="$task_root/cache" GOMODCACHE="$task_root/modcache" go -C "$repo_root" build \
	-o "$task_root/glippy" ./cmd/glippy
env -u GOWORK GOCACHE="$task_root/cache" GOMODCACHE="$task_root/modcache" go -C "$repo_root" build \
	-o "$task_root/editor-latency" ./benchmarks/cmd/editor-latency

workload="$repo_root/benchmarks/testdata/workload/hostile.go"
"$task_root/editor-latency" \
	--binary "$task_root/glippy" \
	--input "$workload" \
	--warmups 5 \
	--runs 20 \
	--budget-ms "$budget_ms"
