# Go Frontend Capability Audit

Date: 2026-08-09

Audited Go 1.26.5 at
[`c19862e`](https://github.com/golang/go/commit/c19862e5f8415b4f24b189d065ed739517c548ba)
and `golang.org/x/tools` v0.48.0 at
[`05f9cb5`](https://github.com/golang/tools/commit/05f9cb5d358503005bd6f82b17916d226ca7b210).
The latter requires Go 1.25.

## Conclusion

The standard Go frontend satisfies Gox syntax and semantic requirements. Gox
needs a thin concrete-source ledger over immutable bytes and a second
`go/scanner` pass; no competing parser is justified.

`go/parser` does not expose its scanner stream. The scanner uniquely reports
explicit semicolons as `";"` and inserted semicolons as `"\n"`, while the
parser consumes ordinary separators. Original bytes remain authoritative
because whitespace, BOM presence, newline style, token gaps, and raw carriage
returns are not preserved. Scanner and AST values remove carriage returns from
raw strings and comments, and `CommentGroup.Text` performs further
normalization.

Physical edits use source identity and byte offsets from `token.File.Offset`
or unadjusted `PositionFor(pos, false)`. They never use adjusted positions or
raw `token.Pos` ordering: `//line` changes logical locations and concurrent
`go/packages` parsing does not guarantee ordered file bases.

## v0.5 Pre-Release Refresh

The frontend boundary was refreshed on 2026-08-21 against Go 1.26.7 at tag
[`e3336a2`](https://go.googlesource.com/go/+/e3336a22ad3f0a90bd252c95d8b5544e02674205)
and the still-selected `golang.org/x/tools` v0.48.0 at
[`05f9cb5`](https://go.googlesource.com/tools/+/05f9cb5d358503005bd6f82b17916d226ca7b210).

The current `go/parser`, `go/ast`, `go/token`, and `go/scanner` contracts retain
the same concrete-source limitations: partial ASTs do not own whitespace,
semicolon origin, exact comment bytes, or formatter attachment policy. Current
`go/packages` still supplies concurrent-safe parse hooks, overlays, build
selection, tests, types, and package diagnostics. `go/analysis` still lacks
Glippy's rule tier, severity, generated-file, node-interest, and fix-safety
metadata. CFG construction remains an explicit per-function operation.

The current SSA API exposes `Program.SetNoReturn`; Glippy supplies its proven
standard-library, project-contract, and selected-module no-return predicate at
`internal/analysis/ssa.go`. This improves semantic fidelity without replacing
the standard frontend. No Go 1.26.7 or x/tools v0.48.0 evidence requires a new
parser, type checker, CFG, SSA implementation, or broader analyzer adapter.

## v0.8 Go 1.27 Refresh

The frontend boundary was refreshed on 2026-08-25 against Go 1.27.0 at tag
[`8af2175`](https://go.googlesource.com/go/+/8af21751f066eced273ca3ce49506b366847c623)
and `golang.org/x/tools` v0.49.0 at
[`18332fe`](https://go.googlesource.com/tools/+/18332fec72972efbb8ab9881984fec2d8cfc2b58).

Go 1.27 adds generic methods and promoted-field keys in struct literals.
The standard parser, AST, token, scanner, types, package loader, and SSA layers
represent both forms without a competing frontend. Glippy fixtures exercise
formatting, reparsing, idempotency, typed loading, and SSA construction for
these forms. The original-source and trivia requirements remain unchanged:
neither new AST shape owns whitespace, semicolon origin, exact comment bytes,
or formatter attachment policy.

x/tools v0.49.0 retains the package-loading, analyzer, CFG, SSA, and fact
boundaries Glippy uses. Its analyzer updates are covered by the adapted-rule
contract tests and the refreshed Go vet compatibility inventory. No Go 1.27.0
or x/tools v0.49.0 evidence requires replacing the standard frontend or
expanding the external analyzer boundary.

## Fidelity Matrix

| Concern | Standard frontend | Required Gox state |
| --- | --- | --- |
| Tokens | Scanner kind, start, and literal | Raw start/end slice, delimiter ledger, semicolon origin |
| Whitespace | Token positions only | Every gap, tabs, blank lines, trailing bytes |
| Newlines/BOM | Line table; initial BOM skipped | Exact BOM, CRLF/LF policy, final newline |
| Parentheses | Usually `ast.ParenExpr` | Exact token accounting and source-order identity |
| Literals | Usually spelling; Go 1.26 `ValueEnd` | Original byte slice, including removed CR bytes |
| Comments | Lexical list and selected AST links | Raw bytes/end, identity, gap, ownership, directive class |
| Directives | Mostly ordinary comments | Exact text, class, adjacency, scope, build effect |
| Errors | Partial AST and `ast.Bad*` | Raw malformed region and bounded diagnostic state |

Primary evidence includes scanner semicolon handling
([source](https://github.com/golang/go/blob/c19862e5f8415b4f24b189d065ed739517c548ba/src/go/scanner/scanner.go#L780-L888)),
parser separator consumption
([source](https://github.com/golang/go/blob/c19862e5f8415b4f24b189d065ed739517c548ba/src/go/parser/parser.go#L352-L370)),
raw-string CR removal
([source](https://github.com/golang/go/blob/c19862e5f8415b4f24b189d065ed739517c548ba/src/go/scanner/scanner.go#L703-L729)),
comment normalization
([source](https://github.com/golang/go/blob/c19862e5f8415b4f24b189d065ed739517c548ba/src/go/ast/ast.go#L62-L130)),
and heuristic comment maps
([source](https://github.com/golang/go/blob/c19862e5f8415b4f24b189d065ed739517c548ba/src/go/ast/commentmap.go#L112-L127)).

## Analysis Tiers

- **Lexical:** immutable bytes, scanner tokens, gaps, comments, directives,
  physical positions. No subprocess.
- **Syntax:** `parser.ParseFile` with `ParseComments | SkipObjectResolution`.
  Partial ASTs are diagnostic-only for the stable formatter.
- **Types:** one `go/packages` load per explicit build configuration, starting
  from `LoadSyntax | NeedModule`. Package errors are read from each
  `Package.Errors`; objects from separate loads are never mixed.
- **CFG:** lazily construct `cfg.New` per required function. It omits
  short-circuit edge semantics, conditional predicates, and panic flow.
- **SSA:** build only for well-typed required packages. `ssautil.AllPackages`
  and `LoadAllSyntax` are whole-program costs and require rule-specific proof.

`CompiledGoFiles` may contain synthesized cgo files and must never become a
write target. `Tests=true` creates multiple package variants, so canonical
physical diagnostics require deduplication. A custom concurrent-safe
`packages.Config.ParseFile` can capture source and use
`SkipObjectResolution`.

`ast/inspector` pays one full indexing traversal and becomes favorable only
when repeated filtered queries amortize that index. Gox performs one union
node-interest dispatch, so the Phase 3 benchmark compared direct shared
`ast.Inspect`, one inspector union query, and naive per-rule walks at 1, 3, 5,
10, and 25 rules. One naive walk had a 1.81-microsecond lower median at one
rule; direct dispatch had lower medians from three through 25 rules and used
456-1,896 bytes per operation versus 28,672-30,160 for inspector indexing. Gox
keeps one direct shared traversal path until a representative workload with
repeated queries or a material one-rule regression provides contrary evidence.

## `go/analysis` Boundary

The native rule API remains primary because `analysis.Analyzer` has no Gox
requirement tier, severity/preset metadata, generated-file policy, fix safety,
typed configuration schema, or node-interest declaration. Imported suggested
fixes default to suggestion-only, carry a source digest through the Gox
adapter, and pass through the same conflict and validation coordinator.

The Phase 3 adapter therefore accepts only analyzers without prerequisites,
facts, result types, or flags. It reparses one isolated AST and matching file
set per analyzer and file, supplies a package-name shell without type
information, and restricts `Pass.ReadFile` to the exact immutable source. A
run-local analyzer descriptor prevents mutation through `Pass.Analyzer` from
escaping into later runs. Diagnostic, related, and edit positions must belong
to the adapted file; panics, foreign positions, undeclared fixes, and unexpected
results fail the run. Diagnostic help follows the upstream category and
relative-URL resolution contract. Safe imported fixes require an explicit audit
assertion.

The driver checks cancellation before and after `Analyzer.Run`, discarding
findings when cancellation is observed. The upstream callback has no context
parameter, so it cannot be preempted safely if it does not return. Typed
prerequisites, facts, configuration-backed flags, results, and broader file
access remain Phase 4 compatibility work rather than hidden syntax-tier costs.
Because a function value does not reveal whether it reads deprecated
`ast.Object` links or silently assumes absent pass fields, suitability for this
syntax-only contract requires a maintainer audit before registration.

## Printer And Formatter Boundary

`go/printer` and `go/format` remain differential references. They expose no
document grouping API; `go/format` sorts imports and normalizes literals; the
printer can relocate or synthesize build constraints. Gox must not use them as
a post-pass or fidelity validator. `go/format` also documents that output can
change across Go releases, so all compatibility evidence records the exact
toolchain.
