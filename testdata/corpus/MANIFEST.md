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

The corpus test reparses each golden output, proves byte idempotency, and checks
the provisional gofmt relation under the executing Go toolchain. Any divergence
must be named in both the test and this manifest and paired with an exact
`.gofmt.golden` output; an unclassified, changed, or stale difference fails the
suite.
Focused fixtures separately cover flat, boundary-width, broken, nested,
commented, directive, and invalid variants. External corpus additions require a
recorded source revision, license review, acquisition procedure, and expected
classification; external source must not be copied into this repository merely
for convenience.
