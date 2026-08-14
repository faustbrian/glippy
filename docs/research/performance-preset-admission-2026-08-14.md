# Performance Preset Admission, 2026-08-14

## Scope And Current Sources

This batch establishes Glippy's first populated `performance` preset. The
primary external reference is Staticcheck HEAD
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, verified from the upstream Git
repository on 2026-08-14. The corresponding current implementations inspected
were SA6000, SA6002, SA6003, and SA6006.

The rules use Glippy's shared Go 1.25 and 1.26 syntax and type information. They
do not copy Staticcheck's IR frontend, make speculative micro-optimization
claims, or enable an expensive analysis tier merely to mirror an upstream
implementation.

## Admitted Rules

| Glippy rule | Reference | Tier | Observable cost | Precision boundary |
| --- | --- | --- | --- | --- |
| `regexp-compile-in-loop` | SA6000 | types | `regexp.Match`, `MatchReader`, and `MatchString` compile a constant pattern for every repeated call | Exact standard functions, constant patterns, and syntactic loop execution only; indirect calls are excluded |
| `sync-pool-non-pointer` | SA6002 | types | Boxing a non-pointer pool value generally allocates; slice headers require boxing allocation | Exact `(*sync.Pool).Put`; interfaces and type parameters are excluded because their dynamic representation is unknown |
| `string-range-rune-conversion` | SA6003 | types | Converting a string to a rune slice allocates before iteration | A nonblank index excludes the finding because direct string range yields byte offsets rather than rune indexes |
| `inefficient-io-string-write` | SA6006 | types | Converting a byte slice to string allocates and copies before `io.WriteString` | Exact standard function and explicit byte-slice conversion only |

All four rules are warning-level, opt-in `performance` members. Generated files
and ill-typed packages are excluded. Suppressions, baselines, changed-code
filtering, machine reporters, rule discovery, LSP diagnostics, and source
version selection use the existing shared contracts.

## Fix Safety

No fix is admitted in this batch:

- regexp compilation requires choosing scope, error handling, and `Compile`
  versus `MustCompile`;
- changing a pool value to a pointer changes ownership and aliasing;
- removing the rune conversion needs a separate comment-ownership proof; and
- replacing `io.WriteString` with `Write` can change dispatch when the writer
  implements both `io.Writer` and `io.StringWriter`.

These are useful diagnostics without a sufficiently general
semantics-preserving automatic transformation.

## Behavioral And Cost Evidence

Focused red-green package fixtures cover the four positive contracts, constant
regexp calls in immediately invoked loop closures, dynamic-pattern and
outside-loop exclusions, pointer-like pool values, used rune indexes,
non-byte conversions, custom APIs, exact source ranges, absence of fixes,
generated and type-error policy, suppressions, and Go-version selection.

One-iteration cold package probes on Darwin arm64 with an Apple M4 Max measured:

| Rule | Cold package probe |
| --- | ---: |
| `inefficient-io-string-write` | 226.3 ms |
| `regexp-compile-in-loop` | 126.6 ms |
| `string-range-rune-conversion` | 107.0 ms |
| `sync-pool-non-pointer` | 103.2 ms |

These numbers include fresh package loading and are proportional admission
evidence, not stable latency budgets or isolated rule execution costs.

Non-mutating `--only` dogfood produced no diagnostics across 214 Glippy Go
files at base revision `05952a2865867c514b3902d518b038b1127c6f83` plus the
current batch, and 77 `go-libraries/pkg/prompts` Go files at
`c09c5268b1facb88317f933913b2e7cf4377948b`. Dependencies were explicitly
prefetched into a disposable module cache; Glippy's ordinary package loader
remained network-disabled.

## Revisit Triggers

Revisit when dogfood demonstrates an indirect-call regexp pattern, compiler
escape behavior materially changes, comment ownership supports the rune-range
suggestion, or an `io.Writer` replacement contract can prove dispatch and error
behavior. SA6001 remains deferred because its byte-slice map-key optimization
depends on a subtler compiler and placement contract.
