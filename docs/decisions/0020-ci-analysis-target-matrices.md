# ADR 0020: CI Analysis Target Matrices

- Status: accepted for v0.5 development
- Date: 2026-08-16

## Context And Evidence

Go build constraints can select different files and declarations by operating
system, architecture, build tags, and cgo policy. A single package load sees
only one such selection. Projects previously had to invoke Glippy repeatedly
and combine results themselves, which lost deterministic deduplication,
consistent configuration, target attribution, and one baseline contract.

Clippy exposes broad target and feature checks through Cargo. Go has no exact
feature equivalent, and blindly enumerating every supported platform would be
expensive and often impossible where cgo toolchains are unavailable. The
project therefore needs an explicit bounded CI selection rather than ambient
environment expansion.

## Decision

Schema version 1 accepts up to 32 `[[analysis.targets]]` entries. Each entry has
exact GOOS and GOARCH values, optional sorted build tags, and explicit cgo
state. Entries are canonicalized by a stable textual identity and duplicate
identities fail configuration loading.

Package-aware lint, combined check, and baseline generation execute targets in
canonical order. Results from equal source versions are merged by diagnostic
or prerequisite identity. Equal results carry the sorted union of target IDs;
target-specific results remain independent. All diagnostic reporters expose
target attribution. Persistent cache keys already bind the individual build
selection, so each target retains an isolated result namespace while sharing
one invocation-owned store and statistics collector.

Syntax-only lint remains physical-file oriented. The LSP uses the base
`[analysis]` selection because multiplying interactive work by a CI matrix
would make latency and diagnostics surprising. Fix and preview modes reject a
matrix: applying a target-dependent edit would require a separate policy for
conflicting diagnostics, validation targets, and transaction ownership.

## Alternatives

- Analyze every known GOOS and GOARCH automatically: rejected because most
  combinations are irrelevant and cgo or platform dependencies may be absent.
- Require one external invocation per target: rejected because consumers would
  have to reproduce result identity, ordering, deduplication, and baseline
  behavior.
- Let ambient GOOS, GOARCH, tags, or cgo expand the matrix: rejected because
  environment-dependent output violates deterministic configuration.
- Apply fixes independently for every target: deferred because multiple target
  analyses can propose incompatible edits to one physical source version.
- Run the matrix in the LSP: rejected for the initial contract because CI
  completeness is different from interactive latency and workspace coherence.

## Consequences

CI can detect build-tagged and platform-specific defects in one deterministic
invocation. Runtime and memory cost grow with the configured matrix, bounded by
32 entries. Identical findings remain concise, while target-only failures show
their exact selection. Users remain responsible for configuring targets whose
Go and cgo prerequisites exist on the analysis host.

The base analysis selection and matrix coexist. This is deliberate: the base
owns LSP and fix behavior, while the matrix owns non-mutating package checks.

## Revisit Trigger

Revisit fix exclusion only after a transaction design can prove compatible
edits and validate the final source across every selected target. Revisit LSP
matrix exclusion only if measured editor workflows require platform-aware
diagnostics with bounded latency. Revisit the 32-target limit from corpus and
resource evidence rather than configuration breadth alone.
