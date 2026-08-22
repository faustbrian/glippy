# `uncatchable-signal` Rule Admission, 2026-08-22

## Decision

Admit `uncatchable-signal` to the default `correctness` preset at warning
severity. The rule uses the existing types tier and reports exact `os/signal`
calls that attempt to affect `SIGKILL` or `SIGSTOP`. It offers no fix.

## Defect and authority

The Go `os/signal` documentation states that `SIGKILL` and `SIGSTOP` cannot be
caught and therefore cannot be affected by the package. Passing either signal
to `Ignore`, `Notify`, `NotifyContext`, or `Reset` requests behavior that the
runtime cannot provide. The compiler accepts these calls, and the standard vet
catalog has no equivalent diagnostic.

Staticcheck SA1016 has enforced the same standard-library contract for
`Ignore`, `Notify`, and `Reset`. Glippy also includes `NotifyContext` because it
registers the same signal set through the same `os/signal` delivery mechanism:

- <https://pkg.go.dev/os/signal>
- <https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa1016/sa1016.go>
- <https://github.com/dominikh/go-tools/tree/2026.2.1/staticcheck/sa1016/testdata/go1.0/CheckUntrappableSignal>

## Detection contract

The rule recognizes direct calls, by typed object identity, to the package-level
`os/signal.Ignore`, `Notify`, `NotifyContext`, and `Reset` functions. It checks
every signal argument after the channel or context parameter where applicable.
An argument reports only when it directly references `os.Kill`,
`syscall.SIGKILL`, or `syscall.SIGSTOP`; explicit type conversions around one
of those objects are unwrapped. Each offending argument has its own exact
primary range and deterministic source order.

User-defined functions with matching names, function-value aliases, local
constant or variable aliases, arbitrary numeric signal conversions, and other
catchable signals do not report. Generated files and ill-typed packages remain
excluded through shared typed-rule policy.

## False-positive and fix boundary

Exact standard-library function and signal object identity proves the call
cannot achieve its requested effect. Conservative value-flow exclusions trade
false negatives for a near-zero false-positive boundary appropriate to the
default correctness preset.

There is no automatic fix. Removing the only signal from `Notify`, `Ignore`, or
`Reset` changes the call to its all-signals form, while replacing the signal
with `SIGTERM` or another catchable signal changes application behavior.

## Cost expectation and evidence

The rule participates in the shared typed call-expression traversal and does
constant bounded work per argument. It does not request CFG, SSA, facts,
dependency syntax, or another package load. The default correctness preset
already requires a more expensive tier, so admission does not raise its
maximum analysis requirement.

Focused behavior covers five positive arguments across all four APIs, nearby
non-diagnostics, conversion handling, exact ranges, deterministic ordering,
metadata, source-version gating, generated and ill-typed exclusion,
suppression ownership, and configured severity. Five one-iteration samples of
the retained 100-call in-process package benchmark measured 71.94-74.05 ms,
4.34-4.90 MB, and 31,463-31,845 allocations. Process-tree, signal,
interruption, Docker, and RSS probes are explicitly outside the permitted
evidence boundary. Exact-rule non-mutating dogfood was clean over Glippy and
`go-libraries/pkg/prompts`.
