# `invalid-struct-tag` Rule Admission, 2026-08-14

## Decision

Admit the Go `structtag` analyzer as warning-level `correctness`. Tags rejected
by `reflect.StructTag.Get`, serialization tags on unexported fields, and
duplicate json/xml tags are ineffective or ambiguous. Go 1.26.6 default
`go vet` owns the upstream detection; Glippy owns product policy and reporting.

## Boundary

The typed rule follows x/tools v0.48.0 and does not interpret application tag
namespaces. Valid quoted tags and effective exported fields do not report. It
runs despite unrelated type errors, matching upstream, but excludes generated
files. Exact suppressions apply and sources before Go 1.25 do not select it.
No fix is offered because the intended tag spelling or field visibility is not
provable.

## Cost And Dogfood

A one-iteration package-load probe measured 255,680,792 ns on darwin/arm64; it
is proportional evidence, not a release budget. Non-mutating dogfood found no
diagnostics across 178 Glippy files or 57 prompts files at
`2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
