# `overlapping-encoder-slices` Rule Admission, 2026-08-22

## Decision

Admit `overlapping-encoder-slices` to the default `correctness` preset at
warning severity. The rule uses the existing SSA tier and reports expanding
standard-library encoders whose destination and source slices are proven to
start in the same storage. It offers no fix.

## Defect and authority

`encoding/ascii85.Encode`, `encoding/hex.Encode`, and the `Encode` methods of
`encoding/base32.Encoding` and `encoding/base64.Encoding` write expanded output
while continuing to read input. When destination and source overlap at the
same lower bound, a destination write can replace source bytes before they are
read and silently corrupt the result. The compiler accepts these calls, and
the standard vet catalog has no equivalent diagnostic.

Staticcheck SA1031 enforces this exact non-overlap contract. Glippy follows its
2026.2.1 call set and overlap proof rather than generalizing to arbitrary
encoders:

- <https://github.com/dominikh/go-tools/blob/2026.2.1/staticcheck/sa1031/sa1031.go>
- <https://pkg.go.dev/encoding/ascii85#Encode>
- <https://pkg.go.dev/encoding/hex#Encode>
- <https://pkg.go.dev/encoding/base32#Encoding.Encode>
- <https://pkg.go.dev/encoding/base64#Encoding.Encode>

## Detection contract

The rule recognizes exact static calls to the two package functions and two
methods by typed function and receiver identity, including method expressions
and statically resolved package-function or bound-method aliases. Same-file
package variables initialized directly by `make` or a composite literal also
participate in the typed-variable fallback. Package initializer aliases,
unknown values, and cross-file declarations remain conservative unless SSA
independently proves overlap. Named-slice conversions represented by SSA
change-type instructions are normalized together with equivalent phi values.

A call reports when destination and source are the same typed variable,
normalize to the same non-nil SSA value, or are slices of the same normalized
base whose lower bounds are the same value or equal constants. The primary
range is the exact destination argument, and diagnostics retain deterministic
source order.

Nil inputs, distinct buffers, different or unproven lower bounds, dynamic
calls, user-defined lookalikes, decoder APIs, and append encoders do not report.
Generated files and ill-typed packages remain excluded through shared SSA-rule
policy.

## False-positive and fix boundary

Exact standard-library identity and same-start storage prove that these
expanding encoders may overwrite unread input. Conservative alias and range
reasoning trades false negatives for the near-zero false-positive boundary
required by the default correctness preset.

There is no automatic fix. Choosing a separate allocation, a reusable buffer,
or a relocated destination depends on application ownership, capacity, and
allocation policy.

## Cost expectation and evidence

The rule scans each already-built SSA function once and performs bounded work
for each call instruction. It reuses shared SSA, syntax, type, and source-range
state and does not load packages or construct another semantic representation.
The default correctness preset already requires SSA, so admission does not
raise its maximum analysis tier.

Focused behavior covers all four encoder identities, direct and statically
resolved calls, method expressions, bound-method aliases, package initializer
ownership, named-slice conversions, copied aliases, equivalent phis, equal
slice bounds, nearby non-diagnostics, exact ranges, metadata, source-version
gating, generated and ill-typed exclusion, suppression ownership, and
configured severity. A retained one-iteration 100-call in-process package
benchmark measured 77.43 ms on darwin/arm64. Process-tree, signal,
interruption, Docker, and RSS probes are explicitly outside the permitted
evidence boundary.
Non-mutating exact-rule dogfood over the Glippy candidate based on `bb3429c`
and `go-libraries/pkg/prompts` at `8ea88fd` produced no diagnostics or tool
failures.
