# Go Vet Compatibility

Glippy reuses selected analyzers from `golang.org/x/tools` and integrates them
with one rule registry, preset model, suppression syntax, baseline format,
deterministic reporter, and fix coordinator. It does **not** claim to replace
the complete `go vet` command.

This boundary was audited on 2026-08-14 against Go 1.26.6 and x/tools v0.48.0.
It must be revisited when either dependency changes its analyzer catalog or
behavior.

## Directly Adapted Analyzers

| Go analyzer | Glippy rule |
| --- | --- |
| `appends` | `append-no-values` |
| `assign` | `self-assignment` |
| `atomic` | `atomic-update-assignment` |
| `copylocks` | `copied-lock` |
| `defers` | `deferred-time-since` |
| `errorsas` | `errors-as-target` |
| `hostport` | `unsafe-host-port` |
| `httpresponse` | `http-response-before-error` |
| `ifaceassert` | `impossible-type-assertion` |
| `nilfunc` | `nil-function-comparison` |
| `printf` | `printf-arguments` |
| `shift` | `oversized-shift` |
| `slog` | `invalid-slog-arguments` |
| `stdmethods` | `standard-method-signature` |
| `stringintconv` | `suspicious-string-conversion` |
| `structtag` | `invalid-struct-tag` |
| `testinggoroutine` | `testing-goroutine-call` |
| `timeformat` | `time-layout` |
| `unmarshal` | `invalid-unmarshal-target` |
| `unreachable` | `unreachable-code` |
| `unusedresult` | `unused-result` |
| `waitgroup` | `waitgroup-misuse` |

`bools` is adapted only for its contradictory-condition subset. Glippy owns
other boolean rules under separate contracts and does not expose the complete
analyzer as one rule.

## Native Overlapping Contracts

| Go analyzer | Glippy boundary |
| --- | --- |
| `loopclosure` | `loop-capture` is native and follows Glippy's supported-source-version and range contract. It is not promised to reproduce every `loopclosure` diagnostic. |
| `lostcancel` | `context-cancel-leak` is native and uses shared CFG. Package-local summaries and exact standard-library terminal APIs are shared, but Glippy does not reproduce `lostcancel`'s complete transitive dependency-fact graph. |

These native rules address the same defect families, but they are documented
differences rather than analyzer-equivalence claims.

## Unsupported Default Vet Analyzers

Glippy does not currently run these Go 1.26.6 default analyzers:

- `asmdecl` and `framepointer`, which require Go assembly ownership;
- `buildtag` and `directive`, whose lexical/toolchain directive contracts need
  a dedicated source-fidelity integration;
- `cgocall` and `unsafeptr`, whose unsafe and cgo boundaries need separate
  admission evidence;
- `composites`, whose unkeyed-literal policy and compatibility exceptions are
  not admitted as a default Glippy rule;
- `sigchanyzer`, which has not completed Glippy's rule admission gate;
- `stdversion`, because Glippy has not stabilized a migration/version-policy
  rule for too-new standard-library symbols; and
- `tests`, whose test/example signature catalog has not completed admission.

Run `go vet` in addition to Glippy when any unsupported analyzer is required.

## Behavioral Differences

- Glippy rule IDs, severities, preset membership, suppressions, baselines,
  generated-file policy, and source-version gates are product-owned contracts.
- `printf` uses the upstream default formatting-function set; Glippy does not
  expose `-printf.funcs`.
- `unused-result` uses the upstream default function and string-method sets;
  Glippy does not expose `-unusedresult.funcs` or
  `-unusedresult.stringmethods`.
- The experimental `testinggoroutine` subtest flag remains disabled.
- Upstream suggested fixes are never assumed safe. Glippy classifies the
  admitted `printf`, `stringintconv`, `unreachable`, and `hostport` edits as
  suggestions, then applies selected edits through stale-source, overlap,
  reparse, formatting, validation, and atomic-write checks.
- Most adapted typed rules exclude ill-typed packages. `invalid-struct-tag` and
  `unreachable-code` retain their upstream run-despite-errors behavior.
- Generated files are not analyzed by these rules and are never writable by
  default.
- Glippy intentionally strips upstream analyzer flag sets whose storage is
  package-global instead of permitting cross-run mutation.

The canonical metadata and exact limitations for each supported rule are
available through `glippy explain <rule>` and the generated
[lint rule catalog](lint-rules.md).
