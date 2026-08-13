package diff

import (
	"fmt"
	"strings"
	"testing"
)

func TestDifferChargesLCSCellsToGlobalWorkBudget(t *testing.T) {
	t.Parallel()

	var before strings.Builder
	var after strings.Builder
	for region := range 7 {
		before.WriteString(strings.Repeat(fmt.Sprintf("before-%d\n", region), 800))
		after.WriteString(strings.Repeat(fmt.Sprintf("after-%d\n", region), 800))
		if region < 6 {
			anchor := fmt.Sprintf("anchor-%d\n", region)
			before.WriteString(anchor)
			after.WriteString(anchor)
		}
	}
	d := differ{before: splitLines(before.String()), after: splitLines(after.String())}

	d.rangeOperations(0, len(d.before), 0, len(d.after), 0)

	if d.work > maximumWork {
		t.Fatalf("diff work = %d, want at most %d", d.work, maximumWork)
	}
	if d.work < maximumWork - maximumLCSCells {
		t.Fatalf(
			"diff work = %d, want LCS matrix cells charged to the global budget",
			d.work,
		)
	}
}
