# Zero Replace Count Rule Admission, 2026-08-15

## Decision

Admit `zero-replace-count` as a default correctness types-tier rule. It reports
direct calls to exact `strings.Replace` and `bytes.Replace` package functions
when the replacement-count argument is compile-time integer zero. Both APIs
interpret zero as a request to replace no occurrences, so the apparent
substitution is ineffective.

No fix is offered. A positive count, a negative count, and `ReplaceAll` have
different contracts, and the intended replacement limit is not inferable from
the call alone.

## Defect Evidence

Current Staticcheck SA1018 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` reports zero counts for both exact
standard-library functions. The Go API contracts confirm that zero requests no
replacements.

`github.com/diskfs/go-diskfs` tag `v1.9.3` resolves to
`dab48780fdd086fa77366ebe466f5d936f2ff83c` and contains this path-normalization
attempt in `filesystem/iso9660/testdata/isoutil.go`:

```go
ps := strings.Replace(p, "\\", "/", 0)
```

The call cannot replace any path separator. The same defect remains at current
upstream revision `8ac7fad4d33030776710cfe47bc160f1102e1d0f`. A direct Glippy
run against the tagged source reports the zero argument at line 308.

The compiler accepts every admitted form, and the supported default `go vet`
catalog has no equivalent diagnostic identity. Glippy therefore adds a useful
default-toolchain gap without duplicating a compiler failure.

## Precision Contract

The rule uses `go/types` package and function identity, so import aliases do not
affect recognition and same-named local functions or methods do not report.
Literal zero, named constants, and constant expressions equal to zero are
included. Nonzero constants, dynamic counts, function values, generated files,
and packages with type errors remain excluded.

The rule supports Go 1.25 and Go 1.26 source. It introduces no configuration
option, source edit, additional package traversal, control-flow graph, or SSA
construction.

## Evidence And Cost

The initial focused test failed because the rule ID was absent. The final
fixtures cover both exact APIs, import aliases, literal, named, and derived
zero constants, positive and all-occurrence counts, dynamic counts, local
lookalikes, function values, exact diagnostic ranges and messages,
suppressions, generated and type-error policy, Go-version selection, and fix
absence.

Five one-iteration cold package probes over 100 findings on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured a median of 101.0 ms, about 2.92 MB, and
25,986 allocations. Package loading dominates this proportional admission
probe; the rule itself performs one filtered call traversal, object lookup, and
constant lookup.

Non-mutating exact-rule dogfood completed without findings or tool failures on
Glippy at `b684f99d11afa828a1823317519e5e1a09d3528d` and
`go-libraries/pkg/prompts` at
`e38bab8527e9ec97f668b262b23c70660cac0378`. The prompts module was analyzed
with `GOWORK=off` after dependencies were prefetched into a disposable module
cache; neither repository was modified.

## Revisit Triggers

Add another replacement API only from exact supported-version documentation
and object identity. Consider value-flow counts only if real defects justify a
more expensive representation. Revisit fixability only when call-site evidence
proves one intended replacement contract rather than suggesting a likely one.
