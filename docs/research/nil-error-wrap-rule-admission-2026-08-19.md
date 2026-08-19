# `nil-error-wrap` Rule Admission, 2026-08-19

## Decision

Admit `nil-error-wrap` to the opt-in `suspicious` preset at warning severity.
The native SSA rule reports an exact `fmt.Errorf` `%w` operand only when the
operand is literal nil or an exact built-in `error` value proven nil on the
control-flow path to the call. Generated files and ill-typed packages are
excluded, and no fix is offered.

The diagnostic is a correctness finding, but the initial preset remains
`suspicious` so the default profile does not make every package pay for SSA
debug mappings before broader dogfood establishes its signal and cost. Exact
selection and the recommended, strict, and pedantic profiles can enable the
rule without changing default correctness latency. Recommended already selects
SSA debug consumers including `nilness` and `overwritten-error`, so this
addition does not raise that profile's maximum tier or builder mode; it adds the
measured rule callback work.

## Defect And Existing Tools

Go 1.26.6's `fmt.Errorf` implementation returns a non-nil wrapping error even
when its `%w` operand is nil. The standard-library fixture requires
`fmt.Errorf("%w", nil)` to render `%!w(<nil>)` while `errors.Unwrap` returns
nil. The malformed text can escape through logs and API responses while the
result misleadingly appears to carry an underlying cause.

The open Go issue
[`golang/go#32808`](https://github.com/golang/go/issues/32808) asks the
`x/tools` nilness analyzer to detect exactly this path-proven misuse. The
current Go 1.26.6 `printf` analyzer validates `%w` operand types but accepts an
ordinary error-typed value and does not establish its path state. The current
`nilness` implementation has no `fmt.Errorf` consumer, so the default Go
toolchain does not make the rule redundant.

The defect occurs in maintained Go repositories. The 2026
[`e2b-dev/infra#3581`](https://github.com/e2b-dev/infra/issues/3581) report
records two non-directory error paths returning `%!w(<nil>)` after a successful
`os.Stat`. The 2026
[`jaegertracing/jaeger#8881`](https://github.com/jaegertracing/jaeger/issues/8881)
report records two factory-selection failures that wrap an error already
proven nil. The accepted contract covers both control-flow shapes without
requiring application-specific facts.

## Precision And SSA Contract

The rule resolves the exact standard-library `fmt.Errorf` object and requires a
compile-time format string. It maps ordinary sequential formatting directives
to arguments, recognizes each `%w`, and rejects explicit argument indexes or
star width and precision conservatively. Dynamic formats and missing arguments
do not report.

Literal nil reports directly. Other operands must have the exact built-in
`error` interface type and map to the SSA value for that source expression. A
nil SSA constant reports. Otherwise, the value must participate in an exact
equality or inequality comparison with nil, and the comparison's nil outcome
edge must dominate the `fmt.Errorf` call.

Edge dominance is evaluated by proving that the target is unreachable from the
function entry when that exact control-flow edge is removed. Successor-block
dominance alone is insufficient: a nil successor that is also a loop back-edge
can dominate the next iteration's body even though the current iteration took
the non-nil edge. The exact edge proof retains nil-branch and nil-fallthrough
findings while excluding ordinary `if err := operation(); err != nil` returns
inside loops.

Typed nil pointers, named error-like interfaces, values forwarded through
aliases or phis without an exact proof, dynamic formats, indexed directives,
and star operands remain conservative. The rule does not infer cross-package
return state. No fix is registered because the intended repair may return nil,
remove wrapping, select a different error, construct a new error, or alter the
surrounding branch.

## Behavioral And Cost Evidence

The first focused test failed because the registry did not recognize the rule.
After initial implementation, exact-rule self-dogfood exposed thirteen false
positives in ordinary non-nil error branches inside loops. A focused loop
regression then failed with a fifth diagnostic where four proven-nil findings
were expected. SSA inspection showed that the false nil successor was the loop
header: its block dominated the reporting block through the cycle, but its
incoming edge did not. Exact edge dominance corrected the first divergence.

The green suite covers literal nil, zero-value errors, explicit nil branches,
nil fallthrough, non-nil branches with and without initializers, loop-contained
non-nil branches, unknown values, dynamic formats, indexed directives, typed
nil pointers, suppressions, generated files, type errors, source versions,
exact ranges, metadata, and absence of fixes.

Five complete 100-function, 100-finding package-analysis samples ran on Go
1.26.6, Darwin arm64, and an Apple M4 Max:

| Sample | Time | Bytes | Allocations |
| ---: | ---: | ---: | ---: |
| 1 | 63.48 ms | 5,304,389 | 54,915 |
| 2 | 64.97 ms | 5,303,965 | 54,914 |
| 3 | 63.44 ms | 5,303,888 | 54,914 |
| 4 | 65.79 ms | 5,302,676 | 54,909 |
| 5 | 63.21 ms | 5,303,861 | 54,912 |

The median was 63.48 ms, 5,303,888 bytes, and 54,914 allocations per
operation. Each operation includes fresh package loading and debug SSA
construction, so this is proportional admission evidence rather than a
portable latency budget.

Non-mutating exact-rule dogfood completed without findings on Glippy and on
`go-libraries/pkg/prompts` with its standalone module selected. The prompts
run preserved its pre-existing `go.sum` diff and untracked-file state
byte-for-byte.

## Revisit Trigger

Revisit default correctness admission after multi-repository dogfood proves
near-zero noise and portable measurement shows that default SSA debug mappings
fit the stable latency and memory budgets. Extend value forwarding or
cross-package return facts only when reviewed defects justify the added state
contract. Do not add a fix without one canonical semantics-preserving repair.
