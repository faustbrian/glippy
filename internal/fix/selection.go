package fix

import (
	"fmt"
	"slices"
	"strings"

	"github.com/faustbrian/gox/internal/rules"
)

// SelectSafe chooses the only safe named fix offered by each diagnostic.
func SelectSafe(diagnostics []rules.Diagnostic) ([]Selection, error) {
	selections := make([]Selection, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		safeNames := make([]string, 0, 1)
		for _, offered := range diagnostic.Fixes {
			if offered.Safety == rules.FixSafe {
				safeNames = append(safeNames, offered.Name)
			}
		}
		switch len(safeNames) {
		case 0:
			continue
		case 1:
			selections = append(selections, Selection{
				Diagnostic: diagnostic,
				FixName:    safeNames[0],
			})
		default:
			slices.Sort(safeNames)
			return nil, fmt.Errorf(
				"diagnostic %q at bytes %d:%d offers multiple safe fixes: %s",
				diagnostic.RuleID,
				diagnostic.Range.Start,
				diagnostic.Range.End,
				strings.Join(safeNames, ", "),
			)
		}
	}
	return selections, nil
}
