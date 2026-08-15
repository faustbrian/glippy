# `sql-transaction-not-completed` Rule Admission, 2026-08-15

## Decision

Admit `sql-transaction-not-completed` to the default `correctness` preset at
warning severity. The native control-flow rule recognizes direct
`database/sql` transaction acquisitions through `DB.Begin`, `DB.BeginTx`, and
`Conn.BeginTx` followed by a conventional returning error guard. It reports
when a normally returning path can retain the transaction without an exact
`Tx.Commit`, exact `Tx.Rollback`, or conservative ownership transfer.
Generated files and ill-typed packages are excluded, and no fix is offered.

## Defect And Existing Tools

Go 1.26.6 documents the transaction contract directly in `database/sql`:
"A transaction must end with a call to Tx.Commit or Tx.Rollback." Returning
from a successful acquisition without either call can retain a connection,
locks, and transaction state. The compiler accepts the omission. The Go 1.26.6
default vet catalog has no transaction-lifecycle analyzer.

Current Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` has no corresponding transaction
completion rule. Its nearby SQL checks cover query and row behavior rather
than proving transaction completion on every returning path.

A reviewed public occurrence exists in
[`haxpax/gosms` at `1cb89c4`](https://github.com/haxpax/gosms/blob/1cb89c41d4578c217782511180a2145d54bc6649/db.go#L48-L89).
Both transaction helpers return after `Prepare` or `Exec` errors without
rolling back the acquired transaction. A final `Commit` call does not complete
those earlier error paths.

Glippy also has independent organizational demand in
`go-libraries/pkg/analysis/analyzers/transactionrollback`. That analyzer
enforces immediate rollback ownership for configurable transaction APIs. The
Glippy rule is deliberately standard-library-specific and instead proves the
weaker but semantic requirement that every normal path completes or transfers
the transaction.

## Precision And Source Behavior

The rule requires typed identity for the standard `database/sql` acquisition
and completion methods. Same-named user methods and transaction wrappers do
not report. The transaction and error must be direct assignment identifiers,
and the immediately following guard must compare that exact error object with
nil and return from its body. Noncanonical acquisition and guard shapes remain
false negatives.

The shared CFG begins after the successful acquisition guard. Exact Commit or
Rollback calls discharge that path. Returning, passing, sending, storing,
capturing, or extracting a method value from the transaction counts as a
conservative ownership transfer. Composite-literal wrapping is detected
recursively so returning a transaction-owning wrapper does not report. The
callee or new owner is not inspected. Reassigning the original transaction
object before completion reports the lost obligation.

Cleanup must cover every normally returning path. Conditional defers,
conditional commits, and earlier error returns therefore report when another
path remains open. Built-in panic is non-returning through the shared CFG;
imported and project-local no-return helpers remain conservatively returning.

No fix is registered. Inserting a rollback, changing an ownership boundary,
or selecting commit behavior requires caller intent, and successful parsing
would not prove the transformation semantics-preserving.

## Behavioral And Cost Evidence

The focused suite began red with an unknown rule and missing metadata. It now
covers open transactions, partial commit and rollback branches, conditional
defer, reassignment, `sql.Conn.BeginTx`, deferred method values, asynchronous
transfer, direct return, argument transfer, composite-literal wrapping,
lookalike methods, noncanonical guards, exact ranges, suppressions, generated
and ill-typed exclusions, Go-version selection, and absence of fixes.

One five-iteration complete package-analysis benchmark over 100 transaction
functions on Go 1.26.6, Darwin arm64, and an Apple M4 Max averaged
`75,995,058 ns/op`, `5,651,937 B/op`, and `56,214 allocs/op`. Package loading
dominates the measurement; it is proportional admission evidence rather than
a stable latency budget.

Non-mutating exact-rule dogfood produced no findings over Glippy at
`762761f9e175b228e0edae01529a349e6a4ec458`,
`go-libraries/pkg/prompts`, `go-libraries/pkg/migrations`, and
`go-libraries/pkg/capability` at
`5ead3d540eb6109a6bc8cfc2a2449640cb847108`. Additional focused runs were
clean over `vuja/internal/scoring` at
`870bc063adfe94fd23abbc6061c73f9c737935fe` and Tarvero at
`730045493a324211843e603d9edc37a492c0fb1c`. A whole-repository Vuja run was
not evidence because an unrelated pre-existing integration-package type error
prevented complete loading.

## Revisit Trigger

Revisit wrapper acquisitions, aliases, and interprocedural ownership when
dogfood produces missed defects that justify shared obligation facts. Revisit
the immediate-guard restriction only when alternate acquisition shapes can be
modeled without treating a possibly nil transaction as owned. Do not add an
automatic fix without one canonical semantics-preserving completion strategy.
