# Oxfmt, Oxlint, And Oxc Audit

Date: 2026-08-09

Audited source: Oxc commit
[`9d19ab3`](https://github.com/oxc-project/oxc/commit/9d19ab308066d82dc3e53d5fc359807f8127b3d4),
website commit
[`d5ae73b`](https://github.com/oxc-project/website/commit/d5ae73b8630a9a090931d64fcc55cece034477ad),
and release
[`apps_v1.77.0`](https://github.com/oxc-project/oxc/releases/tag/apps_v1.77.0)
(Oxlint 1.77.0, Oxfmt 0.62.0). Current source and product documentation
outweigh the repository's partly stale `ARCHITECTURE.md`.

## Final Readiness Refresh

The Phase 5 comparison was refreshed on 2026-08-13 against Oxc commit
[`6e040e4`](https://github.com/oxc-project/oxc/commit/6e040e494eb7607a481afff11944f41e01e0e41b)
and website commit
[`7c4f077`](https://github.com/oxc-project/website/commit/7c4f07766d992c1a8dd7bc8db1274aca46b36d73).
The audited packages report Oxfmt 0.63.0 and Oxlint 1.78.0. This is the final
readiness review required by Phase 5; another review remains necessary only if
the release candidate materially changes the corresponding Gox subsystem or
the reference products materially change before release.

Current Oxfmt source and documentation confirm:

- default write mode, separate `--check` and `--list-different` modes, stdin
  through `--stdin-filepath`, nested configuration, and LSP integration;
- a language-neutral document IR with line modes, groups, fill, line suffixes,
  conditionals, and best-fitting variants, plus iterative printing with
  reusable fit stacks and queues; and
- ordinary [`fs::write`](https://github.com/oxc-project/oxc/blob/6e040e494eb7607a481afff11944f41e01e0e41b/apps/oxfmt/src/cli/service.rs#L79-L96)
  for changed files rather than a validated atomic replacement transaction.

Gox deliberately retains a smaller Go-native configuration dialect, explicit
stdout/check/write behavior, immutable source and trivia ownership, bounded
canonical layouts, and prevalidated atomic writes. It continues to defer an
LSP because the proven stdin/stdout editor path does not yet show a latency,
diagnostic, fix, or cache requirement that needs a resident service.

Current Oxlint source and documentation confirm:

- independent `--fix`, `--fix-suggestions`, and `--fix-dangerously` switches;
  the last authorizes its combined dangerous-fix-or-suggestion class;
- type-aware and type-check modes, shared type-program use, and fixes from
  type-aware rules;
- default, agent, Checkstyle, GitHub, GitLab, JSON, JUnit, SARIF, stylish, and
  Unix reporters;
- ESLint- and Oxlint-compatible line, next-line, range, multi-rule, and
  disable-all suppressions; and
- separate safe and dangerous LSP fix-all actions, with suggestions remaining
  individual quick fixes.

The current JSON envelope still has no explicit schema-version field and still
includes runtime-dependent `threads_count` and `start_time`. SARIF uses its
standard 2.1.0 schema. The current batch fixer still sorts candidate edits,
keeps the first eligible fix, silently leaves later overlapping or adjacent
fixes unapplied, reparses only in debug builds, and writes through ordinary
[`fs::write`](https://github.com/oxc-project/oxc/blob/6e040e494eb7607a481afff11944f41e01e0e41b/crates/oxc_linter/src/service/runtime.rs#L198-L205).
The LSP similarly drops later overlapping edits.

Gox therefore retains versioned deterministic machine records without runtime
timing or worker identity, exact source-version checks, rejection of every
participant in an edit conflict, release-build reparsing and validation,
formatter normalization, and atomic replacement. It also retains exact-rule
auditable suppressions and separate safe, suggestion, and unsafe authorization.
Reporter breadth remains consumer-driven: Gox does not add formats merely to
match a reference catalog.

No current Oxfmt, Oxlint, or Oxc evidence invalidates the shared frontend,
separate formatter and linter engines, document renderer, tiered Go analysis,
diagnostic model, or fix coordinator. The refresh changes no Gox architecture;
it reconfirms the deliberate Go- and safety-specific differences above.

## v0.5 Pre-Release Refresh

The comparison was refreshed again on 2026-08-21 against Oxc commit
[`2dad1e0`](https://github.com/oxc-project/oxc/commit/2dad1e0ec7ba2878fc5472c9430b8ee28aa41b54)
and website commit
[`84e863f`](https://github.com/oxc-project/website/commit/84e863ff38308165e2b8ceb4c47363f042895ad2).
The audited packages report Oxfmt 0.64.0 and Oxlint 1.79.0.

Oxfmt retains the language-neutral arena document IR, iterative fit printer,
CLI/LSP split, nested configuration, explicit stdin filepath, check and
list-different modes, and ordinary `fs::write` replacement described above.
The material formatter change since the previous snapshot is support for an
experimental operator-position option. JavaScript users may select operators
at the start or end of broken lines. Glippy deliberately keeps Go binary
operators on the preceding line because Go semicolon insertion makes that
placement part of syntax safety rather than a style preference. Oxfmt also
clarifies that explicitly requested stdin, LSP documents, and file paths are
not excluded merely by `.gitignore`; this remains compatible with Glippy's
explicit-input policy.

Oxlint retains independent safe, suggestion, and dangerous CLI fix switches;
safe and dangerous LSP fix-all actions; suggestion-only quick fixes; shared
multi-file and type-aware analysis; broad reporters; and ESLint-compatible
suppressions and plugins. Its production fixer still sorts edits, silently
leaves later overlapping or boundary-adjacent fixes unapplied, reparses only
under debug assertions, and writes through ordinary `fs::write`. Its JSON
envelope still includes runtime-dependent thread and start-time fields without
an explicit schema version. Catalog and compatibility work since 1.78.0 does
not change those boundaries.

The current references therefore do not require a Glippy architectural change.
They reinforce the retained decisions: one Go-native product with separate
formatter and linter engines, bounded document rendering, demand-driven shared
analysis, deterministic versioned diagnostics, and transactional fix and write
validation.

Reporting was refreshed on 2026-08-10 against Oxc commit
[`8e9b95f`](https://github.com/oxc-project/oxc/commit/8e9b95f3b61534b220bc6577a2fa3552c91433a4).
Oxfmt still separates check and list-different output and has no machine result
schema. Oxlint's JSON reporter remains the useful diagnostic reference, but its
runtime timing and thread fields are not deterministic result data and its
example envelope carries no explicit schema version. ADR 0011 records Gox's
deliberate versioned formatter-report boundary.

Version behavior was refreshed on 2026-08-11 against Oxc commit
[`fed2b90`](https://github.com/oxc-project/oxc/commit/fed2b900ebca2deafdc4fa41680ee47af09f2f28).
Oxfmt 0.63.0 and Oxlint 1.78.0 both expose their build-owned Cargo package
version through the CLI parser. Gox retains its contracted `version`
subcommand, gives explicit release metadata the same authority, and defines Go
module and `devel` fallbacks for other builds.

Diff behavior was refreshed against the same current commit. Oxfmt exposes
`--check` and `--list-different`, but no unified source-diff mode. Gox retains
its path-listing check output and deliberately adds a bounded, Go-familiar
unified `--diff` surface; ADR 0012 records the resource and stdout boundaries.

Suppression behavior was refreshed on 2026-08-11 against Oxc commit
[`f2125a8`](https://github.com/oxc-project/oxc/commit/f2125a8aab3476954b803ebfe1993f2cb0e255e3).
Oxlint still accepts both ESLint and Oxlint disable forms, including unscoped
disable-all comments, and uses interval lookup with compatibility-oriented
rule-name matching. Its current plugin API separates rule text from an
optional `--` justification. Gox retains only the explicit separator and
rejects disable-all, rule lists, and plugin-name equivalence; ADR 0006 records
the exact physical-scope grammar.

Expiry behavior was refreshed the same day against current Oxc commit
[`e224f5f`](https://github.com/oxc-project/oxc/commit/e224f5f3852f7368f02d4f280e164878d64c4cd9).
Oxlint's disable-directive and suppression sources still expose no structured
expiry contract. Gox therefore uses its own explicit `expires=YYYY-MM-DD`
metadata and configured cutoff. It does not infer the cutoff from the wall
clock because that would make identical source and configuration produce
date-dependent results.

Lint reporting was refreshed against the same current commit. Oxlint's JSON
formatter still buffers diagnostics, delegates their shape to Miette, and adds
file, rule, thread, and elapsed-time counts without a schema version. It offers
multiple human and integration formats, but its JSON test remains disabled on
Windows because newline conversion changes offsets. Gox retains physical byte
offsets over immutable source bytes, an explicit source digest and schema
version, and no timing or thread identity in ordinary result data.

Fix-mode behavior was refreshed on 2026-08-11 against Oxc commit
[`00e1b76`](https://github.com/oxc-project/oxc/commit/00e1b762e0dd0d1cb451c3e46ea2751459dadb46).
Current Oxlint
[`FixOptions`](https://github.com/oxc-project/oxc/blob/00e1b762e0dd0d1cb451c3e46ea2751459dadb46/apps/oxlint/src/command/lint.rs#L219-L256)
and documentation expose independent `--fix` and `--fix-suggestions` switches,
while `--fix-dangerously` enables its combined dangerous-fix-or-suggestion
class. Gox retains separate safe, suggestion, and unsafe authorizations so
selecting unsafe transformations never implicitly selects suggestions.

## Formatter Findings

Oxfmt separates language parsing and lowering, a language-neutral document IR,
printing, and CLI/LSP integration. Its
[`FormatElement`](https://github.com/oxc-project/oxc/blob/9d19ab308066d82dc3e53d5fc359807f8127b3d4/crates/oxc_formatter_core/src/format_element/mod.rs#L43)
is an arena-oriented flat IR with line modes, groups, indentation, fill,
conditionals, line suffixes, expansion propagation, labels, and interned
subdocuments. The
[`printer`](https://github.com/oxc-project/oxc/blob/9d19ab308066d82dc3e53d5fc359807f8127b3d4/crates/oxc_formatter_core/src/printer/mod.rs#L89)
is iterative and reuses scratch storage during fit measurement.

Oxfmt retains source text and uses positional scans for information its AST
does not own. Its comment cursor is source ordered, and its formatter policy
requires comments and suppressions not to cross content boundaries. This
supports Gox retaining immutable bytes, lexical tokens, token gaps, comment
identity, directives, and byte offsets rather than treating `go/ast` as a CST.

Blank-line behavior was refreshed on 2026-08-12 against Oxc commit
[`dd3e416`](https://github.com/oxc-project/oxc/commit/dd3e4160a230d376e93b91fa9c2031d282ceefdd).
The current formatter builders explicitly preserve intentional blank lines
between adjacent nodes while capping consecutive empty lines to one, and the
call-argument printer applies the same source-gap policy between arguments.
This supports retaining one source-authored statement-group separator in Gox
without retaining arbitrary indentation or repeated blank padding.

The formatter rejects any parse diagnostic. Its CLI provides check, stdin,
configuration discovery, bounded threads, CI, and LSP workflows. Gox must not
copy three weaker operational choices: default in-place mutation, plain
non-atomic writes, or lazy nested-configuration failure after earlier files
have already been written.

## Linter Findings

Oxlint generates node-interest metadata and dispatches interested rules during
one shared AST pass. This validates Gox's shared traversal direction. Oxlint's
normal semantic builder enables CFG broadly, however, so it is not evidence
against Gox's cheaper explicit syntax, types, CFG, and SSA tiers.

Oxlint has correctness-focused defaults, multiple reporter formats, metadata,
precise diagnostics, and safe/suggestion/dangerous fix classes. Production
parallel reporting is not globally ordered across every reporter. Gox must
collect canonical diagnostics and perform one deterministic final ordering
before rendering.

The Oxlint fixer sorts edits and silently skips later overlaps, carries no
source-version identity, reparses only under debug assertion, and writes
without a formatter transaction. Gox adopts the fix categories but rejects
those coordinator semantics: stale and conflicting edits are explicit
outcomes; every accepted result is reparsed, formatted, validated, and
atomically replaced before success is reported.

Oxlint's suppressions permit broad disable-all comments and its configuration
intentionally inherits ESLint compatibility breadth. Gox instead requires
exact rule identifiers, auditable ownership, optional reason enforcement, and
a small typed configuration surface.

## Adopted Lessons

1. One binary may share frontend infrastructure while formatter and linter
   remain separate engines.
2. A flat document arena and iterative renderer deserve preference over a
   boxed recursive tree, subject to Go benchmarks.
3. One canonical flat form and one broken form should be the default; extra
   best-fitting alternatives require a documented construct.
4. Original source and byte-position access remain available through lowering,
   formatting, diagnostics, and edits.
5. Syntax rules share node-interest traversal; expensive representations are
   demand-driven and run-owned.
6. All configuration that can affect a write transaction is validated before
   the first write.
7. Every reporter consumes the same sorted diagnostic records and stable exit
   outcome.
8. Persistent caching requires an independently specified complete key; the
   audited Oxc implementation is not evidence for such a cache.

## Rejected Reference Directions

Gox will not copy Oxfmt's write safety, Oxlint's conflict resolution,
always-on CFG cost, nondeterministic reporter arrival order, coarse exit codes,
unscoped suppression behavior, or ESLint-compatible plugin/configuration
surface. LSP service work remains deferred until stdin/stdout workflows and a
shared-state latency need are proven.
