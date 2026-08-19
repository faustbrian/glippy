# ADR 0005: Shared Frontend And Analysis Tiers

- Status: accepted
- Date: 2026-08-09

## Context And Evidence

The Go 1.26.5 and x/tools v0.48.0 audit shows the standard parser, types,
packages, analysis, CFG, and SSA layers satisfy language and semantic needs.
The missing capability is concrete source fidelity, which a scanner ledger over
immutable bytes supplies without a replacement frontend. Detailed evidence is
in [`../research/go-frontend-audit.md`](../research/go-frontend-audit.md).

## Decision

Glippy uses separate scanner and parser passes over one immutable source version.
It stores physical byte offsets, token gaps, semicolon origin, raw comment and
directive bytes, and a source digest. Expensive analysis is selected through
lexical, syntax, types, CFG, and SSA tiers. One scheduler owns loading and
reuses representations within a run.

Syntax node-interest rules use one direct preorder AST traversal and dispatch
matching nodes to enabled rules. The scheduler does not build an
`ast/inspector` index for this single union query. The recorded 1, 3, 5, 10,
and 25-rule benchmark found one naive walk with a lower median at one rule and
the direct pass with lower medians from three through 25 rules. Direct dispatch reduced allocation
from roughly 28-30 KiB for inspector indexing to 456-1,896 bytes per traversal.
The scheduler keeps the direct path at one rule because the measured median
difference was 1.81 microseconds and does not justify a second execution path.

Typed prerequisite loading uses one run-owned `go/packages` graph for the
types, CFG, and SSA tiers. Requests require a normalized absolute working
directory, at least one non-empty package pattern, and normalized absolute
overlay paths. The loader requests typed root syntax, imports, module and embed
metadata, export data, type sizes, and test-variant ownership. Dependency types
needed to check the roots come from export data; dependency syntax and type-info
maps are requested only for fact or dependency analysis. The loader returns the
canonically ordered matched roots while retaining canonically ordered load and
type diagnostics from every populated package in the requested graph.
Package-local errors therefore preserve partial typed results instead of
becoming an opaque load failure.

One typed load admits at most 10,000 reachable packages, 20,000 unique parsed
Go source files, and 256 MiB of aggregate unique source bytes. These fixed
product defaults are intentionally lowerable by internal bounded callers but
are not another public configuration dialect. The parser reserves unique path,
digest, and size identity before constructing Glippy source units; the graph is
counted iteratively before any CFG, SSA, fact, cache, or rule execution. The
pinned x/tools loader bounds active I/O and CPU work, and Glippy serializes later
deep consumers within the one shared graph. Revisit these ceilings only when a
real repository cannot fit and new corpus plus peak-memory evidence supports a
different policy.

The package parser callback reserves one immutable source version from the
exact bytes supplied by `go/packages`, after overlays and build selection. It
rejects incompatible bytes for the same normalized absolute identity. Once
root ownership is known, reporter-visible roots receive the complete physical
source model. Fact-only dependencies retain compact immutable bytes, digests,
metadata, and physical-range validation alongside the package AST and types,
without formatter token, trivia, comment, directive, and reconstruction
indexes. Native rules that explicitly declare dependency syntax still receive
the complete dependency source model. Syntax-invalid or directive-invalid root
inputs remain available as diagnostic-only source units. Typed consumers map
package positions and edits to the bytes that were actually type-checked; they
do not reread a path after loading and guess that it is unchanged.

Native types-tier rules declare AST node interests and share one direct
preorder traversal per selected physical file. Each callback receives the
owning root package's `go/types.Package`, `types.Info`, opaque package ID,
package file set, type-error state, and the exact immutable source captured by
the loader. Package positions map through the physical file set with `//line`
adjustments disabled and are rejected when either endpoint belongs to another
file or falls outside the captured bytes.

