// Package fix coordinates selected lint fixes over one immutable source file.
package fix

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sort"
	"unicode/utf8"

	glippyformat "github.com/faustbrian/glippy/internal/format"
	"github.com/faustbrian/glippy/internal/rules"
	"github.com/faustbrian/glippy/internal/source"
)

// RejectionReason is one stable reason a selected fix was not applied.
type RejectionReason string

const (
	RejectionMissingFix RejectionReason = "missing-fix"
	RejectionStaleSource RejectionReason = "stale-source"
	RejectionSuggestion RejectionReason = "suggestion-not-selected"
	RejectionUnsafe RejectionReason = "unsafe"
	RejectionInvalidSafety RejectionReason = "invalid-safety"
	RejectionInvalidRange RejectionReason = "invalid-range"
	RejectionInvalidText RejectionReason = "invalid-text"
	RejectionConflict RejectionReason = "conflict"
	RejectionValidation RejectionReason = "validation"
)

// Selection chooses one named fix from one source-versioned diagnostic.
type Selection struct {
	Diagnostic rules.Diagnostic
	FixName string
}

// Options controls explicit non-default safety classes and formatter policy.
type Options struct {
	AllowSuggestion bool
	AllowUnsafe bool
	Format glippyformat.Options
	Validate func(*source.File) error
}

// Applied records one complete named fix included in the result.
type Applied struct {
	RuleID string
	FixName string
	Range source.Range
}

// Rejection records one complete named fix left unapplied.
type Rejection struct {
	RuleID string
	FixName string
	Range source.Range
	Reason RejectionReason
	Message string
}

// Result is one validated, formatter-normalized in-memory transaction.
type Result struct {
	Bytes []byte
	Applied []Applied
	Rejected []Rejection
	ImportChanges []ImportChange
}

// ImportAction identifies one coordinator-owned import operation.
type ImportAction string

const ImportRemove ImportAction = "remove"

// ImportChange records one deterministic import operation required by fixes.
type ImportChange struct {
	Action ImportAction
	Path string
	Name string
}

type candidate struct {
	diagnostic rules.Diagnostic
	fix rules.Fix
	span source.Range
}

type candidateEdit struct {
	candidate int
	edit rules.Edit
}

// Coordinate validates, coordinates, applies, reparses, and formats selected fixes.
func Coordinate(file *source.File, selections []Selection, options Options) (Result, error) {
	if file == nil {
		return Result{}, errors.New("fix coordination requires a source file")
	}
	if !file.CanFormat() {
		return Result{}, errors.New("fix coordination requires valid Go source")
	}
	if options.Format.Width <= 0 ||
		options.Format.TabWidth <= 0 ||
		options.Format.FitBudget <= 0 {
		return Result{}, errors.New("fix coordination requires valid formatter options")
	}

	input := file.Bytes()
	ordered := slices.Clone(selections)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return compareSelection(ordered[left], ordered[right]) < 0
		},
	)
	candidates := make([]candidate, 0, len(ordered))
	rejected := make([]Rejection, 0)
	for _, selected := range ordered {
		prepared, rejection := prepareCandidate(file, input, selected, options)
		if rejection != nil {
			rejected = append(rejected, *rejection)
			continue
		}
		candidates = append(candidates, prepared)
	}

	conflicts := conflictingCandidates(candidates)
	accepted := make([]candidate, 0, len(candidates))
	for index, prepared := range candidates {
		if conflicts[index] {
			rejected = append(
				rejected,
				reject(
					prepared,
					RejectionConflict,
					"selected fix conflicts with another edit",
				),
			)
			continue
		}
		accepted = append(accepted, prepared)
	}
	if len(accepted) == 0 {
		sortRejections(rejected)
		return Result{Bytes: input, Applied: []Applied{}, Rejected: rejected}, nil
	}

	edited := applyCandidates(input, accepted)
	editedFile, err := source.Load(file.Path(), edited)
	if err != nil {
		return rejectedTransaction(
			input,
			rejected,
			accepted,
			fmt.Sprintf("fixed source did not parse: %v", err),
		), nil
	}
	var importChanges []ImportChange
	edited, importChanges, err = pruneNewlyUnusedImports(file, editedFile)
	if err != nil {
		return rejectedTransaction(
			input,
			rejected,
			accepted,
			fmt.Sprintf("fixed source import coordination failed: %v", err),
		), nil
	}
	editedFile, err = source.Load(file.Path(), edited)
	if err != nil {
		return rejectedTransaction(
			input,
			rejected,
			accepted,
			fmt.Sprintf("import-coordinated fixed source did not parse: %v", err),
		), nil
	}
	formatted, err := glippyformat.File(editedFile, options.Format)
	if err != nil {
		return rejectedTransaction(
			input,
			rejected,
			accepted,
			fmt.Sprintf("fixed source did not format: %v", err),
		), nil
	}
	formattedFile, err := source.Load(file.Path(), formatted)
	if err != nil {
		return rejectedTransaction(
			input,
			rejected,
			accepted,
			fmt.Sprintf("formatted fixed source did not parse: %v", err),
		), nil
	}
	if options.Validate != nil {
		if err := options.Validate(formattedFile); err != nil {
			return rejectedTransaction(
				input,
				rejected,
				accepted,
				fmt.Sprintf("formatted fixed source failed validation: %v", err),
			), nil
		}
	}
	applied := make([]Applied, len(accepted))
	for index, prepared := range accepted {
		applied[index] = Applied{
			RuleID: prepared.diagnostic.RuleID,
			FixName: prepared.fix.Name,
			Range: prepared.span,
		}
	}
	sortApplied(applied)
	sortRejections(rejected)
	return Result{
		Bytes: formatted,
		Applied: applied,
		Rejected: rejected,
		ImportChanges: importChanges,
	}, nil
}

