# ADR 0001: Product And Module Name

- Status: provisional; replacement recommendation awaiting approval
- Date: 2026-08-09
- Refreshed: 2026-08-11

## Context And Evidence

The working name `gox` collides with an established Go cross-compiler and at
least two active Go tools, including a linter with overlapping `check` and
`explain` commands and a tool with overlapping `fmt` and `version` commands.
The complete evidence and candidate screen are recorded in
[`../research/naming-audit.md`](../research/naming-audit.md).

## Decision

`gox` remains an internal working name only. The current
`github.com/faustbrian/gox` module path follows the configured repository
remote but is not a stable public import contract. Public installation,
package publication, integrations, and releases are blocked until a replacement
product name, binary name, and owner-controlled module path pass a repeated
ecosystem audit and appropriate trademark review.

The technical audit recommends **Gofettle**, binary `gofettle`, and module path
`github.com/faustbrian/gofettle`. This recommendation is not adoption authority:
the maintainer must approve the product name, obtain appropriate trademark
advice, and authorize any external repository or namespace changes. Until then,
the repository continues to use the private `gox` working identity.

## Alternatives Rejected

- Retain `gox`: rejected because installed-binary and command collisions are
  already concrete, not hypothetical.
- `Goburnish`: technically clear in the checked namespaces, but longer and less
  direct as a frequently typed Go command.
- `Goquoin`, `Goarden`, `Gomeld`, and `Gosculpt`: technically clear in the
  checked package and repository namespaces, but rejected for spelling,
  pronunciation, ambiguity, or unrelated search noise.

## Consequences

Internal implementation packages must remain unexported. Documentation must
label the name as provisional. The local rename and external namespace
reservation remain blocked on maintainer approval and legal review; they must
happen before downstream consumers exist.

## Revisit Trigger

Before the first public tag, installation instructions, external integration,
or exported package contract.