Native package-wide types rules instead declare no node interests and run once
per canonical selected package that owns at least one eligible physical source.
Their context exposes every valid captured compiled file for package-wide
reasoning alongside the package's type information and architecture-specific
sizes, with an explicit target bit that preserves ordinary-package ownership
for production files and augmented-test ownership for test-only files.
Findings must return a target descriptor from that exact context, so a rule
cannot report against an unowned test variant, ineligible generated file,
dependency, or stale source version. This retains single-file diagnostic and
fix semantics while admitting cross-file package reasoning.

A package-wide native types rule may additionally declare dependency syntax in
its canonical metadata. The scheduler then extends the shared load exactly once
and supplies that rule with the complete transitive graph in sorted-import,
dependency-first order. Dependency descriptors expose shared typed state and
captured compiled files, but every such file is permanently non-target. Rules
without the declaration receive an empty dependency view even when another
native rule or an adapted fact graph caused the shared loader to populate the
graph. The declaration is part of human `explain` output and persistent native
cache snapshots, while dependency source digests and load selection remain
part of the existing cache key.

Native CFG-tier rules do not declare node interests. The CFG runner visits
every function declaration and nested function literal with a body once in
canonical physical source order. It constructs one
`golang.org/x/tools/go/cfg` graph per eligible function and shares that graph,
body, typed package values, package file set, type-error state, and exact source
with all eligible rules in rule-ID order. Reporter-visible diagnostics are
sorted after execution, and rule admission rejects observable behavior that
depends on the upstream graph's otherwise undefined block-slice order.

The shared no-return policy recognizes calls whose type information resolves
to the predeclared `panic`, a proven named function or method in the loaded
package set, a versioned same-module imported function summary requested by an
enabled effect consumer, or the documented exact standard-library terminal
set. It treats shadowed identifiers, dynamic and interface calls, and imported
helpers outside selected modules as returning. Effect inputs use independent
package loads and stable function identities, so their syntax does not become
lint targets or enter the shared SSA program. The graph also does not claim
short-circuit expression edges or complete abnormal panic flow; rules needing
those contracts require SSA or a later reviewed control-flow extension.

Native SSA-tier rules do not declare node interests or run on ill-typed
packages. The SSA runner filters the selected roots to well-typed packages
containing at least one physical source eligible for an enabled SSA rule, then
uses `ssautil.Packages` to build one run-owned program. Dependencies receive
type-backed package shells without speculative body construction. Each source
function declaration and literal maps to its package's built `ssa.Function`;
package initializer closures and methods are included, while synthetic wrappers
and range-over-function helpers are not separate callbacks. Rules share the
program, package, function, typed values, and exact source without mutation.

When tests are enabled, `go/packages` may expose one production file through
both its ordinary package and an augmented test variant. Glippy analyzes that
physical source once and prefers the ordinary package as its type owner; a
test-only source uses its test variant. This prevents duplicate diagnostics
while preserving the ordinary package's production type context. The loader
also removes the synthetic test-main package from the selected package set, so
its generated Go-cache artifact cannot become a reporter-visible lint target.
Native diagnostics remain root-package-only. Package-wide native rules inspect
dependencies only through the explicit metadata contract and cannot turn
dependency syntax into lint targets.

Cgo is a distinct ownership boundary because the toolchain replaces a file
that imports `C` with synthesized Go-cache `CompiledGoFiles` for type checking.
Glippy retains the original `GoFiles` bytes as the only syntax, formatting,
reporting, suppression, and edit identity and excludes every synthesized path
from rule ownership. The generated AST cannot be mapped losslessly back to
exact original bytes, so the original cgo file receives syntax analysis plus a
deterministic prerequisite diagnostic while deep callbacks and typed fixes are
refused. Revisit only if the standard frontend exposes or Glippy proves an exact
generated-to-original range map.

