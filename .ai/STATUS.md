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
filesystem, configuration, and CLI proof supports 45%. Windows and other
unverified filesystem semantics, release artifacts, and
external-repository adoption remain open before the 55% gate.

The replacement integration suite now passes on Darwin 27.0.0 arm64 with APFS
and Linux arm64 with overlayfs under Go 1.26.5. Windows amd64 filesystem, fix,
and CLI tests cross-compile, but no Windows runtime evidence exists and Go does
not promise atomic non-Unix rename. Release support is therefore scoped to the
two recorded platform/filesystem pairs; Windows, network filesystems, and
crash-durability under forced power loss remain unverified. This narrows the
platform gate without advancing progress beyond 45%.

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
fixtures and public fixes retain the positive evidence. No fix is offered
because removal, return, and a labeled loop exit have different semantics.
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
but the validation-to-open race and broader platform runtime evidence remain
open. Progress stays 45% behind the Phase 2 naming,
release, platform-runtime, and approved external-adoption gates.

Cache-root admission now resolves one immutable prospective target, validates
that target outside the selected project before creating directories, and
opens every resolved component through pinned rooted handles with identity
checks. A deterministic symlink-swap regression proves that changing the
caller-supplied link after validation cannot redirect cache writes into the
project, while rejected roots create nothing. This closes the
validation-to-open race in the current Darwin runtime evidence. Linux and
Windows runtime evidence for this path remains open, and the Phase 2 naming,
release, platform-runtime, and approved external-adoption gates keep overall
progress at 45%.

The Phase 2 prototype release builder now produces path-trimmed, cgo-free
Darwin arm64 and Linux arm64 binaries with explicit linked versions inside
normalized tar/gzip archives. A versioned manifest binds the complete source
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
the extracted current-host binary's version command. The final name, an
independent Linux-host artifact rehearsal, signing/publication, Windows runtime,
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
