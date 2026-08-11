package analysis

import (
	"slices"
	"strings"
	"testing"
)

func TestPackageLoadEnvironmentDisablesEveryOrdinaryModuleNetworkRoute(t *testing.T) {
	t.Parallel()

	environment := packageLoadEnvironment(PackageLoadOptions{Env: []string{
		"GOPROXY=https://proxy.invalid",
		"GONOPROXY=*",
		"GOPRIVATE=*",
		"GOSUMDB=sum.golang.org",
		"GOTOOLCHAIN=auto",
		"GOVCS=*:all",
	}})
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	want := map[string]string{
		"GOPACKAGESDRIVER": "off",
		"GOPROXY":          "off",
		"GONOPROXY":        "none",
		"GOSUMDB":          "off",
		"GOTOOLCHAIN":      "local",
		"GOVCS":            "off",
	}
	for name, value := range want {
		if values[name] != value {
			t.Errorf("packageLoadEnvironment() %s = %q, want %q", name, values[name], value)
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

	for _, options := range []PackageLoadOptions{
		{ModuleMode: ModuleMode("mod")},
		{BuildTags: []string{"two tags"}},
		{BuildTags: []string{""}},
	} {
		if _, err := packageBuildFlags(options); err == nil {
			t.Fatalf("packageBuildFlags() accepted %#v", options)
		}
	}
}
