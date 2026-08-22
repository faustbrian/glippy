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
with `go vet` and exact Staticcheck v0.8.1. Go build, module, configuration,
telemetry, and temporary state is isolated outside every checkout. Audit
processes receive only a small compiler/runtime environment allowlist; caller
credentials and unrelated environment variables are not inherited. Go package
loading is offline during the audit, including direct version-control access.

Each repository result contains:

- normalized Glippy diagnostic and measured statistics JSON per profile;
- a sorted finding inventory per profile;
- normalized `go vet` and Staticcheck output; and
- one result document binding the repository, revision, tool versions, exit
  codes, completeness state, and deterministic artifact digests.

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
disposable checkouts and caches, fetches module inputs before the offline audit,
uploads normalized artifacts, and removes task-owned resources. It does not
publish an Action, modify upstream repositories, submit changes, or claim that
an optional-profile finding is a defect before manual adjudication.
The current corpus harness uses Go 1.26.6 because several pinned repositories
declare that patch level; this is evidence metadata rather than a change to the
separately documented Glippy release toolchain.

Every `default` and `recommended` finding must be classified as true positive,
false positive, duplicate of vet or Staticcheck, unsupported source/build
state, or unresolved before it can support a release claim. Strict and
pedantic results are calibration inputs unless a separate review records a
stronger conclusion.
