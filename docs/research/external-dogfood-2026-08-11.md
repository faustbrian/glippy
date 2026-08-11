# External Formatter Dogfood, 2026-08-11

## Scope

The first external formatter check used an immutable archive of
`/Users/brian/Developer/go-libraries` commit
`a6f1c1f66a1b754e7384da0f6e97e0b3587c5f71`. The live repository was rejected
as evidence after a concurrently removed `.gocache-*` directory interrupted
discovery. The archived snapshot prevents mutable workspace state from changing
the selected corpus during a run.

This audit is non-mutating adoption evidence. It does not transfer Gox's local
test results to the external repository or claim that the resulting migration
has been accepted by that repository's owners.

## Finding

The initial immutable check selected 5,051 files and stopped with an internal
formatter error in two files:

- `pkg/bulkhead/execution_test.go` places a `//nolint:staticcheck` directive
  after a logical OR operator; and
- `pkg/prompts/theme.go` places `//nolint:gosec` directives after arithmetic
  addition operators.

Both are valid Go. The binary-expression lowerer treated all comments after an
operator as inline block comments, so a line comment had no grammar-safe owner
and aborted the invocation. The first divergence was shared binary-boundary
lowering rather than either external caller.

## Corrected Result

Binary chains now keep the operator and trailing line comment on the preceding
line, force the following operand to the continuation line, and use the normal
canonical broken form for the rest of the same-precedence chain. The focused
logical and arithmetic regression failed with the original unsupported-boundary
error before the fix.

The corrected immutable check completes all 5,051 selected files. It reports
4,816 formatting differences, no source, validation, or internal errors, and
does not mutate the snapshot. The two original files now produce valid,
idempotent formatted output while preserving both lint directives.

## Remaining Adoption Boundary

A source scan identified 40 generated files, and a full-tree write rehearsal
correctly refused the selection before any replacement. Therefore this run does
not prove write-mode adoption, compilation of a migrated tree, or human
acceptance of the 4,816-file diff. A later dogfood rehearsal must select writable
non-generated files deliberately, inspect the complete migration, and run the
external repository's own scoped gates before it can count as completed
adoption.
