# Gox Development Status

- Progress: 45%
- Current phase: Phase 2, production-usable formatter
- Phase 0 completed: 2026-08-09
- Phase 1 completed: 2026-08-11

Phase 0 established the reviewed product contracts, shared-frontend and edit
boundaries, initial hostile-valid corpus, bounded document renderer, controlled
baseline harness, and working-name replacement requirement.

The proven formatter foundation includes isolated immutable syntax views,
physical token and trivia reconstruction, bounded document rendering, comment
and directive ownership, normalized equivalence, golden and idempotency
behavior, and invalid-source refusal. The hostile corpus passes at widths 20,
60, 100, and 120. Five 15-second fuzz campaigns completed 333,855 executions
across the source, fragment, formatter, and document boundaries without a
failure.

The product-wide gofmt incompatibility classes are recorded. Renderer execution
is bounded at 100,000 nested groups and 20,000 sibling groups, and the formatter
prototype scaling probe shows allocation growth proportional to syntax size.
The earlier full, race, static-analysis, build, corpus, differential, and fuzz
gates found no semantic, source-fidelity, directive-loss, idempotency, or
unbounded-layout defect. They did not prove human layout quality.

Implemented Phase 2 work includes deterministic file and directory discovery,
strict configuration discovery, configuration-aware stdin and check modes, and a
prevalidated write-mode prototype with stale-source refusal, unchanged-file
preservation, permission-preserving same-directory replacement, and generated
file and symlink refusal. Formatting preparation is now bounded by selection
size, `GOMAXPROCS`, and 32 workers while retaining normalized task order;
interrupt, termination, and caller cancellation stop scheduling and are checked
before every replacement with prior writes disclosed. Path-based check and
write modes now share deterministic text outcomes and a versioned JSON envelope
covering success, findings, invalid input, source failures, partial writes,
conflicts, and reporting failures. The required version command now reports
explicit release metadata, a versioned Go installation, or a deterministic
development fallback without loading project state. A text-only `fmt --diff`
mode now renders deterministic, bounded, three-context unified differences in
path order without mutating source. The standard-input editor path now has
current Conform.nvim and Helix format-on-save guidance plus in-process and
fresh-process latency probes. All 100 recorded fresh processes satisfy the
provisional 250 ms reference-host budget, but scheduler variance still blocks a
stable CI threshold.

Self-dogfood validates all 32 discovered repository files and changes 30. The
control-flow repair reduces stranded control keywords from 82 to zero, and the
receiver repair reduces 20 receiver-prefix breaks to zero. The selector repair
separates terminal call arguments from callee fit decisions and reduces 193
selector-pattern targets to two lines belonging to one intentionally broken,
deeply indented indexed chain. The complete migration snapshot is classified,
valid, and idempotent, restoring the Phase 1 exit gate. Existing safe
filesystem, configuration, and CLI proof supports 45%. Complete
platform-specific filesystem semantics, release artifacts, and
external-repository adoption remain open before the 55% gate.

An immutable external check of `go-libraries` at
`a6f1c1f66a1b754e7384da0f6e97e0b3587c5f71` selected 5,051 files. It exposed
two valid binary expressions with trailing line comments that previously
aborted formatting. The corrected binary boundary completes the same snapshot
with 4,816 formatting differences and no tool error. This is check-mode corpus
evidence, not completed adoption: generated-file refusal prevented a full-tree
write rehearsal, and the 4,816-file migration diff has not received human
readability approval.

A bounded disposable write rehearsal on the external `pkg/prompts` module
selected 77 files, changed 69, and produced a zero-difference second Gox check.
The migrated snapshot passes its tests, race tests, vet, and tidy check. A
total of 63 files remain non-fixed-points under gofmt, so the module's current
formatting gate and the unapproved 7,611-insertion/4,077-deletion migration keep
external adoption open. The migration guide now defines non-mutating and
disposable rehearsals, sole-formatter cutover, human review, and coherent
rollback for repositories replacing gofmt, gofumpt, or golines; it does not
advance the 45% capability gate.

The current formatter dialect is now published with evidence-linked examples
covering width, indentation, blocks, semicolons, control flow, lists, binary
expressions, selectors, comments, directives, preserved source choices, and
write refusal. This closes the Phase 2 formatter-rule documentation item but
does not resolve the remaining platform, naming, or external-adoption gates.

