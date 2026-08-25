#!/bin/sh

set -eu

reported_peak_bytes=${1:-}
sampled_peak_bytes=${2:-}

case "$reported_peak_bytes" in
	''|*[!0-9]*)
		printf '%s\n' 'reported peak RSS must be a positive integer' >&2
		exit 1
		;;
esac
if [ "$reported_peak_bytes" -le 0 ]; then
	printf '%s\n' 'reported peak RSS must be a positive integer' >&2
	exit 1
fi

if [ "$sampled_peak_bytes" = missed ]; then
	printf '%s\n' "$reported_peak_bytes"
	exit 0
fi

case "$sampled_peak_bytes" in
	''|*[!0-9]*)
		printf '%s\n' 'process-tree peak RSS must be a positive integer' >&2
		exit 1
		;;
esac
if [ "$sampled_peak_bytes" -le 0 ]; then
	printf '%s\n' 'process-tree peak RSS must be a positive integer' >&2
	exit 1
fi

if [ "$sampled_peak_bytes" -gt "$reported_peak_bytes" ]; then
	printf '%s\n' "$sampled_peak_bytes"
else
	printf '%s\n' "$reported_peak_bytes"
fi
