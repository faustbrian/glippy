package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProbeMeasuresWarmupsAndEnforcesMaximumSample(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		durations  []time.Duration
		wantError  string
		wantOutput string
	}{
		{
			name:       "inside budget",
			durations:  []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 249 * time.Millisecond},
			wantOutput: "maximum_ms,249.000",
		},
		{
			name:       "maximum exceeds budget",
			durations:  []time.Duration{10 * time.Millisecond, 251 * time.Millisecond, 20 * time.Millisecond},
			wantError:  "editor latency budget exceeded: 251.000 ms > 250 ms",
			wantOutput: "maximum_ms,251.000",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			invocations := 0
			var output bytes.Buffer
			err := probe(
				probeOptions{warmups: 2, runs: len(test.durations), budget: 250 * time.Millisecond},
				func() (time.Duration, error) {
					invocations++
					if invocations <= 2 {
						return time.Millisecond, nil
					}
					return test.durations[invocations-3], nil
				},
				&output,
			)
			if invocations != 2+len(test.durations) {
				t.Fatalf("probe invocations = %d, want %d", invocations, 2+len(test.durations))
			}
			if test.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantError != "" && (err == nil || err.Error() != test.wantError) {
				t.Fatalf("probe error = %v, want %q", err, test.wantError)
			}
			if !strings.Contains(output.String(), test.wantOutput) {
				t.Fatalf("probe output = %q, want %q", output.String(), test.wantOutput)
			}
		})
	}
}

func TestProbeStopsOnInvocationFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("start formatter")
	err := probe(
		probeOptions{warmups: 1, runs: 1, budget: 250 * time.Millisecond},
		func() (time.Duration, error) { return 0, want },
		&bytes.Buffer{},
	)
	if !errors.Is(err, want) {
		t.Fatalf("probe error = %v, want %v", err, want)
	}
}
