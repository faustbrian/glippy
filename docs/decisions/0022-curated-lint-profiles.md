# ADR 0022: Curated Lint Profiles

- Status: accepted for v0.5 development
- Date: 2026-08-16

## Context And Evidence

Glippy already provides coherent rule groups, exact severity overrides,
Clippy-style lint levels, path overrides, suppressions, baselines, selective
execution, and configuration introspection. Projects nevertheless have to
assemble those primitives manually before they can choose a policy stronger
than the correctness default.

PHPStan's numbered levels make progressive adoption obvious, while Clippy's
groups preserve the nature and expected signal of each rule. Copying either
surface directly would lose useful information: a single number hides policy
composition, and raw groups do not distinguish the low-noise suspicious rules
that are appropriate for broad recommendation.

## Decision

Glippy provides four named profiles in increasing strictness:

- `default` selects the complete correctness group;
- `recommended` adds a fixed, reviewed low-noise suspicious set;
- `strict` selects the complete correctness, suspicious, performance,
  complexity, and style groups; and
- `pedantic` adds the complete pedantic group to strict policy.

Restriction rules remain exact-ID-only. Migration remains unavailable without
an explicit target. A profile cannot be combined with either preset field.
Explicit presets remain available for projects that want composition without a
profile, and an explicitly empty preset list selects only exact rules.

Profile rules are retained separately from user overrides. Resolution applies
the profile first, then exact user rules, ordered path overrides, command-line
filters and levels, and warning escalation. `config show` reports the profile
and attributes each selected rule to the effective profile or later override.
Configuration and cache identity include both the profile name and its resolved
exact policy.

`glippy init` writes the default profile unless
`--profile=default|recommended|strict|pedantic` selects another. Shell
completion exposes the same finite set.

## Alternatives

- Numeric PHPStan-style levels were rejected because they obscure rule kind,
  cost, and expected false-positive policy.
- Treating profiles as aliases for preset arrays was rejected because
  `recommended` intentionally selects only part of `suspicious` and because
  introspection must distinguish profile policy from manual composition.
- Merging profile rules into `lint.rules` was rejected because it would report
  built-in policy as a user-authored override and make precedence ambiguous.
- Enabling restriction or migration wholesale was rejected because both require
  project-specific intent.

## Consequences

The default diagnostic set remains unchanged: the former implicit correctness
preset is now identified as the default profile. Stronger profiles can increase
analysis tiers, runtime, and findings. Their composition is therefore a
release-visible compatibility surface subject to dogfood, cost evidence, and
migration notes.

Projects can always override or disable an exact rule after selecting a
profile. Custom preset arrays remain supported and report `profile: none`.

## Revisit Trigger

Revisit the four-profile set when adoption shows that one transition is too
large, when rule cost makes complete strict groups impractical, or when a
versioned migration target needs its own curated profile. Do not add another
profile solely to rename an existing composition.
