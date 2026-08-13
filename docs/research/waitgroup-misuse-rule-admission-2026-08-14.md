# `waitgroup-misuse` Rule Admission, 2026-08-14

## Decision

Admit the Go `waitgroup` analyzer as warning-level `correctness`. Calling
`WaitGroup.Add` inside the goroutine being counted races with `Wait`, which may
return before the increment. Go 1.26.6 default `go vet` provides authoritative
detection; Glippy integrates it with its rule contracts.

## Boundary

The typed x/tools v0.48.0 analyzer reports the direct function-literal pattern
where `Add` is the first launched statement. Incrementing before launch does not
report. Indirect synchronization protocols are false-negative territory rather
than guessed findings. Generated files and ill-typed packages are excluded,
exact suppressions apply, and Go versions before 1.25 do not select the rule.
Moving synchronization statements is not automatically fixed.

## Cost And Dogfood

A one-iteration package-load probe measured 155,930,167 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
