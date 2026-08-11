# `duplicate-condition` Rule Admission, 2026-08-11

## Defect And Existing-Tool Boundary

The first built-in rule reports a side-effect-free condition repeated within
one `if`/`else if` chain. When the earlier occurrence is false, an identical
later occurrence is also false, so the later branch cannot be selected. This
usually means a copied condition was not updated or an obsolete branch was not
removed.

Go 1.26.5 `go vet ./...` exited successfully for a disposable module containing
the defect. The compiler accepts it. Current Staticcheck independently defines
the same defect as
[SA4014](https://github.com/dominikh/go-tools/blob/d69e7ee19e2d79b721aa696626cea310c807dd3e/staticcheck/sa4014/sa4014.go)
at `go-tools` commit `d69e7ee19e2d79b721aa696626cea310c807dd3e`.
Its implementation also rejects a
chain containing an initializer or a condition that may have side effects and
processes each chain once.

Two public fixes establish that this is a real defect class rather than a
synthetic style preference:

- [Cloudflare Backoff commit
  `3fdf2d1620b5dadab9af24c8e9325c06dd9a9503`](https://github.com/cloudflare/backoff/commit/3fdf2d1620b5dadab9af24c8e9325c06dd9a9503)
  removed a copied `else if b.n != i` branch and names SA4014 in the commit
  message.
- [Arduino CLI commit
  `38ebf641ac1502ff4ee673ac2ce964220f91dec8`](https://github.com/arduino/arduino-cli/commit/38ebf641ac1502ff4ee673ac2ce964220f91dec8)
  removed an unreachable `else if err == nil` branch from
  `arduino/resources/helpers.go` as part of its recorded SA4014 fixes.

The rule therefore adds a correctness diagnostic that the default Go
toolchain does not provide.

## Contract

`duplicate-condition` is enabled as a warning in the `correctness` preset. It
requires only syntax, is interested only in `if` statements, excludes generated
files, and offers no fix. The diagnostic primary range is the repeated
condition; a related range identifies its first occurrence. Each distinct
repeated condition receives one diagnostic at its second occurrence, avoiding
duplicate noise when the same expression occurs three or more times.

The implementation compares deterministic formatted expression structure. It
does not infer semantic equivalence between syntactically different
expressions. A complete chain is ignored if any branch has an initializer or
its condition contains a call, channel receive, or address operation. Calls
include conversions because syntax-only analysis cannot distinguish them
without paying for types. These exclusions trade recall for a bounded
false-positive contract.

Deleting or changing a repeated branch requires developer intent, so no safe,
suggestion, or unsafe automatic fix is registered. `gox explain
duplicate-condition` renders the canonical metadata, examples, exclusions, and
no-fix state used by the scheduler.

## Behavioral Evidence

The public `gox lint --reporter=json` path proves default registration, warning
severity, exact primary and related byte ranges, help text, absence of fixes,
findings exit status, and non-mutation. Focused rule tests cover multiple
repetitions without nested-chain duplicate noise, distinct and separate chains,
calls, receives, address operations, initializers, comments between `else` and
`if`, type-invalid but parse-valid source, generated-file exclusion,
suppression, and severity overrides.

The initial end-to-end test failed with zero diagnostics while the production
registry was empty, then passed after registration. The broader focused rule
and CLI suite passed.

## Cost And Dogfood Signal

On Darwin arm64 with an Apple M4 Max and Go 1.26.5, five 200 ms benchmark
samples over a 100-chain, 300-condition file ranged from 1.76 to 2.86 ms per
complete source-backed analysis. The median was 1.92 ms, with approximately
492 KiB and 6,981 allocations per run. This includes constructing the isolated
syntax view, shared traversal, expression fingerprints, diagnostics, and
ordering; it is a scaling observation, not an editor-latency budget.

The default rule was then run without mutation over 7,466 files:

| Corpus | Revision or state | Files | Diagnostics |
| --- | --- | ---: | ---: |
| Gox | current implementation worktree | 61 | 0 |
| go-libraries | `cc7d2f9b7ba692ddea4b86aadaf8d28621a1fc26` | 5,075 | 0 |
| ack | `912ca202d24b25fce9e71f067c9f2e396539bc9b` | 54 | 0 |
| ics | `73f4702657113e1e9a06ef2425fbf45cd4b5a05c` | 15 | 0 |
| tarvero | `f14bfeae85cbd84df5f94c69a8e72a79acc493d1` | 1,567 | 0 |
| vuja | `870bc063adfe94fd23abbc6061c73f9c737935fe` | 694 | 0 |

External runs used immutable `git archive` snapshots and completed without
suppression, configuration, source, or tool errors. Zero findings establish no
observed false-positive noise in this sample; they do not establish recall.
The focused defect fixtures and two reviewed public fixes provide the positive
evidence.

## Admission Decision

Admit `duplicate-condition` as the first built-in correctness rule. Revisit its
syntax-only exclusions only if missed-defect evidence justifies typed analysis
or a narrower side-effect model. Revisit its default status if dogfood or user
reports produce a false-positive class not covered by the current exclusions.
