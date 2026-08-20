# `context-cancel-leak` Rule Admission, 2026-08-13

## Decision

Admit `context-cancel-leak` to `correctness` at warning severity as a native
shared-CFG rule derived from the x/tools v0.48.0 `lostcancel` contract. It uses
the control-flow tier, excludes generated and ill-typed packages, and offers no
fix.

The standard analyzer was evaluated first. It cannot be admitted through the
current adapter at acceptable cost because its `ctrlflow` prerequisite exports
transitive `noReturn` facts. Correct fact execution makes Glippy load and
analyze dependency syntax for the complete import closure. The native rule is
therefore justified by a measured scheduler and memory requirement, not by a
desire to claim native ownership.

## Defect And Existing Tools

The cancellation function returned by `context.WithCancel`,
`WithCancelCause`, `WithTimeout`, `WithTimeoutCause`, `WithDeadline`, and
`WithDeadlineCause` releases timers, child registrations, and retained
references. Discarding it or reaching a return without using it leaks those
resources until the parent ends. The compiler accepts both cases. Go 1.26.5
enables `lostcancel` in default vet, and its fixtures include regressions for Go
issues 16143, 16230, 31856, and 64547.

No exact Staticcheck v0.8.0-rc.1 context-cancel rule was found; its SA5001
addresses early closer defers instead. Rust has no equivalent standard context
cancellation API, so Clippy offers no interchangeable frontend.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `lostcancel`, `ctrlflow`, their tests, and fixtures;
- Go 1.26.5 `go tool vet help`;
- Staticcheck v0.8.0-rc.1; and
- the current Clippy resource and async lint catalog.

## Precision And Source Behavior

The rule uses typed package identity, reports the blank identifier exactly, and
searches the shared function CFG from each cancel definition for a return path
without a use. As in `lostcancel`, returning or otherwise referencing the
cancel function counts as a transfer, and capture by a nested function literal
counts as a use. `main.main` is excluded. Lookalike methods do not report.

The shared CFG recognizes built-in panic but does not import transitive
no-return facts. A project-local helper that never returns can therefore remain
a false-positive boundary; this is documented in canonical metadata. Generated
and ill-typed packages are excluded. Suppressions and baselines use the same
deterministic product path as other native rules.

No fix is registered. Adding a defer, invoking immediately, or transferring
ownership have different lifetimes and semantics.

## Performance Root Cause And Resolution

The initial adapted benchmark reproduced one run at 2.074 seconds,
1,399,131,296 bytes, and 14,141,956 allocations. A heap allocation profile
showed the first divergence at dependency syntax and type loading: 374 source
files were retained for a one-file target, with large allocations in Glippy
token/trivia construction, `go/types`, parsing, inspector traversal, and CFG
construction.

A red integration test proved that selecting only this rule loaded 374 sources
instead of the target file. The native shared-CFG implementation makes the same
test retain one source and pass its diagnostic contract.

Five complete-load samples after the correction measured:

| Case | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| typed baseline | 162,996,700 | 5,344,929 | 43,352 |
| `context-cancel-leak` | 135,351,883 | 5,340,627 | 43,437 |
| complete five-rule batch | 141,169,708 | 5,516,414 | 44,143 |

The environment was Go 1.26.5, Darwin arm64, Apple M4 Max. Load variance
dominates latency; the allocation explosion is eliminated.

## Admission Evidence

Focused tests cover discarded cancellation, one-path leakage, safe defers,
returned cancellation, nested closure capture, `main.main`, lookalike methods,
exact ranges, dependency-source bounds, metadata, suppressions, generated and
type-error exclusion, deterministic baselines, and absence of fixes.

Non-mutating correctness lint completed with no findings over Glippy and
`go-libraries/pkg/prompts` at `633a5508c570d08b8976689a206f9df27e73ff90`.
The prompts repository head and pre-existing dirty status were unchanged.

## Later Precision

The v0.5 returned-alias cancellation work on 2026-08-20 narrows one assignment
case that this admission record conservatively counted as transfer. An exact
contracted result assigned back to the same cancellation variable now preserves
the outstanding invocation obligation. Replacement, new alias bindings, and
uncontracted helpers retain the original conservative boundary. See
[`v0.5-returned-alias-cancellation-obligations-2026-08-20.md`](v0.5-returned-alias-cancellation-obligations-2026-08-20.md).
