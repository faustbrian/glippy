# CLI Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

The binary name is written as `gox` while ADR 0001 remains provisional.

## Commands

```text
gox fmt [paths...]
gox fmt --write [paths...]
gox fmt --check [paths...]
gox fmt --diff [paths...]
gox lint [paths...]
gox lint --fix [paths...]
gox check [paths...]
gox explain <rule>
gox version
```

`gox version` MUST accept no operands or flags and MUST write exactly
`gox <version>\n` to standard output. Version resolution MUST prefer explicit
link-time release metadata, then a non-development Go module build version, and
finally `devel`. The command MUST NOT read source or configuration files or
modify the filesystem.

`gox explain <rule>` MUST accept exactly one rule ID and render the complete
human documentation derived from that rule's immutable compiled metadata. It
MUST NOT discover project files or load configuration. Unknown rule IDs and
invalid argument counts exit as invalid invocation without writing stdout.
Cancellation and output failures retain the common exit categories. The
initial compiled registry remains empty until a rule satisfies the admission
gate, so no successful public rule ID exists yet.

`fmt` without a write, check, or diff flag writes formatted content to stdout
for one explicit file or stdin. Multiple filesystem inputs require `--write`,
`--check`, or `--diff`. Those three modes are mutually exclusive. Standard input
MAY use `--stdin-filepath` for language version and configuration context but
MUST NOT make that path writable.

`fmt --diff` is path-based and non-writing. It MUST emit three-context-line
unified differences for changed files in normalized path order, omit unchanged
files, and use the normalized source path plus `.orig` as the old label and the
source path as the new label. Labels containing tabs or line breaks MUST be
quoted. A changed selection exits with findings; an already formatted selection
exits successfully without output. Matching MUST remain bounded: Gox uses
unique patience anchors, a capped LCS for remaining regions, and a complete
delete-and-insert region when a work ceiling is reached.

Path-based check and write modes MUST accept `--reporter=text|json`; text MUST
remain the default. JSON MUST NOT be accepted when stdout contains formatted
source or unified differences. A valid JSON request MUST receive JSON even when
the remaining invocation is invalid.

Successful text-mode `fmt --write` is silent; JSON mode emits its result
envelope. Write mode validates every selected configuration, source file, and
formatted result before beginning replacement. Generated files, explicit
symlinks, and explicit paths traversing an in-project symlink remain readable
in non-writing modes but are refused by write mode.

Standard-input fragments use an explicit fragment kind and the wrapper/mapping
contract in [`fragments.md`](fragments.md). Fragment kind inference after a
parse error is prohibited.

The supported format-on-save setup and editor failure contract are documented
in [`editor-integration.md`](../editor-integration.md).

The Phase 3 syntax-only `lint` check accepts explicit files and directories,
defaults to the current directory, and accepts `--reporter=text|json` plus an
optional explicit configuration path. It discovers and sorts physical Go files,
loads the selected preset and exact-rule overrides, runs the file-owned syntax
driver, and never writes source. Visible diagnostics, suppression problems, and
unused suppressions exit with findings; suppressed diagnostics alone do not.
Parse failures, invalid configuration, filesystem failures, cancellation, and
reporting failures retain their common exit categories. JSON remains valid and
incomplete for invalid invocations and failures. Package patterns such as
`./...`, typed loading, suggestion selection, and unsafe-fix flags remain
rejected until their separate contracts are implemented.

`lint` never writes unless a fix flag is present. Ordinary `--fix` applies safe
fixes only. It automatically selects a diagnostic only when that diagnostic
offers exactly one named safe fix; multiple safe alternatives violate the rule
contract and fail before any write. Suggestion and unsafe fixes remain visible
but require future distinct explicit selections.

`lint --fix` prevalidates every selected configuration and source before its
first write, refuses generated files and paths traversing symlinks, coordinates
each source version independently, reparses and formatter-normalizes accepted
edits, reruns syntax analysis over the final bytes, and then uses the shared
atomic replacement boundary. Stale files and overlapping fixes are conflicts.
One file's conflict does not silently select a winner or prevent independent
file transactions from being attempted. Cancellation stops before the next
replacement and reports earlier confirmed writes. A post-format analysis engine
failure remains an internal or cancellation outcome and MUST NOT be downgraded
to an ordinary rejected-fix finding.

Successful text fix output contains only diagnostics and rejected-fix reasons
left after coordination; a completely fixed invocation is silent. Lint-fix JSON
uses file statuses `pending`, `unchanged`, `fixed`, `conflict`, `failed`, and
`possibly_fixed`. Each file retains its original source digest and the analyzed
result digest, while applied and rejected records retain original-source rule,
fix, and range provenance without exposing replacement text. An applied record
describes the validated in-memory coordination result; the file status controls
whether replacement was confirmed, refused as stale, or may have completed.
Stale replacement records also reject each coordinated fix with the stable
`stale-source` reason so text consumers receive an actionable explanation.
`check` combines formatting differences and enabled lint diagnostics over one
immutable discovery snapshot and MUST never write.

Formatting inputs are file-oriented. Typed lint patterns are package-oriented.
The CLI MUST reject combinations whose file and package interpretations are
ambiguous instead of silently changing modes.

## Determinism And Reporting

Discovery results MUST be normalized and sorted before scheduling. Parallel
work MUST feed canonical outcome records that are sorted by normalized path,
physical byte range, rule ID, severity, and stable message key before any
reporter renders them. Text and versioned JSON reporters MUST derive their exit
category from the same aggregate result.

