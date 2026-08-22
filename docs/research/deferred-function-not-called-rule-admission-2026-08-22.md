# `deferred-function-not-called` Rule Admission, 2026-08-22

## Decision

Admit `deferred-function-not-called` to the opt-in `suspicious` preset at
warning severity. A setup, tracing, or test helper can perform work immediately
and return a function that restores state or records completion. This form
defers the outer call and discards its returned function:

```go
defer beginTrace()
```

The intended form is usually:

```go
defer beginTrace()()
```

The rule is not in `correctness`. Go permits callers to defer a function for
its own side effects and intentionally discard a function-valued result. That
case is unusual but valid and requires limited intent judgment.

No fix is offered. A returned function can require arguments, and even adding
an empty invocation changes behavior. Glippy does not guess the arguments or
whether the result should be called.

## Defect And Existing Tools

Staticcheck SA9010 is the external authority. The reviewed implementation at
[`31e1ee5e554a3ea6217dbbcfb21c2435ace7e579`](https://github.com/dominikh/go-tools/blob/31e1ee5e554a3ea6217dbbcfb21c2435ace7e579/staticcheck/sa9010/sa9010.go)
uses the standard Go type information for the deferred call and reports when
its result has a function signature. Its originating commit
[`eaff1c59baa4289f1f43ea347fae00fba5d6f375`](https://github.com/dominikh/go-tools/commit/eaff1c59baa4289f1f43ea347fae00fba5d6f375)
records a scan of the Go standard library, the author's code, and about 200
popular Go packages with no observed false positives.

That scan found a real defect in gRPC Go. Merged
[`grpc/grpc-go#7270`](https://github.com/grpc/grpc-go/pull/7270), commit
[`639ada214071ed20b1d7ca02105fb7c705fcd5d6`](https://github.com/grpc/grpc-go/commit/639ada214071ed20b1d7ca02105fb7c705fcd5d6),
added the missing second invocation to a test helper that saved and returned a
closure restoring the global ALTS dialer. Without that invocation, the old
dialer was never restored at test exit.

The Go compiler accepts the discarded result, and the Go 1.27 default vet
catalog has no equivalent check.

## Precision Contract

The rule inspects only `defer` statements in well-typed, non-generated files.
It asks `go/types` for the type of the deferred call expression and reports only
when the result's underlying type is a function signature. Direct functions,
methods, function variables, generic calls, named function results, and
returned functions requiring arguments therefore share one type-proven
boundary. Calls returning no value, a non-function value, or multiple values do
not report. A returned function that is actually invoked in the defer statement
does not report unless that invocation itself returns another uninvoked
function.

The primary range covers the deferred call. Exact suppressions, configured
severity, generated-file exclusion, type-error exclusion, and source-version
selection use the shared policy engine. When a project contract also marks the
same result as must-use, the more specific `must-use-result` diagnostic owns the
location and the suspicious diagnostic is removed before suppression or
baselining.

## Evidence And Cost Boundary

The focused product test first failed because the rule ID was absent. Current
fixtures cover direct and named function results, methods, returned functions
requiring arguments, executed returned functions, ordinary deferred results,
exact ranges, metadata, minimum Go-version selection, suppressions, configured
severity, generated files, ill-typed packages, no fixes, and diagnostic
ownership with `must-use-result`.

The implementation is a filtered types-tier traversal with constant work per
defer statement. It adds no independent package load, CFG, SSA, fact, cache,
subprocess, or network requirement beyond the shared types-tier load. Exact-rule
non-mutating dogfood over Glippy and `go-libraries/pkg/prompts` produced no
diagnostics or tool failures. No benchmark, RSS probe, signal test,
interruption test, process-tree test, descendant-cleanup test, or Docker test
was executed for this admission. It therefore makes no fresh portable latency
or memory claim.

## Revisit Triggers

Revisit preset placement only after broad external dogfood shows near-zero
intentional discards. Add a suggestion only if invocation arguments and the
resulting edit can be represented without guessing. Preserve project-contract
diagnostic ownership if either rule's range changes.
