# CLI Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

The current product and binary name is `glippy`. ADR 0001 requires one final
collision and trademark audit before the first public tag.
The user-facing summary of this normative contract is the
[`command-reference.md`](../command-reference.md) guide.

## Commands

```text
glippy fmt [paths...]
glippy fmt --write [paths...]
glippy fmt --check [paths...]
glippy fmt --diff [paths...]
glippy lint [paths...]
glippy lint --fix [paths...]
glippy lint --new-from=<git-ref> [paths...]
glippy lint --generate-baseline=<path> [paths...]
glippy check [paths...]
glippy check --new-from=<git-ref> [paths...]
glippy explain <rule>
glippy version
glippy completion <bash|zsh|fish>
```

`glippy version` MUST accept no operands or flags and MUST write exactly
`glippy <version>\n` to standard output. Version resolution MUST prefer explicit
link-time release metadata, then a non-development Go module build version, and
finally `devel`. The command MUST NOT read source or configuration files or
modify the filesystem.

`glippy completion <bash|zsh|fish>` MUST accept exactly one supported shell and
write one deterministic completion script to standard output. The script MUST
cover the complete command surface, command-specific flags and enum values,
filesystem operands, supported shells, and every rule ID in the compiled
registry. Completion generation MUST NOT read standard input, project source,
configuration, package state, or the network and MUST NOT modify the
filesystem. Invalid shells and argument counts are invalid invocations;
cancellation and output failures retain the common exit categories.
Installation is documented in
[`shell-completion.md`](../shell-completion.md).

`glippy explain <rule>` MUST accept exactly one rule ID and render the complete
human documentation derived from that rule's immutable compiled metadata. It
MUST NOT discover project files or load configuration. Unknown rule IDs and
invalid argument counts exit as invalid invocation without writing stdout.
Cancellation and output failures retain the common exit categories. The
compiled registry contains only rules that satisfy the admission gate. The
default correctness rules are `duplicate-condition` and
`ineffective-break`; the types-tier `context-key`, CFG-tier
`defer-in-infinite-loop`, types-tier `errors-is-arguments`, and SSA-tier
`nilness` rules remain in the opt-in `suspicious` preset. The types-tier
`redundant-bool-comparison` rule is the first opt-in `style` rule.

`fmt` without a write, check, or diff flag writes formatted content to stdout
for one explicit file or stdin. Multiple filesystem inputs require `--write`,
`--check`, or `--diff`. Those three modes are mutually exclusive. Standard input
MAY use `--stdin-filepath` for language version and configuration context but
MUST NOT make that path writable.

Source-language discovery and the supported Go 1.25 through Go 1.26 range are
defined in [`supported-go-versions.md`](../supported-go-versions.md).
Unsupported or malformed selected language directives are source errors and
must fail before output or mutation.

`fmt --diff` is path-based and non-writing. It MUST emit three-context-line
unified differences for changed files in normalized path order, omit unchanged
files, and use the normalized source path plus `.orig` as the old label and the
source path as the new label. Labels containing tabs or line breaks MUST be
quoted. A changed selection exits with findings; an already formatted selection
exits successfully without output. Matching MUST remain bounded: Glippy uses
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

