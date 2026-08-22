# `busy-select-loop` Rule Admission, 2026-08-22

## Decision

Admit `busy-select-loop` to the opt-in `suspicious` preset at warning severity.
A conditionless loop whose only statement is a select with an empty default
cannot block when no communication is ready:

```go
for {
	select {
	case value := <-updates:
		consume(value)
	default:
	}
}
```

The loop immediately begins another iteration and can consume an entire CPU
core without doing work. Removing the empty default allows the select to block.

The rule does not belong in the default `correctness` preset. Purpose-built
stress and scheduler-starvation tests sometimes spin deliberately, so deciding
whether the loop should block requires limited contextual judgment. Those cases
use a narrow suppression.

No fix is offered. Removing the default changes blocking and scheduling
behavior, and a source edit can also change the ownership or placement of
comments inside the clause.

## Defect And Existing Tools

Staticcheck 2026.2.1 SA5004 is the primary external authority. Its
[`sa5004.go`](https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa5004/sa5004.go)
implementation matches a conditionless `for` whose body is one select and
reports an empty default clause. Its
[`CheckLoopEmptyDefault` fixture](https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa5004/testdata/go1.0/CheckLoopEmptyDefault/CheckLoopEmptyDefault.go)
excludes standalone selects, nonempty defaults, and selects without defaults.

Real fixes demonstrate both the defect and the intent boundary:

- [`miyamo2/qilin@8fa9fd6`](https://github.com/miyamo2/qilin/commit/8fa9fd698f5cad9ab25836309ad2f522d3a3d495)
  removed an empty no-op default from a stream connection loop so the select
  could block instead of spinning.
- [`AgentsMesh/AgentsMesh@58d31ad`](https://github.com/AgentsMesh/AgentsMesh/commit/58d31adb64bf4f6d0ee9e2a26172617160722334)
  added `runtime.Gosched` to a stress loop after SA5004 identified the empty
  default spin.
- [`yasyf/synckit@8a6a16f`](https://github.com/yasyf/synckit/commit/8a6a16fdf64d0c5653a3b265364eb44eb583de6c)
  documented and suppressed an intentional scheduler-starvation spin, proving
  why the rule requires contextual judgment.

The Go compiler accepts every form, and the Go 1.27 default vet catalog has no
equivalent check.

## Precision Contract

The rule uses filtered syntax traversal over `for` statements. It reports only
when the loop has no initializer, condition, or post statement, its body
contains exactly one direct select statement, and that select has a default
clause with no executable statements. Comment-only defaults and explicit empty
statements still report because neither makes the loop block or perform work.

Standalone selects, range loops, conditioned loops, loops with init or post
statements, loop bodies with surrounding work, nonempty defaults, and selects
without defaults do not report. The primary range is the exact `default:`
clause. Shared severity, suppression, generated-file, and source-version
policies apply.

## Evidence And Cost Boundary

The focused product test first failed because the rule ID was absent. Current
fixtures cover receive and send cases, default-only selects, comment-only
defaults, explicit empty statements, exact ranges, nearby excluded control-flow
shapes, metadata, minimum Go-version selection, configured severity,
suppressions, generated files, and the explicit no-fix boundary.

The implementation performs constant work for each selected direct for-loop
body plus one linear scan of that select's clauses. It adds no types, CFG, SSA,
fact, cache, subprocess, or network requirement. Exact-rule non-mutating
dogfood over Glippy and `go-libraries/pkg/prompts` produced no diagnostics or
tool failures. No benchmark, RSS probe, signal test, interruption test,
process-tree test, descendant-cleanup test, or Docker test was executed for
this admission.

## Revisit Triggers

Revisit preset placement only if broad external dogfood shows that deliberate
busy polling is rare enough for near-zero-noise default correctness. Consider a
suggestion only if blocking behavior and comment ownership can be presented for
explicit user review without implying semantic safety.
