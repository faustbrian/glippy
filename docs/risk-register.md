# Risk Register

`Controlled` means the implemented mitigation has focused evidence. It does not
replace fresh release-candidate verification. `Open` means a required external
decision or evidence boundary remains unresolved.

| Risk | Status | Current control and evidence | Remaining release proof or trigger |
| --- | --- | --- | --- |
| AST is not a CST | Controlled | Immutable bytes, scanner ledger, gaps, directives, physical offsets, reconstruction tests, candidate corpus, and source-ledger fuzzing | Rerun the complete corpus and fuzz gates if the candidate changes |
| Width breaks change Go syntax | Controlled | Grammar-owned break sites, generated trailing commas, reparsing, normalized equivalence, semicolon/precedence/delimiter goldens, and candidate corpus | Rerun supported-version corpus and equivalence gates if formatting changes |
| Gofmt workflow conflict | Controlled | Recorded fixed-point divergence classes, migration guidance, and maintainer-approved `pkg/prompts` adoption branch `d6b0fba8`, which removes competing gofmt and goimports formatter authority | Integrate the dedicated adoption branch; repeat adoption evidence if its patch changes |
| Comment or directive migration | Controlled | Token-boundary ownership, identity accounting, dense fixtures, directive corpus, candidate corpus, and fuzzing | Rerun the release corpus and fuzz budget if source or formatting changes |
| Layout pathologies | Controlled | Iterative renderer, bounded fit work, eight-worker ceiling, adversarial allocation guards, and candidate-native 250 ms/90 s/2 GiB evidence | Rerun the four-runner workflow if rendering or release inputs change |
| Oversized source exhausts memory | Controlled | One 64 MiB exact-byte boundary before cloning, parsing, overlays, and write/fix snapshots; bounded streams read one proving byte | Recheck release-candidate peak memory and revisit only for a validated generated-source journey |
| Type-aware latency | Controlled | Demand tiers, one run-owned package load, persistent typed-cache measurements, cancellation tests, and the release workflow's typed side-workload | Rerun cold and warm typed workloads on the release candidate |
| Rule noise | Controlled | Two-rule correctness default, per-rule admission evidence, historical 7,732-file multi-repository noise audit, and current clean Gox plus approved `pkg/prompts` dogfood | Rerun representative default-preset dogfood on the release candidate and disposition every finding |
| Unsafe or conflicting fixes | Controlled | Source digests, safety classes, stale and overlap rejection, reparsing, formatting, reanalysis, rollback, and atomic single-file replacement tests | Rerun the fix integration and race gates on the release candidate |
| Configuration sprawl | Controlled | Strict typed TOML version 1, few formatter options, immutable rule options, and no implicit inheritance | Require a demonstrated adoption need before adding any option or inheritance model |
| Premature plugin API | Controlled | Internal native rules and constrained `go/analysis` adapters; no dynamic plugin surface | Revisit only for an external use case that the stable adapter boundary cannot serve |
| Name collision | Open | Final-candidate searches confirm exact binary, repository, package-manager, social-name, and domain collisions | Secure maintainer acceptance of the documented collision and trademark-risk boundary before the first public tag |
| Unlicensed distribution | Open | Direct release dependencies were inventoried and their MIT, BSD, and Go patent notices are now deterministic archive entries, but the repository has no project license | Select a project license, require it in every release archive, and rerun release-candidate evidence before publication |
| Physical/logical position confusion | Controlled | Physical byte offsets plus `//line`, CRLF, BOM, canonical-LF, and package-position mapping fixtures | Rerun package, reporter, and source-fidelity gates on the release candidate |
| Cgo synthesized source writes | Controlled | Editable source identity excludes generated compiled files; cgo package loading and fix refusal have integration coverage | Rerun cgo package and write/fix refusal gates on supported release environments |
| Cache returns stale semantics | Controlled | Complete canonical keys, exact source/config/build selection, verified payload digests, corruption-as-miss recomputation, bounded pruning, and pinned cache roots | Rerun corruption, invalidation, and platform cache suites on the release candidate |
| Test-package duplicate diagnostics | Controlled | Canonical physical ownership assigns production files to ordinary packages and test-only files to augmented variants | Rerun internal/external test-package fixtures with deterministic result comparison |

No controlled risk is considered permanently closed. New formatter behavior,
analysis inputs, platforms, storage drivers, extension surfaces, or release
claims must update the corresponding control and evidence boundary.
