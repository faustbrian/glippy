# `buffer-string-conversion` Rule Admission, 2026-08-14

## Decision

Admit `buffer-string-conversion` to the opt-in `pedantic` preset at warning
severity. The types-tier rule reports `string(buffer.Bytes())` and
`[]byte(buffer.String())` for direct `bytes.Buffer` receivers.

## Evidence And Boundary

Staticcheck S1030 is the primary Go authority, inspected at
[`d69e7ee19e2d79b721aa696626cea310c807dd3e`](https://github.com/dominikh/go-tools/commit/d69e7ee19e2d79b721aa696626cea310c807dd3e).
Glippy retains S1030's map-index exclusion because
`map[string(buffer.Bytes())]` benefits from a compiler allocation optimization.
Defined conversion targets, promoted lookalike methods, and the `bytes` package
itself are excluded.

No fix is offered: `Buffer.Bytes` aliases mutable storage whereas a newly
converted byte slice does not, so the apparent simplification is not a general
semantic-preserving edit.

## Admission Evidence

The focused test failed first because the rule was absent. Both directions,
map indexing, lookalikes, exact ranges, metadata, shared policy, and source
versions pass. A one-iteration typed package probe measured `106,158,833
ns/op`, including loading. Non-mutating Glippy and `pkg/prompts` dogfood at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc` produced no diagnostics.

## Revisit Trigger

Add a fix only for a context whose aliasing and lifetime contract proves the
replacement cannot expose mutable buffer storage differently.
