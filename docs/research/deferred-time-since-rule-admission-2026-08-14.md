# `deferred-time-since` Rule Admission, 2026-08-14

## Decision

Admit the Go `defers` analyzer as warning-level `correctness`. Arguments to a
deferred call are evaluated at registration, so direct `time.Since(start)`
records the wrong instant rather than function-exit latency. Go 1.26.6 default
`go vet` owns this check.

## Boundary

The typed x/tools v0.48.0 rule reports standard `time.Since` calls evaluated
inside a defer call and prunes nested function literals. Correct closure-based
defer evaluation does not report. Equivalent custom timing helpers are outside
scope. Generated files and ill-typed packages are excluded, exact suppressions
apply, and pre-1.25 sources do not select it. No fix introduces a closure
because capture and comment boundaries require review.

## Cost And Dogfood

A one-iteration package-load probe measured 143,053,875 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
