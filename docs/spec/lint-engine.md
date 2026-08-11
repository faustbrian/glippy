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
fix availability and safety, typed options schema, deprecation metadata, known
limitations, and paired incorrect/correct examples.

The scheduler MUST compute the maximum required representation across enabled
rules and MUST NOT construct types, CFG, or SSA speculatively. Syntax rules
SHOULD share filtered traversal when its construction cost is amortized by the
enabled node interests. Typed, CFG, and SSA values are run-owned and MUST NOT
cross incompatible `go/packages` loads.

The default `correctness` preset is limited to incorrect, unsafe, ineffective,
misleading, or highly suspicious behavior with measured signal. Suspicious,
performance, complexity, style, and migration groups are opt-in until their
admission evidence supports a compatibility change.

The Phase 3 file driver MUST resolve the preset and overrides once, record the
maximum enabled requirement, execute the shared syntax runner once, and apply
the source-versioned suppression index before returning reporter-facing
records. Unsuppressed diagnostics, suppressed diagnostics, unused directives,
and suppression problems MUST remain distinct outcomes. Until another tier
runner is implemented, the driver MUST reject every enabled non-syntax rule
rather than skip it or construct a representation speculatively.

## Diagnostics

A diagnostic MUST contain rule ID, resolved severity, stable message key and
text, precise physical primary range, optional related ranges, notes/help,
source identity and digest, named fixes, and fix safety. Every reporter MUST
consume the same globally sorted diagnostic records. JSON carries an explicit
schema version before external consumers are supported.

Schema-version-1 lint JSON MUST identify every analyzed file by normalized
absolute path and lowercase SHA-256 source digest. Primary and related ranges
MUST use half-open physical UTF-8 byte offsets. Ordinary lint JSON MUST expose
only fix name and safety, not source snippets or replacement text. It MUST keep
suppression syntax problems and unused directives distinct from rule
diagnostics, and MUST represent suppressed diagnostics by count without
disclosing their bodies by default.

Lint text MUST render each primary diagnostic as
`path:line:byte-column: severity[rule-id]: message`. Related locations, notes,
help, and named fix safety MUST use indented continuation lines. Suppression
syntax problems and unused directives MUST remain visibly distinct. The text
reporter MUST order source files and diagnostics canonically, validate the
exact source identity plus every primary, related, fix-edit, directive, and
suppression-target range, and MUST NOT emit source excerpts or replacement
text. Physical locations follow the source model and MUST NOT be adjusted by
`//line` directives.

## Suppressions

Suppressions MUST name exact rule IDs. The grammar MUST define line, next-line,
range, and file ownership without allowing an unscoped silent disable-all.
Configuration MAY require a non-empty reason. Unknown, malformed, unused, and
expired suppressions SHOULD be independently diagnosable.

Suppression ownership is based on physical token boundaries and source
identity, not incidental output line numbers. Formatting MUST preserve the
directive bytes and target relationship. A formatter change that would move a
suppression to a different target MUST be rejected.

The initial grammar accepts one exact rule per line comment:

```text
//gox:ignore rule-id [-- reason]
//gox:ignore-line rule-id [-- reason]
//gox:ignore-start rule-id [-- reason]
//gox:ignore-end rule-id
//gox:ignore-file rule-id [-- reason]
```

`ignore` MUST target only the immediately following physical line;
`ignore-line` MUST target only the directive's physical line. A same-rule
`ignore-start` and `ignore-end` pair MUST target the half-open byte range
between the comments and MUST NOT nest. `ignore-file` MUST appear before the
package clause and MUST target the complete source file. The matcher MUST
suppress a diagnostic only when its primary range start belongs to the target.
Overlap by a larger enclosing range is insufficient.

The matcher MUST require the normalized source path and exact source digest
that produced the index, and MUST reject invalid or out-of-bounds diagnostic
ranges. When multiple same-rule directives match one diagnostic, the first
source-ordered directive MUST own it. Application MUST preserve diagnostic
order and MUST report every valid directive that owns no diagnostic as unused.

`--` introduces a reason. When reason policy is enabled, starts and direct
scopes MUST carry a non-empty reason. Range ends MUST NOT carry a reason.
Unknown rules, malformed directives, missing reasons, misplaced file scopes,
nested ranges, unmatched ends, and unclosed starts MUST be reported in source
order. The parser MUST NOT accept a directive that omits a rule ID or disables
all rules.

## `go/analysis` Interoperability

Suitable analyzers MAY be adapted without replacing the native scheduler or
metadata. Imported diagnostics and facts are sorted independently of emission
order. Suggested fixes default to suggestion safety unless separately audited.
Analyzers that mutate shared AST state, depend on deprecated object resolution,
or require unsupported multi-file edits MUST be isolated or rejected with a
clear compatibility diagnostic.

Rule documentation and `explain` output MUST derive from the same immutable
registry metadata and examples. Human `explain` output MUST include the rule
ID, summary, full documentation, default severity, presets, minimum Go version,
analysis tier, node interests, generated-file policy, categories, deprecation
and replacement metadata when present, named fix safety, typed configuration,
known limitations, and every paired example. Empty fix, configuration, or
known-limitation sets MUST remain explicit instead of disappearing from the
documentation contract.
