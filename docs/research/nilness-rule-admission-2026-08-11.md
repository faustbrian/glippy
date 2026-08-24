# `nilness` Rule Admission, 2026-08-11

> The return-state comparison boundary is refined by the
> [v0.6 testing-assertion precision record](v0.6-nilness-testing-assertion-precision-2026-08-24.md).

## Authority And Existing-Tool Boundary

The rule reports operations whose operand is proven nil, degenerate nil
comparisons, nil channel and map operations, nil panics, and invalid nil-slice
to array-pointer conversions. These defects compile, and Go 1.26.5 default
`go vet` does not register the `nilness` analyzer. A disposable module
containing a dereference dominated by `pointer == nil` passed `go vet ./...`
without a diagnostic. The default vet registry does include the narrower
`nilfunc` analyzer, so function-versus-nil comparisons are not the reason to
admit this rule.

The implementation authority is
[`golang.org/x/tools/go/analysis/passes/nilness`](https://github.com/golang/tools/blob/05f9cb5d358503005bd6f82b17916d226ca7b210/go/analysis/passes/nilness/nilness.go)
from x/tools v0.48.0, released 2026-07-09 at commit
`05f9cb5d358503005bd6f82b17916d226ca7b210`. The module proxy reported v0.48.0
as current during the audit, and the installed source SHA-256 matched the
upstream default branch at
`ea8fc8145bbcdc812fd00ad362f47b335762b8401c8a069e4a92efd66861e9d6`.

The upstream analyzer adds correctness coverage beyond the default toolchain,
and Gox can reuse it without maintaining a divergent nilness data-flow
implementation.

## Contract And Execution Boundary

`nilness` is registered as a warning in the opt-in `suspicious` preset. Its
diagnostics remain categorized as correctness and safety findings. The rule
requires SSA, excludes generated files and ill-typed packages, and offers no
fix. A source operation, comparison, or panic is the primary range; Gox maps
the upstream point diagnostic to the exact physical lexical token. The
upstream category becomes the stable message key.

The rule supplies Gox's already-built `ssa.Package` and one exact source
function as the `buildssa` prerequisite result expected by the current
analyzer. It therefore exercises the production shared SSA scheduler without
building a second program. The wrapper checks the analyzer prerequisite and
diagnostic shapes, converts panics to errors, and fails if a future x/tools
version changes those assumptions.

The upstream dominance walk intentionally loses some facts at control-flow
joins. Gox's shared SSA program also does not yet import the interprocedural
no-return facts supplied by the upstream analyzer stack, so findings whose
proof depends on a terminating callee may be missed. Both boundaries lower
recall without inventing nilness. Functions marked with `//go:cgo_unsafe_args`
remain excluded because the SSA form does not represent their runtime behavior
faithfully. These limitations are part of canonical metadata and appear
through `gox explain nilness`.

Automatic edits are not credible for these findings: the intended correction
may require changing a guard, returning early, initializing a value, or
redesigning control flow. No safe, suggestion, or unsafe fix is registered.

## Behavioral Evidence

The initial focused tests failed because the production registry did not know
`nilness` and package analysis had no selected deep-tier rule. After admission,
the focused package and public CLI tests pass.

The fixtures prove:

- a dereference dominated by a nil branch reports `nilderef` at `*`;
- a nil slice to non-empty array-pointer conversion reports
  `conversionpanic` at its opening delimiter;
- an impossible nil comparison reports `cond` at its operator;
- `panic(nil)` reports `nilpanic` at the call delimiter;
- the corresponding non-nil and unknown paths do not report;
- interprocedural no-return behavior is not guessed;
- suppressions retain the canonical rule identity and configured severity;
- generated files and ill-typed packages do not receive callbacks;
- diagnostics have no fixes; and
- `gox lint` is non-mutating while `gox explain nilness` renders the shared
  metadata.

The complete repository test suite passed after keeping the rule opt-in. An
earlier experiment in the `correctness` preset correctly forced every default
lint plan to SSA, but exposed incomplete typed support in standalone-file,
combined-check, and fix-only journeys. The opt-in boundary prevents those
existing product surfaces from regressing while explicit selection still uses
the package-aware SSA path.

## Cost And Dogfood Signal

The cost probe compares a no-op SSA callback with `nilness` over 100 source
functions and 100 findings. Five one-second, single-CPU samples on Darwin
arm64, Apple M4 Max, and Go 1.26.5 observed:

| Run | Time range | Median | Bytes per operation | Allocations per operation |
| --- | ---: | ---: | ---: | ---: |
| Shared SSA baseline | 1.07-1.91 ms | 1.51 ms | about 434 KiB | 7,513 |
| Shared SSA plus `nilness` | 1.41-3.23 ms | 2.07 ms | about 657 KiB | 8,825 |

The first implementation copied the immutable token ledger for every
diagnostic and allocated about 10 MiB per operation. Exact token lookup now
uses a non-copying immutable source query, reducing the observed allocation to
about 657 KiB. Timing variance makes this a proportional cost observation, not
a stable performance budget.

Two explicit, non-mutating `suspicious`-preset runs completed without package,
source, suppression, or tool errors:

| Corpus | Revision or state | Files | Diagnostics |
| --- | --- | ---: | ---: |
| Gox | current implementation worktree | 94 | 0 |
| x/tools analysis passes | v0.48.0 | 241 | 0 |

Zero findings establish no observed false-positive noise in this 335-file
sample. They do not establish recall; the focused positive fixtures and the
current upstream analyzer contract provide the positive evidence.

## Admission Decision

Admit `nilness` as the first built-in SSA correctness rule under the opt-in
`suspicious` preset. Revisit membership in the default `correctness` preset
only after typed rules work coherently in combined check, standalone-file, and
fix-only planning and after broader dogfood remains acceptably quiet. Revisit
the wrapper whenever x/tools changes the analyzer prerequisite, diagnostic,
or cgo exclusion contracts.
