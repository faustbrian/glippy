# `redundant-closure` Rule Admission, 2026-08-14

## Decision

Admit `redundant-closure` to the opt-in `pedantic` preset at warning severity.
The types-tier rule reports a one-statement function literal that forwards all
parameters unchanged to an identically typed declared function.

## Evidence And Boundary

Clippy's redundant-closure family motivates the developer experience, while
Go-critic's current wrapper catalog was inspected at
[`325d070a6839c2f5958f2d587d466730d7ea2e3a`](https://github.com/go-critic/go-critic/commit/325d070a6839c2f5958f2d587d466730d7ea2e3a).
Neither is a semantic oracle for Go function values. Glippy therefore excludes
method values, captures, changed arguments, differing variadic signatures,
comments, and multi-statement bodies.

No fix is offered. Removing a wrapper can change stack inspection and panic
traces even when ordinary return values are identical.

## Admission Evidence

The focused test failed first because the rule was absent. Direct return and
void delegation, captures, transformed arguments, extra statements, exact
ranges, metadata, shared policy, and source-version behavior pass. A
one-iteration typed package probe measured `163,211,625 ns/op`, including
loading. Non-mutating Glippy and `pkg/prompts` dogfood at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc` produced no diagnostics.

## Revisit Trigger

Add a fix only if the product explicitly accepts stack-observation differences
for this pedantic transformation and comment ownership remains provable.
