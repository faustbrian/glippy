# `needless-blank-identifier` Rule Admission, 2026-08-14

## Decision

Admit `needless-blank-identifier` to the opt-in `pedantic` preset at warning
severity. The types-tier rule reports range variables and channel-receive
assignments whose values Go can discard without an explicit blank identifier.

## Evidence And Boundary

Staticcheck S1005 is the primary Go authority, inspected at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
The compiler and default vet catalog accept these forms. Glippy follows S1005
for ordinary ranges and receives, excludes range-over-function variables and
map-lookup presence documentation, and reports exact physical ranges.

The `remove-blank-identifier` fix is suggestion-only. It preserves the receive
or range operation, refuses comment-dropping edits, and passes complete-file
reparse, formatting, typed validation, and repeated application.

## Admission Evidence

The focused tests failed first because the rule was absent. Positive, nearby
negative, exact-range, metadata, suppression, generated-file, type-error,
source-version, fix, and CLI idempotency cases pass. A one-iteration typed
package probe on Go 1.26.6 Darwin arm64 measured `54,570,417 ns/op`, including
package loading. Non-mutating Glippy and `pkg/prompts` dogfood at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc` produced no diagnostics.

## Revisit Trigger

Broaden discard forms only when the syntax communicates no additional presence
or arity intent and one comment-preserving replacement is provable.
