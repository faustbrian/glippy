# ADR 0011: Versioned Machine Reporting

- Status: accepted for formatter prototype; lint extensions deferred
- Date: 2026-08-10

## Context And Evidence

Oxfmt source at Oxc commit `8e9b95f` exposes write, check, and
list-different modes, but its formatter reporter only collects graphical errors
and does not provide a machine result schema. Oxlint at the same revision uses
`--format=json` for detailed diagnostics, but its example envelope includes
run timing and thread counts and does not identify a schema version. Those
runtime fields are useful for performance output but are not deterministic
result identity.

Gox needs one machine envelope that reports the same outcome and exit category
as text mode, preserves normalized task order, distinguishes complete from
partial work, and can later carry lint and fix diagnostics without exposing
source snippets by default. Formatter stdout content cannot share one stream
with a JSON result envelope.

## Decision

Path-based `fmt --check` and `fmt --write` accept
`--reporter=text|json`; text remains the default. JSON uses schema version 1,
is emitted on stdout, and contains command, mode, symbolic and numeric outcome,
summary, ordered file outcomes, and errors. Invalid invocations that request
JSON also return a JSON envelope. Formatter-content stdout modes reject JSON so
the stream has one unambiguous media type.

Successful reports use `check` or `write` mode. Invalid invocations retain a
resolvable requested mode and otherwise use `invalid`.

Schema version 1 uses normalized absolute paths and deterministic arrays. It
omits elapsed time, thread count, source snippets, and environment-dependent
metadata. `summary.complete` is false when discovery, preparation,
replacement, or cancellation prevented a complete outcome; counts
in an incomplete summary must not be interpreted as totals for work that was
not reached.

Check file statuses are `unchanged` and `different`. Write statuses are
`pending`, `unchanged`, `formatted`, `conflict`, `failed`, and
`possibly_formatted`. A JSON output failure falls back to a concise text error
on stderr and still discloses completed or possibly completed replacements.

Lint diagnostics and fix metadata will extend the versioned envelope through a
separately reviewed Phase 3 decision. Existing version 1 fields will not be
silently repurposed.

## Alternatives Rejected

- Copy Oxfmt's human-only formatter reporter: insufficient for CI consumers.
- Copy Oxlint's current JSON object verbatim: language-specific diagnostics,
  timing, and thread fields do not define Gox formatter outcomes.
- Encode formatted source inside JSON: increases memory and source-disclosure
  risk while weakening ordinary stdin/stdout editor compatibility.
- Emit JSON on stderr beside formatted stdout: two result channels make shell
  and editor integration ambiguous.
- Use newline-delimited per-worker records: completion order would leak
  concurrency nondeterminism and partial output would lack one final outcome.

## Consequences

Machine consumers can distinguish findings from failures without parsing text
and can detect incomplete summaries. The driver must retain ordered per-file
outcomes until final rendering. Large selections therefore add one small
record per file, and JSON is buffered before one stream write so encoding
failure cannot produce a partial document.

## Revisit Trigger

Before lint reporters stabilize, before relative-path or URI policy changes,
before source snippets are exposed, or when a validated consumer requires
streaming, SARIF, GitHub annotations, or another schema.
