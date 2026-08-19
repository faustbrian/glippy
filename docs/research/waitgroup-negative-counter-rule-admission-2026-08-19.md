# `waitgroup-negative-counter` Rule Admission, 2026-08-19

## Decision

Admit `waitgroup-negative-counter` to the default `correctness` preset at
warning severity. The native CFG rule reports an exact `sync.WaitGroup.Add` or
`Done` only when every reaching tracked counter state would become negative and
panic. Generated files and ill-typed packages are excluded, and no fix is
offered.

## Defect And Existing Tools

The Go 1.26.6 `sync.WaitGroup` contract states that `Add` panics when the task
counter becomes negative. `Done` is exactly `Add(-1)`. The runtime's own
`TestWaitGroupMisuse` fixture proves the direct zero, `Add(1)`, `Done`, `Done`
panic sequence. The compiler accepts these operations because counter state is
not part of the type system.

The Go 1.26.6 `waitgroup` analyzer and Staticcheck SA2000 report the separate
ordering defect where `Add` runs inside the goroutine being counted. Neither
tracks counter values or reports a direct path-proven underflow, so the native
rule complements the existing Glippy `waitgroup-misuse` adapter instead of
replacing or duplicating it.

The defect family is represented by
[`dnstapir/pop#159`](https://github.com/dnstapir/pop/issues/159). At revision
`9f48d2736c8b4c47a52228854e506d45da4cd45d`, `main.go` initializes a local
counter to one and calls `Done` from two repeatable shutdown branches without
leaving the surrounding loop. A second shutdown event can therefore drive the
counter negative. That occurrence requires goroutine-capture and repeated-loop
ownership beyond this first direct-local contract, so it motivates the family
without being presented as a detected positive.

## Precision And State Contract

The rule tracks only objects declared inside the analyzed function and
initialized as an exact zero `sync.WaitGroup`. Supported initializers are a
zero value declaration, an empty composite value or pointer, and exact built-in
`new(sync.WaitGroup)`. Parameters, package variables, fields, indirect
constructors, and lookalike types do not establish state.

A bounded monotone CFG worklist represents exact counters from zero through 60
plus conservative unknown state. Constant `Add` arguments and direct `Done`
calls update every reaching exact state. Paths that would panic are removed
from normal continuation. A diagnostic is emitted only when no represented
state can continue without underflow. Branches with both safe and underflowing
states therefore do not report.

A direct `Wait` returns only when the counter is zero. If an exact state set
contains zero, the normal continuation is narrowed to zero. If the only exact
states are positive and the local value has not escaped, propagation stops
because the call cannot return. Unknown state may return and is narrowed to
zero on that continuation. This prevents an unreachable operation after a
locally unfulfillable wait from becoming a false underflow diagnostic.

Aliases, storage, helper calls, closure capture, and asynchronous operations
establish persistent escape state. Dynamic deltas and exact counts above 60
become unknown. Deferred operations are not applied at registration and are not
modeled at function exit. Alias and escape state remains persistent across
`Wait` and reassignment because another reference may mutate the same storage,
including when a reassignment publishes a pointer to that storage in the same
statement. A CFG node with multiple relevant operations becomes unknown instead
of treating AST traversal order as evaluation order. More than 4,096 candidates
or more than the derived shared transition bound fails closed. An exact
reassignment establishes zero only while no persistent escape exists.

No fix is registered because the intended repair may add or remove work,
return from a repeated path, move ownership between goroutines, or replace the
synchronization design.

## Behavioral And Cost Evidence

The first focused test failed because the registry did not recognize the new
rule. A second red test exposed an invalid diagnostic after `Add(1); Wait()`:
the local wait cannot return, so a later `Done` is unreachable. The corrected
transfer stops that path instead of assuming every `Wait` returns. Two further
red cases prevented precision from being reestablished after a pointer alias
survived zero-value reassignment and after an extracted `Add` method value hid
a counter increment. A final red case made channel transfer retain persistent
escape state instead of reporting against counter changes through the sent
pointer.

The green suite covers direct `Done`, repeated `Done`, negative `Add`, all-path
and partial-path joins, returning and unfulfillable waits, exact
reinitialization, pointer and value initialization, aliases, helpers, channel
transfer, closure capture, method values, goroutines, defers, dynamic and
bounded-large counts, parameters,
globals, suppressions, generated files, type errors, source versions, exact
ranges, deterministic ordering, metadata, and absence of fixes.

Five complete 100-function, 100-finding package-analysis samples ran on Go
1.26.6, Darwin arm64, and an Apple M4 Max:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 70.42 ms | 3,828,768 | 31,240 |
| 2 | 67.43 ms | 3,272,408 | 30,840 |
| 3 | 67.49 ms | 3,268,856 | 30,824 |
| 4 | 70.20 ms | 3,257,680 | 30,804 |
| 5 | 69.20 ms | 3,262,280 | 30,823 |

The median was 69.20 ms, 3,268,856 bytes, and 30,824 allocations per
operation. Each operation includes fresh package loading, so this is
proportional admission evidence rather than a portable latency budget.

Non-mutating exact-rule dogfood completed without findings or prerequisite
problems on Glippy and `go-libraries/pkg/prompts` at
`4fd1d2dc22f8096d099363382e031c956dc4edff`. The prompts run used task-owned
build, module, and Glippy caches with network lookup disabled during analysis.
Its pre-existing `go.sum` diff digest remained
`f1fe8ad4638565a0d3cf6e15fc440b6a6d133f8871ab5822040a749a307809f7`, and
its empty untracked-file digest remained
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

## Revisit Trigger

Revisit the representation when reviewed defects justify larger exact counts,
deferred-exit evaluation, locally owned goroutine summaries, counter ownership
transfer, or concurrency-aware Wait and reuse state. Do not add a fix without
one canonical semantics-preserving repair.
