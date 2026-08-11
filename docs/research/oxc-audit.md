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
