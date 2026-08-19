# `sql-transaction-used-after-completion` Rule Admission, 2026-08-19

## Decision

Admit `sql-transaction-used-after-completion` to the default `correctness`
preset at warning severity. The native CFG rule reports an exact
`database/sql.Tx` operation or repeated completion only when every reaching
path has already committed or rolled back the directly acquired transaction.
Generated files and ill-typed packages are excluded, and no fix is offered.

## Defect And Existing Tools

Go 1.26.6 documents the state contract on `database/sql.Tx`: after Commit or
Rollback, every operation fails with `ErrTxDone`. The standard-library tests
exercise both Rollback followed by Commit and Commit followed by Rollback and
require `ErrTxDone`. Continuing to query, execute, prepare, or finalize the
same transaction therefore performs an operation whose useful result is
impossible. The compiler accepts it.

The Go 1.26.6 default vet catalog has no database transaction analyzer. The
published Staticcheck catalog reviewed on 2026-08-19 likewise has no
`database/sql` transaction-completion state rule. This rule complements
Glippy's existing `sql-transaction-not-completed`: that rule proves missing
finalization on returns, while this rule proves invalid work after
finalization.

## Precision And State Contract

The rule reuses the exact acquisition boundary already admitted for
`sql-transaction-not-completed`: direct `DB.Begin`, `DB.BeginTx`, or
`Conn.BeginTx` assignment followed immediately by a returning `err != nil`
guard. Same-named methods, wrappers, noncanonical guards, generated files, and
ill-typed packages do not report.

A bounded monotone CFG worklist begins at the successful side of that guard.
Each candidate is open, completed, a join of reaching states, or conservatively
unknown. Exact Commit and Rollback calls establish completed state. An exact
same-module effect fact or configured project contract that guarantees
transaction completion does the same. A proven borrowing helper preserves the
current state.

A diagnostic requires the exact completed state. A conditional completion
joined with an open path does not report. Dynamic calls, aliases, return or
storage transfer, reassignment, asynchronous use, and unknown helpers become
unknown. A later exact Commit, Rollback, or guaranteed completion helper
reestablishes completed state because `database/sql` guarantees the
transaction is done afterward regardless of its prior state. Deferred calls
are not evaluated at registration time, and multiple transaction calls in one
CFG node become unknown instead of assuming nested evaluation order.

The primary range covers the invalid call. One related completion range is
included only when a single preceding completion call is unambiguous. No fix is
registered because the intended repair may move the operation, remove it,
begin another transaction, or change which transaction owns the work.

## Behavioral And Cost Evidence

The first focused test failed because the product registry did not recognize
the rule. A later focused regression failed when exact Commit after an unknown
helper did not reestablish completed state. The green suite covers Commit,
Rollback, all-path branch completion, repeated finalization, conditional
completion, deferred completion, asynchronous, transfer, and alias fail-closed
behavior, dynamic helper use, multiple calls in one node, proven helper
completion and borrowing, exact completion after escape, exact ranges, related
locations, suppressions, generated and ill-typed exclusions, source-version
selection, metadata, and absence of fixes.

One complete 100-function, 100-finding package-analysis probe on Go 1.26.6,
Darwin arm64, and an Apple M4 Max measured `87,117,208 ns/op`, `7,647,376 B/op`,
and `76,756 allocs/op`. The single iteration is proportional regression
evidence, not a stable latency budget.

Non-mutating exact-rule dogfood completed without findings on Glippy and
`go-libraries/pkg/prompts` on 2026-08-19. The prompts run preserved its exact
pre-existing byte status. Both runs used task-owned prepopulated module and
build caches because ordinary Glippy analysis disables network module lookup.

## Revisit Trigger

Revisit aliases, deferred execution, ordered multi-call nodes, and asynchronous
operations only when reviewed defects justify the added representation. Reuse
one transaction-state result with missing-completion analysis when that avoids
measurable duplicate work without weakening either rule's contract. Do not add
an automatic fix without one canonical semantics-preserving repair.
