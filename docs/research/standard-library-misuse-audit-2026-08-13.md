# Standard-Library Misuse Audit, 2026-08-13

## Scope And Authorities

This audit closes the Glippy v0.2 expansion request for `context`, `errors`,
`time`, HTTP, and atomic APIs. It inspected Go 1.26.5 source and documentation,
the standard vet analyzer catalog, Staticcheck at
`d69e7ee19e2d79b721aa696626cea310c807dd3e`, and Clippy at
`9a73ad846274efca140b1d2ea316b830fa1fb8de`. The supported Glippy source
versions are Go 1.25 and 1.26; supported runtime targets are macOS and Linux on
amd64 and arm64.

## Decisions

| Area | Capability | Decision | Reason |
| --- | --- | --- | --- |
| context | inappropriate `WithValue` keys | Retain `context-key` | Typed identity proves the standard API and the existing opt-in rule covers collision and panic hazards beyond default vet. |
| context | lost cancellation | Retain `context-cancel-leak` | The adapted lost-cancel semantics are already integrated through shared CFG scheduling and deterministic Glippy policy. |
| context | nil context arguments | Retain `nil-context` | Direct nil values violate the package contract; the rule checks every exact `context.Context` parameter and excludes tests by default from measured noise. |
| errors | reversed `errors.Is` arguments | Retain `errors-is-arguments` | Typed argument roles expose a common silent false result that default vet does not report. |
| errors | invalid `errors.As` targets | Retain adapted `errors-as-target` | The standard `errorsas` analyzer is authoritative and Glippy adds cohesive configuration, suppressions, baselines, and reporting without reimplementation. |
| time | bare duration literals | Retain `time-duration-unit` | Selected waiting APIs interpret literals as nanoseconds; the suspicious group avoids guessing that every literal is accidental. |
| time | transposed reference layout | Retain adapted `time-layout` | The standard `timeformat` analyzer provides the authoritative diagnostic and suggestion. |
| time | `time.Tick` leaks | Reject for v0.2 | Since Go 1.23 the garbage collector can recover unreferenced tickers. Both supported source versions are newer, so the historic SA1015 defect no longer applies. |
| time | pre-Go-1.23 timer reset/drain patterns | Reject for v0.2 | The supported source versions use the newer timer-channel contract. A rule would be a targeted migration for unsupported source versions, not current correctness. |
| HTTP | response used before error check | Retain adapted `http-response-before-error` | The standard `httpresponse` analyzer catches the proven nil-response dereference pattern. |
| HTTP | local values with `Close() error` | Retain `resource-not-closed` | The conservative general ownership rule covers direct call results whose own static type has `Close() error`; `*http.Response` itself does not satisfy that contract. |
| HTTP | direct response-body ownership | Admit `http-response-body-not-closed` | The CFG rule recognizes exact package and Client acquisitions after a successful error guard, distinguishes body consumption from closer-typed ownership transfer, and reports partial cleanup and reassignment. |
| HTTP | noncanonical direct header keys | Admit `http-canonical-header-key` | Constant direct map access bypasses method canonicalization; public fixes and Staticcheck SA1008 establish the defect and exception boundary. |
| HTTP | interprocedural response-body ownership | Defer | Proving cleanup through arbitrary helpers, middleware, body replacement, and custom transports still needs effect facts beyond the admitted local contract. |
| atomic | update result assigned back through the same pointer | Retain adapted `atomic-update-assignment` | The standard atomic analyzer proves the second non-atomic write and resulting race hazard. |
| atomic | 64-bit alignment on 32-bit targets | Defer outside v0.2 support | Staticcheck SA1027 is target-sensitive and applies to 32-bit architectures, while Glippy v0.2 supports amd64 and arm64 runtimes only. Revisit if 32-bit analysis targets are admitted. |
| atomic | typed wrapper analogues | Reject as currently redundant | `atomic.Int64`, `atomic.Uint64`, and related wrapper methods return scalar values, so the legacy pointer double-write shape does not type-check as an assignment back to the wrapper. Other ignored-return patterns can be intentional. |

## Cohesive Boundary

The admitted surface uses object or named-type identity rather than import
spelling, excludes generated files by default, and does not add layout rules.
Standard analyzers remain adapted where they already own the semantics. Native
rules exist only for broader argument positions, narrower false-positive
contracts, or capabilities absent from the standard analyzer set.

No additional automatic fix was admitted. Context selection, duration units,
resource ownership, raw header-map semantics, and atomic synchronization intent
cannot be chosen safely from syntax alone.

## Revisit Triggers

Revisit when a supported Go release changes one of these API contracts, when
Glippy adds 32-bit runtime or analysis support, or when dogfood supplies a
repeatable HTTP ownership defect that package facts can prove without guessing
transfer semantics.
