# ADR 0007: CLI, Configuration, And Filesystem Boundary

- Status: accepted for prototype
- Date: 2026-08-09
- Refreshed: 2026-08-12

## Context And Evidence

Oxfmt demonstrates cohesive check, stdin, editor, and bounded-thread workflows,
but current writes are not atomic and nested configuration may fail after
earlier mutation. Glippy requires non-mutating check behavior and prevalidated
writes.

## Decision

The command and exit contracts are defined in `docs/spec/cli.md`. Configuration
is typed TOML with one selected project configuration, explicit precedence, and
no implicit nested inheritance in the initial product. All discovery,
configuration, and formatting/fix validation completes before a file's atomic
replacement. Recursive discovery excludes vendor, module caches, VCS metadata,
and symlink traversal by default.

Rule options use one `lint.rule-options."rule-id"` table and are validated
against canonical registry metadata as boolean, integer, string, or string-list
values. Unknown rules, unknown option names, wrong scalar kinds, and missing
required values on enabled rules fail before traversal. Required options have
no default; every optional option declares one canonical metadata default of
the same type. One immutable resolved snapshot is routed to that rule across
every native analysis tier; no option value is an ambient environment input.

File-formatting preparation is bounded by the selection size, `GOMAXPROCS`, and
a hard ceiling of 8 workers. A release-scale study of 4, 8, and 16 workers found
that 16 workers increased the Darwin median and maximum peak-memory envelope
for only a small latency improvement, while Linux showed no material median
latency improvement. Sorted task indexes own result and error ordering.
Signal or caller cancellation stops scheduling and is observed before every
filesystem replacement; exit code 130 distinguishes cancellation from an
internal tool defect.

Write-mode support claims are scoped to platform and filesystem pairs with
runtime integration evidence. Glippy supports macOS and Linux only. The current
Phase 2 replacement evidence covers Darwin arm64 on APFS and Linux arm64 on
overlayfs; admitted release artifacts additionally target amd64. Windows and
all other operating systems are intentionally unsupported, so Windows runtime
evidence is not a release gate. The evidence and revisit boundary are recorded
in
[`../research/filesystem-platform-evidence-2026-08-11.md`](../research/filesystem-platform-evidence-2026-08-11.md).

Write and fix guarantees apply only to local filesystems whose documented
semantics satisfy the recorded same-directory replacement contract. Network,
distributed, and userspace filesystems are outside the supported write/fix
boundary unless separately admitted. Glippy does not claim survival of a forced
power loss; a successful return proves the implemented file and directory sync
sequence completed, not storage-hardware durability beyond the operating system
contract.

The lint CLI resolves configuration before selecting its analysis boundary.
Syntax-only files, directories, and terminal `...` patterns reuse deterministic
physical-file discovery and cannot invoke `go/packages`. A selection containing
types-tier, CFG-tier, or SSA-tier rules instead converts its inputs into one
non-mutating, test-aware package request rooted at the common project boundary.
It selects a compatible effective module or workspace vendor manifest when one
exists and otherwise uses read-only module mode, then uses the shared package
driver and typed reporters. Package prerequisite, vendor-consistency, and
source-model problems map to source-error exit code 2 while retaining valid
partial results.

One typed invocation accepts only one project root and configuration. This
avoids silently applying one package graph to heterogeneous policy until
per-path package configuration has an explicit design. Typed fixes reuse the
single-file transaction boundary: planning uses a fresh package analysis,
candidate validation reloads the package through an exact-path overlay, and
package or source-model failures preserve the original file. A final fresh load
supplies complete reporting results. Typed fixing is serialized and
cache-independent and does not claim multi-file atomicity; syntax-only recursive
fixes retain the file-owned validation path.

Combined `check` uses the same tier-sensitive planning. Syntax selections keep
the physical file driver. A types, CFG, or SSA selection performs one package
load, and both formatting and linting consume only the immutable sources
captured by that load. Package prerequisite and source-model problems remain
completed source-error results alongside valid formatting outcomes. Text is
buffered until reporting succeeds, while JSON retains those typed problem
channels and exact source digests. This prevents a second filesystem read from
giving formatting and deep analysis different source versions.

## Alternatives Rejected

- Default in-place `fmt`: unsafe and surprising for stdin/editor workflows.
- Lazy nested configuration during writes: permits partial mutation after a
  late configuration error.
- General cascaded configuration or executable config: unnecessary complexity
  and a nondeterministic security surface.
- Follow symlinks by default: unclear root authorization and race semantics.

## Consequences

Some monorepositories may require explicit invocations or future typed path
overrides. Exit categories are more detailed than common formatter tools but
support reliable CI and machine consumers. Parallel preparation increases open
file and memory pressure up to the documented ceiling, while serialized
replacement preserves deterministic disclosure of partial multi-file writes.
Successful compilation on another target does not imply supported write
atomicity or durability there.

## Revisit Trigger

Dogfood evidence demonstrates one-root configuration cannot model a required
repository, or macOS/Linux integration proves a safe broader local-filesystem
policy. Adding another operating system requires an explicit maintainer change
to the supported-platform policy before runtime work begins.
