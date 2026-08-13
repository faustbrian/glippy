# `nil-function-comparison` Rule Admission, 2026-08-14

## Decision

Admit the Go `nilfunc` analyzer as warning-level `correctness`. A declared
function object cannot be nil, so comparing it with nil has a constant result
and usually confuses the function with a callback variable or return value.
Go 1.26.6 default `go vet` owns this check.

## Boundary

The typed x/tools v0.48.0 rule reports identifiers resolved to `*types.Func`.
Function-valued variables remain valid nil-comparison operands and do not
report. Generated files and ill-typed packages are excluded, exact suppressions
apply, and pre-1.25 sources do not select it. No fix is offered because the
intended call, variable, or condition cannot be inferred.

## Cost And Dogfood

A one-iteration package-load probe measured 120,190,542 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
