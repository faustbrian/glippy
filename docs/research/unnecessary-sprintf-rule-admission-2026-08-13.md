# `unnecessary-sprintf` Rule Admission, 2026-08-13

## Decision

Admit `unnecessary-sprintf` to the opt-in `pedantic` preset at warning severity.
The types-tier rule reports standard-library `fmt.Sprintf("%s", value)` when
the single value is already string-representable without formatting machinery.

## Defect And Existing Tools

Formatting an existing string, defined string, or byte slice through the exact
`%s` directive adds work and hides a direct representation. The compiler and
Go 1.26.5 default vet `printf` analyzer validate the call but do not recommend
the simpler operation.

Staticcheck S1025 is the primary Go authority, inspected in current source at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Revive's current `unnecessary-format` rule at
[`1de8243783d480e24c0db1a3dc45976aeaf715e9`](https://github.com/mgechev/revive/commit/1de8243783d480e24c0db1a3dc45976aeaf715e9)
covers the related no-directive case. Clippy's formatting simplifications
support the catalog category, but Rust formatting traits are not a semantic
oracle for Go's `fmt` package.

## Precision, Policy, And Fixes

The rule resolves the exact standard-library `fmt.Sprintf` object, requires one
compile-time `%s` directive and one data argument, and admits strings, defined
string types, and byte slices. Dynamic formats, other verbs, multiple
directives, interfaces, type parameters, Stringer or Formatter values, and rune
slices are excluded because their output or replacement contract can differ.

Generated and ill-typed packages are excluded. The minimum source version is Go
1.25. Exact suppressions and baselines use `unnecessary-sprintf` and
`direct-string-representation`.

No fix is registered until the replacement expression, conversion choice,
precedence, and argument-comment ownership are all proven.

## Admission Evidence

The focused test first failed because the rule was absent. Fixtures cover
strings, defined strings, byte slices, interfaces, dynamic and alternate
formats, multiple directives, exact ranges, metadata, generated and type-error
policy, source versions, suppressions, deterministic baselines, JSON, explain,
and no-fix behavior.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured a
median of `72,726,575 ns/op`, `1,665,131 B/op`, and `13,229 allocs/op`. Import
and package loading dominate this proportional measurement.

Non-mutating pedantic dogfood produced no findings for this rule in Glippy or
`go-libraries/pkg/prompts` at
`6ed3a06a4e1aba412d2a6b91454774234f30a464`.

## Revisit Trigger

Broaden the recognized value set only when the direct replacement is unique and
preserves Go formatting semantics. Add fixes per replacement class rather than
assuming one edit fits strings, byte slices, and method-based values.
