# Standard-Library No-Return Foundation, 2026-08-15

## Decision

The shared CFG and SSA no-return predicate now recognizes a bounded set of
exact standard-library terminal functions in addition to the predeclared
`panic` and package-local summaries:

- `os.Exit`, `runtime.Goexit`, and `syscall.Exit`;
- package-level and `*log.Logger` `Fatal`, `Fatalf`, `Fatalln`, `Panic`,
  `Panicf`, and `Panicln`; and
- `testing`'s receiver methods `FailNow`, `Fatal`, `Fatalf`, `SkipNow`, `Skip`,
  and `Skipf`.

The predicate uses `go/types` package and function identity, so import aliases
do not affect recognition. It does not match source spellings, package names,
arbitrary `Fatal` conventions, function values, interface dispatch, or nearby
nonterminal APIs.

## Authority And Scope

Go 1.26.6 source and API documentation define every admitted call as terminal:
`os.Exit` terminates the program immediately; `runtime.Goexit` terminates the
calling goroutine; `syscall.Exit` is provided by the runtime; log fatal methods
call `os.Exit`, log panic methods call `panic`; and the selected testing methods
end in `FailNow`, `SkipNow`, or `runtime.Goexit`. The same public contracts are
present throughout Glippy's supported Go 1.25 and 1.26 source range.

Only macOS and Linux are supported runtime targets, so platform-specific
Windows syscall functions are not modeled. Calls launched with `go` or
registered with `defer` do not make the containing function terminal at the
call site.

This is not general cross-package effect inference. Project and third-party
helpers without loaded source or analyzer facts remain conservatively
returning. Dependency syntax is not loaded merely to recognize these exact
standard-library objects.

## Product Effect

All native CFG rules receive graphs with impossible continuations removed, and
the same immutable predicate is installed before the shared SSA program is
built. A branch ending in `os.Exit` therefore no longer creates a false
`context-cancel-leak` path, and `nilness` can use the resulting impossible
continuation. Stream and lifecycle CFG consumers inherit the same semantics.

No diagnostic, configuration key, suppression identity, or fix is introduced.
The effect is part of the shared analysis contract and remains demand-driven by
an enabled CFG or SSA rule.

## Evidence And Cost

The focused CFG regression initially left every target reachable after exact
standard-library terminal calls. The context-cancellation integration reported
a false leak, and the SSA nilness integration missed an impossible condition.
All three failed before the predicate extension and pass after it.

Fixtures cover every admitted log and testing name, package-level and logger
methods, import aliases, `os`, `runtime`, and `syscall`, nearby nonterminal
calls, local lookalikes, dynamic calls, goroutine launches, deferred calls, CFG
and SSA consumers, and the one-source dependency-syntax bound.

Five 100-iteration probes over 100 direct `os.Exit` functions measured a 47.5
microsecond median, about 52.9 KiB, and 1,051 allocations per shared CFG run on
Go 1.26.6, Darwin arm64, Apple M4 Max. Package loading is excluded; this is a
proportional analysis-layer probe rather than a stable repository budget.

Non-mutating exact-rule dogfood for `context-cancel-leak`, `nilness`,
`unchecked-rows-error`, and `unchecked-scanner-error` completed without
findings or tool failures over Glippy at `f2bd775` plus this batch and
`go-libraries/pkg/prompts` at `e38bab8`. Neither repository was modified.

## Revisit Triggers

Add another standard-library terminal only from supported-version API and
source evidence. Add project or third-party effects only through a
dependency-first, cache-safe fact contract that preserves cold/warm diagnostic
identity and does not force dependency syntax into syntax-only or unrelated
typed runs.