Suitable types-tier `go/analysis` analyzers may run package-wide over the same
load-owned syntax and type state after all native types, CFG, and SSA consumers
finish. Admission requires an explicit read-only audit because public Go APIs
cannot cheaply clone `types.Info` while preserving AST-key identity. Native
metadata continues to own selection, generated-file and type-error eligibility,
and fix safety. Adapted package analyzers remain deterministic by package and
rule ID, expose only captured compiled-source reads, and cannot use cross-file
related locations or multi-file fixes. Flagged analyzers require a factory that
returns fresh contract-identical graphs and exact one-to-one native option
bindings; shared analyzer flag mutation remains prohibited. A
deterministic prerequisite DAG may produce exact-type results once per package;
shared prerequisites execute once, diagnostics from metadata-less prerequisites
fail, and cancellation stops dependent analyzers.

An adapted analyzer DAG that declares package facts extends that same schedule
vertically through sorted imports. Dependency syntax and type information are
loaded only for this explicit requirement, each analyzer-package pair executes
once across shared roots, and only selected root diagnostics are visible.
Package facts are keyed by original analyzer identity, package identity, and
declared type. Gob export snapshots isolate later mutations; import decodes an
independent value; and enumeration exposes only the current package plus direct
imports in canonical order. Two unequal immediate encodings fail, while
maintainer admission remains responsible for excluding nondeterministic custom
Gob encoders and analyzer-owned state.

Object facts use the same dependency-first schedule and encoded-value
isolation. Each analyzer-package pair owns a run-local view keyed by exact
`types.Object` identity. A package inherits direct dependency views using the
current x/tools export-data overapproximation, so method and type facts can
cross an intermediate alias boundary while irrelevant unexported functions are
dropped. Exports for nil or foreign objects fail. Enumeration is canonical and
returns decoded copies, using encoded fact bytes to break ties between distinct
synthetic objects with otherwise identical source identity. Persistent object
facts now have a stable identity primitive that binds package paths to canonical
x/tools `objectpath` values and resolves them against a newly loaded type graph.
Objects outside the package API graph fail closed. Canonical, versioned
snapshots serialize one analyzer-package pair's owned package and object facts
with stable declared-type identity. Restore validates the complete snapshot
before merging it without overwriting different live facts. Cache consumer
wiring and export-data invalidation remain deferred; process pointers are never
persistence identities.

Rules skip generated files unless their metadata opts in. Packages with type
errors are also skipped by default; a types-tier or CFG-tier rule may explicitly
declare that partial type information satisfies its contract. Syntax-invalid
or source-model-invalid files remain diagnostic-only and are never traversed.

The suppression-aware package driver is a separate entry point from the
file-owned syntax driver. It resolves one rule selection, requires at least one
types-tier, CFG-tier, or SSA-tier rule, overwrites the loader requirement with
that selection's maximum tier, and performs one typed load. Unsupported
lexical rules fail before that load rather than disappearing from a mixed
selection. It then runs enabled syntax, types, CFG, and SSA rules over the
selected physical roots, orders their combined diagnostics, and applies each file's
suppression index once. Package/type diagnostics and source-model problems
remain separate result channels. Syntax-only callers retain the existing file
path and therefore never reach `go/packages`.

Package selection delegates module and `go.work` semantics to the active Go
toolchain and accepts explicit test inclusion, canonical build tags, overlays,
GOOS, and GOARCH inputs. Module loading is read-only by default and supports an
explicit vendor mode; arbitrary Go build flags and mutation-enabled module
resolution are not part of this boundary. Ordinary loads disable proxy and
direct VCS module resolution, checksum-database access, and automatic toolchain
download. Network use requires an explicit request and a caller-owned
environment. Ambient `GOPACKAGESDRIVER` execution is always disabled so this
boundary remains tied to the standard Go command driver; support for external
build-system drivers requires a separate security and identity contract.

