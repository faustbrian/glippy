# `printf-arguments` Rule Admission, 2026-08-14

## Decision

Admit the Go `printf` analyzer as warning-level `correctness`. Invalid directives,
missing arguments, and directive/type mismatches produce observably malformed
output. Go 1.26.6 enables this analyzer in default `go vet`; Glippy adds stable
identity, policy, deterministic reporting, facts, baselines, and fix safety.

## Boundary

The typed rule follows x/tools v0.48.0's standard function catalog and inferred
wrapper facts. It does not expose the analyzer's package-global `funcs` flag.
Nearby valid directive/type pairs do not report. Non-constant single-argument
formats report under the upstream source-version gate; inserting `%s` is a
suggestion because caller intent is not proven.

Generated files and ill-typed packages are excluded. Exact suppressions apply;
Go sources before 1.25 do not select the rule. Focused behavioral tests cover
diagnostics, exact ranges, shared policy, JSON, baseline, explain, and repeated
fixing.

## Cost And Dogfood

A one-iteration typed package probe measured 587,996,708 ns on darwin/arm64; it
includes package loading and wrapper-fact execution and is not a latency budget.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 files in
`go-libraries/pkg/prompts` at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
