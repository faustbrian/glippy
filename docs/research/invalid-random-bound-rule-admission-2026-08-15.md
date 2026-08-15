# Invalid Random Bound Rule Admission, 2026-08-15

## Decision

Admit `invalid-random-bound` as a default correctness types-tier rule. It
reports direct calls to exact bounded functions and `Rand` methods from
`math/rand` and `math/rand/v2` when the bound is a compile-time integer less
than or equal to one:

- a nonpositive bound panics; and
- a bound of one always returns zero because the result interval is `[0, 1)`.

No fix is offered. The intended upper bound and result domain cannot be inferred
from the call alone.

## Defect Evidence

The Go 1.25.12 and Go 1.26.6 standard-library implementations and API
documentation define bounded random results over the half-open interval
`[0, n)` and panic for zero or negative signed bounds. Staticcheck SA4030 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` independently reports the
constant-result case.

Two public examples demonstrate the observable defect:

- `nicholasjackson/building-microservices-youtube` revision
  `0cd586295712a80654ac94f929444b106423c1f0` uses `rand.Intn(1)` to choose
  whether a currency rate moves up or down, so only the zero branch can run;
  and
- `elazarl/goproxy` revision
  `6225cd309d7c0e201659f1e00086b12fdae62df2` uses
  `rand.Intn(1) == 0` as a random request condition, so the condition is always
  true.

The Go 1.26.6 compiler and default `go vet` accepted a Go 1.25 module containing
`rand.Intn(1)` without a diagnostic. Glippy therefore adds a defect contract
that the default toolchain does not own.

## Precision Contract

The rule uses `go/types` package, function, and receiver identity. It covers
the supported package functions and `*Rand` methods:

- `math/rand`: `Intn`, `Int31n`, and `Int63n`; and
- `math/rand/v2`: `N`, `IntN`, `Int32N`, `Int64N`, `UintN`, `Uint32N`, and
  `Uint64N`, with the method subset defined by the standard `Rand` type.

Import aliases do not affect recognition. Function values, interface dispatch,
local lookalikes, nonconstant values, bounds greater than one, generated files,
and packages with type errors remain excluded. Named constants and constant
expressions are included because their exact value is available without value
flow. Intentional panic or deterministic-path tests can use an exact
suppression; they do not weaken the production correctness contract.

The rule supports Go 1.25 and Go 1.26 source and requires only the shared types
tier. It introduces no configuration option, source edit, or additional
analysis representation.

## Evidence And Cost

The product-level regression first failed because the rule ID was absent. The
implemented tests cover all 20 admitted package-function and method forms,
including explicit generic instantiation of `rand/v2.N`,
nonpositive signed and unsigned bounds, named constants, exact diagnostic
ranges and messages, local lookalikes, function values, unknown bounds, values
greater than one, metadata, and fix absence. The close-negative test also
detected an implementation that accidentally reported every positive constant
instead of only one.

Five one-iteration cold package probes over 100 findings on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured a median of 80.2 ms, about 2.11 MB, and
17,718 allocations. Package loading dominates this proportional admission
probe; the rule itself performs one filtered call traversal and constant lookup.

Non-mutating exact-rule dogfood completed without findings or tool failures on
Glippy and `go-libraries/pkg/prompts` at
`e38bab8527e9ec97f668b262b23c70660cac0378`. The prompts worktree was unchanged.

## Revisit Triggers

Add another random API only from exact supported-version documentation and
object identity. Consider value-flow bounds only if real defects justify the
additional tier or cost. Revisit fixability only when a call site supplies a
machine-checkable intended domain rather than a guessed replacement constant.
