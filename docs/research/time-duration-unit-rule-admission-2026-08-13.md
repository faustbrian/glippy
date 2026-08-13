# `time-duration-unit` Rule Admission, 2026-08-13

## Decision

Admit `time-duration-unit` to the opt-in `suspicious` preset at warning
severity. The native types-tier rule reports direct nonzero integer literals
used as durations in selected waiting and timer APIs.

## Defect And Existing Tools

`time.Duration` values are nanoseconds, while developers commonly transfer
seconds or milliseconds from APIs in other languages. The compiler and default
Go 1.26.5 vet accept bare literals. Staticcheck SA1004 detects small literals in
`time.Sleep`; Glippy covers `time.Sleep`, `time.After`, `time.NewTimer`,
`time.NewTicker`, `time.Tick`, and `time.AfterFunc` while keeping the rule
opt-in because literal nanoseconds can be deliberate.

Sources inspected on 2026-08-13 were Go 1.26.5's `time` API contracts and vet
catalog, Staticcheck SA1004 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`. Clippy's duration checks support
the general value of explicit units but do not define Go API identity.

## Precision, Policy, And Fixes

Package-object identity excludes lookalike functions. Zero, named constants,
computed expressions, and explicit conversions are treated as deliberate. The
initial rule reports any direct nonzero integer literal rather than guessing a
numeric threshold. Generated files and packages with type errors are excluded.

No fix is offered. Multiplying by nanoseconds, microseconds, milliseconds, or
seconds can all be plausible, so an automatic unit choice would guess intent.

## Admission Evidence

Focused fixtures cover the supported API set, explicit-unit negatives, exact
ranges, no-fix behavior, suppressions, baselines, generated and type-error
policy, source versions, and CLI JSON output.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
`137,344,933 ns/op`, `1,228,041 B/op`, and `9,279 allocs/op` for the one-file
fixture. Package loading dominates the measurement.

Non-mutating correctness-and-suspicious dogfood completed without findings over
Glippy and `go-libraries/pkg/prompts` at
`d55cfaaf650681fdff0530d05988353570b2e16b`; the prompts head and pre-existing
dirty state were unchanged by the final run.

## Revisit Trigger

Change the literal policy or add APIs only from dogfood and real-defect
evidence; never infer an intended unit for an automatic fix.
