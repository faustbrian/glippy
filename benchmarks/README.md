# Phase 0 Baselines

These benchmarks measure the cost of the standard Go frontend operations that
Glippy expects to compose. They are baselines, not product performance claims or
CI thresholds. The workload is an owned, valid, comment-bearing Go file with
blocks, generics, calls, and boolean expressions.

## Reproduction

```sh
go test ./...
go test ./benchmarks -run '^$' -bench 'Benchmark(Scan|Parse|GoFormat|ASTInspect|InspectorBuildAndFilter|TypeCheck)$' -benchmem -count=5
GOMAXPROCS=1 go test ./benchmarks -run '^$' -bench '^BenchmarkSyntaxRuleTraversalStrategies$' -benchmem -benchtime=500ms -count=7
go test ./benchmarks -run '^$' -bench '^BenchmarkPackagesLoadSyntax(Cold|Warm)BuildCache$' -benchmem -benchtime=1x -count=3
GOMAXPROCS=1 go test ./benchmarks -run '^$' -bench '^BenchmarkPackageAnalyzerFactCache$' -benchmem -benchtime=1x -count=5
GOMAXPROCS=1 go test ./benchmarks -run '^$' -bench '^BenchmarkNativeAnalysisResultCache$' -benchmem -benchtime=1x -count=5
GOMAXPROCS=1 go test ./internal/cli -run '^$' -bench '^BenchmarkLSPWorkspaceUnrelatedDocumentChange$' -benchmem -benchtime=3x -count=3
GOMAXPROCS=1 go test ./benchmarks -run '^$' -bench '^BenchmarkGlippyFormatManyClassicLoops$' -benchmem -benchtime=10x -count=3
go test ./internal/format/doc -run '^$' -bench '^BenchmarkRenderAdversarial(Nesting|Siblings)$' -benchmem -benchtime=3x -count=5
go test ./internal/format/doc -run '^TestRenderBoundsAdversarialDepthAndBreadthAllocations$' -count=1
go test ./internal/format/doc -run '^$' -fuzz '^FuzzRenderDeterministic$' -fuzztime=30s -timeout=45s

(
  task_cache=$(mktemp -d "${TMPDIR:-/tmp}/glippy-editor-benchmark.XXXXXX")
  trap 'find "$task_cache" -mindepth 1 -delete; rmdir "$task_cache"' EXIT HUP INT TERM
  GOWORK=off GOCACHE="$task_cache" go test ./benchmarks -run '^$' -bench '^BenchmarkGlippyEditorStdin$' -benchmem -benchtime=10x -count=5
)
./benchmarks/editor-latency.sh
./benchmarks/peak-rss.sh
```

The first command is a functional check. Benchmark setup that is not part of
the measured operation is performed before the timer starts. Package loading
is measured separately because it invokes Go tooling and is orders of
magnitude more expensive than file-local work. The cold variant creates an
empty disposable `GOCACHE` for every measured load and removes it outside the
timer. The warm variant preloads one disposable `GOCACHE`, reuses it for the
measured loads, and lets the test harness remove it afterward. Both force
`GOWORK=off`; the module download cache is unchanged because the workload has
no non-standard-library dependencies.

## v0.5 Typed Peak-RSS Gate

The peak-RSS probe's typed workload is the default correctness policy plus
`-Wsuspicious`; a suspicious-only configuration is not equivalent because it
omits default correctness analyzers and their dependency facts. The script
accepts `GLIPPY_PEAK_RSS_TYPED_ROOT`, `GLIPPY_PEAK_RSS_TYPED_REVISION`,
`GLIPPY_PEAK_RSS_TYPED_BUDGET_BYTES`,
`GLIPPY_PEAK_RSS_TYPED_BUDGET_SECONDS`, and the optional
`GLIPPY_PEAK_RSS_TYPED_OUTPUT_SHA256` diagnostic fingerprint. The fingerprint
normalizes the selected typed root to `<TYPED_ROOT>` before hashing.

The 2026-08-16 sqlc campaign at
`8a7cddfbb9088666eb981645285d7699e71dcb54` reduced peak RSS from
7,369,064,448 bytes to a worst optimized sample of 3,463,626,752 bytes while
preserving the exact 2,839-line diagnostic fingerprint. Four optimized
samples used 3,236,823,040-3,463,626,752 bytes and completed in 23.19-31.13
seconds. The final-tree confirmation used 3,429,040,128 bytes. The complete
workload, heap attribution, command, and limitations are
recorded in
[`../docs/research/v0.5-memory-reduction-2026-08-16.md`](../docs/research/v0.5-memory-reduction-2026-08-16.md).

