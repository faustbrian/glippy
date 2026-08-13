# Compatibility And Change Policy

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as
shown here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

This policy applies beginning with Glippy's first public release. Before that
release, branch heads, commits, locally built binaries, rehearsal artifacts,
and documented development behavior are not compatibility promises. The first
public release MUST identify the contracts it establishes and MUST NOT claim
stability for a surface that does not pass its final acceptance gate.

## Versions And Release Notes

Published versions MUST use [Semantic Versioning](https://semver.org/). A
prerelease suffix identifies evaluation software and does not carry the
stable-release support promise. The latest stable release and supported target
policy are defined in the [product support policy](support-policy.md).

Every release MUST publish release notes that identify, when applicable:

- formatter-output changes and their migration impact;
- added, removed, renamed, deprecated, or default-enabled lint rules;
- changes to fix availability or safety;
- configuration additions, removals, defaults, and migrations;
- machine-schema and exit-code changes;
- supported Go language, operating-system, and architecture changes; and
- security or correctness fixes whose adoption affects users.

Release notes MUST distinguish deliberate policy changes from corrections that
restore an already documented contract. They MUST NOT describe a breaking
change as an ordinary internal refactor.

## Formatter Output

Canonical formatting is a user-visible compatibility surface. Every release
that changes output for valid supported source MUST:

1. identify each affected syntax construct;
2. publish representative before-and-after source;
3. state whether the change fixes a documented defect or changes policy;
4. update formatter rules, golden fixtures, and corpus fingerprints together;
5. pass parse, equivalence, comment/directive, idempotency, and width gates; and
6. provide a non-mutating `glippy fmt --check` or `glippy fmt --diff` migration path.

A formatter defect correction MAY ship in a patch release when it restores the
documented canonical layout or a safety invariant. A deliberate change to the
documented layout policy MUST NOT ship in a patch release. Before version 1.0 it
MAY ship in a minor release with the evidence above. At or after version 1.0 it
requires a major release unless it is confined to an explicitly named preview
surface.

One release SHOULD NOT combine unrelated formatter policy changes. Glippy MUST
always preserve its own byte-idempotency even when an intentional release
changes canonical output.

## Lint Rules And Presets

Rule IDs become stable with the first public release. A stable rule ID MUST NOT
be silently reused for a different problem. A rename or replacement MUST carry
deprecation metadata, migration guidance, and either a documented alias or an
actionable migration diagnostic before the old ID is removed.

Removing a rule ID or changing it to report a materially different defect MUST
NOT occur in a patch release. At or after version 1.0, removing a stable rule ID
requires a major release. Before version 1.0, removal MAY occur in a minor
release only after a documented deprecation period.

Adding an opt-in rule is backward-compatible. Adding a rule to the default
preset, increasing a default severity, or materially widening default findings
MUST be announced as an adoption-affecting change and MUST NOT ship in a patch
release. Such a change requires current dogfood noise evidence, false-positive
analysis, and explicit release-note migration guidance.

A patch release MAY reduce false positives, correct source ranges, or restore a
rule's documented behavior. It MUST NOT silently broaden default findings while
claiming only a false-positive correction.

A material change to an opt-in rule's documented reporting boundary that is not
such a correction MUST NOT ship in a patch release. Before version 1.0 it MAY
ship in a minor release with updated examples and migration guidance. At or
after version 1.0, changing the problem the rule represents requires a major
release; a backward-compatible precision improvement MAY ship in a minor
release.

Preset membership changes MUST name both the old and new membership. The
`correctness` default MUST remain focused on incorrect, unsafe, ineffective, or
highly suspicious behavior; style, performance, complexity, and migration
policy MUST remain opt-in unless their admission and compatibility evidence
explicitly justifies a default change.

## Fixes And Safety

Fix safety is independent of diagnostic severity. A fix classified as `safe`
MUST continue to satisfy its documented semantics-preserving contract.

If new evidence weakens that proof, Glippy MUST remove the automatic fix or
downgrade it to `suggestion` or `unsafe` immediately; protecting source takes
priority over preserving fix availability. A safety downgrade MAY ship in a
patch release and MUST be called out in release notes. Promoting a fix to
`safe` requires fresh behavioral, conflict, reparse, formatting, validation,
and idempotency evidence.

Changing a safe fix so it performs a materially different transformation MUST
NOT occur silently under the existing fix name. The release MUST either retain
the old contract, introduce a new named fix, or treat the change as an
adoption-affecting compatibility change.

## Configuration

Configuration files carry an explicit schema version. Unknown keys and invalid
values MUST continue to fail rather than being ignored.

Adding an optional field with a backward-compatible default MAY ship in a minor
release. Correcting acceptance of input that was already documented as invalid
MAY ship in a patch release. Removing or renaming a field, changing its type, or
changing a default in a way that alters results MUST provide an actionable
migration and MUST NOT ship in a patch release.

At or after version 1.0, a breaking configuration change requires a major
release or a new configuration schema version with an explicit migration path.
Glippy MUST reject unsupported schema versions with a clear diagnostic; it MUST
NOT guess how to reinterpret them.

Formatter options SHOULD remain limited to adoption-significant choices.
Compatibility pressure MUST NOT introduce switches for individual whitespace
rules or create multiple undocumented formatting dialects.

## Machine Output And Exit Codes

Machine output carries an explicit schema version. Rule identifiers, severity
values, fix safety values, outcome categories, range semantics, completeness,
and exit categories are part of that versioned contract.

Adding an optional field that old consumers can ignore MAY ship without a
schema-version increment when the schema explicitly permits unknown fields.
Removing or renaming a field, changing its type or meaning, changing a required
field to be conditionally absent, or reinterpreting a range requires a new
schema version.

Glippy MAY emit more than one schema version during a documented migration window.
It MUST NOT emit a new incompatible shape while labeling it with an older
schema version. Removal of a supported machine schema or reassignment of a
stable exit category is breaking and, at or after version 1.0, requires a major
release.

Human wording MAY improve without a version change, but scripts MUST use the
machine reporter and stable exit categories rather than parse human prose.
The public [machine output reference](machine-output.md) defines the current
schema-version-1 fields, values, range semantics, ordering, and completeness
contract.

## CLI And Platform Contracts

Adding a command or optional flag is backward-compatible when existing
invocations retain their meaning. Removing or renaming a command or flag,
changing an option's meaning, changing stdin/stdout ownership, or making a
previously non-writing invocation write is breaking.

Before version 1.0, a breaking CLI change MAY ship in a minor release with an
explicit migration. At or after version 1.0, it requires a major release unless
the affected surface was explicitly documented as preview-only.

Glippy MUST NOT broaden write or fix platform guarantees from successful parsing,
cross-compilation, or an unrecorded filesystem. Adding a supported target
requires runtime evidence proportional to its read, write, fix, cache, and
release claims. Removing a supported operating system, architecture, source
language version, or filesystem guarantee MUST be announced as a compatibility
change and MUST follow the supported-version policy. At or after version 1.0,
such a removal requires a major release unless continued support would violate
a security or source-safety boundary documented in the release notes.

## Deprecation And Migration

A deprecation MUST identify the affected stable name, its replacement or
retirement reason, the release that introduced the deprecation, and actionable
migration guidance. Deprecated behavior MUST remain documented while it is
supported.

At or after version 1.0, a public command, flag, configuration field, rule ID,
or machine schema SHOULD remain deprecated for at least one minor release
before removal. A security or source-corruption risk MAY require faster
removal; the release notes MUST state the risk and the safest available
migration.

Migration tooling MUST be explicit and non-mutating by default when inspection
is possible. Glippy MUST NOT silently rewrite configuration, suppressions, or
semantic source merely to make an upgrade succeed.

## Internal Formats

Cache entries, cache keys, fact snapshots, corpus fingerprints, and internal Go
packages are not public APIs. A release MAY invalidate or replace them without
a product major version when stale state degrades to safe recomputation and no
documented public result changes.

Internal-format changes MUST still preserve corruption refusal, deterministic
identity, source safety, and cache disposability. A cache migration MUST NOT be
the only source of diagnostics or facts.

## Release Gate

Before publishing a release, the project MUST audit the candidate against this
policy and record every formatter, rule, fix, configuration, schema, CLI,
platform, and support change since the previous public release. The audit MUST
bind its claims to the release candidate and current corpus evidence.

A public tag or release MUST NOT be created until every pre-publication final
acceptance gate passes and the maintainer personally verifies, reviews, and
authorizes the exact candidate. Publication and provenance are themselves the
final acceptance transaction; their successful verification advances a
candidate from the 95% pre-tag state to 100%.

The normative architecture decision is
[ADR 0008](decisions/0008-version-cache-and-compatibility-policy.md). The
[formatter migration guide](migration-from-go-formatters.md) covers adoption
from existing Go formatters, and the [command reference](command-reference.md)
defines the current implemented CLI.
