# Release Platform Evidence, 2026-08-12

This record covers the two-target arm64 set admitted at revision `c0a15b5`.
The later amd64 admission does not broaden the evidence captured here.

## Contract

The prototype release builder must produce the complete admitted target set
from one clean, exact source revision without network access. The manifest and
checksum file must bind every archive, each host-native binary must execute
with the linked version, and identical inputs on Darwin arm64 and Linux arm64
must produce byte-identical release files.

This evidence covers artifact construction and execution. It does not prove
signing, publication, installer behavior, Windows runtime support, network
filesystem semantics, or crash durability.

## Environments

The source was clean revision
`c0a15b513c9889ae627244a4c34ea3d4a6f54842`. Both builds used Go 1.26.5 and
version `v0.0.0-linux-rehearsal`.

- Darwin host: macOS 27.0, Darwin 27.0.0 arm64.
- Linux host: Linux 6.12.76-linuxkit arm64 on Docker Desktop 29.6.2 with the
  overlayfs containerd snapshotter.
- Linux toolchain image:
  `golang@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651`.

The Linux source mount and module cache were read-only. Its source compilation,
cross-builds, checksum validation, and native-binary execution ran with Docker
networking disabled. The release builder retained its own disposable build
cache; the outer `go run` cache and every output or extraction directory were
task-owned and removed after evidence capture.

## Results

Each host produced the same four files byte-for-byte:

| File | SHA-256 or comparison |
| --- | --- |
| `gox_v0.0.0-linux-rehearsal_darwin_arm64.tar.gz` | `bd39a7a5ae057624356c7217143a98070051609fc0f567ae03b4b1d6131e6ef6` |
| `gox_v0.0.0-linux-rehearsal_linux_arm64.tar.gz` | `ee6c8ca94a90ddd095f5d3d93865f018f3a5b8333d40b320cbfca043891d9bf4` |
| `gox_v0.0.0-linux-rehearsal_manifest.json` | byte-identical across hosts and checksum-valid |
| `gox_v0.0.0-linux-rehearsal_checksums.txt` | byte-identical across hosts; every listed file verified |

The manifest reported schema 1, product `gox`, the exact revision and Go
version, and the expected Darwin arm64 and Linux arm64 targets. The extracted
Darwin binary printed `gox v0.0.0-linux-rehearsal` on Darwin. The extracted
Linux binary printed the same version inside the network-isolated Linux host
and retained executable mode `0755`.

This closed the independent Linux-host artifact rehearsal for the then-current
two-target arm64 set. The name decision, signing, publication, and broader
platform-runtime gates remained open.
