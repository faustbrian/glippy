# `inefficient-string-comparison` Rule Admission, 2026-08-14

## Decision

Admit `inefficient-string-comparison` to the opt-in `pedantic` preset at warning
severity. The types-tier rule reports equality or inequality between two
matching `strings.ToLower` calls or two matching `strings.ToUpper` calls.

## Evidence And Boundary

Go-critic's current `equalFold` rule is the primary Go reference, inspected at
[`325d070a6839c2f5958f2d587d466730d7ea2e3a`](https://github.com/go-critic/go-critic/commit/325d070a6839c2f5958f2d587d466730d7ea2e3a).
Glippy narrows that contract to two matching conversions of distinct simple
identifier or selector expressions. One-sided, mixed-case, bytes, calls, and
index expressions remain excluded.

No fix is offered because Unicode case folding is not identical to every
normalization-based comparison for all possible strings.

## Admission Evidence

The focused test failed first because the rule was absent. Equality,
inequality, mixed normalization, repeated operands, exact ranges, metadata,
shared policy, and source-version behavior pass. A one-iteration typed package
probe measured `78,452,083 ns/op`, including loading. Non-mutating Glippy and
`pkg/prompts` dogfood at `f0067b6dbf812c770ec663249e9abc3f2c41d1bc`
produced no diagnostics.

## Revisit Trigger

Broaden only when equivalence and evaluation-count behavior are proven for the
additional expression class; keep any EqualFold rewrite separately opt-in.
