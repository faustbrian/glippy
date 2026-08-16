# Machine Output Reference

Glippy exposes schema-version-1 JSON for path-based formatter checks and writes,
lint checks and fixes, the combined non-mutating check, and canonical rule
explanations. Select a reporter for path commands or request rule metadata
directly:

```sh
glippy fmt --check --reporter=json ./...
glippy fmt --write --reporter=json ./...
glippy lint --reporter=json ./...
glippy lint --fix --reporter=json ./...
glippy check --reporter=json ./...
glippy explain duplicate-condition --json
glippy lint --reporter=json --stats=json ./...
```

JSON is unavailable when standard output belongs to formatted source or a
unified diff. Consumers must use the schema version and symbolic outcome
category instead of parsing human output or inferring meaning from an exit code
alone.

## Execution Statistics

`lint --stats=json` and `check --stats=json` write a separate
schema-version-1 document to standard error after a complete run. The selected
diagnostic report remains on standard output and does not gain timing fields.
This keeps nondeterministic measurements out of ordinary diagnostic contracts
and permits independent capture when diagnostics also use JSON.

The statistics document contains:

- `command`, `outcome`, `complete`, `maximum_tier`, analyzed-file,
  loaded-source, and package counts;
- a process-local `total` metric;
- ordered `package-loading` and `analysis` phases;
- ordered tiers with the exact rule IDs that required each representation;
- ordered per-rule callback cost plus findings, visible diagnostics,
  pre-existing findings, suppressions, and baselines;
- cache lookups, hits, misses, semantic invalidations, and writes; and
- dependency-syntax and effect-fact load decisions with rule reasons.

Every cost metric contains `calls`, `duration_ns`, `allocations`, and
`allocated_bytes`. Durations and allocations are observations and therefore
not byte-deterministic. Allocation values cover the Glippy process and exclude
Go-tool subprocesses used by package loading. Cache invalidation means a value
was found for the current key but rejected by semantic validation; a changed
key is an ordinary miss.

Arrays and rule identities are canonical. Disposition counts use the final
suppression, baseline, and changed-code result. A cache hit can therefore show
zero rule callbacks while retaining selected rule entries and findings restored
from the cache. `complete` is always true because incomplete, failed, or
canceled runs do not emit this separate document. `outcome` retains the same
symbolic category and numeric exit code as the diagnostic report.

Lint and combined check additionally accept CI-native reporters:

```sh
glippy lint --reporter=github ./...
glippy check --reporter=sarif ./...
```

## GitHub Workflow Annotations

`--reporter=github` writes one GitHub workflow command per formatting finding,
lint diagnostic, suppression or baseline problem, package/source problem, or
tool failure. Clean runs are silent. Properties and messages use GitHub's
required percent escaping. File properties are normalized absolute paths;
locations are physical 1-based lines and UTF-8 byte columns. End positions are
the physical position of the diagnostic's exclusive byte end.

## SARIF 2.1.0

`--reporter=sarif` always writes one deterministic SARIF 2.1.0 log using
`https://json.schemastore.org/sarif-2.1.0.json`. Rule descriptors derive from
the canonical compiled registry where available. Artifact locations are
absolute `file:` URIs and regions use physical 1-based lines and UTF-8 byte
columns. Tool failures set `executionSuccessful` to `false` and appear only as
tool-execution notifications.

Both integration reporters consume the same diagnostics after suppressions,
baselines, and `--new-from` filtering. Pre-existing formatting differences are
omitted. Neither reporter includes source snippets or fix replacement text.
Ordering is stable across repeated runs. Their process exit codes retain the
same categories documented below; unlike Glippy JSON, these formats do not
embed Glippy's numeric outcome category.

## Rule Explanation

`glippy explain <rule> --json` returns canonical compiled metadata without
project or configuration discovery:

```json
{
  "schema_version": 1,
  "command": "explain",
  "rule": {
    "id": "duplicate-condition",
    "summary": "...",
    "documentation": "...",
    "default_severity": "warn",
    "presets": ["correctness"],
    "minimum_go_version": "1.25",
    "analysis_tier": "syntax",
    "node_interests": ["if-stmt"],
    "requires_dependency_syntax": false,
    "requires_effect_facts": false,
    "run_on_generated": false,
    "run_despite_type_errors": false,
    "categories": ["correctness"],
    "fixes": [],
    "options": [],
    "known_limitations": ["..."],
    "examples": []
  }
}
```