Isolated Phase 3 foundation work now defines validated canonical rule metadata,
an immutable ordered registry, preset and override resolution, maximum-tier
selection, complete Go AST node interests, and one shared filtered syntax
traversal. Syntax diagnostics carry exact source identity and physical ranges,
resolve severity deterministically, reject undeclared or malformed fixes, sort
independently of rule registration, and honor generated-file eligibility. No
built-in rule, lint CLI, lint-pipeline suppression integration, fix coordinator,
or lint reporter is implemented yet, so this later-phase foundation does not
advance progress past the incomplete 45% Phase 2 gate.

The first suppression foundation now parses one exact rule per `//gox:`
directive, assigns deterministic physical line, next-line, paired-range, and
file ownership, supports optional or required reasons, and reports malformed,
unknown, misplaced, nested, unmatched, and unclosed directives in source order.
The source-versioned application pass now filters ordered diagnostics, records
their owning directives, and identifies unused waivers without accepting
cross-file or stale ranges. The contract was refreshed against Oxc `f2125a8`.
Canonical unused and expiry diagnostics, lint CLI/reporting, built-in rules,
and lint-driver integration remain open, so progress remains 45%.

The first in-memory fix coordinator now refuses invalid input, validates exact
source identity, fix safety, UTF-8 byte ranges, and replacement text; rejects
every participant in overlapping or same-offset insertion conflicts; applies
independent edits in a deterministic range-safe order; reparses and
formatter-normalizes the complete result; and rolls back all accepted fixes on
validation failure. It records stable applied and rejected provenance without
claiming that coordinated bytes reached disk.

The single-file fix transaction now starts from the shared descriptor-validated
snapshot, coordinates its exact source version, skips writes when no fix can
apply or output is unchanged, and uses the permission-preserving same-directory
atomic writer. It distinguishes confirmed stale refusal, completed replacement,
and other failures that may have followed rename. Lint CLI selection and
reporting remain open, so progress stays 45%.

The first file-owned lint driver now resolves preset and severity policy,
records the maximum required tier, runs the shared syntax traversal once, and
applies the exact-source suppression index. It keeps visible, suppressed,
unused, and malformed-suppression outcomes distinct and refuses unsupported
tiers instead of silently skipping them. Lint reporters, CLI modes, fix
selection, and built-in rule admission remain open, so progress stays 45%.

The first versioned lint-check JSON result now validates exact per-file source
identity, orders files and diagnostics canonically, and reports visible counts,
suppressed counts, suppression problems, unused directives, rich diagnostic
ranges, and named fix safety. It deliberately omits source snippets and edit
replacement text. Text reporting, lint CLI modes, fix outcome reporting, and
built-in rules remain open, so progress stays 45%.

The first lint text renderer now binds each analysis result to its exact source
version, maps physical byte offsets to CRLF-aware 1-based line and byte-column
locations without inheriting `//line` adjustments, and validates every primary,
related, fix-edit, suppression-directive, and suppression-target range at UTF-8
boundaries. It emits canonically ordered diagnostics with related locations,
notes, help, and named fix safety while omitting source excerpts and replacement
text. Lint CLI modes, fix outcome reporting, and built-in rules remain open, so
progress stays 45%.

Canonical rule metadata now carries structured known limitations alongside
existing prerequisites, typed options, fix safety, deprecation, and paired
examples. The human rule renderer exposes that complete immutable contract, and
`gox explain <rule>` now validates arguments, cancellation, unknown IDs, and
output failures without loading project state. The compiled registry is still
empty because no built-in rule has passed its admission and dogfood-noise gate;
the success path is proven with an injected validated registry. Progress stays
45%.

The first syntax-only `gox lint` check now discovers sorted physical files,
resolves each selected configuration against the compiled registry, runs the
shared file driver, and emits the same text or versioned JSON diagnostic
contracts without mutation. Findings include visible diagnostics, suppression
problems, and unused directives; source, configuration, filesystem,
cancellation, and reporting failures preserve distinct exits and incomplete
JSON. The production registry remains empty, while injected-registry tests prove
configured rule severity and both reporters. Package patterns, typed loading,
fix flags, fix outcome reporting, and built-in rule admission remain open, so
progress stays 45%.
