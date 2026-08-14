package report

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/faustbrian/glippy/internal/analysis"
	"github.com/faustbrian/glippy/internal/baseline"
	fixengine "github.com/faustbrian/glippy/internal/fix"
	"github.com/faustbrian/glippy/internal/source"
)

// LintTextInput binds one analysis result to the source used for locations.
type LintTextInput struct {
	File *source.File
	Result analysis.Result
}

type LintFixTextInput struct {
	File *source.File
	ResultFile *source.File
	Result analysis.Result
	Outcome LintFixOutcome
}

const (
	textFrameTabWidth = 8
	textFrameMaxLines = 6
	textFrameMaxSourceBytes = 160
)

type textSource struct {
	file *source.File
	bytes []byte
	lineStarts []int
}

// RenderLintText renders ordered human diagnostics with bounded physical-source
// frames. Machine reporters retain their source-free contracts.
func RenderLintText(inputs []LintTextInput) ([]byte, error) {
	return renderLintText(inputs, true)
}

// RenderLintShortText renders source-free, location-oriented human diagnostics.
func RenderLintShortText(inputs []LintTextInput) ([]byte, error) {
	return renderLintText(inputs, false)
}

func renderLintText(inputs []LintTextInput, sourceFrames bool) ([]byte, error) {
	ordered := slices.Clone(inputs)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].Result.Path < ordered[right].Result.Path
		},
	)
	var output strings.Builder
	for index, input := range ordered {
		if input.File == nil {
			return nil, fmt.Errorf("lint text input %d has no source file", index)
		}
		if input.Result.Path != input.File.Path() ||
			input.Result.Digest != input.File.Digest() {
			return nil, fmt.Errorf(
				"lint text source identity does not match %q",
				input.Result.Path,
			)
		}
		if index > 0 && ordered[index - 1].Result.Path == input.Result.Path {
			return nil, fmt.Errorf(
				"duplicate lint text source path %q",
				input.Result.Path,
			)
		}
		var textFile textSource
		if sourceFrames {
			textFile = newTextSource(input.File)
		}
		for _, diagnostic := range analysis.OrderDiagnostics(input.Result.Diagnostics) {
			if diagnostic.Path != input.Result.Path ||
				diagnostic.Digest != input.Result.Digest {
				return nil, fmt.Errorf(
					"diagnostic source identity does not match %q",
					input.Result.Path,
				)
			}
			position, valid := physicalRangePosition(input.File, diagnostic.Range)
			if !valid {
				return nil, fmt.Errorf(
					"%s: diagnostic has invalid physical range",
					input.Result.Path,
				)
			}
			if sourceFrames {
				writeTextIssueSeparator(&output)
				fmt.Fprintf(
					&output,
					"%s[%s]: %s\n",
					safeHumanText(string(diagnostic.Severity)),
					safeHumanText(diagnostic.RuleID),
					safeHumanText(diagnostic.Message),
				)
				if err := writeSourceFrame(
					&output,
					textFile,
					diagnostic.Range,
					position,
				);
					err != nil {
					return nil, fmt.Errorf(
						"%s: render diagnostic frame: %w",
						input.Result.Path,
						err,
					)
				}
			} else {
				fmt.Fprintf(
					&output,
					"%s:%d:%d: %s[%s]: %s\n",
					safeHumanText(input.Result.Path),
					position.Line,
					position.Column,
					safeHumanText(string(diagnostic.Severity)),
					safeHumanText(diagnostic.RuleID),
					safeHumanText(diagnostic.Message),
				)
			}
			if sourceFrames &&
				(len(diagnostic.Related) > 0 ||
					len(diagnostic.Notes) > 0 ||
					diagnostic.Help != "" ||
					len(diagnostic.Fixes) > 0) {
				output.WriteString("   |\n")
			}
			for _, related := range diagnostic.Related {
				relatedPosition, valid := physicalRangePosition(
					input.File,
					related.Range,
				)
				if !valid {
					return nil, fmt.Errorf(
						"%s: related diagnostic has invalid physical range",
						safeHumanText(input.Result.Path),
					)
				}
				if sourceFrames {
					fmt.Fprintf(
						&output,
						"   = related: %s:%d:%d: %s\n",
						safeHumanText(input.Result.Path),
						relatedPosition.Line,
						relatedPosition.Column,
						safeHumanText(related.Message),
					)
				} else {
					fmt.Fprintf(
						&output,
						"  related %s:%d:%d: %s\n",
						safeHumanText(input.Result.Path),
						relatedPosition.Line,
						relatedPosition.Column,
						safeHumanText(related.Message),
					)
				}
			}
			for _, note := range diagnostic.Notes {
				if sourceFrames {
					fmt.Fprintf(&output, "   = note: %s\n", safeHumanText(note))
				} else {
					fmt.Fprintf(&output, "  note: %s\n", safeHumanText(note))
				}
			}
			if diagnostic.Help != "" {
				if sourceFrames {
					fmt.Fprintf(
						&output,
						"   = help: %s\n",
						safeHumanText(diagnostic.Help),
					)
				} else {
					fmt.Fprintf(
						&output,
						"  help: %s\n",
						safeHumanText(diagnostic.Help),
					)
				}
			}
			fixes := slices.Clone(diagnostic.Fixes)
			sort.Slice(
				fixes,
				func(left, right int) bool {
					return fixes[left].Name < fixes[right].Name
				},
			)
			for _, fix := range fixes {
				for _, edit := range fix.Edits {
					if !physicalRangeValid(input.File, edit.Range) {
						return nil, fmt.Errorf(
							"%s: fix edit has invalid physical range",
							input.Result.Path,
						)
					}
				}
				if sourceFrames {
					fmt.Fprintf(
						&output,
						"   = fix[%s]: %s\n",
						safeHumanText(string(fix.Safety)),
						safeHumanText(fix.Name),
					)
				} else {
					fmt.Fprintf(
						&output,
						"  fix[%s]: %s\n",
						safeHumanText(string(fix.Safety)),
						safeHumanText(fix.Name),
					)
				}
			}
		}
		for _, problem := range input.Result.SuppressionProblems {
			position, valid := physicalRangePosition(input.File, problem.Range)
			if !valid {
				return nil, fmt.Errorf(
					"%s: suppression problem has invalid physical range",
					input.Result.Path,
				)
			}
			if sourceFrames {
				writeTextIssueSeparator(&output)
				fmt.Fprintf(
					&output,
					"suppression[%s]: %s\n",
					safeHumanText(string(problem.Kind)),
					safeHumanText(problem.Message),
				)
				if err := writeSourceFrame(
					&output,
					textFile,
					problem.Range,
					position,
				);
					err != nil {
					return nil, fmt.Errorf(
						"%s: render suppression frame: %w",
						input.Result.Path,
						err,
					)
				}
			} else {
				fmt.Fprintf(
					&output,
					"%s:%d:%d: suppression[%s]: %s\n",
					safeHumanText(input.Result.Path),
					position.Line,
					position.Column,
					safeHumanText(string(problem.Kind)),
					safeHumanText(problem.Message),
				)
			}
		}
		for _, directive := range input.Result.UnusedSuppressions {
			position, valid := physicalRangePosition(input.File, directive.Range)
			if !valid {
				return nil, fmt.Errorf(
					"%s: unused suppression has invalid physical range",
					input.Result.Path,
				)
			}
			if !physicalRangeValid(input.File, directive.Target) {
				return nil, fmt.Errorf(
					"%s: suppression target has invalid physical range",
					input.Result.Path,
				)
			}
			if sourceFrames {
				writeTextIssueSeparator(&output)
				fmt.Fprintf(
					&output,
					"unused suppression[%s]",
					safeHumanText(directive.RuleID),
				)
			} else {
				fmt.Fprintf(
					&output,
					"%s:%d:%d: unused suppression[%s]",
					safeHumanText(input.Result.Path),
					position.Line,
					position.Column,
					safeHumanText(directive.RuleID),
				)
			}
			if directive.Reason != "" {
				fmt.Fprintf(&output, ": %s", safeHumanText(directive.Reason))
			}
			output.WriteByte('\n')
			if sourceFrames {
				if err := writeSourceFrame(
					&output,
					textFile,
					directive.Range,
					position,
				);
					err != nil {
					return nil, fmt.Errorf(
						"%s: render unused suppression frame: %w",
						input.Result.Path,
						err,
					)
				}
			}
		}
		if sourceFrames &&
			len(input.Result.BaselineProblems) > 0 &&
			(len(input.Result.Diagnostics) > 0 ||
				len(input.Result.SuppressionProblems) > 0 ||
				len(input.Result.UnusedSuppressions) > 0) {
			output.WriteByte('\n')
		}
		for _, problem := range input.Result.BaselineProblems {
			if problem.Kind == baseline.ProblemExpired {
				fmt.Fprintf(
					&output,
					"%s: baseline[%s]: %s/%s expired on %s (%d configured occurrence(s))\n",
					safeHumanText(input.Result.Path),
					safeHumanText(string(problem.Kind)),
					safeHumanText(problem.Entry.RuleID),
					safeHumanText(problem.Entry.MessageKey),
					safeHumanText(problem.Entry.ExpiresOn),
					problem.Remaining,
				)
				continue
			}
			fmt.Fprintf(
				&output,
				"%s: baseline[%s]: %s/%s has %d unmatched occurrence(s)\n",
				safeHumanText(input.Result.Path),
				safeHumanText(string(problem.Kind)),
				safeHumanText(problem.Entry.RuleID),
				safeHumanText(problem.Entry.MessageKey),
				problem.Remaining,
			)
		}
	}
	return []byte(output.String()), nil
}

