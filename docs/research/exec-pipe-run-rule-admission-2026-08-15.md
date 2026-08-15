# `exec-pipe-run` Rule Admission, 2026-08-15

## Decision

Admit `exec-pipe-run` to the default `correctness` preset at warning severity.
The typed rule reports `os/exec.Cmd.Run` when the same direct local command
identifier previously created a `StdoutPipe` or `StderrPipe` in that lexical
block.

## Defect And Authority

The Go `os/exec` documentation states that `Cmd.Wait` closes output pipes after
the command exits and that it is incorrect to call `Cmd.Run` with
`StdoutPipe` or `StderrPipe`. `Run` combines `Start` and `Wait`, so it can close
the pipe before the caller finishes reading. The compiler and default vet do
not reject this sequence.

The implementation resolves the exact standard-library method object, requires
an `os/exec.Cmd` receiver, and tracks the exact `go/types` object for a direct
local identifier. A custom type with the same method names does not report.

## Precision Boundary

The first contract follows calls in one lexical statement list. It accepts the
documented `Start`, read, then `Wait` sequence and does not report `Run` before
pipe creation. Reassigning the command variable clears its tracked pipe state.
Fields, aliases, helper-mediated pipe creation, and paths that cross nested
blocks remain conservative false negatives until a value-flow contract is
justified.

No automatic fix is offered. Replacing `Run` requires choosing where reading
finishes and how `Start`, read, and `Wait` errors combine; source syntax cannot
infer that behavior safely.

## Evidence And Cost

The focused regression failed first because the rule was absent. It now covers
both output-pipe methods, the accepted `Start` and `Wait` lifecycle, reversed
call order, command reassignment, custom lookalikes, exact diagnostic ranges,
and absence of fixes.

Five one-iteration complete-load samples on Darwin arm64 with an Apple M4 Max
ranged from 78.8 to 97.5 milliseconds, 1.79 to 2.36 MiB, and 14,243 to 14,677
allocations. Package loading dominates this proportional benchmark; the rule
uses the existing shared typed block traversal and builds no CFG or SSA.

Non-mutating exact-rule dogfood completed without findings or tool failure on
Glippy at base revision `7a1ca13` and `go-libraries/pkg/prompts` at revision
`a3ea0cb39145b4a973cecca86b6ed76fb0cb37a7`.

## Revisit Trigger

Broaden beyond direct local identifiers only when SSA or another shared value
representation can prove receiver identity without loading a more expensive
tier solely for this rule. Add a fix only when an exact error-ordering and read
completion contract is available.
