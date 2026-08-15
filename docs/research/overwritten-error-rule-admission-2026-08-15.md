# `overwritten-error` Rule Admission, 2026-08-15

## Decision

Admit `overwritten-error` as an opt-in `suspicious` SSA rule at warning
severity. It reports a direct identifier assignment or initialized variable
whose value is assignable to Go's built-in `error` interface, has no observable
SSA use, and is followed by another definition of the same typed object.

The rule has no fix. Recovering the lost error check requires control-flow and
product-intent decisions that cannot be inferred from the overwritten value.

## Defect And Existing Tools

Reusing `err` before checking the previous operation loses its failure:

```go
value, err := first()
value, err = second()
if err != nil {
	return err
}
```

The compiler accepts the program because `err` is eventually used. The default
Go vet analyzers do not report this error-flow defect. Staticcheck SA4006 is the
primary authority; its current implementation and documentation were inspected
at commit
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
SA4006 reports any assigned value with no later use. Glippy deliberately narrows
that contract to error-typed values with a later definition of the same object,
which excludes dead parameter assignments and unrelated ineffective values.

## Precision Boundary

The native rule uses the shared x/tools SSA program with expression debug
mapping enabled only when a selected rule requires it. It indexes debug
references once per function rather than repeatedly scanning SSA instructions.
It maps tuple-returning calls through `ssa.Extract`, follows branch joins and
implicit interface conversions, ignores debug references, and treats switch
tags as uses. A direct `_ = err` assignment or blank variable declaration is an
explicit observation and prevents a finding.

Any real use prevents a finding, even when some paths still overwrite the
value. This intentionally favors false negatives over warnings about
partially observed values. Direct fields, indexes, dereferences, range
variables, incoming parameter values, generated files, and ill-typed packages
are excluded. Explicit assignments to parameters are still analyzed.
Address-taken values may also be missed when SSA represents them through
memory. Exact suppressions, configured severity, source-version selection,
baselines, and deterministic reporting use the shared product contracts.

## Fix Safety

No safe, suggestion, or unsafe fix is registered. Adding a check, choosing a
return value, logging, joining errors, or intentionally discarding the first
error are observably different repairs.

## Evidence And Cost

The focused product test first failed because `overwritten-error` was unknown.
A second red boundary proved that an assigned-but-not-overwritten parameter
must remain unreported. Current fixtures cover tuple extraction, ordinary
checks, explicit blank observation, switch use, initialized declarations,
concrete error implementations, branch joins, closure capture, exact ranges,
implicit concrete-to-interface and interface-to-interface conversions,
generated and ill-typed exclusions, suppressions, severity, minimum Go version,
metadata, and absence of fixes. The shared SSA test proves that expression
debug mappings remain disabled for ordinary SSA rules and are enabled on demand
for this rule class.

A one-iteration 100-function package probe on Darwin arm64 produced 100
findings. Across three final-tree samples, the median was 46.0 ms, about 2.60 MB
allocated, and 24,161 allocations; this includes package loading and SSA
construction. A single-function scaling probe measured 44.5 ms and 1.02 MB for
100 assignments versus 54.1 ms and 9.02 MB for 1,000 assignments. The 10x
source increase therefore retained proportional allocation growth without the
quadratic lookup latency of repeated `ValueForExpr` scans. Non-mutating
exact-rule dogfood completed without findings on Glippy's current working tree
based on `aba6e6f` and on `go-libraries/pkg/prompts` at
`0b9bb08727cc1cabdc674bbfe7082fc5642c3f2a`.

## Revisit Triggers

Revisit the suspicious preset assignment only after broader repositories show
near-zero noise. Consider address-taken and loop-carried definitions when
x/tools SSA can prove them without widening the rule to generic unused-value
analysis. Do not add a fix without evidence for one canonical semantics-
preserving repair.