The 2026-08-19 follow-up isolates the exact upstream `printf` fact graph in a
serialized same-binary unitchecker process. Three production-path samples used
1,306,836,992 bytes cold and 552,550,400-555,696,128 bytes warm, completed in
20.330 seconds cold and 1.530-1.600 seconds warm, and preserved normalized
diagnostic SHA-256
`030f4474aec74877307118376e77e8ad7254a46c32777242fd11e3223c7282d0`.
The script's default typed ceiling is now 2,147,483,648 bytes; callers may set a
lower explicit budget but must not raise a release claim without new recorded
evidence. The execution decision and remaining portable-evidence boundary are
in
[`../docs/research/v0.5-printf-fact-execution-2026-08-19.md`](../docs/research/v0.5-printf-fact-execution-2026-08-19.md).

### Retained-heap phase profiles

The opt-in external typed-analysis test writes one garbage-collected heap
profile after package loading, source-model construction, effect facts, each
analysis tier, adapted analyzers, and final result assembly. It is a diagnostic
harness rather than a latency or peak-RSS gate because forcing garbage
collection and writing profiles deliberately changes execution behavior.

Run it against an existing pinned checkout and an empty output directory:

```sh
GLIPPY_TYPED_PROFILE_ROOT=/path/to/sqlc \
GLIPPY_TYPED_PROFILE_DIR=/path/to/empty/profiles \
GLIPPY_TYPED_PROFILE_GO_VERSION=go1.26 \
go test ./benchmarks -run '^TestProfileExternalTypedAnalysis$' -count=1 -v
```

Ordinary tests skip the external workload when `GLIPPY_TYPED_PROFILE_ROOT` is
unset. The analysis phase observer is not enabled by the CLI or LSP and cannot
change ordinary diagnostics, caching, or runtime behavior.

## v0.5 Incremental Workspace Result Probe

The owned two-package LSP benchmark changes one open package while keeping an
unrelated open package unchanged. Every measured operation must perform exactly
one package load; two loads would prove that the supposedly unaffected result
was not reused. Three three-operation samples on the non-isolated Apple M4 Max
host measured 28.55-35.87 ms/op, 557,989-558,144 B/op, and 3,926-3,929
allocations/op with exactly 1.000 package load/op. This is a focused
same-process reuse probe, not a portable editor-latency budget or evidence of
incremental type checking within the changed package. The complete contract and
limits are recorded in
[`../docs/research/v0.5-incremental-workspace-analysis-2026-08-16.md`](../docs/research/v0.5-incremental-workspace-analysis-2026-08-16.md).

The workspace result session also enforces a deterministic 256 MiB accounted
memory budget across at most eight entries. Three samples after adding that
eviction policy measured 30.66-51.96 ms, 565,042-565,216 bytes, and
3,982-3,983 allocations per operation while preserving exactly 1.000 package
load per operation. The accounting policy is a deterministic retained-source
weight and does not substitute for the separate process-RSS gate.

## Initial Results

Recorded 2026-08-09 on an Apple M4 Max with 128 GiB RAM, macOS 27.0
(26A5388g), Go 1.26.5, `darwin/arm64`. The machine was not isolated, so elapsed
times show substantial scheduler variance. Allocation counts were stable and
are the more useful initial comparison point.

| Workload | Median time | Observed range | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Scan | 61.2 us | 48.9-93.7 us | 1,960 | 106 |
| Parse with comments | 408.7 us | 351.4-801.8 us | 17,248-17,251 | 460 |
| `go/format` | 985.3 us | 696.4 us-1.36 ms | 21,080-21,132 | 510 |
| `ast.Inspect` | 27.0 us | 22.0-30.8 us | 24 | 2 |
| Build inspector and filtered traversal | 60.9 us | 13.6-372.9 us | 28,648-28,650 | 7 |
| Type check | 3.87 ms | 2.68-12.0 ms | 243,343-250,104 | 2,345-2,347 |
| `packages.Load` syntax and types, cold build cache | 7.30 s | 6.46-11.30 s | 156,172,168-156,390,624 | 1,644,467-1,645,176 |
| `packages.Load` syntax and types, warm build cache | 4.19 s | 3.22-4.78 s | 155,999,760-156,246,992 | 1,644,191-1,644,351 |

