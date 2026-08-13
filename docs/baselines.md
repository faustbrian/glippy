# Lint Baselines

Lint baselines support incremental adoption without weakening the policy for
new or changed code. Generate one deterministic document for a single project
root and configuration:

```sh
gox lint --generate-baseline=.gox-baseline.json ./...
```

Generation analyzes normally visible, unsuppressed diagnostics, writes the
baseline with same-directory atomic filesystem semantics, and exits
successfully when the document is written. It cannot be combined with fix
flags or the JSON reporter. The configured path is portable and relative to
the discovered project root.

Enable the document explicitly:

```toml
[lint.baseline]
path = ".gox-baseline.json"
report-stale = true
expiry-cutoff = "2026-08-13"
```

Each entry identifies an exact rule ID and message key, a slash-separated
project-relative path, a lowercase SHA-256 fingerprint of the diagnostic's
exact source span, and an occurrence count. The document never stores source
snippets. Entries may carry a reason and a `YYYY-MM-DD` expiry date.

Gox applies ordinary source suppressions before the baseline. Matching
diagnostics are counted as baselined, are omitted from visible diagnostics,
and are never selected by `--fix`. A changed source span no longer matches.
When `report-stale` is true, unmatched counts in files analyzed by the current
invocation are findings so teams can shrink the document. Entries for files
outside the current selection are not reported as stale.

Expiry is deterministic and never reads the wall clock. When
`expiry-cutoff` is configured, entries expiring on or before that date stop
hiding diagnostics and produce an expired baseline finding. Advancing the
cutoff is an explicit repository policy change.

The JSON schema is strict and versioned independently as
`schema_version: 1`. Unknown fields, unknown rules, duplicate entries,
non-portable paths, invalid dates, invalid fingerprints, and non-positive
counts fail. Canonical encoding sorts entries by path, rule, message key, and
fingerprint and ends with one newline.
