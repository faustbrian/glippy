# ADR 0017: Cross-Package Semantic Effect Facts

- Status: accepted for v0.4 development
- Date: 2026-08-15

## Context And Evidence

Glippy already propagated no-return behavior through functions in one loaded
package and recognized an exact standard-library terminal set. Calls to a
same-module helper loaded only from export data remained conservatively
returning. That created false continuations for `context-cancel-leak`,
`nilness`, stream checks, and lifecycle rules.

Using the complete `go/analysis` control-flow fact graph was previously
measured at 374 retained sources, about 1.4 GB allocated, and more than 14
million allocations for a one-file cancellation target. Loading the complete
dependency syntax graph into Glippy's ordinary source set would also make
dependencies appear eligible for reporting and fixing unless every later
boundary remained perfectly filtered.

## Decision

Native CFG and SSA metadata may declare `RequiresEffectFacts`. The scheduler
loads only imported packages whose paths belong to the selected root modules,
in deterministic dependency layers. It derives summaries through independent
typed loads and identifies functions with stable package-qualified object
strings rather than process pointers.

Effect schema version 3 retains the proven no-return and parameter summaries
and adds returned nil/error relationships. A known parameter summary
distinguishes borrowing from an effect that every normally returning path
reaches: conventional `Close`,
`database/sql` `Commit` or `Rollback`, cancellation invocation, or ownership
transfer. A return summary records a nil-capable result only when every explicit
return for an exact built-in `error` state agrees. Exact `nil`, address, `new`,
`make`, function-literal, slice/map-literal, `errors.New`, and `fmt.Errorf`
forms are recognized. Bare results, delegated or recursive results, `&*x`,
unknown error construction, conflicting returns, dynamic calls, interface dispatch,
unsupported local aliases, ill-typed packages, third-party modules, and
workspace modules outside the selected module paths remain conservative.

The shared CFG builder and SSA no-return predicate consume the same immutable
summary set. Effect source is not added to the selected source set, is never a
diagnostic or fix target, and is not built into the root SSA program. Loading
is demand-driven by enabled rule metadata and remains serialized. Root and
effect sources share the configured package, file, and byte limits.

Native cache snapshots bind the effect requirement. Cache keys include a
digest identified as `native-effects-v3`, derived from the schema version,
canonically ordered stable no-return identities, ordered parameter-effect
records, and ordered returned-state records containing both result indexes and
both error-state relationships. A changed summary therefore invalidates native
diagnostics without making source pointers persistent.

## Alternatives

- Load the complete dependency syntax closure with `packages.NeedDeps`:
  rejected because the measured allocation and source-retention cost is
  disproportionate and includes standard-library bodies already covered by
  exact terminal contracts.
- Match helper names such as `Fatal`, `Must`, or `Exit`: rejected because names
  do not prove control-flow behavior.
- Reuse type-object pointers across independent loads: rejected because those
  identities are process- and load-specific.
- Treat every helper argument as ownership transfer: retained as the fallback
  only when no safe summary is available because proven borrowing must not hide
  a local leak.
- Infer returned states through arbitrary calls or aliases: rejected because
  delegated values and unknown error construction do not prove a relationship.

## Consequences

CFG and SSA rules can remove impossible continuations through same-module
helpers without making unrelated typed rules pay for effect loading. Cold runs
perform additional module-local package loads; warm native-result caching and
the bounded source policy constrain that cost.

The third schema improves the four lifecycle consumers and lets `nilness`
diagnose direct dereferences and nil comparisons dominated by a proven error
branch. It does not prove relationships through unsupported aliases,
delegation, external-module helpers, or multi-module workspace effects. Those
gaps remain explicit rather than being filled with optimistic assumptions.

## Revisit Trigger

Advance the schema again only when a returned-state or additional ownership
effect has exact positive and close-negative fixtures, stable cross-load
identity, deterministic cache invalidation, bounded cold and warm cost, and
dogfood evidence. Revisit aliasing when a repeated real helper pattern can be
modeled without optimistic ownership claims. Revisit module selection when
workspace or replacement-module dogfood demonstrates a material same-repository
false negative.
