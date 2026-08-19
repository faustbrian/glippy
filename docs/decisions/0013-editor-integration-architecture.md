# ADR 0013: Editor Integration Architecture

- Status: accepted; persistent diagnostics and code actions admitted for v0.3
- Date: 2026-08-12

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Context And Evidence

Glippy already exposes a complete-file editor boundary through standard input and
standard output. `--stdin-filepath` provides only file identity and
configuration-discovery context. Configuration, parsing, formatting,
equivalence validation, and idempotency validation complete before formatted
bytes are returned. Fragment formatting has a separate explicit kind and does
not infer a contract from malformed input.

The 2026-08-11 editor probe measured the complete one-shot formatter path on a
non-isolated Darwin arm64 reference host. All 100 fresh-process samples for the
owned 879-byte workload completed within the provisional 250 ms local adoption
budget. The observed 3.9-151.2 ms range is evidence that process startup is
currently usable on that host, not a cross-platform latency guarantee or a
stable regression threshold.

The current Oxfmt 0.63.0 source at Oxc commit
`73acba93fba517cee1f584951e41d250a59de591` retains
[`--stdin-filepath`](https://github.com/oxc-project/oxc/blob/73acba93fba517cee1f584951e41d250a59de591/apps/oxfmt/src/cli/command.rs)
and also provides an
[LSP formatter](https://github.com/oxc-project/oxc/blob/73acba93fba517cee1f584951e41d250a59de591/apps/oxfmt/src/lsp/server_formatter.rs).
Its server advertises document formatting, formats supplied in-memory text,
returns one minimal edit, synthesizes paths for non-file URIs, watches
configuration, and swaps configuration snapshots without invalidating
in-flight readers. It also carries Oxfmt-specific multi-language,
`.editorconfig`, nested-configuration, ignore, and external-service policy.
Those are evidence for the concerns a persistent service must own, not evidence
that Glippy needs the same service before a measured Go workflow requires it.

## Decision

The initial stable formatter integration MUST remain the one-shot process
contract:

```text
glippy fmt --stdin-filepath=/absolute/path/to/source.go
```

The editor MUST send the exact current buffer on standard input. On success,
Glippy MUST return only the complete formatted buffer on standard output. The
editor MUST retain the original buffer on every nonzero exit or output-stream
failure. Glippy MUST NOT read or write the file named by `--stdin-filepath` during
this mode; the path supplies only source identity and configuration-discovery
context. An editor SHOULD pass a normalized absolute path for a saved buffer
and SHOULD omit the flag when an unsaved buffer has no truthful project path.

Complete-file formatting MUST reject invalid Go instead of producing a partial
edit. Fragment buffers MUST select their fragment kind explicitly. Invalid or
unknown configuration MUST remain a visible failure and MUST NOT silently fall
back to built-in defaults. An integration MUST NOT run another formatter over
the same buffer after Glippy because a second layout owner can reintroduce churn.

Glippy MUST NOT add an LSP or long-running editor service solely to avoid process
startup while the supported one-shot workloads meet their recorded latency
budget. A persistent service MAY be introduced when measured evidence shows a
material benefit for at least one of these owned boundaries:

- supported-host or representative-file formatter latency;
- continuous diagnostics that would otherwise repeat typed package loading;
- version-bound lint code actions that cannot use a filesystem transaction;
- shared package or result-cache lifetime across editor requests; or
- two validated editor consumers that require a protocol feature the process
  contract cannot provide safely.

Any future service MUST reuse the same source, configuration, formatter,
analysis, suppression, fix-coordination, and reporting contracts as the CLI.
It MUST NOT create a second layout dialect or a weaker configuration fallback.
Each request MUST bind the exact document version or source digest that produced
its diagnostics and edits. Configuration changes MUST replace an immutable
resolved snapshot; in-flight requests MAY finish against their original
snapshot, while later requests MUST use the replacement. Cancellation MUST
prevent publication of a result that was not completely validated.

A future editor lint action MUST preserve the existing single-file transaction
order: select one explicitly authorized named fix, reject stale or overlapping
edits, apply it to the exact buffer version, reparse, format through the shared
formatter, reparse and validate the final source, and only then return the final
replacement. Suggestion and unsafe fixes MUST require their existing explicit
authorization. The editor MUST NOT apply a semantic edit and then delegate
normalization to an unrelated formatter. Multi-file actions remain prohibited
until a separate recovery and atomicity decision is accepted.

The initial release therefore supports format-on-save through standard
input/output and external lint or check invocations. It does not advertise live
diagnostics, editor code actions, or an LSP server.

## Alternatives Rejected

- Add an LSP immediately because Oxfmt has one: rejected because the current
  Glippy formatter path meets its provisional local budget and no validated Glippy
  consumer yet requires persistent protocol state.
- Maintain editor-specific libraries or plugins: rejected because they would
  duplicate configuration, validation, and release boundaries in each editor.
- Let a service ignore invalid configuration or invalid source: rejected
  because it would make editor output disagree with CLI and CI outcomes.
- Apply lint edits and formatting as separate editor actions: rejected because
  stale ranges, overlapping edits, or a second formatter could expose an
  intermediate or noncanonical buffer.

## Consequences

Formatter integrations pay process startup for each request, but remain easy to
adopt in editors that already support stdin/stdout formatters. The initial
binary has no resident daemon, protocol lifecycle, workspace watcher, or
in-memory cache invalidation surface. Live diagnostics and code actions remain
unavailable until their benefit and safety contracts justify that surface.

The recorded reference-host timings do not prove latency on every supported
platform or file-size class. Release readiness still requires broader editor
latency evidence proportional to the supported-platform claim.

## Revisit Trigger

Revisit this decision when a supported environment exceeds the formatter
latency budget, representative typed editor diagnostics repeatedly pay package
loading cost, a validated editor adoption requires safe in-memory code actions,
or shared cache lifetime demonstrates a material measured improvement.

## 2026-08-14 Revisit

The trigger is satisfied by version-bound lint actions over unsaved buffers and
continuous typed diagnostics using exact package overlays. The v0.3 service
therefore adds `glippy lsp` without replacing the established one-shot
formatter path.

The admitted service reuses configuration discovery, source loading, package
analysis, persistent cache identity, suppressions, baselines, formatter
validation, and fix coordination. It supports full-document synchronization,
versioned diagnostics, formatting, individual authorized fixes, safe fix-all,
request cancellation, and stale-version refusal. Suggestion and unsafe actions
remain explicit process-start flags, and no LSP operation writes project
source. Incremental synchronization, non-file documents, multi-file actions,
workspace mutation, and a separate editor plugin API remain outside this
decision.

## 2026-08-16 Revisit

Repeated typed analysis of one buffer at a time produced inconsistent package
state when several related files had unsaved changes, and reopening dependent
documents repeated equivalent package loads. The service now captures one
immutable, canonically ordered snapshot of all open Go buffers for each editor
event. Compatible same-package documents under the same root, configuration,
source language, and analysis tier share one package load, and every open document is
republished so an upstream change invalidates dependent diagnostics.

The batch contract preserves exact document versions, treats incompatible URI
aliases as errors, excludes buffers outside the selected root from its overlay,
and keeps formatting and code actions document-local. Persistent reuse across
successive changed snapshots, debounce, superseded-version cancellation,
configuration watching, and reusable typed graphs remain later work under the
same revisit trigger.

## 2026-08-19 Revisit

Successive package reuse still depended on polling captured filesystem inputs
only after another document event. With clients that advertise the capability,
the service now dynamically registers bounded
`workspace/didChangeWatchedFiles` patterns and accepts created, changed,
or deleted absolute local file notifications. It cancels any superseded
analysis, records canonical changed paths in the shared backend session, and
analyzes one new immutable snapshot of all open documents.

The retained package graph identifies the directly affected package and open
reverse dependants. Unrelated package results remain reusable, including when a
notification reports unchanged bytes, while editor overlays remain
authoritative. Retained results are now bounded by both eight entries and a
deterministic 128 MiB accounted-memory budget. Fully indexed source is charged
at sixteen times its exact bytes and compact dependency source at twice its
bytes, based on the distinct retention classes observed in the typed-memory
profile. This is an eviction weight rather than a process RSS claim. The newest
oversized entry remains available alone; older entries cannot accumulate around
it. Same-package incremental type checking and persistent typed graphs remain
separate later decisions.

Previously, opening a package group without a retained result invalidated every
cached package beneath the same workspace root because the new import identity
was not yet known. The session now analyzes cache-miss groups first and uses
their validated root package paths for the subsequent reuse decision. An
unrelated new package leaves known results reusable, while a newly opened
dependency invalidates retained reverse dependants. Failed or unidentified
graphs preserve the conservative root-wide fallback. This changes scheduling,
not analysis identity: same-package edits still rebuild their complete typed
representations, and metadata-only graph discovery remains future work.

The next retained-state boundary incrementally re-typechecks compatible
same-package overlay edits. Each eligible entry keeps compact dependency package
types but discards dependency syntax and type-value maps. A changed root is
reparsed in full into a fresh file set, rechecked through `go/types`, and then
passed through the ordinary CFG, SSA, effect, suppression, baseline, and action
pipelines. Result retention and typed-graph retention each use an eight-entry,
128 MiB stable weight, preserving a 256 MiB ordinary aggregate session budget.
Nested packages use the same path. Mutable local dependency source and module
control are revalidated before reuse, including local module replacements
outside the selected project root, while immutable toolchain and module-cache
inputs avoid per-edit source polling. File notifications discard retained
graphs without waiting for a package load to finish.

The service can admit a newly direct import only when its exact package is
already present in the retained graph and Go internal/vendor visibility permits
the root to import it. Base, internal-test, and external-test variants may be
retained as one bounded family and are rechecked in that order. An external
test import of the package under test resolves to the freshly checked internal
variant. A single internal or external test root is also reusable when its
closed dependencies remain unchanged. Repeated edits refresh the retained
family instead of degrading to a later full load.

The service falls back to the standard package loader for malformed or
ambiguous test families, cgo-generated sources, imports absent from the retained
graph, dependency overlays, file notifications, source-membership,
build-constraint, or project-control changes, parse/type failures, and every
uncertain graph. This conservative fallback is part of the editor correctness
contract. Incremental support for those cases requires separate evidence; it is
not inferred from successful root type checking.
