// Package diff renders deterministic, bounded unified source differences.
package diff

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const (
	contextLines = 3
	maximumLCSCells = 1_000_000
	maximumWork = 4_000_000
	maximumDepth = 128
)

type operationKind uint8

const (
	operationEqual operationKind = iota
	operationDelete
	operationInsert
)

type operation struct {
	kind operationKind
	line string
}

type differ struct {
	before []string
	after []string
	work int
}

// Unified returns a three-line-context unified diff. Labels are quoted when
// they contain characters that would make a header span multiple fields or
// lines. Adversarial regions fall back to one replacement after bounded work.
func Unified(beforeLabel, afterLabel string, before, after []byte) string {
	if bytes.Equal(before, after) {
		return ""
	}
	d := differ{before: splitLines(string(before)), after: splitLines(string(after))}
	operations := d.rangeOperations(0, len(d.before), 0, len(d.after), 0)
	return render(quoteLabel(beforeLabel), quoteLabel(afterLabel), operations)
}

func (d *differ) rangeOperations(
	beforeStart, beforeEnd, afterStart, afterEnd, depth int,
) []operation {
	result := make([]operation, 0, beforeEnd - beforeStart + afterEnd - afterStart)
	for beforeStart < beforeEnd &&
		afterStart < afterEnd &&
		d.before[beforeStart] == d.after[afterStart] {
		result = append(
			result,
			operation{kind: operationEqual, line: d.before[beforeStart]},
		)
		beforeStart++
		afterStart++
	}
	beforeSuffix := beforeEnd
	afterSuffix := afterEnd
	for beforeStart < beforeSuffix &&
		afterStart < afterSuffix &&
		d.before[beforeSuffix - 1] == d.after[afterSuffix - 1] {
		beforeSuffix--
		afterSuffix--
	}
	if beforeStart == beforeSuffix {
		result = appendLines(result, operationInsert, d.after[afterStart:afterSuffix])
		return appendLines(result, operationEqual, d.before[beforeSuffix:beforeEnd])
	}
	if afterStart == afterSuffix {
		result = appendLines(result, operationDelete, d.before[beforeStart:beforeSuffix])
		return appendLines(result, operationEqual, d.before[beforeSuffix:beforeEnd])
	}

	d.work += beforeSuffix - beforeStart + afterSuffix - afterStart
	if depth >= maximumDepth || d.work > maximumWork {
		result = appendLines(result, operationDelete, d.before[beforeStart:beforeSuffix])
		result = appendLines(result, operationInsert, d.after[afterStart:afterSuffix])
		return appendLines(result, operationEqual, d.before[beforeSuffix:beforeEnd])
	}
	anchors := d.patienceAnchors(beforeStart, beforeSuffix, afterStart, afterSuffix)
	if len(anchors) == 0 {
		result = append(
			result,
			d.boundedLCS(beforeStart, beforeSuffix, afterStart, afterSuffix)...,
		)
		return appendLines(result, operationEqual, d.before[beforeSuffix:beforeEnd])
	}
	previousBefore, previousAfter := beforeStart, afterStart
	for _, anchor := range anchors {
		result = append(
			result,
			d.rangeOperations(
				previousBefore,
				anchor.before,
				previousAfter,
				anchor.after,
				depth + 1,
			)...,
		)
		result = append(
			result,
			operation{kind: operationEqual, line: d.before[anchor.before]},
		)
		previousBefore, previousAfter = anchor.before + 1, anchor.after + 1
	}
	result = append(
		result,
		d.rangeOperations(
			previousBefore,
			beforeSuffix,
			previousAfter,
			afterSuffix,
			depth + 1,
		)...,
	)
	return appendLines(result, operationEqual, d.before[beforeSuffix:beforeEnd])
}

type linePosition struct {
	count int
	index int
}

type anchor struct {
	before int
	after int
}