Machine tier names are `lexical`, `syntax`, `types`, `control-flow`, and
`ssa`. Option defaults are deterministic textual values using the same typed
metadata as configuration validation. Integer options include numeric
`minimum` and `maximum` fields when the corresponding inclusive bound is
declared. Deprecation appears only when present;
all collection fields are arrays rather than `null`.

## Common Envelope

Every report has this top-level shape:

```json
{
  "schema_version": 1,
  "command": "fmt",
  "mode": "check",
  "outcome": {
    "category": "success",
    "exit_code": 0
  },
  "summary": {
    "files": 1,
    "changed": 0,
    "complete": true
  },
  "files": [],
  "errors": []
}
```

`command` is `fmt`, `lint`, or `check`. `mode` identifies the selected
operation: formatter reports use `check` or `write`; lint reports use `check`
or `fix`; combined reports use `check`. An invalid invocation that requested
JSON uses `invalid` where the command can identify that mode safely.

`summary.complete` says whether Glippy completed the selected work and constructed
the report from every result available to that invocation. It is not a success
flag. A complete report may contain findings or package prerequisite errors.
An incomplete report may retain files processed before cancellation or
failure; consumers must not interpret absent files as clean.

`errors` contains tool-level failures as objects with one `message` field.
Syntax, type, suppression, and lint diagnostics use their dedicated channels
instead of being flattened into this array.

### Outcomes

| Exit code | `category` | Meaning |
| ---: | --- | --- |
| 0 | `success` | No actionable findings |
| 1 | `findings` | Formatting differences or lint findings |
| 2 | `source_error` | Parse, source-version, or required package/type failure |
| 3 | `invalid_invocation` | Invalid invocation or configuration |
| 4 | `conflict` | Stale or conflicting write/fix transaction |
| 5 | `filesystem_error` | Discovery, read, write, replacement, or output failure |
| 6 | `internal_error` | Internal invariant or unexpected tool failure |
| 130 | `canceled` | Cancellation or deadline expiry |

The process exit status and `outcome.exit_code` describe the same category.
Cancellation terminates the incomplete invocation rather than participating in
the ordinary finding-severity order.

## Source Identity And Ranges

Lint and combined-check records identify source with:

- a lexically normalized absolute `path`; and
- a lowercase hexadecimal SHA-256 `source_digest` of the exact analyzed bytes.

Ranges are half-open physical UTF-8 byte offsets: `start` is included and `end`
is excluded. They refer to the bytes identified by `path` and `source_digest`,
not logical positions changed by `//line` directives. They are not character,
code-point, terminal-cell, or UTF-16 offsets.

```json
{
  "path": "/project/source.go",
  "source_digest": "4d88483e1fb88f2e2ab55c5d63f6f9f054d37349c68084e472b9a09bf14f28a9",
  "range": {
    "start": 26,
    "end": 34
  }
}
```

The formatter report does not expose source digests because it reports only
file transaction state. Lint fix provenance always uses the original analyzed
source digest. A fix-mode file may also contain `result_digest`, which identifies
the result represented by the remaining diagnostics. For `fixed` and
`possibly_fixed`, it is the validated post-format result; otherwise it matches
the unchanged source identity.

## Formatter Reports

Formatter summaries contain:

| Field | Meaning |
| --- | --- |
| `files` | Number of selected files |
| `changed` | Formatting differences in check mode, or confirmed replacements in write mode |
| `complete` | Whether every selected result was completed |

Each `files` entry has `path` and `status`. The status is one of:

| Status | Meaning |
| --- | --- |
| `pending` | Selected but not reached before an incomplete outcome |
| `unchanged` | Canonical bytes already matched |
| `different` | Check mode found a formatting difference |
| `formatted` | Write mode confirmed replacement |
| `conflict` | Source identity changed before replacement |
| `failed` | Replacement failed and disk state is known not to be the validated result |
| `possibly_formatted` | Rename may have completed, but final disk state could not be confirmed |

`possibly_formatted` is deliberately not success. The user or automation must
read the file again before retrying or making a claim about disk state.

