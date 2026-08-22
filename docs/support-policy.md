# Product Support Policy

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Release Support

Gox v0.1.0 is the current stable release. Glippy v0.5 branch heads, commits,
locally built binaries, workflow artifacts, rehearsal archives, and prerelease
plans are development artifacts and are not supported releases. Release
support follows this table:

| Release line | Support state |
| --- | --- |
| Latest stable release | Supported for security and correctness fixes |
| Earlier stable releases | Unsupported unless their release notes explicitly state a later end date |
| Prereleases | Evaluation only; reports are accepted but compatibility and fixes are not guaranteed |
| Branches and untagged commits | Development only |

A stable release is a published semantic version without a prerelease suffix.
Publishing a newer stable release ends support for every earlier stable release
unless the newer release notes explicitly retain one. Glippy has no long-term
support release line. Users SHOULD update to the latest stable release before
reporting a defect and SHOULD reproduce a security claim there when safely
possible.

Support means that the project accepts reports, evaluates them against the
documented contract, and provides a fix or mitigation when a confirmed issue
requires one. It does not promise a response-time, remediation-time, or
availability service-level agreement.

## Supported Runtime Targets

Official release artifacts support:

- macOS on amd64 and arm64; and
- Linux on amd64 and arm64.

Windows and other operating systems are unsupported. Write and fix guarantees
are limited to the documented local-filesystem evidence. Network, distributed,
and userspace filesystems and forced-power-loss durability are outside the
supported write/fix contract unless a later release explicitly admits them.
The exact filesystem and atomic-replacement boundaries are defined in
[`spec/cli.md`](spec/cli.md) and
[`decisions/0007-cli-configuration-and-filesystem.md`](decisions/0007-cli-configuration-and-filesystem.md).

## Supported Go Source And Toolchains

Product release support and Go source-language support are separate. The
current source contract admits Go 1.25 and Go 1.26 as documented in
[`supported-go-versions.md`](supported-go-versions.md). Official binaries do
not require a separately installed Go runtime. Source installation requires
the toolchain selected by `go.mod`.

Encountering unsupported syntax or an unsupported platform is not itself a
security vulnerability. A defect that crosses an authorized filesystem root,
executes code, discloses source, or defeats another security boundary remains
reportable even when unsupported input helped expose it.

## Compatibility And Upgrade Boundaries

Users SHOULD read release notes before upgrading across formatter-output,
configuration-schema, machine-diagnostic-schema, rule-default, rule-ID, or fix
safety changes. The latest stable release is authoritative for its documented
contracts; unreleased branch behavior does not override a published contract.
The complete versioning, deprecation, and migration rules are defined in the
[compatibility and change policy](compatibility-policy.md).

Security reports follow [`../SECURITY.md`](../SECURITY.md). Ordinary defects
belong in the repository's public issue tracker only when they contain no
embargoed vulnerability details, secrets, or private source.
