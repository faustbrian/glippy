# Initial Pedantic Catalog Audit, 2026-08-13

## Scope And Sources

This audit resolves every candidate proposed for the first Glippy pedantic
catalog. It inspected Go 1.26.5 compiler and default vet behavior, Staticcheck
at `d69e7ee19e2d79b721aa696626cea310c807dd3e`, Clippy at
`d2c4d1532d89488a56ec2c3ca12757117fc0b4e2`, Revive at
`1de8243783d480e24c0db1a3dc45976aeaf715e9`, and unconvert at
`4a038b3d31f56ff5ba511953b745c80a2317e4ae`.

Formatting layout remains formatter-owned. None of these rules controls spaces,
indentation, line width, wrapping, braces, or blank lines.

## Disposition

| Candidate | Disposition | Product boundary |
| --- | --- | --- |
| Redundant else after termination | Admitted as `redundant-else` | Syntax proof only; no initializer or `else if`; no fix |
| Unnecessary conversions | Admitted as `unnecessary-conversion` | Identical runtime types only; constants excluded; no fix |
| Unnecessary allocations | Deferred | Escape behavior and replacement ownership need a narrower defect contract than a general allocation heuristic |
| Needless blank-identifier discards | Deferred | `discarded-error` already covers the bug-prone error case; Staticcheck S1005 should be evaluated as a separate simplification adapter |
| Overly broad interfaces | Deferred | Requires package-consumer evidence and a stable definition of breadth; whole-program assumptions would be noisy |
| Confusing receiver naming | Admitted narrowly as `inconsistent-receiver-name` | Consistency only; generic `this`, `self`, and length policy remain separate style questions |
| Mixed pointer and value receivers | Admitted as `mixed-receiver-type` | Opt-in because deliberate mutator/observer splits exist; never auto-fixed |
| Unnamed results in complex signatures | Deferred | Needs evidence-backed complexity thresholds and naming-quality boundaries |
| Excessive parameter or result complexity | Deferred | Needs small typed threshold options and real-repository calibration before admission |
| Unidiomatic error inspection or wrapping | Existing narrow coverage retained | `errors-is-arguments`, `errors-as-target`, vet, and standard library misuse rules cover proven defects; broader wrapping taste is not yet a rule |
| `fmt.Sprintf` where direct use is clearer | Admitted as `unnecessary-sprintf` | Exact `%s`, one argument, proven direct representations; no fix |
| Inefficient string construction | Deferred | Evaluate modern `strings.Builder`, `strings.Concat`, and compiler optimizations against measured cases before selecting a contract |
| Exported API documentation policy | Deferred to restriction/style | Organizational documentation coverage is not suitable for wholesale pedantic enablement |
| Needless nested control flow | Partially admitted | `redundant-else` owns the proven terminating-branch subset; broader flattening may change scope or readability |
| Redundant closures or function literals | Deferred | Immediate invocation, defer, panic, captures, generics, and comment movement need a narrow semantic contract |

## Initial Catalog Contract

The five admitted rules are all warning-level, opt-in `pedantic`, individually
configurable, excluded from generated files, and fix-free. Types-tier rules do
not run in ill-typed packages; the syntax-tier `redundant-else` rule remains
available when unrelated type errors exist. All use the shared deterministic
suppression, baseline, JSON, explain, scheduling, and cache contracts.

Dogfood found three true-positive `redundant-else` opportunities and one
intentional `mixed-receiver-type` exception across Glippy and the immutable
`go-libraries/pkg/prompts` revision
`6ed3a06a4e1aba412d2a6b91454774234f30a464`. That result supports keeping this
group opt-in and requiring explicit suppressions where a receiver-form exception
is a deliberate ownership signal.

## Revisit Order

Next evaluate the bounded, existing-tool overlaps first: Staticcheck S1005 for
blank-identifier simplification and a narrow redundant-closure contract. Do not
admit allocation, interface-breadth, signature-complexity, or documentation
rules until real-code evidence establishes useful boundaries and option
defaults.