```json
{
  "schema_version": 1,
  "command": "fmt",
  "mode": "check",
  "outcome": {
    "category": "findings",
    "exit_code": 1
  },
  "summary": {
    "files": 1,
    "changed": 1,
    "complete": true
  },
  "files": [
    {
      "path": "/project/source.go",
      "status": "different"
    }
  ],
  "errors": []
}
```

## Lint Reports

Lint summaries always contain `files`, `diagnostics`, `suppressed`,
`baselined`, `baseline_problems`, `suppression_problems`,
`unused_suppressions`, and `complete`. Typed runs add
`package_diagnostics` and `source_problems` when nonzero. Fix runs add
`fixed_files`, `applied_fixes`, and `rejected_fixes` when nonzero.
Fix runs also add deterministic `import_changes` when accepted fixes require
coordinator-owned import coordination. Each entry carries the source identity,
an `add` or `remove` action, import path, and local name. Additions come only
from exact requirements declared by accepted fixes; removals cover imports made
unused by those fixes. This does not make the formatter a general import
organizer.
Changed-code runs add `preexisting_diagnostics` when full analysis found
diagnostics outside the lines selected by `--new-from`; those diagnostics are
not emitted and do not affect the outcome.

Each lint file has `path`, `source_digest`, and `status`. Fix results may add
`result_digest`.

| Status | Meaning |
| --- | --- |
| `analyzed` | Non-writing analysis completed |
| `pending` | Selected but not reached before an incomplete fix run |
| `unchanged` | No selected safe fix changed the file |
| `fixed` | Replacement was confirmed |
| `conflict` | A stale source or incompatible edit prevented the transaction |
| `failed` | Validation or replacement failed with known disk state |
| `possibly_fixed` | Rename may have completed, but final disk state could not be confirmed |

### Diagnostics

An ordinary diagnostic contains:

```json
{
  "rule_id": "call-rule",
  "severity": "error",
  "targets": ["darwin/arm64", "linux/amd64"],
  "message_key": "call",
  "message": "call requires review",
  "path": "/project/source.go",
  "source_digest": "4d88483e1fb88f2e2ab55c5d63f6f9f054d37349c68084e472b9a09bf14f28a9",
  "range": {
    "start": 26,
    "end": 34
  },
  "related": [
    {
      "range": {
        "start": 15,
        "end": 20
      },
      "message": "owning function"
    }
  ],
  "notes": [
    "review the result"
  ],
  "help": "replace the target",
  "fixes": [
    {
      "name": "rewrite",
      "safety": "safe"
    }
  ],
  "withheld_fixes": [
    {
      "name": "rewrite-with-comments",
      "reason": "comments",
      "message": "rewriting this call would remove comments"
    }
  ]
}
```

`severity` is `warn` or `error` for an emitted diagnostic. Fix safety is
`safe`, `suggestion`, or `unsafe`. The report intentionally omits source
snippets, edit replacement text, and suppressed diagnostic bodies.
`targets` is omitted for ordinary single-selection and syntax-only analysis.
For an explicit analysis matrix it contains the strictly sorted target
identities on which the exact diagnostic occurred. Identical cross-target
diagnostics appear once with the union of identities.
`withheld_fixes` is omitted when empty. It identifies a fix declared by the
rule but not offered for this exact source and currently uses the stable
`comments` reason when the transformation would discard comment ownership.
This is distinct from `rejected_fixes`: a withheld fix never entered transaction
selection, while a rejected fix was selected and then refused by source,
safety, conflict, or validation checks. The additive field remains in schema
version 1 because version-1 consumers are required to ignore unknown fields.

`suppression_problems` are distinct records with `kind`, source identity,
`range`, and `message`. Schema version 1 defines these kinds:

```text
malformed
unknown-rule
missing-reason
invalid-expiry
expired
misplaced-file-scope
unmatched-range-end
nested-range
unclosed-range
invalid-configuration
```

`unused_suppressions` records `rule_id`, `scope`, source identity, directive
`range`, owned `target`, `reason`, and optional `expires_on`. `scope` is `line`,
`next-line`, `range`, or `file`. See the [suppression reference](suppressions.md)
for ownership semantics.

