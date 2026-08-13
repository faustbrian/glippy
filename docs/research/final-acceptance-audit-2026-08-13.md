# Final Acceptance Audit, 2026-08-13

## Decision

The refreshed candidate satisfies the formatter, linter, fixer, platform,
licensing, reproducibility, and performance gates. Overall progress remains 95%
because public-name risk acceptance, maintainer review, and the authorized
tag-driven provenance/publication transaction are still pending.

`Proven` means the current candidate has direct implementation, behavioral, or
release evidence. `Ready` means the implementation is complete but its
user-authorized external activation has deliberately not occurred. `Pending`
means a required maintainer decision remains.

## Formatter

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Canonical flat and broken supported syntax | Proven | `docs/formatter-rules.md`, the complete `testdata/format` contracts, and width-boundary tests in `internal/format` |
| Motivating compressed examples become readable | Proven | Motivating fixtures and `TestFormatExpandsMotivatingHostileGo`; approved `pkg/prompts` adoption |
| Deterministic documented width | Proven | ADR 0003, document renderer tests, representative-width corpus tests, and self-adoption |
| Output reparses for Go 1.25 and 1.26 | Proven | Supported-version routing tests plus mandatory post-render parsing and equivalence validation |
| Byte idempotency | Proven | Every golden/corpus formatter test, formatter fuzzing, and zero-difference Gox self-check |
| Normalized syntax equivalence over the release corpus | Proven | Complete 5,138-file pinned corpus result with zero errors |
| Comments and directives retain identity and ownership | Proven | Source ledger, directive corpus, dense comment fixtures, suppression-anchor regressions, corpus, and fuzzing |
| Trailing commas and semicolons remain syntax-correct | Proven | Delimited-list, binary, classic-for, empty-statement, and semicolon fixtures plus reparsing |
| Gofmt compatibility matches the published decision | Proven | ADR 0004, divergence classification, migration guide, self-adoption, and approved `pkg/prompts` migration |
| Check and stdout do not mutate | Proven | CLI and integration transaction tests and the repository-owned CI check |
| Write mode validates and replaces atomically | Proven | Snapshot replacement, stale-source, cancellation, permission, rollback, and platform integration tests |
| Generated, vendor, ignored, symlink, and root behavior | Proven | Discovery/configuration tests, ADR 0007, and Darwin/APFS plus Linux/overlayfs evidence |
| Editor and repository performance budgets | Proven | Native four-runner release-budget run `31697171821` |

## Linter

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Correctness-focused defaults with acceptable signal | Proven | Two-rule correctness default, six-repository noise record, clean Gox and approved `pkg/prompts` dogfood |
| Rules execute only declared tiers | Proven | Registry validation, maximum-tier scheduling, and syntax/types/CFG/SSA routing tests |
| Syntax linting avoids typed loading | Proven | File-oriented syntax driver and CLI routing tests |
| Typed, CFG, and SSA representations are shared and bounded | Proven | Phase 4 audit, graph limits, shared representation tests, and cache/cancellation gates |
| Diagnostics are precise, stable, and deterministic | Proven | Exact source identity/range validation and canonical parallel-output ordering tests |
| Suppressions are narrow, auditable, and formatter-stable | Proven | Exact-rule ownership, reason/expiry, malformed/unused diagnostics, and anchor-preservation tests |
| Every admitted rule has behavioral tests and canonical docs | Proven | Seven admission records and generated `docs/lint-rules.md` freshness test |
| `go/analysis` boundary is documented and proven | Proven | Syntax and package analyzer adapter/fact suites plus ADRs 0005 and 0006 |
| Machine schemas are versioned | Proven | Schema-version-1 JSON reference and reporter tests |
| Rule IDs and presets have a compatibility policy | Proven | Metadata registry, `docs/compatibility-policy.md`, and generated catalog |

