# Glippy v1 Pre-Publication Acceptance Audit, 2026-08-27

## Decision

Status: complete; publication closed by the separate
[v1.0.0 release evidence](release-v1.0.0-evidence-2026-08-27.md).

The accepted source-bearing candidate is
`18004f2eba702471e03c9ab5656c905cf470e946`. CI run `33056719958` and native
budget/reproducibility run `33056742540` passed at that revision. Final fuzz run
`33040254402` and corpus run `33040254261` carry forward from `8f52f20` because
the intervening changes cannot affect formatter, lint, fix, fuzz, or corpus
results. The canonical corpus adjudication retains all 334 prior finding
fingerprints, zero false positives, and 11 accepted gaps. Independent final
review found no issues. Pre-publication acceptance advanced stable-v1 roadmap
progress to 95%; the subsequent verified release transaction advances it to
100%.

`Proven` means the current candidate has direct implementation, behavioral, or
external evidence for the complete stated boundary. `Pending` means the
criterion cannot yet support release readiness.

## Formatter

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Canonical flat and broken layout for supported syntax | Proven | Formatter rules, complete golden fixtures, hostile corpus contracts, and frozen v1 formatter snapshots |
| Motivating compressed examples become readable | Proven | Motivating fixtures, formatter tests, Glippy self-adoption, and approved `pkg/prompts` adoption |
| Width decisions are deterministic and documented | Proven | Width ADR, document renderer tests, formatter documentation, and native frozen-contract reproduction |
| Output reparses for Go 1.25 through Go 1.27 | Proven | Supported-version routing plus mandatory parse and equivalence validation in CI `33056719958` |
| Formatting is byte-idempotent | Proven | Owned fixtures and fuzzing pass; corpus run `33040254261` completed every supported formatter unit without an idempotency failure |
| Normalized syntax equivalence passes the release corpus | Proven | Corpus run `33040254261` completed every supported formatter unit without a parse or equivalence failure |
| Comments and directives retain identity and ownership | Proven | Owned regressions and fuzzing pass; corpus run `33040254261` completed every supported formatter unit without comment or directive failure |
| Trailing commas and semicolon handling are syntax-correct | Proven | Delimited-list, binary-expression, classic-for, empty-statement, semicolon, and reparsing contracts |
| Gofmt compatibility matches the published decision | Proven | ADR 0004, divergence inventory, migration guide, and frozen formatter contracts |
| Check and stdout modes do not mutate | Proven | CLI integration contracts and non-mutating corpus runner design |
| Write mode is validated and atomic within supported limits | Proven | Native write/fix jobs on all four targets in run `33056742540` |
| Generated, vendor, ignored, symlink, and root behavior | Proven | Discovery/configuration contracts and documented Darwin/Linux local-filesystem evidence |
| Formatter performance meets release budgets | Proven | All four native jobs in run `33056742540` passed editor, formatter, and aggregate-memory ceilings |

## Linter

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Correctness-focused defaults have acceptable signal | Proven | Run `33040254261` retains 334 fully classified default/recommended findings and zero false positives |
| Rules execute only their declared analysis tier | Proven | Registry validation and syntax/types/CFG/SSA scheduler contracts in CI `33056719958` |
| Syntax-only linting avoids typed loading | Proven | File-oriented syntax driver and CLI routing contracts |
| Typed, CFG, and SSA representations are shared and bounded | Proven | Shared package session, graph, fact, cache, cancellation, and memory-budget evidence |
| Diagnostics have precise locations and deterministic order | Proven | Source-version/range validation, frozen reporter contracts, and deterministic parallel collection |
| Suppressions are narrow, auditable, and formatter-stable | Proven | Exact-rule ownership, reasons, expiry, malformed/unused diagnostics, and ownership-preservation tests |
| Every admitted rule has behavioral tests and canonical docs | Proven | Generated 129-rule catalog freshness and admission records |
| `go/analysis` interoperability matches its documented boundary | Proven | Syntax/package adapters, fact suites, and ADRs 0005 and 0006 |
| Machine schemas are versioned | Proven | Frozen v1 machine contracts and schema references reproduced by run `33056742540` |
| Rule IDs and presets follow compatibility policy | Proven | Frozen rule/profile contracts and compatibility policy |

