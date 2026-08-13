# Self-assignment rule admission, 2026-08-13

## Decision

Admit `self-assignment` to the `correctness` preset at warning severity by
adapting `golang.org/x/tools/go/analysis/passes/assign`. The rule requires the
types tier, runs once per typed package, excludes generated files and packages
with type errors, and supports Go 1.25 and Go 1.26 source.

Glippy does not copy the analyzer. `internal/rulecatalog` is the product
composition boundary between the native registry and the existing
`go/analysis` adapter, avoiding a dependency cycle between `internal/rules`
and `internal/analysis`.

Because `correctness` is the default group, admitting this types-tier rule
raises the default lint plan from syntax to types. Standalone files outside a
module, workspace, or repository root must either be placed in a package-aware
project or disable `self-assignment` explicitly to retain syntax-only linting.
This is a deliberate v0.2 compatibility change rather than hidden speculative
loading; disabling the rule restores the previous syntax-only plan.

## Observable defect and overlap

`value = value` is ineffective and commonly indicates that either the target
or source expression was copied incorrectly. The Go compiler accepts it and
cannot diagnose intent. The Go 1.26.5 toolchain's default vet catalog includes
the `assign` analyzer, and x/tools v0.48.0 reports only ordinary `=` assignments
whose paired expressions are structurally equal and proven free of relevant
effects. It deliberately excludes map indexes and expressions with possible
effects.

The current Staticcheck catalog independently provides SA4018,
"Self-assignment of variables", in its ineffective-code family. Current Clippy
master provides `SELF_ASSIGNMENT` in the `correctness` group and compares the
value expressions before reporting. These independent defaults support
correctness placement; Glippy's value is cohesive selection, deterministic
reporting, suppressions, baselines, and fix coordination, not a new detection
algorithm.

The upstream Go analyzer fixtures include mistaken local and field assignments,
slice and array indexes, mixed multi-assignments, and exclusions for map indexes
and effectful index expressions. The fixture comment `x = x` where `s.x = x`
was intended is a concrete copied-target defect representative of code review
findings.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `go/analysis/passes/assign/assign.go` and its fixtures;
- <https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/assign>;
- <https://staticcheck.dev/docs/checks/#SA4018>;
- current Clippy `clippy_lints/src/operators/self_assignment.rs` and the
  `SELF_ASSIGNMENT` declaration in `operators/mod.rs`.

## Precision and ranges

The adapted analyzer reports the complete assignment statement. Glippy maps
that token position to the exact physical byte range and renders the beginning
of `value = value` at the verified line and column. It does not report `:=`,
cardinality-mismatched assignments, effectful expressions, or map indexes.

The analyzer does not opt into ill-typed packages, so Glippy skips it when type
loading reports errors. Generated files are excluded by Glippy metadata before
their diagnostics can become visible. Standard source suppressions and
PHPStan-style baselines operate on the adapted diagnostic exactly as they do
for native rules.

## Fix classification

The upstream suggestion named `Remove self-assignment` is exposed as
`remove-self-assignment` and classified as **suggestion**, not safe. Deleting
the ineffective assignment preserves expression behavior under the upstream
analyzer's no-effects contract, but the statement may be an intentional marker
or may reveal that a different assignment was intended. Developer review is
therefore required.

The fix coordinator still verifies source identity, applies the exact byte
edit, reparses, formats, validates, and writes atomically. A focused integration
test proves the suggestion is not applied by ordinary linting, is applied only
with `--fix-suggestions`, and is idempotent. Explicit-semicolon source can leave
an empty statement after the upstream deletion; this is documented in canonical
metadata and is another reason not to classify the edit as safe.

## Behavioral and policy evidence

Focused tests cover:

- positive detection and the exact human line and column;
- a nearby ordinary assignment that must not diagnose and non-mutating lint mode;
- canonical metadata and `glippy explain` output;
- generated-file and type-error exclusion;
- exact-rule suppression ownership;
- deterministic baseline generation and application; and
- suggestion-only application, final formatting, and second-run idempotency.

## Performance

Five 200 ms samples on Darwin arm64, Apple M4 Max, and Go 1.26.5 compared the
same one-package typed load with a no-op package rule against the adapted rule:

| Case | ns/op samples | bytes/op range | allocations/op range |
| --- | --- | ---: | ---: |
| typed baseline | 28.44M, 28.46M, 28.66M, 32.92M, 34.31M | 169,424-170,118 | 1,164-1,167 |
| with `self-assignment` | 27.67M, 28.03M, 29.52M, 29.72M, 33.01M | 184,154-190,302 | 1,224-1,231 |

Package loading variance dominates latency. The observed incremental allocation
cost is approximately 15-21 KiB and 57-67 allocations for this fixture. The
analyzer reuses Glippy's typed package load; its `inspect` prerequisite is
bounded to the analyzer graph.

## Default-enablement gate

The rule remains in `correctness` because both the standard Go vet catalog and
Clippy treat the pattern as a correctness defect and the analyzer excludes
effectful equality.

The locally built Glippy binary completed non-mutating correctness lint over
both `./...` in this repository and `/Users/brian/Developer/go-libraries/pkg/prompts`
with exit 0 and no diagnostics. The prompts run used a task-owned module cache
preloaded through the selected workspace dependency graph because ordinary
Glippy analysis intentionally disables module lookup. The external repository's
pre-existing dirty status was unchanged. There were no self-assignment findings
to classify as true positives, false positives, or intentional exceptions.
