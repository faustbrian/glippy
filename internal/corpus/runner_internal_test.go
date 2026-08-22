package corpus

import (
	"math"
	"testing"
)

func TestLessFindingOrdersExtremeOffsetsWithoutOverflow(t *testing.T) {
	t.Parallel()

	lowest := finding{Range: findingRange{Start: -1, End: -1}}
	highest := finding{Range: findingRange{Start: math.MaxInt, End: math.MaxInt}}
	if !lessFinding(lowest, highest) {
		t.Fatal("lessFinding() did not order -1 before MaxInt")
	}
	if lessFinding(highest, lowest) {
		t.Fatal("lessFinding() ordered MaxInt before -1")
	}
}