No timing budget is set from this run. A stable benchmark runner and larger
representative workloads are prerequisites for regression thresholds. Future
records must continue to keep cold and warm package-loading results separate
and include peak resident memory outside the Go benchmark allocation metric.

## Package Fact Result-Cache Probe

The 2026-08-11 result-cache probe runs one deterministic fact-bearing adapted
analyzer over the owned workload module and its 42-package reachable graph.
Each cold operation starts with an empty persistent result store and includes
entry publication. Warm setup populates one store outside the timer; each
measured operation performs an independent `go/packages` load and restores the
analyzer-package entries into its new type graph. Both variants therefore
include package loading, source capture, cache identity construction, and
reporting. The analyzer execution counter is an additional functional metric:
a warm operation is invalid if any analyzer package reruns.

Five one-operation samples ran with Go 1.26.5, `GOMAXPROCS=1`, and
`darwin/arm64` on an Apple M4 Max. The build cache remained task-owned and warm
within the bounded campaign; result-cache roots were task-owned and removed by
the benchmark harness.

| Result cache | Median time | Observed range | Analyzer runs/op | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Cold population | 1.32 s | 883 ms-3.53 s | 42 | 350,266,520-350,612,800 | 3,849,678-3,851,420 |
| Warm restore | 478 ms | 442 ms-1.18 s | 0 | 350,922,856-351,304,184 | 3,857,489-3,857,603 |

Raw elapsed samples, in nanoseconds per operation:

```text
cold 3525387125 1363047709 1318246793 883456001 1146057875
warm  498534333 1179880958 441888208 478344875 458701208
```

The zero warm analyzer executions prove functional reuse across independent
type graphs. The lower warm median is directional evidence only: the host was
not isolated, subbenchmarks were ordered, and package loading still dominates
both allocation profiles. No CI latency or allocation threshold is set. A
larger module/workspace corpus, isolated runner, CLI-owned cache policy, and
peak-resident-memory measurement remain prerequisites for a product-wide warm
cache performance claim.

## Native Analysis Result-Cache Probe

The 2026-08-12 native result-cache probe runs cumulative native rule plans over
the owned workload module: types includes one node-scoped and one package-wide
rule, control flow adds one per-function CFG rule, and SSA adds one per-function
SSA rule. Every rule deliberately produces no diagnostics, so the probe also
exercises complete rule-set persistence for zero-diagnostic callbacks. Each
cold operation creates an empty result store outside the timer and includes
entry publication. Warm setup populates one store outside the timer, while each
measured operation performs an independent package load and restores its native
result. Package loading, source capture, cache identity construction, and
reporting remain inside both measured paths.

Five one-operation samples ran with Go 1.26.5, `GOMAXPROCS=1`, and
`darwin/arm64` on an Apple M4 Max with 128 GiB RAM and macOS 27.0 (26A5388g).
The task-owned build cache remained warm within the bounded campaign; all
result-store roots and the build cache were removed afterward.

| Maximum tier | Result cache | Median time | Observed range | Native callbacks/op | Bytes/op | Allocs/op |
| --- | --- | ---: | ---: | --- | ---: | ---: |
| Types | Cold population | 125 ms | 111-283 ms | types 8, package 1 | 2,919,728-3,681,072 | 11,660-12,896 |
| Types | Warm restore | 112 ms | 92.7-149 ms | all 0 | 2,920,720-2,921,056 | 11,617-11,621 |
| Control flow | Cold population | 121 ms | 95.3-198 ms | types 8, package 1, CFG 3 | 2,926,192-3,059,024 | 11,748-11,776 |
| Control flow | Warm restore | 150 ms | 124-214 ms | all 0 | 2,924,672-2,925,496 | 11,644-11,654 |
| SSA | Cold population | 118 ms | 106-160 ms | types 8, package 1, CFG 3, SSA 3 | 3,139,208-3,272,120 | 13,407-13,435 |
| SSA | Warm restore | 130 ms | 104-162 ms | all 0 | 2,928,632-2,929,248 | 11,679-11,683 |

Raw elapsed samples, in nanoseconds per operation:

```text
types/cold 282506709 119151667 110935292 124788416 155787042
types/warm 148704125  92967792 119071375 112349917  92726583
cfg/cold   121155458 118864292 125968124 198338751  95344042
cfg/warm   214029209 123711709 129681834 208126500 149525958
ssa/cold   159720208 106366749 118163417 148926000 113805583
ssa/warm   161502958 104205750 106707500 159895417 130179791
```