`baselined` counts diagnostics hidden by the configured adoption baseline.
`baseline_problems` contains stale or expired entries with `kind`, portable
`path`, `rule_id`, `message_key`, `source_fingerprint`, original `count`, and
unmatched `remaining`; optional `reason` and `expires_on` preserve reviewed
metadata. Baseline problems are actionable findings. Baseline files use the
separate strict format documented in the [baseline reference](baselines.md).

### Typed Prerequisite Channels

Typed, CFG, and SSA runs keep package-loader diagnostics and captured-source
problems separate from rule diagnostics:

```json
{
  "package_diagnostics": [
    {
      "package_id": "example.com/project",
      "targets": ["linux/amd64"],
      "kind": "type",
      "position": "/project/source.go:7:3",
      "message": "undefined: value"
    }
  ],
  "source_problems": [
    {
      "path": "/project/generated.go",
      "source_digest": "4d88483e1fb88f2e2ab55c5d63f6f9f054d37349c68084e472b9a09bf14f28a9",
      "targets": ["linux/amd64"],
      "message": "captured source did not parse"
    }
  ]
}
```

Package `kind` is `unknown`, `list`, `parse`, or `type`. `package_id` is an
opaque Go package-loader identity. `position` is omitted when the upstream
diagnostic has no position. Source problems identify the exact captured bytes
that failed to enter the ordinary analyzed-file channel. Matrix package and
source problems use the same sorted, omitted-when-empty `targets` contract as
rule diagnostics.

GitHub annotations append `[target,...]` to the annotation title. SARIF results
store the same array in `properties.targets`. Target identity never changes a
rule ID or source range.

### Fix Provenance

`applied_fixes` records `rule_id`, `fix_name`, original source identity, and the
primary diagnostic `range`. `rejected_fixes` adds a stable `reason` and human
`message`. Reasons in schema version 1 are:

```text
missing-fix
stale-source
suggestion-not-selected
unsafe
invalid-safety
invalid-range
invalid-text
conflict
validation
```

A fix record proves selection and transaction disposition; it does not expose
the edits. `possibly_fixed` has the same uncertainty rule as
`possibly_formatted` and requires a fresh disk read before retry.

## Combined Check Reports

The combined `check` report reuses lint diagnostics, suppression records, and
typed prerequisite channels. Its summary replaces lint fix counts with
`formatting_differences`, and each file has `path`, `source_digest`, and
`format_status`:

```json
{
  "schema_version": 1,
  "command": "check",
  "mode": "check",
  "outcome": {
    "category": "findings",
    "exit_code": 1
  },
  "summary": {
    "files": 1,
    "formatting_differences": 1,
    "diagnostics": 0,
    "suppressed": 0,
    "baselined": 0,
    "baseline_problems": 0,
    "suppression_problems": 0,
    "unused_suppressions": 0,
    "complete": true
  },
  "files": [
    {
      "path": "/project/source.go",
      "source_digest": "4d88483e1fb88f2e2ab55c5d63f6f9f054d37349c68084e472b9a09bf14f28a9",
      "format_status": "different"
    }
  ],
  "diagnostics": [],
  "suppression_problems": [],
  "unused_suppressions": [],
  "baseline_problems": [],
  "errors": []
}
```

`format_status` is `unchanged`, `different`, or `preexisting`. The last value
is used only with `--new-from` when canonical formatting differs but the full
transformation touches a line outside changed-code ownership. Such a file
increments optional `preexisting_formatting_differences`, not
`formatting_differences`, and does not create a finding. Formatting and analysis are
bound to the same source path and digest; a mismatch is an internal error, not
a report Glippy silently combines.

## Determinism And Compatibility

Glippy orders files by normalized path. Diagnostics, suppression records, package
prerequisites, source problems, and fix dispositions use stable source- and
identity-based ordering within their channels. Object field order is stable in
the current encoder, but consumers should bind fields by name rather than use
textual JSON comparison.

Consumers should ignore unknown fields within schema version 1. They must reject
an unsupported `schema_version` instead of guessing at field meaning. The
[compatibility policy](compatibility-policy.md#machine-output-and-exit-codes)
defines which additions can retain a schema version and which changes require a
new one. The [machine-reporting decision](decisions/0011-machine-reporting-schema.md)
records the architectural boundary behind this public reference.
