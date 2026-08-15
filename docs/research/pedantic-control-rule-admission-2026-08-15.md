# Pedantic Control Rule Admission, 2026-08-15

## Decision

Admit three warning-level rules to the opt-in `pedantic` preset:

| Rule | Tier | Contract |
| --- | --- | --- |
| `empty-branch` | syntax | Report a direct empty `if` or `else` block unless the block contains a comment |
| `manual-min-max` | types | Report a one-statement integer bound update over two exact variables |
| `redundant-type-declaration` | types | Report a one-variable declaration whose initializer infers the identical type |

None enters the default correctness preset. All three exclude generated files
and packages with type errors do not run the two typed rules.

## Clippy And Go Toolchain Boundary

Current Clippy source was reviewed at
[`e52501913b75235e3d41422566a2d05d6f00b699`](https://github.com/rust-lang/rust-clippy/tree/e52501913b75235e3d41422566a2d05d6f00b699).
Its `needless_ifs` lint establishes the empty-branch analogue while preserving
condition side effects, and `redundant_type_annotations` establishes the
explicit-type policy with conservative inference bounds. Go's `min` and `max`
built-ins make a direct Go-specific replacement available for integer bound
updates; this rule is not presented as a port of Clippy's differently scoped
`min_max` lint.

The Go compiler and the Go 1.26.6 default vet catalog accept all three shapes.
They are readability and maintainability findings rather than proof of a
runtime defect, which is why the complete batch remains pedantic and opt-in.

## Precision And Fixes

`empty-branch` treats explicit empty statements as empty and treats any comment
inside the block as deliberate documentation. It does not report loops,
switches, selects, or an `else if` wrapper as an empty direct alternative. No
fix is offered because removing the branch can discard condition evaluation or
change the surrounding alternative.

`manual-min-max` requires `<` or `>`, two distinct plain identifiers with the
same integer type, and one direct assignment between those exact objects. It
excludes floats because NaN behavior differs, and excludes initializers,
alternatives, fields, indexed values, compound assignments, and additional
statements. No fix is offered because comment placement and the preferred
assignment spelling remain review choices.

`redundant-type-declaration` requires one `var`, one initializer, and an
identical inferred type. Constant declarations, tuples, untyped nil, and
default-constant type changes remain excluded. Its `remove-redundant-type` fix
is safe only when the removed source gap contains no comment; the product
coordinator still reformats, reparses, reanalyzes, and typed-validates the
complete result.

## Evidence And Cost

Focused fixtures cover direct and alternative branches, comments, explicit
empty statements, integer directions and named types, float and structural
exclusions, typed and untyped constants, named conversions, nil, comment-owned
fix refusal, exact ranges, metadata, source versions, generated files, type
errors, suppressions, and safe-fix idempotency. The explicit-empty-statement
regression failed before the block classifier was expanded and passes after the
change.

One-iteration proportional probes on Go 1.26.6, Darwin arm64, Apple M4 Max
measured:

| Rule | Time | Bytes | Allocations |
| --- | ---: | ---: | ---: |
| `empty-branch` syntax | 0.100 ms | 24,056 | 80 |
| `manual-min-max` package | 65.644 ms | 249,328 | 1,705 |
| `redundant-type-declaration` package | 36.721 ms | 217,512 | 1,317 |

Exact-rule non-mutating dogfood found five genuine `manual-min-max` candidates
in Glippy at `358d080` plus this batch and one in
`go-libraries/pkg/prompts` at `e38bab8`. The other two rules produced no
findings. The six findings were inspected as ordinary integer bound updates;
neither repository was modified.

## Revisit Triggers

Revisit empty switch or select bodies only with a separate control-flow
contract. Revisit type-parameter min/max only when the built-in constraint and
inference behavior can be expressed without false positives. Broaden redundant
type inference only when `go/types` can prove the omitted declaration's exact
type rather than reflecting its existing contextual type.
