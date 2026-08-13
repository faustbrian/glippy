# `copied-lock` Rule Admission, 2026-08-13

## Decision

Admit `copied-lock` to `correctness` at warning severity by adapting the
authoritative x/tools v0.48.0 `copylock.Analyzer`. It uses the types tier,
excludes generated files, follows the analyzer's `RunDespiteErrors` policy,
and offers no fix.

## Defect And Existing Tools

Copying a value containing `sync.Mutex`, `sync.RWMutex`, `sync.WaitGroup`, or a
recognized `noCopy` marker splits state whose contract depends on one stable
identity. The compiler accepts these copies. Go 1.26.5 lists `copylocks` in the
default vet catalog, so Glippy reuses that analyzer instead of duplicating its
lock-path, generic-version, and noCopy logic.

The upstream fixtures cover assignments, declarations, parameters, returns,
ranges, calls, composite literals, value receivers, generic types, and
regressions from Go issues 61678 and 67787. Staticcheck has concurrency rules
but no equivalent full copylocks contract in the inspected v0.8.0-rc.1
catalog. Rust's ownership and trait model makes Clippy's lock rules materially
different; no Clippy rule is an appropriate detection frontend for Go copies.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `go/analysis/passes/copylock` source, tests, and fixtures;
- Go 1.26.5 `go tool vet help`;
- Staticcheck v0.8.0-rc.1 concurrency checks; and
- the current Clippy lint catalog for lock and synchronization rules.

## Precision, Policy, And Fixes

The adapter preserves upstream diagnostic positions and maps them to exact
physical byte ranges. Nearby pointer passing is accepted. Glippy suppressions
and baselines operate after deterministic mapping. Generated files are
excluded. Ill-typed packages are eligible because the analyzer explicitly
opts into partial type information.

No fix is registered. Replacing a value with a pointer can change ownership,
aliasing, method sets, public APIs, allocation, and lifetime, so intent is
required.

## Evidence And Cost

Focused CLI fixtures report a copied return and accept pointer sharing. Policy
tests cover exact suppression ownership, generated exclusion, type-error
execution, deterministic baselines, and absence of fixes.

Five one-iteration samples on Go 1.26.5, Darwin arm64, Apple M4 Max measured
the complete one-package load:

| Case | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| typed baseline | 162,996,700 | 5,344,929 | 43,352 |
| `copied-lock` | 140,298,950 | 5,359,579 | 43,380 |

Load variance dominates latency; the observed allocation delta is about 15 KiB
and 28 allocations. Non-mutating correctness lint completed with no findings
over Glippy and `go-libraries/pkg/prompts` at revision
`633a5508c570d08b8976689a206f9df27e73ff90`; the external worktree head and
dirty status were unchanged.
