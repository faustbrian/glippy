# Regexp Correctness Rule Admission, 2026-08-15

## Decision

Admit two default correctness rules at the shared types tier:

- `invalid-regexp` reports invalid compile-time constant patterns passed to
  exact `regexp` compilation and Match helpers; and
- `zero-regexp-match-limit` reports exact `*regexp.Regexp` FindAll methods
  whose compile-time limit is zero and therefore permits no result.

Neither rule offers a fix. An invalid pattern's intended syntax is not
inferable, and replacing a zero limit with a positive or negative value
requires knowing whether the caller intended one, some, or all matches.

## Defect Evidence

The supported Go 1.25 and Go 1.26 `regexp` contracts make both outcomes
deterministic. `MustCompile` and `MustCompilePOSIX` panic for an invalid
pattern; `Compile`, `CompilePOSIX`, and the package Match helpers return an
error instead of performing the intended match. Every FindAll method returns
at most `n` matches when `n` is nonnegative, so `n == 0` always returns an
empty result; a negative value requests every match.

Current Staticcheck independently exposes these defects as SA1000 and SA1010
at revision `d69e7ee19e2d79b721aa696626cea310c807dd3e`. Both checks have been
part of its correctness catalog since 2017.1. Glippy additionally covers the
two POSIX compiler functions through their exact standard-library contracts.
The Go compiler accepts these calls, and the supported default `go vet`
catalog has no equivalent diagnostic identity.

No matching production defect was found in the local Glippy and Go-libraries
search. That absence does not weaken the deterministic API outcome, but it
does bound the evidence claim: admission rests on the standard-library
contract and the mature Staticcheck precedent rather than a newly discovered
public occurrence.

## Precision Contract

Both rules use `go/types` object, package, function, and receiver identity, so
import aliases and dot imports cannot change recognition while local
lookalikes do not report. Function and method values, interface dispatch,
dynamic arguments, generated files, and ill-typed packages remain excluded.
Named constants and constant expressions are included without value flow.

`invalid-regexp` covers exact direct calls to `regexp.Compile`,
`CompilePOSIX`, `MustCompile`, `MustCompilePOSIX`, `Match`, `MatchReader`, and
`MatchString`. POSIX calls are validated with the POSIX parser. Diagnostic
messages retain only the parser's error category and do not copy the pattern
into machine output. Constant patterns larger than 64 KiB are skipped to
bound parser work inside the existing source-size boundary.

`zero-regexp-match-limit` covers all eight exact `*regexp.Regexp` FindAll
methods. It reports only a compile-time integer zero and excludes positive,
negative, and unknown limits.

Both rules support Go 1.25 and Go 1.26 source, require only the shared types
tier, and add no configuration option, edit, CFG, SSA, dependency traversal,
or cache input.

## Evidence And Cost

The focused product tests initially failed because both rule IDs were absent.
The final fixtures cover all fifteen exact APIs, aliases, named and derived
constants, POSIX-only invalid syntax, valid and dynamic patterns, positive and
negative limits, local lookalikes, function and method values, exact ranges,
stable messages, fix absence, suppressions, generated files, ill-typed
packages, metadata, source versions, and the 64 KiB validation bound.

Five one-iteration cold package probes over 100 findings on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured:

- `invalid-regexp`: 124.7 ms median, about 2.75 MB, and 24,167 allocations;
  and
- `zero-regexp-match-limit`: 93.9 ms median, about 2.78 MB, and 24,164
  allocations.

Package loading dominates both proportional probes. Each rule otherwise uses
one filtered call traversal plus constant and object lookup; invalid patterns
also run the bounded standard-library regexp parser.

Non-mutating exact-rule dogfood completed without findings or tool failures on
the Glippy candidate tree based on `6df802a` and on
`go-libraries/pkg/prompts` at
`e38bab8527e9ec97f668b262b23c70660cac0378`. The prompts module was analyzed
with `GOWORK=off` after dependencies were prefetched into a disposable module
cache; neither repository was modified.

## Revisit Triggers

Add another regexp API only from an exact supported-version contract. Revisit
value-flow arguments only if real defects justify a more expensive tier.
Revisit fixability only when call-site evidence proves the intended pattern or
match limit instead of suggesting one.
