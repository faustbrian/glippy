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

Effect schema version 1 contains only a proven no-return bit. A dependency
summary may consume summaries from deeper same-module dependencies. Dynamic
calls, interface dispatch, recursion without a terminal proof, ill-typed
packages, third-party modules, and workspace modules outside the selected
module paths remain conservatively returning.

The shared CFG builder and SSA no-return predicate consume the same immutable
summary set. Effect source is not added to the selected source set, is never a
diagnostic or fix target, and is not built into the root SSA program. Loading
is demand-driven by enabled rule metadata and remains serialized. Root and
effect sources share the configured package, file, and byte limits.

Native cache snapshots bind the effect requirement. Cache keys include a
digest identified as `native-effects-v1`, derived from the schema version and
canonically ordered stable no-return identities. A changed summary therefore
invalidates native diagnostics without making source pointers persistent.

## Alternatives

- Load the complete dependency syntax closure with `packages.NeedDeps`:
  rejected because the measured allocation and source-retention cost is
  disproportionate and includes standard-library bodies already covered by
  exact terminal contracts.
- Match helper names such as `Fatal`, `Must`, or `Exit`: rejected because names
  do not prove control-flow behavior.
- Reuse type-object pointers across independent loads: rejected because those
  identities are process- and load-specific.
- Export resource and return-state effects in schema version 1: deferred until
  their ownership and alias contracts have focused false-positive evidence.

## Consequences

CFG and SSA rules can remove impossible continuations through same-module
helpers without making unrelated typed rules pay for effect loading. Cold runs
perform additional module-local package loads; warm native-result caching and
the bounded source policy constrain that cost.

The first schema does not prove cleanup, transaction completion, cancellation
transfer, nil/error result relationships, external-module helpers, or
multi-module workspace effects. Those gaps remain explicit rather than being
filled with optimistic assumptions.

## Revisit Trigger

Advance the schema only when a lifecycle or returned-state effect has exact
positive and close-negative fixtures, stable cross-load identity, deterministic
cache invalidation, bounded cold and warm cost, and dogfood evidence. Revisit
module selection when workspace or replacement-module dogfood demonstrates a
material same-repository false negative.
