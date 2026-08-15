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

The `use-format-operand` suggestion replaces the complete call with the exact
source bytes of its constant operand. It is suggestion-only because a typed
string constant can preserve its defined type when used directly, while
`fmt.Sprintf` always produces an ordinary string. The rule withholds the edit
when replacement would remove a comment. It does not organize imports: when
the call is the final `fmt` use, complete-file typed validation rejects the edit
and preserves the original source.

## Admission Evidence

The initial focused test failed because the rule was absent; the later noise
test failed with four diagnostics before the narrowed contract emitted one.
Exact ranges, close negatives, metadata, shared policy, and source versions
pass. A one-iteration typed package probe measured `104,708,042 ns/op`,
including loading. The narrowed rule produced no diagnostics in non-mutating
Glippy or `pkg/prompts` dogfood at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc`.

The 2026-08-15 fixability revisit proves literal and named-constant
replacements, retained parenthesized comments, lossy-comment refusal,
successful complete-file application while another `fmt` use remains,
validation refusal when the edit leaves `fmt` unused, and repeated fixed-point
behavior.

## Revisit Trigger

Broaden one API family at a time only after real-repository noise evidence and
a unique alternative justify that family.
