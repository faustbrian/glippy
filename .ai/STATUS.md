# Gox Development Status

- Progress: 35%
- Current phase: Phase 2, production-usable formatter
- Phase 0 completed: 2026-08-09
- Phase 1 completed: 2026-08-10

Phase 0 established the reviewed product contracts, shared-frontend and edit
boundaries, initial hostile-valid corpus, bounded document renderer, controlled
baseline harness, and working-name replacement requirement.

Phase 1 proves isolated immutable syntax views, physical token and trivia
reconstruction, complete prototype syntax dispatch, comment and directive
ownership, normalized equivalence, grammar-aware width decisions, golden and
idempotency behavior, and invalid-source refusal. The hostile corpus passes at
widths 20, 60, 100, and 120; repository-wide standard-input dogfood succeeds.
Five post-fix 15-second fuzz campaigns completed 333,855 executions across the
source, fragment, formatter, and document boundaries without a failure.

The product-wide gofmt incompatibility classes are recorded. Renderer execution
is bounded at 100,000 nested groups and 20,000 sibling groups, and the formatter
prototype scaling probe shows allocation growth proportional to syntax size.
Current full, race, static-analysis, build, corpus, differential, and fuzz gates
pass with no known semantic, source-fidelity, directive-loss, idempotency, or
unbounded-layout defect.

Phase 2 now proves deterministic file and directory discovery, strict
configuration discovery, configuration-aware stdin and check modes, and a
prevalidated write-mode prototype with stale-source refusal, unchanged-file
preservation, permission-preserving same-directory replacement, and generated
file and symlink refusal. Bounded parallelism, complete platform-specific
filesystem semantics, reporting, editor workflows, release artifacts, and
real-repository adoption remain open. Progress therefore remains at the Phase
1 exit gate of 35%.
