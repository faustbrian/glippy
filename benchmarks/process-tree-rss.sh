#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
	printf '%s\n' 'usage: process-tree-rss.sh <root-pid>' >&2
	exit 2
fi

root_pid=$1
case "$root_pid" in
	''|*[!0-9]*|0)
		printf '%s\n' 'root pid must be a positive integer' >&2
		exit 2
		;;
esac

ps_command=${GLIPPY_PS_COMMAND:-ps}
if ! process_snapshot=$("$ps_command" -axo pid=,ppid=,rss=); then
	printf '%s\n' 'failed to capture process snapshot' >&2
	exit 3
fi
printf '%s\n' "$process_snapshot" | awk -v root="$root_pid" '
{
	if (NF == 0) {
		next
	}
	if (NF != 3 || $1 !~ /^[0-9]+$/ || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/) {
		invalid = 1
		next
	}
	pid = $1
	parent[pid] = $2
	rss[pid] = $3
	if (pid == root) {
		found_root = 1
	}
	count++
}
END {
	if (invalid) {
		exit 4
	}
	if (!found_root) {
		exit 5
	}
	included[root] = 1
	for (pass = 0; pass < count; pass++) {
		for (pid in parent) {
			if (included[parent[pid]]) {
				included[pid] = 1
			}
		}
	}
	total = 0
	for (pid in included) {
		if (included[pid]) {
			total += rss[pid]
		}
	}
	printf "%.0f\n", total * 1024
}
'
