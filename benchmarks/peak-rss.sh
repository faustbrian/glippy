#!/bin/sh

set -eu

runs=${GLIPPY_PEAK_RSS_RUNS:-5}
case "$runs" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GLIPPY_PEAK_RSS_RUNS must be a positive integer' >&2
		exit 1
		;;
esac

format_budget_bytes=${GLIPPY_PEAK_RSS_FORMAT_BUDGET_BYTES:-2147483648}
case "$format_budget_bytes" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GLIPPY_PEAK_RSS_FORMAT_BUDGET_BYTES must be a positive integer' >&2
		exit 1
		;;
esac

format_budget_seconds=${GLIPPY_PEAK_RSS_FORMAT_BUDGET_SECONDS:-90}
case "$format_budget_seconds" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GLIPPY_PEAK_RSS_FORMAT_BUDGET_SECONDS must be a positive integer' >&2
		exit 1
		;;
esac

typed_budget_bytes=${GLIPPY_PEAK_RSS_TYPED_BUDGET_BYTES:-2147483648}
case "$typed_budget_bytes" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GLIPPY_PEAK_RSS_TYPED_BUDGET_BYTES must be a positive integer' >&2
		exit 1
		;;
esac

typed_budget_seconds=${GLIPPY_PEAK_RSS_TYPED_BUDGET_SECONDS:-40}
case "$typed_budget_seconds" in
	''|*[!0-9]*|0)
		printf '%s\n' 'GLIPPY_PEAK_RSS_TYPED_BUDGET_SECONDS must be a positive integer' >&2
		exit 1
		;;
esac

typed_output_sha256=${GLIPPY_PEAK_RSS_TYPED_OUTPUT_SHA256:-}
if [ -n "$typed_output_sha256" ]; then
	case "$typed_output_sha256" in
		*[!0-9a-f]*)
			printf '%s\n' 'GLIPPY_PEAK_RSS_TYPED_OUTPUT_SHA256 must be lowercase hexadecimal' >&2
			exit 1
			;;
	esac
	if [ "${#typed_output_sha256}" -ne 64 ]; then
		printf '%s\n' 'GLIPPY_PEAK_RSS_TYPED_OUTPUT_SHA256 must contain 64 characters' >&2
		exit 1
	fi
fi

time_command=${GLIPPY_TIME_COMMAND:-/usr/bin/time}
if [ ! -x "$time_command" ]; then
	printf 'time command is not executable: %s\n' "$time_command" >&2
	exit 1
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
format_root_input=${GLIPPY_PEAK_RSS_FORMAT_ROOT:-$repo_root}
if [ ! -d "$format_root_input" ]; then
	printf 'GLIPPY_PEAK_RSS_FORMAT_ROOT is not a directory: %s\n' \
		"$format_root_input" >&2
	exit 1
fi
format_root=$(CDPATH='' cd -- "$format_root_input" && pwd -P)
typed_root_input=${GLIPPY_PEAK_RSS_TYPED_ROOT:-$repo_root}
if [ ! -d "$typed_root_input" ]; then
	printf 'GLIPPY_PEAK_RSS_TYPED_ROOT is not a directory: %s\n' \
		"$typed_root_input" >&2
	exit 1
fi
typed_root=$(CDPATH='' cd -- "$typed_root_input" && pwd -P)

glippy_revision=${GLIPPY_RELEASE_GLIPPY_REVISION:-}
if [ -n "$glippy_revision" ]; then
	actual_glippy_revision=$(git -C "$repo_root" rev-parse HEAD)
	if [ "$actual_glippy_revision" != "$glippy_revision" ]; then
		printf 'Glippy revision is %s; want %s\n' \
			"$actual_glippy_revision" "$glippy_revision" >&2
		exit 1
	fi
	if [ -n "$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all)" ]; then
		printf '%s\n' 'Glippy repository must be clean for a release budget run' >&2
		exit 1
	fi
fi