The CLI selects this package boundary only when the enabled rule plan requires
types or higher. It converts exact files, directories, and terminal `...`
filesystem patterns into one read-only, test-aware package request after every
input resolves to the same project root and configuration. Syntax-only patterns
remain on physical-file discovery and cannot invoke `go/packages`. Required
package or source-model diagnostics map to source-error exit code 2 without
discarding valid partial rule results. Typed fixes use the same boundary without
persistent result reuse: the initial selection and every formatted candidate
receive fresh package loads, with candidate bytes supplied as an exact-path
overlay before the single-file transaction may replace source. One final fresh
load supplies complete reporting results after every serialized transaction.

An admitted fact-bearing analyzer may declare one versioned audited external
execution mode when exact in-process fact propagation cannot satisfy the
recorded resource budget. The first and only such mode is `printf-v1`. The CLI
runs native and fact-free adapted consumers first, releases their package graph,
and invokes its own executable as the exact upstream `printf` unitchecker
through `go vet -json -p=2`. One process-wide semaphore serializes this phase.
The runner inherits the loader's read-only, offline, local-toolchain, target,
workspace, overlay, and cancellation policy; bounds stdout and stderr; and maps
only exact retained root ranges and declared fixes back into the product result.
This does not admit arbitrary executables, runtime plugins, analyzer-selected
flags, or dependency diagnostics.

The dependency-first wave and stable fact-snapshot implementation remains the
in-process fallback when no external runner is installed. It also preserves the
serialization boundary for a future fact-bearing analyzer, but its selection is
not evidence that it meets the `printf-v1` 2 GiB product budget. The exact
decision and alternative measurements are recorded in
[`../research/v0.5-printf-fact-execution-2026-08-19.md`](../research/v0.5-printf-fact-execution-2026-08-19.md).

## Alternatives Rejected

- Custom Go parser/type checker: no evidenced missing standard capability and
  an unacceptable compatibility burden.
- AST-only fidelity: loses token gaps, semicolon origin, BOM/newline details,
  and normalized raw bytes.
- Always load types/CFG/SSA: violates editor latency and rule-cost boundaries.
- Independent rule loaders: duplicate work and permit incompatible type
  identities.
- An inspector index for one union query: pays a full indexing traversal and
  roughly 28-30 KiB without a repeated filtered query to amortize it.

## Consequences

Lexing occurs twice through public APIs. Physical positions are distinct from
logical reported positions. Typed loading may invoke Go tooling and requires
explicit environment/cache inputs. Arbitrary analyzers cannot automatically
share immutable state.

Syntax dispatch visits uninterested nodes once to avoid indexing or repeated
walks; a secondary index requires new representative benchmark evidence.

Fact-bearing adapted analyzers now have opt-in dependency-first cache reuse.
Heterogeneous per-path package configuration remains separate tier-runner work;
typed fix execution deliberately bypasses persistent analysis caching so
validation cannot accept a stale package result. Types-only requests do not
construct CFG or SSA, and CFG-only requests do not construct SSA.

Reporter-visible typed package ASTs and physical source models use separate
parser file sets. The package parser preserves `go/packages`' existing AST
object-resolution behavior for analyzer compatibility, while the complete root
source model keeps its isolated syntax, scanner ledger, trivia, directives,
and digest. Fact-only dependencies avoid that second parse and formatter index
retention while preserving exact bytes and type identity. The v0.5 sqlc probe
records a 53.00% worst-sample peak-RSS reduction with byte-identical
diagnostics; native dependency-source consumers retain the complete model.
The later exact `printf-v1` boundary reduces the same production workload to
1,306,836,992 bytes cold with an unchanged diagnostic digest and makes 2 GiB
the durable reference-host ceiling. Native Linux and final-candidate macOS
measurements remain required before that ceiling becomes a portable release
claim.

## Revisit Trigger

A concrete supported syntax or fidelity requirement cannot be represented by
original bytes plus scanner, parser, and token mappings, or a measured tier
cannot meet its correctness contract.