func newTextSource(file *source.File) textSource {
	physical := file.Bytes()
	starts := []int{0}
	for index, value := range physical {
		if value == '\n' {
			starts = append(starts, index + 1)
		}
	}
	return textSource{file: file, bytes: physical, lineStarts: starts}
}

func writeTextIssueSeparator(output *strings.Builder) {
	if output.Len() > 0 {
		output.WriteByte('\n')
	}
}

func writeSourceFrame(
	output *strings.Builder,
	textFile textSource,
	sourceRange source.Range,
	position source.Position,
) error {
	if !physicalRangeValid(textFile.file, sourceRange) {
		return errors.New("invalid physical range")
	}
	lineIndex := position.Line - 1
	if lineIndex < 0 || lineIndex >= len(textFile.lineStarts) {
		return errors.New("invalid physical line")
	}
	lastOffset := sourceRange.Start
	if sourceRange.End > sourceRange.Start {
		lastOffset = sourceRange.End - 1
		for lastOffset > sourceRange.Start && !utf8.RuneStart(textFile.bytes[lastOffset]) {
			lastOffset--
		}
	}
	lastPosition, valid := textFile.file.Position(lastOffset)
	if !valid {
		return errors.New("invalid physical range end")
	}
	lastLineIndex := lastPosition.Line - 1
	lineWidth := len(strconv.Itoa(lastPosition.Line))
	fmt.Fprintf(
		output,
		"  --> %s:%d:%d\n",
		safeHumanText(textFile.file.Path()),
		position.Line,
		position.Column,
	)
	fmt.Fprintf(output, "%*s |\n", lineWidth + 1, "")
	indices := boundedFrameLineIndices(lineIndex, lastLineIndex)
	previous := lineIndex - 1
	for _, current := range indices {
		if current > previous + 1 {
			fmt.Fprintf(output, "%*s | ...\n", lineWidth + 1, "...")
		}
		lineStart, lineEnd := textFile.lineBounds(current)
		highlightStart := max(sourceRange.Start, lineStart)
		highlightEnd := min(sourceRange.End, lineEnd)
		if highlightEnd < highlightStart {
			highlightEnd = highlightStart
		}
		line, marker := renderFrameLine(
			textFile.bytes[lineStart:lineEnd],
			highlightStart - lineStart,
			highlightEnd - lineStart,
		)
		fmt.Fprintf(output, " %*d | %s\n", lineWidth, current + 1, line)
		fmt.Fprintf(output, "%*s | %s\n", lineWidth + 1, "", marker)
		previous = current
	}
	return nil
}

