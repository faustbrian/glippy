# `finalizer-captures-object` Rule Admission, 2026-08-22

## Decision

Admit `finalizer-captures-object` to the default `correctness` preset at warning
severity. The rule uses the existing SSA tier and reports exact static
`runtime.SetFinalizer` calls whose finalizer closure captures the variable cell
from which the finalized object is loaded. It offers no fix.

## Defect and authority

`runtime.SetFinalizer` schedules a finalizer only after the object becomes
unreachable. A registered closure that retains that same object prevents the
unreachable state, so neither finalization nor collection can occur. The Go
1.27 runtime documentation establishes the reachability prerequisite and now
recommends `runtime.AddCleanup` for new code because finalizers are error-prone.

Staticcheck has shipped the equivalent SA5005 diagnostic since 2017.1. Its
current 2026.2.1 implementation matches the exact standard-library function,
unwraps the two interface arguments, resolves the finalized variable cell, and
compares it with the finalizer closure bindings. Its regression corpus covers
both an assigned closure and an inline function literal, plus finalizers that
correctly use their argument:

- <https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa5005/sa5005.go>
- <https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa5005/testdata/go1.0/CheckCyclicFinalizer/CheckCyclicFinalizer.go>
- <https://pkg.go.dev/runtime#SetFinalizer>

The Go compiler accepts the retaining closure, and the default Go vet catalog
has no equivalent diagnostic. Reusing Go SSA preserves Glippy's shared frontend
and avoids importing Staticcheck's intermediate representation or scheduler.

## Detection contract

The rule reports only when all of these conditions hold:

1. SSA resolves an exact static call to the package-level
   `runtime.SetFinalizer` function;
2. the finalized interface argument unwraps to a load from one exact escaping
   local allocation;
3. the finalizer interface argument unwraps to one exact closure; and
4. that closure binds the same escaping local allocation.

The complete `SetFinalizer` call is the primary range. The closure literal is a
related range. Direct and assigned closures, captured function parameters, and
function-value aliases that SSA resolves exactly to `runtime.SetFinalizer` must
report. A finalizer that uses its parameter, a named finalizer, a closure that
captures only unrelated state, a user-defined `SetFinalizer`, and unresolved or
ambiguous value flow must not report.

## False-positive and fix boundary

Equality of the SSA allocation is proof that the registered closure retains the
same variable cell used to load the finalized object. The rule does not infer
pointer aliases, helper return identities, dynamic calls, or transformed value
flow. Those omissions trade false negatives for a near-zero false-positive
boundary appropriate to `correctness`.

There is no automatic fix. The usual repair is to name and use the finalizer's
parameter, but rewriting the body can require application-specific naming,
method selection, synchronization, or migration to `runtime.AddCleanup`.
Generated files and ill-typed packages remain excluded through shared SSA
policy; suppressions retain the standard exact-rule and reason contract.

## Cost expectation and evidence

The rule performs one bounded pass over each already-built source function's
SSA instructions and compares only the bindings of proven finalizer closures.
It does not request debug mappings, CFG, facts, dependency syntax, or another
package load. The default correctness preset already requires SSA, so admission
does not raise its analysis tier.

Focused behavior covers four positive call and closure shapes, nearby non-diagnostics,
exact primary and related ranges, metadata, source-version gating, generated
and ill-typed exclusion, suppression ownership, and configured severity. Five
one-iteration samples of the retained 100-function in-process package benchmark
measured 72.34-85.81 ms, 6.33-6.91 MB, and 55,289-55,677 allocations. Exact-rule
non-mutating dogfood was clean over Glippy and `go-libraries/pkg/prompts`.
Process-tree, signal, interruption, and RSS probes are explicitly outside the
permitted evidence boundary.
