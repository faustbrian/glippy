# Roadmap To v1

Glippy is one opinionated Go CLI combining width-aware formatting, linting,
safe fixes, repository checks, machine output, stdin/stdout, and the existing
LSP. The v1 objective is stable tool behavior, not an integration ecosystem or
community-adoption program.

## v0.6: Real-World Rule Validation

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

- Repeat the complete corpus at exact revisions and classify every formatter
  divergence and diagnostic.
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

The exact candidate must prove deterministic and source-preserving formatting,
zero known default-correctness false positives over the audited corpus,
credible strict and pedantic signal, deterministic diagnostics, transactional
safe fixes, bounded macOS/Linux resource use, stable public contracts, and
reproducible archives with checksums and GitHub provenance.

Installation requires only GitHub Releases or `go install`. Documentation must
include concise GitHub Actions, Woodpecker, generic editor, and migration
examples. A tag and GitHub release may be created only after maintainer review
and explicit authorization of the final candidate.
