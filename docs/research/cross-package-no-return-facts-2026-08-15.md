# Cross-Package No-Return Facts, 2026-08-15

## Decision

Glippy exports and consumes versioned no-return summaries for direct named
functions and methods in the selected modules. The summaries extend the shared
package no-return analysis across package boundaries without retaining imported
source as a lint or fix target.

Only same-module dependencies are eligible. Standard-library packages and
unrelated third-party modules remain outside native source summarization;
authoritative standard-library terminal functions continue to use their exact
typed contracts. Rules request this layer explicitly through
`RequiresEffectFacts`, and only control-flow or SSA rules may do so.

## Scheduling And Identity

The loader discovers eligible imports in deterministic dependency layers,
disables tests for effect inputs, and analyzes the deepest layer first. A fact
uses `types.ObjectString` with import paths as its stable function identity, so
independent `go/packages` loads agree without sharing pointer identity.

The root and effect inputs share the configured package, source-file, and
source-byte limits. Cancellation is checked between layers and during no-return
construction. Package cycles and recursion remain conservative: an unresolved
cycle does not invent a terminal function.

Imported effect source is not added to `PackageResult.Sources`. Diagnostics,
suppressions, baselines, and fixes therefore remain restricted to the selected
root source set.

## Cache Contract

Native package cache keys include the schema-versioned digest
`native-effects-v1`. The digest sorts stable identities before hashing, so it
is deterministic and changes when any imported terminal summary changes. A
schema change that alters encoded meaning requires a new version and cache-key
namespace.

## Precision Findings

Focused cross-package fixtures first reproduced false continuation edges after
an imported panic wrapper. With the imported summary installed, both
`context-cancel-leak` and `nilness` use the same corrected control-flow fact.
Independent-load identity, digest ordering and sensitivity, module-boundary
selection, root-source ownership, cache invalidation, and reporter visibility
have dedicated regressions.

Self-dogfood also exposed four pre-existing `resource-not-closed` findings.
The precision review admitted three conservative ownership patterns:

- consecutive terminating acquisition guards followed by a conventional
  non-nil error guard;
- retry loops whose success branch returns the acquired resource; and
- constructor-rejection guards whose terminal true branch proves that normal
  continuation has a nil resource.

The exact `os/exec.Cmd` pipe methods are excluded from caller-owned closer
analysis because `Cmd.Start` closes their descriptors on start failure and
`Cmd.Wait` closes them after a successful start. Custom methods with the same
names remain covered through exact package and receiver identity.

## Cost And Dogfood

Five one-iteration samples of the imported no-return benchmark on Darwin arm64
with an Apple M4 Max ranged from 79.6 to 100.3 milliseconds, 401 to 963 KiB,
and 2,735 to 3,165 allocations. The probe includes dependency loading and is a
comparison baseline, not a stable release budget.

At Glippy revision `7c6f137`, a working-tree build ran the eight rules that
consume effect facts over Glippy and `go-libraries/pkg/prompts` at revision
`a3ea0cb39145b4a973cecca86b6ed76fb0cb37a7`. Both non-mutating runs completed
without findings or tool failure:

- `context-cancel-leak`;
- `defer-in-infinite-loop`;
- `http-response-body-not-closed`;
- `nilness`;
- `resource-not-closed`;
- `sql-transaction-not-completed`;
- `unchecked-rows-error`; and
- `unchecked-scanner-error`.

This evidence establishes the no-return fact kind only. Resource, transaction,
cancellation, and returned-state fact kinds remain separate future schema work.

## Revisit Triggers

Broaden native facts only when a consumer has a precise observable contract and
nearby negative fixtures. Revisit third-party facts only with an interoperable
export format and trust policy. Revisit strongly connected components when a
real recursive helper graph demonstrates a false positive or false negative
that the conservative cycle policy cannot accept.
