# `unchecked-rows-error` Rule Admission, 2026-08-14

## Decision

Admit `unchecked-rows-error` to the opt-in `suspicious` preset at warning
severity. The native control-flow rule recognizes direct identifier-backed
`database/sql.Rows.Next` loops and reports when a normally returning path can
bypass observation of the matching `Rows.Err` result. Generated files and
ill-typed packages are excluded, and no fix is offered.

## Defect And Existing Tools

Go 1.26.6 documents that `Rows.Next` returns false both at the end of a result
set and when preparing the next row fails. The same API contract says
`Rows.Err` should be consulted to distinguish those cases. Returning collected
results without that check can silently turn an interrupted query into a
successful partial result.

The compiler accepts the omission. The Go 1.26.6 default vet catalog has no
database-row iteration analyzer. Staticcheck 2026.1 has no equivalent check;
its nearby SA5001 rule covers deferring `Close` before checking an acquisition
error.

The specialized `rowserrcheck` analyzer does cover this defect class. Source at
pseudo-version `v0.0.0-20260419091836-c5f79b8a11ba` builds SSA and traces rows
values from query calls, but any reachable `Err` call counts. In particular,
its fixtures accept `_ = rows.Err()`. Glippy's initial boundary is narrower in
alias coverage but stricter about actual result observation and proves the
check on every normally returning post-loop path. It reuses the already shared
CFG rather than requesting SSA.

A reviewed public occurrence exists in
[`facebookincubator/nvdtools` v0.1.5](https://github.com/facebookincubator/nvdtools/blob/v0.1.5/vulndb/summary.go#L51-L74):
`SummaryRecords` iterates a `*sql.Rows`, then returns accumulated records and a
nil error without consulting `rows.Err`. A driver-side iteration failure can
therefore yield an incomplete successful result.

## Precision And Source Behavior

The rule requires exact `database/sql.Rows.Next` and `Rows.Err` method identity
on the same identifier-backed typed object. Unrelated `Next` and `Err` methods,
fields, containers, and promoted wrapper methods do not report. A direct
assignment to the rows variable invalidates a later check against the
replacement value.

The post-loop CFG search stops a path when the original `Rows.Err` result is
returned, used in a condition, assigned to a nonblank variable, or passed to
another call. A standalone expression statement and assignment to the blank
identifier do not count. Normally returning paths are required to observe the
result; paths ending in the predeclared `panic` function are not. Imported and
project-local termination helpers remain conservatively returning because the
shared CFG does not load transitive no-return facts. Passing the result to
another call is considered observation without interprocedural proof of the
callee.

The initial contract does not track aliases, fields, container elements,
range-target reassignment, deferred closure checks, or implementations outside
`database/sql.Rows`. Those gaps prefer false negatives over speculative
ownership. No fix is registered because error propagation, aggregation,
logging, and partial-result policy are caller decisions.

## Behavioral And Cost Evidence

The focused suite began red with `unknown rule "unchecked-rows-error"` and now
covers completely unchecked loops, partial-branch checks, blank and expression
discards, reassignment, checks after enclosing branches, nested function
literals, no-return paths, exact ranges, lookalike types, rows returned by a
helper package without a direct `database/sql` import, suppressions, generated
and ill-typed packages, source versions, severity, and absence of fixes.

Five one-iteration complete package-analysis samples over 100 checked loops on
Go 1.26.6, Darwin arm64, Apple M4 Max produced a median of
`100,975,458 ns/op`, `4,063,360 B/op`, and `40,941 allocs/op`. Package loading
dominates this measurement; it is proportional admission evidence, not a
release latency budget.

Non-mutating dogfood enabled only this rule over Glippy at
`a718478d1e772e56c6c2ead8bf2abd63517e4a3d` and
`go-libraries/pkg/prompts` at
`0b9bb08727cc1cabdc674bbfe7082fc5642c3f2a`. Both produced zero findings, and
the external repository's pre-existing dirty state and revision were
unchanged. This establishes an initial noise sample; the focused fixture and
reviewed public occurrence provide positive signal.

## Revisit Trigger

Revisit alias and interface tracking when dogfood produces missed defects that
justify SSA or a bounded ownership model. Revisit preset membership only after
broader repositories establish signal and CFG latency remains within the
published typed-lint budget.
