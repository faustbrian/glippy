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
- The initial audit also found 20 method receivers broken after `func` and 207
  lines ending in a selector component followed by `.`. These counts identify
  review targets; they do not by themselves decide that every occurrence is
  defective.

The control-flow fix keeps the keyword with its first operand, retains
semicolon breaks for initialized headers, and retains the range break after
`range`. Reformatting the same snapshot reduces stranded control keywords from
82 to zero. All 32 files still validate and 30 still differ; receiver and
selector counts are unchanged and require separate root-cause work.

## Receiver Follow-Up

The committed control-prefix snapshot at `08638c6` reproduced all 20 receiver
breaks. A receiver used the ordinary parameter-list group, whose fit decision
included the following method signature as continuation. Width pressure in
later parameters therefore broke a short receiver that fit by itself.

Treating a comment-free single receiver as an atomic receiver construct reduces
the receiver-prefix count from 20 to zero. The same 32 files validate and the
formatted snapshot is idempotent. The newer source snapshot contains 193
selector-pattern review targets.

## Selector Follow-Up

The selector review traced 191 of those 193 lines to terminal calls whose
selector callee fit but whose independently breakable argument list did not.
The selector group measured the complete argument continuation and broke the
callee before the argument group could select its own layout.

A bounded document lookahead now measures the selector callee with the opening
delimiter while attaching the argument list as an independent tail. An empty
terminal call still counts both delimiters, and a broken callee gives its
arguments the selector continuation indentation. Reformatting the same 32-file
snapshot changes only `benchmarks/corpus_test.go` and
`internal/format/format.go` relative to the receiver-fixed output, validates
every file, and is idempotent. Selector-pattern targets fall from 193 to two;
both lines are one intentionally broken, deeply indented
`betweenConditionAndPost[len(...)-1].Range.End` chain that cannot fit flat.

The final 30-file migration diff contains 5,893 insertions and 3,333 deletions.
Review of that complete snapshot found no remaining unclassified control-prefix,
receiver, or selector layout. The motivating hostile examples, comments,
directives, exact-width boundaries, and canonical broken forms remain covered
by the formatter and corpus suites.

## Gate Impact

The three readability classes that invalidated the earlier Phase 1 claim now
have root-cause fixes and complete snapshot classifications. With the current
corpus, equivalence, idempotency, fuzz, bounded-rendering, and migration-review
evidence, the Phase 1 formatter prototype exit gate is restored. Existing safe
filesystem, configuration, and CLI proof supports the 45% Phase 2 milestone;
production-usable formatter release requirements remain open.

## Repository Adoption, 2026-08-13

The earlier disposable audit has now become a repository-owned migration.
Starting from `333b88d`, Gox selected 121 tracked Go files and formatted them
through its validated write path. The resulting Go source patch contains
22,196 insertions and 10,844 deletions; no non-Go source was rewritten by the
formatter. A root `.gox.toml` fixes schema version 1, width 100, tab width 8,
and the correctness preset as the repository policy.

Review of the complete migration exposed two ordinary assignment targets whose
selector chains broke only because the right-hand call did not fit. Assignment
lowering now measures an ordinary left-hand side through its operator and lets
the right-hand side choose its layout independently. The exact production
shape has a focused regression, independently over-width assignment targets
remain breakable, and communication-clause assignments retain their coupled
receive layout.

The formatted tree is a zero-difference `gox check ./...` fixed point. The
complete ordinary and race test suites, `go vet ./...`, and `go mod tidy
-diff` pass with Go 1.26.5 and task-owned disposable build and module caches.
The first CI rehearsal incorrectly exported `GOWORK=off`, which disabled the
temporary workspaces owned by two package-loading tests. Those exact tests fail
with the override and pass without it; removing the override restores the full
suite without changing formatter output or package-loading behavior.

The repository now carries a non-mutating GitHub Actions gate that builds Gox
from the checked-out source and runs its own combined check after the test,
race, vet, and module-metadata gates. This makes Gox the repository's sole
formatter authority and converts the earlier snapshot review into continuous
self-adoption evidence. Final maintainer review of the release candidate still
owns the human acceptance boundary for this complete repository layout.
