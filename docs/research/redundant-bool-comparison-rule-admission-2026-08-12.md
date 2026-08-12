# `redundant-bool-comparison` Rule Admission, 2026-08-12

## Authority And Existing-Tool Boundary

The rule detects equality and inequality comparisons between a statically
boolean expression and a compile-time boolean constant. These comparisons are
valid Go, but they add no information and often obscure the condition being
tested.

Current Staticcheck independently defines this class as
[S1002](https://github.com/dominikh/go-tools/blob/d69e7ee19e2d79b721aa696626cea310c807dd3e/simple/s1002/s1002.go)
at `go-tools` commit `d69e7ee19e2d79b721aa696626cea310c807dd3e`.
That commit was still repository HEAD on 2026-08-12. The implementation uses
typed constant values, requires the other operand's complete type set to be
boolean, excludes generated code, and supplies a simplification edit.

Go 1.26.5's vendored `bools` analyzer inspects repeated or contradictory
operands within `&&` and `||` chains. It does not diagnose a standalone
comparison with a boolean constant, so the rule adds an opt-in contract beyond
the default vet surface.

## Contract And False-Positive Boundary

`redundant-bool-comparison` is a warning in the opt-in `style` preset. It
requires the types tier, subscribes to binary expressions, excludes generated
files and ill-typed packages, and recognizes `==` and `!=` only. Either operand
may be a literal, named constant, or another expression whose typed constant
value is boolean. The retained operand must have predeclared `bool`, an alias
resolving directly to it, or untyped boolean type. Candidates with a retained
dynamically typed or defined boolean value, numeric comparisons, ordinary
boolean-to-boolean comparisons, shadowed identifiers named `true`, and
unresolved type parameters do not report.

A retained defined boolean operand is excluded because the comparison result
has type `bool`, while the operand retains its defined type. A comparison may
therefore be an intentional type normalization whose removal would change an
interface value's dynamic type. Parent-context analysis would be required to
distinguish that use from a redundant condition without introducing false
positives.

## Fix Safety

The named `simplify-comparison` fix is classified as safe only when the
retained operand is predeclared `bool`, an alias resolving to it, or untyped
boolean. Its truth-table transformations are:

| Comparison | Replacement |
| --- | --- |
| `value == true`, `true == value` | `value` |
| `value != false`, `false != value` | `value` |
| `value != true`, `true != value` | `!value` |
| `value == false`, `false == value` | `!value` |

The compile-time constant contributes no runtime evaluation, and the retained
operand remains evaluated exactly once. Negation preserves precedence by
retaining existing parentheses or adding a group around non-atomic operands.
The edit uses the exact immutable source slice, preserving spelling and comments
inside the retained operand. If any comment within the comparison would fall
outside the retained range, the rule reports without a fix instead of deleting
or relocating that comment.

Ordinary `gox lint --fix` may select this safe fix. The shared typed-fix path
then reparses, formatter-normalizes, reanalyzes through an exact overlay, and
atomically replaces only a validated result. A second invocation is a no-op.

## Behavioral Evidence

The initial focused rule and CLI tests failed because the metadata and
registration did not exist and `--fix` left the comparison unchanged. After
implementation, an additional precedence fixture failed because a
parenthesized boolean chain received redundant double parentheses; the source
rewrite now retains the existing grouping. A later safety fixture failed
because a defined boolean operand still received a diagnostic even though the
comparison could intentionally normalize its result type; that ambiguous case
is now excluded.

The focused fixtures cover:

- constants on either side of both equality operators;
- literals, untyped named constants, predeclared booleans, and aliases;
- exact primary and edit ranges, fix name, and safe classification;
- precedence, existing unary negation, parentheses, and comments retained
  inside an operand;
- fix refusal when removed trivia contains a comment;
- non-constant, dynamically typed, defined boolean, numeric, and shadowed-`true`
  exclusions;
- generated files, type errors, suppression ownership, severity overrides, and
  disabled-rule behavior;
- stable JSON diagnostics and canonical `gox explain` output; and
- formatter-normalized typed fixing, reanalysis, and second-run idempotency.

## Cost And Dogfood Signal

On Darwin arm64 with an Apple M4 Max and Go 1.26.5, five 200 ms benchmark
samples over one already-loaded package containing 100 findings ranged from
63.565 to 91.317 microseconds per complete shared types traversal. The median
was 64.506 microseconds, with approximately 126.0 KiB and 733 allocations per
run. Package loading is intentionally excluded so the measurement isolates the
incremental rule cost within the shared types tier.

Non-mutating style-preset dogfood over the current Gox worktree analyzed 113
files with zero diagnostics, suppression problems, or tool errors. An immutable
`git archive` snapshot of `go-libraries` revision
`1223a78db4de383b0313383cb3b88f7e6289b798` produced:

| Module selection | Files | Diagnostics |
| --- | ---: | ---: |
| `pkg/openrpc/jsonschema` | 7 | 1 |
| `pkg/kafka` | 68 | 0 |
| `pkg/search/adapters/opensearch` | 54 | 0 |

The OpenSearch fixture includes `body["version"] == true`, but the map lookup
has static type `any`; its exclusion confirms the typed false-positive
boundary. An earlier implementation reported ten Kafka comparisons whose
option values have defined boolean types; the final contract excludes those
potential type-normalization comparisons. The remaining OpenRPC comparison has
a predeclared boolean operand and provides positive real-repository signal.
Dogfood was read-only and does not claim external adoption or approval of the
suggested style.

## Admission Decision

Admit `redundant-bool-comparison` as the first built-in `style` rule and the
first built-in safe fix. Keep the rule opt-in because it improves clarity
rather than detecting incorrect behavior. Revisit defined boolean diagnostics
only if parent-context analysis can prove that result-type normalization is not
part of the expression's observable contract.
