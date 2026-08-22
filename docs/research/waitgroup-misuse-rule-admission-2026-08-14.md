# `waitgroup-misuse` Rule Admission, 2026-08-14

## Decision

Admit `waitgroup-misuse` as warning-level `correctness`. Calling a positive
`WaitGroup.Add` inside the goroutine being counted races with an outside
`Wait`, which may return before the increment. Go 1.26.6 default `go vet`
provides the authoritative direct pattern. Glippy conservatively narrows that
pattern by requiring a later direct `Wait`
in straight-line function-body statements, then adds a native same-package
extension for exact helper chains.

## Boundary

Direct launched positive `Add` calls and function literals report when the
call is the first executed statement and a direct outside `Wait` follows later
in the launching function body's straight-line statements. Launches or outside
waits nested in conditional, loop, switch, select, case, communication, or
standalone blocks remain conservative at the types tier. A bounded static
helper chain additionally requires every call to be an unconditional first
statement and exact parameter identity to map the `Add` receiver back to the
launched argument. An intervening
assignment to that receiver makes the identity unknown and does not report.
Intervening synchronously evaluated calls or explicit channel synchronization
between launch and `Wait` also stop the proof because they may establish that
`Add` completed. Later goroutine or deferred calls do not establish ordering.
A helper-owned group that performs its own ordered `Add`, worker launch, and
`Wait` does not report.

Zero, negative, and non-constant deltas; fields or globals hidden behind
helpers; dynamic or interface calls; side-effecting launch evaluation; nested
control-flow launches or waits; and synchronization before `Add` remain
false-negative territory rather than guessed findings. Generated files and
ill-typed packages are excluded, exact suppressions apply, and Go versions
before 1.25 do not select the rule. Moving synchronization statements is not
automatically fixed.

## Cost And Dogfood

Five one-iteration package-load probes for the native implementation measured
82.35-188.92 ms on Darwin arm64, with a 91.76 ms median, 5.62 MB median
allocation, and approximately 45,378 allocations. Package loading dominates
this proportional admission probe; it is not a release latency budget.

Non-mutating exact-rule dogfood on the current Glippy candidate and
`go-libraries/pkg/prompts` produced no diagnostics or tool failures. Focused
fixtures cover direct function literals, direct method expressions, bounded
helper chains, receiver identity, positive versus zero/negative/non-constant
deltas, launch-time and caller-side synchronization, helper-owned WaitGroups,
suppression, generated files, type errors, and source-version selection.

## Revisit Trigger

Revisit helper tracing only when CFG-backed caller-side wait placement or
field-sensitive receiver identity can expand coverage without weakening the
near-zero-false-positive correctness contract.
