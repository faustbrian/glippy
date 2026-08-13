# Defer In Loop Rule Admission

Date: 2026-08-13

## Decision

Admit `defer-in-loop` to the opt-in `suspicious` preset at the syntax tier. It
reports defers directly enclosed by a finite, conditional, or range loop and
excludes nested function literals and conditionless loops.

## Evidence

- Clippy's `await_holding_lock` and `large_futures` families demonstrate the
  value of diagnostics about resources retained beyond the programmer's
  apparent lexical scope.
- Staticcheck SA9001 covers one channel-range case. Glippy's broader contract
  covers ordinary finite and range loops while retaining a separate CFG rule
  for defers that can never execute in a conditionless loop.
- The compiler permits this code, and `go vet` has no general defer-in-loop
  analyzer.
- Focused fixtures cover counted loops, range loops, nested function scopes,
  conditionless-loop delegation, exact ranges, policies, and source versions.

Sources inspected on 2026-08-13 were Go 1.26.5's language and default vet
contracts, Staticcheck source at `d69e7ee19e2d79b721aa696626cea310c807dd3e`
including SA9001, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`.

## False-Positive Boundary

A deliberately bounded loop may accumulate a small number of defers. That
requires contextual judgment, so the rule is `suspicious`, not default
`correctness`. No fix is offered because extracting a helper or moving cleanup
can change ordering and panic behavior.

## Cost

Twenty syntax-analysis iterations on Apple M4 Max measured medians of
`12,233 ns/op`, `8,912 B/op`, and `88 allocs/op`.

Non-mutating suspicious-preset dogfood completed without findings over Glippy
and `go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

## Revisit Trigger

Revisit CFG precision if dogfood shows high noise from provably single-iteration
loops or if the compiler gains an equivalent diagnostic.
