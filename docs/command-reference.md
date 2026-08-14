# Command Reference

Glippy v0.2 uses its intended product and binary identity. This reference
documents the implemented command surface; it is not an installation contract
or release announcement. The source repository has not been remotely renamed
and no v0.2 tag or release is authorized before maintainer review.

## Command Summary

| Command | Purpose | Writes source |
| --- | --- | --- |
| `glippy fmt [path]` | Format standard input or one explicit file to standard output | No |
| `glippy fmt --check [paths...]` | Report files whose canonical formatting differs | No |
| `glippy fmt --diff [paths...]` | Print unified formatting differences | No |
| `glippy fmt --write [paths...]` | Validate and replace changed files | Yes |
| `glippy lint [paths...]` | Report enabled lint diagnostics | No |
| `glippy lint --new-from=<git-ref> [paths...]` | Report diagnostics on changed lines | No |
| `glippy lint --fix [paths...]` | Apply safe fixes | Yes |
| `glippy lint --generate-baseline=<path> [paths...]` | Write a deterministic adoption baseline | Baseline only |
| `glippy check [paths...]` | Check formatting and lint diagnostics together | No |
| `glippy check --new-from=<git-ref> [paths...]` | Check changed-line formatting and diagnostics | No |
| `glippy explain <rule>` | Print canonical rule documentation | No |
| `glippy version` | Print the resolved Glippy version | No |
| `glippy completion <shell>` | Generate Bash, Zsh, or Fish completion | No |

## Inputs

Formatting is file-oriented. Lint and combined check accept explicit `.go`
files, directories, and recursive filesystem patterns whose final component is
`...`, such as `./...`. Commands that accept paths use the current directory
when their contract documents a default; `lint` and `check` default to `.`.

Typed lint and check selections must resolve to one project root and one
configuration. A heterogeneous typed selection fails instead of silently
analyzing only part of the request. Package loading includes test variants and
uses the build tags, GOOS, GOARCH, and cgo policy selected by configuration.

## Format

### Standard input and one file

With no path, `fmt` reads one complete Go file from standard input and writes
only its formatted bytes to standard output:

```sh
glippy fmt < source.go > source.formatted.go
```

One explicit file may be formatted to standard output without changing it:

```sh
glippy fmt source.go
```

Use `--stdin-filepath` to supply project, configuration, and source-version
context for editor input. The named file is not read or written:

```sh
glippy fmt --stdin-filepath=/project/source.go < buffer.go
```

Incomplete editor buffers are not accepted as complete files. Explicit
declaration, statement, and expression fragments use an equals-form flag:

```sh
glippy fmt --fragment=declaration < declaration.go
glippy fmt --fragment=statement < statement.go
glippy fmt --fragment=expression < expression.go
```

Fragment mode and `--stdin-filepath` cannot be combined with filesystem
operands or a write, check, or diff mode.

### Check, diff, and write

```sh
glippy fmt --check .
glippy fmt --diff .
glippy fmt --write .
```

`--check` prints changed paths and exits with findings when any selected file
differs. `--diff` prints deterministic three-context unified differences.
Neither mode changes source or metadata.

`--write` validates the complete selection before replacement starts, formats
changed files through the canonical formatter, preserves ordinary permissions,
and does not replace an unchanged file. It refuses generated files and paths
that traverse symlinks. Replacement uses the documented stale-source and
same-directory atomic-write boundary.

`--write`, `--check`, and `--diff` are mutually exclusive. Multiple filesystem
inputs require one of these modes.

### Format options

| Option | Accepted with | Meaning |
| --- | --- | --- |
| `--config=<path>` | All format modes | Use one explicit configuration instead of discovery |
| `--reporter=text\|json` | Path-based check and write | Select human or schema-version-1 output |
| `--stdin-filepath=<path>` | Standard input | Supply non-writing filepath context |
| `--fragment=<kind>` | Standard input | Select `declaration`, `statement`, or `expression` |

`--reporter=text` is also accepted with `--diff`; JSON is rejected when
standard output contains formatted source or a unified diff.

## Lint

```sh
glippy lint .
glippy lint ./...
glippy lint --reporter=json ./...
```

Ordinary lint is non-writing. Fix-class flags may write source, while
`--generate-baseline` writes only its named baseline document. The default
`correctness` preset group enables the current high-signal default rules.
Projects compose additional groups with `lint.presets`, escalate every
resolved warning with `lint.warnings-as-errors`, and retain final per-rule
control through `lint.rules`. The legacy singular `lint.preset` remains
accepted for v0.1 configuration compatibility but cannot be combined with
`lint.presets`. Use
`glippy explain <rule>` to inspect a rule's prerequisites, configuration, fix
safety, examples, and known limitations. The
[suppression reference](suppressions.md) defines exact-rule waivers, scopes,
reasons, expiry, unused directives, and formatter ownership.

### Fix classes

