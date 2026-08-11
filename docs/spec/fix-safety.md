# Fix Safety Model

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Classes

- **Safe:** semantics-preserving under a documented and tested rule contract;
  eligible for ordinary `--fix`.
- **Suggestion:** useful but requiring review; available through explicit
  selection or editor code action.
- **Unsafe:** capable of changing behavior or public contracts; requires an
  explicit unsafe-fix selection.

Severity and safety are independent. Imported or incompletely audited fixes
MUST NOT default to safe.

The coordinator MUST receive an explicit diagnostic and named-fix selection.
Selecting among multiple named fixes is driver policy and MUST NOT be inferred
from edit order. Ordinary coordination MUST accept safe fixes only. Suggestion
and unsafe fixes MUST require independent explicit authorization.

The ordinary CLI driver selects a fix only when one diagnostic offers exactly
one safe named alternative. It selects none when no safe fix exists and MUST
fail the complete prevalidation pass when a diagnostic offers multiple safe
alternatives. That ambiguity is a rule-contract error, not a deterministic
first-choice policy.

## Edit Identity

Every edit MUST identify one physical source file, exact source digest, and
half-open byte range. Replacement bytes are opaque data until the full result
passes validation. Ranges MUST be checked against UTF-8 byte boundaries and the
exact source length when required by the edit contract.

The coordinator MUST refuse a syntactically invalid input file. Lint fixing
MUST NOT become an invalid-source recovery mechanism.

Half-open replacements MAY coexist with insertions at their start or end.
Insertions inside a replacement, overlapping replacements, and multiple
insertions at one byte offset MUST conflict. A conflict in any edit MUST reject
the complete named fix that owns it.

## Coordination Transaction

For one source version, the coordinator MUST:

1. validate source identity, digest, and every range;
2. sort by start, end, stable rule ID, and stable fix name;
3. reject overlapping replacements and incompatible same-offset insertions;
4. report every rejected fix and MUST NOT select a winner silently;
5. apply accepted edits from highest offset to lowest;
6. parse the complete edited result;
7. format through the canonical formatter;
8. reparse and run normalized syntax, comment, directive, and fix-specific
   validation;
9. recheck the on-disk source identity and digest;
10. replace atomically while preserving permissions where supported; and
11. report diagnostics and fixes left unapplied with a stable reason.

Any validation failure or replacement failure before rename MUST preserve the
original file. A post-rename durability failure MAY leave the validated new
content in place and MUST be reported as possibly written. A single-file
transaction MUST NOT claim multi-file atomicity. Multi-file fixes remain
prohibited until recovery from partial filesystem failure has an accepted
design and integration evidence.

The in-memory coordinator MAY apply independent fixes after rejecting every
fix in a conflict group. If the complete edited source does not parse or fails
formatter validation, it MUST reject every otherwise accepted fix and return
the original bytes. Applied and rejected records MUST retain stable rule, fix,
range, and reason provenance.

The disk transaction MUST begin from a validated regular-file snapshot and
MUST recheck identity, digest, permissions, and authorized-root ownership before
replacement. A stale-source result MUST be reported as not written. Any other
replacement error that cannot prove whether rename occurred MUST be reported
as possibly written. Reporters MUST NOT collapse coordinated bytes, confirmed
replacement, and possible replacement into one success state.

Before the first CLI replacement, every selected source MUST load, parse, pass
generated-file policy, and resolve through a non-symlink path inside its
authorized root. After coordination and formatting, the same enabled syntax
analysis MUST run against the final source. A validation failure rejects every
otherwise accepted fix in that file and preserves the original bytes. A failure
of the analysis engine itself retains its tool-failure or cancellation category;
it MUST NOT be represented only as an actionable lint finding.

Formatter normalization after fixes MUST NOT make a semantic rewrite appear to
be formatter behavior. Fix provenance remains attached to the resulting
diagnostic outcome.
