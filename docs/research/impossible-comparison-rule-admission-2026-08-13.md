# `impossible-comparison` Rule Admission, 2026-08-13

## Decision

Admit `impossible-comparison` to `correctness` at warning severity. The native
types-tier rule excludes generated and ill-typed packages and offers no fix.

## Defect And Existing Tools

Comparisons such as `uintValue < 0`, `uint8Value > 255`, and
`int8Value >= -128` are always false or always true. They leave dead branches,
invalidate tests, or hide an incorrect boundary operator while compiling
normally.

Staticcheck SA4003 and Clippy's `absurd_extreme_comparisons` establish mature
cross-tool precedent. Real Go repairs include dedis/kyber commit
`58f7e33b5a7396c71e6584834f7c434a17289b6f` removing a `uint64 < 0` check and
unheaded/unheaded commit `3de955bb0453786979f7d00be93ad9c5b8e0e804`
removing dead `uint8 > 255` and always-true `uint16 <= 65535` checks.

Sources inspected on 2026-08-13 were Staticcheck at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, Clippy at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`, Go 1.26.5, and those public
repair commits.

## Precision, Policy, And Fixes

The rule accepts only ordered comparisons between one runtime integer value
and one compile-time integer constant. Fixed-width signed and unsigned types
use their exact representable extrema. `int`, `uint`, and `uintptr` retain only
their architecture-independent zero minimum rule; architecture-dependent
extrema are excluded.

No fix is registered because removing the condition or changing its operator
can alter intended behavior. Exact suppressions and deterministic baselines use
the shared coordinator.

## Admission Evidence

Fixtures cover both operand orders, minimum and maximum boundaries, nearby
reachable comparisons, exact ranges, metadata, suppressions, generated files,
ill-typed packages, CLI exposure, deterministic baselines, and incremental
benchmarking. Three complete-load samples on Go 1.26.5, Darwin arm64, Apple M4
Max measured a median of 55,914,375 ns/op, 180,056 B/op, and 1,187
allocations/op.

Non-mutating comparison-catalog dogfood completed with no remaining findings
over Glippy and `go-libraries/pkg/prompts` at
`f232c8265a9c011daa38027718c44fc6507e9dcf`. The first Glippy run found and
removed an ineffective negative-bound check on the unsigned analysis-tier
enum. The prompts repository's concurrently changing dirty worktree was not
modified.
