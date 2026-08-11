# Persistent Cache Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Identity

A cache key MUST identify one result namespace and MUST include the Gox tool
version, build Go toolchain, selected source language version, canonical
configuration digest, build tags, GOOS, GOARCH, cgo selection, and formatter
compatibility mode. Every enabled lint rule MUST contribute its rule ID,
severity, and canonical options digest.

A consumer MUST add one named, digested component for every applicable source,
module file, workspace file, overlay, package/build selection, result-changing
environment input, dependency export, and imported fact. Component identity
MUST distinguish different paths, packages, environment fields, and fact
producers. A consumer MUST NOT use the cache until it can enumerate every input
capable of changing its result.

The key schema is `gox-cache-key-v1`. Fields are length-prefixed before hashing
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

## Current Boundary

Fact-bearing typed `go/analysis` execution is the first integrated consumer.
It is opt-in through a caller-owned store and explicit tool, build-toolchain,
source-language, configuration, rule-option, cgo, and formatter identities.
Cache-enabled package loading MUST also receive explicit GOOS and GOARCH,
`GOENV=off`, and an exact `CGO_ENABLED` value. This prevents an unrecorded Go
environment file or platform default from changing a reused result.

The consumer builds one canonical run manifest over the reachable loaded graph,
then derives one key per native rule and analyzer package. Package keys include
the run manifest and each direct dependency package key, so imported package and
object facts invalidate dependency-first. Location-only build-cache paths,
unrelated process environment, and dependency export-file locations are not
semantic identity; export bytes remain digested under package identity.

This proves cold population, independent-load hits, source invalidation,
corruption-as-recomputation, and transactional restore for the implemented
fact-bearing adapter boundary. The owned workload benchmark also proves zero
analyzer executions on warm independent loads; its timing remains directional
because package loading dominates and the reference host is not isolated. This
does not enable caching in the CLI, cache native types/CFG/SSA rules, establish
eviction, or support a product-wide warm-performance claim.

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
ownership, severity, ranges, related locations, fixes, and canonical order.
Fact snapshots and diagnostics MUST validate into an isolated candidate before
any live fact state changes. A malformed or stale entry is a miss and normal
analysis remains authoritative. If any package-owned object lacks stable
identity, that package and every dependent package MUST run uncached rather
than publish a partial fact graph.
