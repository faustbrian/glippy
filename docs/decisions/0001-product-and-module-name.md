# ADR 0001: Product And Module Name

- Status: accepted for development; final public-release audit required
- Date: 2026-08-09
- Refreshed: 2026-08-12

## Context And Evidence

The working name `gox` collides with an established Go cross-compiler and at
least two active Go tools, including a linter with overlapping `check` and
`explain` commands and a tool with overlapping `fmt` and `version` commands.
The complete evidence and candidate screen are recorded in
[`../research/naming-audit.md`](../research/naming-audit.md).

## Decision

The maintainer has selected **Gox**, binary `gox`, repository
`github.com/faustbrian/gox`, and module path `github.com/faustbrian/gox` for
continued development. The earlier **Gofettle** recommendation is rejected for
the current development identity; no repository or namespace rename is
authorized.

This is not final public-release clearance. Before the first public tag or
installation contract, the project must refresh the ecosystem-collision audit,
obtain appropriate trademark advice, and either retain Gox with an explicit
acceptance of the documented collisions or select a replacement. Until that
gate passes, the current module path must not be presented as a stable public
import contract.

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

Internal implementation packages remain unexported. Documentation may use Gox
as the current product identity but must not claim final trademark clearance or
a stable public module contract. No rename work is pending during ordinary
development; the final release audit remains mandatory.

## Revisit Trigger

Immediately before the first public tag, installation instructions, external
integration, or exported package contract, and whenever a material new `gox`
collision appears.
