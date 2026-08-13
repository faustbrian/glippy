# `unnecessary-conversion` Rule Admission, 2026-08-13

## Decision

Admit `unnecessary-conversion` to the opt-in `pedantic` preset at warning
severity. The types-tier rule reports a one-argument conversion only when the
source and target types are identical and the operand is not a constant.

## Defect And Existing Tools

Converting a runtime value to the exact type it already has cannot change its
representation or method set. Such conversions commonly survive refactors and
obscure the remaining type boundaries. The compiler and Go 1.26.5 default vet
catalog accept them.

The Go-specific external authority is `unconvert`, inspected at
[`4a038b3d31f56ff5ba511953b745c80a2317e4ae`](https://github.com/mdempsky/unconvert/commit/4a038b3d31f56ff5ba511953b745c80a2317e4ae).
Current Staticcheck has no matching identity-conversion check. Clippy's
`useless_conversion` at
[`d2c4d1532d89488a56ec2c3ca12757117fc0b4e2`](https://github.com/rust-lang/rust-clippy/commit/d2c4d1532d89488a56ec2c3ca12757117fc0b4e2)
checks same-type conversion traits and provides the closest catalog analogue.
Revive has no equivalent rule in its current catalog.

## Precision, Policy, And Fixes

`go/types` must identify the call target as a type and prove the operand and
target types identical. Distinct defined types, merely identical underlying
types, ordinary calls, and all compile-time constants are excluded. Constants
remain explicit because a conversion can establish their intended type even
when contextual type information later makes the result look identical.

Generated and ill-typed packages are excluded. The minimum source version is Go
1.25. Exact suppressions and baselines use `unnecessary-conversion` and
`identity-conversion`.

No fix is registered until removing delimiters has dedicated precedence and
comment-preservation evidence.

## Admission Evidence

The focused test first failed because the rule was absent. Fixtures cover basic,
defined, slice, distinct-type, constant, and ordinary-call cases; exact ranges;
metadata; generated and type-error policy; source versions; suppressions;
deterministic baselines; JSON and explain output; and no-fix behavior.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured a
median of `25,578,375 ns/op`, `174,993 B/op`, and `1,152 allocs/op`. Package
loading dominates this proportional rule measurement.

Non-mutating pedantic dogfood produced no findings for this rule in Glippy or
`go-libraries/pkg/prompts` at
`6ed3a06a4e1aba412d2a6b91454774234f30a464`.

## Revisit Trigger

Revisit constant handling only with a source-level proof that removing an
explicit conversion does not erase useful API intent. Add a fix only after
precedence and interior-comment ownership are proven.
