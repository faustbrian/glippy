# Self-Dogfood Audit, 2026-08-11

## Scope

Commit `8d6c518` was formatted through the path-based check and write surfaces
on an Apple M4 Max with Go 1.26.5. Check mode processed all 32 discovered Go
files without mutation or validation failure: 30 were different and two were
unchanged. Write mode then formatted a disposable clone successfully. The
resulting 30-file diff contained 6,433 insertions and 3,470 deletions.

This is adoption evidence, not a release corpus. The disposable clone was
removed after the observed layouts and aggregate counts were captured.

## Classification

The audit distinguishes three classes:

- Expected dialect changes include hard-line blocks, width-aware calls and
  lists, removed gofmt alignment, and deterministic blank-line normalization.
- A confirmed control-flow defect stranded `if`, `for`, and `switch` before an
  uncommented operand. The repository snapshot contained 82 such lines. The
  lowering layer offered a keyword-adjacent break before the operand's own
  grammar-aware group, so it also added an unnecessary indentation level.
- Two formatter-readability findings remain open: 20 method receivers broke
  after `func`, and 207 lines ended in a selector component followed by `.`.
  These counts identify review targets; they do not by themselves decide that
  every occurrence is defective.

The control-flow fix keeps the keyword with its first operand, retains
semicolon breaks for initialized headers, and retains the range break after
`range`. Reformatting the same snapshot reduces stranded control keywords from
82 to zero. All 32 files still validate and 30 still differ; receiver and
selector counts are unchanged and require separate root-cause work.

## Gate Impact

Successful parsing, equivalence, and idempotency did not establish acceptable
human layout. The Phase 1 readability exit claim is therefore withdrawn and
progress returns to the proven 20% source/trivia and document-renderer state.
Phase 2 CLI capabilities remain implemented but cannot advance progress until
the reopened formatter dogfood findings are resolved and the complete
migration diff is reviewed.
