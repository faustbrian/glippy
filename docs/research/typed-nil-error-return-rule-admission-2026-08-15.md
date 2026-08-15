# `typed-nil-error-return` Rule Admission, 2026-08-15

## Decision

Admit `typed-nil-error-return` as an opt-in `suspicious` SSA rule at warning
severity. It reports explicit return operands where a definitely nil concrete
value is converted to an `error` interface result. The resulting interface is
non-nil because it retains the concrete dynamic type.

Although the reported value is definite, enabling an SSA rule in the default
correctness preset would make ordinary syntax-only lint configurations load
packages and SSA. The rule remains opt-in until default semantic-analysis cost
and source-error behavior are an explicit product decision.

The rule has no fix. Replacing the operand with `nil` changes observable
behavior, while adding a branch or changing the function signature requires
API and product-intent judgment.

## Defect And Existing Tools

This accepted Go program returns a non-nil `error`:

```go
type Problem struct{}
func (*Problem) Error() string { return "problem" }

func run() error {
	var problem *Problem
	return problem
}
```

The compiler and default `go vet` analyzers do not report the conversion.
Staticcheck SA4023 is the primary authority; its current implementation and
documentation were inspected at commit
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
SA4023 reports comparisons that can never observe a nil interface, using
interprocedural nilness summaries of callees. Glippy reports the narrower
definite defect at its return site and does not require a caller comparison.

## Precision Boundary

The rule uses the shared SSA program with source-expression debug mappings.
It considers only explicit returns whose operands correspond one-to-one with
the function result tuple. The destination must be an interface implementing
Go's built-in `error`; the source must be a non-interface concrete type
assignable to that result. Untyped `nil`, already interface-typed operands,
bare returns, and tuple-returning calls are excluded.

An operand reports only when its SSA value is a nil constant or is composed
exclusively from definitely nil values through type changes, interface
construction, or Phi joins. A single non-nil or unknown incoming value prevents
the finding. This supports pointer and other nilable concrete error types while
preferring false negatives over speculative diagnostics.

Generated files and packages with type errors are excluded. The suspicious
preset keeps this SSA cost out of default syntax-only workflows. Exact
suppressions, configured severities, minimum Go-version selection, baselines,
and deterministic output use the shared product contracts.

## Fix Safety

No safe, suggestion, or unsafe fix is registered. Returning untyped `nil`
usually expresses the intended success result but changes the existing public
behavior. Glippy cannot infer whether compatibility requires the non-nil
interface or whether the function should instead return a concrete type.

## Evidence And Cost

The focused product test first failed because the rule ID was unknown. Current
fixtures cover zero-valued pointers and slices, explicit typed-nil conversion,
multiple and richer error results, all-nil and maybe-non-nil joins, untyped nil,
interface operands, unknown concrete operands, non-nil values, exact ranges,
metadata, generated and ill-typed exclusions, suppressions, configured
severity, minimum Go version, and absence of fixes.

The admission benchmark analyzes 100 functions through one package load and
shared SSA build. Across three Darwin arm64 samples, the median was 59.3 ms,
about 2.02 MB allocated, and 21,494 allocations. This is proportional rule
admission evidence rather than a stable release latency budget.

## Revisit Triggers

Consider promotion to `correctness` only with an explicit default SSA loading
and source-error policy. Consider tuple-returning calls and named bare returns
only when exact source ownership remains deterministic. Consider
interprocedural nilness facts only when they improve useful coverage without
making the correctness preset depend on speculative may-be-nil inference. Do
not add a fix without evidence for one canonical semantics-preserving
transformation.
