# Clippy Lint-Level Audit, 2026-08-14

## Current Reference

The reference is rust-lang/rust-clippy commit
`e52501913b75235e3d41422566a2d05d6f00b699`, fetched from upstream on
2026-08-14. The inspected authoritative surfaces were:

- `src/main.rs`, whose command help exposes `-A/--allow`, `-W/--warn`,
  `-D/--deny`, and `-F/--forbid`;
- `book/src/configuration.md`, which documents exact-lint and lint-group
  targets plus ordered command-line overrides;
- `book/src/usage.md`, which documents concatenated short forms such as
  `-Aclippy::style` and the `warnings` target; and
- `clippy_lints/src/cargo/lint_groups_priority.rs`, which distinguishes the
  warning umbrella from named lint and group configuration.

This record captures the evidence used for Glippy's command-line policy. It is
not a claim that Rust compiler lint resolution and Glippy configuration are
identical.

## Adopted Contract

Glippy adopts the four familiar levels, exact-rule and group targeting,
left-to-right command-line precedence, a warning umbrella, irreversible
`forbid`, and both separated and concatenated short forms. This supports
evaluation and CI strengthening without editing repository configuration.

The policy is applied once by Glippy's registry and shared by syntax, types,
CFG, SSA, baselines, fixes, combined check, and every reporter. Go-native rule
IDs are used directly instead of Rust's `clippy::` namespace.

## Deliberate Differences

- Glippy's persistent TOML preset list is order-independent; ordered
  command-line directives provide the temporary override surface.
- `restriction` remains exact-rule-only and `migration` remains unavailable
  without an explicit target, even though Clippy can address its complete
  restriction group.
- `--except` is an absolute Glippy selection filter and cannot be reversed by
  a later lint-level directive.
- Configured warning escalation runs after command-line directives to preserve
  the existing repository CI contract.
- `--cap-lints` and `--force-warn` are not admitted. Glippy analyzes selected
  project code rather than dependency crates, and no demonstrated Go journey
  currently requires those Rust compiler integration controls.

## Revisit Triggers

Revisit when current Clippy changes its level or group behavior, Glippy admits
dependency diagnostics, a consumer needs a force-warn equivalent, or dogfood
shows that configuration escalation after command-line policy is surprising.
