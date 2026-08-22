# `non-octal-file-mode` Rule Admission, 2026-08-22

## Decision

Admit `non-octal-file-mode` to the opt-in `suspicious` preset at warning
severity. A three-digit decimal integer made entirely of octal digits and used
directly as an exact `os.FileMode` or `io/fs.FileMode` argument is usually a
permission literal whose octal prefix was omitted:

```go
if err := os.WriteFile(path, data, 644); err != nil {
	return err
}
```

Decimal `644` is file mode `0o1204`, not `0o644`. The intended spelling is:

```go
if err := os.WriteFile(path, data, 0o644); err != nil {
	return err
}
```

The rule remains suspicious rather than default correctness because Go permits
decimal file modes and application-specific bit patterns can be deliberate.
Those rare cases require a narrow, reasoned suppression.

The rule offers `use-octal-file-mode` as a suggestion. Changing the literal
changes runtime permissions, so the edit is not semantics-preserving and MUST
NOT run under ordinary safe-fix selection.

## Defect And Existing Tools

Staticcheck 2026.2.1 SA9002 is the primary external authority. Its
[`sa9002.go`](https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa9002/sa9002.go)
implementation requires an exact `os.FileMode` or `io/fs.FileMode` argument and
reports only three-digit decimal literals whose digits are all valid in octal.

The defect has caused real incorrect permissions. Moby commit
[`fdc1b220`](https://github.com/moby/moby/commit/fdc1b22030f2a04931fb0005f34020109b0562b8)
corrected decimal `666` and `600` file modes after SA9002 showed that they
evaluated to `0o1232` and `0o1130` rather than the intended `0o666` and `0o600`.

The Go compiler accepts these literals, and the Go 1.27 default vet catalog has
no equivalent analyzer.

## Precision Contract

The rule uses filtered typed traversal over call expressions. For each direct
basic integer literal argument, it requires `go/types` to resolve the argument
to the exact standard `os.FileMode` type or its `io/fs.FileMode` alias. The
source spelling must contain exactly three ASCII digits, must not start with
zero, and every digit must be between zero and seven.

Canonical `0o` literals, legacy leading-zero literals, hexadecimal and binary
literals, digit-separated literals, constants, variables, calculated modes,
dynamic calls, and distinct defined mode types do not report. Import aliases,
exact type aliases, and explicit standard FileMode conversions retain the
standard identity and report. Generated files and packages with type errors
remain excluded.

The primary range covers only the decimal literal. The suggestion replaces
that exact range with the same digits prefixed by `0o`, preserving surrounding
comments and source. Shared severity, suppression, source-version, stale-edit,
conflict, reparse, formatter, and validation policies apply.

## Evidence And Cost Boundary

The focused behavioral test first failed because the rule ID was absent. Its
green acceptance cases cover standard file APIs, exact aliases, diagnostic and
edit ranges, octal evaluation in the message, explicit suggestion
authorization, formatted fixed output, fix idempotency, nearby
non-diagnostics, metadata, minimum Go-version selection, suppressions,
generated files, and ill-typed packages.

The implementation is limited to constant work per direct call argument. It
adds no CFG, SSA, fact, cache, subprocess, network, write, signal, process-tree,
RSS, interruption, descendant-cleanup, or Docker requirement. Five one-iteration
samples over 100 matching calls on Go 1.26.5, Darwin arm64, and an Apple M4 Max
measured 73.24-118.84 ms, 3,683,880-4,258,496 bytes, and
32,059-32,442 allocations per complete package load and analysis.

Exact-rule non-mutating dogfood over the current Glippy tree and
`go-libraries/pkg/prompts` produced no diagnostics or tool failures. No
benchmark outside the in-process rule benchmark, RSS probe, signal test,
interruption test, process-tree test, descendant-cleanup test, Docker test, or
filesystem deletion was executed for this admission.

## Revisit Triggers

Revisit correctness placement only after broad external dogfood demonstrates
that deliberate matching decimal modes are effectively absent. Extend the
source forms only when intent remains comparably strong. The suggestion MUST
remain explicit because correcting the mode changes program behavior.