func prepareCandidate(
	file *source.File,
	input []byte,
	selected Selection,
	options Options,
) (candidate, *Rejection) {
	base := candidate{
		diagnostic: selected.Diagnostic,
		fix: rules.Fix{Name: selected.FixName},
		span: selected.Diagnostic.Range,
	}
	if selected.Diagnostic.Path != file.Path() || selected.Diagnostic.Digest != file.Digest() {
		rejection := reject(
			base,
			RejectionStaleSource,
			"diagnostic source identity does not match the selected file",
		)
		return candidate{}, &rejection
	}
	matched := make([]rules.Fix, 0, 1)
	for _, offered := range selected.Diagnostic.Fixes {
		if offered.Name == selected.FixName {
			matched = append(matched, offered)
		}
	}
	if len(matched) != 1 {
		rejection := reject(
			base,
			RejectionMissingFix,
			"diagnostic does not contain exactly one selected fix",
		)
		return candidate{}, &rejection
	}
	base.fix = matched[0]
	base.fix.Edits = slices.Clone(matched[0].Edits)
	if reason, message, disallowed := safetyRejection(base.fix.Safety, options); disallowed {
		rejection := reject(base, reason, message)
		return candidate{}, &rejection
	}
	if len(base.fix.Edits) == 0 {
		rejection := reject(base, RejectionInvalidRange, "selected fix contains no edits")
		return candidate{}, &rejection
	}
	span := source.Range{Start: len(input), End: 0}
	for _, edit := range base.fix.Edits {
		if !validEditRange(input, edit.Range) {
			rejection := reject(
				base,
				RejectionInvalidRange,
				"selected fix contains an invalid UTF-8 byte range",
			)
			return candidate{}, &rejection
		}
		if !utf8.ValidString(edit.NewText) {
			rejection := reject(
				base,
				RejectionInvalidText,
				"selected fix replacement is not valid UTF-8",
			)
			return candidate{}, &rejection
		}
		span.Start = min(span.Start, edit.Range.Start)
		span.End = max(span.End, edit.Range.End)
	}
	base.span = span
	return base, nil
}

func safetyRejection(safety rules.FixSafety, options Options) (RejectionReason, string, bool) {
	switch safety {
	case rules.FixSafe:
		return "", "", false
	case rules.FixSuggestion:
		if options.AllowSuggestion {
			return "", "", false
		}
		return RejectionSuggestion, "suggestion fix was not explicitly selected", true
	case rules.FixUnsafe:
		if options.AllowUnsafe {
			return "", "", false
		}
		return RejectionUnsafe, "unsafe fix requires explicit authorization", true
	default:
		return RejectionInvalidSafety, "selected fix has an invalid safety classification", true
	}
}

func validEditRange(input []byte, sourceRange source.Range) bool {
	if sourceRange.Start < 0 ||
		sourceRange.End < sourceRange.Start ||
		sourceRange.End > len(input) {
		return false
	}
	return byteBoundary(input, sourceRange.Start) && byteBoundary(input, sourceRange.End)
}

func byteBoundary(input []byte, offset int) bool {
	return offset == len(input) || utf8.RuneStart(input[offset])
}

func conflictingCandidates(candidates []candidate) []bool {
	conflicts := make([]bool, len(candidates))
	edits := make([]candidateEdit, 0)
	for candidateIndex, prepared := range candidates {
		for _, edit := range prepared.fix.Edits {
			edits = append(edits, candidateEdit{candidate: candidateIndex, edit: edit})
		}
	}
	sort.Slice(
		edits,
		func(left, right int) bool {
			first, second := edits[left], edits[right]
			if first.edit.Range.Start != second.edit.Range.Start {
				return first.edit.Range.Start < second.edit.Range.Start
			}
			if first.edit.Range.End != second.edit.Range.End {
				return first.edit.Range.End < second.edit.Range.End
			}
			if order := cmp.Compare(
				candidates[first.candidate].diagnostic.RuleID,
				candidates[second.candidate].diagnostic.RuleID,
			);
				order != 0 {
				return order < 0
			}
			return cmp.Compare(
				candidates[first.candidate].fix.Name,
				candidates[second.candidate].fix.Name,
			) <
				0
		},
	)

	insertions := make(map[int]int)
	activeCandidate := 0
	activeRange := source.Range{}
	hasActive := false
	for _, current := range edits {
		currentRange := current.edit.Range
		if hasActive && currentRange.Start >= activeRange.End {
			hasActive = false
		}
		if currentRange.Start == currentRange.End {
			if previous, found := insertions[currentRange.Start]; found {
				conflicts[previous] = true
				conflicts[current.candidate] = true
			} else {
				insertions[currentRange.Start] = current.candidate
			}
			if hasActive && currentRange.Start > activeRange.Start {
				conflicts[activeCandidate] = true
				conflicts[current.candidate] = true
			}
			continue
		}
		if hasActive && currentRange.Start < activeRange.End {
			conflicts[activeCandidate] = true
			conflicts[current.candidate] = true
		}
		if !hasActive || currentRange.End > activeRange.End {
			activeCandidate = current.candidate
			activeRange = currentRange
			hasActive = true
		}
	}
	return conflicts
}

