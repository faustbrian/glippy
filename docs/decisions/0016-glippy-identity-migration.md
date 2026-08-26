# ADR 0016: Glippy Identity Migration

- Status: accepted for the initial Glippy development line
- Date: 2026-08-13

## Context And Evidence

Gox v0.1.0 is already published from `github.com/faustbrian/gox` and its tags,
module path, binary, archives, checksums, attestations, and evidence are
immutable. The maintainer subsequently directed the next development line to
adopt the Glippy identity and accepted the collision risk recorded in the
refreshed [naming audit](../research/naming-audit.md).

The exact-name GitHub search found nine repositories. The material Go collision
is `F1bonacc1/glippy`, an active multi-platform clipboard project with published
Go module versions. The `glippy` GitHub account is also occupied. These checks
are technical ecosystem evidence, not trademark or legal clearance.

## Decision

Beginning with post-v0.1 development, the product is **Glippy**, the binary and
command package are `glippy` and `cmd/glippy`, and the intended module and
repository identity is `github.com/faustbrian/glippy`.

`.glippy.toml`, `.glippy-baseline.json`, `//glippy:`, `GLIPPY_CACHE_DIR`, Glippy
cache namespaces, completion names, release archives, manifest product values,
and version output are canonical. No new `gox` binary is shipped.

Through the first stable Glippy release:

- automatic discovery accepts `.gox.toml` only when `.glippy.toml` is absent;
- discovery fails if both names occur in the searched project scope;
- an explicit `--config` path bypasses automatic-name ambiguity; and
- `//gox:` suppressions retain their suppression behavior but always produce a
  deterministic `legacy-directive` finding directing migration to `//glippy:`.

Machine schemas remain at version 1 because command, rule, range, outcome, and
ordering structures do not change. Documented identity values change to
Glippy. Cache magic, namespaces, environment variables, and formatter identity
change deliberately so v0.1 cache entries cannot be mistaken for Glippy
entries.

## Alternatives

- Retain Gox: rejected by the maintainer for the next development line.
- Ship both binaries: rejected because it creates a second public command and
  prolongs ambiguous installation behavior.
- Silently prefer `.glippy.toml`: rejected because two policy files must not
  produce an implicit winner.
- Reject all v0.1 configuration and suppressions immediately: rejected because
  a bounded source-compatible migration window is inexpensive and auditable.

## Consequences

Consumers must update install paths, executable names, cache environment
variables, configuration names, baselines, suppressions, CI, and editor setup.
The GitHub repository itself is not renamed by this source change; remote
renaming is a separate maintainer operation. v0.1 references remain under the
Gox repository and module identity.

## Revisit Trigger

Remove `.gox.toml` and `//gox:` compatibility only in a documented major
release after the first stable Glippy release and measured usage showing no
remaining reliance on either legacy form. Refresh collision evidence before
any package-manager registration or stable-identity expansion.
