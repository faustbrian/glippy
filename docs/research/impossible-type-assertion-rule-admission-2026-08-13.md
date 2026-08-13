# `impossible-type-assertion` Rule Admission, 2026-08-13

## Decision

Admit `impossible-type-assertion` to `correctness` at warning severity by
adapting x/tools v0.48.0 `ifaceassert.Analyzer`. It uses the types tier,
excludes generated and ill-typed packages, and offers no fix.

## Defect And Existing Tools

An interface-to-interface assertion cannot succeed when the two method sets
contain the same method name with incompatible signatures. The single-result
form will panic and the comma-ok form will always fail. The compiler rejects
some concrete impossibilities but accepts this interface-to-interface case.
Go 1.26.5 enables `ifaceassert` in default vet.

The upstream implementation uses `types.MissingMethod` and deliberately avoids
conclusions involving free type parameters under Go issue 50658. Its fixtures
cover conflicting methods, compatible assertions, type switches, embedded
interfaces, aliases, and generics. No exact Staticcheck v0.8.0-rc.1 rule was
found. Rust has no direct interface assertion construct, so Clippy has no
equivalent semantic rule.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `go/analysis/passes/ifaceassert` source, tests, and fixtures;
- Go 1.26.5 `go tool vet help`;
- Staticcheck v0.8.0-rc.1; and
- the current Clippy type and cast lint catalog.

## Precision, Policy, And Fixes

Only conflicts proven from interface method sets report. Concrete assertions,
compatible interfaces, and type-parameter cases outside the upstream proof are
not duplicated. Glippy maps the upstream position to the exact physical byte
range and applies deterministic suppressions and baselines. Generated and
ill-typed packages are excluded.

No fix is registered because the intended target interface or method signature
cannot be inferred.

## Evidence And Cost

Focused CLI fixtures report the incompatible assertion and accept the same
source interface. Policy tests cover exact suppressions, generated and
type-error exclusion, baselines, and absence of fixes.

| Case | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| typed baseline | 162,996,700 | 5,344,929 | 43,352 |
| `impossible-type-assertion` | 145,374,825 | 5,358,278 | 43,357 |

These are five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max;
load variance dominates. Non-mutating correctness lint completed with no
findings over Glippy and `go-libraries/pkg/prompts` at
`633a5508c570d08b8976689a206f9df27e73ff90`, without changing the external
worktree.
