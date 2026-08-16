# Configuration Contract

This document defines the implemented pre-release schema version 1. Public
compatibility follows the [compatibility policy](../compatibility-policy.md);
unknown schema versions and fields fail rather than receiving guessed meaning.

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Format And Version

The canonical configuration file is `.glippy.toml` under ADR 0016. It MUST
contain `version = 1`. Unknown keys, duplicate semantic
keys, unknown rule IDs, and invalid values MUST fail with a source-located
diagnostic where the TOML decoder permits it.

During the v0.2 compatibility window, automatic discovery accepts `.gox.toml`
only when no `.glippy.toml` exists in the searched project scope. Finding both
names fails rather than choosing one. An explicit `--config=<path>` remains an
exact selection and bypasses automatic-name ambiguity.

`glippy init [directory]` creates this canonical starter policy through
exclusive atomic creation and refuses to replace any existing file or symlink:

```toml
version = 1

[format]
line-width = 100
tab-width = 8

[lint]
presets = ["correctness"]
warnings-as-errors = false
```

`glippy config check [path]` validates the complete selected schema without
running analysis. `glippy config show [path]` renders the fully defaulted rule
selection and its provenance, source language, analysis tier and build inputs,
file policies, baseline status, suppression policy, and cache limits. Both
commands accept `--config=<path>` for exact selection. The reported migration
target remains unset until a migration rule target is part of the schema.

```toml
version = 1

[format]
line-width = 100
tab-width = 8

[analysis]
build-tags = []
goos = "linux"
goarch = "amd64"
cgo-enabled = true

[lint]
presets = ["correctness"]
warnings-as-errors = false

[lint.suppressions]
require-reason = false

[lint.baseline]
path = ".glippy-baseline.json"
report-stale = true
expiry-cutoff = "2026-08-13"

[lint.rules]
no-unexplained-suppression = "error"

[lint.rule-options."no-unexplained-suppression"]
require-ticket = true

[cache]
enabled = false
max-entries = 4096
max-bytes = 536870912
```

`lint.presets` MUST be an order-independent list of unique preset groups. Glippy
MUST canonicalize configured groups in this order: `correctness`, `suspicious`,
`performance`, `complexity`, `style`, then `pedantic`. Omission defaults to
`["correctness"]`; an explicitly empty list selects no group and permits only
rules enabled through `lint.rules`. A rule belonging to any selected group is
enabled once at its metadata severity before explicit rule overrides apply.

`lint.preset` remains a compatibility alias for selecting one group. A
configuration MUST NOT specify both singular and plural fields. A `restriction`
rule MUST be enabled only through its exact ID in `lint.rules`; the restriction
group MUST NOT be selected wholesale. The `migration` group remains unavailable
until configuration supplies an explicit migration target. Unknown groups,
duplicates, wholesale `restriction`, and untargeted `migration` MUST fail.

`lint.warnings-as-errors` MUST default to `false`. When `true`, Glippy MUST
escalate every enabled rule whose final severity after group selection and
per-rule overrides is `warn` to `error`. It MUST NOT enable an `off` rule or
alter an existing `error`. The resolved escalation policy and normalized group
set MUST contribute to configuration and cache identity.

`lint.rule-options` MUST use one rule-ID table per configured rule. The
documented canonical spelling quotes the rule ID even where TOML also permits a
bare key.
Every field MUST be declared by that rule's canonical metadata as a boolean,
integer, string, or string list. Unknown rule IDs, unknown fields, and values
of the wrong type MUST fail configuration loading. A required option MUST be
present whenever its rule is enabled and MUST NOT declare a default. Every
optional option MUST declare one canonical metadata default of the same type.
Integer options MAY declare inclusive minimum and maximum bounds. Bounds MUST
apply only to integer options, the minimum MUST NOT exceed the maximum, and any
default or configured value outside the declared range MUST fail before source
discovery. The bounds MUST appear in human and machine rule documentation.
Options for disabled rules MAY remain in configuration so preset changes do not
require destructive edits. Each run MUST bind one immutable option snapshot,
including resolved defaults, to every callback for that rule across syntax,
types, CFG, and SSA execution.

