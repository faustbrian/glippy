package format

import (
	"bytes"
	"os"
	"testing"

	"github.com/faustbrian/glippy/internal/source"
)

func TestRenderUsesBoundedArenaCapacityHint(t *testing.T) {
	input, err := os.ReadFile("../../benchmarks/testdata/workload/hostile.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.Load("hostile.go", input)
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Width: 100, TabWidth: 8, FitBudget: 10_000}
	tokens := file.Tokens()
	baseline, err := renderFileWithCapacity(file, options, tokens, 1)
	if err != nil {
		t.Fatal(err)
	}
	hinted, err := render(file, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hinted, baseline) {
		t.Fatal("arena capacity hint changed formatted output")
	}

	allocatedBytes := func(renderOne func() error) int64 {
		result := testing.Benchmark(
			func(b *testing.B) {
				for b.Loop() {
					if err := renderOne(); err != nil {
						b.Fatal(err)
					}
				}
			},
		)
		return result.AllocedBytesPerOp()
	}
	baselineBytes := allocatedBytes(
		func() error {
			_, err := renderFileWithCapacity(file, options, tokens, 1)
			return err
		},
	)
	hintedBytes := allocatedBytes(
		func() error {
			_, err := render(file, options)
			return err
		},
	)
	if hintedBytes * 100 > baselineBytes * 80 {
		t.Fatalf(
			"capacity-aware render allocated %d bytes/op, want at most 80%% of %d",
			hintedBytes,
			baselineBytes,
		)
	}
}
