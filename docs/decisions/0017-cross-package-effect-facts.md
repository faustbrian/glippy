# ADR 0017: Cross-Package Semantic Effect Facts

- Status: accepted for v0.4 development; schema advanced by ADR 0021
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

The shared CFG builder, native `unreachable-code` control-flow walk, and SSA
no-return predicate consume the same immutable summary set. Effect source is
not added to the selected source set, is never a diagnostic or fix target, and
is not built into the root SSA program. Loading is demand-driven by enabled
rule metadata and remains serialized. Root and effect sources share the
configured package, file, and byte limits.

Native cache snapshots bind the effect requirement. Cache keys include a
digest identified as `native-effects-v3`, derived from the schema version,
canonically ordered stable no-return identities, ordered parameter-effect
records, and ordered returned-state records containing both result indexes and
both error-state relationships. A changed summary therefore invalidates native
diagnostics without making source pointers persistent.

ADR 0021 advances the active schema and cache component to
`native-effects-v4` for configured project contracts. The version-3 name in
this record remains the historical identity of the v0.4 decision.

The 2026-08-20 cleanup-managed result refinement advances the active schema and
cache component to `native-effects-v6`. Exact stable function and result
identities now record when selected-module source proves that a returned local
resource is registered through `testing.T.Cleanup` on every normal return.
Conditional, asynchronous, nested, reassigned, ambiguous, and non-testing
cleanup shapes remain absent from the fact set.

The 2026-08-20 receiver terminal-effect refinement advances the active schema
and cache component to `native-effects-v7`. Exact method identities now record
effects reached on the receiver across every normally returning path. A direct
receiver `Close` or a statically resolved receiver method with the same proven
effect may establish closure. Conditional or asynchronous closure, receiver
reassignment, aliases, address escape, recursion without a proof, and dynamic
dispatch remain conservative. A promoted method does not prove an effect for
the outer receiver value. The selected source boundary now includes root modules
plus reachable active-workspace modules and reachable local filesystem
replacements. Downloaded dependencies and workspace modules outside the root
import graph remain excluded.

The 2026-08-20 unconditional result-state refinement advances the active schema
and cache component to `native-effects-v8`. Exact function and result indexes
record nilness only when every explicit normal return proves the same state.
Bare, delegated, recursive, dynamic, typed-nil, unknown, or conflicting returns
remain absent. Package variants must agree on the exact state or the fact is
discarded. This lets an exact static caller classify a delegated error result
without retaining dependency syntax or process-local type identity.

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

CFG and SSA rules can remove impossible continuations through selected
local-source helpers without making unrelated typed rules pay for effect
loading. The native `unreachable-code` rule reports the first statement after
those proven terminal calls while preserving branch, label, nested-function,
type-error, and suggestion-fix behavior. Cold runs perform additional
module-local package loads; warm native-result caching and the bounded source
policy constrain that cost.

The third schema improves the four lifecycle consumers and lets `nilness`
diagnose direct dereferences and nil comparisons dominated by a proven error
branch. It does not prove relationships through unsupported aliases,
downloaded dependency bodies, or workspace modules outside the selected import
graph. Those gaps remain explicit rather than being filled with optimistic
assumptions.

## Revisit Trigger

Advance the schema again only when a returned-state or additional ownership
effect has exact positive and close-negative fixtures, stable cross-load
identity, deterministic cache invalidation, bounded cold and warm cost, and
dogfood evidence. Revisit aliasing when a repeated real helper pattern can be
modeled without optimistic ownership claims. The module-selection trigger was
satisfied by the 2026-08-20 receiver-effect dogfood record. Revisit downloaded
dependency inference only with a bounded provenance and source-trust policy
that does not turn arbitrary dependency bodies into analysis inputs.
