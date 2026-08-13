# `subsumed-condition` Rule Admission, 2026-08-13

## Decision

Admit `subsumed-condition` to the opt-in `suspicious` preset at warning
severity. The native types-tier rule excludes generated and ill-typed packages
and offers no fix.

## Defect And Existing Tools

In `if value > 0 { ... } else if value > 10 { ... }`, the second branch is
unreachable because every value satisfying it was accepted by the first. This
commonly comes from ordering thresholds broadest-first or copying the wrong
operator. The compiler and default vet accept it. Staticcheck and Go vet cover
adjacent unreachable-condition families but have no exact ordered-bound rule;
Clippy supplies the closest product precedent through its condition and
unreachable-code lints.

Sources inspected on 2026-08-13 were Go 1.26.5, current Go tools at
`18332fec72972efbb8ab9881984fec2d8cfc2b58`, Staticcheck at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`. The rule remains suspicious until
dogfood establishes whether intentional broad-before-narrow branches are rare.

## Precision, Policy, And Fixes

The initial contract compares adjacent `if` and `else if` ordered integer
conditions over the same type object and compile-time bounds. It supports the
strict and inclusive relationships that prove the later value set is contained
in the earlier set. Initializers, compound expressions, selectors, calls, and
non-adjacent reasoning are excluded.

No fix is registered because reordering branches can change effects and
correcting a threshold requires intent. Exact suppressions and deterministic
baselines use the shared coordinator.

## Admission Evidence

Fixtures cover upper and lower bounds, safe branch order, initializer
exclusion, exact primary and related ranges, metadata, suppressions, generated
files, ill-typed packages, CLI exposure, deterministic baselines, and
incremental benchmarking. Three complete-load samples on Go 1.26.5, Darwin
arm64, Apple M4 Max measured a median of 63,064,291 ns/op, 187,712 B/op, and
1,306 allocations/op.

Non-mutating comparison-catalog dogfood completed with no remaining findings
over Glippy and `go-libraries/pkg/prompts` at
`f232c8265a9c011daa38027718c44fc6507e9dcf`. The prompts repository's
concurrently changing dirty worktree was not modified.
