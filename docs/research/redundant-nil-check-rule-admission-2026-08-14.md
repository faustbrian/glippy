# `redundant-nil-check` Rule Admission, 2026-08-14

## Decision

Admit `redundant-nil-check` to the opt-in `pedantic` preset at warning severity.
The types-tier rule reports nil checks already implied by a supported `len`
comparison on the same slice, map, or channel.

## Evidence And Boundary

Staticcheck S1009 is the primary Go authority, inspected at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Go defines the length of nil slices, maps, and channels as zero. Glippy ports
the operator truth table but restricts repeated values to identical identifier
or selector chains. Pointers to arrays, type parameters, calls, and indexing
remain excluded.

The `remove-redundant-nil-check` safe fix replaces the complete conjunction or
disjunction with the exact source bytes of its length comparison. The rewrite
is withheld when any comment inside the removed expression falls outside the
retained comparison range. Comments inside the retained length comparison
remain byte-identical before formatter normalization.

## Admission Evidence

The focused test failed first because the rule was absent. Fixtures cover the
supported equality and ordering table, semantically distinct zero and negative
thresholds, pointer arrays, exact ranges, metadata, suppressions, generated
files, type-error behavior, and source versions. A one-iteration typed package
probe measured `54,518,292 ns/op`, including loading. Non-mutating Glippy and
`pkg/prompts` dogfood at `f0067b6dbf812c770ec663249e9abc3f2c41d1bc`
produced no diagnostics. A 2026-08-15 fixability revisit additionally proves
safe-fix metadata, exact replacement text, removed-comment refusal,
retained-comment preservation, complete-file formatting, validated write, and
second-run idempotency. Non-mutating exact-rule lint and fix preview produce no
findings or diffs on Glippy at `7844a785580b822ecfc7fc72a2f37f1a16dbebe7`
or `go-libraries/pkg/prompts` at
`5ead3d540eb6109a6bc8cfc2a2449640cb847108`.

## Revisit Trigger

Broaden expression matching only with a shared side-effect and repeated-
evaluation proof. Revisit safe classification if supported expressions expand
beyond direct identifier and selector chains.
