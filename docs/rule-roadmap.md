# Rule Roadmap

Glippy expands lint coverage only after the shared syntax, types, CFG, SSA, fact,
cache, and fix foundations have passed their evidence gates. The
[Phase 4 exit audit](research/phase4-exit-gate-2026-08-13.md) closes that
foundation boundary. This roadmap therefore starts rule-growth planning; it
does not reserve rule IDs, promise a rule count, or weaken the admission gate.

The current catalog remains intentionally small. `correctness` is the default
preset group. `suspicious`, `performance`, `complexity`, `style`, and
`pedantic` are composable adoption choices. Restriction rules are enabled
individually, and migration rules require an explicit target.

## Priorities

Work proceeds in this order:

1. **Improve precision of admitted rules.** Resolve evidence-backed false
   positives, false negatives, and documented representation gaps before
   adding nearby rules that would repeat them.
2. **Add proven correctness defects.** Prefer code that panics, loses data,
   makes control flow ineffective, violates a standard-library contract, or
   produces a result the author cannot use as intended.
3. **Grow the suspicious preset cautiously.** Admit likely defects only after
   reviewed positive examples and broad negative dogfood establish a useful
   boundary.
4. **Add opt-in policy groups.** Performance, complexity, style, pedantic,
   restriction, and migration rules require an explicit consumer and
   group-specific cost and noise evidence. Restriction remains rule-by-rule;
   migration requires a target. None justifies increasing default analysis
   cost.

Formatter-owned layout never enters this queue. A semantic transformation is
considered through the fix coordinator, not hidden inside formatting.

The response and stream lifecycle track has admitted its first two CFG rules:
`unchecked-rows-error` and `unchecked-scanner-error` prove that direct
`database/sql.Rows.Next` and `bufio.Scanner.Scan` loops check the matching
terminal error on every normally returning path. Alias and interface expansion
remains evidence-gated rather than silently increasing either rule to SSA.

The first restriction rule, `blank-error-discard`, provides an exact-ID policy
for projects that prohibit explicit `_ = err` and tuple-error discards. It is
not part of a selectable preset: deliberate best-effort operations require a
reasoned suppression, and test files remain separately configurable.

The error-flow track now admits `overwritten-error` as an SSA-backed suspicious
rule. It is deliberately narrower than Staticcheck SA4006: only error-typed
values followed by another definition of the same object report, while any
observable use or explicit blank assignment prevents a finding.

The same track now admits `typed-nil-error-return` as an opt-in suspicious
rule. It reports only concrete error values proven nil at an explicit return;
untyped nil, interface operands, unknown values, bare returns, and tuple calls
remain excluded. This complements Staticcheck SA4023 by locating the definite
return-site defect without waiting for a caller to compare the result to nil.
Keeping it opt-in preserves syntax-only default scheduling until default SSA
cost and source-error behavior are explicitly adopted.

The error-flow track also admits `shadowed-error` as an opt-in suspicious
types rule. A broad adaptation of x/tools `shadow` was rejected after it
reported ordinary local error handling during dogfood. The admitted contract
is limited to two stale flows: breaking from a loop on an inner error before
observing the unchanged outer error, and deferred assignment to an inner error
that hides a named result. General lexical shadowing remains out of scope.

The lifecycle track now admits `sql-transaction-not-completed` as a default
correctness CFG rule for direct `database/sql` transactions. After a
conventional successful acquisition guard, every normally returning path must
call the exact transaction Commit or Rollback method or conservatively transfer
ownership. Reassignment loses the original obligation; wrapper constructors and
arbitrary finalizers remain outside the initial contract.

The first shared obligation/effect layer now summarizes bounded intraprocedural
CFG paths as open, completed, transferred, or lost. Both SQL transactions and
local `Close() error` resources use it; `resource-not-closed` therefore reports
partial cleanup and reassignment instead of accepting one close anywhere in a
function. Shared CFG and SSA construction now propagates no-return behavior
through statically called functions and methods in the loaded package. Imported
completion, transfer, and no-return facts remain a separate evidence-gated
expansion.

The response lifecycle track now also admits
`http-response-body-not-closed`. It follows exact `net/http` package and Client
acquisitions after a conventional error guard, requires close or transfer on
every normal return, and does not mistake body reads for ownership transfer.
Arbitrary helper cleanup, custom transports, and body replacement remain in
the interprocedural investigation rather than weakening the local rule.

