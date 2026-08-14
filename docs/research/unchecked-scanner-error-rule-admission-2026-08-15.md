# `unchecked-scanner-error` Rule Admission, 2026-08-15

## Decision

Admit `unchecked-scanner-error` to the opt-in `suspicious` preset at warning
severity. The native control-flow rule recognizes direct identifier-backed
`bufio.Scanner.Scan` loops and reports when a normally returning path can
bypass observation of the matching `Scanner.Err` result. Generated files and
ill-typed packages are excluded, and no fix is offered.

## Defect And Existing Tools

Go 1.26.6 documents that `Scanner.Scan` returns false both at end of input and
when scanning stops because of an error. `Scanner.Err` distinguishes a
non-EOF scanning error from successful completion. Returning consumed data
without that check can therefore silently accept truncated input.

The compiler accepts the omission. The Go 1.26.6 default vet catalog has no
scanner terminal-error analyzer. Staticcheck 2026.1 has no equivalent check,
and the GolangCI-Lint v2.12.2 catalog has no dedicated scanner-error linter.

A reviewed public occurrence exists in
[`alessio/shellescape` v1.5.1](https://github.com/alessio/shellescape/blob/v1.5.1/cmd/escargs/escargs.go#L69):
the command consumes `scanner.Scan` in a loop and returns without consulting
`scanner.Err`. A read or tokenization failure can therefore yield incomplete
successful output.

## Precision And Source Behavior

The rule requires exact `bufio.Scanner.Scan` and `Scanner.Err` method identity
on the same identifier-backed typed object. Unrelated `Scan` and `Err` methods,
fields, containers, and promoted wrapper methods do not report. Scanners
returned by helper packages are recognized without requiring a direct `bufio`
import. A direct assignment to the scanner variable invalidates a later check
against the replacement value.

The post-loop CFG search stops a path when the original `Scanner.Err` result is
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
`bufio.Scanner`. Those gaps prefer false negatives over speculative ownership.
No fix is registered because propagation, logging, partial-input policy, and
recovery are caller decisions.

## Behavioral And Cost Evidence

The focused suite began red with `unknown rule "unchecked-scanner-error"` and
now covers completely unchecked loops, partial-branch checks, blank and
expression discards, reassignment, checks after enclosing branches, no-return
paths, exact ranges, lookalike types, scanners returned by a helper package,
suppressions, generated and ill-typed packages, source versions, severity, and
absence of fixes. The shared rows-error regression suite covers the same CFG
engine's nested-function boundary.

Five one-iteration complete package-analysis samples over 100 checked loops on
Go 1.26.6, Darwin arm64, Apple M4 Max produced a median of
`231,631,291 ns/op`, `2,966,728 B/op`, and `31,120 allocs/op`. Package loading
dominates this measurement; it is proportional admission evidence, not a
release latency budget.

Non-mutating dogfood enabled only this rule over Glippy and
`go-libraries/pkg/prompts`; both produced zero findings. Broader probes found
two real unchecked scanner loops in `go-libraries/cmd/golib/main.go` and
`go-libraries/pkg/openrpc/internal/specification/matrix.go`. The external
repository's pre-existing dirty state was left unchanged. This establishes an
initial noise sample and positive signal without treating dogfood as a release
budget.

## Revisit Trigger

Revisit alias and interface tracking when dogfood produces missed defects that
justify SSA or a bounded ownership model. Revisit preset membership only after
broader repositories establish signal and CFG latency remains within the
published typed-lint budget.
