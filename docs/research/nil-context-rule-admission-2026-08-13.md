# `nil-context` Rule Admission, 2026-08-13

## Decision

Admit `nil-context` to the default `correctness` preset at warning severity.
The native types-tier rule reports a direct predeclared `nil` passed to any
parameter whose static type is exactly `context.Context`.

## Defect And Existing Tools

The `context` package contract says not to pass a nil context and recommends
`context.TODO` when the appropriate context is unknown. Nil contexts can panic
inside APIs that assume the interface is usable. The compiler and default Go
1.26.5 vet accept the call. Staticcheck SA1012 detects the same defect only for
the first function parameter; Glippy extends the cohesive check to every
statically identified `context.Context` parameter.

Sources inspected on 2026-08-13 were Go 1.26.5's `context` contract and vet
catalog, Staticcheck SA1012 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`. Clippy has no direct equivalent
because this is a Go API contract.

## Precision, Policy, And Fixes

Object and type identity prevent reports for lookalike interfaces. The rule
reports only direct `nil` arguments and does not infer nilness through
variables, interface values, helper returns, or data flow. Generated files and
packages with type errors are excluded. Test files are excluded by default and
can be enabled with the typed `include-tests` option.

No fix is offered. `context.TODO`, `context.Background`, or an available parent
context can each be correct in different ownership boundaries, so selecting one
requires developer intent.

## Admission Evidence

Focused fixtures cover argument positions, valid contexts, exact ranges,
no-fix behavior, suppressions, baselines, generated and type-error policy,
source versions, and CLI JSON output.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
`133,810,942 ns/op`, `1,228,948 B/op`, and `9,285 allocs/op` for the one-file
fixture. Package loading dominates the measurement.

Initial dogfood reported only deliberate nil-input contract tests: five in
Glippy and six in `go-libraries/pkg/prompts`. That evidence established the
default test-file exclusion; both repositories retain opt-in coverage through
`include-tests`. Final non-mutating correctness-and-suspicious dogfood completed
without findings over Glippy and prompts revision
`d55cfaaf650681fdff0530d05988353570b2e16b`; the prompts head and pre-existing
dirty state were unchanged by the final run.

## Revisit Trigger

Add value-flow detection only when SSA evidence demonstrates useful precision,
reconsider test-file default policy only from lower-noise evidence, and
reconsider fixes only if a uniquely correct local parent context can be proven.
