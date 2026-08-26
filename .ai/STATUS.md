# Glippy Development Status

- Original Phase 0-5 progress: 100%
- Active development: post-v0.1 Clippy-comparability expansion
- Stable-v1 roadmap progress: 75%
- Active stable-v1 milestone: v0.8 large-repository hardening
- v0.7 semantic depth and credible fixes: complete at revision `35e93c5`;
  focused interaction and independent-review gates passed
- v0.6 real-world rule validation: complete at analyzed revision `ee2ea4f`;
  exact corpus run `32723865179`, attempt 2, passed all 20 jobs and classified
  all 386 default and recommended entries
- v0.8 native release budgets: exact workflow `32814696805` passed all four
  supported macOS/Linux amd64/arm64 jobs and cross-runner byte-for-byte archive
  reproducibility; later workflow `32921575657` rejected its universal 120 s
  formatter latency ceiling on Darwin amd64. That runner provisionally uses
  180 s p80/360 s hard while the other targets retain 120 s/240 s; a green
  exact-candidate rerun remains required. The stable ceilings remain 250 ms
  editor latency, 2 GiB aggregate formatter RSS, 240 s typed latency, and 3 GiB
  aggregate typed RSS for the pinned workloads
- v0.8 corpus runtime bound: exploratory run `32832151515` proved that the
  formatter, safe-fix preview, four profiles, and `go vet` complete for pinned
  Kubernetes before the former 120-minute job ceiling, which cancelled the
  final Staticcheck comparison; the bounded ceiling is now 180 minutes
- v0.5 typed retained-memory attribution: complete
- v0.5 exact printf fact isolation and 2 GiB reference-host gate: complete
- v0.5 bounded incremental workspace-result reuse: complete
- v0.5 superseded editor analysis cancellation: complete
- v0.5 workspace file notifications: complete
- v0.5 memory-aware workspace-result eviction: complete
- v0.5 graph-first workspace invalidation: complete
- v0.5 metadata-only package-graph discovery: complete
- v0.5 same-package incremental typed analysis: complete
- v0.5 test-package incremental typed analysis: complete
- v0.5 import-only typed discovery: complete
- v0.5 changed local dependency incremental typed analysis: complete
- v0.5 changed dependency import-only typed discovery: complete
- v0.5 memory-aware SSA package waves: complete
- v0.5 curated strictness profiles: complete
- v0.5 transaction state transition: complete
- v0.5 channel state transition: complete
- v0.5 WaitGroup counter state transition: complete
- v0.5 receiver terminal effects and reachable local-module facts: complete
- v0.5 no-op closer lifecycle precision: complete
- v0.5 unconditional result-state facts and delegated writer success: complete
- v0.5 unconditional nil-error wrapping facts: complete
- v0.5 bounded delegated result-state facts: complete
- v0.5 bounded delegated return-relationship facts: complete
- v0.5 authoritative testing cleanup receivers: complete
- v0.5 testing termination return-shim precision: complete
- v0.5 multi-target analysis: complete
- v0.5 project semantic contracts: complete
- v0.5 state-transition correctness pack: complete
- v0.5 restriction policy and exported API documentation: complete
- v0.5 direction exit audit: engineering, canonical delivery, and selected
  `pkg/prompts` adoption complete; corrected code candidate `a4de9b3` passed
  exact-revision CI `32547111862`
- v0.5 independent review: complete for the bounded engineering milestone;
  the final source-only review reported no findings
- v0.5 native macOS/Linux amd64/arm64 release budgets: the prior per-process
  result at `724d8a2` is superseded by the v0.8 aggregate four-runner evidence
  above
- Phase 0 completed: 2026-08-09
- Phase 1 completed: 2026-08-11
- Phase 2 completed: 2026-08-13
- Phase 3 completed: 2026-08-13
- Phase 4 completed: 2026-08-13
- Phase 5 completed: 2026-08-13

The sections below retain chronological evidence from earlier checkpoints.
Embedded progress statements describe those checkpoints. Post-v0.1 expansion
does not reopen the completed original phase scale; its separate stable-v1
roadmap progress is tracked above.

