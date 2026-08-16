# Lock State-Transition Rule Admission, 2026-08-16

## Decision

Admit `unlock-without-lock` as a warning-level default `correctness` rule and
`lock-not-released` as a warning-level opt-in `suspicious` rule. Upgrade
`lock-held-across-blocking-call` from lexical types-tier tracking to the same
shared CFG state result and allow exact configured blocking contracts.

None of the three rules offers a fix. A valid repair may add an acquisition,
move or remove a release, change read versus write mode, narrow a critical
section, or preserve an intentional handoff. Selecting among those outcomes is
not mechanically semantics-preserving.

## Observable Defects

Go 1.26.6 documents that `sync.Mutex.Unlock` is a run-time error when the mutex
is not locked, `sync.RWMutex.Unlock` is a run-time error without a write lock,
and `sync.RWMutex.RUnlock` is a run-time error without a read lock. These errors
terminate the process rather than returning an ordinary error.

Two reviewed public defects establish the rule value:

- [crossplane-contrib/crossview#172](https://github.com/crossplane-contrib/crossview/issues/172)
  records a deterministic process crash from `RLock`, immediate deferred
  `RUnlock`, a conditional manual `RUnlock`, and an early return. The deferred
  call then executes on the already unlocked mutex.
- [CaliLuke/loom#44](https://github.com/CaliLuke/loom/issues/44) records a
  returning branch that leaves a mutex held and permanently blocks subsequent
  reader and close operations.

The compiler accepts both control-flow shapes. The Go 1.26.6 vet catalog owns
lock copying and WaitGroup ordering but does not provide these path-sensitive
release checks. Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` contains SA2001 empty-critical-
section and SA2003 deferred-lock checks, but no matching missing-release or
unmatched-unlock analyzer.

## Shared Analysis Contract

The implementation recognizes exact `sync.Mutex` and `sync.RWMutex` method
identities on direct identifiers and one-field selector paths. It propagates a
finite state per storage identity through the shared no-return-aware CFG:

- unknown or not-yet-initialized;
- unlocked;
- write-locked;
- exact local read-lock depth from one through eight; and
- an unknown state after deeper read nesting.

The worklist joins block-entry states monotonically and is bounded by graph
size, tracked lock count, lattice height, a 4,096-lock per-function limit, and
a one-million-change hard ceiling. Diagnostics are collected only in a second
pass over stable block entries. This prevents traversal or branch order from
leaking transient states into output.

An ordinary direct deferred `Unlock` or `RUnlock` is applied at every normal
return. Multiple or conditionally different deferred operations become
unknown. Passing or returning the receiver, capturing it in a function,
assigning an alias, starting an asynchronous use, or calling an unknown method
on its owning value also makes state unknown. Invalid-release paths that would
terminate do not continue into later diagnostics.

`lock-held-across-blocking-call` recognizes exact `time.Sleep`,
`sync.WaitGroup.Wait`, and `os/exec.Cmd.Wait` calls plus functions declared
blocking by the project semantic-contract layer. `sync.Cond.Wait` does not
report: its authoritative contract requires the associated Locker to be held,
atomically unlocks it while waiting, and relocks it before returning.

## Severity And False-Positive Boundary

`unlock-without-lock` reports only when an observed path establishes an
incompatible state. Parameters, package variables, and fields start unknown,
so an initial release does not report merely because the acquisition may be in
another function. A local zero-value declaration, observed acquisition and
release, or observed read/write mode can establish the state required for a
finding.

`lock-not-released` remains suspicious because Go locks are not associated
with one goroutine and code can deliberately return while another function or
goroutine owns the eventual release. Any direct escape suppresses the finding.
The rule therefore identifies review-worthy locally visible paths without
claiming every handoff is incorrect.

Generated files and ill-typed packages are excluded. Ordinary exact-rule
suppressions, baselines, severity policy, source identity, target attribution,
and deterministic reporting apply through the shared engine. No rule runs on
syntax-only invocations.

## Behavioral Evidence

Focused package tests cover:

- a missing release on one return branch and complete release on every branch;
- ordinary deferred release and a deferred double `RUnlock` crash path;
- local unmatched, direct double, and read/write-mode-mismatched releases;
- nested branches, bounded loops, balanced nested read locks, and the deep-read
  saturation boundary;
- unknown parameter and field state, helper escape, nested function capture,
  no-return paths, exact sync identity, and lookalikes;
- blocking calls across CFG branches, `sync.Cond.Wait` exclusion, and a project
  blocking contract; and
- generated, type-error, and suppression behavior plus exact diagnostic
  ranges and metadata.

Three isolated ten-iteration samples of the 100-function, 300-finding package
benchmark on an Apple M4 Max with Go 1.26.6 measured a 63.42 ms/op median,
7.71 MB/op, and 84,642 allocs/op. The observed latency range was
62.92-64.02 ms/op. This includes fresh package loading for every iteration and
is proportional
admission evidence rather than a portable latency promise.

Non-mutating exact-rule dogfood completed without findings on Glippy and on
`go-libraries/pkg/prompts`. The prompts run used a task-owned prepopulated
module cache because ordinary Glippy analysis correctly keeps module lookup
offline.

## Deliberate Limits And Revisit Triggers

Indexed receivers, multi-level selector storage identities, `TryLock`, exact
helper lock effects, arbitrary alias propagation, more than one deferred lock
operation per storage identity, and read-lock nesting deeper than eight remain
conservative false-negative boundaries.

Revisit the representation when reviewed defects require one of those shapes,
when dogfood identifies a false positive, when project contracts gain receiver
state effects, or when the first resource/channel state rules prove that the
generic worklist needs another bounded lattice operation.
