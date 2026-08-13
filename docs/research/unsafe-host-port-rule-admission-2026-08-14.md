# `unsafe-host-port` Rule Admission, 2026-08-14

## Decision

Admit the Go `hostport` analyzer as warning-level `correctness`. Formatting
`host:port` with `fmt.Sprintf` omits brackets required for IPv6 literals, so
recognized dialing calls fail for valid hosts. Go 1.26.6 default `go vet`
provides the data-flow check.

## Boundary

The typed x/tools v0.48.0 rule recognizes `%s:%s` and `%s:%d` address values
used by supported `net` dial APIs. Unrelated formatting does not report. Its
type-index prerequisite is shared through Glippy's analyzer graph. Generated
files and ill-typed packages are excluded, exact suppressions apply, and
pre-1.25 sources do not select it. `net.JoinHostPort` is suggestion-only because
imports and malformed-input formatting may change.

## Cost And Dogfood

A one-iteration package-load probe measured 109,170,834 ns on darwin/arm64.
Non-mutating dogfood found no diagnostics across 178 Glippy files or 57 prompts
files at `2c9842015ab62fd7790f0d99bf54855ffa7000f2`. CLI tests prove formatted,
validated, repeated suggestion application.
