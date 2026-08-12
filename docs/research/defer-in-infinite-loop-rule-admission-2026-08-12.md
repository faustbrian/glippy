# `defer-in-infinite-loop` Rule Admission, 2026-08-12

## Defect And Existing-Tool Boundary

A defer statement schedules its call for function return or panic unwinding,
not for the end of the current loop iteration. Repeatedly executing a defer in
a function that cannot exit retains the deferred arguments and call until the
function ends, so the intended cleanup never occurs and resource use can grow.

Go 1.26.5 `go vet ./...` exited successfully for a disposable module containing
a defer in a conditionless loop whose only `break` exits a nested switch. The
default compiler and vet toolchain therefore do not diagnose this defect.

Current Staticcheck defines the related SA5003 rule at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e):

- rule source SHA-256:
  `ddbc5cf48b4f4f5a1acbcc584a7d11ec2016122555a015e3bd986ec5fc3a7c5c`;
- fixture SHA-256:
  `fa1a454ebbb4b14810168fee4d5374cb0c4fe2d0811bbb5011e5d60cc7a7465f`.

SA5003 uses a syntax walk and suppresses every loop diagnostic after any
`break`. Its source records that a break belonging to a nested switch or select
causes a false negative. Gox uses the already shared function CFG to retain the
actual branch target instead of adding a second syntax approximation.

A reviewed public repair demonstrates the defect in ordinary code. The
[`apimgr/ipgaze` fix
`052aa2112a0bf654bddb6161b39110a80c7c59ad`](https://github.com/apimgr/ipgaze/commit/052aa2112a0bf654bddb6161b39110a80c7c59ad)
replaced `defer cancel()` inside an infinite shutdown loop with an explicit
`cancel()` after the operation that used the context.

## Contract And CFG Boundary

`defer-in-infinite-loop` is a warning in the opt-in `suspicious` preset. It
requires the control-flow tier, excludes generated and ill-typed packages, and
offers no fix. The exact `defer` keyword is the primary range.

For each function declaration or literal, the rule identifies defer statements
lexically enclosed by a conditionless `for` loop while excluding nested
function bodies. It then finds the live CFG block containing each defer and
reports only when no successor path can reach:

- an explicit or synthetic function return;
- the predeclared `panic` function; or
- `runtime.Goexit`, which terminates the goroutine after running deferred calls.

The reachability calculation uses successor and predecessor edges rather than
the unspecified `cfg.Blocks` order. Nested conditionless loops cannot duplicate
a finding. Unreachable defers do not report. A break or goto that can leave the
loop, a reachable return, panic, or Goexit prevents a diagnostic; a break that
only exits a nested switch or select does not.

The initial contract deliberately recognizes only conditionless loops. A loop
written as `for true` and semantically infinite loops with non-constant exit
conditions remain unreported. Interprocedural calls that panic, call Goexit, or
terminate the process are not modeled. CFG feasibility is conservative, so a
syntactically reachable function-exit path prevents a report even when runtime
values may make that path impossible.

Moving cleanup outside the loop or calling it explicitly can change timing,
error handling, and resource ownership. No generally safe, suggestion, or
unsafe automatic fix is registered.

## Behavioral Evidence

The initial focused package test observed zero diagnostics because the rule was
absent from the production registry. After registration, focused rule and CLI
tests prove:

- exact diagnostics for direct conditionless loops and loops whose apparent
  break belongs only to a nested switch;
- no diagnostic when a real loop break, goto, return, built-in panic, or
  `runtime.Goexit` can execute the defer;
- predeclared panic recognition by object identity, including parentheses,
  without mistaking a shadowed function for panic;
- `runtime.Goexit` recognition through selectors and dot-imported identifiers;
- one finding for a defer inside nested conditionless loops;
- exclusion of unreachable defers, nested function bodies, conditioned loops,
  generated files, and ill-typed packages;
- suppression and severity behavior;
- canonical opt-in registry ordering and no-fix metadata; and
- non-mutating public `gox lint` diagnostics and canonical `gox explain`
  output.

## Cost And Dogfood Signal

Five 500-millisecond, single-CPU benchmark samples on Darwin arm64 with an
Apple M4 Max and Go 1.26.5 reused one typed package containing 100 functions.
The callback baseline and rule both received the shared function CFGs:

| Run | Median | Bytes per operation | Allocations per operation |
| --- | ---: | ---: | ---: |
| No-op CFG callback | 39.317 us | 59,224 | 1,130 |
| CFG plus `defer-in-infinite-loop` | 103.747 us | 178,200 | 2,147 |

The delta measures rule execution and diagnostic construction over preloaded
typed state; it excludes package-loading latency and is not a stable budget.

Two explicit non-mutating runs enabled only this suspicious rule:

| Corpus | Revision or state | Selected files | Diagnostics |
| --- | --- | ---: | ---: |
| Gox | implementation worktree | 111 | 0 |
| go-libraries `pkg/clock` | `0223e6490dd696a6242c490d458f2ee9c371faa8` | 18 | 0 |

The external run used a task-owned immutable Git archive. Zero findings show no
observed false-positive noise in these 129 files; they do not prove recall.
Focused positive fixtures and the reviewed public repair provide positive
evidence.

## Admission Decision

Admit `defer-in-infinite-loop` as the first built-in CFG-tier rule under the
opt-in `suspicious` preset. This exercises the shared CFG scheduler without
raising the default correctness preset's loading tier. Revisit condition
reasoning and recognized terminating calls only when missed defects justify a
more expensive representation or a proven standard-library no-return model.
Revisit default membership after broader dogfood establishes signal and CFG
latency budgets.
