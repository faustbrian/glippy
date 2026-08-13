# Gox v0.1.0 Release Evidence, 2026-08-13

## Published Identity

- Release: [`v0.1.0`](https://github.com/faustbrian/gox/releases/tag/v0.1.0)
- Source commit: `c0435d6fd70918bcdc0b1acbc9c107f9af1424fa`
- Annotated tag object: `12211ffae74535a2733672f70bf32b35c8fd1850`
- Go toolchain: `go1.26.5`
- Publication workflow:
  [`31699926922`](https://github.com/faustbrian/gox/actions/runs/31699926922)

The maintainer accepted the final documented naming and trademark-collision
risk, personally reviewed the exact source commit, and explicitly authorized
the tag. The GitHub Release is neither a draft nor a prerelease.

The tagged commit differs from technical candidate `06cce4a` only by the five
release-evidence Markdown files in commit `c0435d6`. CI run
[`31699074682`](https://github.com/faustbrian/gox/actions/runs/31699074682)
passed the full source-and-behavior gate on the exact tagged commit. The
formatter corpus, fuzz, and native release-budget evidence therefore remain
applicable without a production, test, configuration, fixture, or workflow
change after their recorded runs.

## Published Assets

| Asset | SHA-256 |
| --- | --- |
| `gox_v0.1.0_darwin_amd64.tar.gz` | `973148744996285578caee760984335852ef05a43deb4b158ff83ddc869b5b56` |
| `gox_v0.1.0_darwin_arm64.tar.gz` | `ccaecd3f41b59a58c6c0e401ebed1238f42de33628dc2700df557c75529eb4b2` |
| `gox_v0.1.0_linux_amd64.tar.gz` | `e9e7b9d756bde5112c1f3aa3a4ccadb435a9c2e7c4c156153583ec57f089ba2d` |
| `gox_v0.1.0_linux_arm64.tar.gz` | `21a4feeb16307a9e0001c6c5a186ae585588863005721453e05c052d7e857038` |
| `gox_v0.1.0_manifest.json` | `cd8461d24f17890f48dc9d5aadecadbdb6323fad2728123aa33034f5cd2a59a4` |
| `gox_v0.1.0_checksums.txt` | `347b4c7b76b3eecb3a777c317050779f5840af07445e43b17098e8ac92c0268c` |

## Post-Publication Verification

The published checksum file verified all four archives and the manifest. The
schema-version-1 manifest binds product `gox`, version `v0.1.0`, Go 1.26.5, all
four supported target pairs, and the exact source commit above. Every archive
contains exactly, in order:

```text
gox
LICENSE
THIRD_PARTY_LICENSES.txt
```

The Darwin arm64 archive executed natively and reported `gox v0.1.0`; its
license files matched the source commit byte-for-byte. `gh attestation verify`
succeeded for every archive, the manifest, and the checksum file against
`faustbrian/gox`. The publication workflow itself passed the build, provenance,
release, retention, and cleanup steps.

This is the immutable release record. Later repository changes must not rewrite
these v0.1.0 identities or digests.
