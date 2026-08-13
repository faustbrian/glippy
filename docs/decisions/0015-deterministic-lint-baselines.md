# 0015: Deterministic Lint Baselines

- Status: accepted
- Date: 2026-08-13

## Context

Composable Clippy-style policy makes strict analysis easy to enable, but an
established repository may have existing findings that cannot be resolved in
one change. Disabling rules or adding source suppressions would also weaken
new-code enforcement. PHPStan baselines demonstrate a useful adoption surface,
but Gox must preserve exact source identity, deterministic operation, safe fix
selection, and source privacy.

## Decision

Gox uses a strict JSON baseline with its own schema version. An entry binds an
exact rule ID, stable message key, portable project-relative path, SHA-256 of
the exact diagnostic source span, and occurrence count. Optional reason and
expiry fields are metadata. Absolute offsets and source snippets are excluded.

Baseline application follows source suppression and precedes reporting or fix
selection. A match hides only the counted diagnostic instances. Changed spans
remain visible. Stale and expired entries are findings for analyzed files;
entries outside the current selection remain unexamined. Expiry depends on an
explicit configured cutoff rather than wall-clock time.

`gox lint --generate-baseline=<path>` generates from visible diagnostics for
one project root and configuration. It cannot run with fixes or JSON output.
Creation and update use the shared rooted, stale-aware, same-directory atomic
filesystem boundary.

Schema-version-1 machine reports gain additive `baselined` and
`baseline_problems` fields. Existing consumers are already required to ignore
unknown fields within that schema version.

## Alternatives

- Rule-wide disablement was rejected because it permits new defects.
- Line-number identity was rejected because harmless movement creates churn
  and offsets are not semantic source identity.
- Message text identity was rejected because wording is not a stable rule
  contract.
- Wall-clock expiry was rejected because identical inputs would produce
  different results on different dates.
- Automatically rewriting the baseline during ordinary lint was rejected
  because non-writing analysis must remain non-mutating.

## Consequences

Repositories can enable strict groups immediately while keeping every changed
or newly introduced finding actionable. Baseline documents are reviewable,
source-free, deterministic, and safely replaceable. Identical diagnostic spans
within one file intentionally aggregate by count, so reducing occurrences
produces a stale count instead of depending on unstable offsets.

## Revisit Trigger

Revisit if multi-root generation, editor-managed baseline actions, or evidence
for a stronger semantic fingerprint is required without sacrificing
determinism or source privacy.
