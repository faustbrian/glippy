# `redundant-else` Rule Admission, 2026-08-13

## Decision

Admit `redundant-else` to the opt-in `pedantic` preset at warning severity. The
syntax-tier rule reports the exact `else` keyword when the direct `if` body
provably transfers control and the alternative is a block.

## Defect And Existing Tools

An alternative does not need an `else` when the preceding branch returns,
breaks, continues, or jumps away. Keeping it adds nesting and makes the
continuing path harder to scan. The compiler and the Go 1.26.5 default vet
catalog do not diagnose this structure.

Revive's current `superfluous-else` rule at
[`1de8243783d480e24c0db1a3dc45976aeaf715e9`](https://github.com/mgechev/revive/commit/1de8243783d480e24c0db1a3dc45976aeaf715e9)
and its regression fixtures establish repeated Go review demand. Staticcheck's
S1023 covers redundant control flow but does not expose this exact product
contract. Clippy's current `needless_else` implementation at
[`d2c4d1532d89488a56ec2c3ca12757117fc0b4e2`](https://github.com/rust-lang/rust-clippy/commit/d2c4d1532d89488a56ec2c3ca12757117fc0b4e2)
only removes empty Rust alternatives, so it supports the simplification group
but is not an implementation oracle for Go control transfer.

## Precision, Policy, And Fixes

The rule recognizes direct final `return`, `break`, `continue`, and `goto`
statements, nested final blocks, and nested `if` statements whose two block
branches terminate. It excludes `if` initializers because moving the alternative
would change initializer scope, excludes `else if`, and does not infer
termination from calls such as `panic` or `os.Exit`.

Generated files are excluded. As a syntax rule it can still report in an
otherwise ill-typed package. Its minimum source version is Go 1.25. Exact
suppression and baseline identity use `redundant-else` and
`terminating-if-branch`.

No fix is registered. Removing the braces must preserve comments, scope, and
statement ownership, and those edit proofs do not yet exist.

## Admission Evidence

The focused test first failed because the registry did not contain the rule.
Fixtures cover return, continue, nested termination, initializers, live
branches, exact keyword ranges, metadata, generated files, type-error behavior,
source versions, suppressions, deterministic baselines, JSON, explain output,
and absence of fixes.

Five syntax-only samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured a
median of `7,858 ns/op`, `11,056 B/op`, and `84 allocs/op`.

Non-mutating pedantic dogfood reported two true-positive simplification
opportunities in Glippy and one in `go-libraries/pkg/prompts` at
`6ed3a06a4e1aba412d2a6b91454774234f30a464`. They were classified only; neither
repository was modified to silence dogfood.

## Revisit Trigger

Add a safe fix only after comment-preserving block extraction and scope
validation are proven. Consider call-based termination only if an authoritative,
bounded contract can distinguish non-returning calls without speculative SSA.