The pedantic track now also admits `empty-branch`, `manual-min-max`, and
`redundant-type-declaration`. These rules retain narrow syntax or exact-type
contracts, remain opt-in, and avoid claiming that a readability preference is
a correctness defect. Only redundant type spelling has a safe fix, and comment
ownership can withhold that edit.

## Investigation Queue

The queue contains defect classes to investigate, not accepted rule IDs.
Every investigation may conclude that the compiler, `go vet`, Staticcheck, or
another existing default tool already owns the problem well enough.

| Order | Defect class | Cheapest plausible tier | Admission question |
| ---: | --- | --- | --- |
| 1 | Cross-package obligation and no-return facts | CFG plus facts | Which stable imported function effects materially improve the admitted transaction, closer, cancellation, and stream rules without loading dependency syntax speculatively? |
| 2 | Standard-library argument roles and state contracts | types | Can typed object identity prove a misuse without guessing caller intent, as it does for the admitted `errors-is-arguments` and `context-key` rules? |
| 3 | Ineffective or misleading control transfer beyond the current terminal-break cases | syntax, then CFG only if needed | Is the transfer observably ineffective, and can Glippy distinguish the intended enclosing construct without path-sensitive speculation? |
| 4 | Resource cleanup inside repeated execution | CFG | Can reachability prove that cleanup is deferred indefinitely or skipped, while excluding deliberate process and goroutine termination? |
| 5 | Impossible nil and state transitions across calls | SSA plus admitted facts | Do interprocedural facts materially improve precision over the current intraprocedural `nilness` boundary? |
| 6 | Response, stream, and closer lifecycle misuse | types plus CFG or SSA | Can ownership and escape boundaries identify a real leak or use-after-close defect without treating every transfer as local ownership? |
| 7 | Repeated, subsumed, or contradictory conditions | syntax or types | Can the comparison remain side-effect-safe and avoid pretending syntactic similarity proves semantic equivalence? |
| 8 | Structurally credible allocation or concurrency costs | types, CFG, or SSA | Is there a reproducible cost mechanism and an opt-in performance contract rather than a generic micro-optimization preference? |
| 9 | Explicit Go-version and API migrations | syntax or types | Is there a configured target version, a canonical replacement, and a migration-specific safety classification? |

The first six tracks focus on correctness and suspicious behavior. The last
three remain opt-in unless release evidence justifies a compatibility-reviewed
preset change.

## Admission Evidence

Before implementation, each candidate receives its own record under
`docs/research`. That record identifies:

- an observable defect and at least one reviewed real occurrence;
- the compiler and default `go vet` boundary for every supported Go version;
- positive examples and close negative examples;
- the expected false-positive and false-negative boundaries;
- the cheapest representation that satisfies the precision contract;
- behavior for generated files, type errors, suppressions, and source versions;
- fix availability and independent safe, suggestion, or unsafe classification;
- a proportional cost probe; and
- non-mutating dogfood across Glippy plus representative external code.

Candidates without this evidence remain investigations. They are not added as
disabled built-ins merely to advertise future coverage.

## Release Discipline

Rule additions remain independently reviewable. A release may add no rules at
all when the evidence queue is weak. Default-preset changes require the
compatibility process, release notes, before-and-after examples, and fresh
noise measurements. Rule IDs become stable only under the published
[compatibility policy](compatibility-policy.md).

Safe fixes need semantic evidence beyond successful parsing. Suggestion and
unsafe fixes stay separately selectable, and every applied edit continues
through stale-source, conflict, reparse, formatter, reanalysis, and atomic
write validation.

The fixability track now includes `remove-redundant-nil-check` as a safe fix.
It replaces the matched boolean expression with the exact length-comparison
source and withholds edits that would discard comments. The shared coordinator
formats, reanalyzes, validates, and writes the complete result idempotently.
It also includes the `use-format-operand` suggestion for `unnecessary-format`.
That edit retains the constant operand's exact source and comments, while final
typed validation refuses a replacement that would leave `fmt` unused.

## Explicit Deferrals

The roadmap does not include:

- copying Staticcheck, `go vet`, Oxlint, or another catalog wholesale;
- adding layout lint rules already owned by the formatter;
- enabling types, CFG, or SSA by default to make a rule easier to write;
- unscoped suppressions or a dynamic plugin marketplace;
- reserving public rule IDs before admission; or
- counting experimental or disabled rules as product progress.

The roadmap is revisited when dogfood produces a repeated defect class, a Go
release changes an existing tool boundary, an admitted rule accumulates a
precision problem, or a consumer proposes a rule with complete admission
evidence.
