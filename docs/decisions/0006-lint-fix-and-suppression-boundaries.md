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

Fix-mode behavior was refreshed against Oxc commit `00e1b76` on 2026-08-11.
Oxlint's current `FixOptions` exposes `--fix`, `--fix-suggestions`, and
`--fix-dangerously`; its dangerous mode also selects suggestions. Gox needs
independent authorization for all three classes because an unsafe
transformation does not imply consent to every suggestion.

## Decision

Native rule metadata controls scheduling, documentation, presets, generated
files, fix safety, typed configuration, paired examples, deprecation, and known
limitations. The human `explain` renderer consumes an immutable metadata copy
from the same registry and makes empty fix, configuration, and limitation sets
explicit. Suppressions name exact rules and bind to physical token ownership.
The fix coordinator source-versions every edit, rejects all conflicts
explicitly, reparses, formats, validates, and performs one atomic single-file
replacement.

The `go/analysis` adapter accepts syntax-only and audited read-only types-tier
analyzers without prerequisites, facts, result types, or flags. Syntax runs
reparse an isolated AST with a matching file set, supply a minimal package-name
shell without type information, expose only the adapted source through
`ReadFile`, and use a run-local analyzer descriptor. Typed runs execute after
all native types, CFG, and SSA consumers over the load-owned package AST, type
information, sizes, module metadata, type errors when admitted, and exact
compiled-source bytes. They are ordered by package and rule ID, preserve one
physical owner across test variants, and exclude the synthetic test-main
package from lint targets.

Native metadata remains authoritative. A typed adapter requires an explicit
read-only audit and cannot opt into type-error packages unless the upstream
analyzer declares `RunDespiteErrors`. Diagnostics may target any captured file
in their package, but related locations and fixes remain single-file. Panics,
foreign positions, cross-file related locations or edits, undeclared fixes,
unexpected results, invalid help URLs, and excessive module replacement chains
fail analysis instead of producing partial findings. Help links follow upstream
category and relative-URL resolution.

Suggested fixes require exact message-to-native-metadata mappings and default
to suggestion safety. A mapping can declare a safe fix only with an explicit
audit assertion. Cancellation is checked immediately around analyzer execution;
the adapter discards findings after cancellation, but cannot preempt an
analyzer callback because the upstream run contract accepts no context.
Maintainers audit imported analyzers for dependence on deprecated object
resolution or other absent pass state before registration; those dependencies
cannot be inferred reliably from an analyzer function value.

The coordinator accepts explicit diagnostic and fix-name selections; choosing
among alternative named fixes remains driver policy. The ordinary CLI driver
selects exactly one safe alternative per diagnostic and treats multiple safe
alternatives as an invalid rule contract before writing. Suggestion and unsafe
selections use `--fix-suggestions` and `--fix-unsafe` respectively. The three
flags compose, but each authorizes only its named class. If the enabled classes
expose more than one named alternative for one diagnostic, prevalidation fails
instead of choosing or applying competing fixes. Every selection is bound to
the diagnostic path and source digest, and every edit boundary must be a valid
UTF-8 byte boundary.
The coordinator refuses an invalid input file rather than treating fixes as a
syntax-recovery mechanism.

Half-open replacements may coexist with insertions at their start or end.
Insertions inside replacements, overlapping replacements, and two insertions at
one offset conflict. A conflict rejects every complete fix that participates,
while independent fixes may proceed. Any parse or formatter-validation failure
rolls back every otherwise accepted fix in that source transaction.

The on-disk transaction begins from one descriptor-validated filesystem
snapshot, coordinates its exact bytes, and delegates replacement to the shared
permission-preserving same-directory atomic writer. A stale-source error proves
that replacement did not occur. Any other replacement error is reported as
possibly completed because directory-sync failure can follow a successful
rename.

The CLI prevalidates every selected configuration and source before its first
replacement, refuses generated and symlink-traversing paths, and reruns the
enabled syntax analysis after formatter normalization. Reporters retain the
original source digest and coordinated fix provenance separately from the
result digest and replacement status. Cancellation and reporting failures
disclose confirmed or possibly completed replacements.

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
default; `lint.suppressions.require-reason = true` requires one for every direct
scope and range start across syntax and package analysis. A missing reason
invalidates the directive instead of suppressing its target. Range ends never
carry reasons.

An optional leading `expires=YYYY-MM-DD` reason field records a structured
calendar deadline. Invalid dates invalidate the directive. The optional typed
`lint.suppressions.expiry-cutoff` supplies the deterministic evaluation date;
an expiry on or before it is an `expired` problem and cannot suppress a
diagnostic. Gox never reads the wall clock, so the same source and configuration
remain reproducible. The human reason remains mandatory after expiry metadata
when a separator is present.

Unknown rules, malformed syntax, missing required reasons, invalid or expired
dates, misplaced file directives, nested same-rule ranges, unmatched ends, and
unclosed starts are source-ordered problems. Application reports unused
directives and machine output retains their structured expiry date.

## Alternatives Rejected

- Make `analysis.Analyzer` the native rule API: insufficient tier, product,
  safety, and configuration metadata.
- Share the native syntax tree with imported analyzers: analyzer mutation would
  corrupt later rules and violate the immutable source contract.
- Run unsupported analyzer prerequisites inside the file adapter: this would
  duplicate typed scheduling and fact ownership before those tiers exist.
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
The coordinator retains validated bytes and provenance independently from disk
status. Reporter and lint-driver integration must preserve the distinction
between not performed, completed, and possibly completed replacement.
Imported syntax analyzers pay one isolated parse per analyzer and file. Typed
adapters reuse package state, so their admission audit must exclude mutation
that could affect a later adapter. Neither tier can use facts, prerequisites,
flags, arbitrary cross-file reads, or preempt a callback that does not return.

## Revisit Trigger

Stable single-file fixing is proven across corpus and failures; an analyzer
requires a documented compatibility extension; or a real multi-file journey
justifies transaction design. Revisit suppression syntax only when real Go
adoption evidence demonstrates that single-rule directives or immediate-line
ownership are insufficient.
