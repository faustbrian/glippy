# `ineffective-break` Rule Admission, 2026-08-12

## Defect And Existing-Tool Boundary

An unlabeled `break` terminates the innermost `for`, `switch`, or `select`.
When it is the final statement of a switch case or select clause directly
inside a loop, it exits only the inner construct at the point where that
construct would end anyway. It therefore cannot terminate the surrounding
loop and commonly records the wrong control-flow intent.

Go 1.26.5 `go vet ./...` exited successfully for a disposable module
containing a final unlabeled `break` in a select clause inside a loop. The
compiler also accepts the source. Current Staticcheck independently defines
the defect as
[SA4011](https://github.com/dominikh/go-tools/blob/d69e7ee19e2d79b721aa696626cea310c807dd3e/staticcheck/sa4011/sa4011.go)
at `go-tools` commit `d69e7ee19e2d79b721aa696626cea310c807dd3e`.
Its current implementation inspects direct switches and selects in `for` and
`range` bodies, excludes labeled breaks, and recognizes one final conditional
level.

Two reviewed public fixes demonstrate distinct real outcomes:

- [Quench commit
  `2f74ecbde8dbae435fb378bd61025b518f1309ec`](https://github.com/qecko-labs/Quench/commit/2f74ecbde8dbae435fb378bd61025b518f1309ec)
  replaced a final select-case `break` with a function return so an error stops
  the surrounding producer loop.
- [NVIDIA NVCF commit
  `1de472145a013e688d1ea98f5590b0699ae5d13b`](https://github.com/NVIDIA/nvcf/commit/1de472145a013e688d1ea98f5590b0699ae5d13b)
  removed two final select-case breaks that had no effect.

The diagnostic therefore adds a correctness check absent from the default Go
toolchain while leaving the required repair to the developer.

## Contract

`ineffective-break` is enabled as a warning in the `correctness` preset. It
requires only syntax, subscribes to `for` and `range` statements, excludes
generated files, and offers `remove-break` as a suggestion fix. The exact
unlabeled `break` token is both the primary range and the deletion range, so
adjacent comments and trivia remain available to formatter normalization.

The rule inspects ordinary switches and selects that are direct statements of
the loop body. A break reports only when it is the clause's final statement or
the final statement of either branch of one final `if`. Labeled breaks, breaks
followed by another clause statement, switches outside loops, and switches
nested behind another block or loop do not report. Type switches and deeper
terminal conditional nesting remain explicit limitations until dedicated
fixtures and need justify expanding the traversal.

Removing the statement preserves the current control flow, but returning or
adding a loop label may be the intended repair. Gox therefore classifies
removal as a suggestion rather than a safe default: ordinary `--fix` reports
and preserves it, while explicit `--fix-suggestions` removes only the break
token before the standard reparse, format, validation, and atomic-write path.

## Behavioral Evidence

The red rule suite produced no diagnostics and treated the new suppression as
an unknown rule before registration. Focused tests now prove exact ranges for
ordinary and range loops, switch and select clauses, final conditional
branches, nested-loop ownership, labeled and effective breaks, out-of-scope
nesting, generated-file exclusion, suppression, severity overrides, suggestion
metadata, and the exact deletion edit. The public lint paths prove default
registration, warning severity, exact range and message fields, ordinary
`--fix` non-mutation, explicit suggestion selection, comment retention,
formatter normalization, reanalysis, and findings or success exit status.
`gox explain ineffective-break` renders the canonical limitations and
suggestion contract.

## Cost And Dogfood Signal

On Darwin arm64 with an Apple M4 Max and Go 1.26.5, five 200 ms benchmark
samples over 100 loop/select defects ranged from 90.303 to 158.753 microseconds
per complete source-backed correctness run. The median was 128.191
microseconds, with approximately 222.5 KiB and 1,678 allocations per run. This
includes source-backed shared traversal, both default correctness rules,
diagnostic validation, and ordering; it is a scaling observation rather than a
latency threshold.

The default correctness preset then ran without mutation over 7,732 selected Go
files:

| Corpus | Revision or state | Files | Diagnostics |
| --- | --- | ---: | ---: |
| Gox | current implementation worktree | 103 | 0 |
| go-libraries | `24d1976e87ec27ce4944e40b483f45a8543898bf` | 5,133 | 0 |
| ack | `912ca202d24b25fce9e71f067c9f2e396539bc9b` | 54 | 0 |
| ics | `73f4702657113e1e9a06ef2425fbf45cd4b5a05c` | 15 | 0 |
| tarvero | `3ceb6ae16952b49ba114ddb2c63abd23290cec08` | 1,733 | 0 |
| vuja | `870bc063adfe94fd23abbc6061c73f9c737935fe` | 694 | 0 |

External runs used task-owned immutable `git archive` snapshots and completed
without suppression, configuration, source, or tool errors. Zero findings
establish no observed false-positive noise in this sample; they do not prove
recall. The focused fixtures and two public fixes provide positive evidence.

## Admission Decision

Admit `ineffective-break` as the second built-in correctness rule. Revisit the
direct-body, type-switch, and one-level-conditional limits only when missed
defects justify the additional traversal. Revisit default enablement if
dogfood or user reports reveal a false-positive class outside the current
contract.
