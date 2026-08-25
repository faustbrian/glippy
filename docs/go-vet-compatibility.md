# Go Vet Compatibility

Glippy reuses selected analyzers from `golang.org/x/tools` and integrates them
with one rule registry, preset model, suppression syntax, baseline format,
deterministic reporter, and fix coordinator. It does **not** claim to replace
the complete `go vet` command.

This boundary was refreshed on 2026-08-25 against Go 1.27.0 and x/tools v0.49.0.
It must be revisited when either dependency changes its analyzer catalog or
behavior.

## Directly Adapted Analyzers

| Go analyzer | Glippy rule |
| --- | --- |
| `appends` | `append-no-values` |
| `assign` | `self-assignment` |
| `atomic` | `atomic-update-assignment` |
| `buildtag` | `invalid-build-constraint` |
| `copylocks` | `copied-lock` |
| `defers` | `deferred-time-since` |
| `directive` | `invalid-directive` |
| `errorsas` | `errors-as-target` |
| `hostport` | `unsafe-host-port` |
| `httpresponse` | `http-response-before-error` |
| `ifaceassert` | `impossible-type-assertion` |
| `nilfunc` | `nil-function-comparison` |
| `printf` | `printf-arguments` |
| `shift` | `oversized-shift` |
| `sigchanyzer` | `unbuffered-signal-channel` |
| `slog` | `invalid-slog-arguments` |
| `stdmethods` | `standard-method-signature` |
| `stdversion` | `standard-library-version` |
| `stringintconv` | `suspicious-string-conversion` |
| `structtag` | `invalid-struct-tag` |
| `testinggoroutine` | `testing-goroutine-call` |
| `tests` | `invalid-test-signature` |
| `timeformat` | `time-layout` |
| `unmarshal` | `invalid-unmarshal-target` |
| `unusedresult` | `unused-result` |

`bools` is adapted only for its contradictory-condition subset. Glippy owns
other boolean rules under separate contracts and does not expose the complete
analyzer as one rule.

## Native Overlapping Contracts

| Go analyzer | Glippy boundary |
| --- | --- |
| `loopclosure` | `loop-capture` is native and follows Glippy's supported-source-version and range contract. It is not promised to reproduce every `loopclosure` diagnostic. |
| `lostcancel` | `context-cancel-leak` is native and uses shared CFG. Package-local summaries and exact standard-library terminal APIs are shared, but Glippy does not reproduce `lostcancel`'s complete transitive dependency-fact graph. |
| `unreachable` | `unreachable-code` is native and preserves the upstream first-statement and run-despite-errors behavior while using Glippy's shared no-return analysis for selected local-source module helpers. |
| `waitgroup` | `waitgroup-misuse` is native. It preserves the upstream direct positive-`Add` boundary only when an outside `Wait` proves the race, and adds bounded same-package helper tracing when stable exact parameter identity reaches that same waited-on receiver. |

These native rules address the same defect families, but they are documented
differences rather than analyzer-equivalence claims.

## Unsupported Default Vet Analyzers

Glippy does not currently run these Go 1.27.0 default analyzers:

- `asmdecl` and `framepointer`, which require Go assembly ownership;
- `cgocall` and `unsafeptr`, whose unsafe and cgo boundaries need separate
  admission evidence;
- `composites`, whose unkeyed-literal policy and compatibility exceptions are
  not admitted as a default Glippy rule.

Run `go vet` in addition to Glippy when any unsupported analyzer is required.

## Exact Printf Fact Execution

`printf` is the only adapted analyzer whose dependency facts run through an
audited external boundary. The CLI first releases its ordinary typed package
graph, then invokes the same Glippy executable as an exact upstream unitchecker
through `go vet -json -p=2`. The command is serialized, cancellation-aware,
offline and read-only under the package-loading policy, and bounded to 64 MiB
stdout plus 1 MiB stderr. Temporary overlay files are task-owned and removed.

Decoded diagnostics are rebound to Glippy's exact retained source versions.
Dependency and ineligible generated-file diagnostics remain hidden, duplicate
test variants collapse deterministically, and suggested edits remain subject to
the declared Glippy safety class and complete fix coordinator. Internal callers
without the executable runner retain the bounded in-process fact scheduler.
This is not a generic vettool, runtime plugin, or external-analyzer API. The
decision and resource evidence are in
[`research/v0.5-printf-fact-execution-2026-08-19.md`](research/v0.5-printf-fact-execution-2026-08-19.md).

## Behavioral Differences

- Glippy rule IDs, severities, preset membership, suppressions, baselines,
  generated-file policy, and source-version gates are product-owned contracts.
- `standard-method-signature` remains an exact `stdmethods` adapter but is
  pedantic rather than default correctness because a conventional method name
  alone does not prove intent to implement the corresponding standard
  interface.
- `printf` uses the upstream default formatting-function set; Glippy does not
  expose `-printf.funcs`.
- `unused-result` uses the upstream default function and string-method sets;
  Glippy does not expose `-unusedresult.funcs` or
  `-unusedresult.stringmethods`.
- The experimental `testinggoroutine` subtest flag remains disabled.
- Upstream suggested fixes are never assumed safe. Glippy classifies admitted
  `printf`, `stringintconv`, and `hostport` edits as suggestions. The native
  `unreachable-code` removal is also suggestion-only. Selected edits pass
  through stale-source, overlap, reparse, formatting, validation, and
  atomic-write checks.
- Most adapted typed rules exclude ill-typed packages. `invalid-struct-tag` and
  the native `unreachable-code` rule retain their upstream run-despite-errors
  behavior.
- Generated files are not analyzed by these rules and are never writable by
  default.
- Glippy intentionally strips upstream analyzer flag sets whose storage is
  package-global instead of permitting cross-run mutation.

The canonical metadata and exact limitations for each supported rule are
available through `glippy explain <rule>` and the generated
[lint rule catalog](lint-rules.md).
