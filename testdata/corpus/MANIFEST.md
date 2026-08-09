# Initial Formatting Corpus Manifest

The Phase 0 corpus contains project-owned source written for Gox. It is not
copied from external repositories. All files are valid Go 1.26 source and are
inputs rather than expected formatter output.

| Path | Provenance | Required coverage |
| --- | --- | --- |
| `hostile/blocks.go` | Project-authored, 2026-08-09 | Compressed block, `if` initializer, explicit ordinary-statement semicolon |
| `hostile/statements.go` | Project-authored, 2026-08-09 | Multiple simple statements separated by explicit semicolons |
| `hostile/expressions.go` | Project-authored, 2026-08-09 | Boolean chain and long variadic call |
| `hostile/comments.go` | Project-authored, 2026-08-09 | Package documentation, tool directive, comments between operands and elements |
| `hostile/generics.go` | Project-authored, 2026-08-09 | Type sets, type parameters, range loop, and compressed statements |

Phase 1 golden fixtures will pair each input with reviewed expected output at
flat, boundary-width, broken, nested, commented, directive, and invalid
variants. External corpus additions require a recorded source revision,
license review, acquisition procedure, and expected classification; external
source must not be copied into this repository merely for convenience.
