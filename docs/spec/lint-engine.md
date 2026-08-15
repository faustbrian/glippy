# Lint Engine Contract

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in BCP 14
[RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown
here.

[RFC2119]: https://www.rfc-editor.org/rfc/rfc2119
[RFC8174]: https://www.rfc-editor.org/rfc/rfc8174

## Native Rules

Every rule MUST declare a stable ID, summary, full documentation, default
severity, preset membership, minimum source version, required analysis tier,
node interests where applicable, generated-file policy, diagnostic categories,
fix availability and safety, typed options schema, deprecation metadata, known
limitations, and paired incorrect/correct examples.

Every native node-scoped types-tier rule MUST declare node interests. It MUST
receive only matching AST nodes together with the root package's shared
`go/types.Package`, `types.Info`, opaque package ID, package file set, package
type-error state, and the exact immutable physical source captured during
loading. The callback MUST NOT mutate shared typed values.

A native package-wide rule MUST require the types tier and MUST NOT declare
node interests or implement another execution interface. It MUST run once per
canonical selected package that owns at least one eligible physical target.
Its context MUST expose all valid captured compiled files for that package in
physical-path order together with the shared package, type information,
architecture-specific type sizes, and file set, while marking only files
canonically owned by that package and admitted by the rule's generated-file
policy as reporter targets. This allows an augmented test package to inspect
production declarations without duplicating production diagnostics; its
test-only files remain its targets.

A package-wide finding MUST bind to a target descriptor from the same callback
context. Foreign, stale, generated-but-ineligible, dependency, or non-owning
test-variant descriptors MUST fail analysis. Primary ranges, related ranges,
and fixes remain single-file under the ordinary finding contract. Package-wide
rules receive the same immutable typed option snapshot and type-error policy as
node-scoped types rules and MUST NOT mutate shared AST or type state.

A package-wide types rule MAY declare that it requires dependency syntax. No
other native execution shape MAY declare that requirement. When at least one
enabled native package rule or adapted fact graph requires dependencies, the
package driver MUST request dependency syntax and type information once from
the shared load; caller-supplied loader preferences MUST NOT make dependency
syntax speculative. A declaring native rule MUST receive the complete
transitive dependency graph in deterministic dependency-first order. Each
dependency descriptor MUST expose its shared package, type information, type
sizes, file set, type-error state, and valid captured compiled files in
physical-path order. Every dependency file MUST be a non-target, and a native
rule that did not declare the requirement MUST receive no dependency
descriptors even when another selected consumer caused the shared load.

Every enabled native rule MUST receive the typed option values resolved from
its own metadata and the selected configuration. A rule MUST NOT observe
another rule's options or mutate string-list values retained by the run.
Required options MUST have no default and fail planning before source traversal
when absent. Optional options MUST declare a canonical metadata default of the
same type, and that resolved default MUST be present in the callback snapshot.
Integer option metadata MAY declare inclusive minimum and maximum bounds.
Registry construction MUST reject bounds on other option kinds, inverted
bounds, and out-of-range defaults. Configuration resolution MUST reject an
out-of-range value before any rule callback runs.

Every native CFG-tier rule MUST NOT declare node interests. It MUST run once
for every function declaration and function literal with a body in canonical
physical file and function-position order. All eligible CFG rules for one
function MUST receive the same graph, body, typed package values, package file
set, type-error state, and exact immutable physical source. A CFG rule MUST NOT
mutate those shared values.

A native CFG-tier or SSA-tier rule MAY declare that it requires semantic
effect facts. No cheaper tier MAY declare that requirement. When at least one
enabled native rule requires effects, the scheduler MUST load same-module
imported packages in deterministic dependency layers, derive versioned stable
function summaries, and install those summaries in the shared CFG and SSA
no-return predicate. It MUST NOT retain effect inputs as lint targets or expose
their independently loaded type objects to rules. Third-party and workspace
module effects remain unavailable until their identity and loading contracts
are separately admitted.

The CFG runner MUST construct `golang.org/x/tools/go/cfg` graphs only when at
least one enabled CFG rule is eligible for the function's file and package. It
MUST treat calls resolved by `go/types` to the predeclared `panic`, proven
package-local no-return functions, versioned same-module imported no-return
summaries requested by an enabled consumer, and the documented exact
standard-library terminal set as non-returning. It MUST conservatively treat
shadowed `panic` identifiers, dynamic calls, interface dispatch, and imported
helpers outside those sets as potentially returning. This CFG tier does not
model short-circuit expression edges or complete abnormal panic flow and MUST
NOT be presented as SSA-equivalent.

The upstream CFG block slice has defined entry ownership but otherwise
undefined order. A rule MUST NOT select or order diagnostics from raw block
iteration order. Reporter-visible diagnostics MUST be canonically ordered by
physical source identity and diagnostic fields after all rule callbacks.

Every native SSA-tier rule MUST NOT declare node interests and MUST NOT opt
into type-error packages. The SSA runner MUST use `x/tools/go/ssa` through one
`ssautil.Packages` program built from only the well-typed selected root
packages containing a physical source eligible for at least one enabled SSA
rule. It MUST create dependency package shells from the shared type graph.
Dependency bodies MUST NOT enter the SSA program. A declared effect-fact
consumer MAY cause the separate bounded effect summarizer to load same-module
dependency syntax; unrelated SSA rules MUST NOT trigger that work.

The SSA runner MUST build that program once and run each eligible rule once for
every source function declaration and function literal in canonical physical
file and function-position order. All SSA rules for one invocation MUST receive
the same program; rules for one package MUST receive the same SSA package; and
rules for one source function MUST receive the same SSA function, typed values,
and exact immutable physical source. An SSA rule MUST NOT mutate those shared
values. Synthetic wrappers and range-over-function helpers MUST NOT become
independent source-rule callbacks.

The scheduler MUST compute the maximum required representation across enabled
rules and MUST NOT construct types, CFG, or SSA speculatively. Syntax rules
MUST share one direct preorder AST traversal per file and receive only nodes
matching their declared interests. The initial scheduler MUST NOT build a
secondary inspector index for its single union dispatch; a secondary index MAY
replace the direct pass only when representative benchmark evidence shows that
its construction cost is amortized. Typed, CFG, and SSA values are run-owned
and MUST NOT cross incompatible `go/packages` loads.

The typed prerequisite loader MUST reject lexical and syntax-only requests. A
typed request MUST identify a normalized absolute directory and one or more
non-empty package patterns; overlay keys MUST be normalized absolute paths.
Matched roots MUST be ordered by opaque package ID, duplicate roots MUST
collapse only when they are the same loaded instance, and incompatible
duplicate IDs MUST fail the run. Package and type diagnostics MUST be retained
from every populated package in the requested graph and ordered by package ID,
position, message, and upstream error kind. Package-local errors MUST NOT
discard otherwise available typed roots.

One typed load MUST admit at most 10,000 packages in its complete reachable
graph, 20,000 unique parsed Go source files, and 268,435,456 aggregate unique
source bytes. Duplicate test variants of the same exact physical source count
once; incompatible bytes for one path remain an error. The source limits MUST
be enforced before constructing Glippy's immutable source model, and the package
graph limit MUST be enforced before CFG, SSA, fact, cache, or rule execution.
Internal callers MAY lower a limit for a bounded host or proving fixture but
MUST NOT raise the product defaults without new corpus and peak-memory
evidence. These aggregate limits complement the 64 MiB per-source limit.

Active package parsing and type checking use the bounded I/O and
`GOMAXPROCS`-derived CPU scheduling provided by the pinned `go/packages`
implementation. Glippy MUST NOT start independent package loads or duplicate
deep representations per rule. The scheduler MAY perform one serialized load
per same-module effect-dependency layer when an enabled rule declares that
requirement; the combined root and effect source set MUST remain within the
ordinary file and byte limits. Native types, CFG, SSA, fact, and cache work
within one load remains deterministically serialized unless a later bounded
scheduler has measured memory and ordering evidence.

Package loading MUST delegate module and workspace interpretation to
`go/packages` with explicit test, build-tag, module-mode, overlay, GOOS, and
GOARCH inputs. Build tags MUST be validated, deduplicated, and ordered before
the Go command boundary. Module mode MUST default to `readonly`; `vendor` is the
only other admitted mode, and mutation-enabled module loading MUST be rejected.
It MUST request typed syntax and type information for matched roots. It MUST
NOT populate dependency syntax and type-info maps unless enabled rules require
dependency or fact analysis. It MUST NOT construct CFG or SSA at this boundary.
Every graph is run-owned; types from separate loads MUST NOT be mixed.

The package parser boundary MUST capture the exact bytes supplied after overlay
and build selection into the shared immutable source model. Each normalized
absolute path MUST identify exactly one digest within a load; incompatible
bytes for one path MUST fail the run. Captured paths and source-model problems
MUST be canonical. A synthetic test-main package and its Go-cache source MUST
NOT become an analyzed file or a reporter target. Invalid selected inputs MUST
remain available as diagnostic-only source units rather than being discarded.

For cgo packages, the toolchain type-checks synthesized `CompiledGoFiles`
rather than the editable file that imports `C`. Glippy MUST capture that original
`GoFiles` source as the syntax, formatting, reporting, and suppression target,
and MUST NOT expose a synthesized Go-cache path as a diagnostic, formatting, or
fix target. Because the generated AST has no lossless exact-byte mapping back
to the original cgo source, types, CFG, SSA, and adapted analyzer callbacks MUST
skip that source and emit one deterministic package prerequisite diagnostic.
The run remains useful for syntax diagnostics and formatting but has a source
error outcome; typed fixes MUST therefore refuse replacement. A future exact
cgo position map MAY replace this conservative boundary only with source and
edit fidelity evidence.

Typed diagnostics and edits MUST resolve package positions against this
captured source index. A typed consumer MUST NOT reread a file after package
loading and treat the new bytes as the analyzed source version. The package AST
MAY use its own parser file set, but its parser mode MUST preserve the upstream
AST object-resolution compatibility expected by suitable `go/analysis`
analyzers.

The native types runner MUST traverse each selected physical root source at
most once and MUST order node-scoped and package-wide rule, package, file, and
diagnostic work deterministically. When test loading exposes a production file
through both its ordinary package and an augmented test variant, the ordinary
package MUST own that file's analysis; test-only files MUST retain their
test-variant owner. Dependency syntax MAY be loaded for declared fact or native
package dependency work but MUST NOT implicitly make dependencies lint targets.

Generated files MUST be excluded unless a rule explicitly opts in. A package
with type errors MUST be excluded for a rule unless that types-tier or CFG-tier
rule explicitly declares that it can operate on partial type information.
Lexical and syntax rules MUST NOT declare that policy. Invalid diagnostic-only
source units MUST NOT be traversed. Every typed range endpoint MUST map through
the package file set without logical `//line` adjustment to the callback's
exact physical file, and cross-file or invalid byte ranges MUST fail the run.

The suppression-aware package driver MUST resolve preset and severity policy
once and MUST replace the loader requirement with the maximum enabled tier. It
MUST require at least one types-tier, CFG-tier, or SSA-tier rule; syntax-only
work MUST remain on the file-owned driver and MUST NOT invoke `go/packages`.
Enabled lexical rules MUST fail before package loading rather than being
silently skipped from a mixed selection.

For one successful typed load, the package driver MUST retain canonical package
and type diagnostics plus source-model problems, run syntax, types, CFG, and
SSA rules only at their declared tiers, combine and order their diagnostics by
exact source identity, and apply each physical file's suppression index once.
Invalid diagnostic-only sources MUST remain represented by those distinct
problem channels but MUST NOT produce an analyzed-file record. Package-local
load or type errors MUST NOT discard valid file results when an enabled rule
explicitly admits partial type information.

The package result MUST retain the load's immutable source index so a text
reporter can map rule ranges without rereading the filesystem. The text reporter
MUST treat that index as read-only and MUST reject a file result whose path or
digest does not match its captured source.

Ordinary package loading MUST disable module proxies, private-module proxy
bypass, direct version-control resolution, checksum-database access, automatic
toolchain download, and ambient external package drivers. Network access MAY
occur only when the caller explicitly opts in and supplies the intended Go
environment. External `GOPACKAGESDRIVER` execution remains unsupported even
with network access enabled. The standard Go command invoked by `go/packages`,
including its selected cgo behavior, is the only subprocess boundary currently
admitted; cache ownership remains the caller's responsibility.

The default `correctness` preset group is limited to incorrect, unsafe,
ineffective, misleading, or highly suspicious behavior with measured signal.
Suspicious, performance, complexity, style, and pedantic groups are composable
opt-ins. Restriction rules are selected individually rather than as a group;
migration rules require an explicit target contract.

The file driver MUST union selected preset groups, apply explicit rule
overrides, apply ordered command-line lint levels, escalate warnings when
configured, and resolve typed options once. Command-line levels MUST preserve
the `--only`, `--except`, warning-set, restriction, migration, and irreversible
forbid semantics defined by the CLI contract.
It MUST then record the maximum enabled requirement, execute the shared syntax
runner once, and apply the source-versioned suppression index before returning
reporter-facing records. Unsuppressed diagnostics, suppressed diagnostics,
unused directives, and suppression problems MUST remain distinct outcomes.
The file driver MUST reject every enabled non-syntax rule rather than skip it
or construct a representation speculatively; typed work uses the separate
package driver.

The syntax-only CLI check MUST bind every discovered file to its selected typed
configuration before analysis, process normalized file paths deterministically,
and retain completed results when reporting an incomplete JSON failure. It MUST
classify visible diagnostics, suppression problems, and unused directives as
findings while allowing fully suppressed diagnostics to succeed. No check path
may invoke the fix coordinator or replace source bytes.

The CLI MUST choose package analysis only when the maximum enabled requirement
is types or higher. Syntax-only files, directories, and recursive filesystem
patterns MUST remain on deterministic file discovery and MUST NOT invoke
`go/packages`. A typed invocation MUST resolve every input to one project root
and configuration, convert each explicit file, directory, or terminal `...`
pattern into one package query relative to that root, enable test variants, and
use read-only module mode. Until per-path package configuration is designed,
heterogeneous roots or configurations MUST fail before package loading.

The typed CLI MUST classify any retained package-list, parse, type, or
source-model problem as a source error while preserving valid partial file
results. A package run that returns all available prerequisite, source, and rule
records without an engine error MUST remain complete even when its exit category
is source error. Typed fix planning MUST use a fresh cache-independent package
analysis, bind selections to the load-owned physical source digests, and
prevalidate generated-file and rooted-filesystem policy before replacement.
Each formatted candidate MUST be reanalyzed through an exact-path package
overlay. Package or source-model problems, a missing target result, or overlay
identity mismatch MUST reject that file's fixes without mutation; an analysis
engine failure retains its tool-failure category. After all serialized
transactions, one final fresh package analysis MUST replace every per-file
reporting result so diagnostics enabled by a later write remain visible.

The syntax-only fix driver MUST select no more than one safe named fix from each
visible diagnostic. A diagnostic with multiple safe alternatives is an invalid
rule contract, not permission to choose by registration or edit order. The
driver MUST complete configuration, source, generated-file, and symlink
prevalidation before replacement begins. For each source version it MUST run the
shared coordinator, formatter, and syntax driver again before atomic
replacement. Suggestion-only and unsafe diagnostics MUST remain visible and
unchanged under ordinary `--fix`.

## Diagnostics

A diagnostic MUST contain rule ID, resolved severity, stable message key and
text, precise physical primary range, optional related ranges, notes/help,
source identity and digest, named fixes, and fix safety. Every reporter MUST
consume the same globally sorted diagnostic records. JSON carries an explicit
schema version before external consumers are supported.

Schema-version-1 lint JSON MUST identify every analyzed file by normalized
absolute path and lowercase SHA-256 source digest. Primary and related ranges
MUST use half-open physical UTF-8 byte offsets. Ordinary lint JSON MUST expose
only fix name and safety, not source snippets or replacement text. It MUST keep
suppression syntax problems and unused directives distinct from rule
diagnostics, and MUST represent suppressed diagnostics by count without
disclosing their bodies by default.

Typed lint JSON MUST keep prerequisite diagnostics and source-model problems in
separate optional arrays rather than converting either into rule diagnostics or
generic tool errors. A prerequisite record MUST contain an opaque package ID, a
stable `unknown`, `list`, `parse`, or `type` kind, the upstream position when
present, and the message. An upstream `-` position MUST be represented as
absent. A source-model problem MUST contain its normalized absolute path,
lowercase SHA-256 digest of the captured bytes, and message. The summary MUST
count both channels when they are non-empty. The reporter MUST reject
unsupported prerequisite kind values, missing identities, digests, or messages,
and non-normalized source-problem paths.

Default lint text MUST render each primary diagnostic as a
`severity[rule-id]: message` heading followed by its physical path, line, byte
column, bounded source frame, and primary underline. One frame MUST render at
most six selected source lines and at most 160 source bytes from each line,
selecting a deterministic window around the primary range and marking cropped
prefixes or suffixes with an explicit `...`. Tabs MUST expand at a deterministic
width of eight cells; combining marks count as zero cells and wide or full-width
Unicode ranges as two. Terminal control and format characters in source and
human diagnostic fields MUST be escaped rather than executed. Related
locations, notes, help, and named fix safety MUST use visibly subordinate
continuation lines. Suppression syntax problems and unused directives MUST
remain visibly distinct and use the same bounded frame contract.

The `short` human reporter MUST instead render each primary diagnostic as
`path:line:byte-column: severity[rule-id]: message` with source-free indented
continuation lines. Both human reporters MUST order source files and
diagnostics canonically, validate the exact source identity plus every primary,
related, fix-edit, directive, and suppression-target range, and MUST NOT emit
fix replacement text. Physical locations follow the source model and MUST NOT
be adjusted by `//line` directives.

Typed lint text MUST render prerequisite diagnostics as
`position: package[kind] package-id: message` when a position exists and omit
only the position prefix when it does not. Source-model failures MUST render as
`path: source: message`. The reporter MUST order prerequisite diagnostics by
package ID, position, message, and kind; source problems by path and message;
and MUST render those channels separately from rule and suppression records.

Fix reporters MUST retain original-source identity for every applied or
rejected fix and use the post-format source identity for remaining diagnostics.
Default text MUST render each rejected rule, fix, reason, and original physical
location with the same bounded original-source frame; `short` MUST retain the
source-free location form.
JSON MUST distinguish pending, unchanged, confirmed fixed, stale or overlapping
conflict, failed, and possibly fixed files. A stale replacement MUST retain the
original analysis result; a possible post-rename failure MAY retain the
validated result digest but MUST NOT be presented as confirmed disk state.

## Baselines

Baseline application MUST occur after source suppression and before reporting
or fix selection. It MUST use exact rule IDs, stable message keys, portable
project-relative paths, exact source-span fingerprints, and occurrence counts.
It MUST NOT use message wording, absolute offsets, source snippets, or the wall
clock as identity. Matched diagnostics MUST remain separately countable and
MUST NOT participate in fix selection.

Stale and expired entries MUST be findings when their source file participates
in the current analysis. An entry outside the selected files MUST remain
unexamined rather than being reported stale. Generation MUST operate on visible
unsuppressed diagnostics for one root and configuration and MUST use the shared
safe filesystem writer. The public [baseline reference](../baselines.md) owns
the concrete schema and CLI behavior.

## Suppressions

Suppressions MUST name exact rule IDs. The grammar MUST define line, next-line,
range, and file ownership without allowing an unscoped silent disable-all.
`lint.suppressions.require-reason` MUST default to `false`. Unknown, malformed,
unused, and expired suppressions MUST be independently diagnosable.

Suppression ownership is based on physical token boundaries and source
identity, not incidental output line numbers. Formatting MUST preserve the
directive bytes and target relationship. A formatter change that would move a
suppression to a different target MUST be rejected.

The initial grammar accepts one exact rule per line comment:

```text
//glippy:ignore rule-id [-- reason]
//glippy:ignore-line rule-id [-- reason]
//glippy:ignore-start rule-id [-- reason]
//glippy:ignore-end rule-id
//glippy:ignore-file rule-id [-- reason]
```

`ignore` MUST target only the immediately following physical line;
`ignore-line` MUST target only the directive's physical line. A same-rule
`ignore-start` and `ignore-end` pair MUST target the half-open byte range
between the comments and MUST NOT nest. `ignore-file` MUST appear before the
package clause and MUST target the complete source file. The matcher MUST
suppress a diagnostic only when its primary range start belongs to the target.
Overlap by a larger enclosing range is insufficient.

The matcher MUST require the normalized source path and exact source digest
that produced the index, and MUST reject invalid or out-of-bounds diagnostic
ranges. When multiple same-rule directives match one diagnostic, the first
source-ordered directive MUST own it. Application MUST preserve diagnostic
order and MUST report every valid directive that owns no diagnostic as unused.

`--` introduces a reason. When `lint.suppressions.require-reason` is `true`,
starts and direct scopes MUST carry a non-empty reason. A missing or empty
reason MUST invalidate that directive so it cannot suppress a diagnostic.
Range ends MUST NOT carry a reason.

The first reason field MAY be `expires=YYYY-MM-DD`. The date MUST be a real
calendar date, and text after that field remains the human reason. An invalid
date MUST invalidate the directive. An expiry field without a human reason
MUST be treated as a missing reason. When the optional, explicit
`lint.suppressions.expiry-cutoff` is configured, an expiry on or before that
cutoff MUST produce an `expired` problem and invalidate the directive. Glippy MUST
NOT consult the wall clock; omitting the cutoff validates and retains structured
expiry metadata without deciding that a waiver has expired.

Unknown rules, malformed directives, missing reasons, invalid or expired dates,
misplaced file scopes, nested ranges, unmatched ends, and unclosed starts MUST
be reported in source order. The parser MUST NOT accept a directive that omits
a rule ID or disables all rules.

## `go/analysis` Interoperability

Suitable analyzers MAY be adapted without replacing the native scheduler or
metadata. A flagless syntax-only or types-tier analyzer MAY use one audited
descriptor. An analyzer graph with flags MUST instead use a factory that
returns a fresh graph for each syntax-file execution or typed invocation.
Syntax analyzers MUST NOT declare prerequisites, facts, or a result type.
Types-tier analyzers MAY declare a prerequisite-result DAG, package facts, and
object facts. Native
metadata MUST declare only the file node interest and either the syntax or
types tier; it remains authoritative for rule identity, selection, severity,
generated-file and type-error policy, documentation, and fix safety.

Each adapted file run MUST receive a fresh AST, its matching `token.FileSet`,
and a run-local analyzer descriptor. The syntax-only pass MUST expose exactly
one file, a package-name shell, no type information, an empty prerequisite
result map, and read access only to an independent copy of the adapted file's
exact bytes. Analyzer AST or descriptor mutation MUST NOT affect native rules,
other adapters, or a later run. An analyzer's own captured mutable state remains
its responsibility.

Every analyzer flag MUST map one-to-one to one native rule option, including
flags owned by typed prerequisites. Boolean, signed-integer, and string flags
MAY bind to native options of the same kind. String-list options, unsupported
flag value kinds, duplicate bindings, unbound options, and unbound flags
MUST be rejected. Distinct flags backed by the same detectable value storage
MUST also be rejected because binding order would otherwise select the winning
option. Resolved defaults and configured values MUST be applied to the fresh
graph before any analyzer callback. The adapter MUST NOT mutate a shared
analyzer flag set.

The factory MUST return two distinct graphs with identical analyzer topology,
documentation, URLs, result types, fact types, type-error policy, and flag
schema during admission. Every runtime graph MUST match that recorded contract.
A nil graph, reused admission graph, contract drift, factory panic, flag getter
or setter panic, flag parse failure, or missing resolved value MUST fail
explicitly. Typed execution MUST use one bound graph across its complete
package and fact schedule so
prerequisite identities and fact ownership remain coherent; independent typed
invocations MUST receive independent graphs.

A types-tier adapter MUST carry an explicit maintainer assertion that the
analyzer was audited as read-only over shared package AST and type state. It
MUST run only after native types, CFG, and SSA consumers have completed. The
adapter MUST reject a native type-error opt-in when the upstream analyzer does
not declare `RunDespiteErrors`. An eligible package run MUST receive the
load-owned file set, compiled syntax, package and type information, type sizes,
module metadata, and exact captured source bytes. Test variants MUST assign
each physical source to one canonical package owner, and the synthetic test
main MUST NOT become a lint target. The pass MUST expose no ignored files,
other files, or reads outside captured compiled Go source. Module replacement
traversal MUST be bounded.

Typed prerequisite analyzers MUST run once per package in deterministic
dependency order. Shared prerequisites MUST execute once, and each pass MUST
receive only its direct prerequisite results keyed by the declared analyzer
identity. Every returned value MUST exactly match its declared result type.
Prerequisite diagnostics MUST fail the run because they have no native
metadata; only the adapted root analyzer MAY produce mapped diagnostics.
Cancellation observed after a prerequisite MUST prevent dependent analyzers
from running.

When any analyzer step declares facts, the scheduler MUST load dependency
syntax and type information and execute the complete analyzer DAG in sorted
import dependency order before each selected root. One analyzer step MUST run
at most once for one package across shared roots. Only selected physical root
sources MAY produce reporter-visible diagnostics; diagnostics produced while
running the adapted root analyzer on dependencies MUST be discarded, while a
diagnostic from a metadata-less prerequisite MUST fail the run.

Package facts MUST be isolated by exact analyzer identity, package identity,
and declared fact type. Export MUST snapshot the value through Gob encoding;
two immediate encodings that differ MUST fail as nondeterministic. Import MUST
reset and decode into the caller's independently owned destination. Enumeration
MUST return only facts for the current package and its direct imports, ordered
by package path and fact type. Analyzer admission MUST separately audit custom
Gob encoders and fact values for deterministic representation: equal immediate
encodings are a runtime guard, not proof that arbitrary analyzer-owned mutable
state is deterministic. Object facts MUST preserve the same isolation and
declared-type checks.

An object fact export MUST name a non-nil object owned by the package currently
being analyzed. Imports MUST use exact run-owned `types.Object` identity and
MUST expose only the current package's facts plus facts inherited through its
sorted direct imports. Each import edge MUST retain the current x/tools
export-data overapproximation: exported package functions, methods, fields,
variables owned by the dependency, type names, and constants may propagate,
while irrelevant unexported package functions and unsupported object kinds MUST
not. The variable category includes x/tools' acknowledged overapproximation for
parameters and locals. This behavior and x/tools' corresponding type-graph TODO
MUST remain an explicit compatibility boundary rather than a claim of exact
export-data reachability.

`AllObjectFacts` MUST return independent decoded facts in deterministic package,
physical-position, object-identity, fact-type, and encoded-value order. The
encoded value MUST provide the final stable tie-breaker for distinct synthetic
objects whose other observable identity fields are equal. Stable object
identity MUST use the owning package path plus the canonical x/tools
`objectpath`, MUST resolve against the newly loaded package, and MUST reject
nil, predeclared, local, and otherwise unreachable objects.

One fact snapshot MUST bind its schema version, analyzer name, package path,
stable declared fact types, canonical object identities, and deterministic Gob
values. It MUST include only facts owned by that package; dependency snapshots
remain separate graph inputs. Decode MUST reject oversized, malformed,
noncanonical, unordered, duplicate, unknown-type, wrong-analyzer, wrong-package,
stale-object, and noncanonical-Gob data before mutating live facts. Restore MUST
reject a different existing value and MUST NOT partially replace facts.
Unsupported local object facts make that package snapshot uncacheable rather
than producing a partial warm result. The opt-in typed analyzer cache persists
one canonical entry per native-rule and analyzer-package pair. It restores
imports dependency-first across independent type graphs and commits diagnostics
and every analyzer-step snapshot only after complete validation. Direct
dependency keys are imported-fact inputs, so a stale dependency prevents a
parent hit. Unsupported local facts disable persistence for that package and
its dependents without changing the authoritative cold result.

The same caller-owned cache MAY persist one canonical result for the complete
selected native types, CFG, and SSA tier set over an error-free loaded graph.
The entry contains pre-suppression diagnostics so current suppression ownership
is always recomputed from the exact source. It MUST bind every selected rule,
including zero-diagnostic rules, to its current scheduling metadata, execution
shape, fix contract, and resolved options. Restore MUST validate every
diagnostic against that complete rule set, its source digest, physical owner
package, ranges, fixes, and canonical order. Any invalid entry is a miss; a
valid entry may skip native callbacks and CFG or SSA construction without
changing reporter or suppression behavior.

An ill-typed selected root MUST retain native metadata eligibility. If native
metadata admits type errors, every analyzer step MUST declare
`RunDespiteErrors`. A required ill-typed dependency whose fact-producing step
does not admit errors MUST fail instead of supplying missing or stale facts.

Typed adapted analyzers MUST run in deterministic package and rule-ID order.
Generated-only and ill-typed packages MUST be skipped unless native metadata
admits them. If `RunDespiteErrors` is active, the pass MUST receive the
load-owned type errors. Cancellation MUST be checked immediately before and
after each non-preemptible callback. An analyzer that mutates shared package
state, blocks without returning, or depends on omitted pass fields is not
suitable for this adapter even when its descriptor validates.

Imported diagnostics MUST enter the native deterministic ordering and
suppression pipeline. A syntax diagnostic MUST map every primary, related, and
edit position to its sole adapted source. A package diagnostic MAY target any
one captured compiled source, but every related location and suggested-fix edit
for that diagnostic MUST remain in its primary file. Foreign or invalid
positions, analyzer panics, undeclared suggested fixes, and unexpected non-nil
results MUST fail analysis. Each suggested-fix message MUST have an explicit
native name and description.

Imported fixes default to suggestion safety; a safe classification MUST carry
an explicit adapter audit assertion. Diagnostic help MUST resolve explicit,
relative, and category-derived URLs according to the upstream `go/analysis`
driver contract; invalid analyzer or diagnostic URLs MUST fail analysis.

The scheduler MUST check cancellation immediately before and after an adapted
analyzer run and MUST discard findings when cancellation was observed. The
`go/analysis` run callback has no context parameter, so the adapter cannot
preempt a callback that does not return; only bounded analyzers are suitable
until an independently cancellable execution boundary is proven.

Before registration, maintainers MUST audit that an analyzer does not depend on
deprecated object resolution or behavior absent from this pass contract. Such
an analyzer is not suitable and MUST NOT be registered. Declared or observed
unsupported CFG, SSA, analyzer flag value kind, cross-file related location, or
multi-file fix behavior MUST be rejected with a clear
compatibility diagnostic.

Rule documentation and `explain` output MUST derive from the same immutable
registry metadata and examples. Human `explain` output MUST include the rule
ID, summary, full documentation, default severity, presets, minimum Go version,
analysis tier, node interests, dependency-syntax policy, generated-file policy,
type-error package policy, categories, deprecation and replacement metadata
when present, named fix
safety, typed configuration, known limitations, and every paired example. Empty
fix, configuration, or known-limitation sets MUST remain explicit instead of
disappearing from the documentation contract.
