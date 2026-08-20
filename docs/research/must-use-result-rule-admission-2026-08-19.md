# `must-use-result` Rule Admission, 2026-08-19

## Decision

Admit `must-use-result` as a default-correctness control-flow rule. It consumes
the exact `must-use` result indexes already validated and cached by project
semantic contracts. A call statement, `go` or `defer` call, or blank assignment
destination reports when it discards one or more contracted results. One call
produces one diagnostic listing every ignored contracted index. Direct
non-blank assignment, return, and argument use count as consumption.

The rule has no fix. Choosing propagation, recovery, storage, or explicit
policy removal depends on the API contract and caller intent.

## Defect And Current References

Go permits calls with discarded results and permits `_` to discard selected
tuple components. The compiler and default vet catalog have no Go annotation
or export-data contract that lets an application mark one exact result as
required. Glippy's existing schema accepted, validated, cached, and exposed
`must-use` facts but no built-in rule made those facts observable, leaving the
published semantic contract incomplete.

Current source reviewed on 2026-08-19:

- Rust Clippy commit
  [`a184fd6db865e41fb9f08ddf4205f992d67a93ef`](https://github.com/rust-lang/rust-clippy/commit/a184fd6db865e41fb9f08ddf4205f992d67a93ef),
  especially `clippy_lints/src/let_underscore.rs`. Its
  `LET_UNDERSCORE_MUST_USE` restriction lint reports `_` bindings of
  `#[must_use]` types and function results, complementing Rust's compiler-owned
  unused-must-use behavior.
- PHPStan source commit
  [`8f7898505c3e60d996e370c109d6bb47fe67a2a9`](https://github.com/phpstan/phpstan-src/commit/8f7898505c3e60d996e370c109d6bb47fe67a2a9),
  especially `CallToFunctionStatementWithNoDiscardRule.php`,
  `CallToMethodStatementWithNoDiscardRule.php`, and the reflection
  `mustUseReturnValue` contract. PHPStan carries required-return metadata into
  statement-call diagnostics for functions and methods.

Glippy follows the shared product direction without copying either frontend.
Its exact package-qualified contract can select individual Go tuple results,
including external functions and methods resolved from export types.

## Precision And Interaction Boundary

Only a statically resolved call with a validated contract can report. Function
values, interface dispatch, dynamic calls, unconfigured APIs, generated files,
and ill-typed packages remain excluded. A result used through a non-blank
destination is considered consumed; this rule does not attempt to prove the
later use meaningful. Contract authors remain responsible for the asserted
runtime policy.

The opt-in `discarded-error` rule can identify the same expression statement
for a contracted error result. When both rules are selected and produce the
same exact source identity and call range, `must-use-result` owns the condition
and supersedes the generic diagnostic before suppression or baseline policy.

## Behavioral Evidence

Focused package tests cover complete and partial tuple discards, expression,
blank, `go`, `defer`, assignment, return, and argument forms; exact ranges and
messages; default metadata; missing contracts; function values; generated and
ill-typed packages; suppressions; specific-versus-generic diagnostic
ownership; and an external dependency contract whose source is not a lint
target. The initial test failed because the compiled registry did not contain
`must-use-result`. A second red test reproduced duplicate visible diagnostics
before contract-specific diagnostic ownership was added.

## Cost

Five complete one-iteration package-analysis samples over 100 contracted calls
on Go 1.26.6, Darwin arm64, and an Apple M4 Max measured a 39.77 ms median,
1,490,264 B/op, and 11,868 allocs/op. The observed range was 37.41-44.22 ms.
This remains a directional development measurement rather than a portable
release budget.

Non-mutating exact-rule dogfood completed without findings on Glippy and on
`go-libraries/pkg/prompts` at
`29e46a9c64a888c670afb93e45a2cf26c23e0318`. Both runs used one freshly built
working-tree binary and a task-owned, prepopulated module cache with ordinary
analysis network access disabled. The prompts repository's pre-existing dirty
paths were unchanged, including `go.sum` digest
`5682cca758f3e1cc9931b18bccfe7111dc56825dc8e8a4549e4171a0ca46db4b`.

## Revisit Trigger

Revisit consumption when real defects require tracking a contracted result
through local aliases or when an API needs a richer relationship than an exact
required result index. Do not infer must-use policy from naming conventions or
third-party function bodies without an explicit admission contract.
