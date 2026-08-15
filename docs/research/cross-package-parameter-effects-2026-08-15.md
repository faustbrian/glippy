# Cross-Package Parameter-Effect Evidence, 2026-08-15

## Scope

Effect schema version 2 adds stable per-parameter summaries for statically
resolved same-module helpers. The first consumers are
`resource-not-closed`, `http-response-body-not-closed`,
`sql-transaction-not-completed`, and `context-cancel-leak`.

## Contract

A summary distinguishes a proven borrow from an unavailable or ambiguous
result. It records an effect only when every normally returning path reaches a
conventional closer call, `database/sql` `Commit` or `Rollback`, cancellation
invocation, or ownership transfer. Conditional effects do not discharge a
caller obligation. Dynamic calls, interface dispatch, unresolved recursion,
unsupported local aliasing, ill-typed packages, and helpers outside selected
modules remain conservative.

Dependency packages are analyzed in deterministic deepest-first layers. Stable
`types.ObjectString` function identities and parameter indexes cross independent
package loads; dependency source remains excluded from diagnostics and fixes.
The canonical `native-effects-v2` digest includes every result-changing summary
field, and the native cache regression changes a helper from borrow to guaranteed
close to prove invalidation and callback re-execution.

## Behavioral Evidence

The integration fixture covers imported helpers that borrow, always complete,
always transfer, conditionally complete, delegate to an unknown external call,
or recurse. Exact diagnostics prove that borrowing and conditional cleanup leave
all four local obligations open, while guaranteed cleanup or transfer discharges
them. Focused analysis tests prove root-package summary exposure and stable
cross-load lookup.

## Performance And Dogfood

The proportional benchmark runs one selected lifecycle rule over 100 root
functions that alternate imported borrow and close helpers and verifies 50
retained obligations. Five one-iteration complete-load samples on Go 1.26.6,
Darwin arm64, and an Apple M4 Max measured an 833.4 ms median, about 5.84 MB,
and 58,789 allocations per run. The range was 699.8 ms to 1.36 s and remains a
proportional cold-cost observation rather than a stable release budget.

A freshly built working-tree binary based on Glippy revision
`554d2681d0aea330dfd52b88b93e31dc7fe697ae` ran the four exact lifecycle rules
over Glippy and `go-libraries/pkg/prompts` at revision
`d7d487f309f3921eff48c5bb277ff11a4fef7077`. Both non-mutating runs completed
without findings or tool failure. The disposable module cache was populated
before the network-disabled package load; dependency source did not become a
lint target.

## Remaining Boundary

Schema version 2 does not model returned nil/error relationships, arbitrary
alias graphs, external-module facts, or multi-module workspace facts. Those
remain separate admission work. Unknown effects keep the prior conservative
ownership-transfer or use behavior; the implementation does not convert
uncertainty into a leak diagnostic.
