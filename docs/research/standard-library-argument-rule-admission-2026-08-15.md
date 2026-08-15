# Standard-Library Argument Rule Admission, 2026-08-15

## Decision

Admit two default correctness, types-tier rules:

- `invalid-strconv-argument` validates compile-time constant bases, bit sizes,
  and floating-point format bytes passed to exact `strconv` parsing,
  formatting, and append functions; and
- `invalid-binary-write` reports statically known values that exact
  `encoding/binary.Write` calls cannot encode because their types are not
  fixed-size.

Neither rule offers a fix. The intended number base, bit width, floating-point
format, or variable-length encoding is not inferable from the invalid call.

## Defect Evidence

Current Staticcheck SA1030 and SA1003 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` establish the corresponding
standard-library contracts. The supported Go 1.25 and Go 1.26 APIs retain the
same boundaries: invalid parsing arguments return errors, invalid integer
formatting bases panic, unsupported float formats produce placeholders, and
`binary.Write` returns an error for data without a fixed-size representation.

`github.com/vinkdong/gox` at
`65f52a6171387fcd1cd5cb7fd3e5e89796eb062b` contains three live calls in
`vtime/types.go` lines 92, 96, and 100 equivalent to:

```go
t.Value = strconv.FormatInt(ttime, 0)
```

Every call panics because zero is not a supported formatting base.

`github.com/kluctl/go-embed-python` at
`46010e689451decaafbe5acf865570378afc367d` contains multiple live calls in
`embed_util/packer.go` lines 174 through 192 equivalent to:

```go
_ = binary.Write(hash, binary.LittleEndian, "symlink")
_ = binary.Write(hash, binary.LittleEndian, fle.Name)
```

`binary.Write` rejects every string, and each error is discarded. The intended
file-kind and file-name components therefore never reach the content hash.
Byte-slice writes in the same function succeed, making the omission observable
rather than merely inefficient.

The compiler accepts every admitted form, and the supported default `go vet`
catalog has no equivalent diagnostic identity. These rules therefore close
deterministic runtime-contract gaps without duplicating compiler diagnostics.

## Precision Contract

Both rules use `go/types` object identity. Import aliases and dot imports remain
recognized; same-named local functions, methods, and function values do not.
Generated files and packages with type errors are excluded through shared
policy, and both rules support Go 1.25 and Go 1.26 source.

`invalid-strconv-argument` covers exact direct calls to:

- `ParseComplex`, `ParseFloat`, `ParseInt`, and `ParseUint`;
- `FormatComplex`, `FormatFloat`, `FormatInt`, and `FormatUint`; and
- `AppendFloat`, `AppendInt`, and `AppendUint`.

Only compile-time integer constants are validated. Dynamic values and value
flow through variables remain conservative.

`invalid-binary-write` recursively validates basic values, arrays, slices,
struct fields, and one permitted outer pointer. Fixed-width numeric, complex,
boolean, array, slice, and struct shapes are accepted. Architecture-sized
integers, strings, maps, channels, functions, nested pointers, and statically
embedded interfaces are rejected. A top-level interface or type parameter is
skipped because its concrete runtime value may be fixed-size. Tuple arguments
and indirect calls remain outside the initial contract.

Neither rule adds configuration, another package traversal, CFG, SSA, facts, or
source edits.

## Evidence And Cost

The initial focused tests failed because both rule IDs were absent. A second
focused binary test failed until slices, structs, and pointers containing
interfaces were distinguished from a top-level dynamic interface. Final
fixtures cover every supported `strconv` argument role and boundary; invalid
and valid binary shapes; aliases; constants; dynamic arguments; local
lookalikes; indirect calls; exact ranges and messages; suppressions; generated
and type-error policy; source-version selection; and fix absence.

Five one-iteration cold package probes over 100 findings on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured:

- `invalid-strconv-argument`: 92.2 ms median, about 2.13 MB, and 16,818
  allocations; and
- `invalid-binary-write`: 126.2 ms median, about 2.82 MB, and 24,858
  allocations.

Package loading dominates both proportional probes. Each rule performs one
filtered call traversal, exact object lookup, and bounded constant or type-shape
inspection.

Non-mutating exact-rule dogfood completed without findings or tool failures on
the Glippy working tree based on
`0066e693ca9272632261ff016c4a69db79d7c069` and on
`go-libraries/pkg/prompts` at
`e38bab8527e9ec97f668b262b23c70660cac0378`. The prompts module was analyzed
with `GOWORK=off` after dependencies were prefetched into a disposable module
cache, and its pre-existing worktree state was unchanged.

Direct non-mutating runs against disposable clones reported all six reviewed
`go-embed-python` binary writes and all three reviewed `vinkdong/gox` zero-base
calls. The latter module's `go 1.12` directive was changed only inside the
disposable clone to the supported Go 1.25 source floor before analysis; its
reviewed Go source was unchanged.

## Revisit Triggers

Add `encoding/binary.Append` or `Encode` only after supported-version and real
occurrence evidence establishes the same adoption value. Consider tuple-call
mapping or constant value flow only when reviewed defects justify the extra
source mapping or analysis cost. Revisit fixes only when call-site evidence can
prove one intended replacement rather than offering a plausible guess.