func (textFile textSource) lineBounds(index int) (int, int) {
	start := textFile.lineStarts[index]
	end := len(textFile.bytes)
	if index + 1 < len(textFile.lineStarts) {
		end = textFile.lineStarts[index + 1]
	}
	for end > start && (textFile.bytes[end - 1] == '\n' || textFile.bytes[end - 1] == '\r') {
		end--
	}
	return start, end
}

func boundedFrameLineIndices(first, last int) []int {
	count := last - first + 1
	if count <= textFrameMaxLines {
		result := make([]int, count)
		for index := range result {
			result[index] = first + index
		}
		return result
	}
	half := textFrameMaxLines / 2
	result := make([]int, 0, textFrameMaxLines)
	for index := range half {
		result = append(result, first + index)
	}
	for index := half; index > 0; index-- {
		result = append(result, last - index + 1)
	}
	return result
}

func renderFrameLine(physical []byte, highlightStart, highlightEnd int) (string, string) {
	highlightStart = max(0, min(highlightStart, len(physical)))
	highlightEnd = max(highlightStart, min(highlightEnd, len(physical)))
	physical, highlightStart, highlightEnd, croppedBefore, croppedAfter := boundedFrameWindow(
		physical,
		highlightStart,
		highlightEnd,
	)
	var rendered strings.Builder
	marker := make([]byte, 0, len(physical))
	if croppedBefore {
		rendered.WriteString("...")
		marker = append(marker, "   "...)
	}
	displayColumn := rendered.Len()
	selected := false
	point := highlightStart == highlightEnd
	for offset := 0; offset < len(physical); {
		if point && !selected && offset >= highlightStart {
			marker = markFrameCells(marker, displayColumn, 1)
			selected = true
		}
		value, size := utf8.DecodeRune(physical[offset:])
		if value == utf8.RuneError && size == 1 {
			cellSelected := offset >= highlightStart && offset < highlightEnd
			cell := "\\x" + fmt.Sprintf("%02x", physical[offset])
			rendered.WriteString(cell)
			marker = appendFrameMarker(marker, displayColumn, 4, cellSelected)
			selected = selected || cellSelected
			displayColumn += 4
			offset++
			continue
		}
		cell := string(value)
		width := textRuneCellWidth(value)
		switch {
		case value == '\t':
			width = textFrameTabWidth - displayColumn % textFrameTabWidth
			cell = strings.Repeat(" ", width)
		case unicode.IsControl(value) || unicode.Is(unicode.Cf, value):
			cell = strconv.QuoteRune(value)
			cell = strings.Trim(cell, "'")
			width = utf8.RuneCountInString(cell)
		}
		intersects := offset < highlightEnd && offset + size > highlightStart
		rendered.WriteString(cell)
		marker = appendFrameMarker(marker, displayColumn, width, intersects)
		selected = selected || intersects
		displayColumn += width
		offset += size
	}
	if !selected {
		marker = markFrameCells(marker, displayColumn, 1)
	}
	if croppedAfter {
		rendered.WriteString("...")
	}
	return rendered.String(), strings.TrimRight(string(marker), " ")
}

