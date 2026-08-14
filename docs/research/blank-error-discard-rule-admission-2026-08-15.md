# `blank-error-discard` Rule Admission, 2026-08-15

## Decision

Admit `blank-error-discard` as Glippy's first `restriction` rule. It reports
error-typed assignment results bound to the blank identifier and can be enabled
only through its exact rule ID. The rule runs at the types tier, excludes test
files by default with an `include-tests` opt-in, excludes generated and
ill-typed files, and offers no fix.

## Clippy And Go Boundary

Current Clippy source at
[`e52501913b75235e3d41422566a2d05d6f00b699`](https://github.com/rust-lang/rust-clippy/tree/e52501913b75235e3d41422566a2d05d6f00b699)
defines `let_underscore_must_use` as a restriction lint: projects can require
values whose type carries a use obligation to be consumed instead of hidden
behind `let _ = expression`. Go has no general `must_use` annotation, but the
predeclared `error` interface is the language's explicit failure channel and
provides a narrow typed analogue.

The Go compiler accepts `_ = fail()` and tuple assignments such as
`value, _ := operation()`. Go 1.26.6 default vet has no general rule for blank
error assignments. Its `unusedresult` analyzer covers a configured call list,
not error-typed tuple positions. Staticcheck SA4006 deliberately skips blank
identifier targets, so this exact policy is not duplicated by its unread-value
analysis.

`discarded-error` remains the suspicious rule for bare call statements. It
continues to treat an explicit blank assignment as an intentional choice.
`blank-error-discard` is a separate restriction identity because enabling it
changes that project policy, should not add default noise, and needs independent
suppressions and test-file configuration.

## Precision And Configuration

The rule uses `go/types` assignment and tuple types to identify the exact blank
position receiving a value assignable to `error`. It handles single values,
multi-result calls, short declarations, ordinary assignments, and multiple
single-valued right-hand expressions. Non-error blanks, nonblank error targets,
and bare call statements do not report. Each finding covers only the offending
blank identifier, so suppressions and changed-line filtering own the exact
policy choice.

Formatted-output helpers and documented always-nil in-memory writer methods
excluded by `discarded-error` retain the same product boundary. Test files are
excluded by default because fixtures, fuzzers, and benchmarks intentionally
discard many failures; `include-tests = true` applies the policy there when a
repository wants it. Generated files and packages with type errors remain
excluded. No fix is available because propagation, aggregation, logging, and
best-effort behavior are caller decisions.

Configuration is explicit:

```toml
[lint.rules]
blank-error-discard = "warn"

[lint.rule-options."blank-error-discard"]
include-tests = false
```

The `restriction` group itself remains invalid in presets and lint-level group
directives; this rule must be selected by exact ID.

## Behavioral, Cost, And Dogfood Evidence

The focused suite began red with `unknown rule "blank-error-discard"`. A second
red step established the test-file option through the rejected unknown option.
The current fixtures cover single and tuple errors, short and ordinary
assignment, multiple right-hand expressions, direct error values, nearby
non-error and handled cases, bare call ownership, documented infallible writes,
exact ranges, suppressions, generated and ill-typed packages, supported source
versions, restriction selection, test-file policy, and absence of fixes.

Five one-iteration package-analysis samples on Go 1.26.6, Darwin arm64, and an
Apple M4 Max produced a median of `38,961,916 ns/op`, `199,544 B/op`, and
`1,243 allocs/op`. Package loading dominates this proportional admission probe;
it is not a release latency budget.

Non-mutating exact-rule dogfood found 29 production-source policy sites in
Glippy and seven in `go-libraries/pkg/prompts` at
`0b9bb08727cc1cabdc674bbfe7082fc5642c3f2a`. In prompts, three findings are
deliberate redaction-format writes whose interface cannot return an error, while
four input mutation or replay paths warrant explicit handling or a reasoned
suppression. The repository's pre-existing dirty state and revision were
unchanged. These findings confirm why the rule belongs to restriction rather
than a default preset and provide concrete evidence for the test-file option.

## Revisit Trigger

Revisit the standard-library exclusion list when an excluded operation can
produce an actionable failure in practice. Revisit preset membership only if
large-repository evidence establishes near-zero noise, which is not expected
for an organizational restriction rule.