Zero callbacks in every warm sample prove that valid hits bypass node-scoped
types, package-wide types, CFG, and SSA execution, including graph construction
owned by the latter runners. The small workload and non-isolated host do not
show a consistent elapsed-time improvement: package loading dominates, the
subbenchmarks were ordered, and control-flow and SSA warm medians exceeded cold
medians under observed scheduler variance. Lower steady-state warm allocations
at the CFG and SSA tiers are directional only. No CI threshold or product-wide
performance claim is set; an isolated runner, larger module/workspace corpus,
peak-resident-memory measurement, and CLI-owned cache lifecycle remain open.

## Syntax Traversal Strategy Probe

The 2026-08-11 Phase 3 probe compares three schedulers over the owned hostile
AST and identical no-op rules distributed across calls, binary expressions, and
function declarations:

- one direct `ast.Inspect` pass with node-interest dispatch;
- one `ast/inspector` index plus one union-filter query; and
- one complete `ast.Inspect` pass per rule.

Parsing and rule construction occur before the timer. Each measured operation
reconstructs the strategy's dispatch state and invokes the same number of rule
callbacks. Seven 500-millisecond samples ran with Go 1.26.5,
`GOMAXPROCS=1`, and `darwin/arm64` on an Apple M4 Max. Timing varied materially
under host contention; allocation results were exact within every row.

| Rules | Strategy | Median | Observed range | Bytes/op | Allocs/op |
| ---: | --- | ---: | ---: | ---: | ---: |
| 1 | direct shared pass | 7.66 us | 4.11-20.7 us | 456 | 5 |
| 1 | inspector union query | 63.5 us | 43.6-138 us | 28,672 | 8 |
| 1 | naive per-rule walks | 5.85 us | 3.07-15.2 us | 56 | 2 |
| 3 | direct shared pass | 7.07 us | 6.09-10.1 us | 504 | 7 |
| 3 | inspector union query | 56.5 us | 43.6-214 us | 28,768 | 11 |
| 3 | naive per-rule walks | 43.1 us | 16.9-53.8 us | 152 | 4 |
| 5 | direct shared pass | 34.0 us | 28.2-46.7 us | 600 | 9 |
| 5 | inspector union query | 135 us | 61.0-225 us | 28,864 | 13 |
| 5 | naive per-rule walks | 50.3 us | 35.4-138 us | 248 | 6 |
| 10 | direct shared pass | 16.4 us | 3.89-40.2 us | 936 | 13 |
| 10 | inspector union query | 39.5 us | 27.1-55.6 us | 29,200 | 17 |
| 10 | naive per-rule walks | 85.1 us | 30.0-149 us | 488 | 11 |
| 25 | direct shared pass | 14.6 us | 7.45-23.4 us | 1,896 | 17 |
| 25 | inspector union query | 28.7 us | 10.4-114 us | 30,160 | 21 |
| 25 | naive per-rule walks | 381 us | 154-619 us | 1,208 | 26 |

Raw timing samples, in nanoseconds per operation, are retained here because the
host contention makes the dispersion material:

```text
1/direct       5178 6086 7658 4108 20728 12630 17331
1/inspector   43591 101282 131155 138217 53817 63547 52026
1/naive       14594 15160 13318 5290 3068 5845 3194
3/direct       6900 6093 7849 9770 6717 7068 10137
3/inspector  214123 47212 56516 45020 43634 86263 76669
3/naive       50891 39112 16859 53759 43094 25613 44932
5/direct      38416 28217 43327 46680 31783 29892 33950
5/inspector  224792 200937 224688 134764 96725 70249 60977
5/naive       50311 50163 73389 138358 35412 113728 50258
10/direct     39336 40196 7760 3893 15470 16380 17271
10/inspector  27067 28155 55611 49113 55431 39515 36548
10/naive      85066 149455 59410 30049 62228 102941 93676
25/direct     10721 7454 23375 18923 16868 11085 14596
25/inspector  12070 28681 18540 36794 10443 71526 113965
25/naive     540681 579542 618585 314789 204202 381080 154482
```

