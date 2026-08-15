# Contributor Architecture And Rule Authoring

Glippy is one binary with separate formatter and linter engines over a shared
source model. The repository intentionally keeps implementation packages under
`internal`; they are not an extension API or a compatibility promise. Add a
public package only after a concrete external consumer and versioning contract
exist.

The Go toolchain frontend remains authoritative. Changes should extend
`go/parser`, `go/ast`, `go/token`, `go/types`, `go/packages`, CFG, and SSA
integration rather than introduce a parallel Go parser or type system.

## Package Ownership

| Area | Owner |
| --- | --- |
| `internal/source` | Immutable bytes, tokens, trivia, directives, positions, fragments, and source limits |
| `internal/format/doc` | Layout document nodes and bounded deterministic rendering |
| `internal/format` | Go syntax lowering, comment placement, output validation, and idempotency |
| `internal/rules` | Native rule metadata, contexts, findings, options, and the built-in registry |
| `internal/analysis` | Tier scheduling; shared syntax, types, CFG, SSA, facts, and analyzer adapters |
| `internal/fix` | Fix selection, source identity, conflicts, edits, and validation transactions |
| `internal/config` | Typed versioned configuration and canonical identity |
| `internal/discovery` | Deterministic file selection and project boundaries |
| `internal/filesystem` | Root-confined snapshots and validated atomic replacement |
| `internal/cache` | Disposable persistent analysis results and bounded retention |
| `internal/report` | Deterministic text and schema-versioned machine output |
| `internal/cli` | Command parsing, orchestration, exit categories, and write disclosure |
| `cmd/glippy` | Process entry point only |

Dependencies should follow this ownership direction. A rule does not load its
own package, construct a private CFG or SSA graph, read a second source version,
write files, or format replacement bytes. The scheduler, source model, fix
coordinator, formatter, and CLI own those operations once per run.

## Formatter Changes

Start with the observable dialect. Define the construct's flat form, canonical
broken form, legal break points, comment behavior, required punctuation, and
width exceptions before changing lowering. The
[formatter specification](spec/formatter.md),
[formatter rules](formatter-rules.md), and
[equivalence contract](spec/equivalence.md) are the current authority.

Formatter work normally touches `internal/format/format.go` and, only when a
language-independent layout primitive is missing, `internal/format/doc`.
Preserve these boundaries:

- original source bytes and `source.File` remain immutable;
- layout decisions use document groups and lines rather than source-line
  rewriting;
- renderer fit work remains bounded;
- grammar-sensitive breaks respect semicolon insertion and trailing commas;
- comments and directives retain exact text and validated ownership; and
- successful output reparses, passes normalized equivalence, and is byte
  idempotent.

Add focused `.input` and `.golden` pairs under `testdata/format` for flat,
boundary-width, broken, nested, and commented forms. Add source-owned corpus
coverage under `testdata/corpus` only when its manifest records provenance and
the expected gofmt relationship. A deliberate output change also updates the
user-facing formatter rules, affected compatibility decision, and migration
examples in the same change.

Useful focused checks include:

```sh
go test ./internal/format/...
go test ./benchmarks -run 'Test(InitialCorpusIsValidGo|InitialCorpusTypeChecks|MotivatingLayoutsAreGofmtFixedPoints)$'
```

Run fuzzing and performance probes when the change affects token/trivia
reconstruction, comment attachment, document rendering, recursion, allocation,
or fit behavior. The reproducible commands and claim boundaries are in the
[benchmark guide](../benchmarks/README.md).

## Native Rule Contract

Every built-in rule implements `rules.Rule` through one immutable `Metadata`
value and exactly the cheapest execution interface that satisfies its
precision contract:

| Requirement | Interface | Callback scope |
| --- | --- | --- |
| lexical or syntax file | `SyntaxFileRule` | Once per immutable source file |
| syntax node | `SyntaxRule` | Declared AST node interests only |
| types node | `TypesRule` | Declared package AST node interests with shared type information |
| package-wide types | `PackageRule` | Once per selected typed package |
| control flow | `ControlFlowRule` | Each source function with a shared CFG |
| SSA | `SSARule` | Each source function with shared SSA |

Do not request types, CFG, SSA, dependency syntax, effect facts, generated files, or
type-error packages for implementation convenience. `PackageRule` is a
types-tier package callback; dependency syntax is available only when its
metadata declares that need. CFG and SSA rules receive same-module imported
effects only when their metadata declares that need. Rules report only through source ranges mapped by
their supplied context. Package-wide findings additionally use the exact
`PackageFile` owned by that callback.

