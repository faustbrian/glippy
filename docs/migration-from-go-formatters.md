# Migrating From gofmt, gofumpt, Or golines

Glippy is not a post-processing step for another Go formatter. A repository that
adopts Glippy must make it the sole formatting authority for the selected files.
Running gofmt, gofumpt, or golines after Glippy can create permanent formatting
churn.

Glippy is the selected development identity, and its collision review is
complete for the current binary and module contract. Publication remains gated
on final release work and explicit authorization. Use a pinned, locally built
revision for a rehearsal; do not treat an untagged command path as a stable
release.

## Why The Outputs Differ

Glippy deliberately does not promise a product-wide gofmt fixed point. The
currently recorded divergence classes are:

1. width-aware layouts that gofmt flattens;
2. retained import spec order that gofmt sorts;
3. preserved numeric literal spelling that gofmt normalizes;
4. preserved redundant parentheses that gofmt removes;
5. structural indentation without gofmt-style tabular alignment; and
6. preserved explicit versus implicit empty-statement spelling that gofmt
   normalizes.

Gofumpt adds policy beyond gofmt, and golines rewrites long lines around a
gofmt-compatible pipeline. They therefore cannot remain authoritative after a
Glippy migration either. The exact Glippy compatibility decision and evidence are in
[ADR 0004](decisions/0004-gofmt-compatibility.md). The current canonical output
is documented in the [formatter rules](formatter-rules.md).

## Rehearse Without Mutation

Build and pin the intended Glippy revision, then inspect the repository from its
root:

```sh
glippy fmt --check .
glippy fmt --diff .
```

Both commands are non-mutating. `--check` establishes whether any selected
file differs; `--diff` provides the reviewable proposed output. Resolve source,
configuration, generated-file, symlink, or internal errors before considering
the result an adoption diff.

## Rehearse Writes In A Disposable Copy

Create a disposable copy at a pinned repository revision. Select only explicit
paths whose generated-file policy and ownership have been reviewed. Start with
one non-generated module or package boundary rather than an entire repository:

```sh
glippy fmt --write ./path/to/non-generated/package
glippy fmt --check ./path/to/non-generated/package
```

The second command must report no differences. Do not weaken generated-file or
symlink refusal merely to obtain a full-tree result. Expand the selection only
after every newly included path has been classified.

Run the migrated repository's own verification against that same disposable
copy. The exact gates are repository-specific, but should include its tests,
race tests where supported, analyzers, builds, module-metadata checks, and
generator or generated-artifact checks. Passing those gates supports only the
behavior they exercise; it does not establish that the formatting diff is
acceptable.

Run the previous formatter in its non-mutating check mode over the migrated
copy. Differences are expected when the migration reaches one of Glippy's
documented divergence classes. Treat that result as proof of a workflow
conflict to remove at cutover, not as a reason to run both formatters.

Have an owner review the complete dedicated formatting-only diff. Do not mix
semantic changes, generated updates, or unrelated cleanup into the migration
change. Large mechanical diffs still require human review for comment
placement, readable wrapping, and repository-specific source conventions.

## Cut Over

Apply these changes together so the repository has one formatter authority:

1. replace gofmt, gofumpt, and golines checks in continuous integration and
   pre-commit hooks with the pinned Glippy command;
2. configure editors to use Glippy's stdin/stdout interface;
3. disable secondary LSP or editor formatting passes; and
4. land the formatter configuration, workflow changes, and formatting-only
   source diff as one coordinated migration.

The [editor integration guide](editor-integration.md) documents the supported
stdin/stdout contract and examples. The
[CI and pre-commit guide](ci-and-precommit.md) provides a pinned development
workflow and the non-mutating gate contract.

## Roll Back

Keep the migration as one formatter-only mechanical change so it can be
reverted coherently. A rollback must restore the previous formatter's CI,
hooks, and editor authority together with the previous source layout. Do not
leave Glippy-formatted source under a gofmt, gofumpt, or golines gate.

## Current Dogfood Evidence

A refreshed adoption of the external `pkg/prompts` module is integrated at
`5eb1b997`. That immutable tree contains 90 Go files. The original approved
migration reached a zero-difference second Glippy check and passed its recorded
formatting, module metadata, test, race, vet, documentation, lint, and nested
module gates. The formatter retains source-authored blank lines between
statement groups while making Glippy the module's sole formatting authority.

The maintainer approved Phase 2 and the complete `go-libraries/pkg/prompts`
adoption layout on 2026-08-13. The current adoption pins Glippy candidate
`724d8a2` on `faustbrian/golib` `main`. Its final integration commit changes 45
module files, including 42 Go files; the 92-file delta from the original
approved baseline also contains intervening behavior and test work and MUST NOT
be represented as one formatter-only patch. The first lint run exposed and
drove a Glippy fix for external `//nolint`
physical-line ownership; the refreshed lint gate passes. See the
[external dogfood record](research/external-dogfood-2026-08-11.md) for the
bounded evidence and the
[integrated delivery record](research/prompts-adoption-delivery-evidence-2026-08-22.md)
for exact current source counts and ancestry. The later complete-package gate
was reported historically but has no retained immutable result artifact and is
not current release evidence.
The dedicated
[`pkg/prompts` adoption review](research/prompts-adoption-review-2026-08-12.md)
provides the maintainer decision boundary and representative before/after
layouts.
