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

The CLI cache boundary was revisited against Oxc commit `9d49d279` on
2026-08-12. The current `apps/oxlint` command surface contains no general
persistent result-cache option to copy. Gox therefore keeps this policy
Go-specific and limited to the package-loading and typed-analysis costs already
measured in this repository.

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

The maintainer-only prototype release builder admits Darwin and Linux on amd64
and arm64. It pins `GOAMD64=v1` and `GOARM64=v8.0`, uses the selected local Go
toolchain,
disables ambient workspace, environment-file, toolchain-download, and VCS
metadata, builds without cgo, trims paths, and links the exact canonical
version. Deterministic tar/gzip metadata, a versioned JSON manifest, and sorted
SHA-256 checksums make two identical-input builds byte-comparable. The builder
requires a repository with no tracked, untracked, or ignored working-tree
content whose `HEAD` equals the complete supplied revision. Local module
replacements must resolve within that same root. Builds use an immutable Git
archive of the supplied object. Git validation and export ignore ambient
repository-routing variables and user/system Git configuration. Builds use an
invocation-owned empty Go build cache, with external cache programs and FIPS
source substitution disabled. The build environment admits only required host
paths plus explicitly pinned Go settings, then binds the revision and exact Go
toolchain version into the output. A pinned private directory owns all artifact
writes and failure cleanup. Completed artifacts are atomically renamed to the
requested path with a platform no-replacement primitive only after all fallible
output closing and temporary-resource cleanup succeeds. The builder neither
signs nor publishes remotely and must be updated after the final product-name
decision. The 2026-08-12 release-platform rehearsal built the complete target
set independently on network-isolated Linux arm64 and Darwin arm64 with Go
1.26.5, executed each environment-native archive, and found all archives,
manifest, and checksums byte-identical across environments on one physical
Darwin host.
That rehearsal predates amd64 admission and proves the earlier arm64 target
set. A later 2026-08-12 rehearsal built the four-target set independently on
Darwin arm64 and emulated Linux amd64, found all six files byte-identical,
validated both checksum sets, and executed every target binary on its declared
operating system and architecture. Darwin amd64 used Rosetta, Linux amd64 used
Docker architecture emulation, and both arm64 executions were native to their
host architecture. This proves the prototype artifact boundary without
claiming native amd64 host evidence, reproduction on a separate physical host,
or public platform support.

The 2026-08-12 cache-platform rehearsal also ran the complete cache and CLI
package suites, including the race detector, on network-isolated Linux arm64
overlayfs. Focused cases proved resolved-root pinning, symlink containment,
reuse, pruning, invalid-root refusal, and cache-open failure reporting. This
adds Linux runtime evidence without extending those filesystem claims to
Windows or unrecorded storage drivers.

The Phase 4 cache foundation uses a versioned SHA-256 key over canonical,
length-prefixed fields. Every consumer must supply its result namespace, tool
version, build Go toolchain, selected source language version, configuration
digest, enabled rules and option digests, build tags, GOOS, GOARCH, cgo
selection, and formatter compatibility mode. Typed consumers must additionally
name and digest every applicable source, module, workspace, overlay, package
selection, result-changing environment input, dependency export, and fact.
Duplicate rule or component identities fail instead of being resolved by input
order.

Resolved typed rule options, including canonical metadata defaults, use the
`gox-rule-options-v1` canonical encoding. The cache consumer derives each digest
from the same immutable snapshot routed to the rule instead of accepting a
caller-supplied surrogate.

Persistent entries live under a caller-selected normalized absolute root and a
store schema directory. Each entry embeds its key, payload length, and payload
SHA-256 digest. Reads are bounded to 16 MiB, and corruption is a cache miss.
Writers create and sync a same-shard temporary file, then publish it with an
atomic create-if-absent hard link. Equal concurrent values converge; different
valid values for one key fail as nondeterministic instead of overwriting one
another. Rooted filesystem operations refuse symlink traversal outside the
cache. Explicit pruning removes canonical corruption and then the oldest
canonical entries until caller-supplied count and encoded-byte limits hold;
unknown and temporary files remain untouched. Concurrent eviction can cause a
miss but cannot supply invalid bytes. Cache data remains disposable and is never
the only source of results.

Fact-bearing typed analyzer runs and native types, CFG, and SSA runs may use a
caller-owned store. The consumers require `GOENV=off`, explicit platform and
cgo inputs, and canonical rule and configuration digests. They hash one
complete loaded-graph manifest once, then derive dependency-first
analyzer-package keys from that manifest and direct
dependency keys. Entries bind canonical diagnostics and every analyzer-step
fact snapshot; restore validates them transactionally against the new type
graph and exact captured source. Unsupported local object facts keep the
package and its dependents uncached. An owned 42-package workload proves zero
analyzer executions on warm independent loads. One separate native entry binds
the complete selected native tier set and canonical pre-suppression diagnostics.
The payload includes every selected rule even when it reports nothing, plus its
scheduling metadata, execution shape, fix contract, and resolved-options
digest. Restore compares that complete snapshot and revalidates exact physical
owner package, source digest, ranges, fixes, and ordering before it may bypass
native callbacks and CFG or SSA construction. Error-bearing loads remain
uncached. Package loading still dominates and host variance prevents a latency
threshold.

The first CLI policy is explicit and opt-in through versioned configuration.
One typed `lint` or combined `check` invocation opens one store under
`GOX_CACHE_DIR` or the platform user-cache directory, supplies the resolved
result-affecting canonical configuration and fixed prototype
language/formatter identities, and closes the store before reporting.
Versioned builds use their product version as tool identity; `devel` builds use
the SHA-256 digest of the running executable rather than sharing the generic
display version. Cache enablement and retention limits do not invalidate
compatible results.
Cache-enabled loading makes GOOS, GOARCH, and CGO explicit and forces
`GOENV=off`; it does not speculate about ambient Go environment files. After
every non-canceled run, the CLI removes canonical corruption and prunes oldest
entries to the configured count and encoded-byte limits. That pass also removes
only canonical publication temporaries strictly older than 24 hours; newer and
unrecognized files remain untouched. A writer suspended beyond the cutoff can
fail visibly but cannot publish partial bytes. Formatting, syntax-only
analysis, and fixing remain cache-independent. Cache failures are visible and
cached state remains disposable and non-authoritative. The store resolves the
prospective root once, validates that immutable target against the project
before creation, and walks its resolved components through pinned rooted
handles with identity checks. Changing the original symlink after validation
cannot redirect the opened store into the project. Broader platform evidence
and product-wide warm-run performance claims remain deferred.

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
- Enable persistent caching implicitly: the platform and lifecycle boundary is
  still narrower than the ordinary uncached CLI contract, so projects must opt
  in while evidence grows.

## Consequences

Early binaries are development artifacts rather than supported releases.
The owned workload benchmark supports a bounded functional and directional
performance claim for fact-bearing analyzer entries. A stable latency or
allocation budget still requires an isolated runner and larger representative
module/workspace workloads. Cache-enabled invocations fail visibly on an
unsupported filesystem without making cached state authoritative. Release
evidence must bind exact toolchain, key schema, store schema, entry schema, and
machine-output schema versions.

## Revisit Trigger

Before Phase 1 claims cross-version syntax support; before CLI caching becomes
the default; when formatter caching, cross-machine sharing, or a filesystem
without reliable hard links is admitted; and before the first public release.