The direct shared pass is the selected Phase 3 scheduler. A single naive walk
had a 1.81-microsecond lower median at one rule, but direct dispatch had lower
medians from three through 25 rules and keeps one scheduling path as the
catalog grows. Glippy
performs one union dispatch, so an inspector index has no repeated query over
which to amortize its roughly 28-30 KiB construction cost. The high timing
variance prevents a CI latency threshold; the allocation result and scaling
direction are the architectural evidence.

## Formatter Prototype Scaling Probe

The 2026-08-10 Phase 1 probe formats functions containing 100 and 1,000
identical classic `for` statements. Source loading occurs before the timer;
each measured operation includes lowering, rendering, parse and equivalence
validation, the idempotency render, and isolated syntax-view reparsing. Three
ten-iteration samples ran with `GOMAXPROCS=1` to reduce scheduler noise:

| Statements | Median time | Observed range | Bytes/op | Allocs/op |
| ---: | ---: | ---: | ---: | ---: |
| 100 | 14.7 ms | 11.7-47.4 ms | 3,688,842-3,688,934 | 16,496-16,498 |
| 1,000 | 200 ms | 129-348 ms | 42,795,524-42,795,536 | 166,685 |

A tenfold syntax increase produced 10.1 times the allocations, 11.6 times the
allocated bytes, and 13.6 times the median elapsed time. Timing variance remains
too high for a latency threshold, but the allocation scaling and bounded
renderer evidence do not show explosive growth. This probe is not an editor
latency claim because initial source loading is outside the measured operation;
end-to-end budgets remain Phase 2 work.

## Renderer Complexity Probe

The 2026-08-10 Phase 1 rerun on the same host increased nesting from 20,000 to
100,000 groups and breadth from 4,000 to 16,000 sibling groups, all with a fit
budget of 32. Five three-iteration samples produced these medians:

| Shape | Median time | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| 20,000 nested groups | 176 us | 10 | 1 |
| 100,000 nested groups | 1.38 ms | 10-85 | 1 |
| 1,000 sibling groups | 142 us | 73,530 | 19 |
| 2,000 sibling groups | 277 us | 198,458 | 22 |
| 4,000 sibling groups | 3.43 ms | 435,002-435,152 | 26-27 |
| 8,000 sibling groups | 2.24 ms | 1,136,698-1,136,773 | 32 |
| 16,000 sibling groups | 3.03 ms | 2,590,778-2,592,562 | 37-39 |

The machine was not isolated and individual samples still had scheduler
outliers, so elapsed values are evidence against explosive growth rather than
latency budgets. A durable allocation guard renders 100,000 nested groups in at
most two allocations and 20,000 sibling groups in at most 64 allocations. A
30-second deterministic-render fuzz run completed 385,961 executions within a
45-second process timeout. Together with the iterative renderer and fixed
per-group fit budget, this closes the Phase 1 bounded-execution proof; release
scale still needs peak-memory and stable-runner budgets.

## Editor Latency Probe

The Phase 2 editor probe formats the 879-byte owned hostile workload through
the complete standard-input CLI path. `BenchmarkGlippyEditorStdin` includes
configuration resolution, input reading, source loading, formatting,
validation, and a discarded stdout write while excluding process startup.
`editor-latency.sh` builds the current binary and an owned Go timing driver with
disposable build and module caches. The driver measures 20 fresh processes with
Go's monotonic clock after five warmups, records every sample, and enforces the
maximum directly. The script can be invoked outside the repository because its
build runs from the resolved repository root. It has no third-party timing-tool
dependency and removes its temporary binaries and caches on every exit path.

Recorded 2026-08-11 on the same non-isolated Apple M4 Max, macOS 27.0
(26A5388g), Go 1.26.5, `darwin/arm64`:

| Boundary | Samples | Time or mean | Observed range | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| In-process CLI, first campaign | 5 x 10 operations | 1.040 ms median | 0.873-1.254 ms | about 1,052,000 | 5,542-5,544 |
| In-process CLI, contention rerun | 5 x 10 operations | 7.086 ms median | 1.500-12.690 ms | 1,051,781-1,052,764 | 5,542-5,545 |
| Fresh process, campaign 1 | 20 | 41.1 ms mean | 6.4-104.8 ms | n/a | n/a |
| Fresh process, campaign 2 | 20 | 4.6 ms mean | 3.9-7.7 ms | n/a | n/a |
| Fresh process, campaign 3 | 20 | 15.9 ms mean | 6.4-39.1 ms | n/a | n/a |
| Fresh process, campaign 4 | 20 | 11.5 ms mean | 5.1-32.0 ms | n/a | n/a |
| Fresh process, outside-repository campaign | 20 | 49.8 ms mean | 5.4-151.2 ms | n/a | n/a |

