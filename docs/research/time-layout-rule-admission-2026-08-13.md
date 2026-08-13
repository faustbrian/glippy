# `time-layout` Rule Admission, 2026-08-13

## Decision

Admit `time-layout` to the default `correctness` preset at warning severity by
adapting Go's authoritative `timeformat` analyzer. Its upstream replacement is
exposed as a suggestion fix, not an automatically safe fix.

## Defect And Existing Tools

Go time layouts use January 2 in the reference date. Writing `2006-02-01`
transposes the month and day tokens and silently parses or formats a different
layout. Go 1.26.5 vet enables `timeformat` by default. Admission supplies
Glippy's presets, suppressions, baselines, reporters, generated-file policy,
and deterministic fix metadata without duplicating the analyzer.

Sources inspected on 2026-08-13 were Go 1.26.5's `timeformat` analyzer and vet
catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`. The standard analyzer is the
authoritative implementation for this Go-specific reference-layout mistake.

## Precision, Policy, And Fixes

The adapted analyzer checks `time.Parse` and `time.Time.Format`, including
constant layouts. It emits an exact substring range for literal layouts and an
upstream suggestion only when a source edit is available. Generated files and
packages with type errors are excluded.

The replacement is a suggestion because a layout containing the byte sequence
may be intentionally noncanonical data even though that is unusual. Ordinary
`--fix` therefore does not apply it.

## Admission Evidence

Focused fixtures cover bad and correct layouts, exact ranges, suggestion
classification and edit text, suppressions, baselines, generated and type-error
policy, source versions, and CLI JSON output.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
`126,017,050 ns/op`, `1,237,254 B/op`, and `9,295 allocs/op` for the one-file
fixture. Package loading dominates the measurement.

Non-mutating correctness-and-suspicious dogfood completed without findings over
Glippy and `go-libraries/pkg/prompts` at
`d55cfaaf650681fdff0530d05988353570b2e16b`; the prompts head and pre-existing
dirty state were unchanged by the final run.

## Revisit Trigger

Track upstream `timeformat` diagnostics and suggested-fix messages. Any
expanded layout validation requires separate evidence and must not be hidden in
this narrow rule.
