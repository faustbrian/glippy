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
  `--fix-suggestions` selection or editor code action.
- **Unsafe:** capable of changing behavior or public contracts; requires an
  explicit `--fix-unsafe` selection.

Severity and safety are independent. Imported or incompletely audited fixes
MUST NOT default to safe.

The coordinator MUST receive an explicit diagnostic and named-fix selection.
Selecting among multiple named fixes is driver policy and MUST NOT be inferred
from edit order. Ordinary coordination MUST accept safe fixes only. Suggestion
and unsafe fixes MUST require independent explicit authorization.

The ordinary CLI driver selects safe fixes only under `--fix`, suggestions only
under `--fix-suggestions`, and unsafe fixes only under `--fix-unsafe`. The flags
MAY be combined, but one flag MUST NOT implicitly authorize another safety
class. For each diagnostic, the driver selects a fix only when the enabled
classes expose exactly one named alternative. It selects none when no enabled
fix exists and MUST fail the complete prevalidation pass when multiple enabled
alternatives exist. That ambiguity is a rule-contract error, not a
deterministic first-choice policy.

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
7. add an import only when an accepted fix declares its exact path and local
   name, that binding is absent, no accepted fix or source binding conflicts,
   and the insertion preserves existing import comments and groups;
8. remove an import only when the original source used that exact path and
   local name, accepted edits removed its final selector use, and its physical
   declaration can be deleted without changing another import, unless another
   accepted fix requires that exact binding;
9. format through the canonical formatter;
10. reparse and run normalized syntax, comment, directive, and fix-specific
   validation;
11. recheck the on-disk source identity and digest;
12. replace atomically while preserving permissions where supported; and
13. report diagnostics, coordinator-owned import changes, and fixes left
    unapplied with stable provenance.

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
authorized root. After coordination and formatting, syntax-only fixes MUST run
the same enabled syntax analysis against the final source. Typed, CFG, and SSA
fixes MUST run a fresh cache-independent package analysis with the candidate
bound through an exact-path overlay, recover the target file and result from
that load, and reject package diagnostics, source-model problems, or identity
mismatches. A validation failure rejects every otherwise accepted fix in that
file and preserves the original bytes. A failure of the analysis engine itself
retains its tool-failure or cancellation category; it MUST NOT be represented
only as an actionable lint finding. Typed fixing MUST perform one final fresh
package analysis for reporting after all serialized writes.

Formatter normalization after fixes MUST NOT make a semantic rewrite appear to
be formatter behavior. Fix provenance remains attached to the resulting
diagnostic outcome. Import coordination belongs to the fix coordinator, not the
formatter. A fix MAY require ordinary imports only by declaring exact path and
local-name bindings. The coordinator MUST reject invalid, duplicate, or
incompatible requirements, a conflicting existing import or package binding,
and a newly introduced selector that resolves to a local source binding. It
MUST reuse an existing exact binding. It MAY append to one safely represented
grouped declaration; otherwise it MUST add independent declarations without
sorting, merging, renaming, or otherwise organizing existing imports.

Import coordination MUST NOT remove blank imports, dot imports, cgo imports,
pre-existing unused imports, imports whose package name cannot be proven from
syntax, or one import from multiple specifications sharing a physical line. It
MAY remove the complete declaration when every import in that declaration
became unused. A withheld cleanup therefore remains visible through final
validation instead of broadening into general import organization.

A rule that declares a fix but cannot construct it safely for one exact source
MUST attach a structured withheld-fix record to the diagnostic. The record MUST
name canonical fix metadata, contain one supported reason and a nonempty human
message, and MUST NOT name a fix also offered by that diagnostic. Names MUST be
unique and canonically ordered. The initial `comments` reason means the
transformation would discard comment text or ownership. Rule-level withholding
happens before selection and MUST remain distinct from coordinator rejection of
a fix that was actually selected. Text, JSON, cached diagnostics, and editor
diagnostic data MUST preserve the same record.

Lint fix preview uses the same coordinator and validation callback but stops
before atomic replacement. A preview MUST validate that every filesystem
snapshot is still current, and a typed preview MUST carry earlier accepted
candidate bytes through the package overlay used for later reselection and
final analysis. Preview output is advisory only: a later writing invocation
MUST repeat source-version and validation checks rather than treating a prior
preview as authorization or reusable transaction state.
