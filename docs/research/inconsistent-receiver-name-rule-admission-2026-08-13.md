# `inconsistent-receiver-name` Rule Admission, 2026-08-13

## Decision

Admit `inconsistent-receiver-name` to the opt-in `pedantic` preset at warning
severity. The package-wide types rule identifies minority receiver names for
methods on one named type.

## Defect And Existing Tools

Receiver names appear throughout method bodies. Switching names within one type
makes copied methods and documentation harder to scan and is a repeated Go
review concern. The Go Code Review Comments explicitly require consistency.
The compiler and Go 1.26.5 default vet catalog do not enforce it.

Staticcheck ST1016 is the primary rule authority and is intentionally
non-default; its current implementation and fixtures were inspected at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Revive's current `receiver-naming` rule at
[`1de8243783d480e24c0db1a3dc45976aeaf715e9`](https://github.com/mgechev/revive/commit/1de8243783d480e24c0db1a3dc45976aeaf715e9)
provides a second Go fixture corpus. Clippy has no receiver-name analogue because
Rust methods use the language-defined `self` receiver.

## Precision, Policy, And Fixes

Methods are grouped by `go/types` named-type identity across canonical package
files. The most frequent nonblank name is canonical; ties use the earliest
declaration. Diagnostics identify each minority receiver name and relate it to
the canonical declaration. Unnamed and blank receivers do not participate.

Generated and ill-typed packages are excluded. The minimum source version is Go
1.25. Exact suppressions and baselines use `inconsistent-receiver-name` and
`receiver-name`.

No fix is registered because renaming a receiver also requires exact edits for
all object-bound references in the method body while preserving comments.

## Admission Evidence

The focused test first failed because the rule was absent. Fixtures cover
majority and stable receiver names, exact primary and related ranges, metadata,
generated and type-error policy, source versions, suppressions, deterministic
baselines, JSON, explain, and absence of fixes.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured a
median of `23,964,742 ns/op`, `199,560 B/op`, and `1,418 allocs/op`.

Non-mutating pedantic dogfood produced no findings for this rule in Glippy or
`go-libraries/pkg/prompts` at
`6ed3a06a4e1aba412d2a6b91454774234f30a464`.

## Revisit Trigger

Consider an object-aware suggestion fix only after all receiver references and
comment boundaries can be proven in one source-versioned transaction. Revisit
the majority rule if real packages demonstrate legitimate per-build-file naming
patterns.
