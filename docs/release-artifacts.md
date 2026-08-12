# Prototype Release Artifacts

The maintainer-only release builder produces deterministic artifacts for the
admitted prototype targets. It is not a public Gox command and does not widen
the product CLI.

Run it from a clean source revision with a task-owned disposable `GOCACHE`:

```text
go run ./internal/releasecmd \
  --version v0.1.0 \
  --revision <complete Git object ID> \
  --output <new output directory>
```

The complete revision must equal repository `HEAD`, and tracked, untracked, or
ignored working-tree content is rejected before output creation. Local module
replacements must resolve inside that same source root. Builds run from an
immutable `git archive` export of the supplied object rather than the mutable
working tree. Repository validation and export ignore ambient Git repository
routing and user/system Git configuration. Artifacts are assembled through a
pinned handle in a private,
same-parent directory, then published atomically without replacing any existing
output path. Failure cleanup operates through the pinned private directory and
refuses a replacement path. The pinned output is closed and temporary source
and build-cache directories are removed before publication, so a returned
cleanup error cannot leave a published release behind.
The builder uses the selected local Go toolchain with
`GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`, `CGO_ENABLED=0`, read-only module
mode, path trimming, disabled implicit VCS metadata, and explicit linked version
metadata. Every invocation creates and removes its own empty build cache,
disables external cache programs and FIPS source substitution, and admits only
the minimal host environment needed to locate the toolchain, module cache, and
temporary directory. It does not sign, publish, upload, tag, or modify a
repository.

The current prototype target set is:

- `darwin/amd64`;
- `darwin/arm64`;
- `linux/amd64`;
- `linux/arm64`.

Windows is excluded because its write and fix behavior has only cross-compile
evidence. Other operating systems require their own runtime and filesystem
evidence before admission. Darwin amd64 has Rosetta execution evidence and
Linux amd64 has Docker architecture-emulation evidence; neither substitutes
for a native amd64 host claim.

For version `v0.1.0`, the builder emits:

```text
gox_v0.1.0_darwin_amd64.tar.gz
gox_v0.1.0_darwin_arm64.tar.gz
gox_v0.1.0_linux_amd64.tar.gz
gox_v0.1.0_linux_arm64.tar.gz
gox_v0.1.0_manifest.json
gox_v0.1.0_checksums.txt
```

Each archive contains one executable named `gox`. Archive ownership, modes,
timestamps, ordering, and compression metadata are normalized. The versioned
JSON manifest binds the product name, release version, verified complete source
revision, exact Go toolchain version, target, size, and SHA-256 digest of every
archive. The sorted checksum file covers every archive and the manifest.

Two builds are reproducible only when their source tree, explicit revision,
version, Go toolchain, module inputs, and target set are identical. The tests
build the complete target set twice, compare every emitted byte, verify the
checksums, extract the current-host binary, and execute `gox version` to prove
the linked version. The independent
[release platform rehearsal](research/release-platform-evidence-2026-08-12.md)
also produced the complete target set on network-isolated Linux arm64, executed
its native archive, and matched all four output files byte-for-byte with an
independent Darwin arm64 build of the same revision and version. That rehearsal
predates amd64 admission and therefore covers the earlier two-target set only.
The later
[amd64 rehearsal](research/release-amd64-evidence-2026-08-12.md) built the
four-target set independently on Darwin arm64 and emulated Linux amd64, matched
all six files byte-for-byte, and executed every target binary on its declared
operating system and architecture. The amd64 executions used Rosetta and Docker
architecture emulation, while both arm64 executions were native to their host
architecture. Both build environments ran on one physical Darwin host, so
separate-host reproduction remains open.

`gox` remains a working name. A final rename requires updating the centralized
release product name and linker path before any public artifact is produced.
Signing, provenance attestations, checksummed installer metadata, and public
publication remain Phase 5 work.
