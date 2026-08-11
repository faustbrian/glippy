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

The key and rooted store are implemented, but no formatter or analyzer result
uses them yet. Gox MUST NOT claim warm persistent-cache performance or completed
invalidation until an integrated consumer supplies the complete identity above
and passes hit, miss, corruption, and invalidation tests.

Persistent object identity is the owning package path plus the canonical
x/tools `objectpath`. It is proven across independent type checks for package
objects, named types, methods, fields, type parameters, parameters, and results.
Objects without a stable API path fail closed. A persisted identity MUST resolve
against a newly loaded package with the same package path; process-local
`types.Object` pointers MUST NOT be serialized as cache identity. Fact snapshot
serialization and cache integration remain deferred.
