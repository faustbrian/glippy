// Package doc provides the language-neutral document model used by the Gox
// formatter. The Phase 0 surface is intentionally small while the renderer's
// bounded behavior is proven.
package doc

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ID identifies an immutable document node owned by one Arena.
type ID struct {
	arena *Arena
	index uint32
}

// Options controls deterministic rendering.
type Options struct {
	Width     int
	TabWidth  int
	FitBudget int
}

type kind uint8

const (
	kindEmpty kind = iota
	kindInvalid
	kindText
	kindVerbatim
	kindConcat
	kindGroup
	kindGroupWithIndependentTail
	kindIndent
	kindSoftLine
	kindLine
	kindHardLine
	kindIfBreak
	kindLineSuffix
	kindLineSuffixBoundary
	kindBreakParent
	kindSourceMarker
)

type node struct {
	kind            kind
	text            string
	flatWidth       int
	flatKnown       bool
	columnSensitive bool
	forcesBreak     bool
	children        []ID
	first           ID
	second          ID
	third           ID
	mark            SourceMark
}

// SourceMark identifies a physical source offset carried through rendering.
type SourceMark struct {
	Offset int
}

// RenderedMarker maps a source marker to a byte offset in rendered output.
type RenderedMarker struct {
	Source       SourceMark
	OutputOffset int
}

// RenderResult contains rendered text and ordered source marker mappings.
type RenderResult struct {
	Text    string
	Markers []RenderedMarker
}

// Arena owns document nodes. Nodes never change after they are appended, so a
// completed document may be rendered concurrently while the Arena is no
// longer being built.
type Arena struct {
	nodes []node
}

// NewArena constructs an Arena containing the shared empty document.
func NewArena() *Arena {
	arena := &Arena{}
	arena.initialize()
	return arena
}

// Empty returns a document that emits no content.
func (a *Arena) Empty() ID {
	a.initialize()
	return ID{arena: a}
}

// Text returns a literal text document.
func (a *Arena) Text(value string) ID {
	if value == "" {
		return a.Empty()
	}
	columnSensitive := strings.ContainsAny(value, "\t\r\n")
	return a.append(node{
		kind:            kindText,
		text:            value,
		flatWidth:       utf8.RuneCountInString(value),
		flatKnown:       !columnSensitive,
		columnSensitive: columnSensitive,
	})
}

// Verbatim returns content whose bytes, including embedded newlines, are
// emitted exactly. It is used for source tokens such as multiline raw strings,
// not for ordinary layout text.
func (a *Arena) Verbatim(value string) ID {
	if value == "" {
		return a.Empty()
	}
	columnSensitive := strings.ContainsAny(value, "\t\r\n")
	return a.append(node{
		kind:            kindVerbatim,
		text:            value,
		flatWidth:       utf8.RuneCountInString(value),
		flatKnown:       !columnSensitive,
		columnSensitive: columnSensitive,
	})
}

