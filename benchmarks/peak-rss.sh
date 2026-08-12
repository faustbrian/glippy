#!/bin/sh

set -eu

runs=${GOX_PEAK_RSS_RUNS:-5}
case "$runs" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GOX_PEAK_RSS_RUNS must be a positive integer' >&2
		exit 1
		;;
esac

format_budget_bytes=${GOX_PEAK_RSS_FORMAT_BUDGET_BYTES:-2147483648}
case "$format_budget_bytes" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GOX_PEAK_RSS_FORMAT_BUDGET_BYTES must be a positive integer' >&2
		exit 1
		;;
esac

format_budget_seconds=${GOX_PEAK_RSS_FORMAT_BUDGET_SECONDS:-15}
case "$format_budget_seconds" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GOX_PEAK_RSS_FORMAT_BUDGET_SECONDS must be a positive integer' >&2
		exit 1
		;;
esac

if [ ! -x /usr/bin/time ]; then
	printf '%s\n' '/usr/bin/time is required' >&2
	exit 1
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
format_root_input=${GOX_PEAK_RSS_FORMAT_ROOT:-$repo_root}
if [ ! -d "$format_root_input" ]; then
	printf 'GOX_PEAK_RSS_FORMAT_ROOT is not a directory: %s\n' \
		"$format_root_input" >&2
	exit 1
fi
format_root=$(CDPATH='' cd -- "$format_root_input" && pwd -P)
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

GOCACHE="$task_root/cache"
export GOCACHE
env -u GOWORK go -C "$repo_root" build -o "$task_root/gox" ./cmd/gox

printf '%s\n' 'version = 1' '[lint]' 'preset = "suspicious"' \
	>"$task_root/gox.toml"

measure() {
	label=$1
	budget_bytes=$2
	budget_seconds=$3
	shift 3
	sample=1
	while [ "$sample" -le "$runs" ]; do
		started=$(date +%s)
		set +e
		if [ "$time_mode" = darwin ]; then
			/usr/bin/time -l "$@" >"$command_output" 2>"$time_output"
		else
			/usr/bin/time -v "$@" >"$command_output" 2>"$time_output"
		fi
		status=$?
		set -e
		finished=$(date +%s)
		elapsed_seconds=$((finished - started))
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
		if [ "$peak_bytes" -gt "$budget_bytes" ]; then
			printf '%s peak RSS budget exceeded: %s bytes > %s bytes\n' \
				"$label" "$peak_bytes" "$budget_bytes" >&2
			exit 1
		fi
		if [ "$elapsed_seconds" -gt "$budget_seconds" ]; then
			printf '%s elapsed-time budget exceeded: %s seconds > %s seconds\n' \
				"$label" "$elapsed_seconds" "$budget_seconds" >&2
			exit 1
		fi
		printf '%s,%d,%s,%s\n' "$label" "$sample" "$elapsed_seconds" "$peak_bytes"
		sample=$((sample + 1))
	done
}

printf '%s\n' 'workload,sample,elapsed_seconds,peak_rss_bytes'
measure formatter-check "$format_budget_bytes" "$format_budget_seconds" \
	"$task_root/gox" fmt --check "$format_root"
measure typed-combined-check 9223372036854775807 9223372036854775807 \
	"$task_root/gox" check \
	--config="$task_root/gox.toml" "$repo_root/..."
