# Command Reference

Glippy v0.5 uses its intended product and binary identity. This reference
documents the implemented command surface; it is not an installation contract
or release announcement. No tag or release is authorized until the corrected
candidate passes its complete verification and independent-review gates.

## Command Summary

| Command | Purpose | Writes source |
| --- | --- | --- |
| `glippy fmt [path]` | Format standard input or one explicit file to standard output | No |
| `glippy fmt --check [paths...]` | Report files whose canonical formatting differs | No |
| `glippy fmt --diff [paths...]` | Print unified formatting differences | No |
| `glippy fmt --write [paths...]` | Validate and replace changed files | Yes |
| `glippy lint [paths...]` | Report enabled lint diagnostics | No |
| `glippy lint -W<target> -Dwarnings [paths...]` | Apply ordered command-line lint levels | No |
| `glippy lint --only=<rules> [paths...]` | Run only exact rule IDs after project policy | No |
| `glippy lint --except=<rules> [paths...]` | Exclude exact rule IDs after project policy | No |
| `glippy lint --new-from=<git-ref> [paths...]` | Report diagnostics on changed lines | No |
| `glippy lint --fix [paths...]` | Apply safe fixes | Yes |
| `glippy lint --fix --diff [paths...]` | Preview validated safe fixes as unified differences | No |
| `glippy lint --generate-baseline=<path> [paths...]` | Write a deterministic adoption baseline | Baseline only |
| `glippy lint --stats[=text\|json] [paths...]` | Report opt-in rule and analysis cost statistics | No |
| `glippy check [paths...]` | Check formatting and lint diagnostics together | No |
| `glippy check -D warnings [paths...]` | Check with command-line warning denial | No |
| `glippy check --new-from=<git-ref> [paths...]` | Check changed-line formatting and diagnostics | No |
| `glippy check --stats[=text\|json] [paths...]` | Check while reporting opt-in analysis statistics | No |
| `glippy lsp [flags]` | Serve live diagnostics, formatting, and validated code actions over stdio | No |
| `glippy init [--profile=<profile>] [directory]` | Create a starter `.glippy.toml` without overwriting | Configuration only |
| `glippy config check [path]` | Validate discovered or explicit configuration | No |
| `glippy config show [path]` | Explain effective configuration and rule selection | No |
| `glippy rules [filters]` | Discover compiled rule metadata | No |
| `glippy explain <rule>` | Print canonical rule documentation | No |
| `glippy explain <rule> --json` | Print versioned canonical rule metadata | No |
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

For CI, `[[analysis.targets]]` can select up to 32 explicit GOOS, GOARCH,
build-tag, and cgo combinations. Package-aware lint, check, and baseline
generation analyze every target, deduplicate identical findings, and label
target-specific output. Syntax-only lint and the LSP continue using their
single file or base-analysis policy. Fix and fix-preview modes reject a target
matrix.

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
glippy lint --reporter=short ./...
glippy lint --reporter=json ./...
glippy lint --reporter=github ./...
glippy lint --reporter=sarif ./...
```

Ordinary lint is non-writing. Fix-class flags may write source, while
`--generate-baseline` writes only its named baseline document. The default
`correctness` preset group enables the current high-signal default rules.
Projects compose additional groups with `lint.presets`, escalate every
resolved warning with `lint.warnings-as-errors`, and retain final per-rule
control through `lint.rules`. The legacy singular `lint.preset` remains
accepted for v0.1 configuration compatibility but cannot be combined with
`lint.presets`. Ordered `[[lint.overrides]]` entries apply exact rule
severities to matching project-relative paths before command-line lint levels:

```toml
[[lint.overrides]]
paths = ["**/*_test.go", "testdata/**"]

[lint.overrides.rules]
discarded-error = "off"
blank-error-discard = "warn"
```

`*`, `?`, and character classes match within one path segment; a complete
`**` segment matches zero or more segments. Paths use `/` on every supported
platform. Later matching entries replace earlier severities for the same rule.
Unknown rules, invalid globs, absolute or parent-traversing patterns, duplicate
patterns, empty path sets, and empty rule sets fail configuration loading. Use
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

Add `--diff` to one or more fix-class flags to preview the complete validated,
formatter-normalized result without writing:

```sh
glippy lint --fix --diff ./...
glippy lint --fix-suggestions --diff ./...
glippy lint --fix --fix-suggestions --diff ./...
```

Preview uses the same selection, stale-source, conflict, parsing, formatting,
changed-line ownership, and post-fix analysis boundaries as replacement. Typed
package previews accumulate accepted candidate bytes in an in-memory overlay,
so every later file is reselected and validated against earlier previewed
changes. Changed files are emitted as deterministic three-context unified
diffs from `<path>.orig` to `<path>` in canonical path order. Rejected fixes
retain their ordinary text diagnostics. Unchanged files are silent. Exit 1
means validated changes would occur; conflicts and failures retain their
ordinary exit categories.

`--diff` requires at least one fix-class flag, accepts only the text reporter,
and cannot be combined with baseline generation. It never replaces source,
changes permissions, or updates modification times.

The flags are independently composable. Enabling unsafe fixes does not enable
safe or suggestion fixes. Glippy refuses ambiguous alternatives, stale source,
overlapping edits, generated files, and symlink-traversing write targets.
Accepted edits are reparsed, formatted, reanalyzed, and validated before one
file is replaced. Fix transactions are single-file and do not claim multi-file
atomicity.

Lint accepts `--config=<path>` and
`--reporter=text|short|json|github|sarif`. Text is the default and renders
bounded physical-source frames with primary underlines. `short` retains one
source-free location line plus notes, help, and fix names for log-oriented
human use. JSON uses Glippy's schema-version-1 envelope; GitHub emits
workflow-command annotations; SARIF emits SARIF 2.1.0. Machine reporters omit
source snippets and replacement text.
Combined `check --reporter=json` uses schema version 2; other diagnostic JSON
uses schema version 1. See the [machine output reference](machine-output.md) for
field, range, completeness, ordering, and fix-provenance semantics.

### Execution statistics

`glippy lint --stats` and `glippy check --stats` append a human statistics
report to standard error after a complete analysis. `--stats=json` emits the
schema-version-1 statistics document instead. The selected diagnostic reporter
continues to own standard output, including when both documents use JSON:

```sh
glippy lint --reporter=json --stats=json ./... \
	> diagnostics.json 2> statistics.json
