# `channel-used-after-close` Rule Admission, 2026-08-19

## Decision

Admit `channel-used-after-close` to the default `correctness` preset at warning
severity. The native CFG rule reports a send or repeated exact built-in close
only when every reaching state proves that the same direct local channel is
already closed. Generated files and ill-typed packages are excluded, and no
fix is offered.

## Defect And Existing Tools

The Go 1.26.6 specification states that a send on a closed channel causes a
run-time panic and that sending to or closing a closed channel panics. The
compiler accepts both operations because channel open or closed state is a
runtime property.

The Go 1.26.6 default vet catalog has no closed-channel state analyzer. The
published Staticcheck catalog reviewed on 2026-08-19 likewise has no send- or
close-after-close rule. The rule therefore adds a path-sensitive runtime-panic
contract without duplicating either default tool.

The exact double-close defect is independently represented by
[`scttfrdmn/giri#52`](https://github.com/scttfrdmn/giri/issues/52), whose
acceptance fixture creates a channel and closes it twice. Go's own runtime
panic tests contain both direct double-close and direct send-after-close cases.
These prove the exact operation contract and demand for static detection.

The reviewed `samuelkarp/purple-docker` revision
`6097d25055899d5d5415b18bcaba77174481fde8` contains a production
close-then-send defect in `plugin/container.go`: `readStringChan` creates a
channel, a goroutine closes it after `ReadString` returns an error, and then
sends the line. That occurrence motivates the broader defect family but is
deliberately outside this first rule because the channel is acquired in an
enclosing function and used through an asynchronous closure. It is not
presented as a detected positive.

## Precision And State Contract

The rule tracks only objects declared inside the analyzed body and initialized
or reinitialized by the exact built-in `make` function with a channel result.
Parameters, package variables, indirect constructors, and shadowed `make` or
`close` functions do not establish state.

A bounded monotone CFG worklist tracks each candidate as untracked, open,
closed, or conservatively unknown. Direct sends and exact built-in closes
report only from the exact closed state. Branch joins retain every reaching
possibility, so a conditionally closed channel does not report. An exact close
after an alias or helper escape reestablishes closed state only on the normal
continuation where that close returned. Direct reacquisition establishes a new
open channel.

Ordinary receives and range receives from a closed channel are legal and do
not report. `len`, `cap`, `print`, and `println` observe a channel without
changing its state. Select sends, directional send-capable locals, and a
channel sent as its own element retain the same send contract. When the sent
value aliases the tracked channel, the send can still report before later
state becomes unknown.

Aliases, returns, storage, helper calls, closure capture, and asynchronous use
become unknown. Deferred calls do not execute at registration. A CFG node with
multiple relevant operations becomes unknown rather than using AST preorder as
proof of Go evaluation order. More than 4,096 candidates or more than the
derived transition bound fails closed with no diagnostic.

No fix is registered because the intended repair may move the send, move or
remove a close, allocate another channel, or change which goroutine owns
closure.

## Behavioral And Cost Evidence

The first focused test failed because the registry did not recognize the new
rule. Later red tests exposed two precision defects: a closed channel sent as
its own value was suppressed as an escape, and an exact close did not
reestablish closed state after an unknown helper or alias. The green suite
covers direct sends, repeated closes, all-path branches, loops, declaration
acquisition, selects, directional channels, channel-valued sends, multiple
objects, exact built-in identity, nested-function isolation, conditional
closure, receives, defers, goroutines, aliases, helper escape, explicit close
after escape, reacquisition, parameters, globals, suppressions, generated
files, type errors, source versions, exact ranges, related locations,
deterministic ordering, metadata, and absence of fixes.

Five complete 100-function, 100-finding package-analysis samples ran on Go
1.26.6, Darwin arm64, and an Apple M4 Max:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 25.10 ms | 2,831,857 | 30,407 |
| 2 | 26.18 ms | 2,831,853 | 30,406 |
| 3 | 25.24 ms | 2,831,279 | 30,404 |
| 4 | 25.34 ms | 2,831,735 | 30,406 |
| 5 | 25.94 ms | 2,831,234 | 30,406 |

The median was 25.34 ms, 2,831,735 bytes, and 30,406 allocations per
operation. Each operation includes fresh package loading, so this is
proportional admission evidence rather than a portable latency budget.

Non-mutating exact-rule dogfood completed without findings or prerequisite
problems on the Glippy candidate worktree based on `ae98a5f` and
`go-libraries/pkg/prompts` at
`77cda571e862ef088cdb1832b049089f00ea8a2a`. The prompts run used a
task-owned prepopulated module cache with network lookup disabled during
analysis and preserved its exact pre-existing byte digest.

## Revisit Trigger

Revisit the representation when reviewed defects require nonlocal acquisition,
alias identities, ordered nested operations, goroutine ownership, or
concurrency-aware close serialization. Do not add a fix without one canonical
semantics-preserving repair.
