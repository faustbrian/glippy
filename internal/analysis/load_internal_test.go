package analysis

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/faustbrian/glippy/internal/source"
)

func TestPackageSourceCollectorOrdersMultipleFatalSourceFailures(t *testing.T) {
	root := t.TempDir()
	aPath := filepath.Join(root, "a.go")
	zPath := filepath.Join(root, "z.go")
	collector := newPackageSourceCollector(defaultPackageResourceLimits(), false)
	collector.add(zPath, nil, fmt.Errorf("z overflow: %w", source.ErrTooLarge))
	collector.add(aPath, nil, fmt.Errorf("a overflow: %w", source.ErrTooLarge))

	result, err := collector.result(false)
	if len(result.Paths()) != 0 || !errors.Is(err, source.ErrTooLarge) {
		t.Fatalf(
			"result() returned paths=%q, error=%v, want ErrTooLarge",
			result.Paths(),
			err,
		)
	}
	want := "package parser did not capture source " +
		fmt.Sprintf("%q: a overflow", aPath) +
		": Go source is too large\npackage parser did not capture source " +
		fmt.Sprintf("%q: z overflow", zPath) +
		": Go source is too large"
	if err.Error() != want {
		t.Fatalf("result() error = %q, want %q", err, want)
	}
}

func TestPackageLoadEnvironmentDisablesEveryOrdinaryModuleNetworkRoute(t *testing.T) {
	t.Parallel()

	environment := packageLoadEnvironment(
		PackageLoadOptions{
			Env: []string{
				"GOPROXY=https://proxy.invalid",
				"GONOPROXY=*",
				"GOPRIVATE=*",
				"GOSUMDB=sum.golang.org",
				"GOTOOLCHAIN=auto",
				"GOVCS=*:all",
			},
		},
	)
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	want := map[string]string{
		"GOPACKAGESDRIVER": "off",
		"GOPROXY": "off",
		"GONOPROXY": "none",
		"GOSUMDB": "off",
		"GOTOOLCHAIN": "local",
		"GOVCS": "off",
	}
	for name, value := range want {
		if values[name] != value {
			t.Errorf(
				"packageLoadEnvironment() %s = %q, want %q",
				name,
				values[name],
				value,
			)
		}
	}
}

func TestPackageBuildFlagsAreReadOnlyAndCanonical(t *testing.T) {
	t.Parallel()

	flags, err := packageBuildFlags(PackageLoadOptions{BuildTags: []string{"z", "a", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(flags, []string{"-mod=readonly", "-tags=a,z"}) {
		t.Fatalf("packageBuildFlags() = %q", flags)
	}

	flags, err = packageBuildFlags(PackageLoadOptions{ModuleMode: ModuleVendor})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(flags, []string{"-mod=vendor"}) {
		t.Fatalf("packageBuildFlags(vendor) = %q", flags)
	}

	for _, options := range
		[]PackageLoadOptions{
			{ModuleMode: ModuleMode("mod")},
			{BuildTags: []string{"two tags"}},
			{BuildTags: []string{""}},
		} {
		if _, err := packageBuildFlags(options); err == nil {
			t.Fatalf("packageBuildFlags() accepted %#v", options)
		}
	}
}