Successive independent reviews found material gaps in aggregate process-tree
RSS accounting and interruption ownership, bounded LSP supersession and
shutdown, external replacement invalidation, conservative RWMutex and
WaitGroup reasoning, target-matrix suppression and partial-result accounting,
baseline and changed-code policy on incomplete output, catalog admission, and
historical evidence attribution. Candidate `2571025` corrected those
boundaries, and `a4de9b3` aligned the CLI compatibility fixture with the
conservative WaitGroup proof contract. The corrected candidate passed the
complete ordinary package and race suites, focused WaitGroup and CLI coverage,
generated-document freshness, vet, tidy, supported-target builds, default
self-check, tagged-probe compile-only checks, shell validation, and final
source-only review with no findings. Exact-revision GitHub CI
[`32547111862`](https://github.com/faustbrian/glippy/actions/runs/32547111862)
also passed on `a4de9b3`. This closes the bounded v0.5 engineering milestone.
The previous typed 2 GiB evidence measured a per-process maximum and does not
prove the aggregate concurrent process-tree ceiling. Aggregate RSS, signal,
interruption, descendant cleanup, and process-containment behavior remain
unproven and are not release evidence in this candidate.
Routine implementation and review are agent-owned; no maintainer review gate is
required. Tagging and publication remain prohibited until the complete product
is genuinely ready and the maintainer has reviewed it and explicitly
authorizes publication.

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
size, `GOMAXPROCS`, and 8 workers while retaining normalized task order;
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
provisional 250 ms reference-host budget; the probe now enforces that maximum,
while native cross-platform and cross-architecture evidence remains open.

Self-dogfood validates all 32 discovered repository files and changes 30. The
control-flow repair reduces stranded control keywords from 82 to zero, and the
receiver repair reduces 20 receiver-prefix breaks to zero. The selector repair
separates terminal call arguments from callee fit decisions and reduces 193
selector-pattern targets to two lines belonging to one intentionally broken,
deeply indented indexed chain. The complete migration snapshot is classified,
valid, and idempotent, restoring the Phase 1 exit gate. At that checkpoint,
safe filesystem, configuration, and CLI proof supported 45%; later native
performance evidence and approved external adoption closed the remaining
Phase 2 gates.

The replacement integration suite now passes on Darwin 27.0.0 arm64 with APFS
and Linux arm64 with overlayfs under Go 1.26.5. Glippy supports macOS and Linux
only; Windows cross-compilation is informational and Windows runtime evidence is
not a release gate. Network, distributed, and userspace filesystems plus forced
power-loss durability are outside the supported write/fix boundary unless
separately admitted. Current replacement guarantees remain scoped to the two
recorded platform/filesystem pairs.

An immutable external check of `go-libraries` at
`a6f1c1f66a1b754e7384da0f6e97e0b3587c5f71` selected 5,051 files. It exposed
two valid binary expressions with trailing line comments that previously
aborted formatting. The corrected binary boundary completes the same snapshot
with 4,816 formatting differences and no tool error. This is check-mode corpus
evidence, not completed adoption: generated-file refusal prevented a full-tree
write rehearsal, and the 4,816-file migration diff has not received human
readability approval.

The external `pkg/prompts` adoption now pins Glippy candidate `724d8a2` and is
integrated on `faustbrian/golib` `main` at `5eb1b997`. The immutable integrated
tree contains 90 Go files. The final 45-file integration commit includes 42 Go
files, removes golangci-lint's competing gofmt/goimports
formatters, and makes module-owned Glippy targets authoritative through the
canonical repository runner. The original approved migration retains its
recorded format, tidy, test, race, vet, documentation, lint, and nested-module
evidence. The later complete-package gate was reported historically but has no
retained immutable result artifact and is not current release evidence.
NilAway retains its documented advisory findings; operational assurance
remains separately scoped at 2/11 scenarios. The
maintainer's earlier layout and Phase 2 approval apply to the refreshed
canonical migration.

The current formatter dialect is now published with evidence-linked examples
covering width, indentation, blocks, semicolons, control flow, lists, binary
expressions, selectors, comments, directives, preserved source choices, and
write refusal. This closed the Phase 2 formatter-rule documentation item; later
native performance evidence and approved adoption closed the other gates.

Isolated Phase 3 foundation work now defines validated canonical rule metadata,
an immutable ordered registry, preset and override resolution, maximum-tier
selection, complete Go AST node interests, and one shared filtered syntax
traversal. Syntax diagnostics carry exact source identity and physical ranges,
resolve severity deterministically, reject undeclared or malformed fixes, sort
independently of rule registration, and honor generated-file eligibility. No
built-in rule has passed admission yet; the implemented lint and fix surfaces
are proven with injected registries and do not advance progress past the
incomplete 45% Phase 2 gate.

The first suppression foundation now parses one exact rule per `//gox:`
directive, assigns deterministic physical line, next-line, paired-range, and
file ownership, supports optional or required reasons, and reports malformed,
unknown, misplaced, nested, unmatched, and unclosed directives in source order.
The source-versioned application pass now filters ordered diagnostics, records
their owning directives, and identifies unused waivers without accepting
cross-file or stale ranges. The contract was refreshed against Oxc `f2125a8`.
Canonical expiry diagnostics and built-in rules remain open; the file driver
and reporters now expose the implemented suppression outcomes. Progress remains
45%.

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
and other failures that may have followed rename. The ordinary lint fix driver
and reporters now consume these transaction states. Progress stays 45%.

The first file-owned lint driver now resolves preset and severity policy,
records the maximum required tier, runs the shared syntax traversal once, and
applies the exact-source suppression index. It keeps visible, suppressed,
unused, and malformed-suppression outcomes distinct and refuses unsupported
tiers instead of silently skipping them. Lint reporters, CLI modes, fix
selection, and built-in rule admission were the next boundaries; reporters,
syntax check, and ordinary safe-fix selection are now implemented. Typed tiers
and built-in rule admission remain open, so progress stays 45%.

The first versioned lint-check JSON result now validates exact per-file source
identity, orders files and diagnostics canonically, and reports visible counts,
suppressed counts, suppression problems, unused directives, rich diagnostic
ranges, and named fix safety. It deliberately omits source snippets and edit
replacement text. The same envelope now carries fix outcomes; built-in rules
remain open, so progress stays 45%.

The first lint text renderer now binds each analysis result to its exact source
version, maps physical byte offsets to CRLF-aware 1-based line and byte-column
locations without inheriting `//line` adjustments, and validates every primary,
related, fix-edit, suppression-directive, and suppression-target range at UTF-8
boundaries. It emits canonically ordered diagnostics with related locations,
notes, help, and named fix safety while omitting source excerpts and replacement
text. Fix reporting now adds rejected-fix provenance while preserving those
diagnostic contracts. Built-in rules remain open, so progress stays 45%.

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
configured rule severity and both reporters.

The syntax-only `gox lint --fix` path now prevalidates the complete selection,
automatically chooses exactly one safe fix per diagnostic, rejects ambiguous
safe alternatives, generated files, symlink paths, stale source, and overlapping
edits, and reruns formatter plus syntax analysis before each atomic single-file
replacement. Text and versioned JSON reporting distinguish remaining
diagnostics, rejected fixes, confirmed writes, pending work, stale conflicts,
and possibly completed writes; cancellation and reporter failures disclose
earlier replacements. Suggestion and unsafe selection, package patterns, typed
loading, built-in rule admission, and dogfood signal measurement remain open,
so progress stays 45%.

The first `go/analysis` compatibility adapter now runs eligible syntax-only
analyzers through native file-interest scheduling while preserving Gox metadata,
severity, generated-file, suppression, diagnostic-ordering, and fix-safety
contracts. Each analyzer receives an isolated AST, matching file set, minimal
untyped package shell, exact single-file reads, and a run-local analyzer
descriptor. Prerequisites, facts, result types, flags, foreign positions,
undeclared fixes, and unexpected results are rejected; panics become errors.
Imported fixes default to suggestion safety, and safe mappings require an
explicit audit assertion. Diagnostic help preserves upstream category and
relative-URL resolution. Cancellation is honored before and after the
non-preemptible upstream callback. Analyzer suitability requires a maintainer
audit for assumptions that cannot be inferred from a function value. Typed
adapter support and analyzer prerequisite scheduling remain Phase 4 work, so
progress remains 45%.

The syntax scheduler benchmark now compares one direct shared AST pass, one
`ast/inspector` index plus union query, and naive per-rule walks across 1, 3, 5,
10, and 25 rules. One naive walk had a 1.81-microsecond lower median at one
rule; direct dispatch had lower medians from three through 25 rules and allocated
456-1,896 bytes per operation. Inspector indexing allocated roughly 28-30 KiB.
The production scheduler uses one direct shared pass without changing
diagnostic ordering or rule-interest semantics; the small one-rule result does
not justify a second execution path. This closes the Phase 3
traversal-strategy benchmark item but does not advance the incomplete 45%
Phase 2 gate.

The production registry now admits its first built-in correctness rule,
`duplicate-condition`. It reports each distinct repeated side-effect-free
condition once per `if`/`else if` chain with the first occurrence as related
context, excludes chains with initializers or conservatively effectful
conditions, excludes generated files, honors suppressions and severity
overrides, and offers no fix.
Current Staticcheck source, two public defect fixes, focused behavioral tests,
the public lint and explain paths, a 100-chain cost probe, and 7,466-file
non-mutating dogfood support the admission. The dogfood sample produced no
diagnostics or observed false positives; positive recall evidence remains the
focused fixtures and reviewed public fixes. This closes the first built-in rule
admission item, while Phase 2 naming, release artifacts, and approved external
adoption keep overall progress at 45%.

The production registry now also admits `ineffective-break` as the second
default correctness rule. It reports final unlabeled breaks in switch cases or
select clauses directly inside `for` and `range` bodies, including one final
conditional level, while excluding labeled breaks, breaks that skip later
clause work, generated files, and explicitly out-of-scope nesting. Go 1.26.5
vet accepted the proving defect. Current Staticcheck SA4011 source, two reviewed
public fixes, red-green behavioral tests, public lint and explain paths, a
100-loop cost probe, and 7,732-file non-mutating dogfood support admission. The
dogfood sample produced no diagnostics or observed false positives; focused
fixtures and public fixes retain the positive evidence. The rule now offers an
exact-token `remove-break` suggestion: ordinary `--fix` preserves and reports
it, while `--fix-suggestions` deletes the ineffective statement, retains
adjacent comments, formatter-normalizes the file, and reanalyzes before write.
It remains suggestion-only because removal, return, and a labeled loop exit
represent different intended repairs.
Phase 2 naming, release artifacts, platform runtime, and approved external
adoption keep overall progress at 45%.

The combined `gox check` command now performs one sorted discovery and
configuration pass, reads each file once, and runs formatting plus enabled
syntax lint rules over the same immutable source version. Text and versioned
JSON reporters order formatting differences, diagnostics, and suppression
records deterministically; incomplete JSON retains completed results while
text failures emit no partial findings. Clean, finding, configuration, source,
cancellation, and output-failure paths are non-mutating. This completes the
combined non-writing CI surface, while Phase 2 naming, release artifacts, and
approved external adoption keep overall progress at 45%.

Lint fixing now exposes independently composable `--fix`,
`--fix-suggestions`, and `--fix-unsafe` modes. Each flag authorizes only safe,
suggestion, or unsafe fixes respectively; diagnostics with multiple enabled
named alternatives fail prevalidation instead of receiving an arbitrary fix.
Every mode retains the existing source-version, conflict, reparse, formatter,
post-analysis, atomic-replacement, and reporting transaction. This closes the
explicit fix-class selection item, while Phase 2 naming, release artifacts,
and approved external adoption keep overall progress at 45%.

The first typed prerequisite loader now creates one run-owned, canonically
ordered `go/packages` graph for types, CFG, and SSA requests while rejecting
cheap-tier use. It retains deterministic diagnostics and partial typed results
across the populated package graph, with dependency syntax loaded only on
explicit demand. It covers test variants, build tags, overlays, GOOS/GOARCH,
and multi-module workspaces. Module resolution is read-only by default with an
explicit vendor mode. Ordinary loads stay offline by disabling proxy and direct
VCS resolution, checksum lookup, toolchain download, and ambient external
package drivers. Explicit network opt-in uses only the caller-supplied Go
environment. The loader itself does not construct CFG or SSA, schedule facts,
cache results, or integrate CLI package patterns.

Typed package parsing now binds every selected or dependency AST to the exact
post-overlay bytes supplied by `go/packages`. A canonical run-owned source
index retains immutable digests, tokens, trivia, directives, and diagnostic-only
invalid files; duplicate package variants share one source identity, and
incompatible bytes for the same path fail instead of racing a later filesystem
read.

The first native types-tier runner now dispatches declared AST node interests
through one shared traversal per selected physical root file. Rules receive the
owning package's shared type package and info, opaque package identity,
type-error state, and exact captured source; physical package positions reject
cross-file and invalid ranges. Generated and ill-typed package eligibility is
explicit per rule, invalid diagnostic-only sources are skipped, and ordinary
package ownership prevents duplicate production-file diagnostics across test
variants. Package-wide rules, dependency analysis, typed fixes, facts, and
caching remain open.

The first suppression-aware typed package driver now resolves one maximum-tier
plan, performs one typed load, runs syntax, types, CFG, and SSA rules only at
their declared tiers, combines their exact-source diagnostics, and applies
suppressions once per selected physical root. It retains canonical load and
type diagnostics, source-model problems, and valid partial file results;
rejects unsupported lexical rules before loading; and leaves every
syntax-only caller on the existing path that cannot invoke `go/packages`. CLI
package patterns, per-path configuration across package boundaries, typed
fixing and post-fix reloads, package-wide rules, dependency analysis, facts,
and caching remain open, so progress stays 45%.

Typed package reporting now retains the immutable load-owned source index for
physical rule locations and exposes package/type diagnostics and source-model
problems as separate deterministic text and versioned JSON channels. Machine
package kinds are stable `unknown`, `list`, `parse`, and `type` values; source
problems retain normalized absolute paths and captured source digests; summary
counts remain distinct from rule findings and generic tool errors. CLI
package-pattern routing and exit handling remain open, so progress stays 45%.

The non-writing lint CLI now accepts terminal `...` filesystem patterns,
retains syntax-only invocations on deterministic file discovery, and routes a
types-or-higher selection through one read-only, test-aware package load when
every input shares one project root and configuration. Typed text and JSON
reporting preserve rule, prerequisite, and source-model channels; required package or
source problems exit with source error while complete partial results remain
reportable. Heterogeneous typed configuration and typed fixes fail before
loading or mutation. No built-in typed, CFG, or SSA rule, per-path package
policy, fact scheduler, or typed cache exists yet, so progress stays 45%.

The first native CFG-tier runner now visits every selected function declaration
and nested function literal once in physical source order, constructs one
`x/tools/go/cfg` graph per eligible function, and shares it across CFG rules in
rule-ID order with the load-owned type information and exact source identity.
Generated-file and ill-typed eligibility match typed-rule policy, production
files are not duplicated across test variants, cancellation is preserved, and
syntax/types/CFG diagnostics are combined before suppression. The conservative
no-return policy recognizes only the predeclared `panic`; interprocedural
no-return facts, short-circuit edges, abnormal panic flow, and built-in CFG
rules remain open. The incomplete Phase 2 naming, release, platform, and
external-adoption gates keep overall progress at 45%.

The first native SSA-tier runner now filters the selected roots to well-typed
packages containing eligible source, builds one run-owned `x/tools/go/ssa`
program, and shares its packages and functions across SSA rules. Package
initializer closures, declarations, methods, and nested function literals map
back to exact physical source callbacks; synthetic wrappers and
range-over-function helpers do not. Generated-file policy, test-variant
ownership, cancellation, deterministic diagnostics, CLI routing, and
suppression after combined syntax/types/CFG/SSA analysis are proven. Ill-typed
packages retain load diagnostics but cannot opt into SSA callbacks. Built-in
SSA rules, facts, caching, package-wide rules, and typed fixes remain open, and
the incomplete Phase 2 gates keep overall progress at 45%.

The production registry now admits its first deep correctness rule,
`nilness`, under the opt-in `suspicious` preset. It runs the current x/tools
v0.48.0 nilness analyzer over Gox's already-shared SSA function rather than
constructing another program, maps upstream point diagnostics to exact
physical tokens, and fails closed if the analyzer prerequisite or diagnostic
shape changes. Default Go 1.26.5 vet accepted the proving defect, while focused
tests cover dereference, comparison, panic, conversion, negative, generated,
type-error, suppression, severity, no-fix, lint, and explain behavior. Explicit
non-mutating dogfood over 335 Gox and x/tools files produced no findings or
tool failures. The rule remains opt-in because its default signal and cost have
not been accepted and typed fixes remain unsupported. Overall progress remains
45% behind the existing Phase 2 gates.

Combined `gox check` now shares the tier-sensitive lint plan. Syntax-only
selections retain physical discovery, while types, CFG, or SSA selections use
one read-only, test-aware package load. Formatting consumes the same immutable
load-owned bytes as every analysis tier, so no later filesystem read can split
their source identities. Text and versioned JSON report formatting differences,
deep diagnostics, package prerequisites, and source-model problems without
mutation; exact digests bind each format result and diagnostic. Terminal `...`
patterns now work for combined checks, and focused SSA, type-error,
invalid-source, reporter, and non-mutation coverage passes. Progress remains
45% behind the existing Phase 2 naming, release, platform, and approved
external-adoption gates.

The typed configuration now exposes suppression reason policy through
`lint.suppressions.require-reason`, defaulting to optional reasons. Enabling it
invalidates direct suppressions and range starts without a non-empty reason,
leaves their diagnostics visible, and reports the existing source-located
`missing-reason` problem. One resolved configuration value now reaches
syntax-only lint, combined check, syntax fix prevalidation, and the shared
types/CFG/SSA package driver. Focused configuration, text, JSON, source-digest,
and non-mutation fixtures cover every routed command. Structured suppression
expiry remains deferred, and the incomplete Phase 2 gates keep progress at
45%.

Suppressions now accept an optional leading `expires=YYYY-MM-DD` reason field
and retain the date as structured directive metadata. Invalid calendar dates
and waivers expiring on or before the typed
`lint.suppressions.expiry-cutoff` become source-located problems, cannot hide
diagnostics, and are consistent across syntax and package analysis. The cutoff
is an explicit reproducible configuration input; Gox never reads the wall
clock. Text and versioned JSON surface expiry problems, while unused JSON
records include `expires_on`. Current Oxlint source still has no structured
expiry contract. The incomplete Phase 2 naming, release, platform, and approved
external-adoption gates keep progress at 45%.

The `go/analysis` compatibility boundary now admits explicitly audited
read-only types-tier analyzers over the existing package load. Adapted passes
reuse load-owned AST, type information, sizes, module metadata, type errors
when admitted, and exact captured source bytes after native types, CFG, and SSA
consumers finish. Package and analyzer order is deterministic; physical files
are owned once across test variants; synthetic test-main cache source is not a
lint target; generated and ill-typed eligibility remains native metadata
policy; and cross-file related locations or fixes fail closed. Facts, analyzer
flags, CFG/SSA adapter tiers, typed fix application, and mutable analyzers
remain unsupported. The incomplete Phase 2 gates keep overall progress at
45%.

Audited typed `go/analysis` adapters now support deterministic prerequisite
result DAGs. Each prerequisite runs once per package, shared nodes are reused,
direct `ResultOf` maps preserve analyzer identity, result types are validated,
metadata-less prerequisite diagnostics fail closed, and cancellation stops
dependent callbacks. A real x/tools atomic analyzer proves inspector-result
interoperability. Facts and analyzer flags remain unsupported, and the open
Phase 2 gates keep overall progress at 45%.

Typed `go/analysis` DAGs now support deterministic package facts across sorted
import dependencies. Dependency syntax is loaded only for fact analyzers;
shared dependency packages execute once; facts retain analyzer, package, and
declared-type identity; Gob snapshots isolate later mutation; enumeration is
limited to the current package and direct imports; and only selected root
diagnostics are reported. The real x/tools `pkgfact` analyzer proves the
dependency boundary. Undeclared or nondeterministically encoded facts,
ill-typed prerequisite incompatibility, cancellation, and every object-fact
operation fail closed. Object facts and persistence are described by the later
foundation entries; analyzer flags and the open Phase 2 gates remain, so
overall progress stays 45%.

Typed `go/analysis` DAGs now also support run-local object facts. Exact object
identity and declared fact type key each isolated Gob snapshot; exports are
limited to non-nil objects owned by the current package; dependency views
propagate deterministically through the current x/tools export-data
overapproximation; and enumeration returns independent facts in canonical
physical and encoded-value order. A real transitive x/tools `ctrlflow`
prerequisite proves a no-return method fact across an intermediate type alias.
Nil and foreign objects, undeclared types, and nondeterministic encodings fail
closed. Persistent fact serialization and cache invalidation remain open, and
the Phase 2 gates keep overall progress at 45%.

The Phase 4 persistent-cache foundation now provides versioned canonical keys
for toolchain, language, configuration, rule, build-selection, environment,
source, module/workspace, overlay, dependency-export, fact, and formatter-mode
inputs. Its rooted store bounds entries, verifies embedded key, length, and
payload digest, treats corruption as a miss, repairs through recomputation, and
uses create-if-absent hard-link publication so concurrent different values fail
instead of silently replacing one another. The analysis consumers and typed CLI
lifecycle are described below; formatter caching, broader platform evidence,
and broader warm-cache benchmarks remain open.
Progress stays 45% behind the Phase 2 gates.

Persistent object facts now have process-independent identity through an owning
package path and canonical x/tools `objectpath`. Package objects, named types,
methods, fields, type parameters, parameters, and results resolve to the exact
corresponding object after an independent type check; nil, predeclared, local,
unexported package variables, mismatched packages, and malformed paths fail
closed. The fact-bearing consumer below now uses this identity; broader cache
consumers and performance evidence remain open. Progress stays 45%.

Package fact snapshots now encode one analyzer-package pair with a version,
analyzer and package identity, stable declared fact types, canonical object
paths, and deterministic Gob values. Encoding is canonical and bounded;
restore validates the full payload and all object paths before merging, rejects
different live values, and leaves unsupported local-object packages
uncacheable instead of producing partial warm behavior.

The first persistent analysis consumer now caches fact-bearing typed
`go/analysis` packages behind an explicit caller-owned store and complete
identity inputs. One loaded-graph manifest binds source, module/workspace,
selection, environment, overlay, export, toolchain, configuration, rule, and
formatter state; dependency package keys carry imported-fact invalidation into
each parent. Canonical entries restore diagnostics plus every analyzer-step
fact snapshot transactionally across independent type graphs. Cold population,
warm hits, source invalidation, stale/corrupt refusal, dependency-first restore,
and uncacheable-local-fact fallback are proven. A five-sample owned workload
probe executes 42 analyzer packages per cold population and zero per warm
independent load. Its 1.32-second cold and 478-millisecond warm medians are
directional only because the host was not isolated and package loading still
dominates allocations. The typed CLI integration is described below, and no
product-wide latency threshold is claimed. Native-tier caching is described
below. Phase 2 naming, release, platform-runtime, and approved
external-adoption gates keep overall progress at 45%.

The cache store now supports explicit bounded pruning over canonical entries.
It removes verified corruption before evicting the oldest publication times to
caller-supplied entry and encoded-byte limits, with key-order ties. Unknown and
non-stale temporary files remain untouched; cancellation, deleted roots, count
and byte limits, deterministic ties, and concurrent equal publication across
independent store handles are covered. The typed CLI policy invokes this pruning
and stale publication recovery as described below; progress stays 45% behind
the Phase 2 gates.

Rule configuration now accepts strict per-rule boolean, integer, string, and
string-list values under `lint.rule-options`. Canonical metadata rejects unknown
or mistyped fields, requires canonical defaults for optional fields, and rejects
defaults or missing values for required fields before source traversal.
Immutable resolved option snapshots reach syntax, types, CFG, and SSA native
contexts and are bound to each discovered configuration in the CLI. Adapted
analyzers still reject flags and unbound native options until an isolated flag
instance contract is proven. Persistent analysis keys derive option digests,
including resolved defaults, from those same snapshots rather than caller
assertions. Progress remains 45% behind the Phase 2 gates.

The `go/analysis` adapter now accepts flagged syntax and typed analyzer graphs
only through a factory that proves distinct, contract-identical instances.
Every boolean, signed-integer, or string flag maps one-to-one to a native typed
rule option; prerequisite flags share the same bound typed graph, while
independent syntax and package invocations receive fresh state. Admission and
runtime checks reject shared admission graphs, reuse of those probes, topology
or metadata drift, unbound or mistyped flags, string lists, nil instances, and
factory panics. Detectable aliases between flag value stores are rejected
instead of making binding order observable. The adapter also contains flag
getter and setter panics and never mutates a shared analyzer flag set. Progress
remains 45% behind the Phase 2 gates.

Native types-tier rules may now choose node-scoped shared traversal or one
package-wide callback without constructing CFG or SSA. Package callbacks see
the complete valid compiled package in physical-path order but may report only
through exact target descriptors owned by that package and admitted by the
generated-file policy. Ordinary packages retain production ownership while
augmented test variants own only test files; foreign, stale, and non-owning
targets fail closed. Typed options, partial-type eligibility, deterministic
ordering, suppression, CLI reporting, and exact source identity remain shared
with node-scoped rules. Native dependency analysis remains open; typed fix
application is described below. Phase 2 naming, release, platform-runtime, and
approved external-adoption gates keep overall progress at 45%.

The naming audit now has a concrete technical recommendation: **Gofettle**,
binary `gofettle`, and proposed module path
`github.com/faustbrian/gofettle`. Exact repository and common package-manager
names were clear in the 2026-08-11 screen, while `gofettle.dev` had no RDAP
record and `gofettle.com` was registered. This narrows the naming gate but does
not close it: maintainer approval, a fresh pre-rename namespace check,
professional trademark review, and explicit authority for external repository
or namespace changes remain required. Progress stays 45%.

The opt-in persistent analysis cache now covers native types, package-wide
types, CFG, and SSA rules as one canonical pre-suppression result over the
complete error-free loaded graph. Warm independent loads revalidate the
complete selected rule set and execution metadata, exact source identity and
physical package ownership, ranges, fixes, and ordering before bypassing all
native callbacks and CFG or SSA construction.
Cold population, identical warm results, storage-corruption recomputation and
repair, source invalidation, zero-diagnostic rule-policy changes, and package
ownership drift are proven across all four native callback shapes. Error-bearing
loads remain uncached. A five-sample owned workload probe now measures cold
population and warm restoration at the types, CFG, and SSA maximum tiers while
retaining package loading in both paths. Every warm sample executes zero native
callbacks, proving reuse for node-scoped types, package-wide types, CFG, and SSA
rules; the 112-150 millisecond warm medians are not consistently faster than
the 118-125 millisecond cold medians on the small workload and non-isolated
host, so no CI or product-wide performance threshold is claimed. The typed CLI
lifecycle is described below. The Phase 2 naming, release, platform-runtime,
and approved external-adoption gates keep overall progress at 45%.

Typed `gox lint` and combined `gox check` may now opt into one invocation-owned
persistent analysis store through strict versioned configuration. The CLI
binds explicit GOOS, GOARCH, CGO, `GOENV=off`, tool, Go, source-language,
formatter-mode, rule, and result-affecting canonical configuration identity;
development binaries use their executable SHA-256 digest instead of sharing
the generic `devel` display version;
reuses one store for the complete package command; prunes canonical entries to
configured count and encoded-byte limits after non-canceled runs; and closes
the store before reporting. The same pass removes only canonical publication
temporaries strictly older than 24 hours while preserving newer and unknown
files. Cache enablement and retention limits do not cause cold result misses.
Syntax-only linting, formatting, and lint fixing remain cache-independent. Cold
lint followed by warm combined check produces the same diagnostics with zero
additional typed callbacks, and CLI tests cover pruning, stale-temporary
recovery, cancel-without-prune behavior, syntax-command independence, invalid
limits, roots inside the project, and cache-open failure categories.
The current source-language identity remains the documented Go 1.26 prototype
policy. Cache-root validation resolves existing symlink ancestry before open,
but broader platform runtime evidence remained open at this point. Progress
stays 45% behind the Phase 2 naming,
release, platform-runtime, and approved external-adoption gates.

Cache-root admission now resolves one immutable prospective target, validates
that target outside the selected project before creating directories, and
opens every resolved component through pinned rooted handles with identity
checks. A deterministic symlink-swap regression proves that changing the
caller-supplied link after validation cannot redirect cache writes into the
project, while rejected roots create nothing. This closes the
validation-to-open race in the current Darwin runtime evidence. A later
network-isolated Linux arm64 overlayfs rehearsal passes the focused root
pinning, escaping-shard, cache reuse, pruning, invalid-root, and failure-mapping
cases, then the complete cache and CLI package suites with and without the race
detector. This closes Linux runtime evidence for the current cache-root and CLI
lifecycle boundary. Windows and unrecorded storage drivers remain open, and the
Phase 2 naming, release, platform-runtime, and approved external-adoption gates
keep overall progress at 45%.

The Phase 2 prototype release builder now produces path-trimmed, cgo-free
Darwin and Linux binaries for amd64 and arm64, with `GOAMD64=v1` and
`GOARM64=v8.0` pinned and explicit linked versions inside normalized tar/gzip
archives. A versioned manifest binds the complete source
revision, exact Go toolchain, target, size, and SHA-256 digest; a sorted checksum
file covers every archive and the manifest. The builder verifies exact `HEAD`
with no tracked, untracked, or ignored content, rejects external local module
replacements, builds from an immutable Git archive with a fresh invocation-owned
Go cache, disables external cache programs and FIPS source substitution, and
refuses existing output. Git validation and export ignore ambient repository
routing and user/system configuration. Artifact writes and cleanup use a pinned
private directory, followed by atomic no-replacement publication to the
requested path. All fallible output closing and source/cache cleanup completes
before publication, preventing a returned cleanup error from coexisting with a
published release. The builder disables ambient Go workspace, environment-file,
toolchain-download, and implicit VCS inputs and performs no signing or remote
publication. Integration evidence builds the complete target set twice,
compares every output byte, verifies the archives and checksums, and executes
the extracted current-host binary's version command. An independent
network-isolated Linux arm64 rehearsal at exact revision `c0a15b5` also built
both targets, validated every checksum, and executed the Linux archive. Its two
archives, manifest, and checksum file were byte-identical to an independent
Darwin arm64 build with the same Go 1.26.5 toolchain and linked version. This
closes the Docker/Linux-environment artifact rehearsal for the earlier
arm64-only target set.
A later exact-revision rehearsal built the four-target set independently on
Darwin arm64 and emulated Linux amd64, validated both checksum sets and archive
modes, matched all six files byte-for-byte, and executed every target binary on
its declared operating system and architecture. Darwin amd64 used Rosetta,
Linux amd64 used Docker architecture emulation, and both arm64 executions were
native to their host architecture. This closes the four-target prototype
artifact evidence without claiming native amd64 host support or reproduction on
a separate physical host. The final name, signing/publication, Windows runtime,
and approved external adoption remain open, so progress stays 45%.

Typed, CFG, and SSA lint selections may now apply admitted single-file fixes.
The fix driver builds its plan from a fresh cache-independent, read-only,
test-aware package analysis; binds diagnostics to the load-owned physical source
and an equal rooted filesystem snapshot; refuses generated and
symlink-traversing sources; and retains the existing stale, overlap, formatting,
and atomic-replacement boundaries. Before each serialized file transaction it
reanalyzes and reselects against the current package graph, preventing an
earlier write from making a later typed selection semantically stale. Every
formatted candidate is then reanalyzed through an exact-path package overlay
before replacement. Package diagnostics, source-model failures, missing target
results, and overlay identity mismatches become validation rejections that
preserve the original file, while package engine failures remain tool failures.
One final fresh package analysis replaces every per-file reporting result, so a
later write cannot hide a newly enabled finding in an earlier file. Transactions
are single-file and do not claim multi-file atomicity. Focused end-to-end
fixtures prove a safe typed rewrite reaches formatted, diagnostic-free output,
a syntax-valid rewrite introducing an undefined identifier is rejected without
mutation, cross-file selections are refreshed, final cross-file findings remain
visible, stale package snapshots conflict, and symlinked-directory sources are
refused.
Persistent lint caching is intentionally bypassed for planning, reselection,
and post-fix validation. The Phase 2 naming, release-platform, publication, and
approved external-adoption gates remain open, so progress stays 45%.

Native package-wide types rules may now declare dependency-syntax access in
canonical metadata. The package scheduler requests dependency syntax only when
an enabled native declaration or adapted fact graph requires it, and each
declaring callback receives the complete transitive graph in deterministic
dependency-first order. Dependency descriptors share load-owned package, type,
size, position, and exact-source state, but every dependency file is a
non-target; undeclared rules receive no dependency view even within the same
shared load. Registry admission excludes node-scoped and non-types declarations,
`gox explain` publishes the requirement, and native cache snapshots bind it
while existing load identity binds dependency source changes. This closes the
native dependency-analysis boundary. Built-in rule admission and the Phase 2
naming, release-platform, publication, and approved external-adoption gates
remain open, so progress stays 45%.

The production registry now admits its first native types-tier built-in,
`context-key`, under the opt-in `suspicious` preset. It identifies the standard
library `context.WithValue` by typed object identity and reports built-in key
types, aliases resolving directly to built-ins, anonymous empty structs, nil
keys, and statically non-comparable keys. Package-defined comparable types,
named empty structs, aliases to named key types, pointers, non-empty anonymous
structs, and unresolved interface or type-parameter values remain accepted. The
rule still reports type parameters whose single structural restriction proves
every permitted type non-comparable. It excludes generated and ill-typed
packages and offers no fix. Go 1.26.5 vet accepted the proving defects. Current
Staticcheck SA1029, three reviewed public fixes, red-green behavioral tests,
the public lint and explain paths, a 100-call
cost probe, and non-mutating dogfood support admission. Gox selected 109 files
without diagnostics; the root selection of an immutable go-libraries snapshot
selected 16 files without diagnostics and does not represent its nested
modules. The incomplete Phase 2 naming, release-platform, publication, and
approved external-adoption gates keep overall progress at 45%.

The production registry now also admits its first CFG-tier built-in,
`defer-in-infinite-loop`, under the opt-in `suspicious` preset. It reports live
defers lexically enclosed by conditionless loops only when the shared function
CFG cannot reach a return, built-in panic, or `runtime.Goexit` after the defer.
This distinguishes real loop exits from breaks owned by nested switches or
selects, excludes nested function bodies, unreachable defers, generated files,
and ill-typed packages, and offers no fix. Go 1.26.5 vet accepted the proving
defect. Current Staticcheck SA5003, a reviewed public repair, red-green
behavioral tests, public lint and explain coverage, a 100-function CFG cost
probe, and non-mutating dogfood support admission. Gox selected 111 files
without diagnostics; an immutable `go-libraries/pkg/clock` snapshot selected
18 files without diagnostics. Overall progress stays 45% behind the Phase 2
naming, release-platform, publication, and approved external-adoption gates.

Formatter acceptance now proves Gox suppression ownership rather than only
directive identity and neighboring-line anchors. Every structurally valid line,
next-line, paired-range, and file suppression retains the same normalized token
ordinals across formatting, including rule IDs absent from the current registry;
malformed syntax retains ordinary byte and anchor protection without acquiring
a target. Ownership drift rejects complete-file and fragment output before any
bytes are returned, so stdout, check, diff, write, combined check, and lint-fix
normalization share one refusal boundary. Focused fixtures cover direct scopes,
paired ranges, file scope, malformed and unregistered directives, every
formatter mode without mutation, and full fix rollback. Progress remains 45%
behind the Phase 2 naming, release-platform, publication, and approved
external-adoption gates.

The production registry now admits `redundant-bool-comparison` as the first
opt-in `style` rule and the first built-in safe fix. The types-tier rule reports
equality and inequality comparisons against compile-time boolean constants
only when the other operand is statically boolean. Its `simplify-comparison`
fix preserves exact operand source and precedence, refuses comment loss, and is
offered only when the retained expression has predeclared or untyped boolean
type; retained defined boolean values are excluded because the comparison may
intentionally normalize an interface value's dynamic type. Focused red-green
behavior covers exact edits,
type and trivia boundaries, public JSON and explain output, formatter-
normalized typed fixing, reanalysis, and idempotency. Current Staticcheck S1002,
the Go 1.26.5 vet boundary, a 100-finding shared-types cost probe, 113-file Gox
dogfood, and 129-file immutable `go-libraries` dogfood support admission. The
external snapshot produced one built-in-boolean finding and correctly excluded
ten defined-boolean comparisons plus an `any`-typed comparison. Overall
progress remains 45% behind the Phase 2 naming, Windows runtime,
publication/signing, and maintainer-approved external adoption gates.

The production registry now also admits `errors-is-arguments` under the opt-in
`suspicious` preset. The native types-tier rule identifies the standard library
`errors.Is` function and reports a directly referenced external package global
in the first position unless the second argument is also an external package
global. Package-local globals, local aliases, fields, calls, composite
expressions, lookalikes, generated files, and ill-typed packages remain
excluded. The rule offers no fix because its deliberately narrow heuristic does
not prove that swapping arguments preserves caller intent. Go 1.26.5 vet
accepted the proving defect. Current Staticcheck SA1032, its historical
both-external-global exclusion, two reviewed public repairs, red-green
behavioral tests, public lint and explain coverage, a 100-finding shared-types
cost probe, and non-mutating dogfood support admission. Gox selected 115 files
without diagnostics; an immutable `go-libraries/pkg/wsdl/...` snapshot at
`1be04c0e6f17f587dc6083b701467620b95d511d` selected 53 files without
diagnostics. Overall progress stays 45% behind the Phase 2 naming, Windows
runtime, publication/signing, and maintainer-approved external adoption gates.

Package-aware `gox lint`, combined `gox check`, and typed fix planning,
reselection, and post-fix validation now share one configuration-owned build
selection. Strict `[analysis]` fields select sorted build tags, GOOS, GOARCH,
and cgo; runtime/build defaults apply when fields are omitted. Cache-disabled
and cache-enabled execution now load the same package graph with `GOENV=off`,
and all selection fields contribute to canonical result identity. Focused
red-green coverage proves a `selected && linux && cgo` file is analyzed through
cold and warm lint/check paths and safely fixed through the same selection.
Syntax-only commands remain independent of package loading and persistent
cache state. Overall progress remains 45% behind the existing Phase 2 release
gates.

ADR 0013 now closes the required editor-architecture decision for the initial
formatter integration. The stable boundary remains complete-file stdin/stdout
with truthful filepath context and visible source or configuration failures.
Current Oxfmt 0.63.0 at Oxc
`73acba93fba517cee1f584951e41d250a59de591` was reviewed: its LSP adds document
formatting, minimal edits, configuration watchers, and immutable snapshot
replacement alongside its stdin path. Gox defers that persistent surface until
formatter latency, typed diagnostics, version-bound code actions, shared cache
lifetime, or validated consumers show a material benefit. Future services must
reuse CLI engines and preserve the existing stale-edit, fix-safety, formatting,
and validation transaction. The current binary still provides no live editor
diagnostics, code actions, or LSP. Progress remains 45% behind the existing
Phase 2 release gates.

The first repeatable peak-resident-memory probe now builds with a task-owned Go
cache and normalizes Darwin and Linux `/usr/bin/time` results to bytes. Five
Darwin arm64 samples over the default repository selection measured formatter
check at a 342,622,208-byte median and 306,724,864-388,268,032-byte range. Recursive
combined check with the opt-in `suspicious` preset exercised types, CFG, and SSA
at a 407,846,912-byte median and 357,351,424-417,349,632-byte range. Every
measurement remained non-writing and the harness removed its binary,
configuration, output, and build cache. The owned repository is not a
release-scale large-module or workspace proxy, and platform `time` does not
sample aggregate simultaneous RSS across every package-loading subprocess.
These results therefore do not set a CI or product-wide memory threshold.
Progress remains 45% behind the Phase 2 release gates.

The peak-memory harness now accepts an explicit formatter-only workload root
without changing its owned typed-analysis selection. A five-sample campaign
over a temporary Git archive of `go-libraries` revision
`1be04c0e6f17f587dc6083b701467620b95d511d` selected 5,138 files and completed
with 4,904 formatting differences. Peak RSS had a 1,760,575,488-byte median and
1,659,682,816-2,057,584,640-byte range on the same non-isolated Darwin arm64
host. The immutable snapshot, binary, output, configuration, and Go cache were
removed after the run. This is one large-repository formatter result, not a
stable budget, cross-platform claim, or typed large-workspace measurement.
Progress remains 45% behind the Phase 2 release gates.

Source ingestion is now bounded by one 67,108,864-byte limit shared by complete
files and physical stdin fragments. Syntax loading, CLI file and stream reads,
typed-package parse hooks and overlays, and write/fix snapshots reject overflow
before Gox performs further cloning or parsing; regular-file snapshots use the
known size for early refusal and bounded reads still detect growth. Oversized
input is a source error, produces no formatted or partial text output,
preserves the original file, and yields incomplete machine results. The policy
is based on an immutable 5,314-file `go-libraries` audit whose largest Go file
was 1,396,160 bytes plus the current Oxfmt/Oxlint source review at Oxc
`73acba93fba517cee1f584951e41d250a59de591`. The boundary does not establish a
release-wide memory budget or bound `go/packages` package-selection reads that
precede Gox's parse hook, so progress remains 45% behind the existing Phase 2
release gates.

The maintainer has retained Gox as the development product, binary, repository,
and module identity; the earlier agent-produced Gofettle recommendation is not
adopted. A fresh collision and trademark audit remains mandatory before the
first public tag. Supported runtime operating systems are macOS and Linux only;
Windows runtime evidence is no longer a release gate. Network, distributed, and
userspace filesystems plus forced-power-loss durability are explicitly outside
the supported write/fix contract unless later admitted. GitHub Releases is the
selected publication channel, while its signing and provenance mechanism
remains a Phase 5 decision. No tag or release may be created until the complete
goal reaches 100%, its release evidence passes, and the maintainer personally
verifies and reviews it. `go-libraries/pkg/prompts` is the selected external
adoption target, but its earlier disposable 69-file migration was neither
retained nor reviewed; it must be reproduced from a current immutable revision
as a dedicated reviewable diff. These decisions remove naming, Windows, and
unbounded-filesystem evidence from the current Phase 2 input list, but stable
performance budgets and approved adoption remain open, so progress stays 45%.

Formatter lowering now gives each document arena an allocation-only capacity
hint of three nodes per physical token, capped at 8,192 nodes per render. The
arena remains growable and formatting output is unchanged. Fixed-iteration
benchmarks reduce allocated bytes by 26.6% for the editor workload, 28.7% for
100 dense loops, and 8.2% for 1,000 dense loops. Three checks over the immutable
5,138-file `go-libraries` snapshot measured a 1,742,274,560-byte median peak RSS,
inside the earlier campaign's range; this does not prove an RSS improvement or
set a stable budget. Larger reservations and retaining the first arena across
idempotency validation both materially worsened peak RSS and were rejected.
Progress therefore remains 45% behind the stable performance and approved
adoption gates.

The current `go-libraries/pkg/prompts` adoption diff exposed a signature
priority defect: a single generic type parameter broke vertically to let the
following ordinary parameters remain flat. Single uncommented type parameters
now stay attached to the function name, while the later parameter list
takes the width-driven break. Commented and multi-parameter generic lists keep
their existing comment-preserving and comma-list layouts. The change improves
seven files in the selected 77-file adoption target but does not constitute
maintainer approval, so progress remains 45%.

Release-scale formatter measurements now select an eight-worker automatic
ceiling. On the immutable 5,138-file `go-libraries` corpus, five final Darwin
arm64 samples completed in at most 8.38 seconds and 1,694,957,568 bytes peak
RSS; five Linux arm64 Docker samples completed in at most 10.86 seconds and
1,588,207,616 cgroup peak bytes. The benchmark probes now enforce provisional
per-sample maxima of 15 seconds, 2 GiB, and 250 ms for the fresh-process editor
workload. Native isolated Darwin/Linux and amd64 reproduction remains required
before the release budget is stable. Progress remains 45% behind that evidence
and maintainer-approved external adoption.

The refreshed `go-libraries/pkg/prompts` adoption diff now preserves one
source-authored blank separator between statement groups while collapsing
repeats and keeping explicit semicolon-expanded statements adjacent. All 168
exact `t.Parallel()` grouping gaps in source revision
`c60393a86b17b070b699805d1b8df99b87a7bfa6` survive formatting; the reviewable
migration falls from 69 to 65 changed files and passes a clean second formatter
check plus the module's tests, race tests, vet, and tidy-diff gates. The live
repository remains unmodified, and maintainer approval of the full migration
is still required, so progress remains 45%.

Release-budget automation now has an owned fresh-process timing driver and a
manual four-runner GitHub Actions matrix for native Darwin/Linux amd64/arm64.
The matrix pins Go 1.26.5, action commits, explicit thresholds, and public
`golib` revision `f28f85133ac6d13169745807fc39e2d5ef6bf780`; the peak-memory
probe rejects Go-host, kernel-architecture, dirty Gox or corpus trees, or
revision drift in either repository.
A local non-isolated Darwin arm64 rehearsal stayed within the provisional
limits, but the workflow has not run from GitHub and therefore supplies no
native remote evidence yet. Stable performance budgets and maintainer-approved
external adoption remain open, so progress stays 45%.

Native workflow run `31611144933` invalidated the provisional 15-second
repository-scale formatter maximum. First samples completed in 17.370 seconds
on Linux arm64, 20.470 seconds on Linux amd64, 30.130 seconds on Darwin arm64,
and 67.690 seconds on Darwin amd64 while remaining below 2 GiB; editor maxima
were 3.438 to 20.677 milliseconds. The campaign now uses a provisional
90-second maximum, giving 33% headroom over the slowest observed native sample,
and current Node-runtime action releases. A complete five-sample rerun on all
four native runners remains required, so progress stays 45%.

Native workflow rerun `31611653501` passed all four supported platform and
architecture jobs at revision `345a8de5c8dfd7980863a075123940919e7c4e63`.
Each job completed 20 editor samples, five repository-scale formatter samples,
the typed side-workload, and artifact retention. Editor maxima were 3.257 to
19.571 milliseconds, formatter maxima were 16.840 to 42.020 seconds by runner,
and peak RSS maxima were 1,305,530,368 to 1,713,582,080 bytes. This establishes
stable release budgets of 250 milliseconds for editor formatting, 90 seconds
for the pinned large formatter corpus, and 2 GiB peak formatter RSS across
native Darwin/Linux amd64/arm64. Stable performance was no longer a Phase 2
blocker; at that checkpoint, maintainer approval of the external `pkg/prompts`
adoption remained open, so overall progress remained 45%.

Publication and signing readiness now use one dormant tag-triggered GitHub
Actions workflow. An authorized canonical semantic-version tag builds the
existing deterministic Darwin/Linux amd64/arm64 archives, manifest, and
checksums with Go 1.26.5; submits every file for GitHub's Sigstore-backed signed
SLSA build provenance through short-lived OIDC identity; publishes the files as
a GitHub Release; and retains an existing candidate after later failure. Pinned
current action commits avoid the prior Node-runtime warning, and checkout
credentials are absent from the build tree. Ordinary pushes and manual
dispatches cannot invoke publication. No tag or release was created, and the
100%-plus-maintainer-review gate remains mandatory. Live tag-triggered
publication and attestation verification cannot be proven before that gate, so
they remain final release-candidate evidence. At that checkpoint, approved
external adoption still blocked Phase 2 and overall progress remained 45%.

The supported-source contract now admits Go 1.25 and Go 1.26, normalizes patch
directives to their language family, and resolves the nearest owning `go.mod`
before a root `go.work` and the documented Go 1.26 default. Malformed files and
older or newer source versions fail before formatting, linting, or writes;
`--stdin-filepath` uses the same non-writing context. Rule scheduling respects
each rule's minimum Go version, and typed cache identity uses the resolved
source version instead of a constant. Release artifacts remain targeted as Go
1.26.5 Darwin/Linux amd64/arm64 builds with no external Go runtime; Windows
remains unsupported. At that checkpoint, this closed the supported-version
policy item without changing the 45% phase gate because maintainer approval of
the external `pkg/prompts` adoption remained open.

Native release-budget workflow run `31615360856` passed all four supported
platform and architecture jobs against source-version revision `d0df995`.
Fresh-process editor maxima were 3.704 to 21.374 milliseconds; five-sample
repository formatter maxima were 16.970 to 45.630 seconds; and formatter peak
RSS maxima were 1,408,126,976 to 1,732,407,296 bytes. The existing 250
millisecond editor, 90 second large-corpus, and 2 GiB formatter RSS budgets
therefore remain valid after adding per-path Go-version resolution. At that
checkpoint, approved external adoption was the sole Phase 2 exit blocker, so
overall progress remained 45%.

The `pkg/prompts` human-review boundary is now explicit rather than represented
only by the complete patch. A compact review record defines what approval
changes, shows the intentional alignment, signature, call, literal, function
literal, condition, and blank-grouping classes, and orders the highest-value
files for inspection. Current Gox revision `d84842b` produces the committed
migration at `d6b0fba8` from baseline `8c9c1e7a`. The pinned format check,
tests, race tests, vet, tidy-diff, documentation, golangci-lint, and nested
workspace tests pass. Sixty-three selected files remain intentionally
incompatible with gofmt, and the committed migration already removes the
module's competing gofmt and goimports formatter authority. At this checkpoint,
the isolated branch was not pushed or integrated and human approval of its
complete layout was the Phase 2 exit gate, so progress remained 45%.

The binary now generates deterministic Bash, Zsh, and Fish completion scripts
through `gox completion <shell>`. Generated scripts cover the complete command
surface, command-specific flags and enum values, filesystem operands, supported
shells, and the current compiled rule IDs. Generation reads no standard input,
project, configuration, package state, or network resource and preserves
cancellation, invalid-invocation, and output-failure exit categories. The
installation guide documents each supported shell and requires regeneration
after upgrades. At this checkpoint, this closed the Phase 5 shell-completion
implementation item without advancing the then-incomplete Phase 2 adoption
gate; progress remained 45%.

The repository now publishes a normative vulnerability-reporting and product
support contract. Private GitHub vulnerability reports are the canonical
channel, with a no-details fallback for establishing private contact; scope
explicitly covers write-root escape, unsafe fix claims, directive corruption,
unexpected execution or network access, cache trust, resource-bound bypasses,
and release provenance. Before the first public release no revision is
supported; afterwards the latest stable release is supported on macOS and Linux
amd64/arm64 with the recorded source-language and filesystem boundaries. The
policy promises no response or remediation SLA and distinguishes unsupported
input from a security-boundary failure. A signed-in private-report submission
remains a final release-readiness check because the public endpoint redirect
does not prove repository private-reporting configuration. Progress remains
45% behind maintainer-approved external adoption.

The implemented CLI now has one user-facing command reference covering every
command, mode, flag, input shape, reporter boundary, fix class, write-safety
boundary, configuration entry point, and exit code. Examples distinguish
stdout, non-mutating, and mutating paths and direct users to the normative CLI,
configuration, formatter, fix, machine-schema, source-version, editor, and
completion contracts. The guide deliberately provides no installation command
or stable module-path promise because ADR 0001 requires the final collision and
trademark audit first. Progress remains 45% behind maintainer-approved external
adoption.

Continuous-integration and pre-commit adoption now have one documented
non-mutating `gox check` path. The GitHub Actions example pins the action
commits, Go toolchain, and exact reviewed Gox source revision while keeping the
tool checkout outside the selected project; the versioned Git hook shares the
same check contract and discloses that partial staging is outside Gox's
filesystem snapshot. Both paths prohibit implicit fixes and preserve the
pre-release naming and installation boundary. Progress remains 45% behind
maintainer-approved external adoption.

Contributor documentation now maps every internal package to its ownership
boundary and defines the formatter, native-rule, fix, and `go/analysis`
extension workflows. Rule authors must start from admission evidence, choose
the cheapest execution interface, keep source and analysis ownership in the
shared engines, derive `gox explain` from canonical metadata, and cover ranges,
configuration, versions, generated and type-error behavior, suppressions,
fixes, determinism, and proportional performance. The guide preserves the
closed internal extension surface and links formatter changes to corpus,
equivalence, compatibility, and user-facing documentation. Progress remains
45% behind maintainer-approved external adoption.

The public repository now has a root product overview. It explains the
hostile-source and width-aware formatter purpose, separate formatter and linter
ownership, Go-native frontend, tiered analysis, safe fix boundary, Oxfmt,
Oxlint, and Oxc reference direction, and explicit rejection of the ESLint
architecture target. The command, development-evaluation, platform,
source-version, filesystem, network, support, security, and release gates link
to their authoritative contracts without presenting the development module or
binary as a stable installation path. Progress remains 45% behind
maintainer-approved external adoption.

Every built-in lint rule now has published Markdown generated from the same
immutable metadata that drives registry validation, scheduling, configuration,
and `gox explain`. The deterministic renderer orders rule IDs, records presets,
tiers, node and package requirements, generated and type-error policies,
categories, fixes and safety, typed options, deprecation, limitations, and
paired examples, and uses source-safe code fences. A generator replaces the
catalog atomically, while a byte-for-byte freshness test prevents metadata and
published documentation from drifting. Progress remains 45% behind
maintainer-approved external adoption.

The public suppression reference now documents every implemented exact-rule
scope, source-bound ownership rule, reason and deterministic expiry policy,
problem class, unused-waiver outcome, reporter boundary, and formatter/fix
stability guarantee. README, command, and contributor documentation link the
same contract while the normative lint and configuration specifications remain
authoritative. This closes the stable-release suppression-documentation item
without changing the Phase 2 gate; progress remains 45% behind
maintainer-approved external adoption.

The public compatibility policy now defines Semantic Versioning, release-note
content, formatter-output classification, stable rule and preset evolution,
fix-safety changes, configuration and machine-schema migration, CLI and
platform compatibility, deprecation windows, and internal-format boundaries.
It preserves the 100%-plus-maintainer-review release gate and turns ADR 0008's
short compatibility decision into an actionable contributor and user contract.
This closes the formatter/rule/configuration compatibility-policy documentation
item without advancing the Phase 2 gate; progress remains 45% behind
maintainer-approved external adoption.

The public machine-output reference now defines the complete schema-version-1
consumer contract for formatter, lint, fix, typed-prerequisite, and combined
check reports. It records the common envelope, exit categories, completeness,
physical byte ranges, source digests, every file status and stable enum, fix
provenance, uncertainty after an unconfirmed rename, deterministic ordering,
and forward-compatible field handling without exposing source or replacement
text. Progress remains 45% behind maintainer-approved external adoption.

The deterministic release builder now preserves the caller's explicit
`GOMODCACHE` across its sanitized inner Go environment. The tag workflow's
task-owned module cache therefore remains the only dependency cache used by
both the outer maintainer command and every target build, and its existing
cleanup covers the complete release build instead of leaving inner builds to
fall back to the user's default module cache. No tag or release was created;
progress remains 45% behind maintainer-approved external adoption.

The current risk and decision indexes now match the implemented product rather
than their Phase 0 snapshot. They record the accepted Go 1.25/1.26 range,
implemented schema-version-1 configuration and machine-reporting contracts,
persistent cache controls, canonical test-variant ownership, current native
performance budgets, and the remaining release-candidate rerun boundaries.
Rule-noise adoption and the final name audit remain explicitly open; no engine
or release-readiness capability is inferred from documentation alone. Overall
progress remains 45% behind maintainer-approved external adoption.

On 2026-08-13 the maintainer explicitly approved Phase 2, including the
complete `pkg/prompts` layout and the migration's sole-formatter cutover. This
satisfies the final Phase 2 human-adoption gate. The production-usable formatter
phase is complete and overall progress advances to 55%. The isolated
`feature/gox-prompts-adoption` branch remains unpushed and unintegrated; that is
a delivery action requiring separate authority, not an unresolved layout or
Phase 2 capability decision. Phase 3 is now the active exit gate.

The Phase 3 exit audit now proves the complete syntax-linter and safe-fix
foundation. Fresh full tests and vet pass; focused race checks pass for rules,
analysis, fixes, reporters, and CLI; 10-second coordinator and suppression
fuzz runs complete 567,415 and 444,646 executions without failure; and the
current one-pass scheduler benchmark retains its bounded direct-dispatch
advantage at representative rule counts. The default correctness preset passes
without findings or tool failures on both Gox and the maintainer-approved
`pkg/prompts` migration, consistent with the recorded 7,732-file
multi-repository noise audit. Phase 3 is complete and overall progress advances
to 75%. Phase 4 is now active; isolated typed, CFG, SSA, and cache work does not
advance progress until that phase's complete module, workspace, correctness,
cost, and invalidation gate is audited.

The Phase 4 exit audit now proves the complete typed, CFG, SSA, fact, cache,
and package-loading foundation. Fresh full tests and vet pass; focused race
checks pass for analysis, cache, CLI, and rules; module, workspace, replacement,
vendor, build-selection, cgo, internal-package, and test-variant fixtures pass;
and current cold and warm fact, types, CFG, and SSA probes retain zero callbacks
on every warm restore. Non-mutating `suspicious`-preset lint passes without
findings or prerequisite failures on Gox and the maintainer-approved
`pkg/prompts` migration. Phase 4 is complete and overall progress advances to
90%. Phase 5 is now active; final naming, publication and provenance,
release-candidate gates, broader adoption, and the maintainer's final pre-tag
review remain open. Stable native release-scale budgets were already
established by GitHub Actions run `31611653501` and remain release-candidate
rerun gates rather than missing policy decisions.

The post-foundation rule roadmap now prioritizes admitted-rule precision,
correctness defects, cautious suspicious-rule growth, and explicitly opt-in
policy groups. It defines eight investigation tracks without reserving IDs or
promising a count, retains candidate-specific real-defect, toolchain-boundary,
false-positive, tier, fix-safety, cost, and dogfood evidence, and rejects
formatter-layout duplication or catalog copying. This closes the Phase 5 rule
roadmap deliverable without changing the 90% release gate.

Gox now adopts its own formatter across all 121 formatter-selected tracked Go
files. The root schema-version-1 configuration fixes width, tab measurement,
and the default correctness preset; the repository-owned GitHub Actions gate
builds Gox and runs its non-mutating combined check after full tests, race
tests, vet, and module-metadata validation. Complete layout review exposed
selector assignment targets whose fit decision incorrectly included the
independently breakable right-hand call; the shared assignment boundary now
preserves flat targets that fit through their operator while retaining
over-width and communication-clause breaks. The migrated tree is a
zero-difference Gox fixed point and all local gates pass with Go 1.26.5 and
disposable caches. An initial CI rehearsal
incorrectly forced `GOWORK=off`, causing the two tests that create temporary
workspaces to fail; focused evidence proved the environment override as the
first divergence, and its removal restored the complete suite. This is the
second documented repository adoption alongside the maintainer-approved
`pkg/prompts` migration. Final human review of the complete Gox layout remains
part of the maintainer's pre-tag release-candidate review rather than a reopened
Phase 2 gate. Overall progress remains 90% until the complete Phase 5
integration and release-candidate audit is recorded.

The Phase 5 integration milestone is now complete at exact revision `aa07ff4`.
Repository-owned GitHub Actions run `31686057237` passed the full test, race,
vet, tidy-diff, build, and non-mutating combined-check sequence from a fresh
checkout and empty Go caches. Editor stdin/stdout behavior, CI and pre-commit
adoption, deterministic release construction, tag-only GitHub publication and
artifact attestation, shell completion, formatter migration, support, security,
and compatibility surfaces are implemented and documented. This advances
overall progress to 95%. Final naming, release-candidate corpus and fuzz gates,
fresh four-runner performance evidence, the requirement-level acceptance audit,
and the maintainer's final pre-tag review remain open.

The release candidate is now exact revision `06cce4a` on pushed `main`.
GitHub CI run `31697040231` passed the full test, race, vet, tidy-diff, build,
and combined-check gate. The pinned 5,138-file corpus completed with 4,807
expected differences and no errors, and all twelve owned fuzz targets passed
fresh 10-second campaigns. Release-budget run `31697171821` passed natively on
Darwin and Linux amd64 and arm64: editor latency remained below 13.615 ms,
formatter checks remained below 41.700 seconds and 1,731,592,192 bytes peak
RSS, every native archive executed, and all six release files were
byte-identical across the four independent builders. Every archive contains the
0BSD project license and the deterministic MIT, BSD, and Go patent notices.
The requirement-level acceptance audit now leaves only the final public-name
collision/trademark-risk decision, the maintainer's personal candidate review,
and the subsequently authorized tag-driven attestation/publication transaction
open. No tag or release exists, so overall progress remains 95%.

The maintainer accepted the documented Gox naming and trademark-collision risk,
personally approved exact candidate `c0435d6`, and authorized the first public
release. Annotated tag `v0.1.0` resolves to that commit. GitHub Actions run
`31699926922` rebuilt the exact tag with Go 1.26.5, produced the four supported
archives plus manifest and checksums, attested all six assets through GitHub
OIDC provenance, and published a non-draft, non-prerelease GitHub Release.
Independent post-publication verification downloaded every asset, passed every
checksum, confirmed manifest source revision `c0435d6`, required the exact
`gox`, `LICENSE`, and `THIRD_PARTY_LICENSES.txt` archive entries, executed the
Darwin arm64 binary as `gox v0.1.0`, and verified all six attestations. Every
final acceptance gate is satisfied; Phase 5 and overall progress reach 100%.

Post-v0.1 development now begins the v0.2 Clippy-style lint-policy expansion.
Schema version 1 accepts order-independent composable `lint.presets`, retains
the singular v0.1 compatibility form, and supports deterministic
`lint.warnings-as-errors` after per-rule overrides. `pedantic` is selectable;
restriction rules remain exact-ID opt-ins; migration remains target-gated.
Syntax and typed package paths share the same selection contract, and
configuration/cache identity includes normalized groups and escalation.
Focused red-green evidence, the complete test suite, focused analysis/CLI race
tests, vet, and Gox's own combined check pass. ADR 0014 records the Clippy and
PHPStan boundary. Baseline support and the admitted v0.2 rule catalog remain
subsequent implementation batches; this checkpoint does not declare v0.2
complete or published.

The deterministic v0.2 baseline batch now adds strict source-free JSON
generation, rooted atomic create/update, exact source-span matching, count
aggregation, stale and explicit-cutoff expiry findings, and additive human and
schema-version-1 machine reporting. Configured baselines apply after source
suppressions across syntax, typed, combined-check, and fix-planning paths;
baselined diagnostics cannot be fixed. ADR 0015 and the public baseline
reference record the contract. Prioritized bug-catching and pedantic/style rule
admission, complete dogfood, and final v0.2 release evidence remain open, so
v0.2 is not complete or published.

The v0.2 catalog now admits `identical-branches` as an opt-in `suspicious`
syntax rule. It reports structurally identical direct `if`/`else` blocks,
conservatively excludes commented statements and `else if` chains, and offers
no fix because removing the condition could remove effects. Current Clippy and
Revive source, a public Go lint finding, red-green fixtures, a 100-pair cost
probe, and 135-file non-mutating Gox dogfood support admission. Self-assignment
remains rejected as redundant with the default Go vet `assign` analyzer. The
remaining prioritized bug-catching and pedantic/style catalog, external target
dogfood, and final v0.2 release gates remain open.

The v0.2 identity migration now uses Glippy as the product, binary, command,
module, configuration, suppression, cache, completion, diagnostic, workflow,
and release-artifact identity. Gox v0.1.0 remains immutable historical release
evidence. Automatic configuration discovery accepts `.gox.toml` for one window
only when `.glippy.toml` is absent and rejects mixed discovery; legacy `//gox:`
suppressions still apply but produce a deterministic migration finding. ADR
0016 and the refreshed collision audit record the maintainer's product-risk
acceptance and the active `F1bonacc1/glippy` collision. The GitHub repository
has not been renamed and no v0.2 tag, release, publication, or push is part of
this batch. The remaining bug-catching and pedantic catalog is still open, so
v0.2 is not complete.

The v0.2 Go-vet compatibility batch now adapts fifteen additional authoritative
x/tools analyzers behind stable Glippy IDs, correctness or suspicious preset
contracts, shared facts and prerequisites, deterministic reporters, baselines,
suppressions, generated/type-error policies, and source-version gates. All
upstream edits are suggestion-only; unambiguous printf, unreachable-code, and
host-port suggestions pass repeated validated CLI application. The published
compatibility matrix explicitly retains ten unsupported default-vet analyzers
and the partial/native differences, so Glippy does not claim full `go vet`
replacement. One-iteration per-rule cost probes pass, and non-mutating dogfood
finds no findings across 178 Glippy files and 57 prompts files at
`2c9842015ab62fd7790f0d99bf54855ffa7000f2`. The seven-rule pedantic batch,
five product-owned suggestion fixes, and final v0.2 release gates remain open.

The v0.2 second pedantic batch now admits eight independently configurable
types-tier rules: `needless-blank-identifier`, direct-delegation
`redundant-closure`, `redundant-nil-check`, separate `time-since` and
`time-until` identities, `buffer-string-conversion`, narrowly scoped
`unnecessary-format`, and `inefficient-string-comparison`. Broad no-directive
formatting initially flooded Glippy dogfood and was narrowed through a failing
noise regression to no-argument `fmt.Sprintf` calls with constant formats.
One-iteration package probes pass for every rule, and the final selection
produces no diagnostics in Glippy or `pkg/prompts` at
`f0067b6dbf812c770ec663249e9abc3f2c41d1bc`. Suggestion-only transformations
for `unnecessary-conversion`, `unnecessary-sprintf`, blank identifiers,
`time-since`, and `time-until` pass exact replacement, complete-file typed
validation, formatted application, and repeated fixed-point tests. The
requested v0.2 rule and fix direction is implemented; final v0.2 release gates
remain open and no tag, push, publication, or release is authorized by this
batch.

The v0.3 adoption work now includes exclusive `glippy init` configuration
creation plus `glippy config check` and `glippy config show`. Introspection
reports the configuration origin, source language, formatter settings, resolved
presets, enabled rules and reasons, options, maximum analysis tier, generated,
test, vendor and type-error policies, build selection, baseline status,
suppressions, cache limits, and the currently unset migration target without
loading packages or analyzing source. Bash, Zsh, and Fish completion and the
public command and configuration contracts cover the new surface. Rule
selection flags and rule discovery were the next v0.3 batch.

Exact lint selection and rule discovery are now implemented. `lint --only` and
`--except` compose after configured policy, reject malformed or unknown IDs,
drive the maximum analysis tier, and constrain checks, baselines, and fixes.
`glippy rules` filters canonical metadata by preset, fix availability, and
exact tier. `glippy explain --json` exposes the same metadata through a
schema-version-1 contract, and all three supported completion shells cover the
new commands, flags, values, and rule IDs. GitHub/SARIF reporters, editor
diagnostics and code actions, and the next pedantic/performance catalog remain
subsequent v0.3 batches.

GitHub workflow-command and SARIF 2.1.0 reporters are now implemented for lint
and combined check across syntax, typed/CFG/SSA, changed-code, fix, invalid
invocation, and failure paths. Both consume the same deterministic filtered
diagnostics as text and JSON, omit source and replacement text, and retain
existing exit categories. GitHub annotations use escaped absolute paths and
physical byte locations; SARIF uses absolute file URIs, canonical rule
metadata, and invocation notifications for tool failures. Formatter-only modes
remain text/JSON. Editor diagnostics and code actions and the next
pedantic/performance catalog remain subsequent v0.3 batches.

The v0.3 editor batch now adds a bounded stdio LSP service over the shared
frontend and engines. Full-document open and change notifications publish
versioned syntax or overlay-backed typed diagnostics; close clears them.
Formatting uses the canonical formatter, while individual fixes and safe
fix-all return exact-version whole-document edits only after conflict checks,
parsing, formatting, and reanalysis. Safe actions are the default; suggestion
and unsafe actions require independent startup flags. Persistent typed caches
include exact overlay identity, active request cancellation does not end the
session, late process-canceled results are not published, and the service never
writes source. Shell completion and public CLI, editor, and ADR contracts cover
the new surface. The next v0.3 batch is the first dedicated performance preset.

The first dedicated v0.3 performance preset now contains four warning-level,
types-tier rules: `regexp-compile-in-loop`, `sync-pool-non-pointer`,
`string-range-rune-conversion`, and `inefficient-io-string-write`. Current
Staticcheck source, focused positive and close-negative contracts, exact
ranges, shared generated/type-error/suppression/version policies, cold package
probes, and non-mutating Glippy plus `pkg/prompts` dogfood support admission.
No fix is offered where scope, ownership, indexing, comments, dispatch, or error
behavior is not generally equivalent. Complexity coverage and Clippy-style
lint-level and fix-preview UX remain subsequent comparability work.

The v0.3 lint-policy batch now adds ordered Clippy-style `allow`, `warn`,
`deny`, and `forbid` directives to `lint` and combined `check`. Exact rule IDs,
selectable groups, and the current warning set share one precedence contract;
restriction remains exact-ID-only, migration remains target-gated, excluded
rules cannot be re-enabled, and forbidden rules cannot be lowered. Syntax and
typed selections, baseline generation, fixes, text and machine reporters, and
shell completion consume the same resolved severities. Non-mutating fix
preview and calibrated complexity coverage remain the next comparability
batches.

The next Clippy-comparability UX batch makes bounded physical-source frames the
default human diagnostic form for lint and combined check. Primary ranges use
multiline underlines, tabs expand deterministically, long lines and ranges are
bounded with explicit omission markers, terminal control data is escaped, and
rejected fixes retain bounded original-source frames.
The new `--reporter=short` mode preserves the source-free
`path:line:column` form for logs across syntax and package-aware lint, combined
check, and fix reporting. JSON, GitHub, and SARIF remain source-free. Focused
red-green renderer and CLI evidence, the complete test and race suites, vet,
build, tidy-diff, non-mutating combined check, and a 100-diagnostic rendering
benchmark pass. Additional defect rules and path-scoped policy remain open, so
the broader Clippy-comparability goal remains active.

Ordered path-scoped lint policy is now implemented through strict
`[[lint.overrides]]` entries. Portable project-relative globs can enable,
disable, warn, or error exact rules for tests and subtrees; later matching
entries win before command-line lint levels and warning escalation. Syntax and
package-aware analysis share the same contract: scheduling uses the union of
potentially enabled tiers, while each physical file is filtered and assigned
its exact severities before suppressions, baselines, reporters, or fixes.
Configuration
identity retains ordered policy, explicit multi-root configurations keep
independent roots, and `config show` reports the selected path and matching
override declarations. Additional error-flow correctness rules remain open, so
the broader Clippy-comparability goal remains active.

The first post-policy error-flow batch now admits `overwritten-error` as an
opt-in suspicious SSA rule. It reports error-typed direct assignments and
initialized declarations whose value has no observable use before a later
definition of the same typed object. Tuple extraction, Phi joins, switch tags,
explicit blank observation, exact ranges, shared policies, and on-demand SSA
debug mapping are covered without expanding ordinary SSA cost. A 100-function
probe and non-mutating Glippy plus `pkg/prompts` dogfood support admission. No
fix is offered because the intended handling path is not inferable. Broader
error-flow coverage and the fixability sweep remain open, so the active
Clippy-comparability goal continues.

The next error-flow batch admits `typed-nil-error-return` as an opt-in
suspicious SSA rule. It reports explicit concrete error operands proven nil
across every incoming SSA value before conversion to an error interface.
Untyped nil, interface operands, unknown or maybe-non-nil values, bare returns,
tuple calls, generated files, and ill-typed packages remain excluded. The rule
has no fix because returning nil changes observable behavior and signature or
control-flow repairs require intent. A 100-function package probe measured a
59.3 ms median with about 2.02 MB allocated on Darwin arm64. Broader error-flow
coverage, any explicit default SSA policy, and the fixability sweep remain
open, so the active Clippy-comparability goal continues.

The first post-catalog fixability revisit adds
`remove-redundant-nil-check` as a safe fix. It retains the exact source of the
equivalent length comparison, refuses edits that would remove comments, and
preserves comments inside the retained comparison. Focused rule evidence and
the product `lint --fix` path prove metadata, exact edits, complete-file
formatting, validated writes, and second-run idempotency. Additional credible
fixes remain open. Non-mutating exact-rule lint and fix preview remain clean on
Glippy and `pkg/prompts`, so the active Clippy-comparability goal continues.

The next fixability batch adds `use-format-operand` as a suggestion-only fix for
`unnecessary-format`. It replaces only a proven no-directive `fmt.Sprintf`
call with the constant operand's exact source, withholds edits that would remove
comments, and preserves comments within a retained parenthesized operand. The
product fix path proves successful formatted application and second-run
idempotency while another `fmt` use remains; when the call is the final use,
typed validation rejects the resulting unused import and leaves the file
unchanged. The broader Clippy-comparability goal remains active.

The next lifecycle batch admits `sql-transaction-not-completed` as a default
correctness control-flow rule for direct standard-library SQL transactions.
After a conventional acquisition error guard, every normally returning path
must call the exact Commit or Rollback method or conservatively transfer
ownership; partial cleanup and reassignment report. Focused fixtures cover DB
and Conn acquisitions, branches, defers, method values, asynchronous and
wrapper transfers, source policies, and exact ranges. A 100-function package
probe averaged 76.0 ms, and exact-rule dogfood remained clean across Glippy,
prompts, migrations, capability, Vuja scoring, and Tarvero. The reusable
obligation/effect model and broader lifecycle pack remain open, so the active
Clippy-comparability goal continues.

The first reusable obligation/effect layer now bounds intraprocedural ownership
analysis by reachable CFG block state and distinguishes open, completed,
transferred, and lost obligations. `sql-transaction-not-completed` uses the
shared engine without changing its contract, while `resource-not-closed` moves
from path-insensitive types traversal to control flow: partial cleanup and
reassignment report, complete close-or-transfer branches do not, and a
conventional acquisition-error guard starts ownership only on success.
Interprocedural effects and the broader lifecycle pack remain open, so the
active Clippy-comparability goal continues.

The next lifecycle batch admits `http-response-body-not-closed` as an opt-in
suspicious control-flow rule. It follows exact `net/http` package and Client
response acquisitions after a conventional error guard and requires body close
or conservative ownership transfer on every normally returning path. A
reviewed listmonk occurrence proves the early-status and read-error leak shape;
body reads no longer masquerade as transfers, while closer-typed parameters do.
Focused eligibility and API-surface fixtures, a 100-candidate cost probe, and
non-mutating Glippy plus `pkg/prompts` dogfood support admission. Arbitrary
helper effects, custom transports, and broader interprocedural lifecycle facts
remain open, so the active Clippy-comparability goal continues.

The next error-flow batch admits `shadowed-error` as an opt-in suspicious
types-tier rule. A first error-only adaptation of x/tools `shadow` was rejected
after exact-rule dogfood produced widespread ordinary local-handling findings.
The native admitted contract reports only an inner non-nil error that breaks a
loop before the stale outer error is observed, or a deferred closure that
assigns a shadowing error instead of the named result. A reviewed cake-repl
close-error defect proves the deferred shape; focused fixtures, a 100-function
probe, a 100-to-1,000-loop scaling probe, and clean Glippy plus `pkg/prompts`
dogfood support admission. No fix is offered because declaration reuse,
renaming, and explicit propagation are not generally equivalent. Additional
interprocedural error and lifecycle facts remain open, so the active
Clippy-comparability goal continues.

The shared CFG and SSA foundation now propagates no-return behavior through
statically called named functions and methods in the loaded package. Named CFGs
are built on demand and reused across rules; SSA receives the same predicate
before program construction. This removes a false context-cancel leak path
through a local panic wrapper and lets nilness diagnose a condition whose
contrary path terminates through that wrapper. Dynamic calls, unresolved
recursive cycles, and imported helpers without source or analyzer facts remain
conservatively returning. A 100-function chain probe has a 38.3-microsecond
median with about 52.6 KiB allocated per run on Darwin arm64. Cross-package
effect facts remain the next foundation boundary, so the active
Clippy-comparability goal continues.

The next pedantic batch admits `empty-branch`, `manual-min-max`, and
`redundant-type-declaration` with narrow syntax and identical-type contracts.
Explicit empty statements count as empty, comments document deliberate
branches, floats and compound min/max forms remain excluded, and constant
defaulting cannot silently change a declaration's type. Only redundant type
spelling has a safe fix, which is withheld across comments and passes the full
fix transaction idempotently. Exact-rule dogfood found five useful manual
min/max candidates in Glippy and one in `pkg/prompts`, with no findings from the
other two rules. Cross-package effect facts and additional evidence-backed
catalog growth remain open, so the active Clippy-comparability goal continues.

The shared CFG and SSA no-return predicate now recognizes exact terminal APIs
from `os`, `runtime`, `syscall`, `log`, and `testing` without loading dependency
syntax. Exact package and function identity makes recognition independent of
import aliases while excluding local lookalikes, dynamic calls, interface
dispatch, goroutine launches, deferred calls, and nearby nonterminal APIs. This
removes an `os.Exit` false path from `context-cancel-leak` and gives `nilness`
the same impossible continuation.
Five 100-iteration probes over 100 direct terminal calls measured a
47.5-microsecond median with about 52.9 KiB and 1,051 allocations per run on
Darwin arm64. General project and third-party effect facts remain open, so the
active Clippy-comparability goal continues.

The next standard-library correctness rule, `invalid-random-bound`, recognizes
exact bounded functions and `Rand` methods from `math/rand` and `math/rand/v2`.
Compile-time nonpositive bounds report because the call panics; a bound of one
reports because the half-open result interval contains only zero. The rule is
independent of import aliases, excludes function values, interface dispatch,
local lookalikes, unknown values, generated files, and ill-typed packages, and
offers no fix because the intended domain size is not inferable. A 100-call
cold package probe measured an 80.2 ms median with about 2.11 MB allocated on
Darwin arm64. Exact-rule dogfood remained clean on Glippy and `pkg/prompts`, so
the active Clippy-comparability goal continues.

The next standard-library correctness rule, `zero-replace-count`, recognizes
exact `strings.Replace` and `bytes.Replace` calls whose compile-time count is
zero. The call replaces no occurrences, and no fix guesses whether the caller
intended one, some, or all replacements. Exact type identity excludes import
aliases as a source of ambiguity while function values, local lookalikes,
dynamic counts, generated files, and ill-typed packages remain excluded.
Current Staticcheck SA1018 and a live `diskfs/go-diskfs` path-normalization
defect support admission. A 100-call cold package probe had a 101.0 ms median
with about 2.92 MB allocated, and exact-rule dogfood remained clean on Glippy
and `pkg/prompts`. The active Clippy-comparability goal continues.

The next standard-library correctness batch admits `invalid-regexp` and
`zero-regexp-match-limit`. Exact `regexp` compilation and Match helpers now
validate compile-time patterns with the appropriate Perl or POSIX parser;
diagnostics retain the error category without copying pattern text, and a 64
KiB pattern bound limits parser work. All eight exact `*regexp.Regexp` FindAll
methods now report a compile-time zero limit because no matches can be
returned. Type identity excludes indirect calls, interface dispatch, and local
lookalikes while generated and ill-typed files remain ineligible. Five
one-iteration 100-finding probes measured 124.7 ms and about 2.75 MB for
invalid patterns, and 93.9 ms and about 2.78 MB for zero limits, on Darwin
arm64. Current Staticcheck SA1000 and SA1010 support admission; local Glippy and
Go-libraries searches found no production occurrence. Non-mutating exact-rule
dogfood remained clean on Glippy and `pkg/prompts`. The active
Clippy-comparability goal continues.

The next standard-library argument batch admits `invalid-strconv-argument` and
`invalid-binary-write` as default correctness types-tier rules. Exact `strconv`
parsing, formatting, and append calls validate compile-time bases, bit sizes,
and float format bytes; exact `encoding/binary.Write` calls reject statically
unsupported variable-size types while retaining conservative top-level
interface and type-parameter behavior. Current upstream occurrences prove both
the zero-base panic and silently omitted hash inputs. Five one-iteration
100-finding probes measured 92.2 ms and about 2.13 MB for `strconv`, and 126.2
ms and about 2.82 MB for binary data, on Darwin arm64. Non-mutating exact-rule
dogfood remained clean on Glippy and `pkg/prompts`. The active
Clippy-comparability goal continues.

The next standard-library correctness rule admits `non-slice-sort` for exact
`sort.Slice`, `sort.SliceStable`, and `sort.SliceIsSorted` calls. Statically
non-slice first arguments report because the reflection-backed API panics;
slices, interfaces, type parameters, indirect calls, and local lookalikes
remain conservative. A current `softlandia/cpd` occurrence proves the
pointer-to-pointer panic shape. Five one-iteration 100-finding probes measured
an 81.4 ms median with about 3.50 MB allocated on Darwin arm64. Non-mutating
exact-rule dogfood remained clean on Glippy and `pkg/prompts`, while the
disposable supported-version cpd clone reported the reviewed line. The active
Clippy-comparability goal continues.

The next standard-library correctness rule admits `net-ip-bytes-equal` for
exact `bytes.Equal` calls whose operands both have exact `net.IP` type. Raw
byte equality does not account for equivalent 4-byte and 16-byte IPv4
representations; aliases and dot imports remain recognized, while ordinary
byte slices, one-IP comparisons, distinct named wrappers, indirect calls, and
local lookalikes remain excluded. Current Staticcheck SA1021 and two live
DeepFlow comparisons support admission. Five one-iteration 100-finding probes
measured a 137.3 ms median with about 4.60 MB allocated on Darwin arm64. A
direct disposable DeepFlow run reported both reviewed lines, and non-mutating
exact-rule dogfood remained clean on Glippy and `pkg/prompts`. The documented
catalog reaches 100 rules, and the active Clippy-comparability goal continues.

The requirement-level Clippy comparability exit audit now closes the active
post-v0.1 expansion goal. All high-value vet integrations, the complete
semantic correctness pack through ten native and two canonical existing rule
identities, changed-code adoption, configuration introspection, selective lint
policy, GitHub and SARIF reporting, and the shared LSP diagnostic and code-action
surface are implemented. The canonical catalog contains 100 rules, including
16 pedantic, four performance, and four complexity rules, without admitting
the five remaining optional candidates whose semantic or signal contracts are
not yet credible. Glippy has reached reasonable Clippy-comparable usefulness
for Go; future evidence-backed rules and cross-package effect facts are product
evolution rather than unfinished comparability foundation.

The next fix-maturity batch gives every source-specific withheld fix structured
provenance. Native findings and diagnostics now distinguish a declared fix that
would discard comments from a rule with no fix and from a fix later rejected by
the coordinator. Canonical validation rejects undeclared, duplicate, invalid,
or simultaneously offered and withheld names; analyzer and native cache entry
version 2 preserves and revalidates the records. Text, schema-version-1 JSON,
and LSP diagnostic data expose the same name, `comments` reason, and message.
All existing native comment-withholding sites participate in the contract.

Effect schema version 2 now exports conservative per-parameter summaries for
same-module helpers alongside no-return facts. The resource, HTTP response-body,
SQL transaction, and context-cancellation lifecycle rules distinguish proven
borrowing from guaranteed completion, invocation, or ownership transfer across
every normally returning helper path. Conditional effects, unknown calls,
unsupported aliasing, and recursion remain conservative; stable identities and
the `native-effects-v2` digest make independent loads and dependency cache
invalidation deterministic. A 100-call cold package probe measured an 833.4 ms
median with about 5.84 MB and 58,789 allocations on Darwin arm64. Exact-rule
dogfood remained clean on Glippy and `go-libraries/pkg/prompts`. That batch left
returned nil/error-state facts as the next semantic-effect boundary.

Effect schema version 3 now exports conservative returned nil/error
relationships for selected-package and same-module helpers. `nilness` consumes
those facts only for direct result uses dominated by an exact built-in `error`
check, diagnosing proven nil dereferences and impossible or tautological nil
comparisons. Explicit returns must agree; bare returns, delegated or recursive
results, unknown error construction, aliases, conflicting returns, and
external modules remain conservative. Stable cross-load identities and the
`native-effects-v3` digest cover returned-state lookup and dependency cache
invalidation without making dependency source a lint target. A 100-function,
200-finding probe measured a 287.8 ms median with about 6.10 MB and 72,782
allocations on Darwin arm64. Exact-rule dogfood remained clean on Glippy and
`go-libraries/pkg/prompts` without modifying the pre-existing dirty prompts
`go.sum`.

The v0.4 fix coordinator now accepts exact path and local-name requirements
from native and adapted fixes. Requirements are validated, cloned, canonically
ordered, persisted in analyzer and native cache schema version 3, and included
when fixes are compared. The coordinator reuses exact bindings, rejects
incompatible fix or source bindings, appends only to a safely represented
single import group, otherwise adds independent declarations, and never prunes
an import required by another accepted fix. Machine fix output distinguishes
deterministic `add` and `remove` operations. The `unsafe-host-port` suggestion
proves the cross-file case by adding `net` to the diagnosed formatting file,
removing its final `fmt` import, completing typed reanalysis, and remaining
idempotent on a second run.

The v0.4 interprocedural-precision and fix-maturity audit is complete. The
current 101-rule catalog has three safe fixes, 15 suggestion fixes, and an
explicit no-fix disposition for every other rule. Package-aware fixing now
keeps unchanged generated package files as read-only inputs, refuses a selected
generated target before any write, and refreshes all remaining selections in
one package analysis after each successful change. Context-dependent untyped
shifts and defined-boolean conversions no longer receive invalid
`unnecessary-conversion` suggestions. Six pinned repositories cover CLI, HTTP,
database, concurrent-server, library, generated, and cgo workloads; reviewed
default and lifecycle findings add value beyond vet and Staticcheck, and 82
applied fix rehearsals are idempotent. Eight additional
NATS suggestions fail closed at the formatter's existing comment-ownership
boundary. The recorded Apple M4 Max budget is 40 seconds cold, 12 seconds warm,
and 8 GiB RSS for this exact corpus; observed 5-7.65 GB large-repository peaks
remain an explicit memory-reduction priority. This closes the v0.4 development
milestone without tagging, publishing, or claiming release readiness.

The native `unreachable-code` rule now consumes the same no-return predicate
as the shared CFG and SSA builders. Same-module imported terminal helpers
therefore remove false continuations from unreachable-code analysis without
retaining dependency source as a lint target. Exact first-statement ranges,
branch and label reachability, nested function literals, type-error behavior,
suggestion application, and repeated fixing are covered by focused regressions.
Corpus review found eight true-positive dead statements across fzf, Caddy,
NATS, and chi while sqlc and the approved `go-libraries/pkg/prompts` target
remained clean. Direct `testing.T.Skip*` bodies and compiler-required return or
panic terminators are excluded to preserve Go testing and type-checking idioms.

The active v0.5 target-matrix batch adds bounded, canonical
`[[analysis.targets]]` configuration for CI-oriented package analysis. Typed
lint, combined check, and baseline generation execute every selected GOOS,
GOARCH, build-tag, and cgo combination; identical diagnostics and prerequisite
problems deduplicate with sorted target attribution across text, short, JSON,
GitHub, and SARIF output. Persistent cache identity remains target-separated,
statistics aggregate the complete matrix, syntax-only lint stays file-oriented,
the LSP retains its base selection, and fix modes reject matrices before
mutation. Fresh full-tree verification passes, and final-diff review found no
remaining requirement, correctness, or test-quality issue. This closes the
multi-target batch without pushing, tagging, or publishing.

The v0.5 project semantic-contract batch adds strict, bounded, versioned TOML
contracts for exact project and dependency functions and methods. No-return,
must-use, parameter lifecycle, blocking, nil/error, and returned-alias facts
resolve against already-loaded type graphs, seed the shared effect layer, and
participate in CLI, LSP overlay, target-matrix, configuration-introspection,
and persistent-cache behavior. Contract reads enforce the 1 MiB file and 4 MiB
snapshot budgets before allocation, package-variant validation is deterministic,
and external contracts use export types without loading dependency source
solely for resolution. The effect schema and native cache component advance to
version 4. The batch passes focused and full repository gates without pushing,
tagging, or publishing. Genuine incremental LSP reuse, shared state-transition
analysis, strictness profiles, and further evidence-backed catalog growth
remain active v0.5 work.

The v0.5 lock-state transition batch adds one bounded function-local CFG
worklist shared by three lock rules. `unlock-without-lock` is a default
correctness rule for path-proven unmatched, double, and read/write-mismatched
releases; `lock-not-released` remains opt-in suspicious because intentional
cross-function lock handoff is legal. `lock-held-across-blocking-call` now
propagates the same stable lock state through branches and loops, consumes
configured blocking contracts, and excludes `sync.Cond.Wait`. Escapes,
ambiguous defers, unknown receiver methods, nonlocal initial state, and read
depth beyond eight fail closed. A 100-function, 300-finding package benchmark
measured a 63.42 ms median, about 7.71 MB, and 84,642 allocations per run on
Darwin arm64; exact-rule dogfood remained clean on Glippy and
`go-libraries/pkg/prompts`. Resource, transaction, HTTP-body, channel, and
WaitGroup transition rules plus strictness profiles and genuine incremental
LSP reuse remain active v0.5 work.

The v0.5 resource state-transition batch admits `resource-used-after-close` as
an opt-in suspicious CFG rule. Direct locally acquired `Close() error` values
move through open, closed, and conservative unknown states; only curated
operations reached exclusively from a proven close report. Exact helper close
effects establish closed state; ownership transfer and every other helper use
stop state tracking because borrowing does not prove unchanged internal state.
Direct close can reestablish precision after an escape, and reacquisition
establishes a new open value. Deferred or
asynchronous close, aliases, arbitrary methods, multiple tracked calls in one
CFG node, uncertain joins, generated files, and ill-typed packages fail closed.
The 100-function package benchmark measured a 54.36 ms median, about 4.56 MB,
and 53,794 allocations per run on Darwin arm64; exact-rule dogfood remained
clean on Glippy and `go-libraries/pkg/prompts`. The catalog then contained 104
rules. Transaction, HTTP-body, channel, and WaitGroup transition rules plus
genuine incremental LSP reuse remain active v0.5 work.

The v0.5 editor scheduler now runs document analysis outside the protocol loop,
briefly debounces replacement notifications, cancels an active superseded
snapshot, and publishes only a result whose complete document versions remain
current. Code actions wait for an in-flight matching snapshot and receive the
standard content-modified error if a later edit supersedes it. Graceful shutdown
drains the current analysis, while session cancellation and exit stop it. Same-
package typed graph reuse, workspace file notifications, memory-aware eviction,
portable editor budgets, and state-transition rules remain active v0.5 work.

The v0.5 policy batch adds `default`, `recommended`, `strict`, and `pedantic`
lint profiles over the existing group and override engine. Recommended policy
adds a fixed low-noise suspicious set; strict and pedantic progressively add
complete opt-in groups without enabling restriction or untargeted migration.
Profiles remain distinct from explicit rule overrides, participate in cache
identity, retain exact-rule and path precedence, appear with rule-level reasons
in `config show`, and are selectable through `glippy init --profile` and shell
completion. Same-package typed graph reuse, workspace file notifications,
memory-aware eviction, portable editor budgets, and additional state-transition
rules remain active v0.5 work.

The v0.5 transaction state-transition batch admits
`sql-transaction-used-after-completion` to default correctness. Direct
`database/sql` transactions move through open, completed, joined, and
conservative unknown states after a proven acquisition guard. Operations and
repeated finalization report only when every reaching path is already completed;
exact Commit, Rollback, or a guaranteed helper effect reestablishes completed
state after an escape. Conditional completion, aliases, transfers, asynchronous
calls, deferred execution, and multi-call nodes fail closed. One 100-function,
100-finding package probe measured 87.12 ms, about 7.65 MB, and 76,756
allocations on Darwin arm64; exact-rule dogfood remained clean on Glippy and
`go-libraries/pkg/prompts`. The catalog now contains 105 rules. HTTP-body,
channel, and WaitGroup transitions, same-package typed graph reuse, workspace
file notifications, memory-aware eviction, and portable budgets remain active
v0.5 work.

The v0.5 HTTP response-body state-transition batch admits
`http-response-body-used-after-close` as an opt-in suspicious CFG rule. Direct
package and Client acquisitions use the same successful error-guard boundary as
the existing not-closed rule. Direct reads, selected exact `io` consumers, and
repeated closes report only when every reaching path proves the body closed;
exact close effects can establish or reestablish that state. Conditional close,
aliases, reassignment, transfer, deferred or asynchronous execution, unknown
helpers, and ambiguous multi-call nodes fail closed. The rule remains outside
`recommended` pending broader adoption evidence. The catalog now contains 106
rules. Channel and WaitGroup transitions, same-package typed graph reuse,
memory-aware eviction, and portable budgets remain active v0.5 work.

The v0.5 workspace-watching batch dynamically registers bounded Go,
module/workspace, configuration, and TOML/JSON file patterns when an LSP client
advertises the capability. Valid created, changed, and deleted notifications
cancel superseded analysis, record sorted unique absolute paths in the backend
session, and refresh one exact snapshot of all open documents. Retained
filesystem and package-graph evidence invalidates the affected package and open
reverse dependants without reloading an unrelated package; captured local
dependencies outside the project root are
also covered when the client reports them. Channel and WaitGroup transitions,
same-package typed graph reuse, and portable budgets remain active v0.5 work.

The v0.5 workspace-memory batch replaces count-only retention with a
deterministic 256 MiB accounted-memory budget while preserving the existing
eight-entry bound. Format-capable source is charged at sixteen times its exact
bytes and compact dependency source at twice its bytes, reflecting the distinct
retention costs observed in the typed-memory profile without claiming an RSS
measurement. Entries are considered most-recent-first; an oversized newest
entry remains usable alone while older entries are evicted. Channel and
WaitGroup transitions, same-package typed graph reuse, memory-aware worker
scheduling, and portable budgets remain active v0.5 work.

The v0.5 channel state-transition batch admits `channel-used-after-close` to
default correctness. Direct local channels initialized by exact built-in
`make` calls move through bounded CFG state; sends and repeated exact closes
report only from the all-path closed state. A direct close reestablishes closed
state on its normal continuation after aliases or helper escape, while
conditional close, nonlocal channels, closure capture, asynchronous execution,
and ambiguous multi-operation nodes remain conservative. Receives after close
remain legal, deferred close is not applied at registration, and direct
reacquisition establishes a new open channel. Five 100-function benchmark
samples measured a 25.34 ms median, about 2.83 MB, and 30,406 allocations per
run on Darwin arm64. Exact-rule dogfood remained clean on Glippy and
`go-libraries/pkg/prompts`, whose pre-existing bytes were unchanged. The
catalog now contains 107 rules. WaitGroup transitions, same-package typed graph
reuse, memory-aware worker scheduling, and portable budgets remain active v0.5
work.

The v0.5 WaitGroup counter batch admits `waitgroup-negative-counter` to default
correctness without duplicating the existing `waitgroup-misuse` analyzer.
Direct local `sync.WaitGroup` values and pointers initialized from the exact
zero value carry bounded counter sets through CFG joins. Exact constant `Add`
and direct `Done` operations report only when every reaching state would make
the runtime counter negative. `Wait` establishes a zero counter only on paths
that can return; an exact positive local count with no escape terminates that
represented path, preventing a diagnostic after an unfulfillable wait. Aliases,
helpers, closure capture, asynchronous operations, dynamic deltas, and counts
above the exact bound remain conservative. Deferred counter operations are not
modeled at function exit. Five 100-function
benchmark samples measured a 69.20 ms median, about 3.27 MB, and 30,824
allocations per run on Darwin arm64. Exact-rule dogfood remained clean on
Glippy and `go-libraries/pkg/prompts`, whose pre-existing `go.sum` diff and
untracked-file state were unchanged. The catalog now contains 108 rules.
Same-package typed graph reuse, memory-aware worker scheduling, and portable
budgets remain active v0.5 work.

The v0.5 typed retained-memory attribution batch adds an opt-in benchmark-only
phase observer and external heap-profile harness. On pinned sqlc, package
loading retains about 1.49 GB before Glippy promotes reporter-visible source;
the largest flat owner is 512.08 MiB of `go/types` type-and-value records.
Removing all SSA rules lowers ordinary peak RSS by only 4.3%, while disabling
the fact-bearing `printf-arguments` adapter lowers it from 3.40 GB to 1.34 GB.
One high-fanout `cmd/sqlc` root still peaks near 2.97 GB, proving that
top-level root batching alone cannot close the scale boundary. The next memory
implementation must produce and rebind stable analyzer fact snapshots in
bounded dependency waves without retaining complete dependency syntax and
type information. The attribution batch does not establish a portable release
budget or change ordinary CLI/LSP behavior.

The v0.5 exact printf fact-execution batch resolves that retained dependency
graph without weakening the upstream analyzer contract. The ordinary process
runs native and fact-free adapted rules first, releases their package graph,
and then invokes the same Glippy executable as the exact upstream `printf`
unitchecker through serialized `go vet -json -p=2`. The runner preserves the
loader's offline, read-only, target, workspace, overlay, and cancellation
policy; bounds output; removes task-owned overlays; rejects cross-line or stale
positions; and rebinds only eligible root diagnostics and declared suggestions.
The bounded in-process fact-wave scheduler remains the fallback when no
external runner is installed.

On the final local tree, pinned sqlc at
`8a7cddfbb9088666eb981645285d7699e71dcb54` used 1,306,836,992 bytes and
20.330 seconds cold, then 552,550,400-555,696,128 bytes and 1.530-1.600 seconds
warm. All three samples preserve normalized diagnostic SHA-256
`030f4474aec74877307118376e77e8ad7254a46c32777242fd11e3223c7282d0` and
pass the durable 2 GiB/40-second reference-host gate. Portable typed budgets
still require native Linux and final-candidate macOS evidence.

The v0.5 graph-first workspace batch removes project-wide invalidation when an
editor opens a package that has no retained result. Glippy analyzes each new
package group before making cache-reuse decisions, uses the exact returned root
package paths to invalidate only retained reverse dependants, and falls back to
root-wide invalidation when the new graph cannot be validated. Opening an
unrelated package now performs one new package analysis while reusing the known
package; opening a dependency still reloads its known consumer. This does not
retain or incrementally update the changed package's `go/types`, CFG, SSA, or
effect graph. Same-package typed graph reuse, metadata-only graph discovery,
memory-aware worker scheduling, and portable budgets remain active v0.5 work.

The v0.5 same-package typed-session batch now reparses and re-typechecks a
changed clean package, including one nested beneath the project root, from
retained compact dependency types instead of invoking the primary `go/packages`
loader again. The session reconstructs fresh AST and `go/types.Info` state from
the complete exact overlay, then rebuilds
selected CFG, SSA, native effects, diagnostics, suppressions, baselines, and
code-action state through the existing engines. Clean-analysis comparison
proves identical self-assignment diagnostics, while dedicated regressions prove
fresh CFG and SSA findings. Active and ignored root and mutable local dependency
source plus module control are revalidated even when a local replacement
resides outside the selected project root. File notifications invalidate
retained graphs without waiting for an active load. A newly direct import is
admitted when its exact package is already retained and Go visibility permits
it. An import absent from that graph, changed dependency overlay,
source-membership or build-constraint change, project-control change,
cgo-generated source, parse/type failure, or uncertain graph
identity falls back to the complete loader. Workspace file notifications
invalidate retained typed graphs before analysis. The result cache and typed
graph session now use
separate 128 MiB stable weights, keeping their ordinary aggregate retained
budget at 256 MiB; neither weight claims process RSS. A ten-edit owned probe
performed zero full primary loads, one incremental load per operation, and
measured 368-484 microseconds per edit on Darwin arm64. Imports absent from the
retained graph, metadata-only package-graph discovery, memory-aware worker
scheduling, and portable budgets remain active v0.5 work.

The v0.5 test-package typed-session batch extends retained re-typechecking to
coherent base, internal-test, and external-test package families and to isolated
internal or external test roots. Families rebuild in dependency order, and an
external test imports the freshly checked internal variant rather than stale
retained types. Repeated external-test edits remain incremental instead of
degrading after one reuse. Root source is deduplicated across variants, retained
weights are recomputed after each edit, and malformed families or generated
compiled roots are rejected. Changed test build constraints, dependency
overlays, source membership, and uncertain identities still use the complete
loader. Focused clean-load comparison and internal/external buffer regressions
pass; no new latency or RSS claim is made. Imports absent from the retained
graph, metadata-only graph discovery, memory-aware worker scheduling, and
portable budgets remain active v0.5 work.

The v0.5 import-only typed-discovery batch admits imports that were not present
in a retained package graph without reloading the primary package root. Glippy
pre-scans the complete fresh root source, requests only the exact missing
package paths at the types tier, merges their validated source identity, and
injects the resolved packages into fresh type checking. Exact package queries
use the documented `pattern=` escaping form, so import paths cannot be
interpreted as `go/packages` metapatterns. Newly loaded local imports remain
available across later edits. Import discovery is bounded, cancelable, excludes
test variants, dependency syntax, and effect facts, and falls back to the
complete loader on load errors, diagnostics, ambiguous identities, visibility
violations, cgo, or any uncertain result. Focused LSP comparison preserves the
same diagnostic code, range, and message as a clean load while changing the
observed path from two full loads to one full load, one import load, and one
incremental load. This is typed import-only discovery, not metadata-only
package-graph discovery; the latter, memory-aware worker scheduling, and
portable budgets remain active v0.5 work.

The v0.5 memory-aware SSA-wave batch replaces one unbounded multi-root
`x/tools/go/ssa` program with canonical waves limited to 64 selected packages
and 8 MiB of compiled root source, deduplicated within each package and charged
again for each selected test variant. One package exceeding the byte budget
remains eligible alone. Glippy still computes no-return and return-state
summaries once across the selected typed roots, runs every enabled SSA rule over
every eligible source function, and lets the completed wave program become
unreachable before constructing the next. Dependency bodies remain excluded,
debug mode is still selected once from enabled rule requirements, and final
diagnostics keep their canonical ordering. The proving regression observed 65
fixture packages sharing one program before the change and two bounded programs
afterward; variant accounting includes every compiled production and test file.
A fixed one-package wave reduced median RSS but added about 46% latency, and
fixed 4/8/16-package studies still traded about 8-19% latency for noisy memory
movement. The accepted adaptive boundary leaves this repository in one wave
while bounding larger inputs. This is a deterministic lifetime contract, not a
portable RSS claim; x/tools exposes no supported per-function build boundary.

The v0.5 portable typed-budget preparation now binds the existing four-runner
native macOS/Linux workflow to pinned sqlc revision
`8a7cddfbb9088666eb981645285d7699e71dcb54`. Every runner will enforce the
40-second and 2-GiB typed ceilings, while Darwin arm64 additionally requires the
known normalized diagnostic digest. This prepares portable evidence but does
not establish it: the workflow has not run against this revision, and no Linux
or cross-architecture v0.5 budget claim is complete until its retained evidence
is reviewed.

The v0.5 error-flow batch admits `nil-error-wrap` to the opt-in suspicious
preset and the curated recommended profile. Exact `fmt.Errorf` calls with
compile-time sequential formats report a literal nil or exact built-in error
value only when the nil comparison's control-flow edge dominates the call.
Exact edge dominance excludes a loop back-edge false positive that
successor-block dominance produced during self-dogfood. Indexed and star
formats, typed nil pointers, dynamic values, generated files, and ill-typed
packages remain conservative, and no fix is offered. Five 100-function
benchmark samples measured a 63.48 ms median, about 5.30 MB, and 54,914
allocations per run on Darwin arm64. Exact-rule dogfood remained clean on
Glippy and `go-libraries/pkg/prompts`, whose pre-existing bytes were unchanged.
The catalog now contains 109 rules. Portable four-runner typed-budget evidence
and further high-signal error-flow coverage remain active v0.5 work.

The v0.5 output-integrity batch admits `unchecked-writer-error` to default
correctness. It reports ordinary, deferred, asynchronous, and explicit
blank-identifier discards from 17 exact standard-library Flush and Close
methods whose contracts write buffered bytes or required framing. Exact
declaring receiver identity includes promoted methods and method expressions
while excluding user methods, generic closers, files, and zero-result CSV
flushes. The specialized rule owns these calls so `discarded-error` and
`blank-error-discard` do not duplicate its diagnostics. Five complete
100-function benchmark samples measured a 64.82 ms median, about 3.11 MB, and
24,754 allocations per run on Darwin arm64. Exact-rule dogfood remained clean
on Glippy and `go-libraries/pkg/prompts`, whose pre-existing `go.sum` change was
unchanged. The catalog now contains 110 rules. Portable four-runner
typed-budget evidence, interface-returning encoder finalization, CSV flush
error observation, and further high-signal interprocedural error-flow coverage
remain active v0.5 work.

The v0.5 CSV output-integrity batch admits `unchecked-csv-writer-error` to
default correctness. For direct identifier-backed `encoding/csv.Writer`
values, the CFG rule reports a `Flush` when any normally returning path fails
to observe the matching `Error`; bare or blank-identifier `Error` calls do not
count. Receiver replacement reports, while aliases, ownership transfer,
closure capture, method-value transfer, fields, deferred execution, and
asynchronous execution remain conservative boundaries. Five complete
100-function benchmark samples measured a 62.83 ms median, about 3.22 MB, and
27,642 allocations per run on Darwin arm64. Exact-rule dogfood remained clean
on Glippy and `go-libraries/pkg/prompts`, whose pre-existing `go.sum` change was
unchanged. The catalog now contains 111 rules. Portable four-runner
typed-budget evidence, interface-returning encoder finalization, and further
high-signal interprocedural error-flow coverage remain active v0.5 work.

The v0.5 interface-encoder output-integrity batch extends
`unchecked-writer-error` without adding a rule or raising its types-tier cost.
Inline results and stable direct bindings from the exact
`encoding/ascii85.NewEncoder`, `encoding/base32.NewEncoder`, and
`encoding/base64.NewEncoder` functions now retain their concrete finalization
identity through the returned `io.WriteCloser`; discarded ordinary, deferred,
asynchronous, and blank-identifier `Close` errors report under the existing
specialized rule. Reassigned bindings, method values, indirect constructors,
and unproven interface closers remain excluded. Five complete 100-function
acquisition benchmark samples measured a 79.54 ms median, about 3.34 MB, and
29,716 allocations per run on Darwin arm64. Exact-rule dogfood remained clean
on Glippy and `go-libraries/pkg/prompts`, whose pre-existing `go.sum` change was
unchanged. The catalog remains at 111 rules. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow coverage remain
active v0.5 work.

The v0.5 required-result contract batch admits `must-use-result` to default
correctness, making the existing schema-version-1 `must-use` facts observable
for exact configured functions and methods. Call statements, `go` and `defer`
calls, and blank tuple destinations report the exact ignored result indexes;
assignments, returns, and argument uses count as consumption. External export
contracts work without making dependency source a lint target, while function
values, dynamic calls, generated files, and ill-typed packages remain
conservative. When selected, the contract-specific diagnostic owns and
supersedes an exact-range `discarded-error` before suppression or baseline
policy. Five complete 100-function benchmark samples measured a 39.77 ms
median, about 1.49 MB, and 11,868
allocations per run on Darwin arm64. Exact-rule dogfood remained clean on
Glippy and `go-libraries/pkg/prompts` at `29e46a9`; the prompts repository's
pre-existing changes, including its `go.sum` bytes, remained unchanged. The
catalog now contains 112 rules. Portable four-runner typed-budget evidence and
further high-signal interprocedural error-flow and ownership coverage remain
active v0.5 work.

The v0.5 returned-alias obligation batch makes the final previously inert
project-contract relationship observable in a built-in lifecycle consumer. An
exact contracted result assigned back to the same tracked transaction or
closer now preserves its outstanding completion obligation instead of being
treated as an ownership transfer or replacement. Guaranteed close and
transaction-completion effects still discharge the obligation; uncontracted
helpers, new alias bindings, and returned identity without a terminal effect
remain conservative. A secondary alias keeps ownership conservative without
erasing a separately guaranteed terminal state from the unchanged tracked
binding. The focused red test reproduced a missing transaction diagnostic
before the shared obligation engine consumed `returns-alias`. Five complete
100-function samples
measured a 91.73 ms median, about 6.14 MB, and 64,634 allocations per run on
Darwin arm64. Exact-rule dogfood remained clean on Glippy and
`go-libraries/pkg/prompts` at `5925270`, whose pre-existing dirty state and
`go.sum` bytes were unchanged. Portable four-runner typed-budget evidence and
further high-signal interprocedural error-flow and ownership coverage remain
active v0.5 work.

The v0.5 cancellation returned-alias batch extends the same project-contract
identity into `context-cancel-leak`. Assigning a contracted alias back to the
same cancel variable, including tuple results, method expressions, duplicate
targets whose final write is the alias, and direct final self-writes, now keeps
the invocation obligation live. Guaranteed cancellation invocation still
discharges it; uncontracted helpers, replacement bindings, and aliases assigned
elsewhere retain the conservative transfer boundary. The initial focused red
test produced no diagnostics for five outstanding obligations before the
cancellation path consumed returned-alias and final-write identity. The final
fixture reports eight obligations across both tuple-result indexes, positional
multi-RHS assignments, imported helpers, and method expressions. A separate
red regression protected three preserving assignments whose blank, address,
or independent references must retain the admitted lostcancel behavior. Five
complete 100-function samples measured a 76.75 ms median, about 4.05 MB, and
43,300 allocations per run on Darwin arm64. Exact-rule dogfood remained
clean on Glippy and `go-libraries/pkg/prompts` at `5925270`, whose pre-existing
dirty state and `go.sum` bytes were unchanged. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow and ownership
coverage remain active v0.5 work.

The v0.5 resource nil-edge batch removes a repeated ownership false positive
from `resource-not-closed`. Exact `resource == nil` and `resource != nil` CFG
edges now discharge only the proven nil path; every non-nil path must still
close or transfer the resource. Focused regressions preserve diagnostics when
the non-nil branch merely observes the value. Read-only dogfood on
`go-libraries/pkg/http-client` removes the motivating request-body-wrapper
diagnostic and two equivalent nil-result findings while retaining 22 other
findings for separate ownership review. The external repository remained
unmodified. Portable four-runner typed-budget evidence and further high-signal
interprocedural error-flow and ownership coverage remain active v0.5 work.

The v0.5 metadata-only package-graph batch removes the remaining root-wide LSP
invalidation when a new package's typed attempt fails before returning graph
identity. Proven root paths now survive later baseline or prerequisite errors;
otherwise Glippy asks `go/packages` only for names, physical files, imports,
transitive dependency metadata, test ownership, and module metadata under the
same overlay, build, module, target, offline, and package-limit policy. It does
not request export data, syntax, types, type information, type sizes, CFG, SSA,
or facts. The red regression reloaded an unrelated cached package when a newly
opened dependency failed typed analysis. The fixed state reloads only its known
reverse dependant and reuses the unrelated package. Discovery failure retains
root-wide invalidation, cancellation propagates, and an incompatible workspace
overlay cannot trigger discovery against substituted disk bytes. No new latency
or RSS claim is made. Portable four-runner typed-budget evidence and further
high-signal interprocedural error-flow and ownership coverage remain active
v0.5 work.

The v0.5 cleanup-managed result batch removes a repeated test-helper ownership
false positive from `resource-not-closed`. An exact stable local result is
cleanup-managed only when every normal helper return first registers an exact
`testing.T.Cleanup` function-literal callback on `*testing.T` and every normal
callback path closes the object directly or through a helper with a guaranteed
close parameter effect. Conditional registration or closure, observation-only
callbacks, goroutines, nested functions, copied `testing.T` values,
reassignment or address escape inside callbacks, aliases, non-testing cleanup
APIs, and disagreeing package variants remain conservative. The fact
crosses selected-module package loads through stable function/result identity
and advances the deterministic native effect cache component to version 6.
Exact-rule dogfood remained clean on Glippy and `go-libraries/pkg/prompts`; an
exact same-revision `http-client` comparison reduced findings from 22 to 17
without modifying the external repository. The retained findings remain for
separate source-specific ownership review. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow and ownership
coverage remain active v0.5 work.

The v0.5 receiver terminal-effect batch extends the shared CFG effect model from
ordinary parameters to exact method receivers. Direct `Close` and statically
resolved receiver delegation can prove closure on every normal path; method
expressions and parameter helpers consume the same stable summary. Conditional
or asynchronous closure, early returns, observation-only methods, receiver
reassignment, aliases, address escape, unresolved recursion, dynamic dispatch,
and disagreeing package variants remain conservative. Effect source selection
now includes root modules plus reachable active-workspace and local filesystem
replacement modules while excluding downloaded dependencies and unrelated
workspace modules. The deterministic native effect schema and cache component
advance to version 7. Focused red regressions covered the receiver and workspace
boundaries. An exact same-revision `go-libraries/pkg/http-client` comparison at
`0e0240b1fdcefaa73619b8d0a5d0dfc778d9e2f0` reduced findings from 17 to 12 by
removing exactly the five `newGoCircuitBreakerIntegration` callers whose cleanup
uses the proven cross-workspace `Breaker.Shutdown` method. The external head and
pre-existing dirty state remained unchanged. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow and ownership
coverage remain active v0.5 work.

The v0.5 direct receiver-consumption batch connects the versioned receiver
terminal summaries to ordinary resource obligations and closed-resource state.
A statically resolved receiver method or method expression whose summary
guarantees close now discharges `resource-not-closed`; the same call establishes
closed state for `resource-used-after-close`. The method-expression receiver is
not reprocessed as an ordinary parameter. Conditional summaries, dynamic
dispatch, promoted methods on an outer receiver, aliases, and unknown facts
remain conservative. The focused red regressions initially retained the direct
leaks and missed both post-shutdown uses. Exact same-revision dogfood on
`go-libraries/pkg/http-client` reduced `resource-not-closed` findings from 12
to 10 by removing the two directly shut down circuit breakers while preserving
the ten unrelated findings and the external repository bytes. Five directional
100-function samples measured 59.57-85.96 ms and 2.11 MB for
`resource-not-closed`, plus 26.24-40.54 ms and 5.39 MB for
`resource-used-after-close`, on Darwin arm64. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow and ownership
coverage remain active v0.5 work.

The v0.5 nil-error return-fact batch connects `nil-error-wrap` to the shared
versioned return-state summaries already consumed by `nilness`. For an exact
direct tuple call, a sibling result proven nil or non-nil on the `%w` path now
proves the error nil only when that sibling state contradicts every non-nil
error return from the selected local helper. The exact control-flow edge must
dominate the formatting call. Dynamic calls, phis, delegated results,
conflicting return summaries, unavailable dependency facts, and typed nil
errors remain conservative. The focused red regression initially produced no
diagnostics for two cross-package defects and now reports both while retaining
the ambiguous and conflicting cases. Exact-rule dogfood remained clean on
Glippy, `go-libraries/pkg/prompts`, and `go-libraries/pkg/http-client`; the
external repositories' pre-existing bytes remained unchanged. Five
100-function samples measured 64.95-65.94 ms, approximately 5.45 MB, and
56,972-56,987 allocations per operation on Darwin arm64. Portable four-runner
typed-budget evidence and further high-signal interprocedural error-flow and
ownership coverage remain active v0.5 work.

The v0.5 required-writer-finalization batch admits `writer-not-finalized` to
default correctness. Direct tar, gzip, and multipart writer acquisitions become
obligated only after an exact output-producing method; direct or deferred
`Close` completes the lifecycle. A missing finalizer reports only on a normal
return with no error result or an explicit nil built-in error result. Returning,
sending, storing, passing, capturing, or asynchronously using the writer stays
conservative, as do aliases, method values, replacement bindings, and unknown
error results. The specific diagnostic supersedes an exact overlapping
`resource-not-closed` finding. Focused red regressions covered unknown rule,
implicit success, transfer forms, diagnostic ownership, and the initially
omitted tar `AddFS` output path. Exact-rule dogfood remained clean on Glippy,
`go-libraries/pkg/prompts`, and
`go-libraries/pkg/http-client`, whose pre-existing bytes remained unchanged.
Five 100-function samples measured 83.66-86.69 ms, 4.47-5.05 MB, and
40,229-40,679 allocations per operation on Darwin arm64. The catalog now
contains 113 rules. Portable four-runner typed-budget evidence and further
high-signal interprocedural error-flow and ownership coverage remain active
v0.5 work.

The v0.5 writer-lifecycle ownership follow-up makes the generic
`resource-not-closed` rule delegate the four exact tar, gzip, and multipart
constructors to `writer-not-finalized`. The focused red regression initially
reported both an unfinalized gzip writer and a multipart writer used only for
boundary validation under the generic rule. After delegation, exact-rule
dogfood remains clean on Glippy and `go-libraries/pkg/http-client`, while the
same-revision generic `http-client` findings fall from ten to seven by removing
exactly the three multipart writer diagnostics. Five one-operation generic-rule
samples measured 73.00-110.71 ms, 2.13-2.72 MB, and 17,037-17,494 allocations
on Darwin arm64. Portable four-runner typed-budget evidence and further
high-signal interprocedural error-flow and ownership coverage remain active
v0.5 work.

The v0.5 constructor-callback ownership refinement makes
`resource-not-closed` treat direct function-literal arguments and the final
direct same-block value of local constructor arguments as conservative
ownership transfers when their callbacks capture the acquired resource.
Conditional or nested assignments do not dominate the constructor and retain
the leak diagnostic. The focused red regression initially suppressed that
conditional leak and now reports exactly the unrelated, replaced, and
conditional containers while accepting the direct and stable-container
captures. The shared acquisition remains tracked by
`resource-used-after-close`, preserving definite post-close diagnostics after
constructor callback capture. Exact same-revision dogfood on
`go-libraries/pkg/http-client` at
`b2bcdc33836d6800db0f51ebf9b816e5d5fb33ee` reduces generic findings from
seven to five by removing the callback-owned `client_test.go` and
`session_test.go` resources; the retained findings are one compression body
and four resume fake files. Exact-rule dogfood remains clean on Glippy and
`go-libraries/pkg/prompts`. All external checks were non-mutating; unrelated
concurrent dirty changes in the shared external repository remain outside this
batch. Five one-operation samples measured 72.11-80.15 ms, 2.13-2.70 MB, and
17,039-17,473 allocations on Darwin arm64. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow and ownership
coverage remain active v0.5 work.

The v0.5 constructor-callback stability follow-up requires an indirectly
capturing local argument to retain its final direct same-block value until the
constructor call. Field or indexed mutation, address escape, call exposure,
and nested mutation invalidate the inferred transfer; harmless reads preserve
it. The focused red regression initially suppressed a leak after replacing the
capturing function field and now reports that acquisition alongside replaced,
conditional, escaped, and unrelated callback containers. Direct and stable
container captures remain accepted, and `resource-used-after-close` continues
to track the shared acquisition independently. Exact-rule dogfood remains
clean on Glippy and `go-libraries/pkg/prompts`; the same-revision
`go-libraries/pkg/http-client` run at
`556e3d5d9a6cd7981f2aaabdbc0f7aaef9ecc7ae` retains exactly the expected one
compression body and four resume fake-file findings. Five one-operation
samples measured 72.96-75.88 ms, 2.13-2.71 MB, and 17,043-17,487 allocations
on Darwin arm64. Portable four-runner typed-budget evidence and further
high-signal interprocedural error-flow and ownership coverage remain active
v0.5 work.

The v0.5 streaming-encoder finalization expansion extends
`writer-not-finalized` to exact direct results from
`encoding/ascii85.NewEncoder`, `encoding/base32.NewEncoder`, and
`encoding/base64.NewEncoder`. A successful path that writes encoded data but
neither closes nor transfers the encoder now reports under default correctness;
finalized, unused, and transferred interface values remain accepted. The
focused red regression produced only the six original writer diagnostics
before the three encoder constructors were admitted. The generic
`resource-not-closed` rule delegates all seven exact writer constructor
functions, preventing duplicate lifecycle diagnostics. Exact-rule dogfood
remained clean on Glippy, `go-libraries/pkg/prompts`, and
`go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; both external repositories were
unchanged. Five 100-function samples measured 84.83-93.63 ms, 4.47-5.06 MB,
and 40,232-40,690 allocations on Darwin arm64. Portable four-runner
typed-budget evidence and further high-signal interprocedural error-flow and
ownership coverage remain active v0.5 work.

The v0.5 initialized-writer precision follow-up makes
`writer-not-finalized` recognize exact constructor results declared through a
local `var` specification as well as assignments. Single-result encoder
declarations and multi-result `gzip.NewWriterLevel` declarations now enter the
same CFG lifecycle, while finalized declared encoders remain accepted and
parallel multi-expression mappings stay conservative. The focused red
regression missed the unclosed declared base64 encoder before CFG `ValueSpec`
acquisitions were admitted. Exact-rule dogfood remained clean on Glippy,
`go-libraries/pkg/prompts`, and `go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; both external repositories were
unchanged. Five 100-function samples measured 86.18-95.41 ms, 4.47-5.06 MB,
and 40,219-40,695 allocations on Darwin arm64. Portable four-runner
typed-budget evidence and further high-signal interprocedural error-flow and
ownership coverage remain active v0.5 work.

The v0.5 initialized-lifecycle precision follow-up gives generic resources,
database transactions, and HTTP response bodies one shared acquisition model
for direct assignments and one-spec initialized local `var` declarations.
Missing finalization and proven post-finalization use now cover declared
resources, transactions, and responses through the same exact result-index,
immediate error-guard, ownership-transfer, and state-transition contracts.
Grouped single specifications are supported, finalized declarations remain
accepted, and declarations containing multiple specifications or parallel
multi-expression mappings stay conservative. Six focused red
regressions produced no lifecycle diagnostics before CFG `ValueSpec`
acquisitions were admitted. Exact-rule dogfood remained clean on Glippy and
`go-libraries/pkg/prompts` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; the same-revision
`go-libraries/pkg/http-client` run retained exactly the expected one
compression body and four resume fake-file suspicious findings. The
non-mutating runs did not alter either external tree. Five one-operation
samples across the six lifecycle benchmarks measured
44.31-202.12 ms, 2.13-9.37 MB, and 17,026-90,785 allocations on Darwin arm64.
Portable four-runner typed-budget evidence and further high-signal
interprocedural error-flow and ownership coverage remain active v0.5 work.

The v0.5 named-error writer-success follow-up extends
`writer-not-finalized` from explicit nil returns to a bounded proof for one
named built-in error result. Language-defined nil initialization, direct nil
assignment, and exact self-assignment preserve successful bare or direct named
returns; other assignments, address escape, closure capture, multiple error
results, deferred named-result mutation, and disagreeing CFG joins remain
conservative. Dataflow begins at function entry, so state established before
writer acquisition is not lost.
The first focused red regression produced no diagnostics for three proven
successful paths; a second safety regression exposed two false positives when
pre-acquisition state was initially omitted. Exact-rule dogfood remained clean
on Glippy, `go-libraries/pkg/prompts`, and `go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; both external repositories kept
their exact revisions and pre-existing status. Five 100-function named-result
samples measured 92.84-179.18 ms, 4.96-5.53 MB, and 49,332-49,763 allocations
on Darwin arm64.
Portable four-runner typed-budget evidence and further high-signal
interprocedural error-flow and ownership coverage remain active v0.5 work.

The v0.5 exact-error-edge writer-success follow-up proves successful named
error returns after exact `err == nil` and `err != nil` control-flow edges.
It covers guards before or after writer acquisition and reversed comparison
operands. The bounded transition engine now supports optional isolated
successor-edge refinement, shared with the existing resource nil-edge
contract. Compound guards, proven non-nil paths, address escape, closure
capture, multiple error results, and reassignment after a guard remain
conservative. The focused red regression retained only five earlier findings
and missed three guarded successful paths before edge transfer was added.
Exact-rule dogfood remained clean on Glippy, `go-libraries/pkg/prompts`, and
`go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; both external repositories kept
their exact revisions and pre-existing status. Five 100-function guarded
named-result samples measured 91.63-150.02 ms, 5.48-6.06 MB, and
59,533-60,019 allocations on Darwin arm64. Portable four-runner typed-budget
evidence and further high-signal interprocedural error-flow and ownership
coverage remain active v0.5 work.

The v0.5 unconditional result-state batch adds stable per-result nilness to the
shared effect facts. A nil-capable result is known only when every explicit
normal return proves the same state; bare, delegated, recursive, dynamic,
typed-nil, unknown, and conflicting returns remain conservative. Stable
function identities carry the summary across independent package loads, and
disagreeing package variants discard it. `writer-not-finalized` now consumes
only exact static error-result facts, including tuple delegation, so a locally
or cross-package helper proven to return nil error on every normal path exposes
the caller's missing finalizer. The native effect schema and cache component
advance to version 8. The focused regression initially produced no diagnostics
for four exact delegated-success paths and now reports those paths while
retaining dynamic, recursive, typed-nil, failure, and unknown cases. Exact-rule
dogfood remained clean on Glippy, `go-libraries/pkg/prompts`, and
`go-libraries/pkg/http-client` at
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; the external repositories retained
their exact revisions and pre-existing state. Five 100-function samples
measured 98.43-112.11 ms, 5.95-6.53 MB, and 66,052-66,484 allocations on Darwin
arm64. Portable four-runner typed-budget evidence and further high-signal
interprocedural error-flow and ownership coverage remain active v0.5 work.

The v0.5 unconditional nil-error wrapping batch connects the version-8
per-result facts to `nil-error-wrap`. Direct same-package and imported static
calls, plus their exact tuple extractions, now prove a `%w` operand nil when
every explicit normal helper return agrees. Phis, dynamic calls, recursion,
typed nil errors, unknown or conflicting returns, and unavailable facts remain
conservative. Initial exact-rule dogfood exposed three false positives from
named error results overwritten by deferred panic recovery. Result inference
now rejects any named binding captured by a function literal or exposed by
address, while retaining harmless unrelated defers. The focused regression
initially produced no diagnostics for four exact helper results, then reproduced
the deferred-mutation safety defect, and now reports only the four proven cases.
Exact-rule dogfood is clean on Glippy, `go-libraries/pkg/prompts`, and
`go-libraries/pkg/http-client` at external revision
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; external bytes and pre-existing
status remain unchanged. Five 100-function samples measured 88.29-91.71 ms,
5.55-6.14 MB, and 58,212-58,662 allocations on Darwin arm64. Portable
four-runner typed-budget evidence and further high-signal interprocedural
error-flow and ownership coverage remain active v0.5 work.

The v0.5 delegated result-state batch advances the versioned effect component
to `native-effects-v9`. Exact static single-result calls and same-arity tuple
returns now reuse unconditional nilness through a bounded 4,096-definition
selected-package traversal. Dependency layers consume only stable facts already
intersected across package variants; same-layer definitions resolve lazily.
Recursive cycles, dynamic calls, result-count or exact-type mismatches, typed
nil errors, unknown returns, deferred calls proven not to return, and
unavailable facts remain conservative. The focused red regressions initially
retained unknown states for direct nil and
non-nil wrappers, a tuple wrapper, and two cross-package `%w` defects. They now
prove those acyclic delegates while mutually recursive helpers remain unknown.
Safety review then reproduced false nil facts for an explicit return followed
by an always-panicking deferred closure or exact local helper. Result inference
now shares the no-return predicate and rejects both shapes, plus unreachable
explicit returns inside functions already proven not to return.
Exact `nil-error-wrap` and `writer-not-finalized` dogfood remains clean on
Glippy, `go-libraries/pkg/prompts`, and `go-libraries/pkg/http-client` at
external revision `127ee12bfa8aa0777716f58618ee8338ba40f0b3`; external bytes
and pre-existing state remain unchanged. Five 100-chain samples measured
88.29-92.73 ms, 7.69-8.31 MB, and 78,919-79,415 allocations on Darwin arm64.
Portable four-runner typed-budget evidence and further high-signal
interprocedural error-flow and ownership coverage remain active v0.5 work.

The v0.5 delegated return-relationship batch advances the versioned effect
component to `native-effects-v10`. A sole exact static multi-value return now
reuses nil/error relationships through the same bounded 4,096-definition,
recursion-rejecting selected-package traversal as unconditional result states.
Same-layer definitions resolve lazily; dependency layers consume only stable
facts already intersected across package variants. Dynamic calls, recursive
cycles, result-count or exact-type mismatches, no-return paths, and unavailable
facts remain conservative. The focused unit regression initially retained
unknown relationships for direct wrappers; cross-package `nil-error-wrap`
reported only two direct defects instead of four, and delegated `nilness`
reported none of five proven operations. All three boundaries now consume the
exact delegated relationship. Self-dogfood then exposed a false positive inside
a compound short-circuit failure guard because one shared successor block was
mistaken for proof of one incoming edge. Nilness now reuses exact control-flow
edge dominance, and the focused regression rejects the sixth false diagnostic
while preserving the five proven findings. Exact `nil-error-wrap` and `nilness`
dogfood is clean on Glippy, `go-libraries/pkg/prompts`, and
`go-libraries/pkg/http-client` at external revision
`127ee12bfa8aa0777716f58618ee8338ba40f0b3`; external bytes and pre-existing
state remain unchanged. Five 100-function samples measured 102.62-105.21 ms,
13.07-13.69 MB, and 155,080-155,600 allocations on Darwin arm64. Portable
four-runner typed-budget evidence and further high-signal interprocedural
error-flow and ownership coverage remain active v0.5 work.

The v0.5 delegated cleanup-managed-result batch advances the versioned effect
component to `native-effects-v11`. Exact static single-result calls and
same-arity tuple returns now reuse cleanup-managed ownership through a bounded
4,096-definition, recursion-rejecting selected-package traversal. Same-layer
definitions resolve lazily; dependency layers consume only stable facts already
intersected across package variants. Recursive cycles, dynamic calls,
conversions, result-count or exact-type mismatches, mixed managed and unmanaged
returns, no-return paths, and unavailable facts remain conservative. Focused
red regressions initially reported delegated single and tuple acquisitions in
same-package and imported callers. Both `resource-not-closed` boundaries now
consume the exact fact, while `resource-used-after-close` continues to track a
delegated acquisition that is explicitly closed before a later operation.
Exact `resource-not-closed,resource-used-after-close` dogfood is clean on
Glippy and `go-libraries/pkg/prompts`. At external revision
`3fe07983906f192a6cd983217a203d89696ffc28`,
`go-libraries/pkg/http-client` reports five conservative
`resource-not-closed` findings and no `resource-used-after-close` finding;
external bytes and pre-existing state remain unchanged. Five 100-function
samples measured 99.06-105.29 ms, 10.77-11.37 MB, and 116,768-117,245
allocations on Darwin arm64. Portable four-runner typed-budget evidence and
further high-signal interprocedural error-flow and ownership coverage remain
active v0.5 work.

The v0.5 authoritative testing-cleanup batch extends cleanup-managed result
facts from `*testing.T` to the exact standard-library `testing.TB`,
`*testing.B`, and `*testing.F` receiver identities. Interface dispatch through
`testing.TB` is resolved from its exact selected method rather than treated as
an arbitrary dynamic cleanup registry. Named interfaces embedding `testing.TB`,
user-defined `Cleanup(func())` interfaces, and other lookalikes remain
unmanaged. The boundary is grounded in `go-libraries` helpers that return
cleanup-managed PostgreSQL pools through `testing.TB` and benchmark transports
through `*testing.B`. Five 100-function samples measured 104.69-116.38 ms,
10.85-11.45 MB, and 126,873-127,354 allocations on Darwin arm64. Exact-rule
dogfood is clean on Glippy, `go-libraries/pkg/prompts`, and the real
`testing.TB` and `*testing.B` helper packages; `http-client` retains its
established five findings and external state remains unchanged. The native
effect schema and persistent-cache component advance to `native-effects-v12`;
the existing bounded delegation and conservative ownership rules remain
unchanged.

The v0.5 testing-termination precision batch excludes compiler-required
zero-value return shims from `unreachable-code`. After an exact standard-library
`testing` `FailNow`, `Fatal`, or `Fatalf` call in a value-returning function,
one final uninitialized variable declaration followed immediately by a return
of only those declared variables is treated as the same syntactic termination
shim as an already-excluded direct return. Empty or initialized declarations,
retained work, local no-return helpers, testing lookalikes, result-free
functions, and statements after the shim remain diagnostics. The regression
covers `*testing.T`, `testing.TB`, `*testing.B`, and `*testing.F`; its initial run
reported all four valid generic zero-value declarations. Exact-rule dogfood is
clean on Glippy and `go-libraries/pkg/prompts`. At external revision
`8409c3d568cc0b921aa271f6dc0cc6dcfcc40625`, it removes the two false positives
from generic `*testing.T` channel helpers in `pkg/http-client` without changing
external bytes. Five one-iteration package-load samples measured
113.95-203.13 ms, 5.73-6.41 MB, and 48,387-48,845 allocations on Darwin arm64.

The v0.5 no-op closer lifecycle batch separates exact source-proven no-op
`Close() error` methods from ordinary receiver-borrow summaries. Only a
selected-module method whose complete body returns a field reached directly
from its receiver is classified as no-op; `return nil`, computed or helper
results, receiver or global mutation, nested cleanup, additional statements,
unavailable source, and disagreeing package variants remain tracked. The first
implementation was rejected by broader regressions because it classified every
`return nil` closer as no-op. The final effect fact advances the native schema
and persistent-cache component to `native-effects-v13`, is stable across
independent loads, participates in deterministic digest and clone behavior,
and requires package-variant agreement. Exact-rule dogfood is clean on Glippy
and `go-libraries/pkg/prompts`; at external revision
`8409c3d568cc0b921aa271f6dc0cc6dcfcc40625`, `pkg/http-client` retains one
credible decompression-body ownership finding while its four field-returning
`fakeResumeFile` findings are removed. External state remained unchanged. Five
one-iteration samples measured 72.01-77.98 ms and 2.14-2.72 MB for
`resource-not-closed`, plus 41.30-45.65 ms and 5.73-5.74 MB for
`resource-used-after-close`. Portable four-runner typed-budget evidence and
further high-signal interprocedural error-flow and ownership coverage remain
active v0.5 work.

The v0.5 changed-dependency incremental-analysis batch extends the retained
typed workspace session beyond same-package edits. Changed active source in the
main module, an active workspace module, or a local filesystem replacement now
reparses and re-typechecks the changed package, its retained reverse dependency
closure, and the selected root without invoking the complete package loader.
Exact dependency overlays participate in the same path. Selected-module effect
facts are rebuilt before root diagnostics, so lifecycle findings cannot reuse
stale ownership summaries. Package membership, build constraints, ignored-file
selection, module and workspace controls, source limits, and supported import
graphs are revalidated; cgo-generated source, unresolved dependency imports,
parse or type failure, ambiguous identities, and immutable dependency edits
retain the conservative full-load fallback. Focused regressions cover
main-module and workspace-module source, external-test dependency overlays,
local replacements, module and sum controls, refreshed cleanup effects, and
ignored-source selection. Five ten-operation Darwin arm64 samples
measured 565.6-692.2 microseconds, 271,711-278,087 bytes, and 1,622-1,627
allocations per operation, with zero full primary package loads and one
incremental load per operation. Portable editor latency and RSS budgets and
further state-transition precision remain active v0.5 work.

The v0.5 changed-dependency import-discovery batch extends retained-import
admission and the bounded exact types loader from root overlays to imports added
by changed local dependencies. New imports are parsed from the changed package
and checked against Go internal visibility. A matching package already in the
retained graph preserves its existing type identity; an absent package loads
through exact escaped package patterns. Newly loaded mutable local layers are
then rechecked against compatible retained transitive types before the package
is attached only to the dependency that introduced it. The expanded graph
participates in reverse dependency rechecking, selected-module effect
reconstruction, source limits, and the next retained session entry. A local
helper introduced by the edit is therefore reusable on later snapshots, and a
subsequent helper change refreshes the caller's lifecycle diagnostics without a
full primary load. Import-load failure, diagnostics, ambiguous identity,
unavailable types, cgo, and invalid source retain the conservative fallback;
cancellation remains an operation-level error. Ten ten-operation
Darwin arm64 samples that alternated adding and removing a standard-library
import measured a 59.70 ms median and 55.99-76.18 ms range, 140.10-140.13 MB,
and 1,722,590-1,722,674 allocations per operation. Each operation used the
incremental path, additions performed one exact import load, removals performed
none, and no operation invoked the complete primary package loader. Portable
editor latency and RSS budgets, cgo-generated inputs, and further
state-transition precision remain active v0.5 work.

The v0.5 direction exit audit reconciles the requested ten-stage program with
the live repository at `e06ffc160bc19fb432ef77bc9da523e7a99fb8e6`. Seven
engineering stages are complete: memory reduction, incremental workspace
analysis, target matrices, semantic contracts, state-transition correctness,
curated profiles, and opt-in catalog expansion. The canonical catalog contains
118 rules. The milestone remains open because local `main` was 56 commits ahead
of both public repository heads, the approved `pkg/prompts` adoption branch was
unpushed, unintegrated, and still used Gox identity, and the current source
revision had not run the four-native-runner release gate or received final
maintainer review. The maintainer-selected `pkg/prompts` repository is the sole
required external adoption target. No push, tag, or publication occurred.

The v0.5 current-source arm64 release rehearsal runs exact Glippy revision
`835a2961d4e47c43e21c551ac56313a743f98667`, Go 1.26.5, immutable golib and
sqlc corpora, and the established release ceilings on native Darwin arm64 and
Docker-hosted Linux arm64. Every 20-sample editor campaign and every five-sample
formatter and typed campaign passes. The old Darwin typed fingerprint fails
against the expanded catalog; five complete gated samples and three additional
normalized runs establish deterministic replacement SHA-256
`13f9c3dd006a105196c13f766a1c849a882754b3083979e9386c78cc2fdb53d2` for 353
findings across five rule IDs. The release workflow now binds that value. The
Docker run is not native or independent-host release evidence, so the exact
pushed candidate still requires all four GitHub-hosted native runners. No push,
tag, publication, or external adoption mutation occurred.

The v0.5 pre-release readiness review refreshes Oxfmt 0.64.0, Oxlint 1.79.0,
Oxc `2dad1e0`, website `84e863f`, Go 1.26.7, and x/tools v0.48.0 evidence
against local revision `c797d7c`. No reference change invalidates Glippy's
architecture or safety boundaries. Fresh full tests, race tests, vet, module
tidiness, build, and default combined self-check pass with task-owned caches.
A complete non-mutating opt-in self-analysis reports 109 reviewed findings:
98 configurable complexity thresholds and eleven genuine pedantic
simplifications, with no package or source problems after dependency preload.
Canonical synchronization, refreshed integrated adoption, the exact pushed
candidate's four-native-runner workflow, and final maintainer review remain
open. No push, tag, publication, or external adoption mutation occurred.

The v0.5 adoption gate now reflects the maintainer's explicit selection of
`go-libraries/pkg/prompts` as the sole representative external adoption target.
The earlier requirement for two additional integrations no longer applies.
The existing `feature/gox-prompts-adoption` branch at `d6b0fba8` remains clean
but is 103 commits behind current `go-libraries` main, unpushed, unintegrated,
and still uses Gox identity. It must be refreshed for Glippy and reviewed before
the adoption gate closes. No external repository was modified.

Canonical `faustbrian/glippy` `main` now exposes exact candidate `df65065`; the
local `origin` also targets the canonical repository. Push-triggered CI run
`32464914062` was still active when this calibration was recorded. Manual
native release-budget run `32464981182` proves that the previously provisional
40-second typed ceiling is not portable:
the first cold sqlc sample took 49.740 seconds on Linux arm64, 51.260 seconds on
Linux amd64, 64.100 seconds on Darwin arm64, and 78.660 seconds on Darwin
amd64. All editor and formatter measurements passed, and typed analysis reached
the elapsed-time check without exceeding the 2-GiB RSS ceiling. The portable
typed ceiling is provisionally recalibrated to 105 seconds, matching the
formatter campaign's established 33% headroom method over the slowest native
observation. The exact candidate must rerun and retain all five typed samples
on every native runner before the budget becomes stable release evidence. No
tag or release was created.

Exact pushed Glippy candidate
`724d8a26eec0ef5883a28e5fee72b34a78c8284a` passes push-triggered CI run
`32465991030` and native release-budget run `32465998309`. Every supported
macOS/Linux amd64/arm64 runner completes the editor, formatter, cold typed, and
warm typed campaigns within the stable 250 ms, 90 second, 105 second, and
2-GiB ceilings. Each runner builds all four candidate archives, executes its
native archive, retains its manifest and checksums, and produces a release
directory identical to the other three runners. The selected
`go-libraries/pkg/prompts` adoption
was refreshed from current `golib` main, reviewed, passed the complete package
gate, pushed, and integrated on `faustbrian/golib` `main` at `5eb1b997`. The
engineering and delivery gates are complete. Final maintainer review remains
required before any tag or release; no tag or release was created.

The corrected v0.5 code candidate is
`a4de9b3099e88116b72c8a64c55655b7115cf295`. Engineering commit `2571025`
closed the reviewed LSP, invalidation, conservative reasoning, target-matrix,
incomplete-output, catalog, process-ownership, and evidence-attribution gaps;
`a4de9b3` aligned the CLI compatibility fixture with the WaitGroup rule's
proven straight-line boundary. The complete safe local verification set passed,
and the final independent source-only review reported no findings. Public
exact-revision CI run `32547111862` also passed. This closes the bounded v0.5
engineering milestone. Aggregate runtime RSS, signals, interruption,
descendant cleanup, process containment, and the refreshed external full-gate
result remain unproven and outside the milestone. No v0.5 tag or release was
created. The complete product goal remains active, and publication remains
prohibited until the product is genuinely ready and the maintainer has reviewed
it and explicitly authorizes publication.

The v0.6 `resource-used-after-close` profile-calibration batch responds to five
adjudicated recommended findings across containerd and gRPC-Go. All five are
intentional closed-state contract tests and none is a true positive. The rule
therefore moves from `suspicious` to `nursery` and leaves the curated
`recommended` profile; an explicit nursery preset or exact rule override still
runs the unchanged analyzer. A focused configuration-to-analysis regression
proves that `default`, `recommended`, `strict`, and `pedantic` do not select the
rule while nursery and explicit selection do. This removes the demonstrated
profile noise without hiding all test-file findings or matching assertion
libraries. The exact external corpus must rerun at the resulting pushed
revision, and the remaining repositories still require complete adjudication,
so stable-v1 progress remains 55% inside v0.6.

The v0.6 `nilness` testing-assertion precision batch responds to three
adjudicated recommended findings in gRPC-Go from pinned corpus run
`32686648982` at exact revision `c0af042`. Return-state inference correctly
proved the compared results nil, but the augmented rule did not distinguish
defective comparisons from direct type-resolved testing failure reports that
assert a callee's nil/error contract. The rule now omits only an impossible
true-edge comparison in a `_test.go` file whose no-else branch is exactly one
`testing.Error`, `Errorf`, `Fail`, `FailNow`, `Fatal`, or `Fatalf` call with a
side-effect-free receiver and arguments. Nil dereferences, ordinary or
tautological comparisons, non-test files, branches with other work, lookalike
methods, and calls with effectful evaluation remain eligible. Focused red
regressions proved the original assertion noise, the effectful-evaluation
over-suppression, the corpus-equivalent non-terminating reporting boundary, and
pointer-indirect field evaluation; the focused regression and complete
`TestNilness` suite pass afterward.
Generated rule documentation is current. The exact external corpus must rerun
at the resulting pushed revision, and remaining repositories still require
complete adjudication, so stable-v1 progress remains 55% inside v0.6.

The v0.6 `standard-method-signature` profile-calibration batch responds to 26
unique default findings in the pinned Prometheus corpus. Every finding is the
upstream `stdmethods` name-only heuristic applied to Prometheus's documented
`Seek(int64) chunkenc.ValueType` iterator contract rather than an attempted
`io.Seeker` implementation. The unchanged analyzer therefore moves from
`correctness` to `pedantic`: `default`, `recommended`, and `strict` no longer
select it, while `pedantic` and exact rule overrides retain its diagnostics.
A configuration-to-analysis regression proves the profile boundary, the
generated rule catalog is current, and the Go-vet compatibility documentation
records the deliberate preset difference. The superseded exact corpus run
`32691406468` was canceled because its candidate could not satisfy the v0.6
signal gate. The complete corpus must restart at the resulting pushed revision,
so stable-v1 progress remains 55% inside v0.6.

The v0.6 `nil-context` optional-helper precision batch responds to two
recommended Grafana findings from pinned corpus run `32698988231`. Both calls
deliberately pass nil to a private package-local method whose available source
guards every context operation and uses nil to select an unlinked span. Exact
candidate `0041f614bdf36219271e0efcaa3a49d23c3ca2ee` suppresses only a direct nil
argument whose unexported current-package callee body proves that parameter
optional. Exported and dependency calls, dynamic dispatch, variadics, method
values and expressions, panic paths, assignments, and unguarded uses remain
diagnostics. The complete corpus must rerun at that pushed revision or a later
correction, so stable-v1 progress remains 55% inside v0.6.

The v0.6 Moby writer-precision batch responds to seven default
`unchecked-writer-error` false positives from the same pinned run. Four
deferred tar finalizers are redundant after a later observed same-writer
Close, two stable in-memory formatter chains accept exact `time.Time` values,
and one finalizer is the terminal action of a source-proven expected-panic
test. Bounded regressions retain diagnostics for conditional or bypassed
closes, mutable package receivers, escaped and later-used writers, custom
formatter callbacks, missing or conditional recovery guards, nonterminal
finalizers, intervening defers, lookalike recover functions, asynchronous
calls, nested-block guards, and non-test code. The remaining 14 Moby default
findings are actionable under the recorded rule contracts. The exact external
corpus must rerun at the resulting pushed revision, and Moby's recommended
findings plus the remaining repositories still require complete adjudication,
so stable-v1 progress remains 55% inside v0.6.

The v0.6 NATS adjudication at pinned corpus run `32698988231`, Glippy revision
`7840a0c`, and NATS revision
`1787eee035cf0253c201fa0f05afad92b6f296dc` classifies all 21 default and
recommended findings: 15 are actionable and six are intentional or noisy. The
actionable set contains seven discarded writer finalization errors, one
genuinely unreachable WAL-compaction assertion, and seven duplicated, stale,
or tautological nilness conditions. Two default `unreachable-code` findings
are final direct returns immediately after exact `testing.T.Fatalf` calls. The
rule now accepts only a result-free direct return that is the sole remaining
statement after exact `testing.FailNow`, `Fatal`, or `Fatalf`; retained work,
nonfinal returns, lookalikes, and other no-return helpers still report. Existing
value-returning syntactic-shim behavior remains unchanged. Four recommended
nilness findings are deliberate callee-contract or loop-postcondition
assertions and remain recorded evidence rather than receiving a broad test-file
exception that would hide genuine NATS defects. Go vet reports nothing;
Staticcheck separately reports one ineffective assignment, migration
deprecations, and one always-true defensive no-return check. Focused and
complete unreachable-code regressions, affected package tests, vet, and the
generated rule-document check pass. The exact corpus must rerun after this
precision correction, and the remaining repositories still require complete
adjudication, so stable-v1 progress remains 55% inside v0.6.

The v0.6 Moby finding adjudication for pinned run `32698988231`, Glippy
revision `7840a0c`, and Moby revision
`b612274c5489b546ff8b4a4f93f25a0b8952713a` classifies all 31 default and
recommended findings. Twenty-four are actionable: three context-cancellation
leaks, eleven writer finalization errors, one unclosed HTTP response body, and
nine unobserved scanner terminal errors. The scanner cases can accept partial
data, hide the root scan error behind a generic failure or delayed timeout, or
silently skip the assertion that made the scan necessary. The seven remaining
findings are the writer false positives corrected in `ec579af`. Go vet reports
nothing. Staticcheck's additional results are dominated by intentional
OpenTelemetry context assignments, an explicitly suppressed test mock,
error-string style, dead-code policy, and migration deprecations; none
establishes a missing default Glippy rule. No further Moby-specific precision
change is required. All four Glippy reports completed, but each correctly
returned source-error exit 2 because typed analysis was unavailable for cgo
sources in two packages. The corpus validator now keeps report completion
distinct from complete rule coverage, marks the default and recommended
profiles incomplete, and requires an explicit unsupported-construct gap. The
exact corpus must rerun on the latest candidate and the remaining repositories
still require complete adjudication, so stable-v1 progress remains 55% inside
v0.6.

The v0.6 Caddy adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and Caddy revision
`0cf03d32f7d99cf160d5375e8a40fbe3d910d515` classifies all six default and
recommended findings. Three are actionable: one discarded gzip finalization
error, one HTTP error path that does not close its response body, and one
scanner error path hidden behind polling and timeout behavior. Three default
`unreachable-code` findings are final unlabeled `break` or `continue` statements
immediately after exact `testing.T.Fatalf` calls. The rule now accepts those
loop-control shims only when they are the sole remaining statement; labeled or
nonfinal branches, retained work, lookalikes, and other no-return calls still
report. Owner-specific break counts remove only an accepted unreachable shim
edge, retaining a separate reachable break while preserving downstream
unreachability after infinite loops and exhaustive switch or select constructs.
Focused and complete unreachable-code regressions, affected package tests, vet,
and the generated rule-document check pass. Go vet reports nothing, while
Staticcheck adds only migration deprecations and one dead-code policy finding.
The exact corpus must rerun on the latest candidate and the remaining
repositories still require complete adjudication, so stable-v1 progress remains
55% inside v0.6.

The v0.6 OpenTelemetry-Go adjudication for pinned run `32698988231`, Glippy
revision `7840a0c`, and OpenTelemetry-Go revision
`736a14fcdca28b8cf5237e6b9b166ec6ed832bf7` records zero default, zero
recommended, one strict, and four pedantic findings. All profiles completed on
the generated-code-bearing library without a tool or source-state failure, so
there are no default or recommended findings to classify. Go vet reports
nothing. Staticcheck's three `U1000` findings target deliberately unread
function fields whose structural purpose is to make test wrapper types
non-comparable; they do not establish a missing default Glippy rule. The strict
`too-many-lines` finding and three pedantic `mixed-receiver-type` findings remain
opt-in policy signals. OpenTelemetry-Go therefore needs no precision correction
or default-rule candidate from this run. The exact corpus must rerun on the
latest candidate and the remaining repositories still require complete
adjudication, so stable-v1 progress remains 55% inside v0.6.

The v0.6 etcd adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and etcd revision
`c34dc7ee0048fd2bcc44d50beff002e5e8069b69` records zero default, zero
recommended, 40 strict, and 41 pedantic findings. All profiles completed across
the workspace and generated-source selection without a tool or source-state
failure, so there are no default or recommended findings to classify. Go vet
reports nothing. Staticcheck's sole `SA1019` finding is an explicitly suppressed
deprecated protobuf import in the benchmark tool; it is accepted targeted
migration debt rather than a missing default correctness rule. Strict reports
34 discarded errors, three resource-lifetime findings, two excessive-nesting
findings, and one loop defer; pedantic adds one redundant closure. etcd
therefore needs no precision correction or default-rule candidate from this
run. The exact corpus must rerun on the latest candidate and the remaining
repositories still require complete adjudication, so stable-v1 progress remains
55% inside v0.6.

The v0.6 sqlc adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and sqlc revision
`99a7d7d0c7b913ded6a60a9d038ece99dcdf7892` classifies all three recommended
findings as actionable `unchecked-scanner-error` defects and none as noisy. One
command helper and two migration filters can return normally with partial input
after an oversized or failed scan. Default remains clean; strict reports 392
findings and pedantic 441. All profiles completed across the cgo,
generated-source, generator, and database selection. Go vet reports nothing.
Staticcheck reports migration, generated-code, simplification, style, dead-code,
and one general ineffective-assignment finding. The latter computes a
ClickHouse parameter name and fallback that the returned node never consumes.
Together with the independent NATS `SA4006` occurrence, it establishes the
first aggregate v0.6 missed-defect queue entry for a separately admitted
SSA-backed ineffective-assignment rule. It does not justify broadening the
error-only `overwritten-error` rule or default-enabling a new rule without a
precision contract. The exact corpus must rerun on the latest candidate and
the remaining repositories still require complete adjudication, so stable-v1
progress remains 55% inside v0.6.

The v0.6 restic adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and restic revision
`a80be1478a4c537f8396e0db2b05120aa78f11e0` classifies all 12 recommended
findings: 11 actionable unchecked scanner errors and one intentional final bare
return after `testing.T.Fatalf`. Six test helpers can false-pass or hide the
root scan failure, three subprocess stderr goroutines can stop draining their
pipes, one release check reads `Scanner.Err` only inside the loop, and
self-update checksum parsing hides a scan failure behind a generic missing-hash
error. The testing return is covered by precision commit `972cfcf`. All four
profiles completed across the backup, cgo, and CLI selection; the old run
records one default, 12 recommended, 201 strict, and 240 pedantic findings. Go
vet reports nothing. Staticcheck adds two explicit API migrations, eight style
findings, and one unused field, none of which establishes a missing default
Glippy rule from this run. The exact corpus must rerun on the latest candidate
and the remaining repositories still require complete adjudication, so
stable-v1 progress remains 55% inside v0.6.

The v0.6 Hugo adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and Hugo revision
`76a5e1880ab46688155b02e99bab9be2a6134492` classifies all eight default and
recommended findings. Five are actionable: two impossible nilness checks, two
unchecked terminal scanner errors, and one overwritten parser error. Three
default `unreachable-code` findings are final unlabeled breaks after Hugo's
source-proven panicking `(*state).errorf` helper in its Go `text/template`
fork. The rule now accepts only a final unlabeled break or continue immediately
after a direct proven no-return helper call; labeled branches, compound
terminal flow, retained work, and separate reachable breaks remain diagnostics.
Accepted break shims are removed from owner reachability accounting. Go vet
reports nothing. Staticcheck adds five intentional self-deprecation findings
and a third independent ineffective-assignment example, now recorded in the
missed-defect queue. All four profiles completed across 889 owned files and 384
packages. The exact corpus must rerun on the latest candidate and the remaining
repositories still require complete adjudication, so stable-v1 progress remains
55% inside v0.6.

The v0.6 containerd adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and containerd revision
`6c12665e1c31e7728d0e0eb1224288bd9153114b` classifies all 11 default and
recommended findings. Seven are actionable: three discarded stdout-backed tab
writer flush errors, one discarded caller-backed tar finalization error, one
tautological error condition, and two unobserved terminal scanner errors. Four
are precision false positives: two tab writers flush only into containerd's
in-memory progress buffer, one fuzz tar writer targets a local `bytes.Buffer`,
and one Linux-selected nil return fact conflicts with the fallible Windows
implementation used by the shared caller. The writer cases are queued for
general bounded writer-chain proof, and the TLS case is queued for
build-variant-aware return-state facts; project-specific exceptions are
rejected. All four profiles completed across 1,044 owned files and 381 packages
without a tool or source-state failure. Go vet reports nothing. Staticcheck
adds migration and style policy, one intentionally suppressed context-key
finding already covered by Glippy's opt-in rule, and one unused test helper;
none establishes a missing default rule. The exact corpus must rerun on the
latest candidate and the remaining repositories still require complete
adjudication, so stable-v1 progress remains 55% inside v0.6.

The v0.6 Grafana adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and Grafana revision
`45a80328c0dd976720af0366efcad0787762b142` classifies all 42 default and
recommended findings. Thirty-four are actionable: four HTTP response-body
leaks, one identical branch, three nil-error wraps, nine nilness findings, one
source-version incompatibility, 15 unchecked rows errors, and one unchecked
scanner error. Eight are intentional or unreachable: two calls to a private
nil-guarding context helper, five final returns after `testing.Fatal` or
`Fatalf`, and one CSV flush inside a literal `if false` audit block. The
general nil-context and no-return shim corrections already cover the first
seven. A shared typed constant-reachability correction now excludes CSV, rows,
and scanner lifecycle candidates inside constant-dead `if`, `else`, and
classic `for` branches while retaining runtime-conditional findings. All 32
distinct diagnostic source files match the pinned revision and the focused
affected rule suite passes. Go vet reports the same source-version issue.
Staticcheck adds broad migration, unused-code, and style policy plus a fourth
general ineffective-assignment example and two exact-suffix-as-cutset misuse
examples, both recorded in the missed-defect queue. The exact corpus must rerun
on the latest candidate and the remaining repositories still require complete
adjudication, so stable-v1 progress remains 55% inside v0.6.

The v0.6 gRPC-Go adjudication for pinned run `32698988231`, Glippy revision
`7840a0c`, and gRPC-Go revision
`4793ad0474669eafbd5346c8a3d098fdfa542498` classifies all 17 default and
recommended findings. Fourteen are actionable: seven stale or redundant
nilness conditions, two mutations of range-value copies, and five discarded
buffer flush errors. Three final returns after `testing.T.Fatalf` are covered
by the general testing no-return correction in `972cfcf`. All referenced source
digests match the retained snapshot. Go vet is clean. Staticcheck adds broad
deprecation and style output plus three harmless final increments after their
last use; this does not establish a strong new default-rule candidate. The
exact corpus must rerun on the latest candidate, so stable-v1 progress remains
55% inside v0.6.

The v0.6 Prometheus adjudication for the same run and Glippy revision, at
Prometheus revision `d15adb9ad7e5d9fbde3a9a8f30200593a5a14d86`, classifies all
ten default and recommended findings. Six are actionable: an impossible
`uint32` comparison, a shadowed named error, one unchecked scanner error, and
three discarded writer finalization errors. Two deliberate nil-slice panics and
two final returns after `testing.T.Fatalf` are intentional or covered by the
testing no-return correction. All referenced source digests match. Go vet's 26
name-only `Seek` findings are the standard-method false-positive family already
moved to pedantic. Staticcheck adds intentional, generated, suppressed,
migration, style, and performance findings but no new default-rule candidate.
The exact corpus must rerun on the latest candidate, so stable-v1 progress
remains 55% inside v0.6.

The v0.6 Terraform adjudication for the same run and Glippy revision, at
Terraform revision `883c7221ee2e0bc2fad6912fd4b3cc4b53a598a7`, classifies all 25
default and recommended findings. Twenty-four are actionable: nine nilness
conditions, eleven unchecked scanner errors, three discarded output
finalization errors, and one cleanup deferred before acquisition success. One
final return after `testing.T.FailNow` is covered by the general testing
no-return correction. All referenced source digests match. Go vet and
Staticcheck are clean. The exact corpus must rerun on the latest candidate, so
stable-v1 progress remains 55% inside v0.6.

Go-Ethereum revision `02b73d4ea7181464175e0a6cbecc0a3a2655a562` and go-sqlite3
revision `58c8e145308ceded07d1df2ac1b65999499e7055` select Go 1.24 and Go 1.21,
respectively. Glippy's accepted source range is Go 1.25 through Go 1.26, so all
four profiles correctly returned source-error exit 2 with zero diagnostics and
`complete: false`. These are explicit unsupported-source corpus boundaries,
not clean runs; directive shims are rejected.

CockroachDB revision `8812064a015d2faf99d3fc7e15880f94042954b0` failed analysis
preflight because `pkg/internal/team/TEAMS.yaml`, required by `go:embed`, is a
generated file absent from the clean checkout. No Glippy profile or comparator
produced adjudicatable findings. This remains an explicit unsupported build
prerequisite unless a digest-bound derivative-source contract is adopted.

Kubernetes revision `e81f39c0e03ce8ed8e2660c9147b391edd9e262b` exposed a
genuine panic in every profile when testing-skip inference passed a nil terminal
call to `typeutil.StaticCallee`. Commit `7a7f720` now treats non-call no-return
terminals conservatively and its regression, affected analysis suite, vet, and
independent review passed. The retained run remains incomplete crash evidence;
Kubernetes must rerun in isolated CI on the latest candidate. Stable-v1
progress remains 55% inside v0.6.

The completed canonical adjudication for run `32698988231` now binds all 17
repository results and classifies all 259 default and recommended profile
entries: default contains 39 true positives, 33 false positives, and one
duplicate-vet finding; recommended contains 145 true positives, 40 false
positives, and one duplicate-vet finding. No finding remains unclassified. The
generated aggregate report validates ten gaps and produces two evidence-backed
rule-queue entries: `ineffective-assignment` across four repositories and
`exact-suffix-as-cutset` from Grafana. Canonical validation remains incomplete
with 12 unresolved profile or comparator units belonging to CockroachDB,
Go-Ethereum, go-sqlite3, Kubernetes, and Moby. The latest exact corpus rerun is
still required, so stable-v1 progress remains 55% inside v0.6.

Pinned corpus run `32723865179`, attempt 2, supersedes that incomplete
candidate at exact analyzed revision `ee2ea4f`. All 20 GitHub Actions jobs
passed, including 17 repository audits and combined evidence collection. The
canonical adjudication classifies all 386 default and recommended entries:
default contains 66 true positives, 26 false positives, one duplicate-vet
entry, and 15 duplicate-Staticcheck entries; recommended contains 229 true
positives, 33 false positives, one duplicate-vet entry, and 15
duplicate-Staticcheck entries. Exact reconciliation carried 159 unique prior
fingerprints, removed 27 unique superseded fingerprints, and added 119
manually reviewed Kubernetes fingerprints. The corrected Kubernetes analysis
completed all four profiles and removes the prior crash gap. Nine gaps remain:
five comparator-backed missed-defect gaps queued as `ineffective-assignment` and
`exact-suffix-as-cutset`, plus four explicitly scoped unsupported source,
generated-build, or cgo-analysis boundaries. Ten incomplete profile or
comparator units remain attached to those accepted boundaries; no finding is
unclassified. This closes v0.6 and advances stable-v1 roadmap progress to 65%
with v0.7 active. Aggregate allocated-byte measurements are recorded, while
stable cross-platform peak-RSS budgets remain a v0.8 requirement. No public
corpus repository was modified, and no tag or release was created.

v0.7 has consumed both rule candidates from the completed v0.6 comparator
queue. `ineffective-assignment` is now an SSA-backed nursery rule covering
unread ordinary, compound, tuple, receive, and increment results without a
guessed fix; exact `overwritten-error` diagnostics supersede it at the same
range. `exact-suffix-as-cutset` is now a types-backed nursery rule that requires
a direct `strings.HasSuffix` guard with identical value and suffix identity
before reporting `strings.TrimRight`, and its exact `TrimSuffix` replacement is
classified unsafe because it intentionally corrects observable trimming
behavior. Both remain outside curated profiles pending the v0.8 corpus
rerun. Stable-v1 progress remains 65% inside the broader v0.7 semantic-depth,
precision, credible-fix, and interaction gate.

The first v0.7 corpus-noise correction propagates exact testing-failure
provenance through selected local-source wrappers. `unreachable-code` now
accepts a final bare return shim after a wrapper only when every terminal path
resolves to `testing.FailNow`, `Fatal`, or `Fatalf`; mixed panic, skip, dynamic,
and lookalike paths remain diagnostics. The native effect schema and cache
component advance to version 15 so cross-load facts and package variants stay
deterministic. This targets the 17 classified Kubernetes default false
positives without suppressing the 17 actionable post-mortem and cleanup
findings. Stable-v1 progress remains 65% until the complete v0.7 precision and
interaction gate passes.

The next v0.7 precision batch moves `unchecked-writer-error` to the shared CFG
tier and consumes versioned effect facts. An exact selected-module
`Write([]byte) (int, error)` method is treated as infallible only when every
normal return proves its error nil through bounded static delegation rooted in
`bytes.Buffer.Write` or `strings.Builder.Write`; fallible, interface-dispatched,
recursive, unavailable, and package-variant-disagreeing implementations remain
diagnostics. The native effect schema and cache component advance to version 16.
This targets the two containerd progress-writer false positives without hiding
the separately fallible progress flushes. Stable-v1 progress remains 65% until
the remaining tar-writer and interaction gates pass.

The remaining containerd fuzz tar finding is now scoped as an accurate but
intentional diagnostic rather than a precision false positive. The fuzz target
consumes its in-memory buffer before deferred tar finalization, discards the
close result, and deliberately accepts malformed archive material. Corpus
adjudication now supports `intentional` separately from `false-positive`; Glippy
does not guess intent from test or fuzz paths, and an upstream repository would
use the ordinary narrow suppression contract to hide such a finding. This
removes the unsound proposal to suppress deferred in-memory tar closes, which
can still fail on incomplete entry state. Stable-v1 progress remains 65% until
the v0.7 interaction gate passes.

The three carried-forward Moby writer fingerprints are now characterized from
their pinned source shapes. The redundant deferred tar source shape is already
excluded; the two identical directory-formatting shapes reproduced because exact
`io/fs.FileMode` values were treated as arbitrary formatting callbacks. Stable
in-memory fmt provenance now accepts that authoritative scalar standard-library
type alongside basic values and `time.Time`, while user-defined method sets
remain conservative. The exact Moby corpus still reruns in v0.8. Stable-v1
progress remains 65% until the v0.7 interaction gate passes.

The v0.7 exit audit closes semantic depth and credible fixes at 75% stable-v1
progress. Both comparator-backed nursery candidates are implemented; all 26
historical default false-positive entries have a bounded current disposition;
safe, suggestion, and unsafe authorization, conflict rollback, formatter and
suppression ownership, diagnostic precedence, and native-cache invalidation
pass one focused interaction matrix. Recursive testing-failure wrapper cycles
remain deliberately unclassified rather than being overstated as supported.
The exact corpus rerun, formatter corpus proof, cross-platform resource budgets,
and fix rehearsals now belong to active v0.8. No runtime process-control or RSS
probe was run locally.

The first v0.8 hardening batch extends the isolated pinned-corpus harness with
a canonical-config `fmt --check` audit over each read-only source snapshot.
Exact formatter machine output, file and difference totals, completeness, and
artifact digests now participate in versioned result, adjudication, and report
schemas. Complete results must reconcile canonical ordered paths, statuses,
counts, errors, and outcome categories. Invalid or incomplete output remains
release-unresolved and requires a formatter-sourced gap. This is harness
capability only; exact public-corpus formatter evidence requires the next run on
the resulting pushed revision.

The next v0.8 hardening batch adds a canonical recommended-profile
`lint --fix --diff` rehearsal after the offline package preflight. The preview
executes the production transactional coordinator, formatter normalization,
and post-fix analysis against the read-only task-owned snapshot, records exact
normalized output, and binds completeness and its artifact digest into result,
adjudication, and aggregate-report schemas. Exit 0 or 1 without tool stderr is
complete; every other tool outcome, including bounded output without a process
exit code, is release-unresolved and requires a fixer-sourced crash or
unsupported-construct gap. Cancellation remains a run-level failure. Exact
post-preflight snapshot inventory binds content, symlink identity, and modes,
including `go.work.sum`, before and after preview. No external repository is
modified. Exact public-corpus fix evidence requires a run at the resulting
pushed revision.

The bounded v0.8 prerelease-upgrade rehearsal now passes at clean revision
`3b4524e6cf1d169ba6a9e0e907d363f9eaada942`. The current binary accepts the
exact schema-v1 `.gox.toml` retained from Gox v0.1.0 and consumes the
schema-v1 Glippy configuration and baseline generated from first-identity
revision `b386ddcd57f7a01841b0c3c77acef8bde2b3550e`; both combined checks exit
successfully. This proves the concrete retained fixtures, not arbitrary future
configuration or cache compatibility. The v0.9 freeze must repeat the matrix
against its final tagless candidate. Stable-v1 progress remains 75% while the
exact corpus and resource gates are open.

Native aggregate process-tree campaign `32804781727` rejects the provisional
2 GiB typed ceiling at Glippy revision `385d981`: Linux amd64 reached
2,238,414,848 bytes, Linux arm64 reached 2,197,422,080 bytes, and Darwin amd64
reached 2,255,237,120 bytes. All editor and formatter samples stayed within
their existing budgets. The release workflow now provisionally uses a 3 GiB
typed ceiling, leaving 42.8% headroom over the largest first observation.
Darwin arm64 produced candidate diagnostic fingerprint `77f989f5...baba7`,
which remains unaccepted until the exact release-workload output is
adjudicated. The separately pinned SQLC corpus provides supporting rule-level
comparison but cannot replace that evidence because it uses a different source
revision. The release workflow now retains the exact normalized Darwin arm64
diagnostic output instead of exposing only its digest. A complete
exact-candidate four-runner rerun is still required, so the resource gate
remains open and stable-v1 progress remains 75%.

SQLC-only corpus run `32807031805` at Glippy revision `1a3acab` preserves the
v0.6 lint signal: default is clean and recommended contains the same three
actionable scanner diagnostics. Its formatter audit exposed one source-fidelity
gap, `comment 3 has no proven output owner`, in SQLC
`internal/sql/ast/type_name.go`. The `addMods:` label lowerer did not claim the
line comment between its colon and nested `if` statement. A focused regression
reproduced that exact failure before the shared label-gap fix and passes after
it. Review found the same ownership gap for comments after a label whose nested
statement is the parser's implicit empty statement; a second focused regression
failed before that extension and now passes. The complete formatter package and
an exact-checkout sequential per-file scan also pass without an internal
formatter error. Independent review found that same-line label suppression
directives were moved and rejected by source-equivalence validation; nested and
implicit-empty focused cases now retain those directives as label line suffixes
and pass. Re-review found mixed suffix/body and trailing implicit directive gaps
were collapsed; the label body now preserves those physical blank boundaries,
and all three focused cases pass. The adjacent suffix-only nested and
implicit-empty blank-gap cases now pass as well. A compatibility review caught
uncommented label spacing churn; the gap rule is now limited to comment-bearing
boundaries, and nested and implicit-empty canonical-spacing regressions pass.
Aggregate SQLC corpus proof still requires an exact pushed rerun, so v0.8
remains active at 75%.

Native campaign `32807509298` retained the Darwin arm64 typed
fingerprint at exact Glippy revision `4a80b27`: 351 findings comprise 343
`discarded-error`, five `resource-not-closed`, and three
`unchecked-scanner-error` diagnostics. The previously accepted workload had
the same counts plus one `nil-error-wrap` and one `unchecked-writer-error`; the
two removals match the documented impossible-branch and infallible-writer
precision changes, but counts cannot exclude offsetting diagnostic-identity
drift. The historical fingerprint remains enforced. The workflow now accepts
an optional historical Glippy revision and retains both exact normalized typed
outputs plus their unified diff before a replacement can be accepted.
Linux amd64 and arm64 passed the provisional 3 GiB campaign. Darwin amd64
recorded one 90.950-second formatter sample against the unchanged 90-second
ceiling, while the prior exact campaign ranged from 35.390 to 70.130 seconds;
one marginal observation does not justify widening the budget. A fresh
four-runner campaign at the final correction must still pass completely, so
v0.8 remains active at 75%.

Exact comparison workflow `32809489235` passed at current revision `344dc3d`,
historical revision `835a296`, typed revision `8a7cddf`, and Go 1.26.5 on
native Darwin arm64. It reproduced historical fingerprint `13f9c3dd...fdb53d2`
and current fingerprint `77f989f5...baba7`; the retained 32-line unified diff
has SHA-256 `6c265a40...479723d`. Its only changes are removal of the impossible
`nil-error-wrap` at SQLC `internal/cmd/parse.go:131` and the source-proven
infallible-buffer `unchecked-writer-error` at
`internal/codegen/golang/gen.go:252`. No other diagnostic identity, range,
message, or ordering changed. The replacement fingerprint is accepted and
bound for the next complete native campaign.

SQLC corpus workflow `32808812849` also passed at exact Glippy revision
`f720881` and SQLC revision `99a7d7d`. The source-fidelity audit completed all
619 selected files without an internal or equivalence error, including the
label-comment regression; 479 files have intentional canonical layout
differences. Safe-fix preview completed without mutation. Default remained
clean, recommended retained the three adjudicated `unchecked-scanner-error`
findings, and strict and pedantic completed with 392 and 441 findings. The
four-runner release-budget rerun remains the active v0.8 resource gate, so
stable-v1 progress remains 75%.

Release-budget workflow `32810535882` disproved the retained 90-second
formatter ceiling at exact Glippy revision `aac0dd4`. Darwin amd64 recorded
77.880 seconds for its first formatter sample and 99.660 seconds for its second,
following the independent 90.950-second breach in campaign `32807509298`.
The workflow now provisionally uses a 120-second per-sample maximum, leaving
20.4% headroom over the largest observation. Linux amd64, Linux arm64, and
Darwin arm64 completed the failed rerun; Darwin amd64 stopped at the old
ceiling, and reproducibility was correctly skipped. A complete exact-candidate
Go 1.27 campaign must still pass every native job and reproducibility gate, so
v0.8 remains active at 75%.

Go 1.27 release-budget workflows `32813174079` and `32813770238`
independently disproved the retained 105-second Darwin amd64 typed latency
ceiling with 174.090-second and 105.910-second cold samples. Both runs passed
all five Darwin amd64 formatter samples and every resource gate on the other
three supported native runners before reproducibility was skipped. The
workflow now provisionally uses a 240-second typed ceiling, leaving 37.9%
headroom over the largest Go 1.27 observation. A complete exact-candidate
campaign must still pass all four native jobs and reproducibility, so v0.8
remains active at 75%.

Exploratory corpus run `32832151515` reached the 120-minute Kubernetes job
ceiling during its final Staticcheck comparison. Before cancellation, the
formatter, safe-fix preview, all four Glippy profiles, and `go vet` had
completed. The four current profile durations were within roughly 1-3% of the
previous accepted Kubernetes run, whose lint-only audit completed in about 95
minutes; the added formatter and transactional fixer evidence, rather than a
profile regression, exhausted the old wall-clock allowance. The corpus job is
now bounded at 180 minutes. Cancellation also exposed concurrent deletion by
the runner's deferred cleanup and the workflow cleanup step; exact-root cleanup
now tolerates entries or the root disappearing concurrently while retaining a
failure when the owned root still exists after a real deletion error. The
complete exact corpus still must pass at the final pushed revision, so v0.8
remains active at 75%.

Release-budget workflow `32921575657` rejected the universal 120-second
formatter p80 at its Darwin amd64 job. The exact five samples were 92.200,
133.410, 120.710, 142.850, and 144.070 seconds, selecting 142.850 seconds as
nearest-rank p80. All formatter RSS samples remained between 1,381,236,736 and
1,592,143,872 bytes, so the 2 GiB ceiling remains unchanged. Linux amd64,
Linux arm64, and Darwin arm64 passed the same campaign. Darwin amd64 now
provisionally uses a runner-specific 180-second p80 and 360-second hard ceiling;
the other targets retain 120 and 240 seconds. A complete exact-candidate rerun
must pass before this becomes stable release evidence, so v0.8 remains active
at 75%.

The v0.9 CLI-freeze candidate now provides successful top-level help for an
empty invocation, `--help`, `-h`, and `help`, plus exact usage through
`help <command>` and each command's help flags. Unknown commands and malformed
help remain invalid invocations. The help path does not initialize project or
rule state, and Bash, Zsh, and Fish completion expose the same command surface.
This advances contract-freeze preparation without closing the still-running
exact v0.8 corpus gate, so stable-v1 progress remains 75%.

The tagless v0.9 release-budget candidate now rehearses more than archive
construction and version execution: every native extracted binary renders the
same help and rule catalog, reproduces the frozen hostile formatter golden, and
consumes the retained Gox v0.1.0 and early-Glippy upgrade fixtures. The
reproducibility job compares those digested contract snapshots across all four
supported runners alongside the release files. Upgrade consumption also fails
if either copied fixture tree changes and records a deterministic unchanged-tree
snapshot. This gate remains unproven until an exact pushed workflow completes,
and it does not advance progress while the v0.8 corpus run remains open.

The v0.9 text-contract freeze now pins exact top-level help and complete rule
catalog output in reviewed golden files. Focused CLI tests and every native
release-candidate rehearsal must match those files, so cross-platform agreement
alone cannot silently redefine command help, rule IDs, preset membership,
analysis tiers, or fix availability. Formatter, configuration, exit-code, and
machine-schema freeze evidence remains separate work, and stable-v1 progress
remains 75% until v0.8 closes.
