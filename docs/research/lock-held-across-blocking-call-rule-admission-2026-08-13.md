# `lock-held-across-blocking-call` Rule Admission, 2026-08-13

## Decision

Admit `lock-held-across-blocking-call` to the opt-in `suspicious` preset at
warning severity. The native types-tier rule tracks direct identifier receivers
for `sync.Mutex` and `sync.RWMutex` within one statement list and reports a
deliberately small set of calls whose standard-library contract is to wait.

## Defect And Existing Tools

Waiting while a lock is held can amplify latency, block unrelated goroutines,
and contribute to deadlocks. The compiler and default Go 1.26.5 vet accept the
pattern. Clippy's lock-across-suspension family establishes the product value of
making hidden critical-section waits visible, but Go requires a Go-specific
lock and API identity model.

Sources inspected on 2026-08-13 were Go 1.26.5 `sync`, `time`, and `os/exec`
contracts and default vet catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de` including its lock-across-await
analysis.

## Precision, Policy, And Fixes

The initial blocking set is `time.Sleep`, `sync.Cond.Wait`,
`sync.WaitGroup.Wait`, and `os/exec.Cmd.Wait`. Object and method identity exclude
lookalikes. Acquisition order is retained deterministically when multiple locks
are held. The rule does not guess whether arbitrary I/O or project functions
block, and it does not propagate lock state through nested control flow until a
CFG-backed contract can do so precisely.

Deliberate serialized waiting exists, so the rule is `suspicious`. No fix is
offered because moving a call or lock boundary can change invariants and race
behavior.

## Admission Evidence

Focused fixtures cover mutex and read-lock acquisition, sleep and wait calls,
unlock-before-wait, lookalike types, exact primary and related ranges,
suppressions, generated and type-error policy, source versions, CLI output,
baselines, and absence of fixes.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
medians of `143,194,283 ns/op`, `1,561,252 B/op`, and `12,549 allocs/op` for the
one-file fixture. Imports and package loading dominate the measurement.

Non-mutating dogfood completed without findings over Glippy and
`go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

## Revisit Trigger

Move to CFG state propagation when nested critical-section evidence justifies
the added cost, and add blocking APIs only with authoritative contracts.
