package corpus

import (
	"strings"
	"testing"
)

func TestAddProfileMeasurementRejectsAggregateOverflow(t *testing.T) {
	t.Parallel()

	maximumInt := int(^uint(0) >> 1)
	maximumInt64 := int64(^uint64(0) >> 1)
	maximumUint64 := ^uint64(0)
	tests := []struct {
		name string
		aggregate profileReport
		measurement profileMeasurement
		want string
	}{
		{
			name: "file count",
			aggregate: profileReport{Files: maximumInt},
			measurement: profileMeasurement{Files: 1},
			want: "files",
		},
		{
			name: "duration",
			aggregate: profileReport{DurationNanoseconds: maximumInt64},
			measurement: profileMeasurement{DurationNanoseconds: 1},
			want: "duration",
		},
		{
			name: "allocated bytes",
			aggregate: profileReport{AllocatedBytes: maximumUint64},
			measurement: profileMeasurement{AllocatedBytes: 1},
			want: "allocated bytes",
		},
	}
	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				err := addProfileMeasurement(&test.aggregate, test.measurement)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf(
						"addProfileMeasurement() error = %v, want %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}