func boundedFrameWindow(
	physical []byte,
	highlightStart, highlightEnd int,
) ([]byte, int, int, bool, bool) {
	if len(physical) <= textFrameMaxSourceBytes {
		return physical, highlightStart, highlightEnd, false, false
	}
	half := textFrameMaxSourceBytes / 2
	start := max(0, highlightStart - half)
	end := min(len(physical), start + textFrameMaxSourceBytes)
	if end - start < textFrameMaxSourceBytes {
		start = max(0, end - textFrameMaxSourceBytes)
	}
	for start < len(physical) && !utf8.RuneStart(physical[start]) {
		start++
	}
	for end > start && end < len(physical) && !utf8.RuneStart(physical[end]) {
		end--
	}
	return physical[start:end], max(
		0,
		highlightStart - start,
	), min(end - start, highlightEnd - start), start > 0, end < len(physical)
}

func textRuneCellWidth(value rune) int {
	if unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) {
		return 0
	}
	if value >= 0x1100 &&
		(value <= 0x115f ||
			value == 0x2329 ||
			value == 0x232a ||
			(value >= 0x2e80 && value <= 0xa4cf && value != 0x303f) ||
			(value >= 0xac00 && value <= 0xd7a3) ||
			(value >= 0xf900 && value <= 0xfaff) ||
			(value >= 0xfe10 && value <= 0xfe19) ||
			(value >= 0xfe30 && value <= 0xfe6f) ||
			(value >= 0xff00 && value <= 0xff60) ||
			(value >= 0xffe0 && value <= 0xffe6) ||
			(value >= 0x1f300 && value <= 0x1faff) ||
			(value >= 0x20000 && value <= 0x3fffd)) {
		return 2
	}
	return 1
}

func safeHumanText(value string) string {
	for offset := 0; offset < len(value); {
		current, size := utf8.DecodeRuneInString(value[offset:])
		if current == utf8.RuneError && size == 1 ||
			unicode.IsControl(current) ||
			unicode.Is(unicode.Cf, current) {
			return escapedHumanText(value)
		}
		offset += size
	}
	return value
}