expected_goos=${GLIPPY_RELEASE_EXPECTED_GOOS:-}
expected_goarch=${GLIPPY_RELEASE_EXPECTED_GOARCH:-}
if [ -n "$expected_goos" ] || [ -n "$expected_goarch" ]; then
	if [ -z "$expected_goos" ] || [ -z "$expected_goarch" ]; then
		printf '%s\n' 'GLIPPY_RELEASE_EXPECTED_GOOS and GLIPPY_RELEASE_EXPECTED_GOARCH must be set together' >&2
		exit 1
	fi
	set -- $(go env GOHOSTOS GOHOSTARCH)
	go_host_os=$1
	go_host_arch=$2
	if [ "$go_host_os" != "$expected_goos" ] || [ "$go_host_arch" != "$expected_goarch" ]; then
		printf 'release budget requires %s/%s; Go host is %s/%s\n' \
			"$expected_goos" "$expected_goarch" "$go_host_os" "$go_host_arch" >&2
		exit 1
	fi
	kernel_os_raw=$(uname -s)
	case "$kernel_os_raw" in
		Darwin) kernel_os=darwin ;;
		Linux) kernel_os=linux ;;
		*) kernel_os=$kernel_os_raw ;;
	esac
	kernel_arch_raw=$(uname -m)
	case "$kernel_arch_raw" in
		x86_64|amd64) kernel_arch=amd64 ;;
		arm64|aarch64) kernel_arch=arm64 ;;
		*) kernel_arch=$kernel_arch_raw ;;
	esac
	if [ "$kernel_os" != "$expected_goos" ] || [ "$kernel_arch" != "$expected_goarch" ]; then
		printf 'release budget requires native %s/%s; kernel is %s/%s\n' \
			"$expected_goos" "$expected_goarch" "$kernel_os" "$kernel_arch" >&2
		exit 1
	fi
fi

format_revision=${GLIPPY_PEAK_RSS_FORMAT_REVISION:-}
if [ -n "$format_revision" ]; then
	actual_format_revision=$(git -C "$format_root" rev-parse HEAD)
	if [ "$actual_format_revision" != "$format_revision" ]; then
		printf 'formatter corpus revision is %s; want %s\n' \
			"$actual_format_revision" "$format_revision" >&2
		exit 1
	fi
	if [ -n "$(git -C "$format_root" status --porcelain=v1 --untracked-files=all)" ]; then
		printf '%s\n' 'formatter corpus must be clean for a release budget run' >&2
		exit 1
	fi
fi

typed_revision=${GLIPPY_PEAK_RSS_TYPED_REVISION:-}
if [ -n "$typed_revision" ]; then
	actual_typed_revision=$(git -C "$typed_root" rev-parse HEAD)
	if [ "$actual_typed_revision" != "$typed_revision" ]; then
		printf 'typed corpus revision is %s; want %s\n' \
			"$actual_typed_revision" "$typed_revision" >&2
		exit 1
	fi
	if [ -n "$(git -C "$typed_root" status --porcelain=v1 --untracked-files=all)" ]; then
		printf '%s\n' 'typed corpus must be clean for a release budget run' >&2
		exit 1
	fi
fi

task_root=$(mktemp -d "${TMPDIR:-/tmp}/glippy-peak-rss.XXXXXX")

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

time_output="$task_root/time-output"
command_output="$task_root/command-output"
normalized_output="$task_root/normalized-output"
if "$time_command" -l true >/dev/null 2>"$time_output" &&
	grep -q 'maximum resident set size' "$time_output"; then
	time_mode=darwin
elif "$time_command" -v true >/dev/null 2>"$time_output" &&
	grep -q 'Maximum resident set size (kbytes)' "$time_output"; then
	time_mode=gnu
else
	printf '%s\n' '/usr/bin/time does not expose supported peak-RSS output' >&2
	exit 1
fi

GOCACHE="$task_root/cache"
GOMODCACHE="$task_root/modcache"
export GOCACHE GOMODCACHE
env -u GOWORK GOFLAGS=-mod=readonly go -C "$repo_root" mod download
if [ "$typed_root" != "$repo_root" ]; then
	env -u GOWORK GOFLAGS=-mod=readonly go -C "$typed_root" mod download
fi
env -u GOWORK go -C "$repo_root" build -o "$task_root/glippy" ./cmd/glippy

