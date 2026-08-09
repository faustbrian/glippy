# ADR 0006: Lint, Fix, And Suppression Boundaries

- Status: accepted for prototype
- Date: 2026-08-09

## Context And Evidence

Oxlint validates node-interest dispatch, correctness-focused defaults, and
explicit fix classes, but its ordinary semantic cost, diagnostic ordering, and
silent overlap skipping do not meet Gox's contract. `go/analysis` provides
useful interoperability but lacks native Gox tiers and safety metadata.

## Decision

Native rule metadata controls scheduling, documentation, presets, generated
files, and fix safety. Suppressions name exact rules and bind to physical token
ownership. The fix coordinator source-versions every edit, rejects all
conflicts explicitly, reparses, formats, validates, and performs one atomic
single-file replacement.

## Alternatives Rejected

- Make `analysis.Analyzer` the native rule API: insufficient tier, product,
  safety, and configuration metadata.
- First-fix-wins or silently skip overlaps: nondeterministic and hides unsafe
  partial application.
- Unscoped disable-all directives: unauditable and too easy to preserve against
  the wrong target.
- Multi-file fixes initially: no credible recovery transaction yet.

## Consequences

Some useful ecosystem fixes remain suggestion-only or unavailable until
audited. Conflicts produce an actionable outcome instead of partial success.
Fixing necessarily depends on the formatter being stable first.

## Revisit Trigger

Stable single-file fixing is proven across corpus and failures; an analyzer
requires a documented compatibility extension; or a real multi-file journey
justifies transaction design.
