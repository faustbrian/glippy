#!/bin/sh

set -eu

runs=${GOX_PEAK_RSS_RUNS:-5}
case "$runs" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GOX_PEAK_RSS_RUNS must be a positive integer' >&2
		exit 1
		;;
esac

if [ ! -x /usr/bin/time ]; then
	printf '%s\n' '/usr/bin/time is required' >&2
	exit 1
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
task_root=$(mktemp -d "${TMPDIR:-/tmp}/gox-peak-rss.XXXXXX")

cleanup() {
	find "$task_root" -mindepth 1 -delete
	rmdir "$task_root"
}
trap cleanup EXIT HUP INT TERM

time_output="$task_root/time-output"
command_output="$task_root/command-output"
if /usr/bin/time -l true >/dev/null 2>"$time_output" &&
	grep -q 'maximum resident set size' "$time_output"; then
	time_mode=darwin
elif /usr/bin/time -v true >/dev/null 2>"$time_output" &&
	grep -q 'Maximum resident set size (kbytes)' "$time_output"; then
	time_mode=gnu
else
	printf '%s\n' '/usr/bin/time does not expose supported peak-RSS output' >&2
	exit 1
fi

GOWORK=off
GOCACHE="$task_root/cache"
export GOWORK GOCACHE
go -C "$repo_root" build -o "$task_root/gox" ./cmd/gox

printf '%s\n' 'version = 1' '[lint]' 'preset = "suspicious"' \
	>"$task_root/gox.toml"

measure() {
	label=$1
	shift
	sample=1
	while [ "$sample" -le "$runs" ]; do
		set +e
		if [ "$time_mode" = darwin ]; then
			/usr/bin/time -l "$@" >"$command_output" 2>"$time_output"
		else
			/usr/bin/time -v "$@" >"$command_output" 2>"$time_output"
		fi
		status=$?
		set -e
		if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then
			printf '%s: command exited %d\n' "$label" "$status" >&2
			sed -n '1,20p' "$command_output" >&2
			sed -n '1,20p' "$time_output" >&2
			exit "$status"
		fi
		if [ "$time_mode" = darwin ]; then
			peak_bytes=$(awk '/maximum resident set size/ { print $1; exit }' "$time_output")
		else
			peak_bytes=$(awk -F: '/Maximum resident set size \(kbytes\)/ {
				gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
				printf "%.0f\n", $2 * 1024
				exit
			}' "$time_output")
		fi
		if [ -z "$peak_bytes" ]; then
			printf '%s\n' 'failed to parse peak RSS' >&2
			exit 1
		fi
		printf '%s,%d,%s\n' "$label" "$sample" "$peak_bytes"
		sample=$((sample + 1))
	done
}

printf '%s\n' 'workload,sample,peak_rss_bytes'
measure formatter-check "$task_root/gox" fmt --check "$repo_root"
measure typed-combined-check "$task_root/gox" check \
	--config="$task_root/gox.toml" "$repo_root/..."
