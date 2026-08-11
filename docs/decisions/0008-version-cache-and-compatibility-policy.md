# ADR 0008: Version, Cache, And Compatibility Policy

- Status: accepted for prototype; release details deferred
- Date: 2026-08-09

## Context And Evidence

The prototype is built with Go 1.26.5 and x/tools v0.48.0. Standard parser and
printer behavior can drift with the build toolchain. Typed analysis also
depends on module/workspace state, build selection, environment, dependency
export data, and overlays. No persistent-cache evidence exists yet.

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

No persistent result cache is implemented before Phase 4. Any cache key must
include tool version, build Go toolchain, selected source language version,
source digest, configuration digest, enabled rules/options, build tags, GOOS,
GOARCH, cgo selection, module/workspace state, overlays, dependency export data
or facts, and formatter compatibility mode as applicable. Corruption degrades
to recomputation.

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
- Treat formatter output or new default findings as invisible compatibility
  changes: both create adoption churn.

## Consequences

Early binaries are development artifacts rather than supported releases.
Performance work cannot claim warm persistent-cache behavior yet. Release
evidence must bind exact toolchain and schema versions.

## Revisit Trigger

Before Phase 1 claims cross-version syntax support, before Phase 4 persistent
caching, and before the first public release.