The provisional local adoption budget is 250 ms for every measured fresh
process on this reference workload after the binary has been built. All 100
recorded invocations satisfy that budget. This budget is an editor-usability
gate for the reference host, not a stable CI threshold or a cross-platform
performance claim. The 38.8-fold spread between the fastest and slowest fresh
processes, and the contention-sensitive in-process timings, require an isolated
runner and broader file-size corpus before a regression threshold can be set.

## Peak Resident Memory Probe

`peak-rss.sh` builds one current-tree binary and keeps the build plus measured
package loads on one task-owned warm Go build cache. It measures peak resident
memory through the platform `/usr/bin/time` and runs five samples by default
for two repository-scale, non-writing workloads:

- formatter-only check over the default Glippy repository selection; and
- recursive combined format and opt-in `suspicious` lint check, which selects
  the current types, CFG, and SSA tiers over all Glippy packages.

Exit 1 is an expected completed measurement because either workload may find
formatting differences or diagnostics. Every other nonzero status fails the
probe. Results are emitted as CSV with peak RSS normalized to bytes on Darwin
and Linux. `GLIPPY_PEAK_RSS_RUNS` may select a different positive sample count.
The script removes its binary, configuration, measurement output, and build
cache on every exit path.

`GLIPPY_PEAK_RSS_FORMAT_ROOT` may replace only the formatter workload with another
directory. The typed workload deliberately remains the owned Glippy packages so
an unrelated module or workspace cannot silently change the selected analysis
contract. Record the external root's immutable revision, selected-file count,
provenance, and environment beside any result.

Recorded 2026-08-12 at source revision `76035e6` on an Apple M4 Max with
128 GiB RAM, macOS 27.0 (26A5388g), and Go 1.26.5, `darwin/arm64`:

| Workload | Samples | Median peak RSS | Observed range |
| --- | ---: | ---: | ---: |
| Formatter check | 5 | 342,622,208 bytes | 306,724,864-388,268,032 bytes |
| Typed combined check | 5 | 407,846,912 bytes | 357,351,424-417,349,632 bytes |

Raw peak-RSS samples, in bytes:

```text
formatter 306724864 342622208 362545152 326582272 388268032
typed     387399680 413581312 407846912 357351424 417349632
```

### Large Formatter Snapshot

An additional 2026-08-12 formatter campaign used a temporary Git archive of
`go-libraries` revision `1be04c0e6f17f587dc6083b701467620b95d511d` as
`GLIPPY_PEAK_RSS_FORMAT_ROOT`. The snapshot contained only committed bytes from
that object and was deleted after the run. The Glippy binary used source revision
`2a21699`. Schema-1 check reporting confirmed 5,138 selected files, 4,904
formatting differences, a complete result, and findings exit 1 without
mutation.

Five samples on the same Darwin arm64 host measured a 1,760,575,488-byte median
peak RSS and a 1,659,682,816-2,057,584,640-byte range:

```text
1678180352 1659682816 1986904064 2057584640 1760575488
```

This is release-scale formatter evidence for one immutable 5,138-file snapshot
on one non-isolated host. It does not establish a memory budget, cross-platform
behavior, or typed large-workspace memory. The typed rows emitted by that
campaign still exercised Glippy itself and are not evidence about `go-libraries`.

### Bounded Arena Capacity Follow-up

A 2026-08-12 follow-up on the same host measured document construction before
and after a physical-token-based capacity hint capped at 8,192 nodes. Both
states used Go 1.26.5, `GOMAXPROCS=1`, matching fixed iteration counts within
each comparison, and task-owned disposable build caches. The focused
allocated-byte results were:

| Workload | Before | After | Change |
| --- | ---: | ---: | ---: |
| Editor stdin, 879 bytes | 1,091,837 B/op | 801,202 B/op | -26.6% |
| 100 dense classic loops | 3,883,051 B/op | 2,769,086 B/op | -28.7% |
| 1,000 dense classic loops | 45,396,918 B/op | 41,661,580 B/op | -8.2% |

The hint changes allocation only; the arena remains growable. Direct lowering
of the editor and 1,000-loop inputs measured 2.88 and 2.40 document nodes per
physical token respectively, supporting a three-node hint. A focused allocation
guard proves that a sufficient hint does not repeatedly grow node storage.

