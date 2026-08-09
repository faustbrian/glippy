# Gox Development Status

- Progress: 10%
- Current phase: Phase 1, formatter core prototype
- Phase 0 completed: 2026-08-09

Phase 0 established the reviewed product contracts, shared-frontend and edit
boundaries, initial hostile-valid corpus, bounded document renderer, controlled
baseline harness, provisional gofmt compatibility target, and working-name
replacement requirement.

Phase 1 must now prove the immutable source/trivia representation, syntax
lowering, comment and directive ownership, normalized equivalence, golden and
idempotency suites, corpus behavior, and fuzz safety. No Phase 1 formatter
correctness or source-fidelity gate is implied by Phase 0 completion.
