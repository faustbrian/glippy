# `deprecated-ioutil` Rule Admission, 2026-08-21

## Decision

Admit `deprecated-ioutil` as an explicit migration rule at warning severity.
The Go standard library deprecated `io/ioutil` in Go 1.16 after moving its API
to `io` and `os`, but compilation and default `go vet` do not require callers
to migrate. The rule reports exact typed references to the eight deprecated
exports without treating an import spelling or local lookalike as evidence.

## Boundary

The types-tier rule covers `Discard`, `NopCloser`, `ReadAll`, `ReadDir`,
`ReadFile`, `TempDir`, `TempFile`, and `WriteFile` only when the selected object
belongs to package `io/ioutil`. Named and dot imports are supported. Current
`io` and `os` APIs, local names, generated files, ill-typed packages, and source
versions before Go 1.25 do not report. Exact suppressions retain ordinary
ownership and reason policy.

No fix is offered. Replacing a selector also changes import ownership and can
conflict with existing `io` or `os` aliases, so an isolated text edit would not
satisfy Glippy's safe-fix contract. The migration group remains opt-in and the
rule can be selected by exact ID.

## Evidence

Focused package fixtures cover all eight exports, named and dot imports,
current replacement APIs, generated-file exclusion, exact suppression,
source-version selection, metadata, and the no-fix boundary. Five one-iteration
package-load probes measured 55.92-67.69 ms on Darwin arm64, with a 58.36 ms median,
1.89 MB median allocation, and approximately 13,509 allocations. Package
loading dominates this proportional admission probe; it is not a release
latency budget.

Non-mutating exact-rule dogfood on the current Glippy candidate and
`go-libraries/pkg/prompts` produced no diagnostics or tool failures.

## Revisit Trigger

Add a fix only after the import coordinator can prove an exact non-conflicting
replacement alias and validate the complete formatted file transactionally.
Reaudit the export list if the supported Go toolchain changes the deprecation
surface.
