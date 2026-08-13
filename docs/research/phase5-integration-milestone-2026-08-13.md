# Phase 5 Integration Milestone, 2026-08-13

## Decision

The editor, continuous-integration, release, and formatter-migration surfaces
satisfy the 95% progress milestone. This advances overall progress from 90% to
95%; it does not establish a stable release candidate, authorize a tag, or
activate publication.

The current integration evidence is bound to commit
`aa07ff431ccf320d8e56a258d7715b3f190ebd98` on `main`. GitHub Actions run
[`31686057237`](https://github.com/faustbrian/gox/actions/runs/31686057237)
completed successfully on that exact revision.

## Integration Evidence

| Surface | Current contract and evidence | Result |
| --- | --- | --- |
| Editor formatting | Standard input, `--stdin-filepath`, and fragment modes use the same formatter and configuration contracts. Conform.nvim and Helix setup, failure behavior, and the deliberate no-LSP boundary are documented; ADR 0013 records code-action ordering for any later editor action. | Complete for the initial release |
| Continuous integration | The repository-owned `CI` workflow uses pinned actions, Go 1.26.5, disposable Go caches, read-only permissions, full tests, race tests, vet, tidy-diff, a fresh build, and non-mutating `gox check ./...`. Run `31686057237` passed on the exact integration revision. | Complete |
| Pre-commit | The versioned-hook contract uses the same non-mutating combined check, documents partial-staging behavior, and requires a pinned development build before release. | Complete |
| Release artifacts | The clean-revision builder emits deterministic Darwin and Linux archives for amd64 and arm64 plus a versioned manifest and SHA-256 checksums. Rehearsals prove cross-environment reproduction and target execution within their recorded native or emulated boundaries. | Complete; the final-candidate native rerun passed |
| Publication and provenance | The tag-only GitHub workflow rebuilds the exact tag with pinned actions, requests GitHub OIDC artifact attestations, and publishes checksummed artifacts through GitHub Releases. Ordinary pushes and workflow dispatches cannot publish. | Complete integration; activation requires the authorized final tag |
| Formatter migration | The gofmt, gofumpt, and golines divergence classes, rehearsal, cutover, rollback, and sole-formatter requirements are documented. The approved `pkg/prompts` migration and Gox self-adoption exercise the contract. | Complete |
| Shell and installation surfaces | Bash, Zsh, and Fish completion generation is implemented and documented. Development evaluation remains revision-pinned; the supported release installation path is intentionally withheld until the first authorized GitHub Release. | Complete for the pre-release boundary |
| Support and compatibility | Supported Go versions, macOS/Linux targets, unsupported Windows and filesystem boundaries, vulnerability reporting, formatter changes, rule IDs, configuration migration, machine schemas, and deprecation policy are published. | Complete |

GitHub- and SARIF-specific diagnostic reporters remain unnecessary because no
validated adoption consumer requires them. An LSP remains unnecessary because
the measured standard-input formatter path satisfies the current editor
journey without adding a long-lived service or protocol surface.

## Current Verification

Before the integration commit, the complete local build, ordinary tests, race
tests, vet, tidy-diff, workflow validation, and combined Gox check passed with
Go 1.26.5 and task-owned disposable build caches. The pushed commit then passed
the repository-owned GitHub workflow from a fresh checkout and empty Go caches.
The self-formatted tree is a zero-difference Gox fixed point.

## Release Candidate Refresh

Exact candidate `06cce4a` now has successful CI run `31697040231`, a complete
5,138-file corpus result with no errors, twelve passing fuzz campaigns, and
successful four-runner release-budget and cross-runner reproducibility run
`31697171821`. The requirement-level result is recorded in the
[final acceptance audit](final-acceptance-audit-2026-08-13.md).

The technical naming search is refreshed, but the documented exact-name and
trademark-risk boundary still needs the maintainer's final public-name
acceptance. Deterministic dependency-notice packaging is now implemented, while
the selected 0BSD project license is also required and packaged. Refreshed
candidate evidence is complete; the maintainer's personal candidate review
remains pending. Only after the remaining human gates may a tag be authorized
to activate GitHub publication and provenance. No tag or release exists, so
progress remains 95%.