Every complete source file and physical stdin fragment is limited to
67,108,864 exact bytes as specified in
[`source-model.md`](source-model.md#source-size-boundary). Oversized stdin,
ordinary files, typed-package sources and overlays, and write/fix snapshots
exit as source errors. No mode may emit formatted bytes, partial text findings,
or replace a file after this limit is exceeded. JSON check, lint, and formatter
path modes retain a valid incomplete source-error envelope. An unrelated
reader, stat, open, or transport failure remains a filesystem error.

The supported format-on-save setup and editor failure contract are documented
in [`editor-integration.md`](../editor-integration.md).

The Phase 3 `lint` check accepts explicit files, directories, and filesystem
package patterns ending in `...`; defaults to the current directory; and
accepts `--reporter=text|json` plus an optional explicit configuration path. It
MUST resolve every input's project root and complete configuration before
choosing an analysis path. When the maximum enabled tier is syntax, recursive
patterns use deterministic physical-file discovery and MUST NOT invoke
`go/packages`. When at least one enabled rule requires types, CFG, or SSA, every
input MUST resolve to one project root and configuration; the CLI converts
files, directories, and recursive patterns into one read-only package load
with test variants enabled. Heterogeneous typed roots or configurations MUST
fail as an invalid invocation until a per-path package configuration design is
accepted.

Both analysis paths run enabled syntax rules and never write source. The typed
path additionally runs types-tier, CFG-tier, and SSA-tier rules over the shared
package result. Visible rule diagnostics, suppression problems, and unused
suppressions exit with findings; suppressed diagnostics alone do not. Required
package-list, parse, type, or source-model problems exit with source error even
when valid partial file results remain reportable. A completed typed load with
prerequisite problems remains a complete report rather than an internal tool
failure. Invalid configuration, filesystem failures, cancellation, and reporting failures retain
their common exit categories. JSON remains valid and incomplete for invalid
invocations and failures.

`lint --generate-baseline=<path>` MUST analyze normally visible diagnostics
before baseline application and write one deterministic strict JSON document
relative to one project root. It MUST reject heterogeneous roots,
non-portable output paths, all fix flags, and JSON reporting. It writes only
the baseline, never source, and MUST use the shared rooted atomic writer. The
[baseline reference](../baselines.md) defines identity, stale, expiry, and
machine-reporting behavior.

`lint` and `check` MAY select `--new-from=<git-ref>`. The driver MUST resolve
the containing Git repository and all common ancestors of the named ref and
`HEAD`, sort multiple merge bases by full object identity, and use the first as
the deterministic comparison base. It MUST analyze complete selected files and
packages before filtering. Only diagnostics whose physical byte range
intersects a current changed line are visible; other diagnostics MUST be
counted separately as pre-existing and MUST NOT affect the exit status. Added
and untracked files are wholly changed. Modified renames MUST preserve
line-level ownership when Git recognizes the rename, while pure renames MUST
introduce no changed-line diagnostics. Deleted paths have no current findings.

Changed-code fixes MUST expose a fix only when every edit line is owned. Before
replacement, the driver MUST compare the complete formatter-normalized result
with the analyzed source and reject the transaction if any changed source line
is not owned. `--new-from` MUST NOT combine with baseline generation. Every
selected source MUST remain inside the resolved repository. Missing refs,
repositories, merge bases, or out-of-repository inputs are invalid
invocations. Git execution MUST observe cancellation, MUST NOT enable external
diff drivers or text conversion, MUST NOT execute repository-configured
content filters, MUST ignore inherited repository-selection overrides and
global or system Git configuration, and MUST NOT require network access.

`lint` never writes source unless a fix flag is present. Baseline generation
writes only its explicitly named baseline document. Ordinary `--fix` applies safe
fixes only, `--fix-suggestions` applies suggestion fixes only, and
`--fix-unsafe` applies unsafe fixes only. The flags are independently
composable; unsafe authorization MUST NOT implicitly authorize suggestions or
safe fixes. For each diagnostic, the driver automatically selects only one
enabled named fix. Multiple enabled alternatives violate the rule contract and
fail before any write. The built-in `ineffective-break` rule offers
`remove-break` only as a suggestion because removal preserves current behavior
but may conceal an intended return or labeled loop exit.
The built-in `redundant-bool-comparison` rule offers
`simplify-comparison` as a safe fix only when the retained operand has
predeclared boolean type, an alias resolving directly to it, or untyped boolean
type. Candidates with a retained defined boolean operand are excluded because
the comparison may intentionally normalize the result type. Comparisons whose
removed source contains a comment remain diagnostics without a fix.

Every lint fix mode prevalidates every selected configuration and source before
its first write, refuses generated files and paths traversing symlinks,
coordinates each source version independently, reparses and
formatter-normalizes accepted edits, and then uses the shared atomic replacement
boundary. Syntax-only fixes rerun syntax analysis over the final bytes. Typed,
CFG, or SSA selections rerun the complete package analysis with the formatted
candidate supplied through an exact-path overlay and reject the fix if package
loading, source capture, or target-file recovery fails. Stale files and
overlapping fixes are conflicts. One file's conflict does not silently select a
winner or prevent independent file transactions from being attempted.
Cancellation stops before the next replacement and reports earlier confirmed
writes. A post-format analysis engine failure remains an internal or
cancellation outcome and MUST NOT be downgraded to an ordinary rejected-fix
finding.

Typed package fixing is cache-independent even when persistent analysis caching
is enabled for non-mutating lint. The initial plan and each candidate validation
use fresh package loads in read-only module mode with test variants enabled.
Every package load uses the same resolved `[analysis]` build tags, GOOS,
GOARCH, and cgo selection as non-mutating typed lint and combined check; ambient
target and cgo variables do not select a different package graph. Candidate
overlay validation and final reselection MUST retain that selection.
Package or source-model problems in the initial plan fail with source error
before replacement; the same problems caused by a candidate become a stable
validation rejection and preserve the original file. A final fresh package
analysis supplies every file's reported result so a later write cannot hide a
finding newly enabled in an earlier file. Transactions remain single-file and
serialized; an invocation MUST NOT claim multi-file atomicity.

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
`check` accepts explicit files, directories, and filesystem package patterns
ending in `...`; defaults to the current directory; and accepts
`--reporter=text|json` plus an optional explicit configuration path. It MUST
resolve configuration and the enabled maximum analysis tier before selecting
its input engine. Syntax-only selections use one sorted physical-file
discovery and run formatting plus lint analysis against each immutable file
read. A selection requiring types, CFG, or SSA MUST use the same one-root,
one-configuration package query contract as typed `lint`; formatting and every
analysis tier MUST consume the exact immutable sources captured by that one
package load and MUST NOT reread them. `check` MUST never write source,
permissions, or modification times.

Text output is buffered until the complete selection succeeds. In normalized
path order it emits `path: format differs` before that file's lint records. A
package-aware check emits all ordered formatting differences before its typed
lint records; package prerequisite and source-model records retain the typed
lint text contract. A source, configuration, discovery, analysis, or formatting
tool failure MUST therefore emit no partial text findings. Package prerequisite
or source-model problems are completed source-error results and MUST remain
reportable with valid formatting and lint results. Successful clean checks are
silent.

Combined JSON uses command and mode `check`. Each ordered file record carries
the normalized absolute path, source digest, and format status `unchanged` or
`different`. Diagnostics and suppression records use the lint schema and MUST
carry the same path and digest as their file record. The summary includes file,
formatting-difference, visible diagnostic, suppressed diagnostic, suppression
problem, unused-suppression, package-diagnostic, and source-problem counts plus
completeness. Package-aware JSON MUST expose package diagnostics and
source-model problems in the same stable channels as typed lint. Incomplete
JSON retains results completed before the failure. Invalid invocations
requesting JSON MUST still receive this envelope.

Formatting inputs are file-oriented. Typed lint inputs are package-oriented;
explicit files use exact-file queries, directories select one package, and a
terminal `...` selects packages recursively below its existing filesystem
anchor. The CLI MUST reject malformed patterns, inputs outside the selected
root, and combinations whose package interpretation would require more than one
project root or configuration.

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

Combined-check JSON reuses the lint diagnostic and suppression arrays while
adding `formatting_differences` to the summary and `format_status` to each file.
The format outcome and lint result for a file MUST match the same normalized
path and source digest or report construction fails as an internal invariant.
When package analysis is selected, combined-check JSON also reuses
`package_diagnostics` and `source_problems` with their distinct summary counts.

Lint text reports physical 1-based line and UTF-8 byte-column locations as
`path:line:column: severity[rule-id]: message`, followed by indented related
locations, notes, help, and named fix safety when present. Suppression problems
and unused directives use distinct labels. Text reporting MUST validate exact
source identity and every range associated with a rendered record before
emitting output, and MUST NOT include source excerpts or fix replacement text.

Formatter read, parse, and layout preparation uses at most the smaller of the
selection size, `GOMAXPROCS`, and 8 workers. Task identity is assigned only
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

Typed `lint` and combined `check` MAY use the persistent analysis cache only
when the selected configuration enables it. One invocation MUST reuse one
caller-owned store across the package run, apply the resolved bounded-pruning
policy after every non-canceled run, remove only canonical publication
temporaries strictly older than 24 hours, and close the store before reporting.
Configuration, cache-root, open, prune, and close failures MUST be visible tool
failures; Glippy MUST NOT present a cache-maintenance failure as cached success.
Syntax-only commands and every formatter or fix path MUST remain independent of
the persistent analysis cache. An invalid cache-root policy is an invalid
invocation, cache filesystem failures use the filesystem category, and cache
identity conflicts or invariant failures use the internal-error category.
Cache-disabled and cache-enabled package runs MUST use the same resolved build
selection and explicit Go environment. Persistent cache identity MUST bind
that complete selection without changing which files or packages are loaded.

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
having a particular inode. Glippy revalidates immediately before rename and
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

Glippy MUST advertise runtime support only for macOS and Linux. It MUST advertise
write and fix mode support only for operating-system, architecture, and local
filesystem combinations with runtime replacement evidence. The current Phase 2
evidence covers Darwin arm64 on APFS and Linux arm64 on overlayfs. Windows and
all other operating systems MUST remain unsupported unless a later maintainer
decision changes the platform policy and corresponding runtime evidence passes.

Network, distributed, and userspace filesystems MUST remain outside the
supported write/fix boundary until separately admitted. Glippy MUST NOT claim
forced-power-loss durability. A successful write means the documented sync and
replacement sequence completed under the operating system contract; it does not
extend that contract to storage hardware or unverified filesystem semantics.
