# `unreachable-code` Rule Admission, 2026-08-14

## Decision

Admit the Go `unreachable` analyzer as warning-level `correctness`. Statements
after unconditional termination cannot execute and commonly preserve stale
work or conceal a control-flow error. Go 1.26.6 default `go vet` provides the
control-flow contract.

## Boundary

The typed x/tools v0.48.0 rule reports the first statement in each contiguous
unreachable region. It runs despite unrelated type errors, matching upstream,
but excludes generated files. Exact suppressions apply and pre-1.25 sources do
not select it. The upstream removal edit is suggestion-only because comments,
examples, and deliberate unreachable sentinels require review.

## Cost And Dogfood

A one-iteration package-load probe measured 110,095,333 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`. CLI tests prove formatted,
validated, repeated suggestion application.