// Concat returns the ordered concatenation of parts.
func (a *Arena) Concat(parts ...ID) ID {
	filtered := make([]ID, 0, len(parts))
	forcesBreak := false
	flatWidth := 0
	flatKnown := true
	for _, part := range parts {
		if !a.valid(part) {
			return a.append(node{kind: kindInvalid, forcesBreak: true})
		}
		if part != a.Empty() {
			filtered = append(filtered, part)
			partNode := a.nodes[part.index]
			forcesBreak = forcesBreak || partNode.forcesBreak
			flatKnown = flatKnown && partNode.flatKnown
			flatWidth = saturatedWidthSum(flatWidth, partNode.flatWidth)
		}
	}
	if len(filtered) == 0 {
		return a.Empty()
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return a.append(node{
		kind:        kindConcat,
		children:    filtered,
		flatWidth:   flatWidth,
		flatKnown:   flatKnown,
		forcesBreak: forcesBreak,
	})
}

// Group selects a flat form when it fits and a broken form otherwise.
func (a *Arena) Group(body ID) ID {
	if !a.valid(body) {
		return a.append(node{kind: kindInvalid, forcesBreak: true})
	}
	bodyNode := a.nodes[body.index]
	return a.append(node{
		kind:        kindGroup,
		first:       body,
		flatWidth:   bodyNode.flatWidth,
		flatKnown:   bodyNode.flatKnown,
		forcesBreak: bodyNode.forcesBreak,
	})
}

// GroupWithIndependentTail selects body layout using only body and lookahead,
// then renders tail with an independent layout decision. A broken body gives
// tail one continuation indentation level.
func (a *Arena) GroupWithIndependentTail(body, lookahead, tail ID) ID {
	if !a.valid(body) || !a.valid(lookahead) || !a.valid(tail) {
		return a.append(node{kind: kindInvalid, forcesBreak: true})
	}
	bodyNode := a.nodes[body.index]
	tailNode := a.nodes[tail.index]
	return a.append(node{
		kind:        kindGroupWithIndependentTail,
		first:       body,
		second:      lookahead,
		third:       tail,
		flatWidth:   saturatedWidthSum(bodyNode.flatWidth, tailNode.flatWidth),
		flatKnown:   bodyNode.flatKnown && tailNode.flatKnown,
		forcesBreak: bodyNode.forcesBreak || tailNode.forcesBreak || a.nodes[lookahead.index].forcesBreak,
	})
}

// Indent increases logical indentation after line breaks within body.
func (a *Arena) Indent(body ID) ID {
	if !a.valid(body) {
		return a.append(node{kind: kindInvalid, forcesBreak: true})
	}
	bodyNode := a.nodes[body.index]
	return a.append(node{
		kind:        kindIndent,
		first:       body,
		flatWidth:   bodyNode.flatWidth,
		flatKnown:   bodyNode.flatKnown,
		forcesBreak: bodyNode.forcesBreak,
	})
}

// SoftLine emits nothing in flat mode and a newline in broken mode.
func (a *Arena) SoftLine() ID {
	return a.append(node{kind: kindSoftLine, flatKnown: true})
}

// Line emits one space in flat mode and a newline in broken mode.
func (a *Arena) Line() ID {
	return a.append(node{kind: kindLine, flatWidth: 1, flatKnown: true})
}

// HardLine always emits a newline and prevents an enclosing group from
// flattening across it.
func (a *Arena) HardLine() ID {
	return a.append(node{kind: kindHardLine})
}

// IfBreak selects broken when the current group is broken and flat otherwise.
func (a *Arena) IfBreak(broken, flat ID) ID {
	if !a.valid(broken) || !a.valid(flat) {
		return a.append(node{kind: kindInvalid, forcesBreak: true})
	}
	return a.append(node{
		kind:        kindIfBreak,
		first:       broken,
		second:      flat,
		flatWidth:   a.nodes[flat.index].flatWidth,
		flatKnown:   a.nodes[flat.index].flatKnown,
		forcesBreak: a.nodes[broken.index].forcesBreak || a.nodes[flat.index].forcesBreak,
	})
}

// LineSuffix defers body until immediately before the next rendered line
// boundary or the end of the document.
func (a *Arena) LineSuffix(body ID) ID {
	if !a.valid(body) {
		return a.append(node{kind: kindInvalid, forcesBreak: true})
	}
	return a.append(node{kind: kindLineSuffix, first: body, forcesBreak: a.nodes[body.index].forcesBreak})
}

// LineSuffixBoundary emits a line boundary when a suffix is pending.
func (a *Arena) LineSuffixBoundary() ID {
	return a.append(node{kind: kindLineSuffixBoundary})
}

// BreakParent prevents an enclosing group from selecting its flat form.
func (a *Arena) BreakParent() ID {
	return a.append(node{kind: kindBreakParent, forcesBreak: true})
}

// SourceMarker records the current rendered byte offset without emitting text.
func (a *Arena) SourceMarker(mark SourceMark) ID {
	return a.append(node{kind: kindSourceMarker, mark: mark, flatKnown: true})
}

func (a *Arena) append(value node) ID {
	a.initialize()
	a.nodes = append(a.nodes, value)
	return ID{arena: a, index: uint32(len(a.nodes) - 1)}
}

func (a *Arena) initialize() {
	if a.nodes == nil {
		a.nodes = []node{{kind: kindEmpty, flatKnown: true}}
	}
}

type layoutMode uint8

const (
	modeBroken layoutMode = iota
	modeFlat
)

type command struct {
	id     ID
	indent int
	mode   layoutMode
	action commandAction
}

type commandAction uint8

const (
	actionDocument commandAction = iota
	actionLine
)

// Render renders root without backtracking. Fit simulation is capped by the
// configured command budget; exhausting the budget chooses a conservative
// broken layout.
func (a *Arena) Render(root ID, options Options) (string, error) {
	result, err := a.RenderWithMarkers(root, options)
	return result.Text, err
}

// RenderWithMarkers renders root and returns physical source-to-output marker
// mappings. Output offsets are bytes.
func (a *Arena) RenderWithMarkers(root ID, options Options) (RenderResult, error) {
	if options.Width <= 0 || options.TabWidth <= 0 || options.FitBudget <= 0 {
		return RenderResult{}, errors.New("width, tab width, and fit budget must be positive")
	}
	if !a.valid(root) {
		return RenderResult{}, errors.New("document root is not owned by this arena")
	}

	var output strings.Builder
	stack := []command{{id: root, mode: modeBroken}}
	var suffixes []command
	var markers []RenderedMarker
	var pendingMarkers []SourceMark
	column := 0
	pendingIndent := -1

	flushMarkers := func() {
		for _, mark := range pendingMarkers {
			markers = append(markers, RenderedMarker{Source: mark, OutputOffset: output.Len()})
		}
		pendingMarkers = pendingMarkers[:0]
	}
	writeIndent := func() {
		if pendingIndent < 0 {
			return
		}
		output.WriteString(strings.Repeat("\t", pendingIndent))
		column = pendingIndent * options.TabWidth
		pendingIndent = -1
		flushMarkers()
	}
	writeLine := func(indent int) {
		flushMarkers()
		output.WriteByte('\n')
		column = 0
		pendingIndent = indent
	}
	scheduleLine := func(indent int) {
		stack = append(stack, command{indent: indent, action: actionLine})
		for index := len(suffixes) - 1; index >= 0; index-- {
			stack = append(stack, suffixes[index])
		}
		suffixes = suffixes[:0]
	}

	for len(stack) > 0 || len(suffixes) > 0 {
		if len(stack) == 0 {
			for index := len(suffixes) - 1; index >= 0; index-- {
				stack = append(stack, suffixes[index])
			}
			suffixes = suffixes[:0]
		}
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current.action == actionLine {
			if len(suffixes) > 0 {
				scheduleLine(current.indent)
			} else {
				writeLine(current.indent)
			}
			continue
		}
		if !a.valid(current.id) {
			return RenderResult{}, errors.New("document contains a reference not owned by this arena")
		}
		value := a.nodes[current.id.index]

		switch value.kind {
		case kindEmpty:
		case kindInvalid:
			return RenderResult{}, errors.New("document contains a reference not owned by this arena")
		case kindText:
			if strings.ContainsAny(value.text, "\r\n") {
				return RenderResult{}, errors.New("text documents cannot contain newlines; use Verbatim")
			}
			if value.text != "" {
				writeIndent()
				output.WriteString(value.text)
				if value.columnSensitive {
					column = advanceColumn(column, value.text, options.TabWidth)
				} else {
					column += value.flatWidth
				}
			}
		case kindVerbatim:
			if value.text != "" {
				writeIndent()
				output.WriteString(value.text)
				column = advanceColumn(column, value.text, options.TabWidth)
			}
		case kindConcat:
			for index := len(value.children) - 1; index >= 0; index-- {
				stack = append(stack, command{id: value.children[index], indent: current.indent, mode: current.mode})
			}
		case kindGroup:
			mode := modeFlat
			if current.mode == modeBroken && value.forcesBreak {
				mode = modeBroken
			} else if current.mode == modeBroken {
				fitColumn := column
				if pendingIndent >= 0 {
					fitColumn = pendingIndent * options.TabWidth
				}
				first := command{id: value.first, indent: current.indent, mode: modeFlat}
				if !a.fits(options.Width-fitColumn, options, stack, first) {
					mode = modeBroken
				}
			}
			stack = append(stack, command{id: value.first, indent: current.indent, mode: mode})
		case kindGroupWithIndependentTail:
			mode := modeFlat
			bodyForcesBreak := a.nodes[value.first.index].forcesBreak || a.nodes[value.second.index].forcesBreak
			if current.mode == modeBroken && bodyForcesBreak {
				mode = modeBroken
			} else if current.mode == modeBroken {
				fitColumn := column
				if pendingIndent >= 0 {
					fitColumn = pendingIndent * options.TabWidth
				}
				first := command{id: value.first, indent: current.indent, mode: modeFlat}
				lookahead := []command{{id: value.second, indent: current.indent, mode: modeFlat}}
				if !a.fits(options.Width-fitColumn, options, lookahead, first) {
					mode = modeBroken
				}
			}
			tailIndent := current.indent
			if mode == modeBroken {
				tailIndent++
			}
			stack = append(stack, command{id: value.third, indent: tailIndent, mode: modeBroken})
			stack = append(stack, command{id: value.first, indent: current.indent, mode: mode})
		case kindIndent:
			stack = append(stack, command{id: value.first, indent: current.indent + 1, mode: current.mode})
		case kindSoftLine:
			if current.mode == modeBroken {
				scheduleLine(current.indent)
			}
		case kindLine:
			if current.mode == modeFlat {
				writeIndent()
				output.WriteByte(' ')
				column++
			} else {
				scheduleLine(current.indent)
			}
		case kindHardLine:
			scheduleLine(current.indent)
		case kindIfBreak:
			selected := value.first
			if current.mode == modeFlat {
				selected = value.second
			}
			stack = append(stack, command{id: selected, indent: current.indent, mode: current.mode})
		case kindLineSuffix:
			suffixes = append(suffixes, command{id: value.first, indent: current.indent, mode: modeFlat})
		case kindLineSuffixBoundary:
			if len(suffixes) > 0 {
				scheduleLine(current.indent)
			}
		case kindBreakParent:
		case kindSourceMarker:
			if pendingIndent >= 0 {
				pendingMarkers = append(pendingMarkers, value.mark)
			} else {
				markers = append(markers, RenderedMarker{Source: value.mark, OutputOffset: output.Len()})
			}
		default:
			return RenderResult{}, fmt.Errorf("unknown document kind %d", value.kind)
		}
	}
	flushMarkers()

	return RenderResult{Text: output.String(), Markers: markers}, nil
}

func (a *Arena) fits(remaining int, options Options, continuation []command, first command) bool {
	budget := options.FitBudget
	stack := []command{first}
	continuationIndex := len(continuation) - 1
	for remaining >= 0 && (len(stack) > 0 || continuationIndex >= 0) {
		if budget == 0 {
			return false
		}
		budget--

		var current command
		if len(stack) > 0 {
			last := len(stack) - 1
			current = stack[last]
			stack = stack[:last]
		} else {
			current = continuation[continuationIndex]
			continuationIndex--
		}
		if current.action == actionLine {
			return true
		}
		if !a.valid(current.id) {
			return false
		}
		value := a.nodes[current.id.index]
		if current.mode == modeFlat && value.flatKnown {
			remaining -= value.flatWidth
			continue
		}

		switch value.kind {
		case kindEmpty:
		case kindInvalid:
			return false
		case kindText:
			if strings.ContainsAny(value.text, "\r\n") {
				return false
			}
			if value.columnSensitive {
				column := options.Width - remaining
				nextColumn := advanceColumn(column, value.text, options.TabWidth)
				remaining -= nextColumn - column
			} else {
				remaining -= value.flatWidth
			}
		case kindVerbatim:
			if strings.ContainsAny(value.text, "\r\n") {
				return false
			}
			column := options.Width - remaining
			nextColumn := advanceColumn(column, value.text, options.TabWidth)
			remaining -= nextColumn - column
		case kindConcat:
			for index := len(value.children) - 1; index >= 0; index-- {
				stack = append(stack, command{id: value.children[index], indent: current.indent, mode: current.mode})
			}
		case kindGroup:
			mode := modeFlat
			if value.forcesBreak {
				mode = modeBroken
			}
			stack = append(stack, command{id: value.first, indent: current.indent, mode: mode})
		case kindGroupWithIndependentTail:
			if current.mode == modeFlat {
				stack = append(stack, command{id: value.third, indent: current.indent, mode: modeFlat})
				stack = append(stack, command{id: value.first, indent: current.indent, mode: modeFlat})
				continue
			}
			bodyMode := modeFlat
			if a.nodes[value.first.index].forcesBreak || a.nodes[value.second.index].forcesBreak {
				bodyMode = modeBroken
			}
			tailIndent := current.indent
			if bodyMode == modeBroken {
				tailIndent++
			}
			stack = append(stack, command{id: value.third, indent: tailIndent, mode: modeBroken})
			stack = append(stack, command{id: value.first, indent: current.indent, mode: bodyMode})
		case kindIndent:
			stack = append(stack, command{id: value.first, indent: current.indent + 1, mode: current.mode})
		case kindSoftLine:
			if current.mode == modeBroken {
				return true
			}
		case kindLine:
			if current.mode == modeBroken {
				return true
			}
			remaining--
		case kindHardLine:
			if current.mode == modeBroken {
				return true
			}
			return false
		case kindIfBreak:
			selected := value.first
			if current.mode == modeFlat {
				selected = value.second
			}
			stack = append(stack, command{id: selected, indent: current.indent, mode: current.mode})
		case kindLineSuffix, kindLineSuffixBoundary:
			return false
		case kindBreakParent:
		case kindSourceMarker:
		default:
			return false
		}
	}
	return remaining >= 0
}

func (a *Arena) valid(id ID) bool {
	return id.arena == a && uint64(id.index) < uint64(len(a.nodes))
}

func advanceColumn(column int, value string, tabWidth int) int {
	for len(value) > 0 {
		runeValue, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		switch runeValue {
		case '\n', '\r':
			column = 0
		case '\t':
			column += tabWidth - column%tabWidth
		default:
			column++
		}
	}
	return column
}

func saturatedWidthSum(left, right int) int {
	maximum := int(^uint(0) >> 1)
	if right > maximum-left {
		return maximum
	}
	return left + right
}
