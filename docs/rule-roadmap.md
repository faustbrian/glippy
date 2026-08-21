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

The project-contract track now admits `must-use-result` as a default
correctness rule. Exact configured function or method results report when a
call statement, `go` or `defer` statement, or blank destination discards them.
Ordinary assignments, returns, and arguments count as consumption. When the
contract rule is selected, its exact call diagnostic owns and supersedes the
broader `discarded-error` diagnostic before suppression or baselining. Dynamic
calls and unconfigured APIs stay outside this exact policy boundary.

The project-contract track also makes `returns-alias` observable in the shared
lifecycle obligation engine. When an exact contracted result is assigned back
to the same tracked transaction, closer, or cancellation function, ownership
returns to the caller and the existing completion obligation remains live. A
guaranteed close, transaction completion, or cancellation invocation still
discharges it. New alias bindings and post-close state remain conservative
rather than inferring an unbounded alias graph.

The response and stream lifecycle track has admitted its first two CFG rules:
`unchecked-rows-error` and `unchecked-scanner-error` prove that direct
`database/sql.Rows.Next` and `bufio.Scanner.Scan` loops check the matching
terminal error on every normally returning path. Alias and interface expansion
remains evidence-gated rather than silently increasing either rule to SSA.

The output-integrity track now admits `unchecked-writer-error`,
`unchecked-csv-writer-error`, and `writer-not-finalized` as default correctness
rules. The first reports
discarded Flush and Close errors from exact standard-library buffered,
archive, compression, encoding, multipart, and tabular writers whose
finalizers emit pending bytes or required framing. Ordinary, deferred,
asynchronous, and explicit blank-identifier discards share one diagnostic
identity; the broader `discarded-error` and `blank-error-discard` rules
delegate these calls to prevent duplicate output. The CSV rule separately
proves that every normally returning path after a direct
`encoding/csv.Writer.Flush` observes the matching `Writer.Error`. Interface-
returning streaming encoders from `encoding/ascii85`, `encoding/base32`, and
`encoding/base64` are also covered when an exact constructor result remains in
a stable direct binding through `Close`. Indirect acquisition and reassigned
interface bindings remain evidence-gated follow-up work.

`writer-not-finalized` covers the distinct missing-finalizer state for direct
tar, gzip, multipart, ascii85, base32, and base64 writer acquisitions. Exact
output-producing method calls start the obligation; direct or deferred `Close`
completes it. Only normal returns with no error result or an explicit nil
built-in error result count as success. Transfers, aliases, asynchronous calls,
and unknown error results remain conservative. The generic
`resource-not-closed` rule delegates these exact constructors so configuration-
only writers and failed output paths do not receive a competing
close-on-every-return diagnostic.

The restriction catalog provides exact-ID policies for explicit blank error
discards, direct panic, process termination, root background contexts, and
placeholder contexts. It is not a selectable preset: deliberate best-effort,
invariant, executable-boundary, detached-work, and compatibility cases require
reasoned suppressions, and test files remain separately configurable for every
rule.

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

The `nil-error-wrap` SSA rule now also consumes the shared return-state facts
used by `nilness`. For one direct tuple call, a sibling result's exact nil or
non-nil branch can prove the error nil only when that state contradicts every
non-nil-error return summarized for the selected local helper. Exact edge
dominance keeps loop back-edges conservative. Dynamic calls, phis, delegated
results, conflicting summaries, and unavailable dependency facts do not
report.

The lifecycle track now admits `sql-transaction-not-completed` as a default
correctness CFG rule for direct `database/sql` transactions. After a
conventional successful acquisition guard, every normally returning path must
call the exact transaction Commit or Rollback method or conservatively transfer
ownership. Reassignment loses the original obligation; wrapper constructors and
arbitrary finalizers remain outside the initial contract.

The transaction state track also admits
`sql-transaction-used-after-completion` as a default correctness CFG rule.
After the same proven acquisition boundary, exact Commit, Rollback, and
guaranteed helper effects establish completed state. A direct transaction
operation or repeated completion reports only when every reaching path is
already completed. Conditional completion, aliases, transfers, asynchronous
use, deferred calls, and multiple transaction calls in one CFG node remain
conservative instead of assuming order or ownership.