Metadata is both the scheduling contract and the canonical source for
`glippy explain`. It includes the stable ID, summary, full documentation, default
severity, presets, minimum Go version, requirement, node interests, generated
and type-error policy, categories, typed options, fixes and their safety,
deprecation data, known limitations, and paired incorrect/correct examples.
`rules.NewRegistry` rejects incomplete or inconsistent metadata and snapshots
it so callers cannot mutate the active registry.

## Rule Admission Workflow

The [rule roadmap](rule-roadmap.md) defines the post-foundation investigation
order and evidence queue. A roadmap entry is not an accepted rule ID; the
candidate-specific admission record below remains the implementation gate.

Before implementation, add an evidence record under `docs/research` that
states:

1. the observable defect or undesirable behavior;
2. why the compiler, vet, or the default Go toolchain does not already make the
   rule redundant;
3. positive cases and nearby cases that must not report;
4. the expected false-positive boundary;
5. the cheapest required analysis tier;
6. generated-file, source-version, type-error, and suppression behavior;
7. whether a fix exists and why it is safe, suggestive, or unsafe; and
8. expected frequency and measured cost proportional to that frequency.

Then:

1. implement the rule in `internal/rules/<rule_name>.go`;
2. use context range helpers instead of calculating byte offsets from AST
   positions or rereading the file;
3. add complete behavioral tests in the adjacent `_test.go` file;
4. add a focused benchmark for a rule expected to run frequently;
5. register the rule in `NewDefaultRegistry` only after its metadata and
   behavior meet the admission gate; and
6. verify `glippy explain <rule-id>` renders the intended canonical documentation.

Regenerate the published catalog after any built-in metadata change:

```sh
go generate ./internal/report
```

The report package test compares `docs/lint-rules.md` byte-for-byte with a
fresh render, so manually edited or stale rule documentation fails verification.

Changes to suppression parsing, ownership, configuration, or reporting must
update the public [suppression reference](suppressions.md) and the normative
lint and configuration specifications in the same batch.

Changes to formatter output, stable rules, fixes, configuration, machine
schemas, CLI behavior, or supported targets must classify their compatibility
and migration impact under the
[compatibility and change policy](compatibility-policy.md).
Machine-reporting changes must update the public
[machine output reference](machine-output.md) in the same batch as the encoder
and its schema fixtures.

Rule tests cover positive diagnostics, conservative non-diagnostics, exact
primary and related ranges, severity and option variants, supported Go
versions, type-error behavior, generated files, suppressions, metadata, and
deterministic ordering. Fix-bearing rules additionally cover the safety class,
selected edit bytes, comment boundaries, fixed output, formatter interaction,
reanalysis, conflicts where relevant, and repeated-fix idempotency.

Run the narrowest tier-aware package first, followed by affected engine and CLI
tests. Typical commands are:

```sh
go test ./internal/rules
go test ./internal/analysis ./internal/fix ./internal/report ./internal/cli
```

## Fixes

Severity and fix safety are independent. A rule declares each named fix as
`safe`, `suggestion`, or `unsafe` in metadata and returns exact byte-range edits
against the source version supplied to its callback. A diagnostic should remain
available without a fix when comment ownership or another precondition makes
the edit contract unprovable.

Rules do not merge edits or choose conflict winners. `internal/fix` validates
ranges and source identity, orders edits, rejects overlaps and ambiguous
alternatives, reparses, formats, reanalyzes, and validates one file before the
filesystem layer may replace it. Multi-file transformations require a separate
transaction design and are not a shortcut through the single-file coordinator.

## `go/analysis` Interoperability

Suitable `analysis.Analyzer` values enter through the adapters in
`internal/analysis`. The adapter translates analyzer requirements, facts,
diagnostics, and suggested fixes into Glippy metadata and source-versioned
outcomes while preserving Glippy scheduling and safety policy. An analyzer does
not receive authority to bypass configuration, package selection, deterministic
ordering, generated-file policy, or fix classification.

Use a native rule when the contract depends on Glippy source trivia, formatter
ownership, tier-specific shared contexts, or a fix policy the analyzer API
cannot express. Do not add dynamic Go plugins while the internal rule boundary
and public extension policy remain intentionally closed.

## Final Change Review

Review the complete change against its observable contract rather than only
the edited package. Formatter changes include source fidelity, corpus, docs,
and compatibility consumers. Rule changes include metadata, configuration,
scheduling tier, reporting, suppressions, fixes, generated files, source
versions, and `explain` output. Cache, CLI, and filesystem changes include
failure classification, cancellation, deterministic ordering, and non-mutating
paths.

Run broader tests, race checks, fuzzing, corpus gates, or benchmarks only to the
scope needed by the claim. Record unavailable platform or external-repository
evidence as unverified instead of generalizing a local result. External corpus
source requires provenance and license review before it is copied into the
repository.