| Option | Authorized fixes |
| --- | --- |
| `--fix` | Safe only |
| `--fix-suggestions` | Suggestion only |
| `--fix-unsafe` | Unsafe only |

The flags are independently composable. Enabling unsafe fixes does not enable
safe or suggestion fixes. Glippy refuses ambiguous alternatives, stale source,
overlapping edits, generated files, and symlink-traversing write targets.
Accepted edits are reparsed, formatted, reanalyzed, and validated before one
file is replaced. Fix transactions are single-file and do not claim multi-file
atomicity.

Lint accepts `--config=<path>` and `--reporter=text|json`. Text is the default.
Machine output is schema version 1 and omits source snippets and replacement
text. See the [machine output reference](machine-output.md) for field, range,
completeness, ordering, and fix-provenance semantics.

`--generate-baseline=<path>` writes a strict source-free JSON adoption
baseline relative to one project root. It analyzes visible diagnostics before
baseline application and cannot be combined with fix flags or the JSON
reporter. See the [lint baseline reference](baselines.md).

### Changed-code adoption

`--new-from=<git-ref>` resolves the deterministic merge base shared by the
named ref and `HEAD`, analyzes the complete selected files and packages, and
reports only diagnostics intersecting lines changed from that merge base.
Modified renames retain line ownership; pure renames introduce no diagnostics.
Untracked files are wholly changed. JSON summary fields distinguish visible
diagnostics from `preexisting_diagnostics` hidden by this filter.

Fix modes remain available. A fix is offered only when every edit is on owned
lines, and the complete formatter-normalized result is rejected if it changes
any pre-existing line. `--new-from` cannot be combined with baseline
generation. The command invokes the local `git` executable without network
access, external diff drivers, text conversions, pagers, or optional Git
locks. Inherited Git repository overrides are removed, global and system Git
configuration is ignored, and repository-configured clean, smudge, or process
filters are neutralized so analyzing an untrusted checkout cannot execute a
filter command. The selected paths must belong to the resolved repository.

## Combined Check

```sh
glippy check .
glippy check --reporter=json ./...
```

`check` is the non-mutating CI entry point. It compares canonical formatting
and runs every enabled lint tier over the same immutable source identity. Exit
code 1 means formatting differences or lint findings; tool and source failures
use distinct nonzero categories. Text output is buffered until the invocation
has a complete result, while JSON reports completeness explicitly.

`check` accepts `--config=<path>` and `--reporter=text|json` and defaults to the
current directory.

With `--new-from`, formatting differences are actionable only when the full
formatter transformation is owned by changed lines. A difference touching an
unchanged line is reported in JSON as `format_status: "preexisting"` and does
not change the exit status. Lint diagnostics use the same changed-line filter.

## Rule Documentation

```sh
glippy explain duplicate-condition
```

`explain` accepts exactly one registered rule ID. Its output derives from the
same immutable metadata used for rule registration and scheduling. It performs
no project discovery or configuration loading. The
[published rule catalog](lint-rules.md) is generated from the same metadata.

## Version

```sh
glippy version
```

The command writes exactly `glippy <version>` followed by a newline. A local build
without version metadata reports `devel`. Version inspection does not load a
project or configuration.

## Shell Completion

```sh
glippy completion bash
glippy completion zsh
glippy completion fish
```

Completion generation includes the commands, supported options, enum values,
filesystem operands, and rule IDs compiled into that binary. See the
[shell-completion guide](shell-completion.md) for shell-specific setup.

## Configuration

Glippy discovers `.glippy.toml` at the selected project boundary unless `--config`
names one exact file. Configuration requires `version = 1`; unknown fields,
unknown rules, duplicate semantic keys, and invalid values fail rather than
being ignored. See the [configuration contract](spec/configuration.md) for the
current schema and discovery policy.

## Exit Codes

| Code | Meaning |
| ---: | --- |
| 0 | Success with no actionable findings |
| 1 | Formatting differences or lint findings |
| 2 | Parse, source-version, or required type errors |
| 3 | Invalid invocation or configuration |
| 4 | Stale or conflicting fixes |
| 5 | Discovery, read, write, or replacement filesystem failure |
| 6 | Internal invariant or unexpected tool failure |
| 130 | Cancellation or deadline exceeded |

Machine output includes both the symbolic outcome category and numeric code.
Callers should distinguish findings from failures instead of treating every
nonzero result as the same outcome.

## Contract References

- [CLI specification](spec/cli.md)
- [Configuration specification](spec/configuration.md)
- [Formatter rules](formatter-rules.md)
- [Lint rule catalog](lint-rules.md)
- [Suppression reference](suppressions.md)
- [Fix safety](spec/fix-safety.md)
- [Machine output reference](machine-output.md)
- [Machine reporting decision](decisions/0011-machine-reporting-schema.md)
- [Supported Go versions](supported-go-versions.md)
- [Editor integration](editor-integration.md)
- [CI and pre-commit setup](ci-and-precommit.md)
- [Contributor architecture and rule authoring](contributing.md)
