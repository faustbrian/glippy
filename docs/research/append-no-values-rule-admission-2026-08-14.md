# `append-no-values` Rule Admission, 2026-08-14

## Decision

Admit the Go `appends` analyzer as warning-level `correctness`. Calling the
built-in `append` with only its destination appends nothing and returns the same
slice, making the call observably ineffective. Go 1.26.6 default `go vet`
provides the detection.

## Boundary

The typed x/tools v0.48.0 rule reports only the built-in append with exactly one
argument; shadowed functions and ordinary append calls do not report. Generated
files and ill-typed packages are excluded, exact suppressions apply, and
pre-1.25 sources do not select it. No fix is offered because removal versus a
missing element argument cannot be distinguished.

## Cost And Dogfood

A one-iteration package-load probe measured 99,869,417 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
