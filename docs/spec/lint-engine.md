# Lint Engine Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Native Rules

Every rule MUST declare a stable ID, summary, full documentation, default
severity, preset membership, minimum source version, required analysis tier,
node interests where applicable, generated-file policy, diagnostic categories,
fix availability and safety, typed options schema, and deprecation metadata.

The scheduler MUST compute the maximum required representation across enabled
rules and MUST NOT construct types, CFG, or SSA speculatively. Syntax rules
SHOULD share filtered traversal when its construction cost is amortized by the
enabled node interests. Typed, CFG, and SSA values are run-owned and MUST NOT
cross incompatible `go/packages` loads.

The default `correctness` preset is limited to incorrect, unsafe, ineffective,
misleading, or highly suspicious behavior with measured signal. Suspicious,
performance, complexity, style, and migration groups are opt-in until their
admission evidence supports a compatibility change.

## Diagnostics

A diagnostic MUST contain rule ID, resolved severity, stable message key and
text, precise physical primary range, optional related ranges, notes/help,
source identity and digest, named fixes, and fix safety. Every reporter MUST
consume the same globally sorted diagnostic records. JSON carries an explicit
schema version before external consumers are supported.

## Suppressions

Suppressions MUST name exact rule IDs. The grammar MUST define line, next-line,
range, and file ownership without allowing an unscoped silent disable-all.
Configuration MAY require a non-empty reason. Unknown, malformed, unused, and
expired suppressions SHOULD be independently diagnosable.

Suppression ownership is based on physical token boundaries and source
identity, not incidental output line numbers. Formatting MUST preserve the
directive bytes and target relationship. A formatter change that would move a
suppression to a different target MUST be rejected.

## `go/analysis` Interoperability

Suitable analyzers MAY be adapted without replacing the native scheduler or
metadata. Imported diagnostics and facts are sorted independently of emission
order. Suggested fixes default to suggestion safety unless separately audited.
Analyzers that mutate shared AST state, depend on deprecated object resolution,
or require unsupported multi-file edits MUST be isolated or rejected with a
clear compatibility diagnostic.

Rule documentation and `explain` output MUST derive from the same canonical
metadata and examples.
