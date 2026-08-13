# Security Policy

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Report A Vulnerability

Security reporters MUST use a private channel. The preferred channel is
GitHub's
[private vulnerability report](https://github.com/faustbrian/glippy/security/advisories/new).
Reporters MUST NOT disclose a suspected vulnerability in a public issue,
discussion, pull request, commit, or other public channel before coordinated
disclosure.

If GitHub does not offer the private report form, request a private contact
channel from the [repository owner](https://github.com/faustbrian) without
including vulnerability details in the request. This fallback is only for
establishing a private channel; the report itself MUST remain private.

A useful report SHOULD include:

- the affected Glippy version or complete commit ID;
- operating system, architecture, Go source version, and relevant filesystem;
- the smallest reproduction that does not disclose unrelated private source;
- expected and observed behavior;
- security impact and required preconditions;
- whether formatting, linting, fixing, caching, or package loading is involved;
  and
- any known workaround or evidence that the issue is already public.

Reports MUST NOT contain live credentials, personal data, proprietary source
that is unnecessary to reproduce the issue, or destructive payloads that run
without an explicit action by the maintainer.

## Security Scope

Security-relevant reports include, but are not limited to:

- writing outside the authorized source or cache root;
- symlink, path traversal, replacement, or stale-source races that can corrupt
  another file;
- a safe fix that violates its documented semantics-preserving contract;
- output that silently loses or changes a compiler, build, cgo, line, embed,
  generated-file, or suppression directive;
- command execution, unexpected network access, or source disclosure during an
  ordinary offline operation;
- cache content that can supply incorrect trusted results after integrity or
  identity validation;
- malformed or adversarial input that bypasses a documented resource bound and
  creates a practical denial of service; and
- release artifacts whose digest, provenance, source revision, or embedded
  version does not match the published release record.

Formatting preferences, diagnostics that are merely noisy, unsupported
platforms, and documented network-filesystem or forced-power-loss limitations
are not security vulnerabilities by themselves. A report remains in scope when
one of those conditions exposes data, executes code, escapes an authorized
root, corrupts unrelated source, or defeats another documented security
boundary.

## Response And Disclosure

The maintainer SHOULD acknowledge a private report after enough information is
available to distinguish it from an ordinary bug. Acknowledgement, triage, fix,
and disclosure times are targets determined by severity and complexity, not a
service-level agreement.

The reporter and maintainer SHOULD keep technical details private until a fix
or documented mitigation is available to users of supported releases. A
confirmed vulnerability MAY receive a GitHub Security Advisory, CVE, release
note, credit, and coordinated publication date. Credit MUST follow the
reporter's stated preference. Glippy MUST NOT publish reporter identity or private
report contents without permission, except when legally required.

Security fixes MUST pass the narrowest applicable regression and release gates
before publication. The project MAY withhold exploit details temporarily when
immediate publication would materially endanger users who have not had a
reasonable opportunity to update.

Good-faith research that respects this policy, avoids privacy violations and
service disruption, and gives the project a reasonable opportunity to respond
will not be treated by the project as malicious activity. This statement does
not authorize testing systems, repositories, data, or accounts that the
researcher does not own or have permission to test.

## Supported Releases

Security support follows the [product support policy](docs/support-policy.md).
Until the first public release, the repository contains development artifacts
only and no revision is a supported release. Reports against development code
are still welcome through the private channel.
