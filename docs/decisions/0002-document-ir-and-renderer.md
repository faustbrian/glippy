# ADR 0002: Document IR And Bounded Rendering

- Status: accepted for prototype
- Date: 2026-08-09

## Context And Evidence

Width-aware Go formatting requires decisions that `go/printer` does not make.
Direct source rewriting cannot safely distinguish grammar semicolons, inserted
semicolons, comments, precedence, or required trailing commas. Oxfmt uses a
flat arena-oriented document IR and iterative printer. Local Go 1.26.5 spikes
confirmed that the motivating blocks, boolean chains, calls, function
literals, selector chains, and type unions can use stable grammar-valid broken
forms.

## Decision

The prototype uses an immutable arena of document nodes addressed by compact
IDs. Required primitives are text with cached rune width, concatenation,
group, indentation, soft line, breakable line, hard line, group-scoped
conditional content, line suffix, suffix boundary, break propagation, verbatim
text, and source markers.

The renderer is iterative. Every group has one flat form and one canonical
broken form unless a later decision documents an additional alternative. Fit
simulation is bounded by a fixed command budget; budget exhaustion
conservatively selects the broken form. The renderer never backtracks after
emitting output. Nodes MAY cache saturated flat summaries where the summary is
independent of the starting column; tabs and other column-sensitive content
require an explicit transition summary or bounded simulation. The Phase 0
prototype remains bounded without relying on a summary cache.

With `D` document commands, output size `B`, and fixed lookahead cap `K`, the
intended upper bound is `O(D*K + B)` time and `O(D + nesting + suffixes)`
memory. The prototype must verify that bound under adversarial nesting before
the decision stabilizes.

Binary operators, type-union operators, and selector dots break after the
operator. Ordinary statement boundaries and block contents use hard lines.
Broken comma lists emit required trailing commas through group-scoped
conditional content. Index and slice delimiters do not inherit that rule
without grammar-specific evidence.

## Alternatives Rejected

- `go/ast` plus `go/printer`: no adequate width model or concrete-gap
  fidelity.
- Direct strings or line rewrites: grammar and comment ownership are too easy
  to corrupt.
- Exhaustive optimal layout or unrestricted unions: adversarial exponential
  work.
- Unbounded recursive fit scans: repeated scanning and stack growth can become
  pathological.
- Pure streaming Oppen printing initially: difficult group-scoped
  conditionals, suffix comments, and source markers; revisit only if the
  bounded model proves too conservative or expensive.

## Consequences

Lowering owns grammar-specific break opportunities. Rendering remains language
neutral. Conservative extra breaks are preferable to unbounded work, invalid
syntax, or nondeterminism. Flat summaries and width caches become part of
benchmark and fuzz coverage.

## Revisit Trigger

Prototype allocation data materially favors another representation; the fit
budget causes unacceptable readable-input churn; or adversarial tests disprove
the documented bound.
