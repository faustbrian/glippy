# `resource-used-after-close` Rule Admission, 2026-08-16

## Decision

Admit `resource-used-after-close` to the opt-in `suspicious` preset at warning
severity. The native CFG rule reports a curated direct operation only when a
locally acquired `Close() error` value is proven closed on every reaching path.
It offers no fix.

## Defect And Existing Tools

Go permits operations after `Close`; the behavior belongs to the concrete
resource contract rather than the type system. The Go `os.File.Close`
documentation says that closing a file renders it unusable for I/O. Go commit
`703cc8abeca5ff4f3da1df6766c6abf43462257b` further documents that file-system
implementations should return `fs.ErrClosed` when operations, including a
second close, are attempted on a closed file. This makes the sequential defect
observable even when it returns an error instead of panicking.

The reviewed `LeJamon/go-xrpl` commit
`3ee46d079fcf1fab136f2b83e73ba31d3133d31f` fixes operations racing with a
Pebble store close because the underlying database can panic after close. It
establishes the broader use-after-close defect family, but it is concurrent
evidence and is not presented as proof of the rule's sequential fixture.

The Go 1.26.6 default vet catalog has no general use-after-close analyzer. The
current published Staticcheck catalog covers close-before-error-check and
defer-in-loop defects but no general path-proven operation after close. A
general rule therefore adds value without duplicating either default surface.

## Precision And State Contract

The rule tracks only direct local assignment results whose static type has
`Close() error`. A bounded monotone worklist reaches stable CFG block-entry
states before diagnostics are collected. Each tracked object is untracked,
open, closed, or conservatively unknown; branch joins retain every reaching
possibility, and a finding requires the exact closed state.

An exact direct `Close` establishes closed state after acquisition, including
after an earlier escape made the state unknown. A direct reacquisition
establishes open state. Exact same-module effect facts and configured project
contracts can establish close. Guaranteed ownership transfer and every helper
without an exact close effect become unknown: an ownership borrow proves that
the helper did not take the value, but it does not prove that arbitrary methods
left the resource's internal state unchanged. Mixed close/transfer effects,
aliases, returns, storage, method values, closure capture, arbitrary receiver
methods, and reassignment also become unknown.

The initial operation set covers direct I/O, file, deadline, synchronization,
flush, and accept method names. Deferred close is not applied at registration
time, and asynchronous close becomes unknown. A CFG node with multiple tracked
calls becomes unknown because AST preorder is not sufficient evidence for
every nested Go evaluation order. Zero-result `Close` methods are excluded.

The rule remains `suspicious`, not `correctness`, because `Close() error` is a
convention rather than a universal post-close semantic contract. No fix is
offered: the intended repair may move the operation, move the close, reacquire
the value, or remove an invalid action.

## Admission Evidence

Focused package fixtures cover direct and both-branch closes, explicit close
after escape, conditional close, direct and helper defer and goroutine behavior,
alias and helper escape, direct and helper arbitrary methods, reacquisition,
method-value escape, zero-result close exclusion, same-package and configured
close and transfer effects, suppressions, generated files, type errors, source
versions, exact ranges, related close locations, metadata, and absence of
fixes. The shared
effect regression proves that zero-parameter functions cannot truncate later
parameter summaries and that asynchronous parameter use is transfer rather
than synchronous completion.

Five benchmark samples over 100 findings on Go 1.26.6, Darwin arm64, Apple M4
Max measured a `54,356,916 ns/op` median, about `4,559,152 B/op`, and `53,794`
allocations per operation. The observed latency range was 51.17-72.29 ms/op.
Each operation includes fresh package loading, so this is proportional
admission evidence rather than a portable latency promise.

Non-mutating exact-rule dogfood completed without findings on Glippy at
`2f7b2e1f02bd51bdfbd0535b59c034f583fa5227` and
`go-libraries/pkg/prompts` at
`65acb13e8166e27c8a9723416214556f07d5e674`. The prompts run used a
task-owned prepopulated module cache with network lookup disabled, and its
pre-existing dirty worktree was byte-status identical afterward.

## Revisit Trigger

Revisit the representation when reviewed defects require alias identities,
ordered nested-call effects, nonlocal acquisition, receiver state contracts,
or concurrency-aware close serialization. Promote the rule only if concrete
resource contracts make the closed-state claim universal enough for the
default correctness preset.
