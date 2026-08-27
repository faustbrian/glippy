# v1 Corpus Adjudication Summary, 2026-08-27

Pinned corpus run `33040254261`, attempt 1, passed all 20 jobs at Glippy
revision `8f52f20deea6ceb9bb316d312e26e65646ca9f6d` with Go 1.27.0 and
Staticcheck 2026.2.1. The canonical adjudication and report are:

- [`v1-corpus-adjudication-33040254261.json`](v1-corpus-adjudication-33040254261.json)
- [`v1-corpus-report-33040254261.json`](v1-corpus-report-33040254261.json)

## Findings

All 334 default and recommended findings are classified:

| Profile | True positive | Intentional | False positive | Duplicate vet | Duplicate Staticcheck |
| --- | ---: | ---: | ---: | ---: | ---: |
| default | 66 | 1 | 0 | 1 | 15 |
| recommended | 229 | 6 | 0 | 1 | 15 |

Every repository, profile, and finding fingerprint is identical to the
previously adjudicated exact-source candidate. The fresh template retained no
new or missing diagnostic fingerprint. The prior human classifications were
therefore transferred by repository, profile, and fingerprint, then validated
against the new result digests. No default or recommended false positive and no
new rule candidate exists.

## Formatter And Fixer

The formatter selected 29,657 files and reported 24,899 aggregate canonical
differences. It completed without an internal, parse, normalized-equivalence,
comment, directive, or idempotency failure on every supported and buildable
snapshot. The same four repository-wide formatter units remain deliberately
incomplete because their selected Go source versions are outside Glippy's
documented Go 1.25 through Go 1.27 window.

Transactional safe-fix preview completed on every otherwise supported
snapshot. CockroachDB still requires generated `TEAMS.yaml`; Go-Ethereum and
go-sqlite3 select unsupported source versions; and Moby retains its two
package-local cgo analysis boundaries. No external checkout was modified.

## Accepted Boundaries

Eleven ordered `not-actionable` gaps cover 19 deliberately incomplete
execution units. They are the same unsupported-source, missing-generated-input,
and cgo boundaries accepted by the v0.8 corpus audit. The report's
`unresolved` count measures those incomplete units even after their required
gaps are explicitly adjudicated; it does not represent unreviewed diagnostics.

## Decision

The final planned 17-repository corpus campaign is complete. Its result set is
unchanged from the previous fully reviewed candidate, zero known default or
recommended false positives remain, and every supported formatter and fixer
unit completed. Documentation and catalog-description changes do not affect
the corpus formatter, lint, or fix result paths, so this evidence carries
forward to the final audit-bearing candidate. Another complete corpus run is
required only if a later result-affecting change invalidates that boundary.

The corpus gate is closed. Stable-v1 progress remains 85% until the final
candidate CI, native release-budget/reproducibility run, and independent review
close the complete v0.9 acceptance audit.
