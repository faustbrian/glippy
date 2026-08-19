# Package No-Return Foundation, 2026-08-15

## Decision

Glippy now computes one demand-driven no-return summary for statically called
named functions and methods whose source is present in the loaded package. The
summary seeds control flow with the predeclared `panic` contract, follows exact
`go/types` callee identity, and shares each named function graph across all
enabled CFG rules. The same summary is installed on the shared SSA program
before it is built.

This is analysis infrastructure rather than a rule. It improves every existing
CFG and SSA consumer without adding a duplicate diagnostic identity, preset, or
configuration switch.

## Observable Contract

A direct or transitive same-package call that cannot return no longer leaves a
spurious normal-return edge. This has two immediate product effects:

- ownership rules such as `context-cancel-leak` do not report an open path that
  terminates through a local panic wrapper; and
- `nilness` can diagnose a condition whose only contrary path terminates
  through a local wrapper, because the shared SSA graph no longer contains the
  impossible continuation.

Dynamic calls, interface dispatch, imported helpers without loaded source or
analyzer facts, and unresolved recursive cycles remain conservatively
returning. The analysis does not inspect function names or comments and does
not treat arbitrary `Fatal`, `Exit`, or `Must` conventions as semantic proof.

## Architecture

The package load already owns one canonical `go/types.Info` per package. The
no-return layer indexes declarations by exact `*types.Func` identity and builds
a graph only when an analyzed CFG function or static callee requires it. A graph
is memoized before reuse by later rules. Re-entrant construction marks an
active definition and treats the cycle as returning, preventing recursion from
inventing a no-return fact. Static-call recursion is capped at 4,096 active
definitions; deeper chains remain conservatively returning.
Construction checks caller cancellation at every named definition and stops
before any rule executes or the SSA program build begins.

CFG rules receive the memoized graph through their existing
`ControlFlowContext`. SSA construction calls `Program.SetNoReturn` before
`Build`, matching the extension point used by x/tools `buildssa` while keeping
Glippy's one shared multi-package SSA program. Its summaries are completed
deterministically before the potentially parallel SSA build, so the installed
predicate is an immutable snapshot.

The v0.5 memory-aware SSA-wave follow-up preserves that immutable snapshot but
installs it into one bounded root-package wave at a time. The unbounded
multi-package lifetime described above is the historical boundary at this
decision's original checkpoint.

## Evidence And Cost

The direct CFG regression initially showed `target` reachable after both a
local panic wrapper and a transitive caller. The context integration initially
reported a cancellation leak on a branch that terminates through that wrapper,
and the nilness regression initially missed the now-provable second condition.
All three failed before the shared summary and pass after it. Nearby tests keep
dynamic calls and recursive cycles conservative and cover methods,
parenthesized calls, shadowed `panic`, exact nilness range mapping, and the
existing direct-panic behavior.

Five 100-iteration probes over a 100-function no-return chain measured a 38.3
microsecond median per CFG run, about 52.6 KiB, and 953 allocations on Darwin
arm64 with an Apple M4 Max. The probe excludes package loading and is
proportional evidence for the analysis layer, not a stable release budget.

## Revisit Triggers

Add cross-package no-return facts only when they can reuse the existing
dependency-first fact scheduler and cache identity without forcing dependency
syntax into unrelated rules. Revisit recursion only if real defects require a
fixed-point strongly connected component analysis. Revisit standard-library
terminal intrinsics only from authoritative API contracts and focused
cross-version evidence.
