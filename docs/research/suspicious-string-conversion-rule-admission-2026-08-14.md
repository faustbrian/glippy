# `suspicious-string-conversion` Rule Admission, 2026-08-14

## Decision

Admit the Go `stringintconv` analyzer as warning-level `suspicious`. `string(x)`
for a non-byte, non-rune integer produces one Unicode code point rather than
decimal digits. That is commonly a defect, but intentional code-point behavior
prevents a near-zero-false-positive correctness contract.

## Boundary

The typed x/tools v0.48.0 rule excludes byte, rune, and untyped-rune sources.
It may miss complex type sets it cannot classify. Generated files and ill-typed
packages are excluded, exact suppressions apply, and pre-1.25 sources do not
select it. Both upstream alternatives are suggestions: `fmt.Sprint` chooses
decimal text while `string(rune(x))` makes code-point intent explicit. Glippy
does not silently choose between those semantics.

## Cost And Dogfood

A one-iteration package-load probe measured 152,755,666 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
