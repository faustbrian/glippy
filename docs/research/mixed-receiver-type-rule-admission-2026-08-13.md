# `mixed-receiver-type` Rule Admission, 2026-08-13

## Decision

Admit `mixed-receiver-type` to the opt-in `pedantic` preset at warning severity.
The package-wide types rule identifies minority pointer or value receiver forms
for methods on one named type.

## Defect And Existing Tools

Mixing receiver forms changes method sets and can obscure whether a type is
copied or mutated. The Go Code Review Comments conclude: "Don't mix receiver
types. Choose either pointers or struct types for all available methods." This
is strong repeated-review guidance, but valid exceptions exist. The compiler
and Go 1.26.5 default vet catalog do not enforce general consistency; `copylocks`
only catches dangerous copies involving locks.

Current Staticcheck and Revive catalogs have no equivalent general
pointer/value consistency rule. Revive's `modifies-value-receiver` addresses a
different concrete mutation error. Clippy has no direct analogue because Rust
receiver borrowing is expressed per method and follows different ownership
semantics.

## Precision, Policy, And Fixes

Methods are grouped by `go/types` named-type identity across canonical package
files. The majority receiver form is canonical; ties use the earliest
declaration. Diagnostics identify each minority receiver type and relate it to
the canonical form.

Generated and ill-typed packages are excluded. The minimum source version is Go
1.25. Exact suppressions and baselines use `mixed-receiver-type` and
`receiver-form`.

No fix is registered. Changing receiver form can alter copying, mutation,
method sets, interface satisfaction, addressability, and performance.

## Admission Evidence

The focused test first failed because the rule was absent. Fixtures cover
majority and stable forms, exact primary and related ranges, metadata, generated
and type-error policy, source versions, suppressions, deterministic baselines,
JSON, explain, and no-fix behavior.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured a
median of `24,618,450 ns/op`, `199,472 B/op`, and `1,417 allocs/op`.

Non-mutating pedantic dogfood found one intentional exception in
`go-libraries/pkg/prompts` at
`6ed3a06a4e1aba412d2a6b91454774234f30a464`: `InputEvent` uses value receivers
for redacting inspection methods and a pointer receiver for destructive
zeroization. No Glippy finding was reported. The external repository was not
modified.

## Revisit Trigger

Revisit precision if deliberate mutator-pointer plus observer-value designs are
common enough to make the opt-in preset noisy. A future refinement may classify
obvious mutators separately, but it must not infer semantics from method names.
