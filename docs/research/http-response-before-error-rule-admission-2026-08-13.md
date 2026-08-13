# `http-response-before-error` Rule Admission, 2026-08-13

## Decision

Admit `http-response-before-error` to `correctness` at warning severity by
adapting x/tools v0.48.0 `httpresponse.Analyzer`. It uses the types tier,
excludes generated and ill-typed packages, and offers no fix.

## Defect And Existing Tools

An HTTP operation may return a nil response with a non-nil error. Deferring
`response.Body.Close` before checking the error can therefore panic and mask
the request failure. The compiler accepts the ordering. Go 1.26.5 enables
`httpresponse` in default vet.

The upstream fixtures cover package functions, client methods, parenthesized
expressions, aliases, nearby correct ordering, and regression case 66259.
Staticcheck SA5001 detects the broader early-defer pattern, including non-HTTP
closers. Glippy deliberately retains the narrower standard HTTP contract for
the default correctness group. Clippy has no Go `net/http.Response` ownership
contract; its resource rules are not interchangeable.

Sources inspected on 2026-08-13:

- x/tools v0.48.0 `go/analysis/passes/httpresponse` source, tests, and fixtures;
- Go 1.26.5 `go tool vet help`;
- Staticcheck v0.8.0-rc.1 SA5001; and
- the current Clippy resource-lifecycle catalog.

## Precision, Policy, And Fixes

The analyzer recognizes the standard `net/http` functions and client methods
and reports response use before the corresponding error check. Glippy maps the
diagnostic to the exact source bytes and provides deterministic suppressions
and baselines. Generated and ill-typed packages are excluded.

No fix is registered. Moving a defer across intervening statements can change
evaluation order or cleanup scope, and broader response ownership remains
outside this rule.

## Evidence And Cost

Focused CLI fixtures report the defer-before-check pattern and accept the
check-before-defer form. Policy tests cover suppressions, generated and
type-error exclusion, baselines, and absence of fixes.

| Case | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| typed baseline | 162,996,700 | 5,344,929 | 43,352 |
| `http-response-before-error` | 146,818,375 | 5,355,033 | 43,342 |

These are five complete-load samples on Go 1.26.5, Darwin arm64, Apple M4 Max.
Non-mutating correctness lint completed with no findings over Glippy and
`go-libraries/pkg/prompts` at `633a5508c570d08b8976689a206f9df27e73ff90`;
the external worktree was unchanged.
