# `http-canonical-header-key` Rule Admission, 2026-08-13

## Decision

Admit `http-canonical-header-key` to the opt-in `suspicious` preset at warning
severity. The native types-tier rule reports compile-time noncanonical string
keys used through direct indexing of the standard library `http.Header` type.

## Defect And Existing Tools

`http.Header` is a map whose methods canonicalize field names. Direct indexing
does not canonicalize, so a noncanonical read can miss a value and a
noncanonical write can create a second entry that `Get`, `Set`, `Add`, or `Del`
addresses under a different spelling. The compiler and default Go 1.26.5 vet
accept these accesses.

Staticcheck SA1008 is the external rule authority. The audit inspected its
current source and fixtures at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
SA1008 checks constant-key reads and deliberately skips assignment left-hand
sides. Glippy covers both reads and writes because the standard library map
contract applies to both, while keeping the rule opt-in because some programs
deliberately use `http.Header` as a raw case-preserving map.

Reviewed public changes demonstrate both ordinary repairs and the intentional
exception boundary:

- hyperengineering/recall replaced four direct noncanonical lookups with
  `Header.Get` in
  [`1df82af3d13ce095c5814cd49804add7449916ac`](https://github.com/hyperengineering/recall/commit/1df82af3d13ce095c5814cd49804add7449916ac);
- privacybydesign/irmago deliberately cast a case-preserving result to the
  underlying map type in
  [`cd1e976f916f985c48dd44b5dc1077f27911a550`](https://github.com/privacybydesign/irmago/commit/cd1e976f916f985c48dd44b5dc1077f27911a550),
  making the exceptional ownership explicit instead of suppressing all header
  checks.

Clippy has no direct equivalent because this is a Go standard-library map
contract. Its API-misuse rules support keeping such diagnostics separate from
layout and from speculative style advice.

## Precision, Policy, And Fixes

The rule recognizes `net/http.Header` by `go/types` package and named-type
identity, including aliases. It visits shared index-expression traversal and
reports the exact key expression when its compile-time string differs from
`http.CanonicalHeaderKey`. Canonical constants, dynamic keys, invalid header
names that the canonicalizer leaves unchanged, ordinary maps, and method calls
do not report.

Generated files and ill-typed packages are excluded. The minimum source version
is Go 1.25. Suppressions and deterministic baselines use the ordinary exact
rule ID and `noncanonical-header-key` message identity.

No fix is registered. Replacing a key or changing direct access to a method can
change behavior when a map intentionally contains noncanonical entries, and an
identifier constant may be shared outside the current access. That decision
requires developer review.

## Admission Evidence

The focused test first failed because the registry did not know the rule. The
implemented fixtures cover literal reads, literal writes, constant identifiers,
request fields, canonical keys, dynamic keys, ordinary maps, invalid names,
exact ranges, no-fix behavior, metadata, suppressions, generated files,
type-error exclusion, source versions, CLI JSON output, and baseline generation.

Five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max observed a
median of `202,556,933 ns/op`, `4,649,897 B/op`, and `38,160 allocs/op` for the
one-file fixture. Package loading dominates this proportional measurement.

Non-mutating correctness-and-suspicious dogfood completed with zero findings
over Glippy at `a8cde9ab44ba5d974a1357ada690d1dd5b1ea9b5` plus the working
implementation and over `go-libraries/pkg/prompts` at
`6ed3a06a4e1aba412d2a6b91454774234f30a464`. The prompts revision and
pre-existing dirty state were unchanged.

## Revisit Trigger

Revisit preset placement if dogfood shows that explicit raw-map ownership is
common. Add a fix only if a uniquely correct replacement can be proven from
the local access and source-preservation contracts.
