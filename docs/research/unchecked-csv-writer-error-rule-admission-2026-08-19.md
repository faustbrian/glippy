# `unchecked-csv-writer-error` Rule Admission, 2026-08-19

## Decision

Admit `unchecked-csv-writer-error` to default `correctness` at warning
severity. The native CFG rule requires every normally returning path after an
ordinary call to `encoding/csv.Writer.Flush` on a direct identifier-backed
writer to observe the matching `Writer.Error`. Generated files and ill-typed
packages are excluded, test files remain eligible, and no fix is offered.

## Defect And Existing Tools

Go 1.26.6 documents that `csv.Writer.Flush` writes buffered data to the
underlying writer but returns no value, and that callers must use
`Writer.Error` to retrieve any error that occurred during an earlier `Write` or
`Flush`. Omitting that observation can report successful CSV output even when
the final buffered write failed.

The compiler and the Go 1.26.6 default vet catalog do not diagnose this
protocol. Broad unchecked-error analyzers cannot see it because `Flush` has no
result to discard. The Go proposal
[`golang/go#72961`](https://github.com/golang/go/issues/72961) sought to make
`Flush` return its stored error directly, demonstrating the API friction.
[`RajajiVignan/Agentic-Data-Analysis#9`](https://github.com/RajajiVignan/Agentic-Data-Analysis/issues/9)
records a CSV export that flushed without checking the stored error.

## Exact Source And Control-Flow Contract

Matching uses the exact selected `encoding/csv.Writer.Flush` and
`encoding/csv.Writer.Error` method objects. Only direct identifier-backed
writer values are tracked. An ordinary `Flush` creates an obligation, and an
`Error` result used in a return, condition, nonblank assignment, or call
argument completes it. A bare `Error` expression statement and `_ =
writer.Error()` do not count as observation. Every normally returning CFG path
must complete the obligation; panicking and otherwise proven no-return paths
do not report.

Reassigning the tracked receiver before observation reports. Explicit aliasing,
passing or returning the writer, closure capture, and method-value transfer
stop analysis conservatively because another owner may observe the stored
error. Fields, containers, indirect aliases, deferred `Flush` calls, and
asynchronous `Flush` calls are outside the initial contract. Same-named project
methods do not match.

No fix is registered. Correct handling may return, join, log, or otherwise
translate the stored error according to the surrounding output contract, and
no single rewrite preserves those choices.

## Behavioral And Cost Evidence

The focused test first failed with `unknown rule
"unchecked-csv-writer-error"`. The green suite covers unchecked and partial
paths, bare and blank-identifier observations, receiver replacement,
observations before and after `Flush`, returned, conditional, assigned, and
call-argument observations, repeated flushes, no-return paths, aliases, helper
calls, closure transfer, deferred and asynchronous exclusions, fields, local
lookalikes, exact writers returned by helper packages, suppressions, generated
and ill-typed packages, test files, source versions, severity, metadata, exact
ranges, and absence of fixes.

Five complete 100-function, 100-finding package-analysis samples ran on Go
1.26.6, Darwin arm64, and an Apple M4 Max:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 61.56 ms | 3,219,760 | 27,642 |
| 2 | 68.30 ms | 3,218,892 | 27,638 |
| 3 | 69.07 ms | 3,218,625 | 27,638 |
| 4 | 62.83 ms | 3,218,383 | 27,642 |
| 5 | 61.45 ms | 3,219,120 | 27,642 |

The median was 62.83 ms, 3,218,892 bytes, and 27,642 allocations per
operation. Every operation includes a fresh package load, so this is
proportional admission evidence rather than a portable latency budget.

Non-mutating exact-rule dogfood completed without findings on Glippy and on
`go-libraries/pkg/prompts` after its dependencies were prepared in a
task-owned module cache. The prompts repository's pre-existing `go.sum` change
remained byte-identical.

## Revisit Trigger

Expand to fields or indirect aliases only when bounded ownership tracking can
preserve the near-zero-false-positive correctness contract. Cover deferred or
asynchronous flushing only when execution order and matching observation can
be proven. Do not add a fix until one rewrite is semantics-preserving across
the surrounding function's error and output contracts.
