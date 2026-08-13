# `loop-capture` Rule Admission, 2026-08-13

## Decision

Admit `loop-capture` to `correctness` at warning severity as a native types-tier
rule derived from the x/tools v0.48.0 `loopclosure` contract. It excludes
generated and ill-typed packages and offers no fix.

The standard analyzer was evaluated first but cannot satisfy Glippy's supported
Go 1.25 and Go 1.26 contract. Since Go 1.22 it skips every file using modern
loop semantics, including loops that assign variables declared outside the
loop. Those variables still have one shared identity and remain vulnerable to
later-value observation and data races when captured by escaping closures.

## Defect And Existing Tools

Variables declared by `for ... := range` and `for index := ...` have
per-iteration identity in modern Go. Variables declared outside a loop and
updated with `=` or the classic loop post statement do not. A goroutine, defer,
errgroup task, or parallel subtest that captures such a reused variable may run
after the loop advances it.

The compiler accepts the pattern. Go 1.26.5 enables `loopclosure` in default
vet, but its version gate intentionally limits reports to files before Go 1.22.
Staticcheck v0.8.0-rc.1 does not provide the missing modern-Go reused-variable
contract. Clippy's closure and async capture rules operate on Rust ownership and
borrow semantics and are not an interchangeable frontend.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `go/analysis/passes/loopclosure` source, tests, fixtures, and
  Go-version gate;
- Go 1.26.5 loop-variable semantics and default vet catalog;
- Staticcheck v0.8.0-rc.1 concurrency checks; and
- the current Clippy closure and async lint catalog.

## Precision, Policy, And Fixes

The rule uses type object identity to distinguish reused variables from modern
per-iteration declarations. It retains the standard analyzer's conservative
escape boundary: ordinary `go` and `defer` closures are checked only when the
launch is the loop body's recursively last statement. It also recognizes
`golang.org/x/sync/errgroup.Group.Go` and `testing.T.Run` closures whose
execution after `t.Parallel` may outlive the iteration. Lookalike methods do not
report.

Generated and ill-typed packages are excluded. Suppressions and deterministic
baselines use the shared product coordinator.

No fix is registered. Declaring the variable in the loop, copying it inside the
loop, or passing it as an argument can differ in scope, address identity, and
observable aliasing, so developer intent is required.

## Admission Evidence

Focused fixtures cover reused range and classic-loop variables, safe
per-iteration declarations, the conservative last-statement boundary,
errgroup tasks, parallel and serial subtests, lookalike methods, exact ranges,
metadata, suppressions, generated and type-error policy, deterministic
baselines, and absence of fixes.

Three complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured:

| Case | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| typed baseline, warm median | 199,356,588 | 5,346,288 | 43,575 |
| `loop-capture`, median | 514,997,583 | 5,352,228 | 43,603 |
| complete six-rule batch, median | 175,689,570 | 5,561,470 | 44,590 |

The first baseline sample included cold setup and is excluded from the warm
median. Latency showed substantial scheduler noise, while the independent rule
added about 6 KiB and 28 allocations relative to the warm typed baseline.
Non-mutating correctness lint completed with no findings over Glippy and
`go-libraries/pkg/prompts` at `633a5508c570d08b8976689a206f9df27e73ff90`.
The prompts repository head and pre-existing dirty status were unchanged.
