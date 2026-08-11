// Package analysis schedules native rules over shared run-owned representations.
package analysis

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/inspector"

	"github.com/faustbrian/gox/internal/rules"
	"github.com/faustbrian/gox/internal/source"
)

type activeSyntaxRule struct {
	rule     rules.SyntaxRule
	metadata rules.Metadata
	severity rules.Severity
}

// RunSyntax executes selected syntax rules through one filtered AST traversal.
func RunSyntax(
	ctx context.Context,
	file *source.File,
	registry *rules.Registry,
	selection []rules.Selection,
) ([]rules.Diagnostic, error) {
	if ctx == nil {
		return nil, fmt.Errorf("syntax analysis requires a context")
	}
	if file == nil {
		return nil, fmt.Errorf("syntax analysis requires a source file")
	}
	if registry == nil {
		return nil, fmt.Errorf("syntax analysis requires a rule registry")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ordered := slices.Clone(selection)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].ID < ordered[right].ID
	})
	dispatch := make(map[rules.NodeKind][]activeSyntaxRule)
	interests := make(map[rules.NodeKind]struct{})
	previousID := ""
	for _, selected := range ordered {
		if selected.ID == previousID {
			return nil, fmt.Errorf("selected rule %q more than once", selected.ID)
		}
		previousID = selected.ID
		switch selected.Severity {
		case rules.SeverityWarn, rules.SeverityError:
		case rules.SeverityOff:
			return nil, fmt.Errorf("selected rule %q has disabled severity", selected.ID)
		default:
			return nil, fmt.Errorf(
				"selected rule %q has invalid severity %q",
				selected.ID,
				selected.Severity,
			)
		}
		nativeRule, found := registry.Lookup(selected.ID)
		if !found {
			return nil, fmt.Errorf("selected unknown rule %q", selected.ID)
		}
		metadata, _ := registry.Metadata(selected.ID)
		if selected.Requirement != metadata.Requirement {
			return nil, fmt.Errorf("selected rule %q requirement does not match registry", selected.ID)
		}
		if metadata.Requirement != rules.RequireSyntax {
			return nil, fmt.Errorf(
				"selected rule %q requires %s; syntax runner requires syntax rules",
				selected.ID,
				metadata.Requirement,
			)
		}
		if file.Metadata().Generated && !metadata.RunOnGenerated {
			continue
		}
		syntaxRule, ok := nativeRule.(rules.SyntaxRule)
		if !ok {
			return nil, fmt.Errorf("selected rule %q does not implement syntax execution", selected.ID)
		}
		active := activeSyntaxRule{
			rule:     syntaxRule,
			metadata: metadata,
			severity: selected.Severity,
		}
		for _, interest := range metadata.NodeInterests {
			dispatch[interest] = append(dispatch[interest], active)
			interests[interest] = struct{}{}
		}
	}
	if len(dispatch) == 0 {
		return []rules.Diagnostic{}, nil
	}

	orderedInterests := make([]rules.NodeKind, 0, len(interests))
	for interest := range interests {
		orderedInterests = append(orderedInterests, interest)
	}
	sort.Slice(orderedInterests, func(left, right int) bool {
		return orderedInterests[left] < orderedInterests[right]
	})
	filter := make([]ast.Node, 0, len(orderedInterests))
	for _, interest := range orderedInterests {
		prototype, found := rules.NodePrototype(interest)
		if !found {
			return nil, fmt.Errorf("node interest %q has no syntax prototype", interest)
		}
		filter = append(filter, prototype)
	}

	diagnostics := make([]rules.Diagnostic, 0)
	ruleContext := rules.NewContext(file)
	err := file.ReadSyntax(func(syntax *ast.File) error {
		sharedInspector := inspector.New([]*ast.File{syntax})
		var runErr error
		sharedInspector.Preorder(filter, func(node ast.Node) {
			if runErr != nil {
				return
			}
			if err := ctx.Err(); err != nil {
				runErr = err
				return
			}
			kind, found := rules.KindOf(node)
			if !found {
				runErr = fmt.Errorf("filtered syntax node %T has no stable kind", node)
				return
			}
			for _, active := range dispatch[kind] {
				findings, err := active.rule.RunSyntax(ruleContext, node)
				if err != nil {
					runErr = fmt.Errorf("%s: %w", active.metadata.ID, err)
					return
				}
				for _, finding := range findings {
					diagnostic, err := diagnosticForFinding(file, active, finding)
					if err != nil {
						runErr = fmt.Errorf("%s: %w", active.metadata.ID, err)
						return
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return OrderDiagnostics(diagnostics), nil
}

// OrderDiagnostics returns one canonically ordered diagnostic slice.
func OrderDiagnostics(diagnostics []rules.Diagnostic) []rules.Diagnostic {
	ordered := slices.Clone(diagnostics)
	sortDiagnostics(ordered)
	return ordered
}

func diagnosticForFinding(
	file *source.File,
	active activeSyntaxRule,
	finding rules.Finding,
) (rules.Diagnostic, error) {
	if strings.TrimSpace(finding.MessageKey) == "" {
		return rules.Diagnostic{}, fmt.Errorf("finding message key is required")
	}
	if strings.TrimSpace(finding.Message) == "" {
		return rules.Diagnostic{}, fmt.Errorf("finding message is required")
	}
	if _, valid := file.Slice(finding.Range); !valid {
		return rules.Diagnostic{}, fmt.Errorf("finding has invalid primary range")
	}
	for _, related := range finding.Related {
		if _, valid := file.Slice(related.Range); !valid {
			return rules.Diagnostic{}, fmt.Errorf("finding has invalid related range")
		}
	}
	fixMetadata := make(map[string]rules.FixMetadata, len(active.metadata.Fixes))
	for _, fix := range active.metadata.Fixes {
		fixMetadata[fix.Name] = fix
	}
	fixes := slices.Clone(finding.Fixes)
	seenFixes := make(map[string]struct{}, len(fixes))
	for fixIndex := range fixes {
		fix := &fixes[fixIndex]
		declared, found := fixMetadata[fix.Name]
		if !found {
			return rules.Diagnostic{}, fmt.Errorf("finding uses undeclared fix %q", fix.Name)
		}
		if fix.Safety != declared.Safety {
			return rules.Diagnostic{}, fmt.Errorf("finding fix %q safety does not match metadata", fix.Name)
		}
		if _, duplicate := seenFixes[fix.Name]; duplicate {
			return rules.Diagnostic{}, fmt.Errorf("finding repeats fix %q", fix.Name)
		}
		seenFixes[fix.Name] = struct{}{}
		fix.Edits = slices.Clone(fix.Edits)
		for _, edit := range fix.Edits {
			if _, valid := file.Slice(edit.Range); !valid {
				return rules.Diagnostic{}, fmt.Errorf("finding fix %q has invalid edit range", fix.Name)
			}
		}
	}
	sort.Slice(fixes, func(left, right int) bool { return fixes[left].Name < fixes[right].Name })
	related := slices.Clone(finding.Related)
	notes := slices.Clone(finding.Notes)
	return rules.Diagnostic{
		RuleID:     active.metadata.ID,
		Severity:   active.severity,
		MessageKey: finding.MessageKey,
		Message:    finding.Message,
		Path:       file.Path(),
		Digest:     file.Digest(),
		Range:      finding.Range,
		Related:    related,
		Notes:      notes,
		Help:       finding.Help,
		Fixes:      fixes,
	}, nil
}

func sortDiagnostics(diagnostics []rules.Diagnostic) {
	sort.SliceStable(diagnostics, func(left, right int) bool {
		first, second := diagnostics[left], diagnostics[right]
		if order := cmp.Compare(first.Path, second.Path); order != 0 {
			return order < 0
		}
		if order := bytes.Compare(first.Digest[:], second.Digest[:]); order != 0 {
			return order < 0
		}
		if first.Range.Start != second.Range.Start {
			return first.Range.Start < second.Range.Start
		}
		if first.Range.End != second.Range.End {
			return first.Range.End < second.Range.End
		}
		if order := cmp.Compare(first.RuleID, second.RuleID); order != 0 {
			return order < 0
		}
		if order := cmp.Compare(first.Severity, second.Severity); order != 0 {
			return order < 0
		}
		if order := cmp.Compare(first.MessageKey, second.MessageKey); order != 0 {
			return order < 0
		}
		if order := cmp.Compare(first.Message, second.Message); order != 0 {
			return order < 0
		}
		if order := compareRelated(first.Related, second.Related); order != 0 {
			return order < 0
		}
		if order := slices.Compare(first.Notes, second.Notes); order != 0 {
			return order < 0
		}
		if order := cmp.Compare(first.Help, second.Help); order != 0 {
			return order < 0
		}
		return compareFixes(first.Fixes, second.Fixes) < 0
	})
}

func compareRelated(left, right []rules.Related) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := cmp.Compare(left[index].Range.Start, right[index].Range.Start); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Range.End, right[index].Range.End); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Message, right[index].Message); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}

func compareFixes(left, right []rules.Fix) int {
	for index := 0; index < min(len(left), len(right)); index++ {
		if order := cmp.Compare(left[index].Name, right[index].Name); order != 0 {
			return order
		}
		if order := cmp.Compare(left[index].Safety, right[index].Safety); order != 0 {
			return order
		}
		leftEdits, rightEdits := left[index].Edits, right[index].Edits
		for edit := 0; edit < min(len(leftEdits), len(rightEdits)); edit++ {
			if order := cmp.Compare(leftEdits[edit].Range.Start, rightEdits[edit].Range.Start); order != 0 {
				return order
			}
			if order := cmp.Compare(leftEdits[edit].Range.End, rightEdits[edit].Range.End); order != 0 {
				return order
			}
			if order := cmp.Compare(leftEdits[edit].NewText, rightEdits[edit].NewText); order != 0 {
				return order
			}
		}
		if order := cmp.Compare(len(leftEdits), len(rightEdits)); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(left), len(right))
}
