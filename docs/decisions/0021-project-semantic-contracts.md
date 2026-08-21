# ADR 0021: Versioned Project Semantic Contracts

- Status: accepted for v0.5 development
- Date: 2026-08-16

## Context And Evidence

Glippy's schema-version-3 native facts proved stable cross-load identities for
no-return, parameter lifecycle, and returned nil/error relationships. Those
facts were inferred only from selected and same-module source or from a fixed
standard-library terminal set. Application wrappers and external libraries
therefore remained unknown even when a project had an authoritative API
contract.

Clippy benefits from Rust compiler semantics and attributes. PHPStan supports
stub files and semantic extensions for application APIs. Go export data does
not encode ownership transfer, required result use, termination, blocking, or
conditional nilness. A dynamic Go plugin would execute repository-selected
code inside the analyzer, freeze immature internal APIs, and conflict with the
bounded untrusted-repository model.

Focused implementation evidence demonstrates strict multi-file parsing,
order-independent canonical identity, bounded declarations and indexes,
source-located failures, exact function and method resolution, signature
validation, configured CFG effects, external export-data resolution without
dependency source, persistent-cache invalidation, target-matrix reuse, and LSP
overlay reuse.

## Decision

Schema-version-1 configuration accepts an explicit, project-relative
`analysis.contract-files` list. Each selected file has its own strict
`version = 1` TOML schema. Declarations use exact `package/path.Function` or
`package/path.Type.Method` symbols and may describe no-return, must-use,
parameter completion or ownership effects, blocking calls, returned nil/error
relationships, and returned aliases.

Configuration loading captures and canonicalizes exact contract bytes. Typed
effect consumers resolve symbols against loaded `go/types` package graphs and
validate all referenced parameter and result indexes plus required type
relationships. Packages absent from a partial load remain deferred; symbols
missing from a present package fail. This keeps file and editor selections
usable without weakening validation of the graph that was actually loaded.

Configured records seed native effects before source inference and win for the
same exact parameter or result relationship. The schema advances to native
effects version 4, adding deterministic must-use, blocking, and alias records
to existing facts. The native cache component becomes `native-effects-v4`.
Returned-alias lifecycle admission later advances the active representation to
version 5 and `native-effects-v5`, separating possible terminal kinds from
effects independently guaranteed on every returning path. This preserves
simultaneous configured guarantees without treating alternative inferred paths
as if each effect occurred universally.

Cleanup-managed result inference later advances the active representation to
version 6 and `native-effects-v6`. That source-derived relationship is not a
configurable project contract, but it shares the stable identity and cache
boundary because native lifecycle consumers use it across package loads.

Receiver terminal-effect inference later advances the active representation to
version 7 and `native-effects-v7`. Receiver summaries are source-derived rather
than configured, but share the stable identity and invalidation boundary used
by parameter and cleanup-managed facts.

Unconditional result-state inference later advances the active representation
to version 8 and `native-effects-v8`. These summaries are also source-derived,
not configurable: every explicit normal return and every selected package
variant must agree on one nil-capable result state before an exact static caller
may consume it.

Exact delegated result-state inference later advances the active representation
to version 9 and `native-effects-v9`. Static same-type single results and
same-arity tuples may consume those source-derived summaries through a bounded,
recursion-rejecting selected-package traversal. Project contracts still cannot
declare these inferred result states.

Exact delegated return-relationship inference later advances the active
representation to version 10 and `native-effects-v10`. A same-arity exact
static tuple delegate may consume the source-derived nil/error relationship
through the same bounded recursion-rejecting traversal. Project contracts still
cannot declare these inferred relationships.

Exact delegated cleanup-managed-result inference later advances the active
representation to version 11 and `native-effects-v11`. Static same-type single
results and same-arity tuples may consume those source-derived summaries
through the same bounded recursion-rejecting traversal. Project contracts still
cannot declare cleanup-managed results.

Exact standard-library testing cleanup receivers later advance the active
representation to version 12 and `native-effects-v12`. Source-derived cleanup
facts accept only `*testing.T`, `testing.TB`, `*testing.B`, and `*testing.F`;
project contracts still cannot declare cleanup-managed results or authorize
lookalike cleanup registries.

Exact source-proven no-op Close facts later advance the active representation
to version 13 and `native-effects-v13`. The fact is inferred only for a
selected-module `Close() error` method whose complete body returns a receiver
field in one statement, and project contracts cannot declare it.

Contract files are data only. They do not execute code, provide replacement Go
types, load runtime plugins, or grant write authority. Syntax-only lint does
not load type state because contracts are configured. External contracts use
loaded export types and do not request source solely for resolution.

## Alternatives

- Infer third-party semantics from function names: rejected because names do
  not prove termination, ownership, blocking, or returned-state behavior.
- Load and analyze every dependency body: rejected because source may be
  unavailable and the measured memory cost contradicts the demand-tier model.
- Require every contract package in every partial load: rejected because one
  repository snapshot would break file and LSP analysis for unrelated
  packages.
- Merge duplicate declarations: rejected because declaration precedence would
  make policy order-sensitive and obscure conflicts.
- Use Go's runtime plugin ABI: rejected because it executes untrusted code,
  narrows platform support, and freezes an immature extension boundary.
- Embed contracts in source comments: rejected because formatter ownership,
  vendored dependencies, and project policy would become entangled.

## Consequences

Built-in control-flow and SSA analysis can understand project and dependency
APIs without a new language frontend or executable extension. The same
snapshot participates in CLI, LSP, target matrices, configuration
introspection, and cache identity. Contract authors receive strict schema and
signature failures, but remain responsible for the truth of runtime assertions.

Reading small bounded contract files becomes part of configuration loading,
including formatter commands that validate the complete selected policy.
Symbol resolution remains deferred until an effect-aware typed analysis is
already required. Projects with many unrelated contracts do not force those
packages into a partial load.

## Revisit Trigger

Revisit the schema when admitted rules require a semantic relationship that
cannot be expressed without ambiguity, or when real repository contracts show
that receiver effects or richer generic relationships are necessary. Revisit
strict package resolution only if whole-repository validation cannot surface
misspelled unused package paths credibly. Consider a statically compiled custom
analyzer boundary only after contracts and the existing `go/analysis` adapter
cannot satisfy a documented external use case.