The first shared obligation/effect layer now summarizes bounded intraprocedural
CFG paths as open, completed, transferred, or lost. Both SQL transactions and
local `Close() error` resources use it; `resource-not-closed` therefore reports
partial cleanup and reassignment instead of accepting one close anywhere in a
function. Shared CFG and SSA construction now propagates no-return behavior
through statically called functions and methods in the loaded package and exact
terminal APIs from `os`, `runtime`, `syscall`, `log`, and `testing`. Same-module
facts and configured project contracts now provide exact completion, transfer,
and no-return behavior. Direct static receiver summaries now discharge generic
resource obligations and establish closed state for later use-after-close
analysis, including method expressions without treating the receiver as an
ordinary parameter. Dynamic dispatch, conditional effects, and promoted
methods remain conservative; speculative third-party inference remains
excluded.

The response lifecycle track now also admits
`http-response-body-not-closed`. It follows exact `net/http` package and Client
acquisitions after a conventional error guard, requires close or transfer on
every normal return, and does not mistake body reads for ownership transfer.
Arbitrary helper cleanup, custom transports, and body replacement remain in
the interprocedural investigation rather than weakening the local rule.

The HTTP response state track now admits
`http-response-body-used-after-close` as an opt-in suspicious rule. The same
acquisition boundary seeds an open body state; direct close and guaranteed
helper effects establish closed state, and direct reads, selected exact `io`
consumers, and repeated closes report only from an all-path closed state.
Conditional close, aliases, transfer, reassignment, deferred or asynchronous
execution, and unknown helpers fail closed. The rule is intentionally absent
from `recommended` until external positive evidence and broader negative
dogfood justify changing that curated profile.

The channel state track now admits `channel-used-after-close` as a default
correctness rule. Direct local channels initialized by the exact built-in
`make` function move through untracked, open, closed, and conservative unknown
states. Sends and repeated exact closes report only from an all-path closed
state; a later exact close reestablishes closed state on its normal
continuation. Conditional closure, nonlocal channels, aliases, helper calls,
closure capture, asynchronous execution, and ambiguous multi-operation nodes
fail closed. Receives remain legal, deferred close is not applied at
registration, and reacquisition establishes a new open channel.

The WaitGroup state track now admits `waitgroup-negative-counter` as a default
correctness rule alongside the existing `waitgroup-misuse` analyzer. Direct
local `sync.WaitGroup` values and pointers initialized from the exact zero
value carry bounded counter states through CFG joins. Constant `Add` and direct
`Done` calls report only when every reaching counter state underflows. A
`Wait` continuation establishes zero only when the represented path can return;
an exact positive local counter with no escape stops propagation instead of
creating diagnostics in unreachable code. Aliases, helpers, closure capture,
asynchronous counter changes, dynamic deltas, and large counts fail closed.
Deferred counter operations are not modeled at function exit.

The first shared state-transition track now admits `unlock-without-lock` as a
default correctness rule and `lock-not-released` as an opt-in suspicious rule.
One finite monotone worklist reaches a stable CFG entry state before any
diagnostic is observed, and the three lock consumers reuse that result for each
function. Exact `sync.Mutex` and `sync.RWMutex` identities distinguish write
locks and bounded local read depth, ordinary deferred releases apply at normal
returns, and branch joins, loops, no-return calls, escapes, generated files,
type errors, suppressions, and configured blocking calls retain one contract.
The existing `lock-held-across-blocking-call` rule now consumes this CFG state
instead of one lexical statement list. `sync.Cond.Wait` is deliberately
excluded because its contract requires and temporarily releases its Locker.
Unknown helper-managed transitions, indexed receivers, and intentional
cross-function lock handoff remain conservative boundaries.

The state-transition track now also admits `resource-used-after-close` as an
opt-in suspicious rule. It follows locally acquired `Close() error` results,
reports curated direct operations only when every reaching CFG state is proven
closed, and consumes exact selected local-source module or configured close
effects on parameters and direct receivers. Proven
ownership transfer and every helper without an exact close effect stop state
tracking: an ownership borrow does not prove that a helper left the resource's
internal state unchanged. Deferred or asynchronous closes do not establish an
immediate closed state, and a direct reacquisition establishes a new open
state. Aliases, nested tracked calls, arbitrary methods, and uncertain joins
remain conservative. No fix guesses whether the operation, close, or
acquisition should move.

