# Initial Formatting Corpus Manifest

The Phase 0 corpus contains project-owned source written for Gox. It is not
copied from external repositories. All files are valid Go 1.26 source. Each
input now has a reviewed Phase 1 `.golden` output at width 60.

| Path | Provenance | Required coverage | Gofmt relation at width 60 |
| --- | --- | --- | --- |
| `hostile/blocks.go` | Project-authored, 2026-08-09 | Compressed block, `if` initializer, explicit ordinary-statement semicolon | Intentional Gox layout: width-aware `if` header break |
| `hostile/statements.go` | Project-authored, 2026-08-09 | Multiple simple statements separated by explicit semicolons | Fixed point |
| `hostile/expressions.go` | Project-authored, 2026-08-09 | Boolean chain and long variadic call | Fixed point |
| `hostile/comments.go` | Project-authored, 2026-08-09 | Package documentation, tool directive, comments between operands and elements | Fixed point |
| `hostile/generics.go` | Project-authored, 2026-08-09 | Type sets, type parameters, range loop, and compressed statements | Fixed point |
| `hostile/directives.go` | Project-authored, 2026-08-10 | Build constraints, generated marker, cgo preamble, embed, generate, compiler directives including linkname, line directive, suppression anchor | Fixed point |
| `hostile/compatibility.go` | Project-authored, 2026-08-10 | Preserved import order, numeric literal spelling, redundant parentheses, and unaligned declarations and fields | Intentional Gox source-fidelity and layout policy; pinned output in `compatibility.gofmt.golden` |
| `hostile/empty-statements.go` | Project-authored, 2026-08-10 | Standalone and labeled explicit empty statements plus an implicit closing-label empty | Intentional source-fidelity policy; pinned output in `empty-statements.gofmt.golden` |

The corpus test reparses each golden output, proves byte idempotency, and checks
the recorded gofmt relation under the executing Go toolchain. Any divergence
must be named in both the test and this manifest and paired with an exact
`.gofmt.golden` output; an unclassified, changed, or stale difference fails the
suite.
Focused fixtures separately cover flat, boundary-width, broken, nested,
commented, directive, and invalid variants. External corpus additions require a
recorded source revision, license review, acquisition procedure, and expected
classification; external source must not be copied into this repository merely
for convenience.

The directive corpus imports `C` and refers to one inert marker symbol. Its Go
declarations are type-checked with a matching `C` package shell because
ordinary `go/types` importers do not invoke cgo; parser, formatter,
equivalence, and gofmt checks still consume the exact cgo source.

The compatibility fixture records the classes that prevent a product-wide
gofmt fixed-point guarantee. Gofmt sorts the retained import group, normalizes
hexadecimal prefixes, removes redundant parentheses, and inserts tabular
alignment that Gox deliberately does not own in the prototype dialect.
Gofmt also removes a standalone explicit empty statement and materializes an
implicit closing-label empty statement as `;`; Gox retains whether those
statements were explicit in the source.
