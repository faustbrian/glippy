# Non-Slice Sort Rule Admission, 2026-08-15

## Decision

Admit `non-slice-sort` as a default correctness, types-tier rule. It reports
statically non-slice first arguments passed directly to `sort.Slice`,
`sort.SliceStable`, or `sort.SliceIsSorted`. These APIs accept `any`, but the
standard library panics when the dynamic value is not a slice.

The rule offers no fix. Removing an address operator, dereferencing a pointer,
copying an array into a slice, or changing the surrounding data shape are not
universally equivalent transformations.

## Defect Evidence

Current Staticcheck SA1028 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` recognizes the same three exact
standard-library functions and reports arrays, strings, integers, and literal
`nil` while remaining conservative for runtime-unknown values.

The current `github.com/softlandia/cpd` default branch at
`8ed712a28923769f141e638c63b3c2133f6feae3` contains this live call in
`cpTable.go` line 34:

```go
func (t *cpTable) sort() *cpTable {
	sort.Slice(&t, func(i, j int) bool { return i < j })
	return t
}
```

`cpTable` is a named slice, so `t` is `*cpTable` and `&t` is `**cpTable`.
The call therefore deterministically panics before sorting. The compiler
accepts it because the first parameter is `any`, and the supported default
`go vet` catalog has no equivalent diagnostic identity.

## Precision Contract

The rule uses `go/types` object identity and recognizes only direct calls to
the three exact `sort` package functions. Import aliases and dot imports remain
recognized. Same-named local functions and methods, function values, and
interface dispatch do not report.

Literal `nil` and statically known arrays, pointers, maps, structs, functions,
and other non-slice types report. Ordinary slices, named slices, and aliases of
slices do not. Interface and type-parameter values remain conservative because
their runtime value may be a slice, including type parameters whose current
constraint has a slice type set.

The diagnostic targets the complete first argument. Generated files and
packages with type errors are excluded through shared policy. The rule supports
Go 1.25 and Go 1.26 source and adds no configuration, CFG, SSA, dependency
syntax, facts, or source edits.

## Evidence And Cost

The first focused test failed because the `non-slice-sort` rule ID was absent.
Final fixtures cover all three APIs; arrays, pointers, maps, structs, functions,
and literal `nil`; slices, named slices, aliases, interfaces, and type
parameters; import aliases; local lookalikes; function values; exact ranges and
messages; source-version selection; suppressions; generated files; type errors;
and fix absence.

Five one-iteration cold package probes over 100 findings on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured a median of 81.4 ms, about 3.50 MB, and
31,002 allocations. Package loading dominates the proportional probe; the rule
performs one filtered call traversal, exact object lookup, and constant-time
type-shape classification.

Non-mutating exact-rule dogfood completed without findings or tool failures on
Glippy and on `go-libraries/pkg/prompts`. The prompts module was analyzed with
`GOWORK=off` after dependencies were prefetched into a disposable module cache,
and its pre-existing worktree state was unchanged.

A direct non-mutating run against a disposable clone of `softlandia/cpd`
reported the exact line-34 argument. The repository declares Go 1.17, below
Glippy's supported source floor, so only the disposable clone's `go` directive
was changed to 1.25 before analysis; the reviewed Go source was unchanged.

## Revisit Triggers

Revisit interface and type-parameter value flow only when reviewed defects show
that the additional analysis would improve signal without introducing runtime
type guesses. Revisit a fix only when a narrower source shape proves one
semantics-preserving replacement. Add other reflection-backed sort helpers only
after their exact standard-library contract and real-defect value are proven.
