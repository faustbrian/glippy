package version

import (
	"runtime/debug"
	"testing"
)

func TestResolvePrefersReleaseMetadataAndFallsBackDeterministically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		linked  string
		build   *debug.BuildInfo
		version string
	}{
		{
			name:    "release metadata",
			linked:  "v1.2.3",
			build:   &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}},
			version: "v1.2.3",
		},
		{
			name:    "installed module",
			build:   &debug.BuildInfo{Main: debug.Module{Version: "v1.4.0"}},
			version: "v1.4.0",
		},
		{
			name:    "development build",
			build:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			version: "devel",
		},
		{
			name:    "missing build information",
			version: "devel",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := Resolve(test.linked, test.build); got != test.version {
				t.Fatalf("Resolve() = %q, want %q", got, test.version)
			}
		})
	}
}
