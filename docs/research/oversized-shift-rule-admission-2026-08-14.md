# `oversized-shift` Rule Admission, 2026-08-14

## Decision

Admit the Go `shift` analyzer as warning-level `correctness`. Shifting a
non-constant integer by at least its width discards every bit and usually
signals an operand-width or count defect. Go 1.26.6 default `go vet` provides
the authoritative check.

## Boundary

The typed x/tools v0.48.0 rule requires a compile-time count and a known
non-constant integer width. Constant bit-width idioms and upstream-proven dead
branches do not report. Generated files and ill-typed packages are excluded,
exact suppressions apply, and sources before Go 1.25 do not select it. No fix
is offered because neither operand type nor intended count is inferable.

## Cost And Dogfood

A one-iteration package-load probe measured 129,685,166 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
