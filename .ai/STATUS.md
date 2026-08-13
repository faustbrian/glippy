# Glippy Development Status

- Progress: 95%
- Current phase: Phase 5, ecosystem integration and stable release
- Phase 0 completed: 2026-08-09
- Phase 1 completed: 2026-08-11
- Phase 2 completed: 2026-08-13
- Phase 3 completed: 2026-08-13
- Phase 4 completed: 2026-08-13

The sections below retain chronological evidence from earlier checkpoints.
Embedded progress statements describe those checkpoints; the summary above is
the current state.

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
and Linux arm64 with overlayfs under Go 1.26.5. Gox supports macOS and Linux
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

The external `pkg/prompts` adoption is now a dedicated committed migration on
the isolated `feature/gox-prompts-adoption` branch at `d6b0fba8`. From immutable
baseline `8c9c1e7a`, Gox selects 77 files and changes 65 Go files; the complete
coordinated patch changes 69 files with 7,608 insertions and 3,598 deletions.
It pins Gox revision `d84842b`, makes Gox the sole formatter authority, and
passes its pinned format check, tests, race tests, vet, tidy-diff,
documentation, golangci-lint, and workspace-dependent comparison-module gates.
Sixty-three selected Go files intentionally remain non-fixed-points under
gofmt. The maintainer approved Phase 2 and the complete layout on 2026-08-13,
closing the formatter exit gate. The branch has not been pushed or integrated;
that delivery state does not reopen Phase 2 and requires separate authority.

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
