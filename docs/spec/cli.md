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
gox lint [paths...]
gox lint --fix [paths...]
gox check [paths...]
gox explain <rule>
gox version
```

`fmt` without a write or check flag writes formatted content to stdout for one
explicit file or stdin. Multiple filesystem inputs require `--write` or
`--check`. `--write` and `--check` are mutually exclusive. Standard input MAY
use `--stdin-filepath` for language version and configuration context but MUST
NOT make that path writable.

Standard-input fragments use an explicit fragment kind and the wrapper/mapping
contract in [`fragments.md`](fragments.md). Fragment kind inference after a
parse error is prohibited.

`lint` never writes unless a fix flag is present. Ordinary `--fix` applies safe
fixes only. Suggestion and unsafe fixes require distinct explicit selections.
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

An invocation MUST choose the most severe applicable nonzero category using
the order 6, 5, 4, 3, 2, 1. Machine output MUST include the symbolic category
and schema version rather than requiring consumers to infer meaning from the
integer alone.

## Filesystem Boundary

Check and stdout modes MUST NOT change tracked or untracked files, metadata, or
cache state required for correctness. Write mode MUST validate every selected
file before replacing any file in that file's transaction. Replacement MUST
use a same-directory temporary file, preserve permissions, verify the source
identity/version, and use atomic rename where the platform supports it.

The stable single-file phase MUST leave original content intact on any failed
validation. Multi-file fixes remain unsupported until a recovery transaction
has a separate accepted decision.
