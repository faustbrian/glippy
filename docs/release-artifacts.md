# Prototype Release Artifacts

The maintainer-only release builder produces deterministic artifacts for the
admitted prototype targets. It is not a public Glippy command and does not widen
the product CLI.

Run it from a clean source revision with a task-owned disposable `GOCACHE`:

```text
go run ./internal/releasecmd \
  --version v0.2.0 \
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
disables external cache programs and FIPS source substitution, preserves an
explicit caller-owned `GOMODCACHE`, and admits only the minimal host environment
needed to locate the toolchain, module cache, and temporary directory. It does
not sign, publish, upload, tag, or modify a repository.

The current prototype target set is:

- `darwin/amd64`;
- `darwin/arm64`;
- `linux/amd64`;
- `linux/arm64`.

Windows and operating systems other than macOS and Linux are intentionally
unsupported. Darwin amd64 has Rosetta execution evidence and
Linux amd64 has Docker architecture-emulation evidence; neither substitutes
for a native amd64 host claim.

For version `v0.2.0`, the builder emits:

```text
glippy_v0.2.0_darwin_amd64.tar.gz
glippy_v0.2.0_darwin_arm64.tar.gz
glippy_v0.2.0_linux_amd64.tar.gz
glippy_v0.2.0_linux_arm64.tar.gz
glippy_v0.2.0_manifest.json
glippy_v0.2.0_checksums.txt
```

Each archive contains the executable `glippy`, the exact tracked 0BSD `LICENSE`,
and `THIRD_PARTY_LICENSES.txt`. The builder rejects a source revision without
either license artifact. Archive ownership, modes, timestamps, ordering, and
compression metadata are normalized, and the reproducibility tests verify
every entry and its content. The versioned JSON manifest binds the product
name, release version, verified complete source revision, exact Go toolchain
version, target, size, and SHA-256 digest of every archive. The sorted checksum
file covers every archive and the manifest.

The maintainer selected the BSD Zero Clause License (`0BSD`) for Glippy. The
third-party notice reproduces the MIT terms for go-toml and the BSD terms and
patent grant for Go and the linked `golang.org/x` modules.

Two builds are reproducible only when their source tree, explicit revision,
version, Go toolchain, module inputs, and target set are identical. The tests
build the complete target set twice, compare every emitted byte, verify the
checksums, extract the current-host binary, and execute `glippy version` to prove
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
that rehearsal alone does not establish separate-host reproduction.

The manual `Release budget evidence` workflow closes that boundary for a
release candidate. Its native Darwin and Linux amd64 and arm64 runners each
build the complete six-file target set and execute the archive matching their
own operating system and architecture. A separate Linux job downloads all four
retained candidates, verifies their manifest identity and checksums, and
requires every file to be byte-identical across runners. Any missing runner,
different archive, manifest, or checksum, or non-native version execution fails
the workflow.

Candidate run
[`31697171821`](https://github.com/faustbrian/glippy/actions/runs/31697171821)
passed this complete matrix and comparison for revision `06cce4a`. The raw
budgets and artifact digests are recorded in the
[release-candidate evidence](research/release-candidate-evidence-2026-08-13.md).

GitHub Releases is the selected publication channel. A push of a canonical
semantic-version tag invokes the repository's `Publish release` workflow. It
checks out the exact tag, builds the complete deterministic target set with Go
1.26.5, submits every archive, manifest, and checksum file to GitHub artifact
attestations, and creates one GitHub Release containing those files. Prerelease
semantic versions create GitHub prereleases. The workflow uses pinned action
commits, does not persist checkout credentials into the build tree, and retains
an existing candidate for 14 days when attestation or publication fails.

GitHub artifact attestations are the accepted signing and provenance mechanism.
The workflow receives a short-lived GitHub OIDC identity, produces signed SLSA
build provenance binding every released file's SHA-256 digest to this repository
and workflow, and stores it in GitHub's attestation service. No long-lived
private signing key exists. Consumers can verify a downloaded file with:

```text
gh attestation verify <artifact> --repo faustbrian/glippy
```

The builder's checksum file remains the portable offline integrity surface;
the signed attestation proves repository and workflow provenance when GitHub is
available. GitHub Release archives and version-pinned `go install` are the
intended v0.2 installation channels; package-manager metadata remains deferred.
A signed Git tag is not part of this artifact-provenance claim.

The historical Gox `v0.1.0` release was built from reviewed commit `c0435d6` by
successful workflow run
[`31699926922`](https://github.com/faustbrian/gox/actions/runs/31699926922).
The workflow published all six checksummed assets and GitHub attestations after
the maintainer accepted the final naming risk, reviewed the candidate, and
authorized the tag. Exact identities and post-publication verification are in
the [v0.1.0 release evidence](research/release-v0.1.0-evidence-2026-08-13.md).
Those assets remain named `gox_*` and contain the `gox` binary; this source
migration does not rewrite them. Ordinary pushes and manual workflow
dispatches still cannot invoke publication.

Published releases use the support and vulnerability boundaries in the
[product support policy](support-policy.md) and the repository
[security policy](../SECURITY.md). Untagged artifacts remain unsupported even
when they passed a release rehearsal.