## Fixes

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Safe, suggestion, and unsafe fixes are distinct | Proven | Metadata validation and independent CLI authorization contracts |
| Safe fixes have semantics-preserving evidence | Proven | Rule admission records and three safe-fix contracts |
| Stale and overlapping edits cannot apply silently | Proven | Exact source digests, UTF-8 range validation, conflict rejection, and fix fuzzing |
| Results reparse, format, and validate before write | Proven | Coordinator, single-file transaction, and native write/fix evidence |
| Failed validation preserves original source | Proven | Rollback, stale-source, formatter-refusal, and filesystem-failure contracts |
| Fix output is deterministic and idempotent | Proven | Owned integration and fuzz evidence passes; run `33040254261` completed safe-fix rehearsal on every supported snapshot without mutation or instability |
| Multi-file failure transaction is documented if present | Proven as absent | Glippy exposes only transactional single-file fixes |

## Product And Operations

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| One binary exposes the stable command surface | Proven | Frozen help, completion, rule, profile, and native candidate execution contracts |
| Configuration discovery, precedence, and migration are stable | Proven | Typed schema, strict discovery, historical upgrade fixtures, and compatibility policy |
| Exit codes distinguish findings from failures | Proven | Frozen exit-category and reporter contracts |
| Modules, workspaces, tests, build tags, GOOS, and GOARCH are covered | Proven | Loader matrix, corpus manifest, and supported-target workflow |
| Cancellation and bounded concurrency are proven | Proven | Scheduler, loader, analysis, write, CLI, and resource-budget contracts |
| Cache keys are complete and corruption is recoverable | Proven | ADR 0008, cache specification, invalidation/corruption suites, and cache fuzzing |
| Editor and CI integrations use stable supported interfaces | Proven | Stdin/stdout, existing LSP, GitHub Actions, Woodpecker, shell CI, and pre-commit documentation |
| Release archives are reproducible and checksummed | Proven | Run `33056742540` reproduced four archives, manifest, checksum file, and frozen contracts across every runner; publication run `33060261742` rebuilt and published the tagged source |
| Publication provenance is verified | Proven | Every `v1.0.0` release asset passed checksum and GitHub attestation verification against tag target `a2e526c` |
| Naming and module-path audit is complete | Proven | ADR 0016 binds the Glippy name, module, binary, configuration, suppressions, caches, and artifacts |
| Supported-version and vulnerability policies are published | Proven | Supported-Go, support, compatibility, security, and vulnerability-reporting documents |
| Final corpus, fuzz, race, integration, and performance gates pass | Proven | Corpus `33040254261` and fuzz `33040254402` validly carry to source-bearing candidate `18004f2`, whose exact CI `33056719958` and native budgets `33056742540` pass |
| Real-repository dogfood is documented | Proven | Glippy self-adoption and maintainer-approved `pkg/prompts` integration |
| Publication is explicitly authorized | Proven | The maintainer granted advance release approval, and every pre-publication gate now passes |

## Architecture

| Acceptance criterion | Result | Current evidence |
| --- | --- | --- |
| Formatter and linter remain separate engines over one frontend | Proven | Internal package boundaries and ADR 0005 |
| Layout policy is not duplicated as lint rules | Proven | Formatter dialect and non-layout lint catalog |
| Semantic fixes do not hide inside formatting | Proven | Formatter equivalence contract and explicit fix coordinator |
| The standard Go frontend is reused | Proven | Parser, AST, token, scanner, packages, types, analysis, CFG, and SSA integration |
| Source fidelity does not rely on AST data alone | Proven | Immutable source bytes, token/trivia ledger, directives, and reconstruction fuzzing |
| Document rendering has bounded complexity | Proven | Iterative renderer, fit budgets, adversarial tests, fuzzing, and native budgets |
| Expensive analysis is demand-driven and shared | Proven | Requirement tiers, shared package graphs, facts, caches, and cold/warm evidence |
| No premature plugin or public package surface exists | Proven | Internal packages and controlled `go/analysis` adapter only |
| Current Oxfmt and Oxlint behavior has been reviewed | Proven | v0.9 product-reference refresh and recorded Go-specific differences |
| Rule expansion follows foundation admission gates | Proven | Rule metadata, evidence records, nursery policy, and post-v1 rule roadmap |

## Publication Boundary

The pre-publication audit advanced the final source candidate to 95%. Annotated
tag `v1.0.0` subsequently resolved to the accepted documentation closure at
`a2e526c`, whose parent is the exact source-bearing candidate `18004f2`.
Publication run `33060261742` rebuilt that tag, published all six checksummed
files, created GitHub OIDC provenance attestations for every file, and created a
non-draft, non-prerelease GitHub Release. The separate release evidence verifies
the tag target, assets, manifest revision and toolchain, checksum closure,
native version metadata, attestations, release body, and `go install` channel.
The publication boundary is satisfied and stable-v1 progress is 100%.
