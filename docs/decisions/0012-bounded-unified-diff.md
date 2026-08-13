# ADR 0012: Bounded Unified Formatter Differences

- Status: accepted for formatter prototype
- Date: 2026-08-11

## Context And Evidence

Phase 2 requires a reviewable formatter-difference surface that does not write
source. Go developers already recognize the unified output of `gofmt -d`.
Current Oxfmt source at Oxc commit `fed2b900` instead exposes check and
list-different modes; it does not provide source diffs. Repeating only changed
paths would duplicate Glippy check mode and would not let developers review the
proposed layout.

An unrestricted longest-common-subsequence search can consume quadratic memory
or time on untrusted repeated-line files. Replacing every changed file as one
hunk is bounded but makes ordinary formatter output unnecessarily difficult to
review.

## Decision

`glippy fmt --diff [paths...]` is a path-based, non-writing, text-only mode. It
emits standard unified differences with three context lines, the normalized
source path as the new label, and the same path plus `.orig` as the old label.
Labels containing tabs or line breaks are quoted. Changed files are emitted in
normalized path order; unchanged files are omitted. Findings exit with code 1,
and an already formatted selection exits successfully without output.

The line matcher first preserves common prefixes and suffixes, then uses unique
patience anchors. Anchor-free regions use a capped LCS matrix. Work, recursion,
and LCS cells have fixed ceilings; a region that reaches a ceiling becomes one
deterministic delete-and-insert replacement. Diff construction is serialized
after bounded parallel formatting so worst-case matcher allocation is not
multiplied by the formatter worker count.

`--diff`, `--check`, and `--write` are mutually exclusive. JSON reporting is
rejected for diff mode because the unified source difference owns stdout. A
JSON-requesting invalid invocation still receives the versioned JSON error
envelope.

## Alternatives Rejected

- Reuse check mode's path list: duplicates an existing surface without showing
  the proposed source.
- Invoke a platform `diff` executable: adds an ordinary-run subprocess and
  platform-dependent output and availability.
- Add an unbounded Myers or LCS dependency: permits adversarial source to turn
  a review aid into unbounded memory work.
- Always replace the complete file in one hunk: bounded, but poor for ordinary
  review when unchanged unique lines can cheaply anchor smaller regions.
- Put source diffs inside schema-version-1 JSON: discloses source through a
  machine format whose accepted contract intentionally excludes snippets.

## Consequences

Ordinary formatter changes produce familiar patchable output without touching
the filesystem. Highly repetitive or adversarial regions can be less minimal,
but remain complete, deterministic, and bounded. The driver retains original
and formatted bytes until the final ordered diff is constructed.

## Revisit Trigger

A validated editor or CI consumer requires structured edits, measured dogfood
shows the fallback makes common diffs unreadable, or large-file benchmarks
justify different ceilings without weakening bounded execution.
