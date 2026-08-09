# ADR 0007: CLI, Configuration, And Filesystem Boundary

- Status: accepted for prototype
- Date: 2026-08-09

## Context And Evidence

Oxfmt demonstrates cohesive check, stdin, editor, and bounded-thread workflows,
but current writes are not atomic and nested configuration may fail after
earlier mutation. Gox requires non-mutating check behavior and prevalidated
writes.

## Decision

The command and exit contracts are defined in `docs/spec/cli.md`. Configuration
is typed TOML with one selected project configuration, explicit precedence, and
no implicit nested inheritance in the initial product. All discovery,
configuration, and formatting/fix validation completes before a file's atomic
replacement. Recursive discovery excludes vendor, module caches, VCS metadata,
and symlink traversal by default.

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
support reliable CI and machine consumers.

## Revisit Trigger

Dogfood evidence demonstrates one-root configuration cannot model a required
repository, or platform integration proves a safe broader filesystem policy.
