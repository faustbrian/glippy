# Complexity Preset Admission, 2026-08-14

## Decision

The first populated `complexity` preset admits four syntax rules:

| Rule | Default maximum | Scope |
| --- | ---: | --- |
| `excessive-nesting` | 5 | Structural control-flow nesting per function |
| `too-many-lines` | 100 | Physical lines containing lexical Go tokens per function |
| `too-many-parameters` | 7 | Value parameters, excluding a method receiver |
| `too-many-results` | 3 | Named or unnamed result values, including `error` |

All four are warning-level, opt-in, fix-free, and excluded from generated files
and `_test.go` files by default. Each owns a bounded integer `maximum` option
and an `include-tests` boolean. Configured thresholds are validated before
source discovery and contribute to the existing canonical option and cache
identity.

## Current Clippy Evidence

Current Rust Clippy source was reviewed at
[`e52501913b75235e3d41422566a2d05d6f00b699`](https://github.com/rust-lang/rust-clippy/tree/e52501913b75235e3d41422566a2d05d6f00b699).
Its `too_many_arguments`, `too_many_lines`, and `excessive_nesting` lints
establish comparable configurable policy surfaces. Go-specific differences are
deliberate: receivers do not count as parameters, result arity is independently
useful in Go, lexical token lines avoid counting comments as code, and nested
function literals are analyzed independently.

Clippy's current `cognitive_complexity` documentation explicitly states that it
does not measure true cognitive complexity, keeps the lint in `restriction`,
and recommends `excessive_nesting` and `too_many_lines` instead. Glippy therefore
does not introduce a misleading cognitive-complexity score into its selectable
complexity group.

## Precision Boundary

The rules report objective threshold crossings rather than claiming that a
specific refactor is correct. Else-if chains do not gain an artificial level
from their AST nesting. Switch, type-switch, and select clauses inherit one
level from their owning construct. Body-based rules ignore bodyless
declarations. Closures do not add nesting or body lines to an enclosing
function and are analyzed as their own functions.

No fix is offered. Splitting a function, grouping parameters, or replacing
multiple results with a struct changes API, ownership, or readability contracts
that cannot be proven from local syntax.

## Calibration And Cost

Non-writing default-threshold dogfood over the current Glippy tree produced 61
opt-in findings: 50 `too-many-lines`, five `too-many-parameters`, and six
`too-many-results`. No function crossed the nesting default. The findings
identify real size or API facts, but remain advisory because several large
engine and protocol functions intentionally centralize state transitions.

The same run over `go-libraries/pkg/prompts` at
`0b9bb08727cc1cabdc674bbfe7082fc5642c3f2a` produced five findings: three
function-length crossings, one twenty-parameter internal constructor, and one
four-result decoder helper. Inspection found no incorrectly counted receiver,
comment-only line, closure body, or result field. Neither repository was
modified.

Five 100-iteration shared syntax benchmark samples on Go 1.26.6, Darwin arm64,
and an Apple M4 Max measured `20,378` to `52,102 ns/op` with a `28,256 ns/op`
median, `32,648` to `32,652 B/op`, and 167 allocations per file for the complete
preset. This is proportional admission evidence, not a stable
repository-latency budget.

## Revisit Triggers

Revisit defaults after more Go repositories supply measured distributions.
Revisit the token-line definition if multiline literals or directive-only lines
produce misleading findings. A cognitive metric remains deferred unless a new
model has a defensible observable contract beyond a weighted syntax score.
