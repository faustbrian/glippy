# ADR 0004: Gofmt Fixed-Point Compatibility

- Status: accepted for prototype; product-wide fixed point rejected
- Date: 2026-08-09

## Context And Evidence

Local Go 1.26.5 experiments show the motivating expanded `if`, split ordinary
statements, trailing-operator boolean chain, one-argument-per-line call,
expanded function literal, selector chain, and type union can all be exact
gofmt fixed points. Gofmt itself leaves ordinary same-line semicolon-separated
statements compressed, so Gox still adds the intended value.

The project-owned Phase 1 corpus now records the incompatible classes directly.
For the same valid formatted file, Go 1.26.5 gofmt sorts a retained import
group, normalizes hexadecimal prefixes, removes redundant parentheses, and
adds tabular alignment to declarations and struct fields. Gox preserves import
order, literal spelling, and parentheses and emits structural indentation
without alignment. The corpus also records the deliberate width-aware
`if`-header layout that gofmt flattens.

## Decision

Gox does not promise a product-wide fixed point under gofmt. For each construct
class recorded as compatible, Phase 1 still requires:

```text
gofmt(goxfmt(input)) == goxfmt(input)
```

under the pinned Go toolchain. Each incompatible corpus fixture MUST name its
divergence and pin the exact gofmt output. Gox's own parse, normalized
equivalence, comment/directive accounting, and idempotency checks remain
mandatory regardless of gofmt compatibility.

The initial divergence classes are:

1. intentional width-aware layout that gofmt flattens;
2. retained import spec order that gofmt sorts;
3. preserved numeric literal spelling that gofmt normalizes;
4. preserved redundant parentheses that gofmt removes; and
5. structural indentation without gofmt-style tabular alignment; and
6. preserved explicit versus implicit empty-statement spelling that gofmt
   normalizes.

Every difference discovered by the corpus must be classified as intentional
Gox layout, accepted toolchain difference, unsupported syntax/version,
source-fidelity defect, semantic-risk defect, or unresolved investigation.
Supported-version expansion MUST revalidate every compatible and divergent
fixture before release.

## Alternatives Rejected

- Promise strict compatibility now: evidence is too narrow.
- Reject compatibility now: the motivating layouts already demonstrate useful
  compatibility.
- Run gofmt blindly as a post-pass: it would obscure engine ownership and can
  violate literal, import, parentheses, or comment policies.

## Consequences

Golden and corpus tests record the Go toolchain version. Repositories that
enforce gofmt cannot run it after Gox over the documented divergent classes
without churn. They must keep their existing formatter until migration or make
Gox the sole formatting authority and remove the conflicting gofmt check. Gox
does not yet provide a gofmt-compatibility mode.

## Revisit Trigger

Whenever supported Go toolchains change, a requested compatibility mode gains
a concrete adoption case, or corpus evidence reveals a new divergence class.
