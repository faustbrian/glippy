# `shadowed-error` Rule Admission, 2026-08-15

## Decision

Admit `shadowed-error` as an opt-in `suspicious` types-tier rule at warning
severity. It reports two bounded stale-error flows:

1. an inner error is checked for non-nil, that path can break the containing
   loop, and the unchanged outer error is returned or checked after the loop;
2. an inner error shadows a named error result and a deferred function assigns
   the inner object, so the named result cannot receive the deferred failure.

The rule has no fix. Changing `:=` to `=` can alter the type, scope, lifetime,
or other destinations of a declaration, while renaming the inner error does not
propagate it to the outer observation.

## Defect And Existing Tools

The canonical loop defect returns a stale outer error:

```go
var err error
for {
	_, err := reader.Read(buf)
	if err != nil {
		break
	}
}
return err
```

The compiler accepts the code. The default Go vet suite does not run the
experimental x/tools `shadow` analyzer. That analyzer was inspected at
`golang.org/x/tools` v0.48.0. Its non-strict mode reports same-name,
identical-type declarations when the outer variable has any lexically later
reference. Upstream explicitly describes the general analyzer as noisy.

An initial Glippy adaptation narrowed the analyzer to error-implementing types,
but exact-rule dogfood still reported ordinary local `if err := ...` handling
throughout Glippy. That design was rejected before admission. Error type alone
does not make general lexical shadowing a useful suspicious diagnostic.

Current Clippy source was reviewed at rust-clippy commit
`e52501913b75235e3d41422566a2d05d6f00b699`. Rust's binding and error-flow
model does not provide a rule that can be copied as a semantic authority for
Go's conventional `err` reuse. Glippy follows Clippy's high-signal opt-in group
contract while keeping the detection Go-native.

## Public Occurrence

`travisennis/cake-repl` commit
[`6cd8f407a0d584f863f37fce4009e39031941890`](https://github.com/travisennis/cake-repl/commit/6cd8f407a0d584f863f37fce4009e39031941890)
fixed a named-result shadow at parent revision
`f5748b37d71b139de4b523b7510e5e2da5e1d9d7`. Inside `run`,
`f, err := os.OpenFile(...)` created a block-scoped error. A deferred closure
then assigned that inner error when closing the debug log, while the function
returned the distinct named error result. The fix reuses the named result and
allows a close failure to reach the caller.

The current development binary reports the original occurrence at
`cmd/cake-repl/main.go:215:6` and emits no other `shadowed-error` diagnostic for
that selected package.

## Precision Boundary

The native rule uses shared `go/types` object identity. Inner and outer objects
must have the same name and identical type, and that type must be assignable to
the built-in `error` interface. Both declarations must belong to the same
function. Package-level and nested-function shadows are excluded.

Loop findings require an `err != nil` condition whose branch contains an
unlabelled break targeting the exact containing `for` or `range`. A later
explicit return of the outer object, a bare return of that named result, or a
later nil comparison is the stale observation. Breaks targeting nested loops,
switches, type switches, or selects do not qualify.

Deferred findings require the outer object to be a named error result and a
deferred function literal inside the inner object's scope to assign the inner
object. Merely reading, checking, logging, returning, or locally handling a
shadowing error does not report. Generated files and ill-typed packages are
excluded. Shared severity, suppression, baseline, path override, changed-code,
source-version, and deterministic reporting contracts apply unchanged.

## Fix Safety

No safe, suggestion, or unsafe fix is registered. A correct repair may reuse
the outer variable, rename the inner value and propagate it, change loop control
flow, or update the named result explicitly. Those alternatives are not
semantically interchangeable.

## Evidence And Cost

The focused product test first failed because `shadowed-error` was unknown.
The broad adapted design was then rejected after exact-rule dogfood produced
ordinary local-handling findings. Current fixtures cover loop escape, named
results updated by deferred closures, same-type local shadows, different error
types, non-error shadows, uninitialized declarations, outer reassignment,
non-loop break targets, inconsequential shadows, nested functions, idiomatic
redeclarations, exact ranges, generated and ill-typed exclusions,
suppressions, severity, metadata, minimum Go version, and absence of fixes.

A five-iteration 100-function package probe produced 100 findings in 39.6 ms,
allocating about 3.30 MB with 30,656 allocations per run. A single-function
scaling probe measured 59.5 ms and 2.21 MB for 100 loops versus 269.1 ms and
20.64 MB for 1,000 loops. The 10x
source increase therefore retained sublinear latency growth relative to the
input multiplier and approximately proportional allocation growth on Darwin
arm64. Non-mutating exact-rule dogfood completed without findings on Glippy at
`f2a055f` plus this change and on `go-libraries/pkg/prompts` at
`e38bab8527e9ec97f668b262b23c70660cac0378`.

## Revisit Triggers

Revisit when public defects demonstrate another stale-flow shape that can be
proved without reviving general shadow noise, or when control-flow evidence can
replace one of the current syntax bounds with a more precise contract. Do not
enable the rule by default or add a fix until broader repository evidence shows
near-zero noise and one semantics-preserving repair.
