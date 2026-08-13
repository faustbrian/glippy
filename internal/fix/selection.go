package fix

import (
	"fmt"
	"slices"
	"strings"

	"github.com/faustbrian/glippy/internal/rules"
)

// SelectionOptions identifies the independently authorized fix classes.
type SelectionOptions struct {
	AllowSafe bool
	AllowSuggestion bool
	AllowUnsafe bool
}

// SelectSafe chooses the only safe named fix offered by each diagnostic.
func SelectSafe(diagnostics []rules.Diagnostic) ([]Selection, error) {
	return Select(diagnostics, SelectionOptions{AllowSafe: true})
}

// Select chooses the only authorized named fix offered by each diagnostic.
func Select(diagnostics []rules.Diagnostic, options SelectionOptions) ([]Selection, error) {
	selections := make([]Selection, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		authorizedNames := make([]string, 0, 1)
		for _, offered := range diagnostic.Fixes {
			if safetyAuthorized(offered.Safety, options) {
				authorizedNames = append(authorizedNames, offered.Name)
			}
		}
		switch len(authorizedNames) {
		case 0:
			continue
		case 1:
			selections = append(
				selections,
				Selection{Diagnostic: diagnostic, FixName: authorizedNames[0]},
			)
		default:
			slices.Sort(authorizedNames)
			return nil, fmt.Errorf(
				"diagnostic %q at bytes %d:%d offers multiple authorized fixes: %s",
				diagnostic.RuleID,
				diagnostic.Range.Start,
				diagnostic.Range.End,
				strings.Join(authorizedNames, ", "),
			)
		}
	}
	return selections, nil
}

func safetyAuthorized(safety rules.FixSafety, options SelectionOptions) bool {
	switch safety {
	case rules.FixSafe:
		return options.AllowSafe
	case rules.FixSuggestion:
		return options.AllowSuggestion
	case rules.FixUnsafe:
		return options.AllowUnsafe
	default:
		return false
	}
}
