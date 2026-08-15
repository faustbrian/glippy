# Net IP Bytes Equality Rule Admission, 2026-08-15

## Decision

Admit `net-ip-bytes-equal` as a default correctness, types-tier rule. It
reports direct calls to exact `bytes.Equal` when both operands have the exact
`net.IP` type. `bytes.Equal` compares the underlying byte representation,
while `net.IP.Equal` treats equivalent 4-byte and 16-byte IPv4
representations as the same address.

The rule offers the suggestion-only `use-net-ip-equal` fix. Replacing byte
equality with semantic IP equality is an intentional behavior correction, so
ordinary safe fixing does not select it. The fix preserves left-to-right
operand evaluation, selects the first operand as the receiver, parenthesizes it
where selector precedence requires that, and is withheld when replacing the
call would remove a comment. The shared coordinator removes `bytes` only when
the accepted edit makes its final qualified use unused.

## Defect Evidence

Current Staticcheck SA1021 at
`d69e7ee19e2d79b721aa696626cea310c807dd3e` recognizes exact `bytes.Equal`
calls whose two values originate from `net.IP` and recommends `net.IP.Equal`.
Its contract and fixtures exclude ordinary byte slices and calls where only
one operand is an IP address.

The current `github.com/deepflowio/deepflow` revision
`e567b167453ffa99f08f26def20379b4f831e073` contains two live occurrences in
`server/libs/kubernetes/interface.go`. `InterfaceInfo.Less` at line 63 uses
byte equality before ordering IP values, and `InterfaceInfo.Equal` at line 92
uses it as the equality definition:

```go
nIP, oIP := n.IPs[i].IP, other.IPs[i].IP
if !bytes.Equal(nIP, oIP) {
	return false
}
```

Both operands have exact `net.IP` type. Equivalent IPv4 addresses can therefore
compare unequal when one value uses the 4-byte representation and the other
uses the 16-byte representation. A direct non-mutating Glippy run against a
disposable clone of the exact revision reported both lines.

The compiler accepts every admitted form because `net.IP` has byte-slice
underlying representation and satisfies `bytes.Equal`. The supported default
`go vet` catalog has no equivalent diagnostic identity.

## Precision Contract

The rule uses `go/types` package, function, and named-type identity. Import
aliases, dot imports, and aliases of `net.IP` remain recognized. Ordinary byte
slices, one-IP comparisons, distinct named wrappers with `net.IP` underlying
type, local lookalike methods, function values, and direct `net.IP.Equal` calls
do not report.

The diagnostic and suggestion target the complete `bytes.Equal` call. Source
for both operands is retained byte-for-byte inside the replacement, while
comments outside those operand ranges withhold the edit. Generated files and
packages with type errors are excluded through shared policy, and exact-rule
suppressions retain normal ownership. The rule supports Go 1.25 and Go 1.26
source and adds no configuration, CFG, SSA, dependency syntax, or facts.

## Evidence And Cost

The first focused test failed because the `net-ip-bytes-equal` rule ID was
absent. Final fixtures cover import aliases, dot imports, `net.IP` aliases,
ordinary byte slices, one-IP calls, distinct named wrappers, local lookalikes,
function values, direct semantic comparisons, exact ranges and messages,
source-version selection, suppressions, generated files, and type errors.

The 2026-08-15 fixability revisit first failed because the diagnostic exposed
no suggestion. Final rule fixtures prove the named safety classification,
receiver precedence, retained operand comments, and comment-based withholding.
The CLI transaction fixture proves the accepted suggestion reparses, formats,
passes typed reanalysis, removes a newly unused qualified `bytes` import,
reports that derived import change in JSON, and reaches a clean fixed result.

Five one-iteration cold package probes over 100 findings on Go 1.26.6, Darwin
arm64, and an Apple M4 Max measured a median of 137.3 ms, about 4.60 MB, and
42,593 allocations. Package loading dominates the proportional probe; the rule
performs one filtered call traversal and constant-time object and type identity
checks.

Non-mutating exact-rule dogfood completed without findings or tool failures on
Glippy at `b4b55dae7e681a1c40c177607e1d9caccdf78b0f` and
`go-libraries/pkg/prompts` at
`e38bab8527e9ec97f668b262b23c70660cac0378`. Both runs used `GOWORK=off` after
exact dependencies were prefetched into a disposable module cache. The
pre-existing prompts worktree state was unchanged.

The fixability revisit repeated non-mutating dogfood with the complete default
policy over the current Glippy working tree and with only
`net-ip-bytes-equal` enabled over `go-libraries/pkg/prompts` at
`1b36d9c660ce793bbcd7922d19f853dfa53cbf6f`. Both runs completed without
findings or tool failure, and the external target remained unchanged.

## Revisit Triggers

Revisit conversions and other named IP wrappers only when reviewed defects
show that value provenance can improve coverage without treating arbitrary byte
slices as addresses. Add representation-sensitive comparisons other than
`bytes.Equal` only from an exact semantic contract and real-defect evidence.
