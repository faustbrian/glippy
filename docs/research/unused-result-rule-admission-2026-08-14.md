# `unused-result` Rule Admission, 2026-08-14

## Decision

Admit the Go `unusedresult` analyzer as warning-level `correctness`. Calling a
curated side-effect-free function or String/Error method as a statement
discards its only useful result. Go 1.26.6 default `go vet` provides the
authoritative default catalog.

## Boundary

The typed x/tools v0.48.0 rule uses the upstream default function and method
sets rather than guessing purity. Glippy does not expose the package-global
`funcs` or `stringmethods` flags. Calls whose results are used and arbitrary
functions outside the catalog do not report. Generated files and ill-typed
packages are excluded, exact suppressions apply, and pre-1.25 sources do not
select it. No fix guesses where a discarded value belongs.

## Cost And Dogfood

A one-iteration package-load probe measured 100,140,959 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
