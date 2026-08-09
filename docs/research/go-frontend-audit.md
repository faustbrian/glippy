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
after roughly five filtered traversals according to its current documentation.
The scheduler should choose it based on enabled syntax-rule count rather than
unconditionally.

## `go/analysis` Boundary

The native rule API remains primary because `analysis.Analyzer` has no Gox
requirement tier, severity/preset metadata, generated-file policy, fix safety,
typed configuration schema, or node-interest declaration. Imported suggested
fixes default to suggestion-only, carry a source digest through the Gox
adapter, and pass through the same conflict and validation coordinator.

Adapters must reject or isolate analyzers that mutate shared AST state, depend
on deprecated `ast.Object` resolution, require multi-file fixes during the
single-file phase, or otherwise violate the run's immutable frontend contract.

## Printer And Formatter Boundary

`go/printer` and `go/format` remain differential references. They expose no
document grouping API; `go/format` sorts imports and normalizes literals; the
printer can relocate or synthesize build constraints. Gox must not use them as
a post-pass or fidelity validator. `go/format` also documents that output can
change across Go releases, so all compatibility evidence records the exact
toolchain.
