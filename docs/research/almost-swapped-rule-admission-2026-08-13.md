# `almost-swapped` Rule Admission, 2026-08-13

## Decision

Admit `almost-swapped` to the opt-in `suspicious` preset at warning severity.
The native types-tier rule excludes generated and ill-typed packages and offers
no fix.

## Defect And Existing Tools

`left = right` followed immediately by `right = left` does not swap the values.
The first assignment destroys the original left value, so the second leaves
both variables equal to the original right value. The compiler and default vet
accept the sequence.

Clippy's `almost_swapped` lint detects the equivalent copied-assignment defect
in Rust and recommends the language's swap primitive. Staticcheck
v0.8.0-rc.1 and the Go 1.26.5 vet catalog have no exact rule. Other Go linter
catalogs were treated as coverage references rather than execution frontends.

Sources inspected on 2026-08-13:

- current Clippy `almost_swapped` implementation and documentation;
- Go 1.26.5 assignment semantics and default vet catalog; and
- Staticcheck v0.8.0-rc.1 rule catalog.

## Precision, Policy, And Fixes

The rule requires two consecutive ordinary assignments, each with one simple
identifier on each side. Type object identity proves the cross-assignment
relationship and excludes shadowed names. Selectors, indexes, dereferences,
compound assignments, declarations, multiple assignment, and intervening
statements are excluded because their evaluation or intent differs.

The rule is `suspicious`, not default `correctness`, because deliberately
converging two variables remains possible. Generated and ill-typed packages are
excluded. Exact suppressions and deterministic baselines use the shared product
coordinator.

No fix is registered. Rewriting to simultaneous assignment would restore the
likely intent but change behavior, so it is not a semantics-preserving safe
fix.

## Admission Evidence

Focused fixtures cover the defective sequence, simultaneous swaps, intervening
statements, declarations, exact ranges, metadata, suppressions, generated and
type-error policy, deterministic baseline output, and absence of fixes.

Three complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured a
median of 41,017,329 ns/op, 173,986 B/op, and 1,180 allocations/op for the
one-file rule fixture. Non-mutating suspicious-preset lint completed with no
findings over Glippy and `go-libraries/pkg/prompts` at
`633a5508c570d08b8976689a206f9df27e73ff90`; the prompts repository head and
pre-existing dirty status were unchanged.
