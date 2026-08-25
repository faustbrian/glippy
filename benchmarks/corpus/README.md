# Pinned Corpus

This corpus is a Glippy validation input, not an adoption program. No external
source is copied into this repository and the runner never writes to a target
checkout.

`manifest.json` records exact repository revisions, license provenance, source
version policy, cgo and generated-code traits, and package patterns. The
validator rejects abbreviated revisions, unknown fields, duplicate or
uncanonical entries, unsafe patterns, and more than 32 repositories.

The runner verifies each checkout's revision, origin, root Go directive,
license file, and tracked, untracked, and ignored status before and after
analysis. Tools run against a task-owned read-only source snapshot, never the
checkout itself. It runs the `default`, `recommended`, `strict`, and `pedantic`
profiles with Glippy's cache disabled, then compares the same package patterns
with `go vet` and exact Staticcheck v0.8.1. A non-executing package-build
preflight (`go list -deps -test -export`) distinguishes analyzer findings from
package loading, type-checking, or build failure. Go build, module,
configuration, telemetry, and temporary state is isolated outside every
checkout. Audit
processes receive only a small compiler/runtime environment allowlist; caller
credentials and unrelated environment variables are not inherited. Go package
loading is offline during the audit, including direct version-control access.

Before package analysis, the runner also executes `glippy fmt --check` over the
read-only source snapshot with a task-owned canonical configuration. Corpus
repositories cannot supply formatter settings. A complete formatter result
proves that every selected file reparsed, passed normalized source and
suppression ownership validation, and was byte-idempotent through the
formatter's acceptance path. Formatting differences are recorded without
writing either the snapshot or the external checkout. Invalid machine output or
an incomplete formatter result is release-blocking evidence and must receive a
formatter-sourced crash or unsupported-construct gap in adjudication.

Each repository result contains:

- normalized Glippy diagnostic and measured statistics JSON per profile;
- a sorted finding inventory per profile;
- a normalized formatter report with selected-file and difference counts;
- normalized `go vet` and Staticcheck output; and
- one result document binding the repository, revision, tool versions, exit
  codes, completeness state, shared source/workflow run identity, and
  deterministic artifact digests.

Statistics retain measured durations and allocations and are intentionally
volatile. `result.json` records their filename and JSON validity, but does not
digest them as deterministic evidence.

Validate the manifest locally without running an external workload:

```sh
go run ./benchmarks/cmd/corpus-runner \
  --manifest benchmarks/corpus/manifest.json \
  --validate-only
```

Run substantial repositories only through the manual **Pinned corpus** GitHub
workflow. Those workloads are intentionally excluded from normal local tests
and pull-request verification because their aggregate CPU and memory demand is
not safe for a shared workstation. Pull requests that change the manifest or
runner execute only the inert validator and unit tests.

The manual workflow accepts `all` or one manifest repository ID. It creates
disposable checkouts and caches, copies each exact checkout into a read-only
snapshot, resolves the complete transitive dependency graph selected by each
module or aggregate `go.work`, including workspace replacements, with module
metadata read-only, fetches the resolved exact module versions outside module
and workspace source context through disposable task-owned module metadata,
then performs the audit offline. It
uploads normalized artifacts and removes task-owned resources. It does not
publish an Action, modify upstream repositories, submit changes, or claim that
an optional-profile finding is a defect before manual adjudication.
Every corpus child command receives Go's `GOMEMLIMIT=4GiB` soft limit so a
single large analysis degrades into recorded incomplete evidence instead of
exhausting the isolated CI worker. This is a harness containment policy, not a
release peak-memory budget.

An `all` run also collects the exact-run repository artifacts and initial
adjudication template into one review bundle. The aggregate report is generated
only after the required manual classifications and incomplete-run gaps have
been recorded.
The current corpus harness uses Go 1.26.6 because several pinned repositories
declare that patch level; this is evidence metadata rather than a change to the
separately documented Glippy release toolchain.

