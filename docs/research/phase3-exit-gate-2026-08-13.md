# Phase 3 Exit Gate, 2026-08-13

## Decision

Phase 3 is complete. Gox has a stable syntax-lint path, deterministic
diagnostics and suppressions, explicit fix classes, and a transactional
single-file fix coordinator. Its two default correctness rules have current
clean dogfood on Gox and the maintainer-approved `pkg/prompts` migration.

This decision advances overall progress from 55% to 75%. It does not establish
the Phase 4 typed, CFG, SSA, and cache exit gate merely because later-tier
infrastructure and opt-in rules already exist.

## Required Capability Audit

| Phase 3 requirement | Current authoritative implementation or evidence | Result |
| --- | --- | --- |
| Rule metadata and registry | `internal/rules` validates immutable metadata, IDs, presets, tiers, interests, generated-file policy, fixes, options, examples, limitations, and deprecation | Proven |
| Shared syntax scheduling | One direct union AST pass dispatches declared node interests; the current 1/3/5/10/25-rule benchmark retains bounded allocation and materially outperforms naive walks at 5, 10, and 25 rules | Proven |
| Deterministic diagnostics | Analysis and reporting validate exact source identity and byte ranges, then canonically sort diagnostics independently of registration and worker order | Proven |
| Severity and presets | The registry resolves the correctness default plus explicit rule severities and typed options before execution | Proven |
| Canonical documentation and explain | `gox explain` and `docs/lint-rules.md` derive from the same metadata; the full suite includes byte-for-byte catalog freshness | Proven |
| Suppressions | Exact-rule line, next-line, range, and file ownership; reason and expiry policies; malformed, unknown, unused, and expired outcomes; formatter-stable ownership | Proven |
| Fix classes | Safe, suggestion, and unsafe metadata and independently composable CLI authorization | Proven |
| Stale and conflicting edits | Exact source digests, validated UTF-8 byte ranges, deterministic ordering, overlap and same-offset insertion rejection, and unapplied-fix provenance | Proven |
| Transactional single-file fixing | Complete candidate reparse, formatter normalization, post-format reanalysis and validation, stale snapshot refusal, atomic replacement, and rollback on failure | Proven |
| Text and JSON reporting | Human output and schema-version-1 JSON share validated canonical records and distinct finding, conflict, source, filesystem, and tool outcomes | Proven |
| GitHub-oriented output | No Phase 2 or Phase 3 adoption fixture requires a separate reporter, so the goal makes this optional surface inapplicable for this gate | Not required |
| `go/analysis` interoperability | Audited syntax adapters retain Gox scheduling, source identity, severity, suppression, reporting, and fix-safety ownership; richer typed adapters are additive later-tier work | Proven |
| Initial syntax correctness rules | `duplicate-condition` and `ineffective-break` are default correctness rules with individual admission records | Proven |
| Behavioral tests | Rule, analysis, suppression, fix, reporter, and CLI suites cover positive and nearby negative behavior, exact ranges, policy, fixes, conflicts, formatting, and non-mutation | Proven |
| Default-preset signal | Historical immutable dogfood covers 7,732 files across six repositories; the current gate also passes without findings on Gox and the approved `pkg/prompts` migration | Proven |

## Rule Admission Audit

The two default syntax rules each have:

- a stable ID and canonical documentation;
- a defect statement and evidence beyond the default compiler and `go vet`;
- positive and negative behavioral fixtures with exact ranges;
- syntax-tier cost and false-positive boundaries;
- generated-file and suppression behavior;
- explicit fix classification; and
- proportional benchmark and multi-repository dogfood results.

`duplicate-condition` has no fix because repairing a copied branch requires
intent. `ineffective-break` exposes removal only as a suggestion because a
return or labeled loop exit may be the intended repair. Neither rule inflates
the default catalog to satisfy a rule-count target.

## Fresh Exit Evidence

All commands ran from clean Gox `main` with Go 1.26.5 and task-owned disposable
Go build and module caches:

- `go test ./... -count=1`: passed;
- `go vet ./...`: passed;
- `go test -race` for rules, analysis, fixes, reporters, and CLI: passed;
- `FuzzCoordinate`, 10 seconds: 567,415 executions, passed;
- `FuzzParse`, 10 seconds: 444,646 executions, passed;
- syntax traversal benchmark, three 100-millisecond samples: passed; and
- default `gox lint ./...` over Gox and `pkg/prompts`: passed with no
  diagnostics, suppression problems, source failures, or tool failures.

The benchmark remains a strategy comparison, not a release latency budget.
At 25 rules the direct shared pass measured 4.027-4.272 microseconds and 1,896
allocated bytes per operation; naive walks measured 43.449-44.356 microseconds.

## Remaining Boundary

Phase 4 is active. Its exit still requires a separate current audit of typed
module and workspace behavior, shared type/CFG/SSA representations, cache
invalidation, tier-specific signal, and bounded cold and warm costs. Final
release-candidate corpus, fuzz, race, performance, publication, naming, and
maintainer-review gates also remain open.
