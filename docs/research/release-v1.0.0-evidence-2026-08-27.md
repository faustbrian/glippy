# Glippy v1.0.0 Release Evidence, 2026-08-27

## Published Identity

- Release: [`v1.0.0`](https://github.com/faustbrian/glippy/releases/tag/v1.0.0)
- Source commit: `a2e526ca1c3c59129ec6176e0fb708d2d70aa772`
- Source-bearing candidate parent: `18004f2eba702471e03c9ab5656c905cf470e946`
- Annotated tag object: `5c9daf61b2de7d5bb2cf694ce7e8898ad0ad0987`
- Go toolchain: `go1.27.0`
- Publication workflow:
  [`33060261742`](https://github.com/faustbrian/glippy/actions/runs/33060261742)

The maintainer approved the release in advance after the final acceptance
gates. The annotated tag resolves to the exact accepted documentation closure.
CI run
[`33058637089`](https://github.com/faustbrian/glippy/actions/runs/33058637089)
passed at that revision, and publication run `33060261742` completed from the
same source identity. The GitHub Release was published on 2026-08-27 and is
neither a draft nor a prerelease. Its body is byte-identical to the tracked
`docs/releases/v1.0.0.md` source.

## Published Assets

| Asset | SHA-256 |
| --- | --- |
| `glippy_v1.0.0_checksums.txt` | `921b6d1309b1f113f4eb8d36b33fc0cba2a113dd88360e8c3e5e2abc5fc417ad` |
| `glippy_v1.0.0_darwin_amd64.tar.gz` | `9f74246caeccdaed851438026835d425beb57ee7ec5f405f75b7fd6c093272e6` |
| `glippy_v1.0.0_darwin_arm64.tar.gz` | `d6429233c29a8b71df2146fbc8d821add32dea196d1a919f466fd9aeba32f45c` |
| `glippy_v1.0.0_linux_amd64.tar.gz` | `27085f27cff5ece626d02f8d0dd5fb25533ca8f00cf8598f993cb47e205b48e3` |
| `glippy_v1.0.0_linux_arm64.tar.gz` | `32e7bfd78c2aa9b653de8e838ea64561d3cb14644e00ed42edc47df1bba95d0b` |
| `glippy_v1.0.0_manifest.json` | `12f73f4326e2afb4aadcf05a4ef4629467fbec41b532aea3920a92060f17b013` |

## Post-Publication Verification

The published checksum file verified all four archives and the manifest. The
schema-version-1 manifest binds product `glippy`, version `v1.0.0`, Go 1.27.0,
all four supported macOS/Linux amd64/arm64 target pairs, and the exact source
commit above. Every archive contains exactly:

```text
glippy
LICENSE
THIRD_PARTY_LICENSES.txt
```

The Darwin arm64 archive executed natively and reported `glippy v1.0.0`.
Go binary metadata reported Go 1.27.0, Darwin arm64, and `CGO_ENABLED=0`.
Every archive, the manifest, and the checksum file passed offline
`gh attestation verify` against repository `faustbrian/glippy`, source digest
`a2e526c`, source ref `refs/tags/v1.0.0`, and signer workflow
`.github/workflows/release.yml` using its published Sigstore bundle.

Version-pinned installation was independently verified with task-owned module,
build, binary, and temporary directories:

```text
go install github.com/faustbrian/glippy/cmd/glippy@v1.0.0
glippy version
```

The installed binary reported `glippy v1.0.0`. Its module metadata binds
`github.com/faustbrian/glippy` at `v1.0.0` with module checksum
`h1:fcIYI9D3WwEUFxWHq3R6FfbwVCVOIuMt4sUw76W9s4g=`.

## Stable-v1 Decision

The formatter, linter, safe-fix, CLI, configuration, machine-schema, corpus,
fuzz, supported-platform, performance, reproducibility, publication, and
installation acceptance gates are satisfied. The stable-v1 roadmap reaches
100%. No additional 17-repository corpus run is required unless a later
result-affecting change invalidates the accepted candidate.

Windows and other operating systems remain unsupported. Network, distributed,
and userspace filesystems and forced-power-loss durability remain outside the
documented v1 write/fix guarantees. These are explicit product boundaries, not
unresolved v1 gates.
