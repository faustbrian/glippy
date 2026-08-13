# `resource-not-closed` Rule Admission, 2026-08-13

## Decision

Admit `resource-not-closed` to the opt-in `suspicious` preset at warning
severity. The native types-tier rule tracks locally owned call results with a
static `Close() error` method and reports only values that are neither closed
nor conservatively transferred.

## Defect And Existing Tools

Leaking files, connections, compressors, and similar resources can exhaust
descriptors, retain network capacity, or defer important flush and shutdown
work. Go permits these local results to leave scope without a close. Default Go
1.26.5 vet has no general local closer ownership analyzer; Staticcheck has
specific resource rules but no equivalent general transfer contract.

Sources inspected on 2026-08-13 were Go 1.26.5 type and method-set semantics,
the default vet catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de` for must-use and ownership-inspired
lint boundaries.

## Precision, Policy, And Fixes

The rule recognizes direct call assignments whose corresponding result has
`Close() error`. It treats a direct close as cleanup and a direct argument,
return, send, assignment, composite insertion, method value, or closure capture
as a transfer. Zero-result `Close` methods are excluded after dogfood showed
that a method name alone includes non-resource reflection values.

The first contract is intentionally path-insensitive: any close counts, so it
can miss a cleanup that runs on only one branch. It is `suspicious`, not
`correctness`, because ownership may be transferred through conventions the
rule cannot prove. No fix is offered because the correct cleanup point and
error policy are contextual.

## Admission Evidence

Focused fixtures cover leaked files, deferred and explicit close, returns,
argument transfer, exact ranges, suppressions, generated and type-error policy,
source versions, CLI output, baselines, and absence of fixes.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
medians of `167,748,492 ns/op`, `1,935,420 B/op`, and `15,617 allocs/op` for the
one-file fixture. Imports and package loading dominate the measurement.

Non-mutating dogfood completed without findings over Glippy and
`go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

## Revisit Trigger

Add all-path cleanup proof only behind the shared CFG or SSA tiers, and expand
resource shapes only from real missed-defect evidence.
