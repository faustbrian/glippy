# Cross-Package Return-State Evidence, 2026-08-15

## Scope

Effect schema version 3 adds stable relationships between nil-capable results
and exact built-in `error` results. The first consumer is `nilness`, which uses
the summaries for direct dereferences and nil comparisons dominated by an
error branch.

## Contract

The analysis inspects explicit return expressions and records a state only
when every return for the selected error state agrees. Exact `nil`, address,
`new`, `make`, function-literal, slice/map-literal, `errors.New`, and
`fmt.Errorf` forms are recognized. Bare returns, delegated or recursive
results, `&*x`, unknown error construction, aliases, conflicting returns,
ill-typed packages, and helpers outside selected modules remain unknown.

Dependency packages retain stable `types.ObjectString` identities across
independent loads. Root summaries are built only when an enabled SSA consumer
requires effect facts. Dependency source remains excluded from diagnostic and
fix ownership. The canonical `native-effects-v3` digest orders function
identities, both result indexes, and both error-state relationships.

## Behavioral Evidence

The integration fixture proves that a same-module imported helper returning
`(nil, non-nil error)` and `(non-nil value, nil)` produces the expected
nil-dereference and impossible-comparison diagnostics. Direct analysis covers
independent identity, ordering and digest sensitivity, exact recognized forms,
nested function literals, bare returns, unknown results, recursion, and
conflicting returns. Separate regressions prove demand-driven root exposure,
dependency-change cache invalidation, and exclusion of a locally replaced
external module from effect loading and diagnostic ownership.

## Performance

The proportional benchmark analyzes 100 root functions that call one imported
helper and verifies 200 returned-state findings. Five one-iteration samples on
Go 1.26.6, Darwin arm64, and an Apple M4 Max measured a 287.8 ms median with
about 6.10 MB and 72,782 allocations. The range was 221.7 ms to 370.4 ms and
remains a directional development measurement rather than a release budget.

A freshly built working-tree binary based on Glippy revision
`4cce41bd44f34bb02a050fef61ff1eeaec22933f` ran exact-rule, non-mutating
`nilness` dogfood over Glippy and `go-libraries/pkg/prompts` at revision
`d7d487f309f3921eff48c5bb277ff11a4fef7077`. Both completed without findings
or tool failure after their task-owned module caches were populated. The
pre-existing dirty prompts `go.sum` remained byte-identical.

## Remaining Boundary

The first consumer follows direct SSA extracts, direct error comparisons, and
direct uses dominated by the selected successor. Phi nodes, aliases, returned
state delegation, custom error constructors, workspace-module sharing, and
third-party facts are not inferred. These exclusions preserve the rule's
false-positive boundary while future corpus evidence determines whether a
broader data-flow summary is justified.
