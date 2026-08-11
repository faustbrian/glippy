# ADR 0008: Version, Cache, And Compatibility Policy

- Status: accepted for prototype; release details deferred
- Date: 2026-08-09

## Context And Evidence

The prototype is built with Go 1.26.5 and x/tools v0.48.0. Standard parser and
printer behavior can drift with the build toolchain. Typed analysis also
depends on module/workspace state, build selection, environment, dependency
export data, and overlays. The implemented cache foundation proves canonical
identity, bounded verified storage, corruption misses, conflict-safe
publication, and an opt-in fact-bearing analyzer consumer.

At Oxc commit `fed2b90`, Oxfmt 0.63.0 and Oxlint 1.78.0 both derive their
version option from the build-owned Cargo package version. Go versioned installs
carry an equivalent main-module version in build information, while ordinary
local builds identify themselves as development builds.

## Decision

Phase 0 development targets Go 1.26 source and uses module language version
1.26. A public release must separately state minimum runtime toolchain,
official build toolchain, supported source versions, and newer-syntax behavior.

`gox version` prints one deterministic product version. Explicit link-time
release metadata takes precedence over the Go main-module version; a binary
with neither reports `devel`. Official release builders set
`github.com/faustbrian/gox/internal/version.linked` through Go's `-X` linker
flag. Version inspection performs no source, configuration, package, or network
work.

The Phase 4 cache foundation uses a versioned SHA-256 key over canonical,
length-prefixed fields. Every consumer must supply its result namespace, tool
version, build Go toolchain, selected source language version, configuration
digest, enabled rules and option digests, build tags, GOOS, GOARCH, cgo
selection, and formatter compatibility mode. Typed consumers must additionally
name and digest every applicable source, module, workspace, overlay, package
selection, result-changing environment input, dependency export, and fact.
Duplicate rule or component identities fail instead of being resolved by input
order.

Persistent entries live under a caller-selected normalized absolute root and a
store schema directory. Each entry embeds its key, payload length, and payload
SHA-256 digest. Reads are bounded to 16 MiB, and corruption is a cache miss.
Writers create and sync a same-shard temporary file, then publish it with an
atomic create-if-absent hard link. Equal concurrent values converge; different
valid values for one key fail as nondeterministic instead of overwriting one
another. Rooted filesystem operations refuse symlink traversal outside the
cache. Cache data remains disposable and is never the only source of results.

Fact-bearing typed analyzer runs may use a caller-owned store. The consumer
requires `GOENV=off`, explicit platform and cgo inputs, and canonical rule and
configuration digests. It hashes one complete loaded-graph manifest once, then
derives dependency-first analyzer-package keys from that manifest and direct
dependency keys. Entries bind canonical diagnostics and every analyzer-step
fact snapshot; restore validates them transactionally against the new type
graph and exact captured source. Unsupported local object facts keep the
package and its dependents uncached. CLI ownership, eviction, platform evidence
for hard-link publication, native-rule caching, and warm-run performance claims
remain deferred.

Formatter output changes are user-visible compatibility changes. They require
construct-specific before/after documentation and updated corpus evidence.
Rule IDs and machine schema versions become stable at the first public release;
default preset changes and behavior changes require release notes and migration
metadata. Formatter, lint, and fix machine output starts at schema version 1
before external integrations are advertised.

## Alternatives Rejected

- Promise broad source-version support from the build parser alone: parsing and
  semantic version enforcement are different boundaries.
- Persistent caching in Phase 0: no stable semantic key or corruption suite.
- Rename-over-existing publication: concurrent processes could silently replace
  different values for one supposedly deterministic key.
- Filename-only integrity: a misplaced or partially written entry could be
  accepted without binding its embedded key and complete payload.
- Treat formatter output or new default findings as invisible compatibility
  changes: both create adoption churn.

## Consequences

Early binaries are development artifacts rather than supported releases.
Warm persistent-cache performance cannot be claimed until representative
benchmarks exist, even though the fact-bearing adapter now proves functional
hits and invalidation. Unsupported filesystems may disable cache writes without
changing computed results. Release evidence must bind exact toolchain, key
schema, store schema, entry schema, and machine-output schema versions.

## Revisit Trigger

Before Phase 1 claims cross-version syntax support; when the CLI, formatter, or
native analysis tiers adopt caching; when eviction, cross-machine sharing, or a
filesystem without reliable hard links is admitted; and before the first public
release.
