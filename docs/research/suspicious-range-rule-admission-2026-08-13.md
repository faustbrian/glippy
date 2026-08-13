# Suspicious Range Rule Admission

Date: 2026-08-13

## Decision

Admit `suspicious-range` to the opt-in `suspicious` preset at the types tier. It
reports field or element mutations rooted in a copied struct or array range
value when the mutation cannot cross a reference-like boundary.

## Evidence

- Clippy's range and needless-copy diagnostics prioritize iteration constructs
  whose surface syntax hides copied state.
- Go range values are copies. Mutating a field of a ranged struct commonly
  looks like an update to the source slice or map but changes only the local
  copy. The compiler and default `go vet` do not diagnose this contract.
- Focused fixtures cover slice and map values, index-based mutation, pointer
  elements, read-only use, exact ranges, policies, and source versions.

Sources inspected on 2026-08-13 were Go 1.26.5 range semantics and default vet
catalog, Staticcheck source at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy source at
`9a73ad846274efca140b1d2ea316b830fa1fb8de` for copied-value and range-loop
lint boundaries.

## False-Positive Boundary

Only assignment and increment targets rooted in the exact range-value object
are reported. Pointer, slice, map, interface, and channel crossings are
excluded because their referenced state may be shared. Direct reassignment,
later storage of the range value, and nested function literals are excluded.
No fix is offered because map values and slice values require different
repairs.

## Cost

Five package-analysis iterations on Apple M4 Max measured medians of
`46,672,858 ns/op`, `185,931 B/op`, and `1,328 allocs/op`. Package loading
dominates the single-rule fixture.

Glippy dogfood identified an intentional mutation followed by write-back and
established that conservative exclusion. Final non-mutating dogfood completed
without findings over Glippy and `go-libraries/pkg/prompts` at
`6aa246ba6cd9e8bcb4d94d4ad156635285cc2f22`; the prompts repository head and
pre-existing dirty status were unchanged.

## Revisit Trigger

Revisit SSA-backed escape and write-back tracking if real code demonstrates
valuable cases beyond the conservative object-rooted contract.
