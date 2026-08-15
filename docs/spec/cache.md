# Persistent Cache Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Identity

A cache key MUST identify one result namespace and MUST include the Glippy tool
version, build Go toolchain, selected source language version, canonical
configuration digest, build tags, GOOS, GOARCH, cgo selection, and formatter
compatibility mode. Every enabled lint rule MUST contribute its rule ID,
severity, and canonical options digest.

The options digest MUST be derived from the resolved typed option snapshot,
including canonical metadata defaults, ordered by option name and encoded with
the `glippy-rule-options-v1` schema. A cache caller MUST NOT substitute an
unrelated digest for the values delivered to the rule callback.

A consumer MUST add one named, digested component for every applicable source,
module file, workspace file, overlay, package/build selection, result-changing
environment input, dependency export, and imported fact. Component identity
MUST distinguish different paths, packages, environment fields, and fact
producers. A consumer MUST NOT use the cache until it can enumerate every input
capable of changing its result.

The key schema is `glippy-cache-key-v1`. Fields are length-prefixed before hashing
with SHA-256. Unordered build tags, rules, and components are canonicalized
without mutating caller input. Duplicate rule IDs and duplicate component
kind/identity pairs MUST fail. Missing required scalar values or digests MUST
fail rather than collapsing to an ambiguous key.

## Storage

The store MUST use a caller-selected normalized absolute root. Every operation
MUST remain beneath that root even when an untrusted symlink appears in a cache
shard. Entries are partitioned by store schema and key prefix; those paths are
an internal format and MUST NOT be treated as a compatibility API.

One entry MUST embed its complete key, exact payload length, and SHA-256 payload
digest. A read MUST verify all three before returning independently owned
bytes. Missing, truncated, oversized, misplaced, or digest-invalid entries MUST
be cache misses so the caller can recompute. Filesystem access failures MAY be
reported separately; they MUST NOT become cached success.

One decoded entry MUST NOT exceed 16 MiB. Writers MUST create a mode `0600`
temporary file in the final shard, write and sync the complete entry, and
publish it only through an atomic create-if-absent operation. Temporary entries
MUST be removed after success or failure. Concurrent writers of equal values
MAY converge successfully. Different valid values for one key MUST produce a
conflict and MUST NOT overwrite the existing value.

Corrupt entries MAY be removed when a recomputed value is ready to publish.
Cache content MUST remain safe to delete and MUST NOT be the only source of
diagnostics, facts, formatted output, or configuration. A cache miss or corrupt
entry MUST therefore degrade to normal computation.

Callers MAY explicitly prune canonical entries by a positive maximum entry
count, encoded-byte count, or both. Pruning MUST remove canonical corrupt
entries first, then evict valid entries by oldest publication modification time
with canonical key order as the tie-breaker until every supplied limit holds.
Zero leaves one dimension unlimited; negative limits and a request with no
positive limit MUST fail. Counts and byte totals cover only canonical entry
files, including their storage headers. A caller MAY also supply an explicit
stale-publication cutoff. Pruning then removes only regular files whose names
match the complete Glippy key-and-random-suffix temporary grammar and whose
modification times are strictly older than that cutoff. Newer temporaries,
malformed names, unknown files, and directories MUST remain untouched. A
writer suspended beyond the cutoff can fail visibly if its temporary is
removed; it cannot publish partial or incorrect bytes. Concurrent pruning MAY
turn a hit into a miss but MUST NOT produce invalid bytes or make cached state
authoritative.

## Current Boundary

Fact-bearing typed `go/analysis` execution is the first integrated consumer.
It is opt-in through a caller-owned store and explicit tool, build-toolchain,
source-language, configuration, rule-option, cgo, and formatter identities.
Cache-enabled package loading MUST also receive explicit GOOS and GOARCH,
`GOENV=off`, and an exact `CGO_ENABLED` value. This prevents an unrecorded Go
environment file or platform default from changing a reused result.

The fact-bearing adapter consumer builds one canonical run manifest over the
reachable loaded graph, then derives one key per adapted native rule and
analyzer package. Package keys include the run manifest and each direct
dependency package key, so imported package and object facts invalidate
dependency-first. Location-only build-cache paths, unrelated process
environment, and dependency export-file locations are not semantic identity;
export bytes remain digested under package identity.

