# Phase 4 Exit Gate, 2026-08-13

## Decision

Phase 4 is complete. Gox now pays for types, control flow, SSA, dependency
facts, and persistent results only when enabled rules require them. The shared
package path is deterministic, source-bound, cancellation-aware, cache-safe,
and bounded across representative modules and workspaces.

This decision advances overall progress from 75% to 90%. It does not establish
the Phase 5 release gate, stable release-scale latency or memory budgets, final
publication readiness, or release-candidate evidence.

## Required Capability Audit

| Phase 4 requirement | Current authoritative implementation or evidence | Result |
| --- | --- | --- |
| Modules and workspaces | `LoadPackages` owns one canonical `go/packages` graph; fixtures cover one and multiple modules, workspace roots, local replacements, vendoring, internal-package enforcement, overlays, build tags, GOOS, GOARCH, cgo, and internal and external test variants | Proven |
| Shared type information | Native node and package rules plus adapted analyzers reuse the load-owned package, `go/types.Info`, sizes, file set, and immutable source index | Proven |
| Package and object facts | Deterministic analyzer dependency graphs support isolated package and object facts, stable object paths, canonical snapshots, independent-type-check restoration, and transactional rejection | Proven |
| Shared CFG | One `x/tools/go/cfg` graph is constructed per eligible physical function and shared across enabled CFG rules; syntax, generated, test-variant, type-error, and cancellation policies are covered | Proven |
| Shared SSA | One run-owned SSA program maps eligible declarations and function literals to physical source callbacks while excluding synthetic wrappers and ill-typed packages | Proven |
| Dependency scheduling | Imports and fact graphs execute dependency-first with canonical package, analyzer, file, diagnostic, and fact ordering; shared dependencies execute once | Proven |
| Partial type-error behavior | Package and source prerequisites remain separate from valid partial file results; each rule explicitly declares whether it may run despite type errors | Proven |
| Cancellation | Loading, native types, CFG, SSA, analyzer prerequisites, facts, cache work, CLI routing, and typed fixing preserve cancellation at their owned boundaries | Proven |
| Cache identity and invalidation | Versioned keys bind toolchain, language, configuration, rules and options, build selection, environment, sources, workspace and module state, overlays, exports, facts, and formatter mode; corruption, stale identity, dependency changes, ownership changes, and policy changes recompute instead of succeeding incorrectly | Proven |
| Bounded resources | One load admits at most 10,000 reachable packages, 20,000 unique sources, and 256 MiB of unique source bytes; the limits are enforced before facts, CFG, SSA, caches, or callbacks, and analysis itself is sequential over the bounded graph | Proven |
| Typed rule signal | `context-key`, `errors-is-arguments`, and `redundant-bool-comparison` have individual admission records, focused positive and negative behavior, exact ranges, toolchain boundaries, cost probes, and dogfood | Proven |
| CFG and SSA rule signal | `defer-in-infinite-loop` and `nilness` use CFG and SSA only where those representations materially improve precision; both have individual admission and dogfood records | Proven |
| Syntax-only independence | Syntax selections retain file-oriented execution and do not invoke package loading or persistent typed caches; CLI fixtures prove the routing boundary | Proven |
| Cold and warm costs | The published fact-cache and native types, CFG, and SSA benchmarks include package loading, source capture, identity construction, result publication or restoration, callbacks, allocations, and zero-callback warm validation | Proven |

## Fresh Exit Evidence

All Go commands ran on Darwin arm64 with Go 1.26.5 and task-owned disposable
build and module caches. The final code state is commit `23515ad`; subsequent
commit `21e0df3` changes only adoption documentation.

- `go test ./... -count=1`: passed;
- `go vet ./...`: passed;
- `go test -race` for analysis, cache, CLI, and rules: passed;
- complete `internal/analysis` tests and race tests: passed;
- focused cgo, local-replacement, and cross-workspace internal-package
  fixtures: passed; and
- non-mutating `suspicious`-preset lint over Gox and the approved
  `pkg/prompts` migration: passed with no findings or prerequisite failures.

The fresh one-operation cache probe measured:

| Maximum tier | Cold | Warm | Warm callbacks |
| --- | ---: | ---: | ---: |
| Fact-bearing analyzer | 820 ms | 455 ms | 0 |
| Native types | 107 ms | 104 ms | 0 |
| Native control flow | 124 ms | 113 ms | 0 |
| Native SSA | 115 ms | 113 ms | 0 |

The benchmark is a functional and directional cost gate, not a stable latency
budget. The existing published five-sample campaigns provide distributions;
the fresh sample confirms that every warm tier still bypasses all corresponding
callbacks after an independent package load.

The dogfood commands populated their disposable module caches explicitly and
then invoked Gox with its ordinary offline package-loading policy. This keeps
the clean result distinct from a claim that Gox may fetch dependencies during
ordinary linting.

## Remaining Boundary

Phase 5 is active. It retains the final naming and collision audit,
release-scale latency and memory budgets, editor and CI release integration,
publication and provenance readiness, release-candidate corpus, fuzz, race,
integration, and platform gates, broader human-reviewed dogfood adoption, and
the maintainer's final pre-tag review. No tag or release is authorized before
the complete goal reaches 100% and that review occurs.
