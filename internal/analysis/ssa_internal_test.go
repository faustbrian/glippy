package analysis

import (
	"path/filepath"
	"testing"

	"github.com/faustbrian/glippy/internal/source"
	"golang.org/x/tools/go/packages"
)

func TestSSAPackageWavesBoundCountAndSourceBytes(t *testing.T) {
	t.Parallel()

	packages_ := make([]*packages.Package, maximumSSAPackagesPerProgram + 1)
	bytesByPackage := make(map[*packages.Package]int64, len(packages_))
	for index := range packages_ {
		packages_[index] = &packages.Package{ID: "package"}
		bytesByPackage[packages_[index]] = 1
	}
	waves := ssaPackageWaves(packages_, bytesByPackage)
	if len(waves) != 2 ||
		len(waves[0]) != maximumSSAPackagesPerProgram ||
		len(waves[1]) != 1 ||
		waves[1][0] != packages_[maximumSSAPackagesPerProgram] {
		t.Fatalf("count-bounded SSA waves = %#v", waves)
	}

	first := &packages.Package{ID: "first"}
	second := &packages.Package{ID: "second"}
	third := &packages.Package{ID: "third"}
	packages_ = []*packages.Package{first, second, third}
	bytesByPackage = map[*packages.Package]int64{
		first: maximumSSAPackageWaveSourceBytes * 5 / 8,
		second: maximumSSAPackageWaveSourceBytes / 2,
		third: maximumSSAPackageWaveSourceBytes * 3 / 8,
	}
	waves = ssaPackageWaves(packages_, bytesByPackage)
	if len(waves) != 2 ||
		len(waves[0]) != 1 ||
		waves[0][0] != first ||
		len(waves[1]) != 2 ||
		waves[1][0] != second ||
		waves[1][1] != third {
		t.Fatalf("byte-bounded SSA waves = %#v", waves)
	}

	oversized := &packages.Package{ID: "oversized"}
	small := &packages.Package{ID: "small"}
	packages_ = []*packages.Package{oversized, small}
	bytesByPackage = map[*packages.Package]int64{
		oversized: maximumSSAPackageWaveSourceBytes + 1,
		small: 1,
	}
	waves = ssaPackageWaves(packages_, bytesByPackage)
	if len(waves) != 2 ||
		len(waves[0]) != 1 ||
		waves[0][0] != oversized ||
		len(waves[1]) != 1 ||
		waves[1][0] != small {
		t.Fatalf("oversized SSA waves = %#v", waves)
	}
}

func TestSSAPackageSourceBytesCountsEachVariantCompiledSet(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	basePath := filepath.Join(root, "sample.go")
	testPath := filepath.Join(root, "sample_test.go")
	baseFile, err := source.Load(basePath, []byte("package sample\n\nfunc Value() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	testFile, err := source.Load(testPath, []byte("package sample\n\nfunc TestValue() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	base := &packages.Package{ID: "sample", CompiledGoFiles: []string{basePath}}
	augmented := &packages.Package{
		ID: "sample [sample.test]",
		CompiledGoFiles: []string{basePath, testPath},
	}
	bytesByPackage, err := ssaPackageSourceBytes(
		[]*packages.Package{base, augmented},
		PackageSourceSet{
			paths: []string{basePath, testPath},
			files: map[string]*source.File{basePath: baseFile, testPath: testFile},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bytesByPackage[base] != baseFile.ByteSize() ||
		bytesByPackage[augmented] != baseFile.ByteSize() + testFile.ByteSize() {
		t.Fatalf("SSA package source bytes = %#v", bytesByPackage)
	}
}