```

Statistics include package-loading and analysis cost, process-local
allocations, selected tiers and their rule reasons, rule callback cost and
finding dispositions, cache hits, misses, semantic invalidations and writes,
and the exact rules that required dependency syntax or effect facts. Timings
and allocation counts are observations rather than deterministic values; the
schema, ordering, identities, and counts derived from the analyzed result are
deterministic for the same inputs.

Allocation counts exclude the Go command and other package-loading
subprocesses. Cache invalidations count entries found under the current key but
rejected by semantic validation; ordinary key changes are misses. Failed or
canceled incomplete runs do not emit a statistics document.

Stats are non-mutating and may be combined with diagnostic reporters,
lint-level directives, `--only`, `--except`, baselines, suppressions, and
`--new-from`. Lint stats cannot be combined with fix flags, fix previews, or
baseline generation because those modes perform repeated or writing analysis
whose costs would need a separate transaction profile.

### Command-line lint levels

`lint` and `check` accept ordered Clippy-style diagnostic policy:

| Short | Long | Result |
| --- | --- | --- |
| `-A <target>` or `-A<target>` | `--allow=<target>` | Disable matching rules |
| `-W <target>` or `-W<target>` | `--warn=<target>` | Enable matching rules as warnings |
| `-D <target>` or `-D<target>` | `--deny=<target>` | Enable matching rules as errors |
| `-F <target>` or `-F<target>` | `--forbid=<target>` | Enable matching rules as errors and prevent later lowering |

A target is an exact rule ID, one of `correctness`, `suspicious`,
`performance`, `complexity`, `style`, or `pedantic`, or the special target
`warnings`. Comma-separated targets are accepted without whitespace.
`restriction` must use exact rule IDs, and `migration` remains unavailable
without an explicit migration target.

Directives apply in command-line order after configured presets, global
per-rule overrides, and matching path overrides. `--only` determines the
eligible rule set first, lint-level
directives update that set, `--except` remains an absolute exclusion, and
configured warning escalation runs last. `warnings` changes only rules that
are warnings at that point and never enables disabled rules. Lowering a rule
after it has been forbidden is an invalid invocation instead of a silent
override. The resolved severity is shared by text, JSON, GitHub, SARIF,
baseline generation, fix planning, and combined check.

The opt-in `complexity` group currently provides `excessive-nesting`,
`too-many-lines`, `too-many-parameters`, and `too-many-results`. Every rule has
one bounded `maximum` integer option and excludes `_test.go` files unless its
`include-tests` option is enabled:

```toml
[lint]
presets = ["correctness", "complexity"]

[lint.rule-options."excessive-nesting"]
maximum = 5

[lint.rule-options."too-many-lines"]
maximum = 100
include-tests = true
```

Complexity findings are advisory and never enabled by the default correctness
policy. None currently offers a fix because splitting functions or changing an
API requires design judgment.

`--only=<id[,id...]>` restricts the resolved project policy to exact rule IDs.
It temporarily re-enables a configured-off rule at its metadata default
severity, or at warning severity when the rule is disabled by default.
`--except=<id[,id...]>` removes exact rule IDs after
`--only`; exclusion wins when an ID appears in both filters. Unknown,
duplicate, empty, or whitespace-padded IDs are invalid. Both filters apply to
ordinary diagnostics, baselines, and every enabled fix class.

`--generate-baseline=<path>` writes a strict source-free JSON adoption
baseline relative to one project root. It analyzes visible diagnostics before
baseline application and cannot be combined with fix flags or a non-text
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
glippy check --reporter=short ./...
glippy check --reporter=json ./...
glippy check --reporter=github ./...
glippy check --reporter=sarif ./...
```

