# ADR 0005: Shared Frontend And Analysis Tiers

- Status: accepted
- Date: 2026-08-09

## Context And Evidence

The Go 1.26.5 and x/tools v0.48.0 audit shows the standard parser, types,
packages, analysis, CFG, and SSA layers satisfy language and semantic needs.
The missing capability is concrete source fidelity, which a scanner ledger over
immutable bytes supplies without a replacement frontend. Detailed evidence is
in [`../research/go-frontend-audit.md`](../research/go-frontend-audit.md).

## Decision

Gox uses separate scanner and parser passes over one immutable source version.
It stores physical byte offsets, token gaps, semicolon origin, raw comment and
directive bytes, and a source digest. Expensive analysis is selected through
lexical, syntax, types, CFG, and SSA tiers. One scheduler owns loading and
reuses representations within a run.

## Alternatives Rejected

- Custom Go parser/type checker: no evidenced missing standard capability and
  an unacceptable compatibility burden.
- AST-only fidelity: loses token gaps, semicolon origin, BOM/newline details,
  and normalized raw bytes.
- Always load types/CFG/SSA: violates editor latency and rule-cost boundaries.
- Independent rule loaders: duplicate work and permit incompatible type
  identities.

## Consequences

Lexing occurs twice through public APIs. Physical positions are distinct from
logical reported positions. Typed loading may invoke Go tooling and requires
explicit environment/cache inputs. Arbitrary analyzers cannot automatically
share immutable state.

## Revisit Trigger

A concrete supported syntax or fidelity requirement cannot be represented by
original bytes plus scanner, parser, and token mappings, or a measured tier
cannot meet its correctness contract.
