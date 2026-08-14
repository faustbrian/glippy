# `time-until` Rule Admission, 2026-08-14

## Decision

Admit `time-until` to the opt-in `pedantic` preset at warning severity. The
types-tier rule reports standard `deadline.Sub(time.Now())` calls.

## Evidence And Boundary

Staticcheck S1024 is the primary Go authority, inspected at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Glippy resolves the standard objects through `go/types`, preserves the imported
package alias, and excludes other `Sub` or `Now` functions.

The `use-time-until` fix is suggestion-only. It retains the deadline expression
and refuses edits that would discard comments.

## Admission Evidence

The focused test failed first because the rule was absent. Exact ranges,
aliases, negative calls, metadata, shared policy, typed validation, CLI
application, and repeated fixed-point behavior pass. A one-iteration typed
package probe measured `142,541,250 ns/op`, including loading. Non-mutating
Glippy and `pkg/prompts` dogfood at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc` produced no diagnostics.

## Revisit Trigger

Revisit only for a standard-library semantic change or a stronger comment-
retention proof.
