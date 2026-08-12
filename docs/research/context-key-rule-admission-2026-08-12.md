# `context-key` Rule Admission, 2026-08-12

## Authority And Existing-Tool Boundary

The rule detects unsafe keys passed to the standard library
`context.WithValue`. Built-in key types and aliases that resolve directly to
them can collide across packages. Anonymous empty structs have the same
collision problem. Nil and statically non-comparable keys panic at runtime.

These programs compile. A disposable Go 1.26.5 module containing string and
`[]byte` keys completed the default `go vet ./...` without a diagnostic, so the
rule adds coverage beyond the default toolchain.

Staticcheck SA1029 is the external rule authority. The audit used Staticcheck
HEAD commit
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Its rule source and fixture match the locally available v0.8.0-rc.1 copies:

- rule source SHA-256:
  `fbd8da00381fbc14c7237de62e63074897dd0d8cd3a9a0a741f14c1e693532ad`;
- fixture SHA-256:
  `23a7d2e7bc529a54d28ea0a8bda9e246525dd1181533863a5ec5e753f4d0b8aa`.

Reviewed public repairs demonstrate that this is an observed defect class:

- noctifab replaced string keys in
  [`6138f6000ae878dde4616064080e6631a028f71e`](https://github.com/diegojromerolopez/noctifab/commit/6138f6000ae878dde4616064080e6631a028f71e);
- workflow replaced built-in keys in
  [`6799e246d23a9a21feaa1164d61a1d00c4ba4fdc`](https://github.com/ehabterra/workflow/commit/6799e246d23a9a21feaa1164d61a1d00c4ba4fdc);
- inx replaced string keys in
  [`60148a2fe873086143ead446f3322178318fecb4`](https://github.com/naamfung/inx/commit/60148a2fe873086143ead446f3322178318fecb4).

## Contract And Execution Boundary

`context-key` is a warning in the opt-in `suspicious` preset. It requires the
types tier, visits shared call-expression traversal, excludes generated files
and ill-typed packages, and offers no fix. It recognizes only the standard
library function whose `go/types` object has package path `context` and name
`WithValue`; local and lookalike functions or methods do not report.

The second argument is the exact primary range. The rule reports:

- built-in key types and aliases resolving directly to built-ins;
- anonymous empty structs;
- nil keys; and
- statically non-comparable keys.

Package-defined comparable key types, named empty structs, aliases to named key
types, pointers, and non-empty comparable anonymous structs are accepted.
Interface-typed keys and type parameters with unrestricted, mixed, or complex
intersected type sets are accepted because the types tier does not prove their
dynamic value unsafe. A type parameter does report when one resolvable
structural restriction proves that every permitted type is non-comparable.

There is no generally safe automatic repair. Introducing a package-defined key
type changes declarations and may affect multiple call sites or package API.
No safe, suggestion, or unsafe fix is registered.

## Behavioral Evidence

The initial focused test failed because `context-key` was absent from the
production registry. After implementation, focused rule and CLI tests pass.
A later acceptance audit added an unconstrained type-parameter fixture; it
failed because the first implementation called a potentially comparable
dynamic value certainly non-comparable. The types-tier boundary now excludes
unresolved type parameters while retaining provably non-comparable constraints,
and that regression passes.

The fixtures prove:

- exact diagnostics for built-in, aliased built-in, anonymous empty struct,
  nil, and statically non-comparable keys;
- no diagnostics for defined comparable types, named empty structs, aliases to
  named types, pointers, non-empty anonymous structs, interfaces, type
  parameters with unresolved, mixed, or empty type sets, or lookalike
  functions;
- diagnostics for a type parameter whose one resolved structural restriction
  contains only statically non-comparable terms;
- standard-library function recognition through imported object identity and
  parenthesized function expressions;
- suppression and severity behavior;
- generated-file and type-error exclusion;
- absence of fixes; and
- non-mutating `gox lint` output plus canonical `gox explain` metadata.

## Cost And Dogfood Signal

The cost probe reuses one preloaded typed package containing 100
`context.WithValue` calls. Five 500-millisecond, single-CPU samples on Darwin
arm64 with Go 1.26.5 observed:

| Run | Median | Bytes per operation | Allocations per operation |
| --- | ---: | ---: | ---: |
| No-op shared types callback | 15.384 us | 1,480 | 20 |
| Shared types plus `context-key` | 85.161 us | 151,569 | 932 |

This measures rule and shared-traversal execution, not package-loading latency.
It is proportional cost evidence rather than a stable performance budget.

Two explicit non-mutating `suspicious`-preset runs completed without package,
source, suppression, or tool errors:

| Corpus | Revision or state | Selected files | Diagnostics |
| --- | --- | ---: | ---: |
| Gox | implementation worktree | 109 | 0 |
| go-libraries root selection | `0223e6490dd696a6242c490d458f2ee9c371faa8` | 16 | 0 |

The go-libraries run exercised only its root workspace or module selection; it
did not select nested modules and is not broad repository dogfood. Zero
findings establish no observed false-positive noise in these 125 files. They do
not establish recall; focused positive fixtures and the reviewed public fixes
provide that evidence.

## Admission Decision

Admit `context-key` as the first native types-tier built-in under the opt-in
`suspicious` preset. This avoids changing the default correctness analysis tier
or cost while the rule accumulates dogfood signal. Revisit default membership
only after broader representative adoption confirms acceptable noise and the
typed path has a stable latency budget. Revisit the implementation if the
standard library function contract or Go's alias, comparability, interface, or
type-parameter semantics change.
