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

The maintainer selected `go-libraries/pkg/prompts` as the external adoption
target on 2026-08-12. Selection does not approve the earlier disposable diff:
the formatter migration must be reproduced from a current immutable revision,
presented as a dedicated reviewable diff, and accepted by the maintainer before
it counts as adoption.

## Bounded Write Rehearsal

The `pkg/prompts` module was selected for a bounded write rehearsal because it
contained one of the original binary-comment failures and has its own module
and verification boundary. Two immutable copies of the same source commit were
created; Gox modified only the disposable rehearsal copy.

Write mode selected 77 files, changed 69, and completed without a generated,
validation, replacement, or reporting failure. A second Gox check selected the
same 77 files with zero differences. Against that formatted snapshot:

- `go test ./... -count=1` passed for the module and terminal package;
- `go test -race ./... -count=1` passed for both packages;
- `go vet ./...` passed; and
- `go mod tidy -diff` reported no module metadata difference.

The complete migration has 7,611 inserted and 4,077 deleted lines across 69
files. Automated review found no stranded control keyword, receiver, or
selector pattern, and remaining lines over width are unbreakable string
literals. However, 63 files are not gofmt fixed points. The module's current
`format-check` gate would reject this migration, exactly matching Gox's recorded
gofmt incompatibility boundary.

This rehearsal proves writable non-generated module behavior, idempotency, and
the named local gates on a disposable snapshot. It still does not establish
owner approval, migration-policy acceptance, complete human review of the
large diff, or a repository-wide write result.