`check` is the non-mutating CI entry point. It compares canonical formatting
and runs every enabled lint tier over the same immutable source identity. Exit
code 1 means formatting differences or lint findings; tool and source failures
use distinct nonzero categories. Text output is buffered until the invocation
has a complete result, while machine reporters preserve their documented
completion or failure signal.

`check` accepts `--config=<path>` and
`--reporter=text|short|json|github|sarif` and defaults to the current directory.
It accepts the same ordered lint-level directives as `lint`; they affect only
the lint half of the combined non-mutating result.

With `--new-from`, formatting differences are actionable only when the full
formatter transformation is owned by changed lines. A difference touching an
unchanged line is reported in JSON as `format_status: "preexisting"` and does
not change the exit status. Lint diagnostics use the same changed-line filter.

## Rule Documentation

```sh
glippy rules
glippy rules --preset pedantic --fixable
glippy rules --tier ssa
glippy explain duplicate-condition
glippy explain duplicate-condition --json
```

`rules` lists rule IDs in deterministic order with default severity, preset
membership, exact analysis tier, and fix safety. Filters compose by metadata;
accepted tier values are `lexical`, `syntax`, `types`, `cfg`, and `ssa`.

`explain` accepts exactly one registered rule ID and an optional `--json` flag
before or after the ID. Its text and schema-version-1 JSON derive from the same
immutable metadata used for rule registration and scheduling. Neither command
performs project discovery or configuration loading. The
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

## LSP

```sh
glippy lsp
glippy lsp --fix-suggestions
glippy lsp --fix-suggestions --fix-unsafe --config=/project/.glippy.toml
```

The server uses LSP 3.17-compatible `Content-Length` framing over standard
input/output, accepts absolute local `file:` URIs, and advertises full-document
synchronization, document formatting, and code actions. Opening or replacing a
buffer publishes diagnostics for that exact document version. One editor event
captures all open Go buffers under the workspace root, uses them together as
typed package overlays, batches compatible same-package documents into one
package load, and republishes dependent open-document diagnostics. Analysis is
asynchronous; rapid replacements debounce, cancel older work, and publish only
the latest complete snapshot. Closing a document removes its overlay and clears
its diagnostics. Typed, CFG, and SSA selections also participate in the
configured persistent cache. Rule diagnostics include a canonical documentation
link matching `glippy explain`.

The retained typed session reparses and re-typechecks compatible changes in the
selected package without a complete package load. A changed active dependency
in the main module, an active workspace module, or a local filesystem
replacement also rechecks its retained reverse dependency closure and refreshes
selected-module effect facts. Imports added by a changed local dependency reuse
a compatible retained package or use a bounded exact types load before that
recheck. Newly loaded mutable local layers preserve compatible retained
transitive type identities. Source or build-selection changes, project-control
changes, cgo-generated inputs, unresolved dependency imports, parse or type
failure, and ambiguous or immutable dependency state fall back to the complete
loader.

Individual safe actions and `source.fixAll.glippy` are available by default.
Suggestion actions require `--fix-suggestions`; unsafe actions require
`--fix-unsafe`. Every action returns one version-bound whole-document edit only
after source identity, edit conflicts, parsing, canonical formatting, and
syntax or typed reanalysis against the same open-buffer snapshot succeed. The
server never writes the document or any project source. Request cancellation
returns the LSP request-canceled error and cannot publish the canceled result.
Code actions received during analysis wait for that exact snapshot. A later
document change rejects the queued request with LSP `ContentModified`.

Malformed source, configuration, package, or analysis state is published as a
Glippy diagnostic for the current version. Incremental edits, stale document
versions, non-file URIs, oversized messages, ambiguous framing, and invalid
UTF-16 ranges are refused. See the [editor guide](editor-integration.md).

## Configuration

Glippy discovers `.glippy.toml` at the selected project boundary unless `--config`
names one exact file. Configuration requires `version = 1`; unknown fields,
unknown rules, duplicate semantic keys, and invalid values fail rather than
being ignored. See the [configuration contract](spec/configuration.md) for the
current schema and discovery policy.

Create a conservative starter policy in the current or selected directory:

```sh
glippy init
glippy init --profile=recommended
glippy init --profile=strict ./module
glippy init ./module
```

Initialization uses exclusive atomic creation with mode `0600`. An existing
regular file or symlink is a conflict and remains unchanged. Profiles are
`default`, `recommended`, `strict`, and `pedantic`; omission selects `default`.

Validate the policy selected for a path, or one exact configuration file:

```sh
glippy config check .
glippy config check --config ./policy/glippy.toml .
```

Explain the same effective policy:

```sh
glippy config show .
glippy config show --config ./policy/glippy.toml ./module
```

The deterministic text output identifies the project root, selection origin,
source language, formatter widths, profile, presets, warning escalation,
enabled rules and their enablement reasons, resolved options, maximum analysis
tier, file and type-error policies, base build selection, canonical analysis
targets, project semantic contract files and effects, baseline status,
suppression policy, and cache limits. The migration target is reported as unset
until migration rules have an explicit target contract.

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
