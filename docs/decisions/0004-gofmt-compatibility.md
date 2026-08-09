# ADR 0004: Gofmt Fixed-Point Compatibility

- Status: provisional target
- Date: 2026-08-09

## Context And Evidence

Local Go 1.26.5 experiments show the motivating expanded `if`, split ordinary
statements, trailing-operator boolean chain, one-argument-per-line call,
expanded function literal, selector chain, and type union can all be exact
gofmt fixed points. Gofmt itself leaves ordinary same-line semicolon-separated
statements compressed, so Gox still adds the intended value.

Strict compatibility remains unproven. Gofmt aligns declarations and comments,
sorts imports, normalizes some numeric literal prefixes, removes tested
redundant parentheses, and can move comments across comma boundaries. Those
behaviors interact with Gox source-fidelity and ownership contracts.

## Decision

Phase 1 targets:

```text
gofmt(goxfmt(input)) == goxfmt(input)
```

for the recorded compatible construct classes and pinned Go toolchain. This is
not yet a product-wide invariant or public claim. Gox's own parse, normalized
equivalence, comment/directive accounting, and idempotency checks remain
mandatory regardless of gofmt compatibility.

Every difference discovered by the corpus must be classified as intentional
Gox layout, accepted toolchain difference, unsupported syntax/version,
source-fidelity defect, semantic-risk defect, or unresolved investigation.
The final decision must state exact supported Go versions and divergence
classes.

## Alternatives Rejected

- Promise strict compatibility now: evidence is too narrow.
- Reject compatibility now: the motivating layouts already demonstrate useful
  compatibility.
- Run gofmt blindly as a post-pass: it would obscure engine ownership and can
  violate literal, import, parentheses, or comment policies.

## Consequences

Golden and corpus tests record the Go toolchain version. Gox must model any
required alignment rather than delegating semantics to gofmt. Repositories
that enforce gofmt will not receive a migration promise until this ADR becomes
accepted or documents precise divergences.

## Revisit Trigger

After Phase 1 corpus, comment/directive, import, literal, parentheses, and
alignment evidence is complete, and whenever supported Go toolchains change.
