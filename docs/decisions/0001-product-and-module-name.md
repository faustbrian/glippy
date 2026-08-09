# ADR 0001: Product And Module Name

- Status: provisional decision
- Date: 2026-08-09

## Context And Evidence

The working name `gox` collides with an established Go cross-compiler and at
least two active Go tools, including a linter with overlapping `check` and
`explain` commands and a tool with overlapping `fmt` and `version` commands.
The complete preliminary evidence is recorded in
[`../research/naming-audit.md`](../research/naming-audit.md).

## Decision

`gox` remains an internal working name only. The current
`github.com/faustbrian/gox` module path follows the configured repository
remote but is not a stable public import contract. Public installation,
package publication, integrations, and releases are blocked until a replacement
product name, binary name, and owner-controlled module path pass a repeated
ecosystem audit and appropriate trademark review.

## Alternatives Rejected

- Retain `gox`: rejected because installed-binary and command collisions are
  already concrete, not hypothetical.
- Select a replacement during this audit: rejected because candidate creation,
  ownership confirmation, domain strategy, and trademark screening have not
  been supplied or completed.

## Consequences

Internal implementation packages must remain unexported. Documentation must
label the name as provisional. A later rename is expected and must happen
before downstream consumers exist.

## Revisit Trigger

Before the first public tag, installation instructions, external integration,
or exported package contract.
