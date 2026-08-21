# `exported-api-documentation` Rule Admission, 2026-08-21

## Decision

Admit `exported-api-documentation` as an exact-ID restriction rule. It reports
exported functions, methods, types, constants, and variables without
substantive documentation. By default it also checks named exported struct
fields and interface methods and requires declaration-specific comments to
begin with the documented name.

The rule runs at the syntax tier, excludes generated files, package `main`,
and test files by default, and offers no fix. Four typed Boolean options control
test files, main packages, members, and name-prefix enforcement independently.
The canonical catalog now contains 118 rules, including six restriction rules.

## Current Authorities And Product Boundary

The official
[Go doc-comment guide](https://go.dev/doc/comment) defines comments immediately
preceding top-level declarations as documentation and recommends complete
sentences beginning with the declared name. Go 1.26.7 default vet does not
require documentation for every exported declaration.

Current Clippy source at
[`e8bea0328512886cca6e0c28e28f3e172689b484`](https://github.com/rust-lang/rust-clippy/tree/e8bea0328512886cca6e0c28e28f3e172689b484)
retains missing private-item documentation as a restriction lint, excludes
tests, and covers documentable members. Current Revive source at
[`e363474ee9f8fd507c8fae5dc7c60fc1341d8cab`](https://github.com/mgechev/revive/tree/e363474ee9f8fd507c8fae5dc7c60fc1341d8cab)
checks missing and malformed exported function, method, type, value, and public
interface-method comments. Staticcheck keeps package and exported declaration
comment-shape checks in its non-default style catalog.

Glippy does not enable this organizational policy through `pedantic` or
`style`. Documentation completeness varies materially between application,
command, generated-adapter, internal, and public-library trees. Exact-ID
restriction membership preserves the Clippy model: repositories select the
policy deliberately and use path overrides, baselines, or reasoned
suppressions for intentional boundaries.

## Observable Contract

The rule operates once over each parsed file:

- exported functions and methods use their declaration doc group;
- exported types use a specific doc group when present and otherwise the
  enclosing general-declaration doc group;
- exported constants and variables use the same specific-then-group ownership;
- exported named struct fields and interface methods accept leading or trailing
  member documentation, including members of unexported named types; and
- comments containing only recognized Go directives are not documentation.

Grouped declarations and multi-name members accept one substantive group
comment without requiring it to begin with every declared name. A specific
type comment may begin with the type name or the conventional `A`, `An`, or
`The` plus that name. Other declaration-specific comments must begin with the
exact exported identifier followed by a word boundary.

The rule intentionally excludes package-clause documentation. A file-level
syntax invocation cannot prove that another file in the package lacks the
canonical package comment, and moving this entire policy to the types tier
would violate the cheapest-representation contract. Package documentation
belongs in a later syntax-package aggregation boundary or a separate rule.
Exported embedded fields are also excluded because promotion and documentation
ownership require package-level type information.

Default configuration is:

```toml
[lint.rules]
exported-api-documentation = "warn"

[lint.rule-options."exported-api-documentation"]
include-tests = false
include-main = false
include-members = true
require-name-prefix = true
```

Every diagnostic covers the exported identifier. A malformed comment also
links its exact comment range. No fix is available because useful API
documentation cannot be synthesized from declaration syntax.

## Behavioral, Cost, And Dogfood Evidence

The focused suite began red with
`unknown rule "exported-api-documentation"`. Current fixtures cover documented,
missing, malformed, grouped, multi-name, directive-only, function, method,
type, constant, variable, struct-field, and interface-method cases. They also
cover articles before type names, unexported declarations, test and main
options, member and name-prefix options, generated files, suppressions, source
version selection, exact ranges, related comment ranges, and absence of fixes.

Five 100-iteration syntax-analysis samples on Go 1.26.7, Darwin arm64, and an
Apple M4 Max produced a median of `17,455 ns/op`, `43,405 B/op`, and
`110 allocs/op` for three findings. This is a proportional rule-admission
probe, not a release latency budget.

Non-mutating exact-rule dogfood found 1,552 sites in Glippy and 530 in the
approved `go-libraries/pkg/prompts` target at
`ee8dfbbb938d4a03e6b48c6c6772423457b94ef1`. Disabling member coverage reduced
the counts to 480 and 155 respectively. Both worktree status digests remained
unchanged. The volume confirms that documentation completeness is a
repository-adoption policy requiring baseline or path-scoped rollout, not a
credible default diagnostic.

## Revisit Triggers

Add package documentation only after the syntax scheduler can aggregate a
package without forcing type loading. Revisit embedded fields when package
type identity can distinguish promoted public API from private implementation
without speculative noise. Revisit default option values only from
human-reviewed adoption evidence, not raw finding counts.
