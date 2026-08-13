# Migrating From gofmt, gofumpt, Or golines

Gox is not a post-processing step for another Go formatter. A repository that
adopts Gox must make it the sole formatting authority for the selected files.
Running gofmt, gofumpt, or golines after Gox can create permanent formatting
churn.

Gox is the selected development identity. Its public binary and module contract
remain gated on the final naming audit and release work. Use a pinned, locally
built revision for a rehearsal; do not treat the current command path as a
stable release contract.

## Why The Outputs Differ

Gox deliberately does not promise a product-wide gofmt fixed point. The
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
Gox migration either. The exact Gox compatibility decision and evidence are in
[ADR 0004](decisions/0004-gofmt-compatibility.md). The current canonical output
is documented in the [formatter rules](formatter-rules.md).

## Rehearse Without Mutation

Build and pin the intended Gox revision, then inspect the repository from its
root:

```sh
gox fmt --check .
gox fmt --diff .
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
gox fmt --write ./path/to/non-generated/package
gox fmt --check ./path/to/non-generated/package
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
copy. Differences are expected when the migration reaches one of Gox's
documented divergence classes. Treat that result as proof of a workflow
conflict to remove at cutover, not as a reason to run both formatters.

Have an owner review the complete dedicated formatting-only diff. Do not mix
semantic changes, generated updates, or unrelated cleanup into the migration
change. Large mechanical diffs still require human review for comment
placement, readable wrapping, and repository-specific source conventions.

## Cut Over

Apply these changes together so the repository has one formatter authority:

1. replace gofmt, gofumpt, and golines checks in continuous integration and
   pre-commit hooks with the pinned Gox command;
2. configure editors to use Gox's stdin/stdout interface;
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
leave Gox-formatted source under a gofmt, gofumpt, or golines gate.

## Current Dogfood Evidence

A dedicated migration of the external `pkg/prompts` module changes 65 of 77 Go
files and reaches a zero-difference second Gox check. Its tests, race tests,
vet, module-metadata, documentation, lint, and nested comparison test gates
pass. The current formatter retains one source-authored blank line between
statement groups, including all 168 exact `t.Parallel()` grouping gaps in this
snapshot. Sixty-three files are not gofmt fixed points, so the migration
replaces the module's gofmt and goimports authorities.

The maintainer has selected `go-libraries/pkg/prompts` as the adoption target,
but has not reviewed or approved the dedicated
`feature/gox-prompts-adoption` commit. The first lint run exposed and drove a
Gox fix for external `//nolint` physical-line ownership; the refreshed lint
gate passes. Passing code gates still does not establish that the output is
readable enough for daily use. See the
[external dogfood record](research/external-dogfood-2026-08-11.md) for the
bounded evidence.
The dedicated
[`pkg/prompts` adoption review](research/prompts-adoption-review-2026-08-12.md)
provides the maintainer decision boundary and representative before/after
layouts.
