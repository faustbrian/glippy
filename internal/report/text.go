package report

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/gox/internal/analysis"
	fixengine "github.com/faustbrian/gox/internal/fix"
	"github.com/faustbrian/gox/internal/source"
)

// LintTextInput binds one analysis result to the source used for locations.
type LintTextInput struct {
	File   *source.File
	Result analysis.Result
}

type LintFixTextInput struct {
	File       *source.File
	ResultFile *source.File
	Result     analysis.Result
	Outcome    LintFixOutcome
}

// RenderLintText renders ordered, source-free human diagnostics.
func RenderLintText(inputs []LintTextInput) ([]byte, error) {
	ordered := slices.Clone(inputs)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].Result.Path < ordered[right].Result.Path
	})
	var output strings.Builder
	for index, input := range ordered {
		if input.File == nil {
			return nil, fmt.Errorf("lint text input %d has no source file", index)
		}
		if input.Result.Path != input.File.Path() || input.Result.Digest != input.File.Digest() {
			return nil, fmt.Errorf("lint text source identity does not match %q", input.Result.Path)
		}
		if index > 0 && ordered[index-1].Result.Path == input.Result.Path {
			return nil, fmt.Errorf("duplicate lint text source path %q", input.Result.Path)
		}
		for _, diagnostic := range analysis.OrderDiagnostics(input.Result.Diagnostics) {
			if diagnostic.Path != input.Result.Path || diagnostic.Digest != input.Result.Digest {
				return nil, fmt.Errorf("diagnostic source identity does not match %q", input.Result.Path)
			}
			position, valid := physicalRangePosition(input.File, diagnostic.Range)
			if !valid {
				return nil, fmt.Errorf("%s: diagnostic has invalid physical range", input.Result.Path)
			}
			fmt.Fprintf(
				&output,
				"%s:%d:%d: %s[%s]: %s\n",
				input.Result.Path,
				position.Line,
				position.Column,
				diagnostic.Severity,
				diagnostic.RuleID,
				diagnostic.Message,
			)
			for _, related := range diagnostic.Related {
				relatedPosition, valid := physicalRangePosition(input.File, related.Range)
				if !valid {
					return nil, fmt.Errorf("%s: related diagnostic has invalid physical range", input.Result.Path)
				}
				fmt.Fprintf(
					&output,
					"  related %s:%d:%d: %s\n",
					input.Result.Path,
					relatedPosition.Line,
					relatedPosition.Column,
					related.Message,
				)
			}
			for _, note := range diagnostic.Notes {
				fmt.Fprintf(&output, "  note: %s\n", note)
			}
			if diagnostic.Help != "" {
				fmt.Fprintf(&output, "  help: %s\n", diagnostic.Help)
			}
			fixes := slices.Clone(diagnostic.Fixes)
			sort.Slice(fixes, func(left, right int) bool {
				return fixes[left].Name < fixes[right].Name
			})
			for _, fix := range fixes {
				for _, edit := range fix.Edits {
					if !physicalRangeValid(input.File, edit.Range) {
						return nil, fmt.Errorf("%s: fix edit has invalid physical range", input.Result.Path)
					}
				}
				fmt.Fprintf(&output, "  fix[%s]: %s\n", fix.Safety, fix.Name)
			}
		}
		for _, problem := range input.Result.SuppressionProblems {
			position, valid := physicalRangePosition(input.File, problem.Range)
			if !valid {
				return nil, fmt.Errorf("%s: suppression problem has invalid physical range", input.Result.Path)
			}
			fmt.Fprintf(
				&output,
				"%s:%d:%d: suppression[%s]: %s\n",
				input.Result.Path,
				position.Line,
				position.Column,
				problem.Kind,
				problem.Message,
			)
		}
		for _, directive := range input.Result.UnusedSuppressions {
			position, valid := physicalRangePosition(input.File, directive.Range)
			if !valid {
				return nil, fmt.Errorf("%s: unused suppression has invalid physical range", input.Result.Path)
			}
			if !physicalRangeValid(input.File, directive.Target) {
				return nil, fmt.Errorf("%s: suppression target has invalid physical range", input.Result.Path)
			}
			fmt.Fprintf(
				&output,
				"%s:%d:%d: unused suppression[%s]",
				input.Result.Path,
				position.Line,
				position.Column,
				directive.RuleID,
			)
			if directive.Reason != "" {
				fmt.Fprintf(&output, ": %s", directive.Reason)
			}
			output.WriteByte('\n')
		}
	}
	return []byte(output.String()), nil
}

// RenderLintFixText renders remaining diagnostics and rejected-fix reasons.
func RenderLintFixText(inputs []LintFixTextInput) ([]byte, error) {
	ordered := slices.Clone(inputs)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Outcome.Path < ordered[right].Outcome.Path })
	textInputs := make([]LintTextInput, 0, len(ordered))
	for index, input := range ordered {
		if input.File == nil {
			return nil, fmt.Errorf("lint fix text input %d has no original source file", index)
		}
		if input.Outcome.Path != input.File.Path() || input.Outcome.SourceDigest != input.File.Digest() {
			return nil, fmt.Errorf("lint fix text source identity does not match %q", input.Outcome.Path)
		}
		resultFile := input.ResultFile
		if resultFile == nil {
			resultFile = input.File
		}
		textInputs = append(textInputs, LintTextInput{File: resultFile, Result: input.Result})
	}
	base, err := RenderLintText(textInputs)
	if err != nil {
		return nil, err
	}
	var output strings.Builder
	output.Write(base)
	for _, input := range ordered {
		rejected := slices.Clone(input.Outcome.Rejected)
		sort.Slice(rejected, func(left, right int) bool {
			return compareFixRejection(rejected[left], rejected[right]) < 0
		})
		for _, item := range rejected {
			position, valid := physicalRangePosition(input.File, item.Range)
			if !valid {
				return nil, fmt.Errorf("%s: rejected fix has invalid physical range", input.Outcome.Path)
			}
			fmt.Fprintf(
				&output,
				"%s:%d:%d: rejected fix[%s/%s/%s]: %s\n",
				input.Outcome.Path,
				position.Line,
				position.Column,
				item.RuleID,
				item.FixName,
				item.Reason,
				item.Message,
			)
		}
	}
	return []byte(output.String()), nil
}

func compareFixRejection(left, right fixengine.Rejection) int {
	if order := cmp.Compare(left.Range.Start, right.Range.Start); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Range.End, right.Range.End); order != 0 {
		return order
	}
	if order := cmp.Compare(left.RuleID, right.RuleID); order != 0 {
		return order
	}
	if order := cmp.Compare(left.FixName, right.FixName); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Reason, right.Reason); order != 0 {
		return order
	}
	return cmp.Compare(left.Message, right.Message)
}

func physicalRangePosition(file *source.File, sourceRange source.Range) (source.Position, bool) {
	if !physicalRangeValid(file, sourceRange) {
		return source.Position{}, false
	}
	start, _ := file.Position(sourceRange.Start)
	return start, true
}

func physicalRangeValid(file *source.File, sourceRange source.Range) bool {
	if _, valid := file.Slice(sourceRange); !valid {
		return false
	}
	_, startValid := file.Position(sourceRange.Start)
	_, endValid := file.Position(sourceRange.End)
	return startValid && endValid
}
