# `errors-as-target` Rule Admission, 2026-08-13

## Decision

Admit `errors-as-target` to the default `correctness` preset at warning
severity by adapting Go's authoritative `errorsas` analyzer. Glippy preserves
the analyzer's types-tier behavior and deterministic whole-call range without
reimplementing `errors.As` target semantics.

## Defect And Existing Tools

An invalid second argument to `errors.As` can panic at runtime. A `*error`
target is accepted by the API but is ineffective because it matches any
non-nil error instead of a specific type. Go 1.26.5 vet enables `errorsas` by
default, so the rule is not novel detection; admission gives the existing
check Glippy's presets, suppressions, baselines, reporters, generated-file
policy, and deterministic scheduler.

Sources inspected on 2026-08-13 were Go 1.26.5's `errorsas` analyzer and vet
catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`. Staticcheck and Clippy do not
provide a more authoritative Go `errors.As` contract than the standard
analyzer.

## Precision, Policy, And Fixes

The adapted analyzer follows the exact `errors.As` object and statically checks
that the target is a non-nil pointer to an error-implementing type or an
interface. It deliberately permits `any` forwarding and excludes the `errors`
package itself. Diagnostics use the upstream whole-call range. Generated files
and packages with type errors are excluded.

No fix is offered. Adding `&` is not universally correct because the intended
target type and ownership may themselves be wrong.

## Admission Evidence

Focused fixtures cover invalid and valid target types, exact ranges, no-fix
behavior, suppressions, baselines, generated and type-error policy, source
versions, and CLI JSON output.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
`143,855,375 ns/op`, `1,243,379 B/op`, and `9,327 allocs/op` for the one-file
fixture. Package loading dominates the measurement.

Non-mutating correctness-and-suspicious dogfood completed without findings over
Glippy and `go-libraries/pkg/prompts` at
`d55cfaaf650681fdff0530d05988353570b2e16b`; the prompts head and pre-existing
dirty state were unchanged by the final run.

## Revisit Trigger

Track upstream `errorsas` behavior and only diverge if Glippy needs a proven
range, language-version, or false-positive contract that the analyzer cannot
provide.
