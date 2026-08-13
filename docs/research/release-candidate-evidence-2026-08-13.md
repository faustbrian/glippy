# Release Candidate Evidence, 2026-08-13

## Candidate

This record is bound to clean `main` revision
`06cce4a9e352be98751bafdbc4fa7a9835da947a` and Go 1.26.5. It is release
candidate evidence, not a tag, public release, or publication authorization.

## Source And Behavior Gates

GitHub Actions CI run
[`31697040231`](https://github.com/faustbrian/gox/actions/runs/31697040231)
checked out the exact candidate with empty task-owned Go caches and passed:

- all package tests;
- all package race tests;
- `go vet`;
- module tidy-diff validation;
- a fresh binary build; and
- the repository-wide non-mutating combined check.

The pinned formatter corpus at
`faustbrian/golib@f28f85133ac6d13169745807fc39e2d5ef6bf780` selected 5,138
files. The JSON result was complete with 4,807 expected formatting differences,
zero source or tool errors, and findings exit 1. This proves that every selected
file parses, formats, validates equivalently, and reaches a deterministic result;
it does not claim that the external repository already uses Gox formatting.

Every owned fuzz target then ran for 10 seconds against the clean candidate and
passed:

- source and fragment ledger reconstruction;
- complete-file and fragment formatting;
- document rendering;
- fix coordination;
- suppression parsing;
- configuration decoding;
- persistent cache entry decoding;
- package analyzer and native package cache decoding; and
- package fact snapshot decoding.

The source-ledger campaign includes the permanent malformed quoted-literal
regression that previously exposed omitted physical bytes after an overlapping
scanner token.

## Native Performance And Artifact Gates

Manual workflow run
[`31697171821`](https://github.com/faustbrian/gox/actions/runs/31697171821)
passed on the exact candidate. Each supported native target stayed below the
250-millisecond editor budget, 90-second formatter-corpus budget, and 2-GiB
formatter peak-RSS budget:

| Native target | Maximum editor latency | Maximum formatter time | Maximum formatter RSS |
| --- | ---: | ---: | ---: |
| Darwin amd64 | 13.615 ms | 41.700 s | 1,731,592,192 bytes |
| Darwin arm64 | 10.984 ms | 29.280 s | 1,512,849,408 bytes |
| Linux amd64 | 4.548 ms | 31.510 s | 1,635,545,088 bytes |
| Linux arm64 | 3.815 ms | 17.400 s | 1,413,726,208 bytes |

Each runner independently built the four target archives, manifest, and
checksum file, then executed the archive matching its native operating system
and architecture. The cross-runner job verified every checksum and required
all six files to be byte-identical across all four builders. Representative
artifact digests were:

| Artifact | SHA-256 |
| --- | --- |
| Darwin amd64 archive | `13595ffd74deb62a47836bfd17737d2ba1cfecad996189d0a002364d92143fbc` |
| Darwin arm64 archive | `7e8ff01fbaf1f1288e9be923cf92ce67b242b9a8b30db4a095a0b124b704d45d` |
| Linux amd64 archive | `e45dbefa254ba2b9fb72a1f6038864350717bdb448f64181d58db27dc3f4fef3` |
| Linux arm64 archive | `5988ef1a667bfa5a1397a6def90496bcc9da9709045ef0f70b1c93693aa8392f` |
| Manifest | `e84302429f66bb108b7a9fbd9a0de428f75f42dae281ae86a02c18ef1061ca53` |
| Checksum file | `6ab1cbd7e24f74bec989802e92d1afec5036ffda3c9bc2f625408d312cb2e6f4` |

The retained GitHub artifacts expire after 14 days. This record preserves the
observed budgets, manifest identity, and digests after those artifacts expire;
the workflow URL identifies the original run and job logs while GitHub retains
them.

## Remaining Human And Publication Gates

The candidate has no known correctness, source-fidelity, fix-safety,
determinism, supported-platform, or release-budget failure. It is not yet a
publicly distributable release because these legal and user-controlled gates
remain:

1. the maintainer must accept the refreshed Gox collision and trademark-risk
   boundary as the final public-name decision;
2. the maintainer must personally review and verify this complete candidate;
3. only after those decisions may a version tag be authorized; and
4. that tag must activate the GitHub Release and OIDC artifact-attestation
   workflow successfully.

No tag or release was created while collecting this evidence.
