# ADR 0019: Opt-In Analysis Statistics

- Status: accepted for v0.4 development
- Date: 2026-08-15

## Context And Evidence

Glippy selects syntax, types, CFG, SSA, dependency syntax, effect facts, and
persistent cache work from enabled rule metadata. Configuration introspection
could explain the maximum selected tier, but it could not show observed loading
cost, rule callback cost, cache effectiveness, or how many findings were later
suppressed or baselined. Expanding interprocedural facts without that evidence
would make latency and memory regressions harder to locate.

Ordinary diagnostic JSON is deterministic and consumed by CI and editors.
Adding timings to that envelope would make byte-stable output impossible and
would force consumers interested only in findings to accept profiling fields.
Writing both documents to standard output would also make either stream
invalid.

## Decision

Complete non-writing `lint` and combined `check` runs accept `--stats` for text
or `--stats=json` for a schema-versioned document. Diagnostics retain their
selected standard-output reporter. Statistics use standard error and are
emitted only after diagnostic reporting succeeds. Ordinary, failed, canceled,
fixing, preview, and baseline-generation runs do not emit statistics.

One invocation-owned collector aggregates file or package work. It records:

- package loading separately from in-process analysis tiers;
- the complete diagnostic outcome category and exit code;
- callback time and process-local allocation deltas by rule and tier;
- selected rule reasons for each representation;
- final visible, pre-existing, suppressed, and baselined findings by rule;
- cache lookups, hits, misses, semantic invalidations, and writes; and
- exact rule reasons for dependency syntax and effect facts.

Schema structure, ordering, rule identities, reasons, and final finding counts
are deterministic. Timing and allocation values are explicitly observational.
Allocation claims exclude Go-tool subprocesses. Cache invalidation means a
verified value for the current key failed analysis-layer validation; changed
keys and store-hidden corruption are misses.

Instrumentation uses the existing shared traversal and package schedule. It
does not rerun rules, isolate rules in separate processes, affect cache keys, or
change diagnostic order. Cache hits retain selected rule records with zero
callbacks rather than pretending the rules executed.

## Alternatives

- Add measurements to ordinary lint JSON: rejected because nondeterministic
  values would weaken its deterministic diagnostic contract.
- Write statistics after diagnostics on standard output: rejected because two
  text or JSON documents would be ambiguous and hard to capture independently.
- Always collect and hide statistics unless requested: rejected because memory
  sampling on hot rule callbacks would impose overhead on ordinary use.
- Attribute Go-command allocations to Glippy rules: rejected because
  subprocess memory is not observable through the Go runtime counters.
- Reexecute each rule independently for exact allocation isolation: rejected
  because it would abandon shared traversal, alter performance materially, and
  risk analyzing a different execution schedule.

## Consequences

Users can compare cold and warm runs, identify expensive rules or tiers, and
verify why deeper analysis loaded without changing their diagnostic pipeline.
Stats mode has measurement overhead and is suitable for investigation and
regression evidence, not as the ordinary latency benchmark itself.

Fix and baseline-generation profiling remain deferred because those workflows
perform repeated analysis and mutation stages requiring a transaction-specific
phase model. Failed-run partial telemetry also remains deferred until it can be
reported without implying completeness.

## Revisit Trigger

Revisit the stream contract if a validated consumer requires a named output
file or one combined machine envelope. Revisit allocation attribution if the Go
runtime exposes a bounded goroutine-local mechanism. Add fix-phase statistics
only with explicit initial analysis, coordination, formatting, validation,
write, and reanalysis phases.