Diff construction MUST follow normalized task order after formatting completes.
It MUST finish the complete ordered difference before the first stdout write so
a later source or formatting failure cannot leave a partial patch.

Schema version 1 JSON has this deterministic envelope:

```json
{
  "schema_version": 1,
  "command": "fmt",
  "mode": "check",
  "outcome": { "category": "findings", "exit_code": 1 },
  "summary": { "files": 1, "changed": 1, "complete": true },
  "files": [
    { "path": "/project/source.go", "status": "different" }
  ],
  "errors": []
}
```

Paths MUST be normalized absolute paths, and arrays MUST retain deterministic
task order.
Check statuses are `unchanged` and `different`. Write statuses are `pending`,
`unchanged`, `formatted`, `conflict`, `failed`, and `possibly_formatted`.
`pending` means replacement or unchanged-file revalidation was not reached.
`summary.complete` MUST be false when the reported file records or counts do
not cover a complete invocation. Machine errors MUST NOT include source
snippets by default. Timing and worker counts MUST NOT form part of result
identity.

Successful reports use mode `check` or `write`. Invalid invocations retain the
requested path mode when it can be resolved; mode is `invalid` when parsing
cannot establish one.

Lint JSON uses the same schema version and outcome envelope with a
command-specific summary, exact source digests, and half-open physical byte
ranges. It MUST NOT include original source snippets or fix replacement text by
default. Suppression problems and unused directives remain distinct arrays;
suppressed diagnostic bodies are represented only by a summary count.

Lint text reports physical 1-based line and UTF-8 byte-column locations as
`path:line:column: severity[rule-id]: message`, followed by indented related
locations, notes, help, and named fix safety when present. Suppression problems
and unused directives use distinct labels. Text reporting MUST validate exact
source identity and every range associated with a rendered record before
emitting output, and MUST NOT include source excerpts or fix replacement text.

Formatter read, parse, and layout preparation uses at most the smaller of the
selection size, `GOMAXPROCS`, and 32 workers. Task identity is assigned only
after normalized-path sorting. Completion timing cannot reorder findings.
Failure selection follows exit severity first and normalized task order within
one severity.

The binary cancels an invocation on interrupt or termination signals. Library
callers MAY supply the same contract through `RunContext`. Cancellation is
checked between discovery, configuration, reads, formatting, output, and each
replacement; it exits with code 130 and MUST NOT begin another replacement once
observed. If cancellation follows an earlier replacement, the diagnostic lists
the files already replaced. Reading an arbitrary standard-input stream cannot
be interrupted until that stream's `Read` operation returns.

## Exit Categories

| Code | Category |
| ---: | --- |
| 0 | Success with no actionable findings |
| 1 | Formatting differences or lint findings |
| 2 | Source parse, language-version, or required type errors |
| 3 | Invalid invocation or configuration |
| 4 | Stale or conflicting fixes |
| 5 | Discovery, read, write, or replacement filesystem failure |
| 6 | Internal invariant or unexpected tool failure |
| 130 | Invocation canceled or deadline exceeded |

An invocation MUST choose the most severe applicable nonzero category using
the order 6, 5, 4, 3, 2, 1. Machine output MUST include the symbolic category
and schema version rather than requiring consumers to infer meaning from the
integer alone. Cancellation terminates the incomplete invocation instead of
participating in aggregate finding severity.

## Filesystem Boundary

Check, diff, and stdout modes MUST NOT change tracked or untracked files,
metadata, or cache state required for correctness. Write mode MUST validate
every selected file before replacing any file in that file's transaction.
Replacement MUST
use a same-directory temporary file, preserve permissions, verify the source
identity/version, and use atomic rename where the platform supports it.

The stable single-file phase MUST leave original content intact on any failed
validation. Multi-file fixes remain unsupported until a recovery transaction
has a separate accepted decision.

Unchanged formatted bytes do not replace the source inode or update its
modification time. Changed files use a same-directory temporary file, preserve
ordinary permission bits, sync and close the temporary file, revalidate the
source inode and bytes, rename the temporary file over the source, and sync the
containing directory. Snapshot reads, temporary files, validation, and rename
operations are scoped through the selected project root. On platforms where
`os.Root` provides descriptor-relative containment, a concurrent symlink change
cannot redirect those operations outside the authorized tree.

Common rename APIs cannot condition replacement on the destination still
having a particular inode. Gox revalidates immediately before rename and
refuses observed source or symlink changes, but it does not claim protection
against a path change in the final validation-to-rename interval. A directory
sync failure is reported as a filesystem error after rename; in that case the
new content may already be visible even though durable replacement was not
confirmed.

Multi-file formatter invocations prevalidate the complete selection but replace
one file at a time. If a later replacement fails after earlier files changed,
the diagnostic lists every earlier replacement. A non-stale filesystem failure
may occur after rename, so the failing changed path is reported as possibly
replaced rather than silently implying rollback.

JSON reporting MUST preserve the same disclosure through ordered file statuses.
If JSON construction, encoding, or stream output fails after replacements,
stderr MUST name every completed or possibly completed replacement, and the
invocation MUST return the applicable failure category.

Gox MUST advertise write and fix mode support only for operating-system,
architecture, and filesystem combinations with runtime replacement evidence.
The current Phase 2 evidence covers Darwin arm64 on APFS and Linux arm64 on
overlayfs. Windows amd64 compilation is proven, but a release MUST NOT advertise
Windows write or fix support until a runtime suite passes and the non-atomic
rename boundary is accepted or replaced. Network and distributed filesystems
are unproven even on an otherwise supported operating system.
