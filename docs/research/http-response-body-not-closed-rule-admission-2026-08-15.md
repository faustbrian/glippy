# `http-response-body-not-closed` Rule Admission, 2026-08-15

## Decision

Admit `http-response-body-not-closed` to the opt-in `suspicious` preset at
warning severity. The native control-flow rule tracks directly acquired
standard-library HTTP responses after a conventional successful error guard
and reports when a normally returning path neither closes nor conservatively
transfers the response body.

## Defect And Existing Tools

Go 1.26.6 documents that callers must close `Response.Body` and that a
successful `Client.Do` response has a non-nil body. Failing to close it can
retain network resources and prevent persistent-connection reuse. The default
Go vet catalog checks only response use before the acquisition error; it does
not prove that every successful path closes the body.

The current `timakin/bodyclose` implementation was reviewed at
`857993a2939c1245454bd304459ac3c7e61a98ad`. It establishes the value of this
diagnostic but uses broader SSA escape heuristics and optional body-consumption
policy that Glippy does not adopt as its initial contract.

A current public occurrence was reviewed in `knadh/listmonk` commit
`d946ce543f90d77caf751efab54ab86f94127e14`, `cmd/updates.go`. Its update check
closes the body only after status validation and `io.ReadAll`; both a non-200
response and a read failure return first. The focused fixture reproduces both
open paths and distinguishes reading a body from transferring its ownership.

## Precision, Policy, And Fixes

The rule recognizes the exact `net/http` package functions `Get`, `Head`,
`Post`, and `PostForm`, plus the corresponding `Client` methods and
`Client.Do`. An assignment or one-spec initialized local `var` declaration must
bind a direct `*http.Response` and error and be followed immediately by an
`err != nil` guard whose body returns. Declarations containing multiple
specifications and parallel multi-expression declarations remain conservative.
Ownership begins on the successful continuation, avoiding redirect-error
responses that the standard client may already have closed.

An exact `response.Body.Close()` completes the obligation. Returning, passing,
sending, storing, or capturing the response transfers ownership. Returning,
storing, or sending the body also transfers it. Passing the body transfers
ownership when unavailable helper facts leave only a destination parameter
with `Close() error`, including variadic closer parameters. An exact
same-module or configured summary takes precedence: proven borrowing preserves
the obligation, while guaranteed close or transfer ends it. Passing the body
as `io.Reader` to `io.ReadAll`, a decoder, or another consumer does not imply
cleanup.

The shared bounded obligation engine reports conditional cleanup and response
reassignment. During admission, its method-value classifier was corrected to
require a real `types.MethodVal`; field selectors such as `response.Body` no
longer masquerade as ownership-transferring method values. Existing SQL
transaction and local-closer regressions remain green under the narrower
classification.

The rule remains `suspicious`, not `correctness`, because arbitrary helpers can
close a response without accepting a close-capable parameter and the initial
contract deliberately does not infer interprocedural effects. Generated files
and ill-typed packages are excluded. No fix is offered because cleanup
placement, error handling, and whether ownership should move are contextual.

## Admission Evidence

Focused package fixtures cover all admitted package and client acquisitions,
partial cleanup, read failures, reassignment, direct and deferred close,
complete branches, response and body transfer, closer-typed variadic transfer,
lookalikes, noncanonical guards, exact ranges, suppressions, generated files,
type errors, source versions, and absence of fixes.

Five complete-load iterations over 100 candidates on Go 1.26.6, Darwin arm64,
Apple M4 Max measured `396,092,967 ns/op`, `7,606,955 B/op`, and `72,014
allocs/op`. Package loading dominates this proportional probe; it is not a
stable latency budget.

Non-mutating exact-rule dogfood completed without findings over the current
Glippy tree based on `044fd9e` and `go-libraries/pkg/prompts` at
`5ead3d540eb6109a6bc8cfc2a2449640cb847108`. Both repository status
fingerprints and the prompts head were unchanged.

## Revisit Trigger

Add `RoundTripper` or helper-summary support only from reviewed defects and
measured precision. Promote the rule to `correctness` only if interprocedural
effect evidence closes the known helper-cleanup false-positive boundary without
making default analysis disproportionately expensive.
