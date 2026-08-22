# `failed-type-assertion-value` Rule Admission, 2026-08-22

## Decision

Admit `failed-type-assertion-value` to the default `correctness` preset at
warning severity. The rule reports a read only when an exact short type
assertion shadows its own input and SSA proves that the read still denotes the
failed assertion result:

```go
if value, ok := value.(string); ok {
	return value
} else {
	return fmt.Sprintf("unexpected %T", value)
}
```

On the failure edge, the inner `value` is the zero value of `string`; it is not
the original interface value. This can corrupt fallback behavior and error
reporting without producing a compile or runtime error.

The rule has no fix. Renaming the assertion result, retaining the original
value under another binding, or restructuring the branch can all be correct,
but selecting among them requires caller intent.

## Defect And Existing Tools

Staticcheck v0.8.1 SA9008 is the primary external authority. Its current
source at
[`1285a6a5ec1e0ebb658f49e82b6c566a878cc3cb`](https://github.com/dominikh/go-tools/tree/1285a6a5ec1e0ebb658f49e82b6c566a878cc3cb/staticcheck/sa9008)
matches the short assertion structurally and verifies reads against its IR
value before reporting. Glippy follows that precision model through the
standard Go AST, type information, and x/tools SSA debug mappings instead of
adopting Staticcheck's frontend.

Go 1.27's default vet catalog has no equivalent analyzer. Its assignment and
impossible-interface-assertion checks do not diagnose the failed assertion's
zero value being read from the `else` scope.

The defect has caused real merged bugs:

- [`hneemann/parser#1`](https://github.com/hneemann/parser/pull/1), merged as
  [`563d1c8d3539d09f9a7fd659a7fc23271e82e86a`](https://github.com/hneemann/parser/commit/563d1c8d3539d09f9a7fd659a7fc23271e82e86a),
  renamed a `vList` assertion result after error reporting printed the empty
  asserted slice instead of the original value;
- [`openbkn-ai/bkn-foundry#1076`](https://github.com/openbkn-ai/bkn-foundry/pull/1076)
  fixed a failed `*NoticeHandlerConnector` assertion whose `else` branch stored
  the nil assertion result and silently discarded the original
  `driver.Connector`.

These occurrences cover both misleading diagnostics and lost production state.

## Precision Contract

The syntax must be exactly `if value, ok := value.(T); ok` with an `else` block
or `else if`. The first result must shadow the exact asserted identifier, and
the condition must be the exact second result identifier. Renamed results,
assignment forms, type switches, compound or negated conditions, and missing
failure branches do not report.

For each read of the shadowing object in the failure branch, the rule asks the
enclosing SSA function for its exact expression value. A diagnostic is emitted
only when that value is still the index-zero extraction from the assertion.
Reassignment, address-taking, closure capture, and Phi joins therefore remain
conservative. Pointer and interface assertion results are covered because
their failed result is likewise a typed zero value. The primary range owns the
proven read; related ranges identify both the shadowing result declaration and
the original asserted value.

The rule requires SSA debug mappings, excludes generated files and ill-typed
packages, supports exact suppressions and configured severity, and is absent
for source versions before Go 1.25. It registers no safe, suggestion, or unsafe
fix.

## Evidence And Cost Boundary

The focused product test first failed because the rule ID was absent. Current
package fixtures cover string, pointer, interface, and `else if` findings;
exact primary and related ranges; renamed and assignment forms; type switches;
compound and negated conditions; reassignment; address-taking; closure capture;
ambiguous joins; missing failure branches; metadata; suppressions; generated
and ill-typed exclusions; configured severity; minimum Go-version selection;
and absence of fixes.

The implementation performs one owned-function AST walk and invokes SSA value
mapping only for exact structural candidates and their same-object failure
reads. It introduces no independent package load or SSA program. It is,
however, the first default correctness member to require SSA debug mappings,
so the default preset now raises its maximum tier from control flow to SSA and
selects `ssa.GlobalDebug`. A dedicated package benchmark is present for future
safe execution, but it measures the absolute exact-rule workload rather than a
with-and-without-debug default-preset delta.

No runtime benchmark, RSS, signal, interruption, process-tree, or
descendant-cleanup probe was executed in this batch because those probes are
excluded on the development host after an unsafe process-cleanup incident.
This admission therefore makes no fresh portable latency or memory claim;
release evidence must establish the default-tier delta at a non-disruptive
native CI boundary.

## Revisit Triggers

Revisit renamed results only if reviewed defects show that broader shadowing
can retain near-zero false positives. Revisit assignment forms or branch
conditions only with an equally exact object and edge proof. Do not report a
Phi, captured value, address-taken value, or post-assignment read merely because
it has the same `types.Object`. Do not add a fix without one canonical
semantics-preserving transformation.
