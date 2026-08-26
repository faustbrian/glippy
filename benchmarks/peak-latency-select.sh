#!/bin/sh

set -eu

LC_ALL=C
export LC_ALL

if [ "$#" -lt 3 ]; then
	printf '%s\n' \
		'usage: peak-latency-select.sh <label> <budget-seconds> <sample>...' >&2
	exit 2
fi

label=$1
budget=$2
shift 2

awk -v label="$label" -v budget="$budget" '
BEGIN {
	if (budget !~ /^[0-9]+([.][0-9]+)?$/ || budget <= 0) {
		print "latency budget must be positive decimal seconds" > "/dev/stderr"
		exit 1
	}
	count = ARGC - 1
	for (sample_index = 1; sample_index < ARGC; sample_index++) {
		raw = ARGV[sample_index]
		if (raw !~ /^[0-9]+([.][0-9]+)?$/) {
			print "latency samples must be non-negative decimal seconds" > "/dev/stderr"
			exit 1
		}
		value = raw + 0
		if (value > budget * 2) {
			printf "%s hard elapsed-time budget exceeded: %.3f seconds > %.3f seconds\n", \
				label, value, budget * 2 > "/dev/stderr"
			exit 1
		}
		values[sample_index] = value
	}
	for (sample_index = 2; sample_index <= count; sample_index++) {
		value = values[sample_index]
		position = sample_index - 1
		while (position >= 1 && values[position] > value) {
			values[position + 1] = values[position]
			position--
		}
		values[position + 1] = value
	}
	rank = int((count * 80 + 99) / 100)
	selected = values[rank]
	if (selected > budget) {
		printf "%s sustained elapsed-time budget exceeded: %.3f seconds > %s seconds\n", \
			label, selected, budget > "/dev/stderr"
		exit 1
	}
	printf "%.3f\n", selected
}
' "$@"