`lint.overrides` MUST be an ordered array of path-scoped exact rule-severity
overrides. Every entry MUST contain a non-empty `paths` array and non-empty
`rules` table:

```toml
[[lint.overrides]]
paths = ["**/*_test.go", "testdata/**"]

[lint.overrides.rules]
discarded-error = "off"
blank-error-discard = "warn"
```

Patterns MUST be normalized, project-relative `/`-separated globs. `*`, `?`,
and character classes match within one segment; a complete `**` segment
matches zero or more segments. Empty segments, `.`, `..`, absolute paths,
backslashes, invalid character classes, and duplicate patterns in one entry
MUST fail. Pattern order within one entry is irrelevant and MUST be
canonicalized; override entry order is significant. Every matching entry MUST
apply in declaration order, and a later entry MUST replace an earlier severity
for the same exact rule ID. Unknown rules, invalid severities, empty entries,
and unknown fields MUST fail configuration loading.

Path overrides control rule severity only. Rule options, presets, warning
escalation, suppressions, baselines, build selection, formatter policy, and
cache policy remain project-wide. A path override MAY enable a rule that is
globally off. The scheduler MUST plan the union of every potentially enabled
rule so a path-only typed, CFG, or SSA rule receives its required shared
representation. After analysis, each physical file MUST retain only its exact
resolved rules and severities before suppressions, baselines, reporters, or
fix selection consume diagnostics. The ordered override policy MUST contribute
to configuration and cache identity.

`lint.suppressions.require-reason` MUST be a boolean and MUST default to
`false`. When it is `true`, every direct suppression and range start MUST carry
a non-empty reason after `--`; a missing or empty reason MUST produce a finding,
and the invalid directive MUST NOT suppress diagnostics. The policy MUST apply
equally to syntax-only and package-aware lint, combined check, and syntax fix
planning. Range ends remain reasonless because the matching start owns the
waiver.

`lint.suppressions.expiry-cutoff` MAY be omitted. When present, it MUST be a
quoted, valid `YYYY-MM-DD` calendar date. Glippy MUST NOT read the wall clock to
evaluate suppressions. Instead, a directive whose structured `expires` date is
on or before the configured cutoff MUST be reported as expired and MUST NOT
suppress diagnostics. This explicit input keeps local and CI results
reproducible; advancing the cutoff is a project policy decision.

```toml
[lint.suppressions]
expiry-cutoff = "2026-08-11"
```

`lint.baseline.path` MAY select one portable path relative to the discovered
project root. When omitted, no baseline is loaded. `report-stale` defaults to
`true` and `expiry-cutoff` is optional. The cutoff has the same deterministic
calendar-date contract as suppression expiry and does not read the wall clock.
The baseline path, stale policy, and cutoff contribute to canonical
configuration and cache identity. See the [baseline reference](../baselines.md).

Formatter configuration MUST remain limited to adoption-significant choices.
Brace placement, individual whitespace rules, and alternative layout dialects
MUST NOT be configurable.

Persistent analysis caching MUST default to disabled until a project opts in
with `cache.enabled = true`. `cache.max-entries` and `cache.max-bytes` MUST be
non-negative integers; zero leaves that dimension unlimited, and an enabled
cache MUST retain at least one positive limit. Defaults are 4,096 entries and
512 MiB. These limits govern disposable storage rather than diagnostic or
formatting behavior and MUST NOT contribute to cached-result identity.
Syntax-only linting, formatting, and fix planning MUST NOT open or prune the
typed analysis cache.

`analysis.build-tags` MUST be a list of Go build-tag identifiers. Glippy sorts
and deduplicates the list before package loading and cache identity. The
`analysis.goos` and `analysis.goarch` values MUST use lowercase Go target
identifier syntax, and `analysis.cgo-enabled` MUST be a boolean. When omitted,
the resolved defaults are the binary's runtime `GOOS` and `GOARCH`, no custom
build tags, and the Go build context's cgo default. Package-aware `lint`,
combined `check`, typed fix planning and reselection, and post-fix validation
MUST use this one resolved selection whether persistent caching is disabled or
enabled. `GOOS`, `GOARCH`, and `CGO_ENABLED` environment variables MUST NOT
override it. Package loads set `GOENV=off`; unsupported but syntactically valid
targets remain package-loading errors rather than configuration-schema errors.
All four resolved fields MUST contribute to result configuration identity.

