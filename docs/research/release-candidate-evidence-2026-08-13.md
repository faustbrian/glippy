# Release Candidate Evidence, 2026-08-13

## Candidate

This record is bound to clean `main` revision
`ccd1e290ee4f385198e3d40e6fc451111300e475` and Go 1.26.5. It is release
candidate evidence, not a tag, public release, or publication authorization.

## Source And Behavior Gates

GitHub Actions CI run
[`31690175325`](https://github.com/faustbrian/gox/actions/runs/31690175325)
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
[`31690694813`](https://github.com/faustbrian/gox/actions/runs/31690694813)
passed on the exact candidate. Each supported native target stayed below the
250-millisecond editor budget, 90-second formatter-corpus budget, and 2-GiB
formatter peak-RSS budget:

| Native target | Maximum editor latency | Maximum formatter time | Maximum formatter RSS |
| --- | ---: | ---: | ---: |
| Darwin amd64 | 17.261 ms | 86.380 s | 1,932,156,928 bytes |
| Darwin arm64 | 5.140 ms | 19.250 s | 1,741,996,032 bytes |
| Linux amd64 | 4.953 ms | 32.390 s | 1,567,236,096 bytes |
| Linux arm64 | 3.622 ms | 17.000 s | 1,308,467,200 bytes |

Each runner independently built the four target archives, manifest, and
checksum file, then executed the archive matching its native operating system
and architecture. The cross-runner job verified every checksum and required
all six files to be byte-identical across all four builders. Representative
artifact digests were:

| Artifact | SHA-256 |
| --- | --- |
| Darwin amd64 archive | `4f05a3bdc32a0212cf10009f3977d4aa12f5ec091285de71e05391e5cdede3d2` |
| Darwin arm64 archive | `f821bfe31436be6597258634ba2056ad2093d12ef6db31985ec82a4ac8c24450` |
| Linux amd64 archive | `3341a17700faf4de9b19dbe4f7e5b6f5aca792190b6bddd3ac4be6a6b5685bac` |
| Linux arm64 archive | `7a7bfc52e3c7efa2737d2c684b35e8cd0e4b229bf12a3b88ce2f6e000bb7b190` |
| Manifest | `ca04df0a8b4d94c2c98a50bcb2b839923a4268ee7c94aa87f5a15458c1557117` |
| Checksum file | `3b7bcc49bb67ab5fc8eff3f7ca5582faf4e5577a76d1ad0ca7f0e6709ead8622` |

The retained GitHub artifacts expire after 14 days. This record preserves the
observed budgets, manifest identity, and digests after those artifacts expire;
the workflow URL identifies the original run and job logs while GitHub retains
them.

## Remaining Human And Publication Gates

The candidate has no known correctness, source-fidelity, fix-safety,
determinism, supported-platform, or release-budget failure. It is not yet a
publicly distributable release because these legal and user-controlled gates
remain:

1. the maintainer must select a project license; release archives already
   reproduce the applicable Go, MIT, and BSD notices and must then also
   reproduce the project license;
2. the maintainer must accept the refreshed Gox collision and trademark-risk
   boundary as the final public-name decision;
3. the maintainer must personally review and verify this complete candidate;
4. only after those decisions may a version tag be authorized; and
5. that tag must activate the GitHub Release and OIDC artifact-attestation
   workflow successfully.

No tag or release was created while collecting this evidence.

The deterministic third-party notice implementation postdates the candidate
revision recorded above. The complete candidate evidence must be refreshed
after the project license is selected and packaged.