Native types, control-flow, and SSA execution MAY use the same explicit cache
identity. One native entry covers the complete selected native tier set for one
error-free loaded graph and stores canonical diagnostics before suppression.
The entry MUST bind the complete ordered rule set even when a rule reports no
diagnostics, including severity, requirement, execution shape, node interests,
generated and type-error policies, minimum Go version, declared fix identity,
and resolved options. Restore MUST compare that snapshot to the current
registry and revalidate each diagnostic's exact source digest, physical owner
package, ranges, offered and withheld fixes, and canonical order before
returning any result. A
missing, corrupt, malformed, stale, or unowned entry MUST fall back to complete
native execution. A valid warm entry MAY bypass native callbacks and CFG or SSA
construction because the key and payload bind every selected rule and
loaded-graph input.

This proves cold population, independent-load hits, source invalidation,
corruption-as-recomputation, and transactional restore for the implemented
fact-bearing adapter and native-tier boundaries. Owned analyzer and native-tier
workload benchmarks prove zero callbacks on warm independent loads; their
timing remains directional because package loading dominates and the reference
host is not isolated.

The CLI MAY enable this cache only through an explicit project configuration.
An enabled invocation MUST own one store for the complete typed `lint` or
`check` run, place it under `GLIPPY_CACHE_DIR` or the platform user-cache
directory, pass the resolved canonical configuration digest, and close the
store before reporting. A versioned build MUST use its product version as tool
identity; a `devel` build MUST bind the SHA-256 digest of its executable so two
different local binaries cannot share results under one generic version. It
MUST set explicit GOOS, GOARCH, and CGO selection, force `GOENV=off`, and bind
those values into the cache identity. It MUST prune after every non-canceled
run to the configured positive entry or encoded-byte limits. Syntax-only
linting, formatting, and fixing MUST NOT open this store.
Cache writes remain outside the selected source tree and MUST NOT be required
for correctness. The configuration digest includes result-affecting formatter,
lint, and suppression values; cache enablement and retention limits are
lifecycle policy and MUST NOT invalidate otherwise compatible results. The
store resolves the prospective root once, validates that immutable target
against the selected project before creating directories, and opens every
resolved component through pinned rooted handles without following a mutable
symlink. A later change to the user-supplied symlink therefore cannot redirect
the validated store into the project. The CLI removes canonical publication
temporaries strictly older than 24 hours during the same non-canceled pruning
pass; newer or unrecognized files remain untouched. Platform evidence for
hard-link publication beyond the recorded Darwin/APFS and Linux/overlayfs
filesystems and a product-wide warm-performance claim remain open. The
[Linux cache-platform rehearsal](../research/cache-platform-evidence-2026-08-12.md)
exercises root pinning, symlink containment, store lifecycle, and the CLI cache
boundary on the recorded Linux pair.

Persistent object identity is the owning package path plus the canonical
x/tools `objectpath`. It is proven across independent type checks for package
objects, named types, methods, fields, type parameters, parameters, and results.
Objects without a stable API path fail closed. A persisted identity MUST resolve
against a newly loaded package with the same package path; process-local
`types.Object` pointers MUST NOT be serialized as cache identity.

## Analysis Fact Snapshots

One fact snapshot owns one analyzer-package pair. It embeds a schema version,
analyzer name, package path, package-owned facts, canonical object identities,
stable fact type package/name pairs, and deterministic Gob values. Imported
facts remain separate dependency inputs rather than being duplicated into the
package snapshot.

Encoding MUST be byte-deterministic and bounded by the cache entry limit.
Decoding MUST require exact canonical JSON, declared fact types, canonical Gob
values, sorted unique records, and resolvable package-owned object paths before
merging any value. A different existing value MUST be a conflict. An object
without a stable path MUST make the snapshot uncacheable; it MUST NOT disappear
from a partial snapshot that could change warm-run analyzer behavior.

One analyzer-package cache entry MUST bind its schema version, native rule ID,
opaque package ID, package path, canonical diagnostics, and one fact snapshot
for every prerequisite step. Diagnostic restore MUST revalidate source digest,
ownership, severity, ranges, related locations, offered and withheld fixes,
and canonical order.
Fact snapshots and diagnostics MUST validate into an isolated candidate before
any live fact state changes. A malformed or stale entry is a miss and normal
analysis remains authoritative. If any package-owned object lacks stable
identity, that package and every dependent package MUST run uncached rather
than publish a partial fact graph.
