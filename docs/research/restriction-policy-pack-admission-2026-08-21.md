# Restriction Policy Pack Admission, 2026-08-21

## Decision

Admit four independently selectable restriction rules:

- `direct-panic` reports the exact predeclared `panic` function;
- `process-exit` reports exact `os.Exit` plus `log.Fatal`, `Fatalf`, and
  `Fatalln` package and `*log.Logger` method calls;
- `context-background` reports exact `context.Background` calls; and
- `context-todo` reports exact `context.TODO` calls.

The rules run at the types tier, exclude test files by default through their
own `include-tests` options, exclude generated and ill-typed packages, and
offer no fixes. The canonical catalog now contains 117 rules, including five
restriction rules. The restriction group remains invalid as a wholesale
selection.

## Clippy And Go Boundary

Current Clippy source at
[`e8bea0328512886cca6e0c28e28f3e172689b484`](https://github.com/rust-lang/rust-clippy/tree/e8bea0328512886cca6e0c28e28f3e172689b484)
retains `panic` and `exit` as restriction lints and explicitly warns users not
to enable the complete restriction group. Its `panic` rule can exclude tests;
its `exit` rule preserves the executable entry point. Glippy follows the same
individually selected policy model but uses Go-specific boundaries instead of
copying Rust syntax or entry-point inference.

The Go 1.26.7 standard-library contracts establish the relevant effects:

- `os.Exit` terminates immediately and does not run deferred functions;
- `log.Fatal` and its format and line variants print and call `os.Exit(1)`;
- `context.Background` is never canceled and has no deadline or values; and
- `context.TODO` marks a context that is unclear or not yet available.

The compiler and default vet correctly accept all four behaviors because each
can be legitimate. These rules therefore encode organizational library and
lifetime policy, not correctness claims. Unlike Clippy's `exit`, Glippy does
not silently exempt a function named `main`: projects that permit termination
only at an executable boundary can scope an override by path or attach a
reasoned suppression at the exact call. This keeps library packages, helper
commands, alternate entry points, and generated command wrappers under one
auditable configuration contract.

## Precision And Configuration

All matching uses `go/types` object identity. Import aliases are recognized,
while shadowed `panic` identifiers, same-named methods, third-party loggers,
function variables, and wrappers remain outside the rules. `process-exit`
includes both package functions and methods on the exact standard-library
`log.Logger` receiver because both have the same termination effect.

`context-background` and `context-todo` remain separate identities. A service
may forbid unresolved placeholders while permitting deliberate process roots,
or may audit detached roots without treating compatibility placeholders the
same way. Combining both calls under one rule would force unrelated exceptions
to share configuration and suppressions.

Each finding covers the called function expression. No fix is available:
return types, error propagation, logging ownership, process exit codes,
context parameters, and detached-work lifetime are API decisions that cannot
be selected safely from one call site.

Example configuration:

```toml
[lint.rules]
direct-panic = "warn"
process-exit = "warn"
context-background = "warn"
context-todo = "warn"

[lint.rule-options."direct-panic"]
include-tests = false
```

Path overrides can permit executable entry points without weakening library
policy. The existing narrow suppression contract owns deliberate local
exceptions and requires a reason when the repository enables that policy.

## Behavioral, Cost, And Dogfood Evidence

The focused suite began red because all four exact IDs were absent. Current
fixtures cover import aliases, package functions, logger methods, shadowed
builtins, same-named package-local methods, exact ranges, suppressions,
generated files, supported source versions, per-rule metadata, wholesale-group
rejection, default test exclusion, test opt-in, and absence of fixes.

Five five-iteration package-analysis samples on Go 1.26.7, Darwin arm64, and an
Apple M4 Max produced a median of `66,275,900 ns/op`, `2,357,884 B/op`, and
`17,961 allocs/op` for all four enabled rules and five findings. Standard
library import and package loading dominate this proportional admission probe;
it is not a release latency budget.

Non-mutating exact-rule dogfood found 32 policy sites in Glippy: 20 direct
panics used for internal invariants, nine process exits in commands and
benchmark tooling, and three root background contexts. The result demonstrates
why the rules are restrictions rather than default diagnostics. The approved
`go-libraries/pkg/prompts` target at
`ee8dfbbb938d4a03e6b48c6c6772423457b94ef1` had no findings. Both worktree
status digests remained unchanged.

## Revisit Triggers

Revisit optional allowlists only when real repositories show a stable boundary
that path overrides and suppressions cannot express. Revisit wrapper detection
through project semantic contracts rather than name matching. Do not move any
rule into a selectable preset without broad evidence that its organizational
policy is universally desirable.
