# Phase 0 Baselines

These benchmarks measure the cost of the standard Go frontend operations that
Gox expects to compose. They are baselines, not product performance claims or
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
GOMAXPROCS=1 go test ./benchmarks -run '^$' -bench '^BenchmarkGoxFormatManyClassicLoops$' -benchmem -benchtime=10x -count=3
go test ./internal/format/doc -run '^$' -bench '^BenchmarkRenderAdversarial(Nesting|Siblings)$' -benchmem -benchtime=3x -count=5
go test ./internal/format/doc -run '^TestRenderBoundsAdversarialDepthAndBreadthAllocations$' -count=1
go test ./internal/format/doc -run '^$' -fuzz '^FuzzRenderDeterministic$' -fuzztime=30s -timeout=45s

(
  task_cache=$(mktemp -d "${TMPDIR:-/tmp}/gox-editor-benchmark.XXXXXX")
  trap 'find "$task_cache" -mindepth 1 -delete; rmdir "$task_cache"' EXIT HUP INT TERM
  GOWORK=off GOCACHE="$task_cache" go test ./benchmarks -run '^$' -bench '^BenchmarkGoxEditorStdin$' -benchmem -benchtime=10x -count=5
)
./benchmarks/editor-latency.sh
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
catalog grows. Gox
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
the complete standard-input CLI path. `BenchmarkGoxEditorStdin` includes
configuration resolution, input reading, source loading, formatting,
validation, and a discarded stdout write while excluding process startup.
`editor-latency.sh` builds the current binary with a disposable Go build cache
and uses Hyperfine to measure 20 fresh processes after five warmups. The script
can be invoked outside the repository because its build runs from the resolved
repository root. It requires `hyperfine` and removes its temporary binary and
build cache on every exit path.

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