func applyCandidates(input []byte, candidates []candidate) []byte {
	edits := make([]candidateEdit, 0)
	for candidateIndex, prepared := range candidates {
		for _, edit := range prepared.fix.Edits {
			edits = append(edits, candidateEdit{candidate: candidateIndex, edit: edit})
		}
	}
	sort.Slice(
		edits,
		func(left, right int) bool {
			first, second := edits[left], edits[right]
			if first.edit.Range.Start != second.edit.Range.Start {
				return first.edit.Range.Start > second.edit.Range.Start
			}
			if first.edit.Range.End != second.edit.Range.End {
				return first.edit.Range.End > second.edit.Range.End
			}
			if order := cmp.Compare(
				candidates[first.candidate].diagnostic.RuleID,
				candidates[second.candidate].diagnostic.RuleID,
			);
				order != 0 {
				return order < 0
			}
			return cmp.Compare(
				candidates[first.candidate].fix.Name,
				candidates[second.candidate].fix.Name,
			) <
				0
		},
	)
	result := bytes.Clone(input)
	for _, selected := range edits {
		edit := selected.edit
		next := make(
			[]byte,
			0,
			len(result) - (edit.Range.End - edit.Range.Start) + len(edit.NewText),
		)
		next = append(next, result[:edit.Range.Start]...)
		next = append(next, edit.NewText...)
		next = append(next, result[edit.Range.End:]...)
		result = next
	}
	return result
}

func rejectedTransaction(
	input []byte,
	rejected []Rejection,
	accepted []candidate,
	message string,
) Result {
	for _, prepared := range accepted {
		rejected = append(rejected, reject(prepared, RejectionValidation, message))
	}
	sortRejections(rejected)
	return Result{Bytes: input, Applied: []Applied{}, Rejected: rejected}
}

func reject(prepared candidate, reason RejectionReason, message string) Rejection {
	return Rejection{
		RuleID: prepared.diagnostic.RuleID,
		FixName: prepared.fix.Name,
		Range: prepared.span,
		Reason: reason,
		Message: message,
	}
}

func compareSelection(left, right Selection) int {
	if order := cmp.Compare(left.Diagnostic.Path, right.Diagnostic.Path); order != 0 {
		return order
	}
	if order := bytes.Compare(left.Diagnostic.Digest[:], right.Diagnostic.Digest[:]);
		order != 0 {
		return order
	}
	if order := cmp.Compare(left.Diagnostic.Range.Start, right.Diagnostic.Range.Start);
		order != 0 {
		return order
	}
	if order := cmp.Compare(left.Diagnostic.Range.End, right.Diagnostic.Range.End); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Diagnostic.RuleID, right.Diagnostic.RuleID); order != 0 {
		return order
	}
	return cmp.Compare(left.FixName, right.FixName)
}

func sortApplied(applied []Applied) {
	sort.Slice(
		applied,
		func(left, right int) bool {
			first, second := applied[left], applied[right]
			if first.Range.Start != second.Range.Start {
				return first.Range.Start < second.Range.Start
			}
			if first.Range.End != second.Range.End {
				return first.Range.End < second.Range.End
			}
			if order := cmp.Compare(first.RuleID, second.RuleID); order != 0 {
				return order < 0
			}
			return cmp.Compare(first.FixName, second.FixName) < 0
		},
	)
}

func sortRejections(rejected []Rejection) {
	sort.Slice(
		rejected,
		func(left, right int) bool {
			first, second := rejected[left], rejected[right]
			if first.Range.Start != second.Range.Start {
				return first.Range.Start < second.Range.Start
			}
			if first.Range.End != second.Range.End {
				return first.Range.End < second.Range.End
			}
			if order := cmp.Compare(first.RuleID, second.RuleID); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.FixName, second.FixName); order != 0 {
				return order < 0
			}
			if order := cmp.Compare(first.Reason, second.Reason); order != 0 {
				return order < 0
			}
			return cmp.Compare(first.Message, second.Message) < 0
		},
	)
}
