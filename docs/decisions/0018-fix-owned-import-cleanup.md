# ADR 0018: Fix-Owned Import Cleanup

- Status: accepted for v0.4 development
- Date: 2026-08-15

## Context And Evidence

Suggestion fixes for `unnecessary-sprintf` and `unnecessary-format` can remove
the final use of `fmt`. Their expression replacements are valid, but typed
validation previously rejected the complete transaction because `fmt` remained
unused. Moving general import organization into formatting would violate the
formatter boundary, while requiring every expression rule to rewrite import
declarations would duplicate comment and conflict logic.

## Decision

The single-file fix coordinator may remove an import as derived transaction
work after it applies non-conflicting named fixes and reparses the result. It
may do so only when:

- the original valid source used the exact import path and local name through a
  selector;
- the edited source has no remaining selector with that local name;
- the import is not blank, dot, cgo, or an unresolved implicit package name;
- a grouped specification owns its physical line, unless every specification
  in the declaration is removed; and
- deleting the declaration or specification leaves source that reparses,
  formats, and passes the ordinary fresh analysis validation.

The coordinator removes comments owned by the deleted import while preserving
comments and grouping owned by remaining imports. It does not sort, merge,
insert, rename, or otherwise organize imports. A case outside the admitted
shape remains unchanged and final validation reports why the selected fix was
withheld.

Every derived removal is sorted by import path and local name and appears in
machine fix output as an `import_changes` entry. The applied named fix retains
its ordinary rule and range provenance. Import cleanup does not acquire a rule
ID because it is a consequence of the selected fix transaction, not an
independent diagnostic.

## Alternatives

- Run `goimports` after every fix: rejected because it can add, remove, group,
  and rename imports beyond the accepted fix transaction.
- Put import edits in every rule: rejected because rules cannot safely
  coordinate overlapping declaration edits or preserve other rules' changes.
- Accept parse-only output and leave typed failures to the compiler: rejected
  because Glippy requires complete validation before replacement.
- Remove every syntactically unused import: rejected because that would repair
  pre-existing code outside the selected fix scope.

## Consequences

The two admitted `fmt` simplifications now apply when they remove the last use,
including grouped imports with comments on surviving entries. Hostile
same-line groups and ambiguous package names remain conservative validation
rejections. JSON consumers can distinguish the expression fix from its derived
import cleanup.

## Revisit Trigger

Add import insertion only when a rule has an exact required package path and
local-name contract, collisions are resolved without guessing, comments and
groups remain stable, and the machine plan exposes the derived operation.
Revisit same-line groups only after token-owned separator deletion has focused
fixtures and no comment-migration ambiguity.
