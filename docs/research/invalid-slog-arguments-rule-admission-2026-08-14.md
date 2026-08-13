# `invalid-slog-arguments` Rule Admission, 2026-08-14

## Decision

Admit the Go `slog` analyzer as warning-level `correctness`. A non-string key or
an unmatched final key violates the structured logging argument contract and
produces malformed records or runtime diagnostics. Go 1.26.6 default `go vet`
provides the standard check.

## Boundary

The typed x/tools v0.48.0 rule recognizes standard `log/slog` calls and the
wrapper behavior supported upstream. Valid string/value pairs and `slog.Attr`
arguments do not report. Dynamic argument slices remain outside the direct-call
contract. Generated files and ill-typed packages are excluded, exact
suppressions apply, and pre-1.25 sources do not select it. No fix can infer the
missing key or value.

## Cost And Dogfood

A one-iteration package-load probe measured 114,482,417 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`.
