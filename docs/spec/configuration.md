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

The configuration file is `.gox.toml`, renamed with the product only if ADR
0001 changes. It MUST contain `version = 1`. Unknown keys, duplicate semantic
keys, unknown rule IDs, and invalid values MUST fail with a source-located
diagnostic where the TOML decoder permits it.

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
preset = "correctness"

[lint.suppressions]
require-reason = false

[lint.rules]
no-unexplained-suppression = "error"

[lint.rule-options."no-unexplained-suppression"]
require-ticket = true

[cache]
enabled = false
max-entries = 4096
max-bytes = 536870912
```

`lint.rule-options` MUST use one rule-ID table per configured rule. The
documented canonical spelling quotes the rule ID even where TOML also permits a
bare key.
Every field MUST be declared by that rule's canonical metadata as a boolean,
integer, string, or string list. Unknown rule IDs, unknown fields, and values
of the wrong type MUST fail configuration loading. A required option MUST be
present whenever its rule is enabled and MUST NOT declare a default. Every
optional option MUST declare one canonical metadata default of the same type.
Options for disabled rules MAY remain in configuration so preset changes do not
require destructive edits. Each run MUST bind one immutable option snapshot,
including resolved defaults, to every callback for that rule across syntax,
types, CFG, and SSA execution.

`lint.suppressions.require-reason` MUST be a boolean and MUST default to
`false`. When it is `true`, every direct suppression and range start MUST carry
a non-empty reason after `--`; a missing or empty reason MUST produce a finding,
and the invalid directive MUST NOT suppress diagnostics. The policy MUST apply
equally to syntax-only and package-aware lint, combined check, and syntax fix
planning. Range ends remain reasonless because the matching start owns the
waiver.

`lint.suppressions.expiry-cutoff` MAY be omitted. When present, it MUST be a
quoted, valid `YYYY-MM-DD` calendar date. Gox MUST NOT read the wall clock to
evaluate suppressions. Instead, a directive whose structured `expires` date is
on or before the configured cutoff MUST be reported as expired and MUST NOT
suppress diagnostics. This explicit input keeps local and CI results
reproducible; advancing the cutoff is a project policy decision.

```toml
[lint.suppressions]
expiry-cutoff = "2026-08-11"
```

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

`analysis.build-tags` MUST be a list of Go build-tag identifiers. Gox sorts
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

## Discovery And Precedence

Without `--config`, Gox discovers one project configuration by walking upward
from each input to the nearest module, workspace, or repository root and then
selecting the nearest configuration at or above that boundary. Inputs that
resolve to different project configurations form separate deterministic runs.
Schema version 1 does not provide implicit parent merging or nested
configuration.

An explicit `--config` selects exactly that file and disables discovery.
Precedence is built-in defaults, selected configuration, matching typed path
override when introduced, then explicit command-line options. Environment
variables MUST NOT silently override formatter or lint policy. Build tags,
GOOS, GOARCH, and cgo selection come from `[analysis]`; toolchain and
module/workspace state remain explicit analysis inputs and cache keys.

`GOX_CACHE_DIR` MAY select the normalized absolute cache root when persistent
analysis caching is enabled. It does not enable caching and MUST NOT override
the configured retention limits. Without the variable, the CLI MUST use the
platform user-cache directory beneath `gox/analysis`. The store resolves and
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
