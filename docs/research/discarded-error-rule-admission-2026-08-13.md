# Discarded Error Rule Admission

Date: 2026-08-13

## Decision

Admit `discarded-error` to the opt-in `suspicious` preset at the types tier. It
reports call statements whose result tuple contains an error and excludes
formatted output, documented infallible writes, and test files by default.
`include-tests = true` opts test sources into the same contract.

## Evidence

- Clippy's `unused_must_use` and PHPStan's unused-result checks make ignored
  failure channels visible instead of relying on callers to notice them.
- Go permits a function or method call as a statement even when all return
  values, including errors, are discarded. The compiler and default `go vet`
  do not diagnose the general case.
- Focused fixtures cover single and multiple results, standard-library
  constructors, handled and explicitly blank-assigned errors, exact ranges,
  policy boundaries, and source versions.

Sources inspected on 2026-08-13 were Go 1.26.5's call-statement semantics and
default `unusedresult` vet catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de` including must-use result lints.
The default vet analyzer covers a configured function list, not general error
results.

## False-Positive Boundary

Best-effort operations can intentionally ignore failure. The rule is therefore
opt-in `suspicious`, and an intentional call needs an explicit handling choice
or reasoned suppression. It does not report explicit blank assignments,
formatted-output helpers, or test files unless configured. No fix is offered
because the correct propagation or recovery policy is contextual.

## Cost

Five package-analysis iterations on Apple M4 Max measured medians of
`43,791,650 ns/op`, `176,988 B/op`, and `1,198 allocs/op`. Package loading
dominates the single-rule fixture.

Initial Glippy dogfood exposed two deliberately ignored cleanup-close errors;
they were made explicit blank assignments. Initial prompts dogfood found only
fixture-driving test calls, which established the default test exclusion and
the explicit `include-tests` option. Final non-mutating dogfood completed
without findings over Glippy and `go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

## Revisit Trigger

Revisit the infallible allowlist when standard-library APIs change or dogfood
identifies another documented always-nil result with material noise.
