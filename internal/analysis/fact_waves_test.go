package analysis

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestPackageFactScheduleIsDependencyFirstAndUnique(t *testing.T) {
	t.Parallel()

	common := completeFactPackage("example.com/common")
	left := completeFactPackage("example.com/left", common)
	right := completeFactPackage("example.com/right", common)
	unsafePackage := completeFactPackage("unsafe")
	cgoPackage := completeFactPackage("C")
	root := completeFactPackage("example.com/root", right, left, unsafePackage, cgoPackage)

	dependencies, roots, err := packageFactSchedule(
		[]*packages.Package{
			{ID: "example.com/root.test", Types: root},
			{ID: "example.com/root", Types: root},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{
		"example.com/common",
		"example.com/left",
		"example.com/right",
		"unsafe",
	};
		!reflect.DeepEqual(dependencies, want) {
		t.Fatalf("dependency schedule = %q, want %q", dependencies, want)
	}
	if want := []string{"example.com/root", "example.com/root.test"};
		!reflect.DeepEqual(roots, want) {
		t.Fatalf("root schedule = %q, want %q", roots, want)
	}
}

func TestPackageFactScheduleRejectsImportCycles(t *testing.T) {
	t.Parallel()

	left := types.NewPackage("example.com/left", "left")
	right := types.NewPackage("example.com/right", "right")
	left.SetImports([]*types.Package{right})
	right.SetImports([]*types.Package{left})
	left.MarkComplete()
	right.MarkComplete()
	_, _, err := packageFactSchedule([]*packages.Package{{ID: "example.com/left", Types: left}})
	if err == nil || !strings.Contains(err.Error(), "import cycle") {
		t.Fatalf("packageFactSchedule() error = %v, want import-cycle refusal", err)
	}
}

func TestPackageFactWavesAreBoundedAndOrdered(t *testing.T) {
	t.Parallel()

	dependencies := make([]string, packageFactWaveSize * 2 + 1)
	for index := range dependencies {
		dependencies[index] = fmt.Sprintf("example.com/dependency/%03d", index)
	}
	waves := packageFactWaves(dependencies)
	if len(waves) != 3 ||
		len(waves[0]) != packageFactWaveSize ||
		len(waves[1]) != packageFactWaveSize ||
		len(waves[2]) != 1 {
		t.Fatalf("package fact waves = %#v", waves)
	}
	flattened := make([]string, 0, len(dependencies))
	for _, wave := range waves {
		if len(wave) == 0 || len(wave) > packageFactWaveSize {
			t.Fatalf("package fact wave size = %d", len(wave))
		}
		flattened = append(flattened, wave...)
	}
	if !reflect.DeepEqual(flattened, dependencies) {
		t.Fatalf("flattened package fact waves = %q, want %q", flattened, dependencies)
	}
	waves[0][0] = "mutated"
	if dependencies[0] == "mutated" {
		t.Fatal("packageFactWaves() retained mutable input storage")
	}
}

func completeFactPackage(path string, imports ...*types.Package) *types.Package {
	pkg := types.NewPackage(path, "fixture")
	pkg.SetImports(imports)
	pkg.MarkComplete()
	return pkg
}
