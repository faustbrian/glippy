#!/bin/sh

set -eu

if [ "$#" -ne 3 ]; then
	printf '%s\n' 'usage: peak-rss-sample.sh <status> <current> <peak>' >&2
	exit 2
fi

sample_status=$1
current=$2
peak=$3

case "$sample_status" in
	''|*[!0-9]*)
		printf '%s\n' 'process-tree sample status must be a non-negative integer' >&2
		exit 1
		;;
esac
case "$peak" in
	''|*[!0-9]*)
		printf '%s\n' 'process-tree peak must be a non-negative integer' >&2
		exit 1
		;;
esac

if [ "$sample_status" -eq 5 ]; then
	if [ "$peak" -gt 0 ]; then
		printf 'complete %s\n' "$peak"
	else
		printf '%s\n' 'complete missed'
	fi
	exit 0
fi
if [ "$sample_status" -ne 0 ]; then
	printf '%s\n' 'process-tree RSS sampling failed' >&2
	exit 1
fi

case "$current" in
	''|*[!0-9]*)
		printf '%s\n' 'process-tree RSS sample must be a non-negative integer' >&2
		exit 1
		;;
esac
if [ "$current" -eq 0 ]; then
	printf 'continue %s\n' "$peak"
	exit 0
fi

if [ "$current" -gt "$peak" ]; then
	peak=$current
fi
printf 'continue %s\n' "$peak"
