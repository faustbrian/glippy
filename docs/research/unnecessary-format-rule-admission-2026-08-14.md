# `unnecessary-format` Rule Admission, 2026-08-14

## Decision

Admit `unnecessary-format` to the opt-in `pedantic` preset at warning severity.
The types-tier rule reports only standard `fmt.Sprintf` calls whose compile-time
constant format contains no percent sign and has no formatting arguments.

## Evidence And Boundary

Revive's current `unnecessary-format` rule was inspected at
[`1de8243783d480e24c0db1a3dc45976aeaf715e9`](https://github.com/mgechev/revive/commit/1de8243783d480e24c0db1a3dc45976aeaf715e9).
Its broader fmt, log, testing, and trace surface produced a repository-wide
pedantic flood during Glippy dogfood. The contract was therefore narrowed,
through a failing noise regression, to `fmt.Sprintf` returning an unchanged
constant string. Dynamic formats, percent escapes, extra arguments, logging,
testing, errors, printing, and scanning are excluded.

No fix is offered because replacing the last fmt use also requires coordinated
import ownership, which this single-expression rule does not claim.

## Admission Evidence

The initial focused test failed because the rule was absent; the later noise
test failed with four diagnostics before the narrowed contract emitted one.
Exact ranges, close negatives, metadata, shared policy, and source versions
pass. A one-iteration typed package probe measured `104,708,042 ns/op`,
including loading. The narrowed rule produced no diagnostics in non-mutating
Glippy or `pkg/prompts` dogfood at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc`.

## Revisit Trigger

Broaden one API family at a time only after real-repository noise evidence and
a unique alternative justify that family.