## Fixes

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Safe, suggestion, and unsafe classes are distinct | Proven | Rule metadata validation and independent CLI authorization tests |
| Safe claims require semantics-preserving evidence | Proven | Admission records; no built-in fix is promoted to safe without that contract |
| Stale and overlapping edits cannot apply silently | Proven | Exact digests, UTF-8 ranges, all-participant conflict rejection, and fix fuzzing |
| Results reparse, format, and validate before write | Proven | Coordinator and single-file transaction suites |
| Failed validation preserves original source | Proven | Rollback, formatter-refusal, stale-snapshot, and filesystem failure tests |
| Fix output is deterministic and idempotent | Proven | Canonical edit ordering, reanalysis, formatter normalization, integration tests, and fuzzing |
| Multi-file failure transaction | Proven as absent | Gox intentionally exposes only mature single-file transactions; no multi-file atomicity claim exists |

## Product And Operations

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| One binary exposes the contracted commands | Proven | CLI integration tests, command reference, completion generation, and native candidate execution |
| Configuration discovery and migration are stable | Proven | Typed schema, discovery/precedence/override tests, and compatibility policy |
| Exit codes separate findings from failures | Proven | Text/JSON reporter contract and CLI integration tests |
| Modules, workspaces, tests, tags, GOOS, and GOARCH | Proven | Phase 4 loader matrix and native supported-target workflow |
| Cancellation and bounded concurrency | Proven | Scheduler, loader, analysis, write, and CLI cancellation tests plus explicit limits |
| Cache keys and corruption recovery | Proven | ADR 0008, cache specification, invalidation/corruption suites, and cache fuzzing |
| Editor and CI integrations use stable surfaces | Proven | Stdin/stdout editor guide, pinned CI/pre-commit contracts, and successful candidate CI |
| Release artifacts are reproducible and checksummed | Proven | Native run `31697171821` produced byte-identical license-bearing six-file sets on all four supported targets |
| Release provenance and publication | Ready | Tag-only pinned GitHub workflow uses OIDC artifact attestations and GitHub Releases; activation is prohibited before maintainer review |
| Naming and module-path audit | Pending | Technical refresh documents substantial exact-name collisions; final maintainer risk acceptance is required |
| Project and third-party licensing | Proven | The tracked SPDX `0BSD` project license plus MIT, BSD, and Go patent notices are deterministic archive entries, and the builder rejects either license artifact when absent |
| Supported-version and vulnerability policies | Proven | `docs/supported-go-versions.md`, `docs/support-policy.md`, and `SECURITY.md` |
| Final corpus, fuzz, race, integration, and performance | Proven | Candidate CI `31697040231`, twelve fuzz campaigns, pinned corpus, and release-budget run `31697171821` |
| Multiple real repositories have documented dogfood adoption | Proven | Gox self-adoption and maintainer-approved `pkg/prompts` coordinated migration |
| Public release candidate personally reviewed | Pending | The maintainer explicitly reserved this gate before any tag or release |

## Architecture

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Separate formatter and linter over a shared frontend | Proven | Package boundaries and ADR 0005 |
| Layout rules are not duplicated as lint rules | Proven | Formatter dialect and seven admitted non-layout lint rules |
| Semantic fixes do not hide in formatting | Proven | Formatter equivalence contract and explicit fix engine |
| Standard Go frontend is reused | Proven | `go/parser`, `go/ast`, `go/token`, `go/scanner`, `go/packages`, `go/types`, analysis, CFG, and SSA audit/implementation |
| Source fidelity does not depend on AST alone | Proven | Immutable bytes, token/trivia ledger, directive ownership, and reconstruction fuzzing |
| Document rendering is bounded | Proven | Iterative renderer, fit budgets, adversarial allocation tests, and native budgets |
| Expensive analysis is demand-driven and shared | Proven | Requirement tiers, maximum-tier scheduler, shared graphs, and cold/warm evidence |
| No premature plugin or public package surface | Proven | Internal packages and controlled `go/analysis` adapter only |
| Current Oxfmt and Oxlint reviewed at readiness | Proven | `docs/research/oxc-audit.md`, refreshed against current Oxc and website revisions |
| Rule expansion follows the foundation audit | Proven | Rule admission gate, individual records, and post-foundation roadmap |

## Release Boundary

This audit does not convert a development revision into a release. The final
sequence remains: maintainer name-risk decision, maintainer candidate review,
explicit tag authorization, then successful GitHub provenance and publication.
Any code or contract change before that sequence invalidates the affected
candidate evidence and requires a bounded rerun.
