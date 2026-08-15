# ADR 0018: Fix-Owned Import Coordination

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

The single-file fix coordinator may add or remove imports as derived
transaction work after it applies non-conflicting named fixes and reparses the
result.

A fix may declare exact required package bindings as import path and local-name
pairs. The analysis engine, analyzer adapters, and caches validate, clone,
canonically order, persist, and restore those requirements. The coordinator:

- rejects invalid, duplicate, and incompatible path/name requirements;
- rejects accepted fixes that require one name for different paths or one path
  with different names;
- reuses an existing exact binding;
- rejects a conflicting import or package declaration and a newly introduced
  selector that resolves to a local binding;
- appends to one grouped declaration only when its closing delimiter has a
  safe physical-line insertion point; and
- otherwise adds independent declarations without sorting, merging, renaming,
  or reorganizing existing imports.

The coordinator may remove an import only when:

- the original valid source used the exact import path and local name through a
  selector;
- the edited source has no remaining selector with that local name;
- the import is not blank, dot, cgo, or an unresolved implicit package name;
- no accepted fix requires that exact binding;
- a grouped specification owns its physical line, unless every specification
  in the declaration is removed; and
- deleting the declaration or specification leaves source that reparses,
  formats, and passes the ordinary fresh analysis validation.

The coordinator removes comments owned by the deleted import while preserving
comments and grouping owned by remaining imports. Insertions preserve existing
comments and groups and do not infer a preferred group. A case outside the
admitted shapes remains unchanged and final validation reports why the selected
fix was withheld.

Every derived addition and removal is sorted by import path, local name, and
action and appears in machine fix output as an `import_changes` entry. The
applied named fix retains its ordinary rule and range provenance. Import
coordination does not acquire a rule ID because it is a consequence of the
selected fix transaction, not an independent diagnostic.

## Alternatives

- Run `goimports` after every fix: rejected because it can add, remove, group,
  and rename imports beyond the accepted fix transaction.
- Put import edits in every rule: rejected because rules cannot safely
  coordinate overlapping declaration edits or preserve other rules' changes.
- Accept parse-only output and leave typed failures to the compiler: rejected
  because Glippy requires complete validation before replacement.
- Remove every syntactically unused import: rejected because that would repair
  pre-existing code outside the selected fix scope.
- Infer imports from replacement text: rejected because selector spelling does
  not prove a package path or intended local binding.

## Consequences

The two admitted `fmt` simplifications now apply when they remove the last use,
including grouped imports with comments on surviving entries. The
`unsafe-host-port` suggestion can now add `net` to the file containing the
diagnosed `fmt.Sprintf`, including when a different file supplies the dialing
use that proves the diagnostic. Hostile same-line groups, ambiguous package
names, and local selector collisions remain conservative validation rejections.
JSON consumers can distinguish the expression fix from derived import changes.

Package-aware fixing retains generated files as read-only compilation inputs
when no selected edit targets them. A generated target still rejects complete
prevalidation before any write. After a successful write or preview, one fresh
package analysis refreshes all remaining selections; each later changed file
is still reselected from current package state. This avoids package reloads for
every unchanged file without weakening stale-diagnostic or generated-file
policy.

## Revisit Trigger

Revisit local-binding precision when real fixes are withheld by unrelated
selectors with the same local name. Revisit same-line groups only after
token-owned separator insertion or deletion has focused fixtures and no
comment-migration ambiguity. General import organization remains outside this
decision.
