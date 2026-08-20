# `http-response-body-used-after-close` Rule Admission, 2026-08-19

## Decision

Admit `http-response-body-used-after-close` to the opt-in `suspicious` preset
at warning severity. The rule reports direct reads, selected standard-library
reader operations, and repeated closes only when every reaching path proves
that the exact locally acquired `net/http.Response.Body` was already closed.

The rule is not `correctness`. `Response.Body` has the static type
`io.ReadCloser`, and a custom `RoundTripper` can return an implementation whose
post-close behavior is not standardized. The default HTTP/1 transport returns
`http: read on closed response body`, but that implementation behavior cannot
justify a universal correctness claim.

## Toolchain Boundary

Go 1.26.6 documents that `Response.Body` is streamed on demand and that callers
must close it. The default `httpresponse` vet analyzer reports a response body
closed before checking the acquisition error; it does not track operations
after close. Staticcheck 2026.1 exposes the corresponding SA5001 acquisition
ordering check and no response-body use-after-close rule.

The compiler accepts these operations because `io.ReadCloser` does not encode
an open or closed state. Glippy therefore adds a distinct path-sensitive state
contract without duplicating the compiler, vet, Staticcheck, or the existing
`http-response-body-not-closed` ownership diagnostic.

## Precision Contract

The acquisition boundary is shared with `http-response-body-not-closed`:

- `http.Get`, `Head`, `Post`, and `PostForm`;
- `Client.Do`, `Get`, `Head`, `Post`, and `PostForm`; and
- assignment or one-spec initialized local `var` bindings for the response and
  error;
- an immediately following `err != nil` guard whose body returns.

Declarations containing multiple specifications and parallel multi-expression
declarations remain conservative.

Analysis starts at the successful guard continuation with one open response.
An exact `response.Body.Close()` call or a statically proven close effect moves
the value to closed state. Direct `Body.Read`, repeated `Body.Close`, and exact
`io.ReadAll`, `ReadAtLeast`, `ReadFull`, `Copy`, `CopyN`, and `CopyBuffer`
reader arguments report only from the exact all-path closed state.

Conditional closure joins open and closed state and does not report. Aliases,
reassignment, ownership transfer, asynchronous or deferred execution, unknown
helpers, nested tracked calls with ambiguous evaluation order, and unsupported
consumers become conservative unknown state. A later exact close can
reestablish closed state. Proven borrow summaries preserve the current state;
proven transfer summaries stop tracking.

## Cost And Safety

The cheapest sufficient tier is CFG plus existing parameter-effect facts. Each
candidate uses the bounded finite state-transition runner starting at its own
successful acquisition continuation. Candidate count and total transition
changes are capped; exceeding either bound fails closed with no diagnostic.
The package benchmark must cover 100 response acquisitions and findings before
the rule is considered complete.

Generated files and ill-typed packages are excluded. Standard suppressions own
the exact operation range. The rule offers no fix because moving a read, close,
or acquisition requires caller intent and may change observable I/O behavior.

## Admission Evidence

Focused red tests establish the previously missing direct-read, immediate
`io`-consumer, repeated-close, branch-join, helper-effect, conservative escape,
suppression, generated-file, type-error, language-version, and no-fix
contracts.

`justjcurtis/kit` supplies a reviewed real occurrence at immutable revision
`6c38e8fc6d197336b2f2a752a513430966d3e005`. In
`internal/cli/cli.go`, `updateBinary` acquired `resp` through direct
`http.Get`, returned from the immediately following `err != nil` guard, closed
`resp.Body`, checked the status, and then called `io.ReadAll(resp.Body)`. The
project fixed the defect in commit
`c6ae77fb247df432c52d09281649f017593e7a29`, whose release notes identify the
body-close ordering bug and whose patch replaces the premature close with a
deferred close. Running the candidate Glippy binary against the exact pre-fix
revision reported the `io.ReadAll(resp.Body)` call and related the preceding
close; the fixed revision no longer contains the closed-state read. This
occurrence matches the admitted acquisition, guard, close, and consumer
boundaries without relying on helper inference or adapted source.

The proportional package benchmark ran five iterations on an Apple M4 Max,
Darwin arm64, and Go 1.26.6. It uses 100 acquisitions and 100 findings:

| Rule | Time | Bytes | Allocations |
| --- | ---: | ---: | ---: |
| `http-response-body-not-closed` | 134.94 ms | 8,101,067 | 75,942 |
| `http-response-body-used-after-close` | 136.30 ms | 9,196,774 | 88,761 |

Non-mutating exact-rule dogfood completed with no findings or prerequisite
problems on Glippy and `go-libraries/pkg/prompts`. The reviewed Kit occurrence
establishes positive defect evidence, but one historical occurrence does not
establish profile-wide noise or adoption behavior. The rule therefore remains
outside the curated `recommended` profile; the broader `suspicious` group or
exact rule ID remains the honest opt-in boundary.

The complete repository tests, affected race tests, vet, module tidy-diff,
generated-documentation freshness, candidate build, formatter check, and
candidate self-check pass. Final admission is subject only to the complete
final-diff review recorded with this batch.
