# ADR 0011: Versioned Machine Reporting

- Status: accepted for formatter, lint-check, lint-fix, and combined-check prototypes
- Date: 2026-08-11

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

Typed lint results add optional `package_diagnostics` and `source_problems`
arrays plus matching summary counts. Package diagnostics carry the opaque
package ID, a stable `unknown`, `list`, `parse`, or `type` kind, the upstream
position when present, and the message. The upstream no-position sentinel `-`
is omitted. Source problems carry a normalized absolute path, lowercase SHA-256
digest of the captured bytes, and message. These records remain separate from
rule diagnostics and generic tool errors so a consumer can classify required
source/type failures without mistaking them for linter findings or internal
failures.

Lint text uses physical 1-based line and UTF-8 byte-column locations. Primary
diagnostics use `path:line:column: severity[rule-id]: message`; related
locations, notes, help, and named fix safety use indented continuation lines.
Suppression problems and unused directives remain distinct. The renderer sorts
files and diagnostics canonically, validates exact source identity and every
range associated with rendered records including fix edits and suppression
targets, and omits source excerpts and replacement text.

Typed lint text renders canonical package diagnostics before canonical
source-model problems and exact-source lint records. It uses
`position: package[kind] package-id: message` when an upstream position exists
and `path: source: message` for source-model failures. The package result retains
the immutable load-owned source index so text locations do not require a later
filesystem read.

The lint check CLI emits these text and JSON contracts for syntax-only and typed
success, findings, invalid invocation, source/configuration/filesystem failure,
and cancellation. A completed typed run keeps prerequisite failures in their
dedicated channels, reports source-error exit code 2, and remains complete.
Incomplete JSON retains every analysis result completed before an engine or I/O
failure.

The combined `check` command uses command and mode `check` and runs formatter
comparison plus lint analysis over one immutable source snapshot per file.
Its summary adds `formatting_differences`; each ordered file carries the exact
source digest and format status `unchanged` or `different`. Diagnostics,
suppression problems, and unused suppressions reuse the lint schema. The
constructor rejects missing, extra, or mismatched format outcomes so a machine
consumer never receives formatting and lint records from different source
versions. Incomplete failures retain completed file records, while text mode
buffers all findings and emits none after a source or execution failure.

Lint fix results use mode `fix` and file statuses `pending`, `unchanged`,
`fixed`, `conflict`, `failed`, and `possibly_fixed`. File records carry the
original source digest and analyzed result digest. Applied and rejected fix
records carry original-source rule ID, fix name, and byte range; rejections add
a stable reason and message. They omit replacement text. Applied provenance is
an in-memory coordination fact, while file status states whether disk
replacement was confirmed. Incomplete cancellation and failure results retain
pending files and every earlier confirmed or possible write. If JSON result
construction, encoding, or output fails after a write, stderr names those
paths. A stale replacement retains the original analyzed digest and adds a
`stale-source` rejection for each coordinated fix.

Existing version 1 fields and the text diagnostic grammar will not be silently
repurposed.

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

Before relative-path, URI, or physical-location policy changes, before source
snippets or code-action edits are exposed, or when a validated consumer
requires streaming, SARIF, GitHub annotations, or another schema.
