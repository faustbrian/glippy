# Gox Development Status

- Progress: 20%
- Current phase: Phase 1, formatter core prototype reopened
- Phase 0 completed: 2026-08-09
- Phase 1 exit gate reopened: 2026-08-11

Phase 0 established the reviewed product contracts, shared-frontend and edit
boundaries, initial hostile-valid corpus, bounded document renderer, controlled
baseline harness, and working-name replacement requirement.

The proven 20% foundation includes isolated immutable syntax views, physical
token and trivia reconstruction, bounded document rendering, comment and
directive ownership, normalized equivalence, golden and idempotency behavior,
and invalid-source refusal. The hostile corpus passes at widths 20, 60, 100,
and 120. Five 15-second fuzz campaigns completed 333,855 executions across the
source, fragment, formatter, and document boundaries without a failure.

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

Self-dogfood then validated all 32 discovered repository files but changed 30
and exposed unacceptable control-flow keyword breaks plus open receiver and
selector-chain readability findings. The control-flow defect is fixed, reducing
stranded control keywords from 82 to zero in the same snapshot. The Phase 1
readability claim is withdrawn until the remaining migration diff is classified
and accepted. Complete platform-specific filesystem semantics, release
artifacts, and external-repository adoption also remain open.
