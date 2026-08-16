# `resource-not-closed` Rule Admission, 2026-08-13

## Decision

Admit `resource-not-closed` to the opt-in `suspicious` preset at warning
severity. As expanded on 2026-08-15, the native control-flow rule tracks
locally owned call results with a static `Close() error` method and reports
when a normally returning path neither closes nor conservatively transfers the
value.

## Defect And Existing Tools

Leaking files, connections, compressors, and similar resources can exhaust
descriptors, retain network capacity, or defer important flush and shutdown
work. Go permits these local results to leave scope without a close. Default Go
1.26.5 vet has no general local closer ownership analyzer; Staticcheck has
specific resource rules but no equivalent general transfer contract.

Sources inspected on 2026-08-13 were Go 1.26.5 type and method-set semantics,
the default vet catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de` for must-use and ownership-inspired
lint boundaries.

## Precision, Policy, And Fixes

The rule recognizes direct call assignments whose corresponding result has
`Close() error`. A conventional immediately following acquisition-error guard
starts ownership on its successful continuation; other shapes start after the
assignment. Exact `Close` calls complete the obligation. A direct argument,
return, send, assignment, composite insertion, method value, or closure capture
transfers it conservatively. Reassignment before either effect loses the
obligation and reports. Zero-result `Close` methods remain excluded because a
method name alone includes non-resource reflection values.

When an exact same-module or configured parameter effect exists, proven
borrowing preserves the obligation while guaranteed close or transfer ends it.
Unavailable helper behavior retains the conservative direct-transfer boundary.

The bounded shared obligation engine summarizes each reachable CFG block as
open, completed, transferred, or lost and stops a path after completion or
transfer. It uses the shared CFG's no-return policy and visits each block state
at most once per candidate. `sql-transaction-not-completed` uses the same
engine with exact `Commit` and `Rollback` completion effects.

The rule remains `suspicious`, not `correctness`, because ownership may be
transferred through conventions it cannot prove. No fix is offered because the
correct cleanup point and error policy are contextual.

## Admission Evidence

Focused fixtures cover leaked files, deferred and explicit close, partial and
complete branches, reassignment, implicit returns, argument and result
transfer, exact ranges, suppressions, generated and type-error policy, source
versions, CLI output, baselines, and absence of fixes.

Five complete-load iterations on Go 1.26.6, Darwin arm64, Apple M4 Max averaged
`106,638,825 ns/op`, `1,969,809 B/op`, and `15,669 allocs/op` for the one-file
CFG fixture. Imports and package loading dominate the measurement; this is
proportional admission evidence rather than a stable latency budget.

Non-mutating dogfood completed without findings over Glippy and
`go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

## Revisit Trigger

Add interprocedural completion and transfer facts only when real missed-defect
evidence justifies their cost. Expand resource shapes only from reviewed
occurrences; do not infer that every method named `Close` is an ownership
boundary.
