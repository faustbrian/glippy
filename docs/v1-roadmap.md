# Roadmap To v1

Glippy is one opinionated Go CLI combining width-aware formatting, linting,
safe fixes, repository checks, machine output, stdin/stdout, and the existing
LSP. The v1 objective is stable tool behavior, not an integration ecosystem or
community-adoption program.

## v0.6: Real-World Rule Validation

Status: complete. The exact evidence and accepted boundaries are recorded in
the [v0.6 exit audit](research/v0.6-exit-audit-2026-08-25.md). At v0.6 closure,
stable-v1 roadmap progress advanced to 65% and v0.7 became active.

- Run the pinned public-repository corpus with `default`, `recommended`,
  `strict`, and `pedantic`.
- Manually adjudicate every default and recommended finding.
- Compare coverage with `go vet` and Staticcheck v0.8.1.
- Record false positives, missed defects, crashes, unsupported source or build
  states, latency, and memory.
- Convert demonstrated gaps into an evidence-backed rule queue.
- Use the opt-in `nursery` group for candidates that need broader validation.

Public repositories are immutable validation inputs. Glippy does not modify
them or seek upstream adoption.

## v0.7: Semantic Depth And Credible Fixes

Status: complete. The exact evidence and conservative boundaries are recorded
in the [v0.7 exit audit](research/v0.7-exit-audit-2026-08-25.md). Stable-v1
roadmap progress is 75%, with v0.8 now active.

- Improve bounded interprocedural alias, interface, recursion, and ownership
  reasoning.
- Expand high-signal concurrency, lifecycle, error-flow, and standard-library
  rules from corpus evidence.
- Grow pedantic and performance coverage only where measured examples justify
  it.
- Add safe and suggestion fixes only when their transformation contract is
  provable.
- Validate rule, fix, formatter, and suppression interaction.

Project semantic contracts remain a built-in configuration feature. They do
not become a package marketplace or plugin system.

## v0.8: Large-Repository Hardening

Status: complete. Exact corpus run `32962070813` passed all 20 jobs at
`3afa56c`, classified all 334 default and recommended findings, retained zero
known default false positives, and produced no new rule candidate. Formatter,
transactional safe-fix preview, source-fidelity, build-variant, upgrade,
macOS/Linux resource-budget, and cross-runner reproducibility gates are proven.
The accepted unsupported-source, generated-prerequisite, cgo-coverage, and
nested-module boundaries remain explicitly dispositioned rather than being
silently excluded. This milestone advanced stable-v1 roadmap progress to 85%
and activated v0.9.

- Repeat the complete corpus at exact revisions, account for every formatter
  execution and fidelity failure, and manually classify every default and
  recommended diagnostic. Canonical formatter differences are aggregate
  adoption evidence; parse, equivalence, comment, directive, or idempotency
  failures require individual investigation and disposition.
- Prove formatter idempotency and source fidelity on the corpus.
- Establish macOS and Linux cold, warm, editor, and memory budgets in isolated
  CI.
- Exercise modules, workspaces, tests, build tags, cgo, generated code, and
  large files.
- Rehearse fixes without committing external repository modifications.
- Rehearse configuration and baseline upgrades across Glippy prereleases.
- Document plain GitHub Actions, Woodpecker, generic shell CI, stdin/stdout,
  and existing-LSP invocation.

This milestone does not include a custom Action, editor plugin, package-manager
integration, contract marketplace, hosted service, or upstream adoption work.

## v0.9: Stability Freeze And Tagless Release Candidate

Status: complete at source-bearing revision `18004f2`. The formatter, rule,
profile, configuration, CLI, completion, reporter, exit-code, and
machine-schema contracts are frozen in executable v1 fixtures. Corpus run
`33040254261` passed all 20 jobs with zero known default or recommended false
positives and no changed finding fingerprint. CI `33056719958` and native
release-budget/reproducibility run `33056742540` passed the final audit-bearing
candidate, and independent review found no issues. Stable-v1 roadmap progress
is 95%; tag publication and post-publication verification remain.

- Freeze formatter output, rule IDs, profiles, configuration, CLI behavior,
  exit codes, and machine schemas.
- Resolve every confirmed default false positive and release-blocking latency
  or memory regression.
- Refresh comparisons with Oxfmt, Oxlint, Clippy, rustfmt, gofmt, vet, and
  Staticcheck.
- Run final corpus, fuzz, race, integration, fix-safety, reproducibility, and
  independent-review gates.
- Produce unsigned, tagless macOS and Linux release-candidate archives.
- Rehearse upgrades from historical Gox and Glippy prerelease snapshots.

## v1.0: Stable CLI

Status: active. Pre-publication acceptance is complete. The authorized
`v1.0.0` tag transaction and post-publication asset, checksum, version, and
attestation verification remain.

The exact candidate must prove deterministic and source-preserving formatting,
zero known default-correctness false positives over the audited corpus,
credible strict and pedantic signal, deterministic diagnostics, transactional
safe fixes, bounded macOS/Linux resource use, stable public contracts, and
reproducible archives with checksums and GitHub provenance.

Installation requires only GitHub Releases or `go install`. Documentation must
include concise GitHub Actions, Woodpecker, generic editor, and migration
examples. A tag and GitHub release may be created only after maintainer review
and explicit authorization of the final candidate.