Three non-isolated formatter checks over the same immutable 5,138-file snapshot
measured peak RSS of 1,682,063,360, 1,742,274,560, and 1,883,815,936 bytes, with
a 1,742,274,560-byte median. This is inside the earlier campaign's observed
range and does not prove an RSS improvement or establish a stable budget.
A 131,072-node ceiling instead produced a 2,252,865,536-byte median, and
retaining the first render's arena through repeat-format validation produced a
2,380,267,520-byte median; both designs were rejected. Latency remained too
variable for a new claim.

The default Glippy repository is an owned repeatable workload, but it is not a
release-scale proxy for a large module or workspace. Measurements therefore
describe one host and revision only. The platform `time` result also does not
sample the aggregate simultaneous RSS of Glippy and every package-loading
subprocess. Do not establish a CI or product-wide memory threshold until
representative large repositories run on stable supported hosts and the
observed variance supports a justified budget. The following campaign adds a
provisional maximum for the selected formatter workload; it does not establish
a product-wide typed-analysis or cross-architecture threshold.

## Provisional Release-Scale Formatter Budgets

The current release-scale campaign uses the immutable public `golib`
revision `f28f85133ac6d13169745807fc39e2d5ef6bf780`: 5,314 Go files totaling
41,763,075 source bytes, of which formatter discovery selects 5,138. The
formatter runs in non-writing check mode and every completed sample must remain
within both provisional budgets:

- at most 90 elapsed seconds; and
- at most 2,147,483,648 bytes peak resident memory.

These are per-sample maximums, not median targets. `peak-rss.sh` enforces both
budgets when it is run against this corpus. `editor-latency.sh` separately
enforces a 250 ms maximum for each fresh-process invocation on the owned
879-byte editor workload. Both scripts allow an explicit threshold override so
release automation can pin the published values rather than silently accepting
a source-tree default change.

The worker-selection study and arm64 samples below used the preceding local
revision `c60393a86b17b070b699805d1b8df99b87a7bfa6`. They justify the provisional
thresholds but are not evidence for the newly pinned public corpus. The native
runner campaign must reproduce the limits against `f28f851` before the budgets
become stable.

Before choosing the repository-scale limits, a three-sample worker study
compared 4, 8, and 16 formatter workers on Darwin arm64. Eight workers reduced
median elapsed time from 9.61 to 5.63 seconds versus four workers. Sixteen
workers reached 4.65 seconds but raised median peak RSS from 1,512,259,584 to
1,857,765,376 bytes and produced a 2,520,662,016-byte outlier. A Linux arm64
Docker runtime showed no material median latency gain from 16 workers and a
higher memory envelope. The automatic formatter ceiling is therefore eight
workers.

The native measurements used the Apple M4 Max host described above. The Linux
runtime used Docker Desktop 29.6.2, Linux 6.12.76-linuxkit arm64, and the
`golang:1.26.5-bookworm` image. Raw worker-study samples are elapsed seconds
followed by peak bytes:

```text
darwin-4  9.61/1256767488 8.95/1264959488 9.61/1527578624
darwin-8  6.11/1512259584 5.43/1486356480 5.63/1732608000
darwin-16 4.45/1857765376 4.65/1659027456 5.99/2520662016
linux-8   9.44/1340276736 9.53/1315958784 10.53/1430319104
linux-16  9.40/1627377664 9.88/1520181248 9.51/1483059200
```

Five final samples with that ceiling produced:

| Runtime | Median elapsed | Maximum elapsed | Median peak memory | Maximum peak memory |
| --- | ---: | ---: | ---: | ---: |
| Darwin arm64, native | 7.16 s | 8.38 s | 1,479,950,336 B | 1,694,957,568 B |
| Linux arm64, Docker | 10.19 s | 10.86 s | 1,391,017,984 B | 1,588,207,616 B |

```text
darwin 8.38/1694957568 7.16/1408253952 6.53/1479950336 7.66/1643085824 6.32/1452720128
linux  10.86/1296248832 10.43/1477431296 9.83/1588207616 10.19/1391017984 9.44/1180524544
```

Darwin memory is the Glippy process maximum from `/usr/bin/time -l`. Linux memory
is the cgroup-v2 `memory.peak` value and therefore also includes the minimal
container runtime processes. The Linux figure is conservative for Glippy but is
not directly interchangeable with Darwin process RSS.

