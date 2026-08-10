# ADR 0010: Standard-Input Fragment Formatting

- Status: accepted for prototype
- Date: 2026-08-09

## Context And Evidence

The product goal requires stdin fragments, but `go/parser` parses files and
`go/format` guesses fragments by wrapping source after matching parser errors.
Implicit guessing makes ambiguous source and diagnostic mapping part of an
unstated compatibility surface.

## Decision

Fragment formatting requires an explicit declaration, statement, or expression
kind. Each kind uses a fixed synthetic file wrapper, but wrapper tokens remain
outside the physical source ledger. Lowering selects and renders only the
wrapped user AST boundary; diagnostics map through an explicit byte-offset
translation. File-position directives and file-wide markers are rejected.
Expression trailing whitespace remains in the physical ledger but outside the
synthetic parentheses, separated from the wrapper close by non-line-breaking
synthetic trivia. Statement references to the named wrapper are rejected. A
keyed `goxfragment` element is rejected when syntax cannot prove that it names
a user-owned local declaration or struct field, because resolving a named
composite as a struct or map would require type information.

The complete normative contract is in `docs/spec/fragments.md`.

## Alternatives Rejected

- Infer fragment kind from parser error text: dependent on unstable messages
  and ambiguous ordering.
- Print the wrapped file and slice lines: wrapper formatting and comments make
  line counts unsafe.
- Load types to resolve named keyed composites: fragment formatting is a
  syntax-tier operation and unresolved surrounding package context would still
  make the result unreliable.
- Separate fragment parser: duplicates Go grammar without evidence.
- Defer all fragments to an editor service: stdin is the stable initial editor
  boundary.

## Consequences

Callers must name the fragment kind. Diagnostics require physical/synthetic
position mapping. Some file-level comments and directives are deliberately
unsupported in fragment mode. Statement fragments using `goxfragment` as a
key in a composite are conservatively rejected unless the key resolves to a
user-owned local declaration or syntax proves a user-owned struct type.

## Revisit Trigger

Editor adoption demonstrates a safe, deterministic inference contract with
better ergonomics, or the standard parser gains a supported fragment API.
