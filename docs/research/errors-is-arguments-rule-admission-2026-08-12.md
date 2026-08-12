# `errors-is-arguments` Rule Admission, 2026-08-12

## Defect And Existing-Tool Boundary

The standard library `errors.Is` function expects the dynamic error being
inspected first and the target error second. Reversing those arguments can make
wrapped errors fail to match and can silently disable error-handling branches.

Go 1.26.5 `go vet ./...` exited successfully for a disposable module containing
`errors.Is(io.EOF, err)`. The default compiler and vet toolchain therefore do
not diagnose this defect.

Current Staticcheck SA1032 is the external rule authority. The audit used
Staticcheck commit
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e)
and source SHA-256
`e39b68451800469fc133e667593258492b4a71152d3e81c66338222c786dd769`.
Staticcheck commit
[`aa47bfb8bbc8d5cb40f25407b1f2a5cb1033fda7`](https://github.com/dominikh/go-tools/commit/aa47bfb8bbc8d5cb40f25407b1f2a5cb1033fda7)
records the important exclusion for calls whose two arguments are both
external package globals, because external tests legitimately compare one
sentinel with another.

Reviewed public repairs demonstrate real impact:

- Incus corrected a reversed network sentinel check in
  [`e7947087b11533493351a6e5dd4be068154f1296`](https://github.com/lxc/incus/commit/e7947087b11533493351a6e5dd4be068154f1296);
- Beats corrected a reversed `fs.ErrNotExist` check in
  [`cd89cc463d9c3d573f66dbfc2932e4343d577ee1`](https://github.com/elastic/beats/commit/cd89cc463d9c3d573f66dbfc2932e4343d577ee1),
  whose commit records that the ineffective check prevented a test from being
  skipped.

## Contract And Types Boundary

`errors-is-arguments` is a warning in the opt-in `suspicious` preset. It uses
the shared types tier and call-expression traversal, excludes generated files
and ill-typed packages, and recognizes only the standard library `errors.Is`
function through its typed package and function identity.

The first argument is the exact primary range. The rule reports only when that
argument directly references a package-level variable declared by another
package and the second argument does not do the same. It accepts:

- ordinary correct calls such as `errors.Is(err, io.EOF)`;
- calls with external globals in both positions;
- package-local globals and local aliases;
- fields, calls, and composite expressions that do not directly identify an
  external package variable; and
- local or imported lookalike functions and methods.

This deliberately narrow heuristic favors precision over recall. It does not
attempt value flow through local variables, fields, or helper calls. Swapping
arguments is not registered as a fix because the heuristic alone does not
prove that an automatic rewrite preserves the caller's intended behavior.

## Behavioral Evidence

The initial focused tests failed because `errors-is-arguments` was absent from
the production registry. After implementation, focused rule and public CLI
tests pass and prove:

- exact diagnostic ranges for aliased, parenthesized, and dot-imported standard
  library calls;
- correct-call, both-external-global, package-local, local-alias, field, call,
  composite-expression, and lookalike exclusions;
- suppression and severity behavior;
- generated-file and type-error exclusion;
- absence of safe, suggestion, and unsafe fixes;
- opt-in preset ordering without raising the default correctness tier; and
- non-mutating `gox lint` diagnostics plus canonical `gox explain` metadata.

## Cost And Dogfood Signal

The cost probe reuses one preloaded typed package containing 100 reversed calls.
Five one-iteration samples on Darwin arm64 with an Apple M4 Max and Go 1.26.5
observed:

| Run | Median | Bytes per operation | Allocations per operation |
| --- | ---: | ---: | ---: |
| No-op shared types callback | 76.584 us | 1,672 | 22 |
| Shared types plus `errors-is-arguments` | 145.541 us | 102,952 | 134 |

This measures shared traversal, rule execution, and diagnostic construction,
not package-loading latency. It is proportional cost evidence rather than a
stable performance budget.

Two explicit non-mutating runs enabled only this suspicious rule and completed
without package, source, suppression, or tool errors:

| Corpus | Revision or state | Selected files | Diagnostics |
| --- | --- | ---: | ---: |
| Gox | implementation worktree | 115 | 0 |
| go-libraries `pkg/wsdl/...` | `1be04c0e6f17f587dc6083b701467620b95d511d` | 53 | 0 |

The external run used a task-owned immutable Git archive. Zero findings show no
observed false-positive noise in these 168 files; they do not prove recall.
Focused positive fixtures and the reviewed public repairs provide positive
evidence.

## Admission Decision

Admit `errors-is-arguments` under the opt-in `suspicious` preset. This adds a
high-confidence native types rule without forcing package loading for the
default correctness preset. Revisit default membership only after broader
representative adoption establishes acceptable typed-path latency and noise.
Revisit the detection boundary if real defects justify value-flow analysis or
if the standard library `errors.Is` contract changes.
