# `unchecked-writer-error` Interface Encoder Expansion, 2026-08-19

## Decision

Extend the default-correctness `unchecked-writer-error` rule to three exact
standard-library streaming encoder constructors:

- `encoding/ascii85.NewEncoder`;
- `encoding/base32.NewEncoder`; and
- `encoding/base64.NewEncoder`.

These functions expose their concrete buffered encoders only as
`io.WriteCloser`, so method identity alone cannot distinguish them from
unrelated interface closers. Glippy now retains the constructor identity for
an inline result or a direct constructor-initialized identifier that is not
reassigned before `Close`. The rule remains at the types tier, keeps its
existing diagnostic identity, and offers no fix.

## Defect And Existing Tools

Go 1.26.6 documents that all three encoder `Close` methods flush pending
output. The base32 and base64 constructor documentation additionally requires
callers to close the returned encoder to flush partially written blocks. Their
implementations return the underlying write error from `Close`.

Two public defects establish practical impact:

- [`tobischo/gokeepasslib#86`](https://github.com/tobischo/gokeepasslib/issues/86)
  records a base64 encoder left unclosed, leaving bytes unflushed and producing
  zero-byte attachments in KeePassXC.
- [`formancehq/auth#136`](https://github.com/formancehq/auth/pull/136) records
  truncated base64 output discovered by a round-trip fuzz test. Its repair
  binds the encoder, closes it, and checks the returned error.

The compiler and the Go 1.26.6 default vet catalog do not connect the dynamic
`io.WriteCloser.Close` call to these exact constructors. Broad unchecked-error
rules can report the discarded interface method, but they cannot make this
small data-integrity set a default without also reporting every unrelated
closer. Glippy gives the exact proven acquisitions one default diagnostic and
delegates overlapping `discarded-error` and `blank-error-discard` findings.

## Exact Source Contract

An inline `NewEncoder(...).Close()` call matches directly. A named result
matches only when the receiver object is defined by a direct short declaration
or initialized variable declaration whose right-hand side resolves to one of
the three exact standard-library functions. Any assignment to that object
between acquisition and finalization makes the binding unproven and excludes
the call.

Ordinary expression statements, deferred calls, asynchronous calls, and
single blank-identifier assignments report. Returned, conditionally handled,
or nonblank-assigned errors do not. Same-named project constructors, arbitrary
`io.Closer` or `io.WriteCloser` parameters, indirect constructors, method-value
calls, and converted or selected receivers remain excluded. Generated files
and ill-typed packages retain the parent rule's existing policy.

The package-loaded syntax tree is now retained in `TypesContext` alongside the
matching `go/types.Info`, allowing the types-tier rule to prove the defining
constructor without reparsing or guessing across AST identities.

## Behavioral And Cost Evidence

The focused regression first produced no findings for all three exact
constructors. After implementation it covers ordinary, deferred,
asynchronous, blank-identifier, inline, and initialized-variable forms;
handled errors; reassignment; arbitrary interface closers; project lookalikes;
exact ranges; and absence of fixes. A mutation that removed generic-rule
delegation produced duplicate `discarded-error` and `blank-error-discard`
diagnostics; the restored coordinator leaves only `unchecked-writer-error`.

Five complete 100-function, 100-finding package-analysis samples ran on Go
1.26.6, Darwin arm64, and an Apple M4 Max:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 86.70 ms | 3,909,296 | 30,155 |
| 2 | 79.54 ms | 3,344,520 | 29,716 |
| 3 | 79.28 ms | 3,343,376 | 29,721 |
| 4 | 78.30 ms | 3,332,032 | 29,693 |
| 5 | 79.86 ms | 3,335,632 | 29,705 |

The median was 79.54 ms, 3,343,376 bytes, and 29,716 allocations per
operation. Every operation includes a fresh package load, so this remains
proportional admission evidence rather than a portable latency budget.

Non-mutating exact-rule dogfood completed without findings on Glippy and on
`go-libraries/pkg/prompts` after both modules were prepared in one disposable
module cache. The prompts repository's pre-existing `go.sum` change remained
byte-identical.

## Revisit Trigger

Add ordinary assignment, alias, helper-returned, field, container, or
method-value acquisition only when the exact concrete encoder identity can be
proven without broadening interface dispatch. Do not add a fix until one
rewrite preserves ordinary, deferred, asynchronous, and surrounding error
contracts.