The budget is executable and has headroom over every recorded local supported-OS
arm64 sample. The native runner campaign below establishes the stable release
budget across the supported operating-system and architecture matrix. A
threshold failure blocks the corresponding release candidate; changing a
budget requires a new recorded campaign and rationale.

### Native Runner Campaign

The manually dispatched `Release budget evidence` GitHub Actions workflow pins
the corpus revision, Go 1.26.5, the 250-millisecond editor maximum, and the
90-second/2-GiB formatter maximum. It runs on native GitHub-hosted
`macos-15-intel`, `macos-15`, `ubuntu-24.04`, and `ubuntu-24.04-arm` runners,
covering Darwin and Linux on amd64 and arm64. Action dependencies are pinned by
complete commit ID, cache reuse is disabled, and each job retains the raw host
metadata and samples as a short-lived artifact.

The workflow supplies the campaign's pinned budgets. `peak-rss.sh` enforces
those supplied thresholds and rejects a run when the requested Go host, kernel
operating system or architecture, clean Glippy revision, or clean corpus revision
does not match. This prevents architecture emulation, uncommitted source, or a
moving corpus from being recorded as native evidence. The workflow is manual
because its purpose is release evidence, not a noisy per-commit wall-clock
assertion. Its existence is not evidence that the four jobs passed; the
recorded run URL, job conclusions, runner image versions, and raw artifacts
remain required before the provisional limits become stable release budgets.

The v0.5 workflow also checks out `sqlc-dev/sqlc` at
`8a7cddfbb9088666eb981645285d7699e71dcb54` and runs the default correctness
policy plus `-Wsuspicious` within a 40-second and 2-GiB ceiling on every native
runner. Darwin arm64 additionally binds normalized diagnostic SHA-256
`13f9c3dd006a105196c13f766a1c849a882754b3083979e9386c78cc2fdb53d2` for the
current 118-rule catalog. The other native targets intentionally retain their
raw result because build-selected findings may differ. The digest update and
current-revision arm64 rehearsal are recorded in
[`../docs/research/v0.5-current-revision-arm64-budget-evidence-2026-08-21.md`](../docs/research/v0.5-current-revision-arm64-budget-evidence-2026-08-21.md).
The exact release candidate must still pass and retain the complete four-runner
campaign.

The first native run, [`31611144933`](https://github.com/faustbrian/glippy/actions/runs/31611144933),
rejected the original 15-second maximum. Its first repository-scale samples
completed in 17.370 seconds on Linux arm64, 20.470 seconds on Linux amd64,
30.130 seconds on Darwin arm64, and 67.690 seconds on Darwin amd64; every sample
remained below the 2-GiB memory ceiling. The replacement 90-second provisional
maximum gave 33% headroom over the slowest observation.

The complete rerun, [`31611653501`](https://github.com/faustbrian/glippy/actions/runs/31611653501),
passed at Glippy revision `345a8de5c8dfd7980863a075123940919e7c4e63`.
Every native runner completed 20 editor samples, five formatter samples, the
typed side-workload, and artifact retention:

| Runner | Editor maximum | Formatter elapsed range | Formatter peak RSS range |
| --- | ---: | ---: | ---: |
| Darwin amd64 | 19.571 ms | 25.070-42.020 s | 1,410,637,824-1,713,582,080 B |
| Darwin arm64 | 15.557 ms | 25.830-28.040 s | 1,327,251,456-1,486,569,472 B |
| Linux amd64 | 3.257 ms | 21.520-21.720 s | 1,152,860,160-1,398,861,824 B |
| Linux arm64 | 3.538 ms | 16.700-16.840 s | 1,231,724,544-1,305,530,368 B |

This establishes the stable 250-millisecond editor, 90-second formatter, and
2-GiB formatter peak-RSS budgets for native macOS and Linux on amd64 and arm64.

The 2026-08-21 local rehearsal at `835a296` reran the current binary sources on
native Darwin arm64 and Docker-hosted Linux arm64 with Go 1.26.5. All 20 editor
samples and all five formatter and typed samples per operating system passed.
Darwin maxima were 8.341 ms editor latency, 5.520 seconds and 1,478,885,376
bytes for formatting, and 22.550 seconds and 1,360,035,840 bytes for typed
linting. Linux maxima were 3.772 ms, 11.230 seconds and 1,317,863,424 bytes,
and 23.390 seconds and 1,256,058,880 bytes respectively. Docker-hosted Linux is
not a substitute for the exact-candidate native workflow.
