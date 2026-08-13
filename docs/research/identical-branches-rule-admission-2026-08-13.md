# `identical-branches` Rule Admission, 2026-08-13

## Defect And Existing-Tool Boundary

The rule detects a direct `if`/`else` pair whose statement blocks are
structurally identical. This usually means a copied branch was not updated or
the condition no longer selects behavior. The condition may still have effects,
so deleting the conditional is not automatically safe.

Go 1.26.5's default vet analyzer catalog has no identical-branch check. Its
`assign` analyzer does cover self-assignment, which is why Gox does not admit a
duplicate self-assignment rule merely to increase rule count. Current Revive
implements the same direct-pair boundary in
[`identical_branches.go`](https://github.com/mgechev/revive/blob/1de8243783d480e24c0db1a3dc45976aeaf715e9/rule/identical_branches.go).
Current Clippy implements the corresponding Rust diagnostic in
[`if_same_then_else.rs`](https://github.com/rust-lang/rust-clippy/blob/9a73ad846274efca140b1d2ea316b830fa1fb8de/clippy_lints/src/ifs/if_same_then_else.rs).

The defect class occurs in public Go maintenance. Grafana App SDK commit
[`d70af66fa0289d0af446fceb2bf842c28d4dee1d`](https://github.com/grafana/grafana-app-sdk/commit/d70af66fa0289d0af446fceb2bf842c28d4dee1d)
records and addresses an adjacent-identical-branch diagnostic discovered while
migrating its lint policy. That example is an `else if` chain, which is broader
than the initial Gox scope, but establishes that copied branch bodies occur in
real Go repositories and require maintainer judgment.

## Contract And Safety

`identical-branches` is an opt-in `suspicious` warning. It requires only syntax,
subscribes to `if` statements, excludes generated files, and compares only a
direct `if` body with its final `else` block. The primary range is the duplicate
`else` block and a related range identifies the matching `if` block.

The comparison uses deterministic formatted AST structure, so whitespace does
not affect the result while identifier, operator, literal, and statement shape
remain significant. An `else if` chain, a missing `else`, or distinct statement
structure does not report. Any comment inside the complete statement excludes
the candidate because comment text may document an intentional distinction that
the AST comparison cannot model.

No fix is offered. Removing the condition could remove its evaluation and side
effects; choosing which duplicated body is wrong requires developer intent.

## Behavioral And Cost Evidence

Focused tests first failed because the rule was absent from the registry. They
now prove direct-pair diagnostics, exact primary and related ranges, distinct,
commented, missing-else, and else-if exclusions, source suppression, generated
file policy, metadata, and no-fix behavior. The complete `internal/rules` suite
passes with the new suspicious selection.

On Darwin arm64, Apple M4 Max, and Go 1.26.5, five 200 ms benchmark samples over
100 identical pairs ranged from 293.991 to 470.682 microseconds per complete
source-backed analysis. The median was 321.330 microseconds with approximately
402 KiB and 5,585 allocations per run.

Non-mutating dogfood with only this rule enabled analyzed 135 Gox files with
zero diagnostics, suppression problems, baseline problems, or tool errors. This
establishes no observed noise in the owned sample; the focused fixture and
public Go lint finding provide positive defect evidence.

## Admission Decision

Admit `identical-branches` to the opt-in `suspicious` preset. Do not add a
self-assignment rule while the default Go vet `assign` analyzer already owns
that defect. Revisit adjacent `else if` and switch-branch comparison only after
separate fixtures and noise evidence define their comment and diagnostic-range
contracts.
