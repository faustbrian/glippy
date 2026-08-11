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

Syntax node-interest rules use one direct preorder AST traversal and dispatch
matching nodes to enabled rules. The scheduler does not build an
`ast/inspector` index for this single union query. The recorded 1, 3, 5, 10,
and 25-rule benchmark found one naive walk with a lower median at one rule and
the direct pass with lower medians from three through 25 rules. Direct dispatch reduced allocation
from roughly 28-30 KiB for inspector indexing to 456-1,896 bytes per traversal.
The scheduler keeps the direct path at one rule because the measured median
difference was 1.81 microseconds and does not justify a second execution path.

## Alternatives Rejected

- Custom Go parser/type checker: no evidenced missing standard capability and
  an unacceptable compatibility burden.
- AST-only fidelity: loses token gaps, semicolon origin, BOM/newline details,
  and normalized raw bytes.
- Always load types/CFG/SSA: violates editor latency and rule-cost boundaries.
- Independent rule loaders: duplicate work and permit incompatible type
  identities.
- An inspector index for one union query: pays a full indexing traversal and
  roughly 28-30 KiB without a repeated filtered query to amortize it.

## Consequences

Lexing occurs twice through public APIs. Physical positions are distinct from
logical reported positions. Typed loading may invoke Go tooling and requires
explicit environment/cache inputs. Arbitrary analyzers cannot automatically
share immutable state.

Syntax dispatch visits uninterested nodes once to avoid indexing or repeated
walks; a secondary index requires new representative benchmark evidence.

## Revisit Trigger

A concrete supported syntax or fidelity requirement cannot be represented by
original bytes plus scanner, parser, and token mappings, or a measured tier
cannot meet its correctness contract.
