# `ineffective-assignment` Rule Admission, 2026-08-25

## Decision

Admit `ineffective-assignment` as an opt-in `nursery` SSA rule at warning
severity. It reports a direct identifier assignment whose resulting value has
no observable use. It covers ordinary and compound assignments, tuple
components, receives, and increments without deleting or suppressing the
right-hand-side operation.

The rule remains outside the curated `default`, `recommended`, `strict`, and
`pedantic` profiles until the complete v0.8 corpus rerun establishes its signal.
It has no fix.

## Defect And Existing Tools

The v0.6 pinned corpus established five missed-defect gap records covering six
comparator findings. Four independent Staticcheck `SA4006` findings show values
computed or received and then never consumed:

- NATS receives an error from `errCh` but never reads the received value;
- sqlc computes a ClickHouse parameter name and fallback but never places them
  in the returned parameter node;
- Hugo increments a missing-file debug-context end offset that is never used;
  and
- Grafana computes a quoted deleted-column name that its condition never uses.

The Go compiler accepts these programs because the variables are otherwise
used, and default `go vet` does not report them. Staticcheck 2026.2.1 `SA4006`
was inspected from the authoritative tagged source at
[`staticcheck/sa4006/sa4006.go`](https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa4006/sa4006.go).
Its current implementation checks assignment and increment results through its
IR, follows phi values, treats switch tags as uses, and excludes generated
goyacc functions.

## Precision Boundary

Glippy uses the shared x/tools SSA program and the same demand-driven debug
mapping already required by `overwritten-error`. It indexes all debug values
for an expression once per function so compound assignments and increments can
select the produced value without repeated whole-function searches. It follows
phi and interface forwarding, handles tuple extracts, preserves switch-tag
uses, and treats an explicit blank assignment as intentional observation.

Only direct identifier destinations are eligible. Fields, indexes,
dereferences, range variables, standalone `var` declarations, address-taken
values that SSA cannot identify, generated files, and ill-typed packages are
excluded. SSA constants remain excluded because the compiler and SSA builder
can merge constant values independently of the source assignment. Nested
function literals are analyzed through their own SSA function and are not
walked as part of the parent.

`overwritten-error` retains ownership when both rules report the same exact
source range. This prevents a general nursery diagnostic from duplicating the
more actionable error-flow message. Suppressions, severities, source-version
selection, baseline handling, and deterministic ordering use the shared driver
contracts.

## Fix Safety

No safe, suggestion, or unsafe fix is registered. Replacing a destination with
`_`, deleting the assignment, restoring a missing consumer, or returning a
different value are observably different repairs. RHS calls and receives may
have required effects even when their result is ineffective.

## Behavioral Evidence

The first focused test failed because the product registry did not know
`ineffective-assignment`. A separate compound-assignment test then failed
because the first implementation mapped the RHS operand rather than the value
produced by `+=`. The final fixtures cover ordinary assignments, channel
receives, increments, compound assignments, tuple components, branch joins,
switch uses, explicit blank observations, used updates, constant exclusions,
exact ranges, metadata, generated and type-error exclusions, suppressions,
configured severity, minimum Go version, and interaction with
`overwritten-error`.

## Revisit Triggers

Promote the rule from nursery only after exact-revision corpus evidence records
its false-positive families and proves acceptable cost. Revisit constant and
address-taken values only with a source-to-SSA identity contract that does not
widen noise. Do not add a fix without one semantics-preserving repair that
retains required RHS effects.
