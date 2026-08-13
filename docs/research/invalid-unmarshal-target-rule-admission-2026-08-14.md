# `invalid-unmarshal-target` Rule Admission, 2026-08-14

## Decision

Admit the Go `unmarshal` analyzer as warning-level `correctness`. Passing a
non-pointer concrete value to recognized unmarshalling APIs returns an invalid
target error and cannot populate the caller's value. Go 1.26.6 default `go vet`
already detects the defect; Glippy supplies cohesive policy and reporting.

## Boundary

The typed rule follows x/tools v0.48.0's recognized encoding functions and
decoder methods. Pointer, interface, and type-parameter targets do not report.
Generated files and ill-typed packages are excluded, exact suppressions apply,
and source versions before Go 1.25 do not select it. No fix is offered because
taking an address can change escape and ownership behavior.

## Cost And Dogfood

A one-iteration package-load probe measured 172,709,000 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
