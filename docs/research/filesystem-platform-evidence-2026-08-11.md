# Filesystem Platform Evidence

Date: 2026-08-11

## Contract

The Phase 2 writer must keep unchanged files untouched, preserve ordinary
permission bits, reject stale source identity or bytes, confine operations to
the authorized root, create the temporary file beside the source, sync and
close it before replacement, and disclose any failure that may occur after the
replacement becomes visible.

Platform evidence is scoped to the operating system, architecture, and
filesystem exercised. It does not transfer atomicity or durability claims to a
different local, network, distributed, or userspace filesystem.

## Runtime Evidence

The complete repository test and race suites passed with Go 1.26.5 on
Darwin 27.0.0 arm64. Filesystem integration tests used APFS and exercised
permission preservation, unchanged-file identity, stale bytes and metadata,
replaced roots, final and intermediate symlinks, temporary cleanup, atomic
replacement, and the formatter and lint-fix callers.

The same `internal/filesystem`, `internal/fix`, and `internal/cli` package tests
passed inside the official `golang:1.26.5` image on a Linux arm64 Docker engine
using overlayfs. The image digest was
`sha256:7caba5286b4c3613a337b709c573047d8ae62ee76106647313b61e72b99f20af`.
The task-owned container, image tag, and image were removed after the run.

These runs establish the current write-mode contract only for the recorded
Darwin/APFS and Linux/overlayfs pairs. They do not establish crash recovery
under forced power loss or atomicity on network mounts.

## Unsupported Platforms And Filesystems

Gox supports macOS and Linux only. The filesystem, fix, and CLI test binaries
cross-compile for Windows amd64, but Windows runtime evidence is intentionally
not a release gate and no Windows write or fix support is claimed.

Plan 9, WebAssembly, WASI, network, distributed, userspace, and other unrecorded
platform/filesystem pairs have no write-mode claim. Forced-power-loss durability
is also outside the supported contract. Check and standard-output modes remain
non-mutating by contract, but compilation alone is not runtime evidence for an
unsupported platform.

## Authorities And Revisit Trigger

- Go 1.26.5 [`os.Rename`](https://pkg.go.dev/os@go1.26.5#Rename) and
  [`os.Root`](https://pkg.go.dev/os@go1.26.5#Root) document the portability
  limits used here.
- Microsoft
  [`FlushFileBuffers`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers)
  documents file and volume flushing but does not establish Gox's parent
  directory durability contract.

Add a platform/filesystem pair only after the replacement integration suite
runs there and the atomicity, symlink-containment, permission, and durability
claims are reconciled with that platform's authoritative APIs.
