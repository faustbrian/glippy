# `exact-suffix-as-cutset` Rule Admission, 2026-08-25

## Decision

Admit `exact-suffix-as-cutset` as an opt-in `nursery` types rule at warning
severity. It reports a direct `strings.TrimRight(value, suffix)` call when an
immediately enclosing direct `strings.HasSuffix(value, suffix)` condition
proves that the second argument represents one exact suffix rather than a
character cutset.

The rule offers an unsafe fix that replaces only the exact standard-library
function identifier with `TrimSuffix`. It remains outside every curated profile
until the v0.8 corpus rerun establishes its signal and fix behavior.

## Defect And Existing Tools

Grafana revision `45a80328c0dd976720af0366efcad0787762b142` contains two
instances that recognize exact PostgreSQL type suffixes and then pass those
suffixes to `strings.TrimRight`. `TrimRight` treats its second argument as an
unordered set of characters. A default expression ending in another character
from the suffix can therefore be truncated beyond the exact suffix already
recognized by the guard.

Staticcheck 2026.2.1 reports both instances as `SA1024`; default `go vet` does
not. Its authoritative rule documentation and implementation were inspected at
[`staticcheck/sa1024`](https://github.com/dominikh/go-tools/tree/2026.2.1/staticcheck/sa1024).
The upstream rule reports compile-time cutsets containing duplicate characters;
it does not prove exact suffix intent from surrounding control flow. Glippy
requires that exact guard-and-call relationship before offering an explicitly
authorized unsafe rewrite.

## Precision Boundary

Typed object identity must resolve both calls to package-level functions in the
standard `strings` package. The condition must be exactly a two-argument
`HasSuffix` call. The first guarded statement must be an assignment, return, or
expression statement whose sole right-hand side or result is the matching
two-argument `TrimRight` call. Assignment targets are limited to identifiers or
selector chains, so no earlier target evaluation can mutate the guarded
operands.
Deferred, asynchronous, nested-function, later-statement, boolean-combination,
helper, negated, and early-return forms are excluded.

The guarded value and suffix must use the same typed identifier or selector,
or equal compile-time string constants. Different objects with the same source
spelling do not match. Import aliases and dot imports remain supported through
typed function identity; user-defined lookalikes and function values remain
excluded. Generated files and ill-typed packages use the shared exclusion
policy.

## Fix Safety

The exact guard proves that `suffix` occurs at the end of `value`. Under that
condition, `strings.TrimSuffix(value, suffix)` removes precisely the recognized
suffix, while `TrimRight` may remove additional cutset characters. Replacing
only `TrimRight` with `TrimSuffix` preserves evaluation order, arguments,
comments, imports, and surrounding control flow, but intentionally changes the
result when additional cutset characters precede the suffix. The fix is
therefore unsafe and requires explicit `--fix-unsafe` authorization.

The fix uses Glippy's ordinary fix coordinator. The complete edited file
is reparsed, formatted, reparsed again, and validated before replacement. The
focused fix fixture proves formatted output and a second analysis produces no
diagnostic.

## Behavioral Evidence

The first focused test failed because the registry did not know
`exact-suffix-as-cutset`. Final fixtures cover ordinary and aliased imports,
equal string constants, different values, different suffixes, combined guards,
later body statements, unguarded deliberate cutsets, exact source ranges,
metadata, source-version selection, generated and type-error exclusions,
suppression severity, unsafe fix authorization, formatting, and fix idempotency.
An owned fixture reproduces both pinned Grafana selector-and-literal shapes and
reports both exact function identifiers.

## Revisit Triggers

Promote the rule only after exact-revision corpus evidence records its signal
and validates the explicitly authorized rewrite across representative
repositories. Broaden to
dominant early-return guards, `TrimLeft` and `HasPrefix`, byte-slice APIs, or
constant propagation only through separate behavioral evidence. Do not infer
exact intent from a standalone `TrimRight` call.
