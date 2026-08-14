# `time-since` Rule Admission, 2026-08-14

## Decision

Admit `time-since` to the opt-in `pedantic` preset at warning severity. The
types-tier rule reports standard `time.Now().Sub(start)` calls.

## Evidence And Boundary

Staticcheck S1012 is the primary Go authority, inspected at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Glippy resolves `time.Now` and `time.Time.Sub` through `go/types`, preserves an
import alias, and excludes lookalike APIs.

The `use-time-since` fix is suggestion-only. It retains the original argument
source and refuses a rewrite when removed call syntax owns comments.

## Admission Evidence

The focused test failed first because the rule was absent. Exact ranges,
aliases, lookalikes, metadata, shared policy, typed validation, CLI application,
and repeated fixed-point behavior pass. A one-iteration typed package probe
measured `152,643,458 ns/op`, including loading. Non-mutating Glippy and
`pkg/prompts` dogfood at `f0067b6dbf812c770ec663249e9abc3f2c41d1bc`
produced no diagnostics.

## Revisit Trigger

Revisit only if the standard library changes monotonic-time behavior or a
comment-preserving rewrite can cover currently refused layouts.
