# `dangerous-remove-all` Rule Admission, 2026-08-22

## Decision

Admit `dangerous-remove-all` to the opt-in `suspicious` preset at warning
severity. The rule detects an exact `os.RemoveAll` call whose argument is
directly proven to be the complete directory returned by one of:

- `os.TempDir`;
- `os.UserCacheDir`;
- `os.UserConfigDir`; or
- `os.UserHomeDir`.

For example, this test cleanup can delete the shared system temporary
directory and disrupt unrelated applications and processes:

```go
directory := os.TempDir()
defer os.RemoveAll(directory)
```

The safe shape creates an application-owned child first:

```go
directory, err := os.MkdirTemp("", "project-*")
if err != nil {
	return err
}
defer os.RemoveAll(directory)
```

The rule remains suspicious rather than default correctness because a caller
can deliberately own a configured system directory in an isolated environment,
even though deleting one is almost always destructive. Such code requires a
narrow, reasoned suppression.

No fix is offered. Choosing the child name, ownership boundary, creation
lifecycle, and failure handling requires application intent.

## Defect And Existing Tools

Staticcheck 2026.2.1 SA9007 is the primary external authority. Its
[`sa9007.go`](https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa9007/sa9007.go)
implementation scans SSA call instructions, requires an exact call to
`os.RemoveAll`, and recognizes the direct results of the same four directory
functions. The published
[`CheckBadRemoveAll` fixture](https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa9007/testdata/go1.0/CheckBadRemoveAll/CheckBadRemoveAll.go)
covers deferred deletion, direct value flow, and conservative exclusion after a
possible child-path assignment.

Staticcheck's rule documentation explicitly identifies confusing `os.TempDir`
with a temporary-directory creator or forgetting to append a suffix to a user
directory as common causes. The defect is unusually severe: deleting the
shared temporary, cache, configuration, or home directory can remove unrelated
user and application data.

The Go compiler accepts these calls, and the Go 1.27 default vet catalog has no
equivalent analyzer.

## Precision Contract

The rule runs once per source function through Glippy's shared SSA program and
opts into the same rule contract for package-level variable initializers. The
initializer scheduler supplies the synthetic package `init` function once per
physical source file; only calls mapped to that file can report. Other SSA rules
remain function-only unless they explicitly implement this marker contract.
The rule requires exact static symbol identity for both `os.RemoveAll` and the
directory provider. Import aliases and local function aliases still report when
SSA retains exact static identity.

Direct `os.TempDir` string values and result zero extracted from the three
`(string, error)` user-directory calls report. Equivalent SSA phi values are
flattened only when every path resolves to the same source value. A possible
assignment of a child or unrelated path makes the origin ambiguous and does
not report.

Calls through unknown function parameters, helper returns, pointer loads,
dynamic providers, string concatenation, and `filepath.Join` remain
conservative. User-defined methods or functions with matching names do not
report. Package-initializer, direct, deferred, and asynchronous exact calls
share the same contract.

The primary range covers the complete deletion call. One related range points
to the exact directory-producing call. Shared severity, suppression,
generated-file, type-error, and source-version policies apply.

## Evidence And Cost Boundary

The focused product test first failed because the rule ID was absent. Current
fixtures cover all four directory providers, package-level initializers, direct
and local-variable flow, import and function aliases, equivalent and ambiguous
phi values, direct, deferred, and asynchronous deletion calls, exact primary
and related ranges, dynamic and transformed exclusions, user-defined symbol
exclusions, metadata, minimum Go-version selection, configured severity,
suppressions, generated files, ill-typed packages, and the explicit no-fix
boundary.

The implementation performs one bounded AST call-index construction and one
linear scan of the current function's SSA instructions. Phi flattening is
cycle-safe and visits each encountered value once. It adds no independent
package load, fact computation, subprocess, network access, or write.
Exact-rule non-mutating dogfood over Glippy and `go-libraries/pkg/prompts`
produced no diagnostics or tool failures. No
benchmark, RSS probe, signal test, interruption test, process-tree test,
descendant-cleanup test, Docker test, or filesystem deletion was executed for
this admission.

## Revisit Triggers

Revisit correctness placement only after broad external dogfood demonstrates
near-zero intentional deletion of these roots. Extend provenance only when the
new flow can be proven without treating transformed or helper-returned paths as
the complete directory. A suggestion may be considered only if an explicit
user-selected owned child and its error handling can be represented without
guessing.
