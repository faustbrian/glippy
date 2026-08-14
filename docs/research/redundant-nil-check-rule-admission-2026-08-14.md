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

No fix is offered until comments between the nil and length comparisons have a
dedicated retention contract.

## Admission Evidence

The focused test failed first because the rule was absent. Fixtures cover the
supported equality and ordering table, semantically distinct zero and negative
thresholds, pointer arrays, exact ranges, metadata, suppressions, generated
files, type-error behavior, and source versions. A one-iteration typed package
probe measured `54,518,292 ns/op`, including loading. Non-mutating Glippy and
`pkg/prompts` dogfood at `f0067b6dbf812c770ec663249e9abc3f2c41d1bc`
produced no diagnostics.

## Revisit Trigger

Broaden expression matching only with a shared side-effect and repeated-
evaluation proof; add a fix only with comment-preserving operand extraction.
