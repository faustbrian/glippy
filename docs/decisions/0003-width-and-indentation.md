# ADR 0003: Width Measurement And Indentation

- Status: accepted for prototype
- Date: 2026-08-09

## Context And Evidence

Byte width produces poor decisions for non-ASCII source. Terminal-cell and
grapheme width require pinned Unicode data and policies for emoji, combining
marks, and ambiguous-width characters. Go's `text/tabwriter`, used by the
standard printer, measures rune cells and defaults to tab width 8.

## Decision

The initial width unit is Unicode code points. Tabs advance to the next
configured tab stop; the default tab width is 8. Indentation emits one literal
tab for each logical Go indentation level. Unbreakable over-width tokens remain
intact. Multiline raw strings and preserved multiline content are verbatim and
reset the measured column after embedded newlines.

Alignment columns are not ordinary indentation. If gofmt fixed-point evidence
requires alignment, a bounded table primitive or separately verified alignment
phase must account for its final width. Alignment-caused overflow is a
documented exception until the implementation can premeasure it safely.

## Alternatives Rejected

- Bytes: inconsistent visual treatment of ordinary Unicode identifiers and
  comments.
- Terminal cells or grapheme clusters in the initial dialect: greater visual
  accuracy but a larger versioned compatibility surface without adoption
  evidence.
- Spaces for indentation: conflicts with idiomatic Go and the recorded
  gofmt-compatible construct classes.

## Consequences

Width behavior is deterministic using the standard library. A future terminal
width mode would be a versioned user-visible formatter change, not a silent
implementation swap.

## Revisit Trigger

Corpus or editor evidence shows rune measurement is materially misleading, or
a stable pinned cell-width dependency and compatibility policy are approved.
