# `standard-method-signature` Rule Admission, 2026-08-14

## Initial Decision

Admit the Go `stdmethods` analyzer as warning-level `correctness`. A method
named for a conventional standard interface but carrying the wrong signature
silently fails dynamic interface checks. Go 1.26.6 default `go vet` owns the
canonical method catalog.

## Boundary

The typed rule follows x/tools v0.48.0's fixed method-name and signature table,
including its intent signals and error-method restrictions. Canonical
signatures and unrelated multi-argument `WriteTo` methods do not report. A
deliberately unrelated same-name method may need suppression. Generated files
and ill-typed packages are excluded; pre-1.25 sources do not select it. No fix
is offered because changing a method signature changes callers and method sets.

## Cost And Dogfood

A one-iteration package-load probe measured 177,530,333 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.

## v0.6 Calibration

The initial preset decision is superseded by the
[v0.6 profile calibration](v0.6-standard-method-signature-profile-calibration-2026-08-24.md).
The analyzer remains admitted and explicitly selectable, but broad pinned
corpus evidence moves it from default correctness to pedantic.
