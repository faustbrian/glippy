# Clippy Comparability Exit Audit, 2026-08-15

## Decision

Glippy has reached a reasonable Clippy-comparable level for the Go ecosystem.
This decision means the product now provides the cohesive defect detection,
policy, adoption, explanation, fixing, reporting, and editor surfaces that make
Clippy valuable in routine development. It does not claim rule-for-rule parity
with Clippy or Staticcheck, and it does not turn future catalog growth into a
completion requirement.

The audit began from exact implementation revision
`3ac8b5f5b8ffdef5c1e7124961eda2a716150bbe`. That snapshot contains 100
documented rules backed by the same canonical metadata used by the registry,
scheduler, configuration decoder, `glippy rules`, and `glippy explain`.

## Milestone Audit

| Milestone requirement | Current evidence | Result |
| --- | --- | --- |
| High-value Go vet surface | `invalid-build-constraint`, `invalid-directive`, `invalid-test-signature`, `unbuffered-signal-channel`, and `standard-library-version` are registered, documented, selectable, and tested through the shared analyzer adapter | Complete |
| Staticcheck-inspired semantic pack | Ten native rules are recorded in `v0.3-semantic-correctness-pack-2026-08-14.md`; the requested possible-nil-dereference and WaitGroup behaviors already use canonical `nilness` and `waitgroup-misuse` identities | Complete without duplicate diagnostics |
| Changed-code adoption | `lint/check --new-from` resolves a deterministic merge base, handles modifications, renames, deletions, and untracked files, runs complete analysis, separates pre-existing findings, and constrains fixes to owned transformations | Complete |
| Configuration introspection | `glippy init`, `config check`, and `config show` expose origin, presets, rule reasons and options, maximum tier, source language, generated/test/vendor policy, build selection, baseline state, suppression policy, cache limits, and migration target | Complete |
| Selective policy and discovery | `--only`, `--except`, suggestion fixing, `rules` preset/fix/tier filters, and JSON `explain` share canonical rule selection and reject malformed or unknown identities | Complete |
| Machine and CI reporting | Text, short, schema-version-1 JSON, GitHub workflow-command, and SARIF 2.1.0 reporters consume the same deterministic diagnostics across full and changed-code lint/check paths | Complete |
| Editor diagnostics and actions | The bounded stdio LSP publishes syntax and overlay-backed package diagnostics, formatting, documentation links, suppression and baseline problems, exact-version fixes, safe fix-all, cancellation, and stale-document refusal without writing source | Complete |
| Pedantic and performance growth | Nine of the fourteen suggested candidates are admitted alongside the broader 16-rule pedantic, four-rule performance, and four-rule complexity groups | Sufficient; remaining candidates stay evidence-gated |

## Catalog Shape

The generated catalog contains 100 rules with this preset shape:

| Preset membership | Rules |
| --- | ---: |
| correctness only | 48 |
| correctness and migration | 1 |
| suspicious | 25 |
| pedantic | 16 |
| performance | 4 |
| complexity | 4 |
| style | 1 |
| restriction | 1 |

The catalog spans syntax, types, CFG, and SSA without making every invocation
pay for the deepest tier. Default correctness remains focused on direct defects
and ineffective behavior. Intent-sensitive diagnostics remain opt-in through
`suspicious`, `pedantic`, `performance`, and `complexity`; organizational
restrictions remain exact-ID-only.

Rule count is supporting evidence, not the exit criterion. The stronger
evidence is that every rule participates in the same severity, suppression,
baseline, generated-file, source-version, deterministic reporting, and fix
safety contracts.

## Clippy-Like Product Behavior

Glippy now provides the Go-native equivalents of the Clippy behaviors that
matter for continuous use:

- coherent `correctness`, `suspicious`, `performance`, `complexity`, `style`,
  `pedantic`, `restriction`, and `migration` groups;
- ordered `allow`, `warn`, `deny`, and irreversible `forbid` levels;
- warnings-as-errors, exact rule overrides, and ordered path-scoped policy;
- `--only`, `--except`, changed-code filtering, and deterministic baselines for
  incremental adoption;
- canonical rule discovery and explanation, including requirements, fixes,
  limitations, examples, and machine-readable metadata;
- safe, suggestion, and unsafe fix classes with stale-source, overlap, parse,
  format, reanalysis, and atomic-write validation;
- compact and source-framed terminal diagnostics plus JSON, GitHub, and SARIF
  integration; and
- editor formatting, diagnostics, explain links, individual code actions, and
  safe fix-all through one persistent service.

These surfaces share the formatter, source model, package loader, analyzer
tiers, registry, fix coordinator, cache, and reporter contracts. The LSP and
CLI do not maintain separate lint implementations.

## Deliberate Go Differences

Glippy remains Go-native rather than copying Rust architecture:

- it uses the standard Go parser, type checker, `go/packages`, `go/analysis`,
  CFG, and SSA instead of compiler-private Rust representations;
- formatting remains a separate deterministic engine rather than a lint group;
- package analysis tiers are demand-driven because ordinary syntax linting
  must not require package loading;
- Go vet analyzers are adapted where their contracts fit instead of being
  reimplemented under duplicate IDs; and
- dependency, macro, Cargo, edition, and Rust type-system lints have no direct
  Go product analogue.

The result is comparable usefulness and ergonomics, not identical catalog
semantics.

## Explicit Non-Blockers And Deferrals

The five unadmitted optional candidates do not block this decision:

- `redundant-slice-copy` needs alias and overlap evidence before claiming an
  allocation or copy is removable;
- `manual-string-prefix-trim` is a low-severity rewrite whose version and
  expression-shape contract has not shown enough adoption value;
- `unnecessary-sort-slice` must preserve comparator, stability, named-slice,
  and side-effect behavior before recommending a generic replacement;
- `error-formatting-without-wrapping` cannot assume callers intend to expose an
  underlying error through `errors.Is` or `errors.As`; and
- `defer-in-hot-loop` lacks a credible definition of hotness, while the
  existing `defer-in-loop` and `defer-in-infinite-loop` rules already cover the
  demonstrated cleanup hazards.

Cross-package effect facts remain a future precision improvement for lifecycle
and no-return rules. They reduce conservative false negatives; their absence
does not invalidate the current intraprocedural contracts or the shared
package-local no-return analysis.

Glippy also does not claim complete Staticcheck, Go vet, or Clippy catalog
parity, unrestricted plugins, dependency linting, or automatic application of
behavior-changing fixes. Those would weaken the evidence-first and safe-default
boundaries used to reach this milestone.

## Continuation Policy

Future work is ordinary product evolution rather than unfinished comparability
foundation. New rules still require real defects, compiler and default-toolchain
boundary analysis, close negatives, proportional cost evidence, dogfood, and a
credible fix classification. Catalog growth may pause when that evidence is
weak.

Reopen this exit decision only if dogfood shows that a missing workflow makes
Glippy impractical for daily use, the shared policy or editor surfaces diverge,
default diagnostics lose acceptable signal, or a major Go release invalidates
the current analyzer and source-version contracts.