`analysis.targets` MAY define an explicit CI-oriented package-analysis matrix:

```toml
[[analysis.targets]]
goos = "linux"
goarch = "amd64"
tags = ["integration"]

[[analysis.targets]]
goos = "darwin"
goarch = "arm64"
cgo-enabled = true
```

Each entry MUST contain `goos` and `goarch`, MAY contain `tags`, and MAY set
`cgo-enabled`; omission of cgo defaults to `false` for that target. Target tags
have the same syntax as `analysis.build-tags` and MUST be sorted and
deduplicated. Target identities use `GOOS/GOARCH`, followed by `+cgo` when
enabled and `+tags=<comma-separated-tags>` when tags are present. Glippy MUST
sort targets by that identity and MUST reject duplicate identities. A matrix
MUST contain no more than 32 targets.

When the selected rule policy requires package analysis, non-writing `lint`,
combined `check`, and baseline generation MUST analyze every configured target.
Identical diagnostics and prerequisite problems MUST be emitted once with the
sorted union of target identities; target-specific findings MUST remain
distinct. Syntax-only lint remains file-oriented and MUST NOT execute the
matrix. The base `[analysis]` selection remains authoritative for LSP analysis
and typed fix validation. Every fix mode MUST reject a configured matrix before
mutation because one source transaction cannot safely choose among
target-dependent fixes.

`analysis.targets` contributes to canonical configuration identity. Each
individual package load and persistent cache entry MUST additionally bind its
resolved target fields, so results from two matrix entries cannot collide.

## Discovery And Precedence

Without `--config`, Glippy discovers one project configuration by walking upward
from each input to the nearest module, workspace, or repository root and then
selecting the nearest configuration at or above that boundary. Inputs that
resolve to different project configurations form separate deterministic runs.
Schema version 1 does not provide implicit parent merging or nested
configuration.

An explicit `--config` selects exactly that file and disables discovery.
Precedence is built-in defaults, selected configuration, matching ordered path
overrides, then explicit command-line lint levels and warning escalation.
`--except` remains an absolute exclusion and a command-line `forbid` cannot be
lowered. Override paths remain relative to each input's discovered project
root even when one explicit configuration is reused across multiple roots. If
an explicit configuration is used without any project boundary, its directory
is the override root and outside inputs fail instead of receiving guessed path
ownership. Environment
variables MUST NOT silently override formatter or lint policy. Build tags,
GOOS, GOARCH, and cgo selection come from `[analysis]`; toolchain and
module/workspace state remain explicit analysis inputs and cache keys.

`GLIPPY_CACHE_DIR` MAY select the normalized absolute cache root when persistent
analysis caching is enabled. It does not enable caching and MUST NOT override
the configured retention limits. Without the variable, the CLI MUST use the
platform user-cache directory beneath `glippy/analysis`. The store resolves and
validates the prospective target outside the selected project before creation,
then opens each resolved path component through pinned rooted handles so a
later symlink change cannot redirect cache operations into the project.

Nested configuration or inheritance MAY be introduced only after a concrete
multi-root journey demonstrates that typed path overrides are insufficient.
Any such addition MUST define cycle handling, boundary rules, and a migration
decision.

## File Policies

Defaults:

- generated files are readable and diagnosable but not writable;
- vendor, module-cache, and VCS metadata trees are excluded;
- `testdata` and fixtures are included only when explicitly selected or
  matched by a configured path policy;
- symlinks are not followed for recursive discovery and explicit symlink
  writes are refused until race-safe semantics are proven;
- files outside the authorized project root require explicit selection and
  are never reached through traversal; and
- invalid configuration aborts the complete run before any write.

Rules and formatter behavior MAY define test-file or generated-file
eligibility, but a rule MUST NOT construct its own discovery policy.
