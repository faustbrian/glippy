# Release amd64 Evidence, 2026-08-12

## Contract

The four-target prototype release builder must produce Darwin and Linux
archives for amd64 and arm64 from one clean, exact revision. Independent
Darwin arm64 and Linux amd64 builds with identical inputs must produce the same
four archives, manifest, and checksum file byte-for-byte. Every archive must
retain executable mode, every checksum must validate, and each target binary
must execute with the linked version on its declared operating system and
architecture, either natively or through the recorded architecture emulator.

This evidence covers artifact construction, architecture execution, and
cross-operating-system reproduction on one physical Darwin host. It does not
prove reproduction on a separate physical host, public support, signing,
publication, installer behavior, Windows runtime support, network filesystem
semantics, or crash durability.

## Environments

The source was clean revision
`837b6fc33127ce775c85ce11b6e507929042cdc1`. Both builds used Go 1.26.5,
version `v0.0.0-amd64-rehearsal`, and the builder's pinned `GOAMD64=v1` and
`GOARM64=v8.0` settings.

- Darwin builder: macOS 27.0, Darwin 27.0.0 arm64 on APFS.
- Linux builder: Linux amd64 under Docker Desktop 29.6.2 architecture
  emulation on a Linux arm64 server with the overlayfs containerd snapshotter.
- Linux runtime: amd64 under the same architecture emulation and arm64 on the
  native Docker server architecture.
- Darwin runtime: arm64 natively and amd64 through Rosetta using an explicit
  `arch -x86_64` invocation.
- Linux toolchain image:
  `golang@sha256:7caba5286b4c3613a337b709c573047d8ae62ee76106647313b61e72b99f20af`.

The Linux build and both Linux executions ran with networking disabled. The
source and module cache were read-only mounts. Outer Go build caches, release
builder caches, output directories, and extraction directories were task-owned
and removed after evidence capture.

## Results

The independent Darwin and Linux builders produced the same six files
byte-for-byte:

| File | SHA-256 |
| --- | --- |
| `gox_v0.0.0-amd64-rehearsal_darwin_amd64.tar.gz` | `5034bfdb6316a9860d565d6a91f397238a1781fee2c32fb59d773d50081bc18e` |
| `gox_v0.0.0-amd64-rehearsal_darwin_arm64.tar.gz` | `6b5ed5aeeca943ae401bf596662d60e05c28341d1475cc7404a9e15eeef57b1a` |
| `gox_v0.0.0-amd64-rehearsal_linux_amd64.tar.gz` | `2dc01102ea08132e1375bb3e0955aa5fed1d090933253ef32497ff2a086b25b8` |
| `gox_v0.0.0-amd64-rehearsal_linux_arm64.tar.gz` | `91c2f4f691d7c8818dbe55a7d511ebb209d795a0d887847442aaa5906da852d4` |
| `gox_v0.0.0-amd64-rehearsal_manifest.json` | `923062d8e902e2f7fdc13bf4cbc3fe21a72618675977542c336e5f211152fe2e` |
| `gox_v0.0.0-amd64-rehearsal_checksums.txt` | `5df6caab577abdf5f2cf013f6f8990060cd36f8bb12a193b877f7ca11949f981` |

Both checksum files validated all four archives and the manifest. The manifest
reported schema 1, product `gox`, the exact revision, Go 1.26.5, and the sorted
Darwin/Linux amd64/arm64 target set. Every archive contained exactly one `gox`
entry with mode `0755` and normalized epoch metadata.

Each extracted binary printed `gox v0.0.0-amd64-rehearsal` in its declared
environment. File inspection independently identified the expected Mach-O
x86-64 and arm64 binaries and statically linked ELF x86-64 and aarch64
binaries.

This closes architecture execution and cross-operating-system byte
reproducibility on the recorded physical host for the admitted four-target
prototype set. Reproduction on a separate physical host and public platform
support remain open, as do the product name, signing, publication, Windows,
network filesystem, and crash-durability boundaries.