measure() {
	label=$1
	budget_bytes=$2
	budget_seconds=$3
	shift 3
	sample=1
	while [ "$sample" -le "$runs" ]; do
		set +e
		if [ "$time_mode" = darwin ]; then
			"$time_command" -l "$@" >"$command_output" 2>"$time_output"
		else
			"$time_command" -v "$@" >"$command_output" 2>"$time_output"
		fi
		status=$?
		set -e
		if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then
			printf '%s: command exited %d\n' "$label" "$status" >&2
			sed -n '1,20p' "$command_output" >&2
			sed -n '1,20p' "$time_output" >&2
			exit "$status"
		fi
		if [ "$label" = typed-lint ] && [ -n "$typed_output_sha256" ]; then
			awk -v root="$typed_root" '
			{
				remaining = $0
				normalized = ""
				while ((offset = index(remaining, root)) != 0) {
					normalized = normalized substr(remaining, 1, offset - 1) "<TYPED_ROOT>"
					remaining = substr(remaining, offset + length(root))
				}
				print normalized remaining
			}' "$command_output" >"$normalized_output"
			if command -v sha256sum >/dev/null 2>&1; then
				actual_output_sha256=$(sha256sum "$normalized_output" | awk '{ print $1 }')
			elif command -v shasum >/dev/null 2>&1; then
				actual_output_sha256=$(shasum -a 256 "$normalized_output" | awk '{ print $1 }')
			else
				printf '%s\n' 'no SHA-256 command is available' >&2
				exit 1
			fi
			if [ "$actual_output_sha256" != "$typed_output_sha256" ]; then
				printf '%s diagnostic fingerprint changed: %s != %s\n' \
					"$label" "$actual_output_sha256" "$typed_output_sha256" >&2
				exit 1
			fi
		fi
		if [ "$time_mode" = darwin ]; then
			elapsed_seconds=$(awk '/ real / { print $1; exit }' "$time_output")
			peak_bytes=$(awk '/maximum resident set size/ { print $1; exit }' "$time_output")
		else
			elapsed_seconds=$(awk -F': ' '/Elapsed \(wall clock\) time/ {
				n = split($2, parts, ":")
				if (n == 2) {
					printf "%.3f\n", parts[1] * 60 + parts[2]
				} else if (n == 3) {
					printf "%.3f\n", parts[1] * 3600 + parts[2] * 60 + parts[3]
				}
				exit
			}' "$time_output")
			peak_bytes=$(awk -F: '/Maximum resident set size \(kbytes\)/ {
				gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
				printf "%.0f\n", $2 * 1024
				exit
			}' "$time_output")
		fi
		if [ -z "$elapsed_seconds" ] || [ -z "$peak_bytes" ]; then
			printf '%s\n' 'failed to parse elapsed time or peak RSS' >&2
			exit 1
		fi
		if [ "$peak_bytes" -gt "$budget_bytes" ]; then
			printf '%s peak RSS budget exceeded: %s bytes > %s bytes\n' \
				"$label" "$peak_bytes" "$budget_bytes" >&2
			exit 1
		fi
		if ! awk -v elapsed="$elapsed_seconds" -v budget="$budget_seconds" \
			'BEGIN { exit !(elapsed <= budget) }'; then
			printf '%s elapsed-time budget exceeded: %.3f seconds > %s seconds\n' \
				"$label" "$elapsed_seconds" "$budget_seconds" >&2
			exit 1
		fi
		printf '%s,%d,%.3f,%s\n' "$label" "$sample" "$elapsed_seconds" "$peak_bytes"
		sample=$((sample + 1))
	done
}

if [ -n "$expected_goos" ]; then
	printf 'metadata,goos,%s\nmetadata,goarch,%s\n' "$expected_goos" "$expected_goarch"
fi
if [ -n "$glippy_revision" ]; then
	printf 'metadata,glippy_revision,%s\n' "$glippy_revision"
fi
if [ -n "$format_revision" ]; then
	printf 'metadata,format_revision,%s\n' "$format_revision"
fi
if [ -n "$typed_revision" ]; then
	printf 'metadata,typed_revision,%s\n' "$typed_revision"
fi
printf '%s\n' 'workload,sample,elapsed_seconds,peak_rss_bytes'
measure formatter-check "$format_budget_bytes" "$format_budget_seconds" \
	"$task_root/glippy" fmt --check "$format_root"
measure typed-lint "$typed_budget_bytes" "$typed_budget_seconds" \
	"$task_root/glippy" lint -Wsuspicious "$typed_root/..."