Every `default` and `recommended` finding must be classified as true positive,
false positive, duplicate of vet or Staticcheck, unsupported source/build
state, or unresolved before it can support a release claim. Strict and
pedantic results are calibration inputs unless a separate review records a
stronger conclusion.

After downloading all result artifacts from one exact manifest run, create the
canonical review document:

```sh
go run ./benchmarks/cmd/corpus-runner \
  --manifest benchmarks/corpus/manifest.json \
  --results /path/to/results \
  --adjudication-template > corpus-adjudication.json
```

Replace every `unresolved` classification that can be decided and retain a
specific reason for every finding. Supported classifications are:

- `true-positive`;
- `intentional` for an accurate diagnostic on deliberately accepted source;
- `false-positive`;
- `duplicate-vet`;
- `duplicate-staticcheck`;
- `unsupported-source`;
- `unsupported-build`; and
- `unresolved`.

Each repository entry binds the exact `result.json` digest and the statistics
digest for all four profiles. Validation also checks the recorded tool
versions, canonical profile results, normalized diagnostics, finding
inventories, and the digested `go vet` and Staticcheck outputs. Reusing an
adjudication with a different run or edited artifact is an error. All
repository results must carry the same run ID and exact Glippy, Go, and
Staticcheck version strings. The manual workflow derives that ID from the
Glippy source revision and GitHub workflow run.

Record demonstrated omissions in the ordered `gaps` list. A gap names its
repository and evidence, identifies `manual`, `vet`, or `staticcheck` as the
source, and classifies the issue as a crash, missed defect, or unsupported
construct. Its disposition is `backlog`, `nursery`, or `not-actionable`.
Backlog and nursery gaps require a proposed rule ID; not-actionable gaps must
not name one.

The template records incomplete default or recommended runs in
`incomplete_profiles` and failed comparator executions in
`incomplete_comparators`. Each affected repository requires a crash or
unsupported-construct gap, and every incomplete profile or comparator
contributes to the reported unresolved count even when its finding inventory
is empty. This keeps an unsuccessful analysis from appearing as clean release
evidence.

`incomplete_format` records an invalid or incomplete formatter report. It also
requires a crash or unsupported-construct gap and contributes to the unresolved
count. Formatter gaps use `formatter` as their source; analysis omissions use
the existing `manual`, `vet`, or `staticcheck` sources.

Validate the completed review against the exact manifest, revisions, result
schema, Staticcheck version, artifact digests, and finding fingerprints:

```sh
go run ./benchmarks/cmd/corpus-runner \
  --manifest benchmarks/corpus/manifest.json \
  --results /path/to/results \
  --adjudication corpus-adjudication.json
```

The adjudication is complete for release evidence only when the validator
reports zero unresolved default or recommended findings, profiles, and
comparators. It exits nonzero while unresolved evidence remains. This command
reads artifacts only; it does not rerun repositories or modify their
checkouts.

Generate the canonical aggregate after any adjudication edit:

```sh
go run ./benchmarks/cmd/corpus-runner \
  --manifest benchmarks/corpus/manifest.json \
  --results /path/to/results \
  --adjudication-report corpus-adjudication.json > corpus-report.json
```

The report revalidates the adjudication and exact result set. It records
classification counts, gap counts, aggregate formatter completeness and
difference totals, deterministic per-profile totals, every per-repository
statistics digest, and one evidence-backed queue entry for each backlog or
nursery rule candidate. Multiple gaps for one rule must use one consistent
disposition. The duration and allocation fields come from Glippy's process-local
statistics. `allocated_bytes` is allocation volume, not peak RSS or aggregate
process-tree memory; those budgets require isolated CI evidence.
An incomplete profile whose statistics output is not JSON remains in the
report with its bound artifact digest and `measured: false`; its crash or
unsupported gap is not discarded merely because cost data is unavailable.
