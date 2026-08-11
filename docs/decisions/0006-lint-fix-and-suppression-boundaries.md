# ADR 0006: Lint, Fix, And Suppression Boundaries

- Status: accepted for prototype
- Date: 2026-08-09

## Context And Evidence

Oxlint validates node-interest dispatch, correctness-focused defaults, and
explicit fix classes, but its ordinary semantic cost, diagnostic ordering, and
silent overlap skipping do not meet Gox's contract. `go/analysis` provides
useful interoperability but lacks native Gox tiers and safety metadata.

The suppression design was refreshed against Oxc commit `f2125a8` on
2026-08-11. Oxlint still accepts ESLint and Oxlint line, next-line, range, and
unscoped disable-all comments. It tracks intervals and unused directives, but
plugin-name compatibility can broaden rule matching. Gox needs exact native
rule identity, one auditable scope per directive, and formatter-stable physical
ownership rather than ESLint-compatible breadth.

## Decision

Native rule metadata controls scheduling, documentation, presets, generated
files, and fix safety. Suppressions name exact rules and bind to physical token
ownership. The fix coordinator source-versions every edit, rejects all
conflicts explicitly, reparses, formats, validates, and performs one atomic
single-file replacement.

The coordinator accepts explicit diagnostic and fix-name selections; choosing
among alternative named fixes remains driver policy. Ordinary coordination
accepts safe fixes only. Suggestion and unsafe selections require independent
explicit options. Every selection is bound to the diagnostic path and source
digest, and every edit boundary must be a valid UTF-8 byte boundary.
The coordinator refuses an invalid input file rather than treating fixes as a
syntax-recovery mechanism.

Half-open replacements may coexist with insertions at their start or end.
Insertions inside replacements, overlapping replacements, and two insertions at
one offset conflict. A conflict rejects every complete fix that participates,
while independent fixes may proceed. Any parse or formatter-validation failure
rolls back every otherwise accepted fix in that source transaction.

The initial suppression grammar is line-comment-only and accepts exactly one
rule ID per directive:

```text
//gox:ignore rule-id [-- reason]
//gox:ignore-line rule-id [-- reason]
//gox:ignore-start rule-id [-- reason]
//gox:ignore-end rule-id
//gox:ignore-file rule-id [-- reason]
```

`ignore` owns the immediately following physical line. `ignore-line` owns the
directive's physical line. A matched `ignore-start` and `ignore-end` pair owns
the half-open physical range between the two comments. `ignore-file` owns the
complete file and is valid only before the package clause. Diagnostic primary
range starts determine ownership; a broad diagnostic does not consume a
suppression merely because it overlaps a target.

An index is bound to the normalized source path, exact source digest, and byte
length that produced it. Application preserves diagnostic order and chooses
the first source-ordered matching directive. Later overlapping directives stay
unused unless a different diagnostic selects them, making redundant waivers
observable instead of silently treating every overlap as used.

Reasons use `--` as an explicit separator. A non-empty reason is accepted by
default and can be required by typed policy. Range ends never carry reasons.
Unknown rules, malformed syntax, missing required reasons, misplaced file
directives, nested same-rule ranges, unmatched ends, and unclosed starts are
source-ordered problems. Application reports unused directives; canonical
unused diagnostics and structured expiry inputs remain deferred.

## Alternatives Rejected

- Make `analysis.Analyzer` the native rule API: insufficient tier, product,
  safety, and configuration metadata.
- First-fix-wins or silently skip overlaps: nondeterministic and hides unsafe
  partial application.
- Unscoped disable-all directives: unauditable and too easy to preserve against
  the wrong target.
- Comma-separated rule lists: one directive could accumulate unrelated waiver
  reasons and makes unused-rule reporting ambiguous.
- Line-number-only ownership: formatter movement could silently change the
  suppressed syntax without changing directive text.
- Multi-file fixes initially: no credible recovery transaction yet.

## Consequences

Some useful ecosystem fixes remain suggestion-only or unavailable until
audited. Conflicts produce an actionable outcome instead of partial success.
Fixing necessarily depends on the formatter being stable first.
Single-rule suppressions are more verbose than ESLint-compatible lists but
retain one reason and one usage record per waived rule.
The initial coordinator returns validated bytes and provenance in memory.
On-disk digest recheck, atomic replacement, and reporter integration remain a
separate transaction boundary.

## Revisit Trigger

Stable single-file fixing is proven across corpus and failures; an analyzer
requires a documented compatibility extension; or a real multi-file journey
justifies transaction design. Revisit suppression syntax only when real Go
adoption evidence demonstrates that single-rule directives or immediate-line
ownership are insufficient.