func (d *differ) patienceAnchors(beforeStart, beforeEnd, afterStart, afterEnd int) []anchor {
	beforePositions := make(map[string]linePosition, beforeEnd - beforeStart)
	for index := beforeStart; index < beforeEnd; index++ {
		position := beforePositions[d.before[index]]
		position.count++
		position.index = index
		beforePositions[d.before[index]] = position
	}
	afterPositions := make(map[string]linePosition, afterEnd - afterStart)
	for index := afterStart; index < afterEnd; index++ {
		position := afterPositions[d.after[index]]
		position.count++
		position.index = index
		afterPositions[d.after[index]] = position
	}
	candidates := make([]anchor, 0)
	for index := beforeStart; index < beforeEnd; index++ {
		beforePosition := beforePositions[d.before[index]]
		afterPosition, exists := afterPositions[d.before[index]]
		if beforePosition.count == 1 && exists && afterPosition.count == 1 {
			candidates = append(
				candidates,
				anchor{before: index, after: afterPosition.index},
			)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	tails := make([]int, 0, len(candidates))
	previous := make([]int, len(candidates))
	for index, candidate := range candidates {
		low, high := 0, len(tails)
		for low < high {
			middle := low + (high - low) / 2
			if candidates[tails[middle]].after < candidate.after {
				low = middle + 1
			} else {
				high = middle
			}
		}
		previous[index] = -1
		if low > 0 {
			previous[index] = tails[low - 1]
		}
		if low == len(tails) {
			tails = append(tails, index)
		} else {
			tails[low] = index
		}
	}
	anchors := make([]anchor, len(tails))
	for candidateIndex, outputIndex := tails[len(tails) - 1], len(anchors) - 1;
		outputIndex >= 0;
		outputIndex-- {
		anchors[outputIndex] = candidates[candidateIndex]
		candidateIndex = previous[candidateIndex]
	}
	return anchors
}

func (d *differ) boundedLCS(beforeStart, beforeEnd, afterStart, afterEnd int) []operation {
	beforeCount, afterCount := beforeEnd - beforeStart, afterEnd - afterStart
	if beforeCount > maximumLCSCells / (afterCount + 1) ||
		(beforeCount + 1) * (afterCount + 1) > maximumLCSCells {
		result := appendLines(nil, operationDelete, d.before[beforeStart:beforeEnd])
		return appendLines(result, operationInsert, d.after[afterStart:afterEnd])
	}
	columns := afterCount + 1
	cells := (beforeCount + 1) * columns
	if cells > maximumWork - d.work {
		result := appendLines(nil, operationDelete, d.before[beforeStart:beforeEnd])
		return appendLines(result, operationInsert, d.after[afterStart:afterEnd])
	}
	d.work += cells
	matrix := make([]uint32, cells)
	for beforeIndex := beforeCount - 1; beforeIndex >= 0; beforeIndex-- {
		for afterIndex := afterCount - 1; afterIndex >= 0; afterIndex-- {
			cell := beforeIndex * columns + afterIndex
			if d.before[beforeStart + beforeIndex] == d.after[afterStart + afterIndex] {
				matrix[cell] = matrix[(beforeIndex + 1) * columns +
					afterIndex +
					1] +
					1
			} else {
				matrix[cell] = max(
					matrix[(beforeIndex + 1) * columns + afterIndex],
					matrix[beforeIndex * columns + afterIndex + 1],
				)
			}
		}
	}
	result := make([]operation, 0, beforeCount + afterCount)
	beforeIndex, afterIndex := 0, 0
	for beforeIndex < beforeCount && afterIndex < afterCount {
		switch {
		case d.before[beforeStart + beforeIndex] == d.after[afterStart + afterIndex]:
			result = append(
				result,
				operation{
					kind: operationEqual,
					line: d.before[beforeStart + beforeIndex],
				},
			)
			beforeIndex++
			afterIndex++
		case matrix[(beforeIndex + 1) * columns + afterIndex] >=
			matrix[beforeIndex * columns + afterIndex + 1]:
			result = append(
				result,
				operation{
					kind: operationDelete,
					line: d.before[beforeStart + beforeIndex],
				},
			)
			beforeIndex++
		default:
			result = append(
				result,
				operation{
					kind: operationInsert,
					line: d.after[afterStart + afterIndex],
				},
			)
			afterIndex++
		}
	}
	result = appendLines(result, operationDelete, d.before[beforeStart + beforeIndex:beforeEnd])
	return appendLines(result, operationInsert, d.after[afterStart + afterIndex:afterEnd])
}

func appendLines(operations []operation, kind operationKind, lines []string) []operation {
	for _, line := range lines {
		operations = append(operations, operation{kind: kind, line: line})
	}
	return operations
}

func splitLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.SplitAfter(value, "\n")
	if lines[len(lines) - 1] == "" {
		lines = lines[:len(lines) - 1]
	}
	return lines
}

func render(beforeLabel, afterLabel string, operations []operation) string {
	changed := make([]int, 0)
	for index, operation := range operations {
		if operation.kind != operationEqual {
			changed = append(changed, index)
		}
	}
	if len(changed) == 0 {
		return ""
	}
	type hunk struct {
		start, end int
	}
	hunks := make([]hunk, 0)
	for _, index := range changed {
		start, end := max(
			0,
			index - contextLines,
		), min(len(operations), index + contextLines + 1)
		if len(hunks) > 0 && start <= hunks[len(hunks) - 1].end {
			hunks[len(hunks) - 1].end = max(hunks[len(hunks) - 1].end, end)
		} else {
			hunks = append(hunks, hunk{start: start, end: end})
		}
	}
	var output strings.Builder
	fmt.Fprintf(&output, "--- %s\n+++ %s\n", beforeLabel, afterLabel)
	oldLine, newLine, operationIndex := 1, 1, 0
	for _, hunk := range hunks {
		for operationIndex < hunk.start {
			advance(&oldLine, &newLine, operations[operationIndex].kind)
			operationIndex++
		}
		oldCount, newCount := countLines(operations[hunk.start:hunk.end])
		fmt.Fprintf(
			&output,
			"@@ %s %s @@\n",
			rangeLabel('-', oldLine, oldCount),
			rangeLabel('+', newLine, newCount),
		)
		for operationIndex < hunk.end {
			operation := operations[operationIndex]
			prefix := byte(' ')
			switch operation.kind {
			case operationDelete:
				prefix = '-'
			case operationInsert:
				prefix = '+'
			}
			output.WriteByte(prefix)
			output.WriteString(operation.line)
			if !strings.HasSuffix(operation.line, "\n") {
				output.WriteString("\n\\ No newline at end of file\n")
			}
			advance(&oldLine, &newLine, operation.kind)
			operationIndex++
		}
	}
	return output.String()
}

func advance(oldLine, newLine *int, kind operationKind) {
	if kind != operationInsert {
		*oldLine++
	}
	if kind != operationDelete {
		*newLine++
	}
}

func countLines(operations []operation) (int, int) {
	oldCount, newCount := 0, 0
	for _, operation := range operations {
		if operation.kind != operationInsert {
			oldCount++
		}
		if operation.kind != operationDelete {
			newCount++
		}
	}
	return oldCount, newCount
}

func rangeLabel(prefix byte, start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%c%d,0", prefix, start - 1)
	}
	if count == 1 {
		return fmt.Sprintf("%c%d", prefix, start)
	}
	return fmt.Sprintf("%c%d,%d", prefix, start, count)
}

func quoteLabel(label string) string {
	if strings.ContainsAny(label, "\t\r\n") {
		return strconv.Quote(label)
	}
	return label
}
