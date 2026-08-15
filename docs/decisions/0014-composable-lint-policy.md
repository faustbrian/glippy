# 0014: Composable Lint Policy

- Status: accepted
- Date: 2026-08-13

## Context

Glippy v0.1 selected exactly one lint preset. That prevented a project from using
the correctness default together with suspicious, performance, complexity, or
style policy without repeating every additional rule as an override. It also
left no concise continuous-integration equivalent to Clippy's warning denial.

Current Clippy groups establish useful semantic expectations: correctness aims
for effectively no false positives, suspicious identifies likely mistakes,
performance and complexity identify credible costs or needless structure,
style owns idiomatic code, pedantic is intentionally opinionated, and
restriction is chosen rule by rule rather than enabled wholesale. PHPStan's
strict identifiers and configuration failures reinforce Glippy's existing exact
rule IDs and typed schema; opaque numeric levels do not communicate policy well
enough for Glippy.

The implementation refresh is recorded in
[the current Clippy lint-level audit](../research/clippy-lint-levels-2026-08-14.md).

## Decision

Schema version 1 accepts an order-independent `lint.presets` list. Glippy unions
rules belonging to any selected group, applies explicit `lint.rules`
overrides, and then optionally promotes every remaining warning to an error
with `lint.warnings-as-errors`. Both decisions enter deterministic
configuration identity. The legacy singular `lint.preset` selects one group
and remains accepted, but singular and plural forms are mutually exclusive.

Selectable groups are `correctness`, `suspicious`, `performance`,
`complexity`, `style`, and `pedantic`. Restriction metadata exists so rules can
be classified consistently, but the group cannot be enabled wholesale;
projects opt into exact restriction rule IDs. Migration metadata remains
valid, while selecting the migration group is rejected until a target version
or API contract exists.

Formatting remains outside lint policy. No preset may add line length,
spacing, brace placement, wrapping, or another layout diagnostic owned by
`glippy fmt`.

The `lint` and `check` commands additionally accept ordered `allow`, `warn`,
`deny`, and `forbid` directives. Targets may be exact rule IDs, selectable
groups, or the special currently-warning set. These directives apply after
configuration and exact `--only` eligibility, while `--except` remains an
absolute exclusion and configured warning escalation remains last. `forbid`
locks every matched rule against a later `allow` or `warn`; such an attempted
lowering is an invalid invocation. The `warnings` target never enables a rule
that is off. Restriction and migration retain their existing exact-ID and
target-gated boundaries.

Schema version 1 also accepts ordered `[[lint.overrides]]` entries containing
project-relative glob paths and exact rule severities. Path sets within one
entry are order-independent; entries apply in declaration order and later
matching entries replace earlier severities for the same rule. Only severity
is path-scoped: rule options, presets, suppressions, baselines, build inputs,
formatting, and cache lifecycle remain project-wide. The scheduler executes
the union of rules that any path can enable, then filters and reseverities each
physical file before suppression, baseline, reporting, or fix selection.
Configuration identity retains entry order and normalized patterns.

## Alternatives

- Keep one preset and require per-rule overrides: rejected because it repeats
  group membership in every strict repository and makes new group rules
  invisible to existing policy.
- Make `pedantic` an opaque cumulative superset: rejected because users cannot
  tell which independent cost and policy groups they selected.
- Add PHPStan-style numeric levels: rejected because levels obscure policy and
  make additions within a number harder to review.
- Permit the complete restriction group: rejected in line with Clippy's own
  guidance because many restriction rules are intentionally contradictory or
  organization-specific.
- Discover and merge nested configuration files: rejected because ordered
  typed path policy handles tests, fixtures, and subtrees without creating a
  filesystem inheritance and cycle contract.
- Path-scope rule options and presets: rejected because changing analyzer
  behavior within one typed package would either duplicate expensive package
  analysis or make package-wide rule contracts ambiguous.

## Consequences

Projects can adopt a Clippy-like union of groups and use one deterministic CI
severity switch while retaining exact exceptions. Adding a rule to a selected
non-default group can create a new diagnostic and follows the published lint
compatibility process. The default remains correctness only. Cache identity
changes when group membership or warning escalation changes.

One invocation can evaluate or temporarily strengthen policy without editing
project configuration. Because directives are ordered, scripts must preserve
argument order. The effective command-line policy is invocation state rather
than persistent configuration. It does not alter the project configuration
digest, but its resolved rule IDs, severities, and options remain cache-key
inputs and determine the analysis result consumed by every reporter and fix
path.

Repositories can express targeted exceptions and stricter subtrees in one
auditable file. Package-aware rules may execute for files where their result is
later filtered when another path in the same run enables them; this bounded
extra work preserves one shared package load and deterministic results.

## Revisit Trigger

Revisit when migration rules have an explicit target contract, a stable preset
needs removal or renaming, dogfood shows that group union creates ambiguous
severity or cost behavior, or a proven use case requires path-scoped options
or nested configuration despite the package-analysis cost.
