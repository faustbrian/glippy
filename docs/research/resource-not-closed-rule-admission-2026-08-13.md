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

The 2026-08-20 nil-result refinement discharges only the exact CFG edge where
`resource == nil` or `resource != nil` proves that no owned resource exists.
The non-nil edge must still close or transfer the value. Compound conditions,
aliases, and indirect comparisons remain conservative.

The 2026-08-20 cleanup-managed result refinement recognizes an exact
`testing.T.Cleanup` function-literal callback when a helper returns one stable
direct local object and every normally returning callback path closes that
object directly or through a statically resolved helper with a guaranteed
close parameter effect. The summary crosses selected-module package loads by
stable function and result identity. Package variants must agree. Conditional
registration or close, observation-only callbacks, goroutines, nested
functions, reassignment, aliases, and non-testing cleanup APIs remain
unmanaged. Cleanup registered on a copied `testing.T` value is also unmanaged
because it is not attached to the test runner's pointer.

The receiver terminal-effect refinement on 2026-08-20 also proves a cleanup
callback that calls an exact method such as `resource.Shutdown()` when the
method closes its receiver on every normal path. Known receiver effects
propagate through statically resolved receiver methods, method expressions, and
parameter helpers. Reachable active-workspace and local filesystem replacement
modules now join root modules inside the selected local-source boundary;
downloaded dependencies and unrelated workspace modules remain excluded.

The constructor-callback refinement on 2026-08-20 applies the same conservative
closure-capture boundary when a direct function-literal argument references the
result being assigned by that constructor call. It also resolves the final
direct assignment to a local argument in the acquisition block, covering stable
callback containers passed through a configuration value. The indirect value
remains stable only when intervening statements do not mutate it, expose it to a
call, or take its address; harmless reads remain eligible. The callback may be
retained and later own or close the resource, so the generic rule treats the
acquisition as transferred. A callback that captures only another value, a
replaced or escaped container, a mutated field, or a capture assigned only
through conditional or nested control flow does not discharge the new resource
obligation.

The transfer classification applies only to the leak rule: the shared closer
acquisition remains eligible for `resource-used-after-close`, so an explicit
local close followed by an operation still reports even when a constructor
callback also captures the value.

The rule remains `suspicious`, not `correctness`, because ownership may be
transferred through conventions it cannot prove. No fix is offered because the
correct cleanup point and error policy are contextual.

## Admission Evidence

Focused fixtures cover leaked files, deferred and explicit close, partial and
complete branches, reassignment, implicit returns, argument and result
transfer, exact ranges, suppressions, generated and type-error policy, source
versions, CLI output, baselines, absence of fixes, same-package and imported
cleanup-managed results, disagreeing package variants, and conservative
conditional, asynchronous, nested, replacement, and copied-test-handle
boundaries, plus direct and stable-container constructor callbacks and their
same-block dominance, mutation, escape, and harmless-read boundaries.

Five complete-load iterations on Go 1.26.6, Darwin arm64, Apple M4 Max averaged
`106,638,825 ns/op`, `1,969,809 B/op`, and `15,669 allocs/op` for the one-file
CFG fixture. Imports and package loading dominate the measurement; this is
proportional admission evidence rather than a stable latency budget.

After the nil-edge refinement, five one-iteration samples on the same runtime
and host measured `71,115,750-75,769,167 ns/op`,
`2,103,768-2,675,120 B/op`, and `16,835-17,277 allocs/op`. The range remains
proportional rule evidence rather than a release latency or allocation budget.

Non-mutating dogfood completed without findings over Glippy and
`go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

The nil-result refinement was reproduced against
`go-libraries/pkg/http-client`: an `io.ReadCloser` result guarded against nil
before transfer into a request-body wrapper previously reported as leaked.
The corrected analyzer removes that diagnostic and two equivalent nil-result
false positives while retaining the other 22 findings for separate ownership
review. The external repository remained unmodified.

The constructor-callback refinement was reproduced against
`go-libraries/pkg/http-client` at
`b2bcdc33836d6800db0f51ebf9b816e5d5fb33ee`. Direct and stable-container
captures remove the two false positives in `client_test.go` and
`session_test.go`. The five retained findings are one response body in
`compression_test.go` and four fake files in `resume_test.go`. Exact-rule
dogfood remained clean on Glippy and `go-libraries/pkg/prompts` at the same
revision. These were non-mutating lint runs; unrelated concurrent dirty changes
in the shared external repository remain outside this batch.

Five one-iteration samples on Go 1.26.7, Darwin arm64, Apple M4 Max measured
`72,107,625-80,146,375 ns/op`, `2,133,768-2,704,144 B/op`, and
`17,039-17,473 allocs/op`. This remains proportional rule evidence rather than
a portable latency or allocation budget.

The stability follow-up retained the exact five `go-libraries/pkg/http-client`
findings at `556e3d5d9a6cd7981f2aaabdbc0f7aaef9ecc7ae` while exact-rule
dogfood remained clean on Glippy and `go-libraries/pkg/prompts`. All external
checks were non-mutating; unrelated dirty state remained outside this batch.
Five one-iteration samples on the same Go 1.26.7 Darwin arm64 host measured
`72,960,291-75,878,708 ns/op`, `2,131,400-2,707,320 B/op`, and
`17,043-17,487 allocs/op`.

## Revisit Trigger

Add interprocedural completion and transfer facts only when real missed-defect
evidence justifies their cost. Expand resource shapes only from reviewed
occurrences; do not infer that every method named `Close` is an ownership
boundary.
