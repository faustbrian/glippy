# Cache Platform Evidence, 2026-08-12

## Contract

The persistent analysis store must keep every operation beneath the validated
cache root, pin the resolved target before returning from validation, reject a
later symlink redirection into the project, refuse escaping shard symlinks, and
preserve the CLI lifecycle for reuse, pruning, invalid roots, and filesystem
failures.

Platform evidence is runtime-specific. Passing on Linux overlayfs does not
establish the same rooted-handle, hard-link publication, or filesystem behavior
on Windows, network filesystems, or other untested platforms.

## Environment

The network-isolated Linux arm64 runs used exact revision
`4032b5996a5d4cbbf3318b54c6ccf4bfa15c22fc`, Go 1.26.5, Linux
6.12.76-linuxkit, Docker Desktop 29.6.2, and the overlayfs containerd
snapshotter. The toolchain image was
`golang@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651`.

The source and existing module cache were mounted read-only. Each ordinary or
race run used its own task-owned disposable Go build cache and temporary
directory. Every container used automatic removal, and all caches and test
artifacts were removed after evidence capture.

## Results

Focused integration tests passed for:

- pinning the resolved cache root across a validation-time symlink swap;
- rejecting a cache root before creating it;
- refusing a shard symlink that escapes the opened root;
- reusing and pruning the configured persistent cache through package-aware
  `lint` and combined `check`;
- rejecting project-contained cache roots without creation; and
- reporting cache-open failures as filesystem failures.

The complete `internal/cache` and `internal/cli` package suites then passed on
Linux arm64, followed by both complete package suites under the race detector.
All runs completed with Docker networking disabled.

This closes Linux runtime evidence for the current cache-root pinning and CLI
lifecycle boundary. Windows, network filesystems, other storage drivers, and a
product-wide warm-performance claim remain open.
