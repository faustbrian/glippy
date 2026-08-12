# Phase 0 Risk Register

| Risk | Current control | Proof required before closure |
| --- | --- | --- |
| AST is not a CST | Immutable bytes, scanner ledger, gaps, directives, physical offsets | Reconstruction, comment/directive accounting, corpus and fuzz evidence |
| Width breaks change Go syntax | Grammar-owned break sites, trailing commas, reparse | Golden semicolon/precedence/delimiter cases across supported versions |
| Gofmt workflow conflict | Recorded fixed-point classes, pinned divergences, and sole-formatter migration boundary | Supported-version reruns and release migration guidance |
| Comment or directive migration | Stable identity and token-boundary ownership | Dense comments, directives, cgo/build constraints, fuzz minimization |
| Layout pathologies | Iterative renderer, fixed fit budget, 100,000-depth and 20,000-breadth allocation guards | Release-scale peak-memory and stable-runner latency budgets |
| Oversized source exhausts memory | One 64 MiB exact-byte boundary before source cloning, parsing, package-overlay cloning, and write/fix snapshots; bounded streams read one proving byte | Release-scale corpus and peak-memory evidence; revisit for validated generated-source workflows |
| Type-aware latency | Demand tiers and one run-owned package load | Cold/warm module/workspace benchmarks and cancellation proof |
| Rule noise | Correctness-only default and admission gate | Multi-repository dogfood false-positive record |
| Unsafe or conflicting fixes | Source digests, explicit conflict rejection, full validation | Stale/overlap/rollback/atomic-write integration suite |
| Configuration sprawl | Typed TOML, few formatter options, no nested inheritance initially | Dogfood evidence before adding every new option or inheritance feature |
| Premature plugin API | Native rules and constrained `go/analysis` adapter only | Stable internal rule and scheduling contract plus external use case |
| Name collision | Working-name-only decision | Replacement ecosystem and trademark audit before public release |
| Physical/logical position confusion | Physical offsets for edits plus `//line`, CRLF, BOM, and canonical-LF formatter fixtures | Concurrent package-load position fixtures |
| Cgo synthesized source writes | Editable source identity excludes generated compiled files | Cgo package loading and fix refusal integration tests |
| Cache returns stale semantics | No persistent cache until complete key decision | Corruption, invalidation, toolchain/config/build-environment matrix |
| Test-package duplicate diagnostics | Canonical physical identity and future variant deduplication | Internal/external test package fixtures with deterministic results |

No risk is closed in Phase 0 merely because a design control exists. Closure
requires the listed behavioral evidence.
