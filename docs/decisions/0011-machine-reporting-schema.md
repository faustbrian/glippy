# ADR 0011: Versioned Machine Reporting

- Status: accepted for formatter and lint-check prototypes
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

Lint check results use the same versioned envelope with command `lint`, mode
`check`, and a command-specific summary. The summary records analyzed files,
visible diagnostics, suppressed diagnostics, suppression problems, unused
suppressions, and completeness. Ordered file records carry normalized absolute
paths, lowercase SHA-256 source digests, and status `analyzed`.

Each lint diagnostic carries rule ID, severity, message key and text, path,
source digest, half-open physical UTF-8 byte range, related ranges, notes, help,
and named fixes with their safety class. Ordinary JSON does not carry original
source snippets or fix replacement text. An explicit editor or code-action
surface must authorize edit payload disclosure later.

Suppression syntax problems and unused directives remain separate ordered
records so reporters cannot confuse malformed policy with rule findings.
Suppressed diagnostic bodies are omitted by default and represented only by
the summary count. The constructor rejects duplicate source paths and
diagnostics whose path or digest does not match their file result. Source files
and diagnostics are sorted canonically before encoding.

Lint-fix file outcomes, text rendering, and CLI integration remain deferred.
Existing version 1 fields will not be silently repurposed.

## Alternatives Rejected

- Copy Oxfmt's human-only formatter reporter: insufficient for CI consumers.
- Copy Oxlint's current JSON object verbatim: language-specific diagnostics,
  timing, and thread fields do not define Gox formatter outcomes.
- Expose source excerpts or replacement text in ordinary lint JSON: violates
  the local-source disclosure boundary and is unnecessary for diagnostics.
- Encode formatted source inside JSON: increases memory and source-disclosure
  risk while weakening ordinary stdin/stdout editor compatibility.
- Emit JSON on stderr beside formatted stdout: two result channels make shell
  and editor integration ambiguous.
- Use newline-delimited per-worker records: completion order would leak
  concurrency nondeterminism and partial output would lack one final outcome.

## Consequences

Machine consumers can distinguish findings from failures without parsing text
and can detect incomplete summaries. Lint consumers can bind diagnostics to an
exact source version without receiving source text. The driver must retain
ordered per-file outcomes until final rendering. Large selections therefore
add one small record per file, and JSON is buffered before one stream write so
encoding failure cannot produce a partial document.

## Revisit Trigger

Before lint-fix results or text reporters stabilize, before relative-path or
URI policy changes, before source snippets or code-action edits are exposed, or
when a validated consumer requires streaming, SARIF, GitHub annotations, or
another schema.
