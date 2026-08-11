# Configuration Contract Draft

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Format And Version

The Phase 0 candidate is `.gox.toml`, renamed with the product if ADR 0001
changes. It MUST contain `version = 1`. Unknown keys, duplicate semantic keys,
unknown rule IDs, and invalid values MUST fail with a source-located diagnostic
where the TOML decoder permits it.

```toml
version = 1

[format]
line-width = 100
tab-width = 8

[lint]
preset = "correctness"

[lint.suppressions]
require-reason = false

[lint.rules]
no-unexplained-suppression = "error"
```

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

## Discovery And Precedence

Without `--config`, Gox discovers one project configuration by walking upward
from each input to the nearest module, workspace, or repository root and then
selecting the nearest configuration at or above that boundary. Inputs that
resolve to different project configurations form separate deterministic runs.
Phase 0 does not provide implicit parent merging or nested configuration.

An explicit `--config` selects exactly that file and disables discovery.
Precedence is built-in defaults, selected configuration, matching typed path
override when introduced, then explicit command-line options. Environment
variables MUST NOT silently override formatter or lint policy; GOOS, GOARCH,
build tags, toolchain, module/workspace state, and cgo selection are explicit
analysis inputs and cache keys.

Nested configuration or inheritance MAY be introduced only after a concrete
multi-root journey demonstrates that typed path overrides are insufficient.
Any such addition requires cycle handling, boundary rules, and a migration
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
eligibility, but no rule may construct its own discovery policy.
