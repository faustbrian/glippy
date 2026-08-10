# ADR 0009: Comment Attachment And Directive Preservation

- Status: accepted for prototype; initial corpus proof complete
- Date: 2026-08-09

## Context And Evidence

Go AST comments are mostly free-floating and `ast.CommentMap` is heuristic.
Comment values can omit raw carriage returns, and normalized comment-group text
can remove directives. Gofmt experiments also moved a comment across a comma
boundary. A nearest-node policy therefore cannot protect source ownership or
directive semantics.

The project-owned Phase 1 corpus now binds exact width-60 output for build
constraints, generated markers, cgo preambles, `//go:embed`, `//go:generate`,
compiler directives including `//go:linkname`, `//line`, and `//gox:`
suppressions. The corpus reparses, is byte-idempotent, preserves normalized
syntax and directive identity, and is a gofmt fixed point under the recorded
prototype toolchain. Focused equivalence tests additionally reject a suppression
whose same-line or adjacent-line anchor changes even when its surrounding token
ownership is otherwise unchanged.

## Decision

Comments retain stable raw identity and attach to explicit source boundaries,
not merely AST nodes. Boundary kinds include file prefix/suffix, declaration
or field documentation, trailing node comment, before/after a statement or
declaration, between two children of one parent, inside empty delimiters, and
directive anchor.

Attachment is assigned in this deterministic order:

1. classify directives and fixed file anchors before ordinary comments;
2. bind same-line trailing comments to the preceding complete token boundary,
   while preserving required punctuation before the comment;
3. bind documentation comments to the following declaration or field only
   when adjacency and blank-line rules permit it;
4. bind comments inside delimiters to the exact gap between child indexes,
   including before the first and after the last child;
5. represent standalone comments between declarations, statements, cases, or
   operands as boundary-owned items rather than arbitrarily choosing a side;
6. use AST ownership and line relationships only to refine an already valid
   token-gap boundary; and
7. reject formatting when one input comment has zero or multiple output owners.

Build constraints, cgo preambles, generated markers, `//line`, compiler/tool
directives, and suppressions are non-movable anchors with class-specific
adjacency. Their raw bytes and relative order are preserved. If canonical
layout conflicts with a directive anchor, the directive wins or formatting is
rejected; it is never silently relocated.

Line comments render as suffixes that force a line boundary. Multiline block
comments retain bytes and participate in semicolon-insertion checks before a
break is selected. Ambiguous comment placement forces a stable broken construct
or a diagnostic rather than migration.

## Alternatives Rejected

- `ast.CommentMap` as authority: heuristic and node-only.
- Nearest AST node or nearest line: loses delimiter and statement-boundary
  ownership.
- Oxfmt-style single source cursor without a stored index: useful for ordered
  emission but insufficient for shared formatter, suppressions, fixes, and
  independent validation.
- Let gofmt repair comments afterward: observed punctuation-boundary movement
  and hidden ownership changes.

## Consequences

The source model needs explicit boundary keys and a directive registry.
Lowerers must expose child-gap identities. Comment-heavy constructs may break
more often than comment-free equivalents. Every formatter result pays an
independent identity and ownership accounting pass.

## Revisit Trigger

Dense-comment, cgo, build-constraint, line-directive, and suppression corpus
evidence demonstrates an unrepresentable placement; or measured index cost
requires a different representation without weakening ownership checks.
