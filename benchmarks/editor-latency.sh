#!/bin/sh

set -eu

if ! command -v hyperfine >/dev/null 2>&1; then
	printf '%s\n' 'hyperfine is required' >&2
	exit 1
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
task_root=$(mktemp -d "${TMPDIR:-/tmp}/gox-editor-latency.XXXXXX")

cleanup() {
	find "$task_root" -mindepth 1 -delete
	rmdir "$task_root"
}
trap cleanup EXIT HUP INT TERM

GOWORK=off GOCACHE="$task_root/cache" go -C "$repo_root" build -o "$task_root/gox" ./cmd/gox

workload="$repo_root/benchmarks/testdata/workload/hostile.go"
hyperfine \
	--shell=none \
	--warmup 5 \
	--runs 20 \
	--input "$workload" \
	"$task_root/gox fmt --stdin-filepath=$workload"
