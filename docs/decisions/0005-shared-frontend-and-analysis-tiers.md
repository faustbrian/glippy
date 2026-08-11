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

Gox uses separate scanner and parser passes over one immutable source version.
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

The package parser callback also constructs the shared physical source model
from the exact bytes supplied by `go/packages`, after overlays and build
selection. It retains one immutable source version per normalized absolute
path, rejects incompatible bytes for the same identity, and exposes canonical
paths plus source-model problems. Syntax-invalid or directive-invalid inputs
remain available as diagnostic-only source units. Typed consumers therefore
map package positions and edits to the bytes that were actually type-checked;
they do not reread a path after loading and guess that it is unchanged.

Native types-tier rules declare AST node interests and share one direct
preorder traversal per selected physical file. Each callback receives the
owning root package's `go/types.Package`, `types.Info`, opaque package ID,
package file set, type-error state, and the exact immutable source captured by
the loader. Package positions map through the physical file set with `//line`
adjustments disabled and are rejected when either endpoint belongs to another
file or falls outside the captured bytes.

When tests are enabled, `go/packages` may expose one production file through
both its ordinary package and an augmented test variant. Gox analyzes that
physical source once and prefers the ordinary package as its type owner; a
test-only source uses its test variant. This prevents duplicate diagnostics
while preserving the ordinary package's production type context. Native typed
execution is currently node-oriented and root-package-only; package-wide rules,
dependency analysis, facts, CFG, and SSA require separate contracts.

Rules skip generated files unless their metadata opts in. Packages with type
errors are also skipped by default; a types-tier rule may explicitly declare
that partial type information satisfies its contract. Syntax-invalid or
source-model-invalid files remain diagnostic-only and are never traversed.

The suppression-aware package driver is a separate entry point from the
file-owned syntax driver. It resolves one rule selection, requires at least one
types-tier rule, overwrites the loader requirement with that selection's
maximum tier, and performs one typed load. Unsupported lexical, CFG, and SSA
rules fail before that load rather than disappearing from a mixed selection. It
then runs enabled syntax rules on the selected physical roots, runs typed rules
over the same source identities, orders the combined diagnostics, and applies
each file's suppression index once. Package/type diagnostics and source-model
problems remain separate result channels. Syntax-only callers retain the
existing file path and therefore never reach `go/packages`.

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

CFG and SSA construction, analyzer fact scheduling, cache reuse, package-wide
typed rules, and CLI package-pattern integration remain separate tier-runner
work. A types, CFG, or SSA request currently shares the same typed prerequisite
graph but does not mean that the more expensive representation has already been
constructed.

The typed package AST and physical source model use separate parser file sets.
The package parser preserves `go/packages`' existing AST object-resolution
behavior for analyzer compatibility, while the source model keeps its isolated
syntax, scanner ledger, trivia, directives, and digest. This additional parse
is the current cost of retaining both type identity and exact source fidelity
through public standard-library boundaries.

## Revisit Trigger

A concrete supported syntax or fidelity requirement cannot be represented by
original bytes plus scanner, parser, and token mappings, or a measured tier
cannot meet its correctness contract.
