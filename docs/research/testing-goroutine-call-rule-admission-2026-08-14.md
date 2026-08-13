# `testing-goroutine-call` Rule Admission, 2026-08-14

## Decision

Admit the Go `testinggoroutine` analyzer as warning-level `correctness`.
`Fatal`, `FailNow`, and `SkipNow` terminate only their calling goroutine, so a
worker call cannot correctly stop the test goroutine. Go 1.26.6 default
`go vet` supplies the detection.

## Boundary

The typed x/tools v0.48.0 rule follows direct and statically resolved local
goroutine calls from functions accepting `*testing.T` or `*testing.B`. Direct
calls in the test goroutine do not report. The experimental upstream subtest
flag remains disabled. Generated files and ill-typed packages are excluded,
exact suppressions apply, and pre-1.25 sources do not select it. No fix is
offered because transferring worker outcomes requires protocol design.

## Cost And Dogfood

A one-iteration package-load probe measured 181,807,709 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
