# `bad-bit-mask` Rule Admission, 2026-08-13

## Decision

Admit `bad-bit-mask` to `correctness` at warning severity. The native
types-tier rule excludes generated and ill-typed packages and offers no fix.

## Defect And Existing Tools

`value & 0b0010 == 0b0001` can never be true because AND cannot introduce the
missing low bit. Conversely, OR cannot remove bits required by its mask. These
conditions compile but silently make a branch constant.

Clippy's `bad_bit_mask` is the primary behavior reference and its corpus
contains both operand orders, constant folding, equality, inequality, AND, and
OR cases. Staticcheck SA4016 covers ineffective identity and erasing bitwise
operations, but not this comparison contract; Go vet has no equivalent rule.
Sources inspected on 2026-08-13 were Clippy commit
`9a73ad846274efca140b1d2ea316b830fa1fb8de`, Staticcheck commit
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Go 1.26.5.

## Precision, Policy, And Fixes

The initial rule covers equality and inequality around integer AND or OR
expressions with exactly one runtime operand, one compile-time mask, and one
compile-time comparison value. It proves that the equality is impossible from
the mask algebra, including the `value & 0` case. Ordered comparisons and merely
ineffective masks are deferred.

No fix is registered because either the mask, comparison value, or surrounding
logic may be wrong. Exact suppressions and deterministic baselines use the
shared coordinator.

## Admission Evidence

Fixtures cover AND, OR, equality, inequality, swapped operands, constant-folded
masks, possible nearby cases, exact ranges, metadata, suppressions, generated
files, ill-typed packages, CLI exposure, deterministic baselines, and
incremental benchmarking. Three complete-load samples on Go 1.26.5, Darwin
arm64, Apple M4 Max measured a median of 66,645,541 ns/op, 179,984 B/op, and
1,183 allocations/op.

Non-mutating comparison-catalog dogfood completed with no remaining findings
over Glippy and `go-libraries/pkg/prompts` at
`f232c8265a9c011daa38027718c44fc6507e9dcf`. The first Glippy run found and
removed an ineffective negative-bound check on the unsigned analysis-tier
enum. The prompts repository's concurrently changing dirty worktree was not
modified.
