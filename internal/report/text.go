package report

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/faustbrian/gox/internal/analysis"
	"github.com/faustbrian/gox/internal/source"
)

// LintTextInput binds one analysis result to the source used for locations.
type LintTextInput struct {
	File   *source.File
	Result analysis.Result
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