func escapedHumanText(value string) string {
	var output strings.Builder
	for offset := 0; offset < len(value); {
		current, size := utf8.DecodeRuneInString(value[offset:])
		if current == utf8.RuneError && size == 1 {
			fmt.Fprintf(&output, "\\x%02x", value[offset])
			offset++
			continue
		}
		if unicode.IsControl(current) || unicode.Is(unicode.Cf, current) {
			escaped := strconv.QuoteRune(current)
			output.WriteString(strings.Trim(escaped, "'"))
		} else {
			output.WriteRune(current)
		}
		offset += size
	}
	return output.String()
}

func appendFrameMarker(marker []byte, column, width int, selected bool) []byte {
	if width == 0 {
		if selected {
			return markFrameCells(marker, max(0, column - 1), 1)
		}
		return marker
	}
	marker = ensureFrameMarkerWidth(marker, column + width)
	if selected {
		marker = markFrameCells(marker, column, width)
	}
	return marker
}

func markFrameCells(marker []byte, column, width int) []byte {
	marker = ensureFrameMarkerWidth(marker, column + width)
	for index := column; index < column + width; index++ {
		marker[index] = '^'
	}
	return marker
}

func ensureFrameMarkerWidth(marker []byte, width int) []byte {
	if len(marker) >= width {
		return marker
	}
	return append(marker, strings.Repeat(" ", width - len(marker))...)
}

// RenderLintFixText renders remaining diagnostics and rejected-fix reasons.
func RenderLintFixText(inputs []LintFixTextInput) ([]byte, error) {
	return renderLintFixText(inputs, true)
}

// RenderLintFixShortText renders remaining source-free diagnostics and
// rejected-fix reasons.
func RenderLintFixShortText(inputs []LintFixTextInput) ([]byte, error) {
	return renderLintFixText(inputs, false)
}

func renderLintFixText(inputs []LintFixTextInput, sourceFrames bool) ([]byte, error) {
	ordered := slices.Clone(inputs)
	sort.Slice(
		ordered,
		func(left, right int) bool {
			return ordered[left].Outcome.Path < ordered[right].Outcome.Path
		},
	)
	textInputs := make([]LintTextInput, 0, len(ordered))
	for index, input := range ordered {
		if input.File == nil {
			return nil, fmt.Errorf(
				"lint fix text input %d has no original source file",
				index,
			)
		}
		if input.Outcome.Path != input.File.Path() ||
			input.Outcome.SourceDigest != input.File.Digest() {
			return nil, fmt.Errorf(
				"lint fix text source identity does not match %q",
				input.Outcome.Path,
			)
		}
		resultFile := input.ResultFile
		if resultFile == nil {
			resultFile = input.File
		}
		textInputs = append(
			textInputs,
			LintTextInput{File: resultFile, Result: input.Result},
		)
	}
	var base []byte
	var err error
	if sourceFrames {
		base, err = RenderLintText(textInputs)
	} else {
		base, err = RenderLintShortText(textInputs)
	}
	if err != nil {
		return nil, err
	}
	var output strings.Builder
	output.Write(base)
	for _, input := range ordered {
		textFile := textSource{}
		if sourceFrames {
			textFile = newTextSource(input.File)
		}
		rejected := slices.Clone(input.Outcome.Rejected)
		sort.Slice(
			rejected,
			func(left, right int) bool {
				return compareFixRejection(rejected[left], rejected[right]) < 0
			},
		)
		for _, item := range rejected {
			position, valid := physicalRangePosition(input.File, item.Range)
			if !valid {
				return nil, fmt.Errorf(
					"%s: rejected fix has invalid physical range",
					input.Outcome.Path,
				)
			}
			if sourceFrames {
				writeTextIssueSeparator(&output)
				fmt.Fprintf(
					&output,
					"rejected fix[%s/%s/%s]: %s\n",
					safeHumanText(item.RuleID),
					safeHumanText(item.FixName),
					safeHumanText(string(item.Reason)),
					safeHumanText(item.Message),
				)
				if err := writeSourceFrame(&output, textFile, item.Range, position);
					err != nil {
					return nil, fmt.Errorf(
						"%s: render rejected fix frame: %w",
						input.Outcome.Path,
						err,
					)
				}
			} else {
				fmt.Fprintf(
					&output,
					"%s:%d:%d: rejected fix[%s/%s/%s]: %s\n",
					safeHumanText(input.Outcome.Path),
					position.Line,
					position.Column,
					safeHumanText(item.RuleID),
					safeHumanText(item.FixName),
					safeHumanText(string(item.Reason)),
					safeHumanText(item.Message),
				)
			}
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