The pedantic track now also admits `empty-branch`, `manual-min-max`, and
`redundant-type-declaration`. These rules retain narrow syntax or exact-type
contracts, remain opt-in, and avoid claiming that a readability preference is
a correctness defect. Only redundant type spelling has a safe fix, and comment
ownership can withhold that edit.

The standard-library correctness track now admits `invalid-random-bound` for
exact `math/rand` and `math/rand/v2` bounded calls. Constant nonpositive bounds
panic, while a bound of one always returns zero. Unknown values, function
values, interface dispatch, and local lookalikes remain excluded, and no fix
guesses the intended result domain.

The same track now admits `zero-replace-count` for exact `strings.Replace` and
`bytes.Replace` calls. Compile-time zero counts replace no occurrences, while
nonzero and dynamic counts, function values, and local lookalikes remain
excluded. No fix guesses whether the caller intended one, some, or all
replacements.

The regexp correctness track now admits `invalid-regexp` and
`zero-regexp-match-limit`. Exact constant patterns are checked with the
appropriate Perl or POSIX parser under a 64 KiB work bound, while exact FindAll
methods report a compile-time zero result limit. Dynamic arguments, indirect
calls, local lookalikes, generated files, and ill-typed packages remain
excluded, and neither rule guesses a replacement.

The standard-library argument track now also admits
`invalid-strconv-argument` and `invalid-binary-write`. Exact `strconv` calls
validate constant bases, bit sizes, and format bytes, while exact
`encoding/binary.Write` calls reject statically unsupported variable-size data
shapes. Dynamic values, indirect calls, top-level interfaces, generated files,
and ill-typed packages remain conservative, and neither rule guesses an
intended representation.

The same track now admits `non-slice-sort` for exact `sort.Slice`,
`sort.SliceStable`, and `sort.SliceIsSorted` calls. Statically non-slice values
panic despite the APIs' `any` parameter, while slices and runtime-unknown
interface or type-parameter values remain conservative. No fix guesses whether
the caller intended a dereference, copy, or different data shape.

The IP comparison track now admits `net-ip-bytes-equal` for exact
`bytes.Equal` calls over two exact `net.IP` values. Raw byte equality can treat
equivalent 4-byte and 16-byte IPv4 representations as different addresses,
while `net.IP.Equal` accounts for both representations. Ordinary byte slices,
distinct named wrappers, indirect calls, and local lookalikes remain excluded,
and the behavior-changing replacement is suggestion-only. The transaction
preserves operand source and removes a qualified `bytes` import only when the
accepted edit makes its final use unused.

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

The first priority now has versioned no-return, parameter-effect,
receiver-effect, and returned nil/error facts. Enabled CFG and SSA consumers
summarize selected local-source module helpers through stable function
identities without retaining dependency source as lint targets. Exact
unconditional result states and exact nil/error return relationships may cross
bounded static delegation chains. The resource, HTTP-body, transaction, and
cancellation lifecycle rules
distinguish proven borrowing from guaranteed completion, invocation, or
ownership transfer on every normally returning path. `nilness` consumes exact
returned relationships for direct uses dominated by error checks. Broader
aliasing, dynamic delegation, workspace modules outside the selected import
graph, and downloaded third-party packages remain outside the admitted boundary
rather than being inferred from names.

Opt-in `lint/check --stats` now exposes package-loading versus analysis cost,
per-tier and per-rule process-local measurements, cache outcomes, final finding
dispositions, and the rule reasons for dependency syntax and effect facts. This
is the measurement surface for evaluating later fact-schema and catalog growth;
ordinary diagnostics retain their deterministic output without timing fields.

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
That edit retains the constant operand's exact source and comments, while the
coordinator removes a qualified `fmt` import only when the accepted edit makes
its final use unused. The `use-net-ip-equal` suggestion exercises the same
transaction with receiver precedence and fix-owned `bytes` cleanup.

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
