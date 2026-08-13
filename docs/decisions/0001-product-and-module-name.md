# ADR 0001: Product And Module Name

- Status: superseded by ADR 0016 for v0.2
- Date: 2026-08-09
- Refreshed: 2026-08-13

## Context And Evidence

The working name `gox` collides with an established Go cross-compiler and at
least two active Go tools, including a linter with overlapping `check` and
`explain` commands and a tool with overlapping `fmt` and `version` commands.
The complete evidence and candidate screen are recorded in
[`../research/naming-audit.md`](../research/naming-audit.md).

## Decision

For v0.1, the maintainer selected **Gox**, binary `gox`, repository
`github.com/faustbrian/gox`, and module path `github.com/faustbrian/gox`, and
explicitly accepted the final-candidate collision and trademark-risk evidence.
The earlier **Gofettle** recommendation was rejected. This product-risk
acceptance was not jurisdiction- or class-specific legal clearance.

## Alternatives Rejected

- Rename immediately: rejected by the maintainer while the product remains in
  development. The collision evidence remains relevant to the final
  public-release decision.
- `Goburnish`: technically clear in the checked namespaces, but longer and less
  direct as a frequently typed Go command.
- `Goquoin`, `Goarden`, `Gomeld`, and `Gosculpt`: technically clear in the
  checked package and repository namespaces, but rejected for spelling,
  pronunciation, ambiguity, or unrelated search noise.

## Consequences

The v0.1 tags, module path, binaries, release assets, and evidence remain
immutable historical Gox identities. ADR 0016 owns the v0.2 Glippy migration.

## Revisit Trigger

Before a later stable-identity expansion or package-manager registration, and
whenever a material new `gox` collision appears.
