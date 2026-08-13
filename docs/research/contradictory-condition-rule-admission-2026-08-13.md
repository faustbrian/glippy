# `contradictory-condition` Rule Admission, 2026-08-13

## Decision

Admit `contradictory-condition` to `correctness` at warning severity by
adapting the contradictory subset of Go's standard `bools` analyzer. The rule
uses the types tier, excludes generated and ill-typed packages, and offers no
fix.

## Defect And Existing Tools

`value == 1 && value == 2` is always false, while
`value != 1 || value != 2` is always true. The compiler accepts both. Go vet's
current `bools` analyzer already provides the authoritative conservative
implementation and treats calls and other effectful expressions as boundaries.

Sources inspected on 2026-08-13 were Go 1.26.5's vendored analyzer and tests,
the current `golang/tools` implementation at
`18332fec72972efbb8ab9881984fec2d8cfc2b58`, Staticcheck v0.8.0-rc.1, and
Clippy's boolean-expression catalog. The standard analyzer's existence and
long-lived default-vet inclusion provide the repeated-review evidence for this
defect class.

## Precision, Policy, And Fixes

Glippy filters the upstream analyzer's `suspect` diagnostics into this stable
rule ID. Redundant same-expression boolean operands remain outside this rule so
they can receive a separate catalog decision. The upstream implementation
requires the same formatted operand, constant comparisons, and one contiguous
side-effect-free boolean group.

No fix is registered because correcting a constant, operator, or operand
requires developer intent. Exact suppressions and deterministic baselines use
the shared coordinator.

## Admission Evidence

Fixtures cover contradictory `&&` and `||` chains, possible nearby chains,
effect boundaries, exact ranges, metadata, suppressions, generated files,
ill-typed packages, CLI exposure, deterministic baselines, and incremental
benchmarking. Three complete-load samples on Go 1.26.5, Darwin arm64, Apple M4
Max measured a median of 46,824,709 ns/op, 204,976 B/op, and 1,267
allocations/op.

Non-mutating comparison-catalog dogfood completed with no remaining findings
over Glippy and `go-libraries/pkg/prompts` at
`f232c8265a9c011daa38027718c44fc6507e9dcf`. The prompts repository's
concurrently changing dirty worktree was not modified.
