# `atomic-update-assignment` Rule Admission, 2026-08-13

## Decision

Admit `atomic-update-assignment` to `correctness` at warning severity by
adapting x/tools v0.48.0 `atomic.Analyzer`. It uses the types tier, excludes
generated files, follows the analyzer's `RunDespiteErrors` policy, and offers
no fix.

## Defect And Existing Tools

`atomic.AddUint64` and its legacy siblings already update the pointed-to value
atomically and return the new value. Assigning that result back through the
same pointer adds a second non-atomic store and defeats the synchronization
contract. The compiler accepts it. Go 1.26.5 enables `atomic` in default vet.

The upstream analyzer covers `AddInt32`, `AddInt64`, `AddUint32`, `AddUint64`,
and `AddUintptr` assignment forms and excludes ordinary use of the returned
value. Staticcheck SA1027 concerns 64-bit alignment on 32-bit platforms, not
this double-write defect. Rust atomic APIs and Clippy's atomic rules have
different return and ownership contracts, so no Clippy rule is reused.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `go/analysis/passes/atomic` source and tests;
- Go 1.26.5 `go tool vet help`;
- Staticcheck v0.8.0-rc.1 SA1027 and concurrency checks; and
- the current Clippy atomic lint catalog.

## Precision, Policy, And Fixes

The analyzer uses typed standard-library identity and exact left/right target
matching. Typed atomic wrapper methods are not covered. Glippy maps the
upstream diagnostic range and applies deterministic suppressions and baselines.
Generated files are excluded; ill-typed packages remain eligible under the
upstream contract.

No fix is registered. Although deleting the outer assignment is usually the
repair, code may instead intend to store the returned value elsewhere, so the
target cannot be selected without developer intent.

## Evidence And Cost

Focused CLI fixtures report the double write and accept an unassigned atomic
update. Policy tests cover exact suppressions, generated exclusion, type-error
execution, deterministic baselines, and absence of fixes.

| Case | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| typed baseline | 162,996,700 | 5,344,929 | 43,352 |
| `atomic-update-assignment` | 142,972,083 | 5,379,728 | 43,370 |

These are five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max;
load variance dominates. Non-mutating correctness lint completed with no
findings over Glippy and `go-libraries/pkg/prompts` at
`633a5508c570d08b8976689a206f9df27e73ff90`, without changing the external
worktree.
