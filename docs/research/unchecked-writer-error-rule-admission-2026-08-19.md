# `unchecked-writer-error` Rule Admission, 2026-08-19

## Decision

Admit `unchecked-writer-error` to default `correctness` at warning severity.
The native types rule originally reported discarded errors from 17 exact
standard-library `Flush` and `Close` methods whose contracts write buffered
bytes or required framing. The later
[interface encoder expansion](unchecked-writer-interface-encoder-expansion-2026-08-19.md)
adds three exact `NewEncoder` acquisition contracts under the same diagnostic
identity. The rule covers ordinary call statements, deferred calls,
asynchronous calls, and explicit assignment to the blank identifier. Generated
files and ill-typed packages are excluded, test files remain eligible, and no
fix is offered.

## Defect And Existing Tools

The affected APIs can return their first underlying write failure or emit
essential output only during finalization. Go 1.26.6 documents, among other
cases, that `gzip.Writer.Close` flushes unwritten compressed data and writes the
footer, `archive/zip.Writer.Close` writes the central directory,
`multipart.Writer.Close` writes the trailing boundary, and
`bufio.Writer.Flush` writes buffered data to its underlying writer. Ignoring
these errors can therefore report success for an incomplete stream even when
earlier writes succeeded.

The compiler and the Go 1.26.6 default vet catalog do not diagnose ignored
writer finalization results. The external `errcheck` analyzer provides a broad
unchecked-error policy, but it treats every non-excluded error alike and makes
blank-identifier checking optional. Glippy instead enables this narrow
data-integrity subset by default, gives all ignored contexts one stable rule
identity, and retains reasoned suppressions and deterministic package policy.

Two current public defects establish the practical impact:

- [`ethereum/go-ethereum#35480`](https://github.com/ethereum/go-ethereum/issues/35480)
  records `admin_exportChain` returning success after a deferred
  `gzip.Writer.Close` failure left a ten-byte archive that failed gzip
  validation.
- [`kubeedge/kubeedge#7145`](https://github.com/kubeedge/kubeedge/issues/7145)
  records backup and compression paths discarding deferred gzip and tar writer
  close errors, allowing full-disk failures to produce truncated backups while
  the operation reports success.

## Exact API And Source Contract

The initial catalog contains these declaring receiver and method identities:

- `archive/tar.Writer.Flush` and `Close`;
- `archive/zip.Writer.Flush` and `Close`;
- `bufio.Writer.Flush`;
- `compress/flate.Writer.Flush` and `Close`;
- `compress/gzip.Writer.Flush` and `Close`;
- `compress/lzw.Writer.Close`;
- `compress/zlib.Writer.Flush` and `Close`;
- `encoding/xml.Encoder.Flush` and `Close`;
- `mime/multipart.Writer.Close`;
- `mime/quotedprintable.Writer.Close`; and
- `text/tabwriter.Writer.Flush`.

Matching uses the exact selected method object, package path, method name, and
declaring receiver type. Bound methods, method expressions, and promoted exact
methods report. Same-named project methods, `os.File.Close`, interface-dispatched
`io.Closer.Close`, and other unrelated finalizers do not.

A call reports only when its result is structurally discarded as an expression
statement, a `defer` or `go` statement, or a single blank-identifier assignment.
Returning the error, assigning it to a nonblank variable, or using it in an
initializer or condition counts as observation. The diagnostic covers the
exact call range. `discarded-error` and `blank-error-discard` delegate matching
calls to this rule so strict policies produce one diagnostic rather than
overlapping generic and specialized findings.

The standard library also exposes buffered encoders from `encoding/ascii85`,
`encoding/base32`, and `encoding/base64` only through `io.WriteCloser`. The
rule now proves their concrete finalization contract through an inline exact
`NewEncoder` result or a direct constructor-initialized binding that is not
reassigned before `Close`. Indirect constructors, method-value calls, and
reassigned interface bindings remain excluded. `encoding/csv.Writer.Flush`
returns no error and instead requires a later `Writer.Error` observation; the
separately admitted `unchecked-csv-writer-error` rule owns that path-sensitive
protocol.

No fix is registered. A deferred call may require a named result, an explicit
close before return, joined body and finalization errors, logging, or a product
specific best-effort policy; no single rewrite preserves every surrounding
contract.

## Behavioral And Cost Evidence

The first focused test failed with `unknown rule "unchecked-writer-error"`.
After initial implementation, the overlap test failed with five diagnostics
for three calls because `discarded-error` and `blank-error-discard` still
reported the same exact operations. Explicit ownership delegation reduced the
result to the three specialized findings.

The green suite covers all 17 admitted method identities, ordinary, deferred,
asynchronous, and blank-identifier discards, handled results, exact method
expressions, promoted methods, local lookalikes, file and interface closers,
zero-result CSV flushing, suppressions, generated files, type errors, test
files, source versions, exact ranges, metadata, overlap ownership, and absence
of fixes.

Five complete 100-function, 100-finding package-analysis samples ran on Go
1.26.6, Darwin arm64, and an Apple M4 Max:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 64.60 ms | 3,109,992 | 24,758 |
| 2 | 64.91 ms | 3,108,781 | 24,753 |
| 3 | 64.71 ms | 3,109,954 | 24,760 |
| 4 | 65.04 ms | 3,108,676 | 24,754 |
| 5 | 64.82 ms | 3,108,618 | 24,753 |

The median was 64.82 ms, 3,108,781 bytes, and 24,754 allocations per
operation. Every operation includes a fresh package load, so this is
proportional admission evidence rather than a portable latency budget.

Non-mutating exact-rule dogfood completed without findings on Glippy and on
`go-libraries/pkg/prompts` after its dependencies were prepared in a
task-owned module cache. The prompts repository's pre-existing `go.sum` change
remained byte-identical.

## Revisit Trigger

Expand interface-returning encoder coverage beyond inline or stable direct
constructor bindings only after bounded acquisition and alias tracking proves
concrete finalizer identity. Keep CSV flushing in the separate path-sensitive
`unchecked-csv-writer-error` rule. Expand beyond the exact standard-library
catalog only when real defects justify a stable project contract or
cross-package finalizer fact. Do not add a fix until one rewrite is
semantics-preserving across ordinary and deferred return paths.
