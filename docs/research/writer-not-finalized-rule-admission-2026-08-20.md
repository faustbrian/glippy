# `writer-not-finalized` Rule Admission, 2026-08-20

## Decision

Admit `writer-not-finalized` to default correctness at warning severity. The
control-flow rule reports a directly acquired standard-library writer only
when an exact output-producing method has executed and one successfully
returning path neither finalizes nor transfers the writer.

The initial exact catalog covers `archive/tar.NewWriter`,
`compress/gzip.NewWriter`, `compress/gzip.NewWriterLevel`, and
`mime/multipart.NewWriter`. Their required finalizer is `Close`.

The streaming-encoder expansion on 2026-08-20 adds exact direct results from
`encoding/ascii85.NewEncoder`, `encoding/base32.NewEncoder`, and
`encoding/base64.NewEncoder`. Their `Write` method starts the same obligation,
and their required finalizer is also `Close`.

## Defect And Existing Tools

These writers can buffer output or require framing that `Close` emits. A
function can therefore return success after earlier writes succeeded while
leaving a tar stream unpadded, a gzip stream without its footer, or multipart
output without its trailing boundary. The compiler and default Go vet catalog
do not require those lifecycle calls.

The three streaming encoders buffer partial blocks until `Close`. Public
defects recorded by
[`tobischo/gokeepasslib#86`](https://github.com/tobischo/gokeepasslib/issues/86)
and [`formancehq/auth#136`](https://github.com/formancehq/auth/pull/136) show
zero-byte attachments and truncated base64 output when callers omit that
finalizer. Their constructors return only `io.WriteCloser`, so exact
constructor identity is required to distinguish these data-integrity APIs from
arbitrary interface closers.

The existing default `unchecked-writer-error` rule detects a discarded error
from a finalizer that was called. It cannot detect an absent finalizer. The new
rule owns that separate path-sensitive defect. The generic
`resource-not-closed` rule delegates these exact constructors instead of
applying a competing close-on-every-return contract.

The exact contracts were checked against the Go 1.26.6 documentation for
[`archive/tar.Writer.Close`](https://pkg.go.dev/archive/tar#Writer.Close),
[`compress/gzip.Writer.Close`](https://pkg.go.dev/compress/gzip#Writer.Close),
and
[`mime/multipart.Writer.Close`](https://pkg.go.dev/mime/multipart#Writer.Close).
The encoder expansion uses the Go 1.26.7 documentation and implementations for
[`encoding/ascii85.NewEncoder`](https://pkg.go.dev/encoding/ascii85#NewEncoder),
[`encoding/base32.NewEncoder`](https://pkg.go.dev/encoding/base32#NewEncoder),
and
[`encoding/base64.NewEncoder`](https://pkg.go.dev/encoding/base64#NewEncoder).
The compiler and default vet boundary was checked with Go 1.26.6 for the
initial admission and Go 1.26.7 for the encoder expansion.

## Precision Contract

The acquisition must be a direct assignment or initialized local variable
declaration from one exact constructor. Multi-result constructors map their
declared result index; parallel multi-expression acquisitions remain outside
the direct mapping contract. The writer becomes obligated only after an exact
output-producing receiver method:

- tar `AddFS`, `Write`, or `WriteHeader`;
- gzip `Write` or `Flush`; or
- ascii85, base32, or base64 `Write`; or
- multipart `CreateFormField`, `CreateFormFile`, `CreatePart`, or `WriteField`.

Construction and configuration alone do not report. Direct or deferred
`Close` completes the lifecycle. Returning, sending, storing, passing, or
capturing the writer transfers ownership conservatively; method values,
asynchronous use, aliases, and replacement bindings also stop or fail closed
without guessing execution order.

A function with no error result treats every normal return as success. When
one exact built-in `error` result exists, only an explicit nil error expression
is proven successful. Named results, tuple delegation, and other unknown error
expressions remain conservative. No fix is registered because correct close
placement and error joining depend on the surrounding return contract.

## Behavioral And Cost Evidence

The first focused test failed with unknown rule `writer-not-finalized`. The
first implementation then missed implicit success returns from functions with
no results. A separate transfer regression initially reported returned, sent,
stored, and method-value-transferred writers. Final review also found that the
exact tar catalog omitted `Writer.AddFS`; its regression failed before the
method was admitted. The corrected dataflow preserves six original exact
missing-finalization findings while excluding handled, deferred,
failure-only, unused, transferred, asynchronous, local-lookalike, generated,
and ill-typed cases. Suppression, severity, minimum-version, exact-range, and
diagnostic-ownership behavior are covered.

The streaming-encoder regression then produced only those six original
diagnostics before constructor identity was added. The final focused fixture
adds one exact missing-finalization diagnostic for each encoder while accepting
finalized, unused, and transferred interface values. A combined correctness
run reports only `writer-not-finalized`; the generic resource rule delegates
all seven exact constructor functions instead of duplicating the findings.

The initialized-declaration follow-up reproduced a missed base64 obligation:
`var writer = base64.NewEncoder(...)` wrote output and returned success without
closing, but produced no diagnostic. The shared acquisition mapping now accepts
CFG `ValueSpec` nodes as well as assignment nodes. Focused coverage includes the
single-result encoder form, multi-result `gzip.NewWriterLevel`, and a finalized
declared encoder without broadening to parallel expression mapping.

Five complete 100-function package-analysis samples on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 86.06 ms | 5,053,696 | 40,679 |
| 2 | 83.66 ms | 4,484,416 | 40,260 |
| 3 | 86.22 ms | 4,482,048 | 40,234 |
| 4 | 86.69 ms | 4,472,288 | 40,236 |
| 5 | 84.47 ms | 4,471,872 | 40,229 |

Exact-rule runs produced no findings on Glippy,
`go-libraries/pkg/prompts`, or `go-libraries/pkg/http-client`. The external
repositories retained their pre-existing bytes and status.

A follow-up ownership regression proved that the generic
`resource-not-closed` rule still reported both an unfinalized gzip writer and a
multipart writer used only to validate a boundary. Delegating the four exact
constructors removes that competing contract. On the same `http-client`
revision, the generic finding count falls from ten to seven by removing exactly
the three multipart writer diagnostics, while `writer-not-finalized` remains
clean. Five one-operation `resource-not-closed` benchmark samples measured
73.00-110.71 ms, 2.13-2.72 MB, and 17,037-17,494 allocations on Darwin arm64.

The streaming-encoder expansion remained clean under exact-rule dogfood on
Glippy, `go-libraries/pkg/prompts`, and `go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`. Both external checks were
non-mutating and preserved their pre-existing status. Five complete
100-function package-analysis samples on Go 1.26.7, Darwin arm64, and an Apple
M4 Max measured `84,827,083-93,625,292 ns/op`,
`4,471,880-5,060,424 B/op`, and `40,232-40,690 allocs/op`.

The initialized-declaration follow-up remained clean under exact-rule dogfood
on Glippy, `go-libraries/pkg/prompts`, and `go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; both external repositories
retained their pre-existing state. Five complete 100-function samples on the
same Go 1.26.7 Darwin arm64 host measured
`86,175,917-95,411,916 ns/op`, `4,469,480-5,059,128 B/op`, and
`40,219-40,695 allocs/op`.

## Revisit Trigger

Expand the exact constructor and method catalog only after public API contracts
and real defects justify each lifecycle. Add success proof beyond explicit nil
error returns only through shared path evidence that cannot reinterpret failure
returns as successful output. Do not infer finalization requirements from a
method name or a general `io.Writer` interface.
