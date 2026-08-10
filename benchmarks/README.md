# Phase 0 Baselines

These benchmarks measure the cost of the standard Go frontend operations that
Gox expects to compose. They are baselines, not product performance claims or
CI thresholds. The workload is an owned, valid, comment-bearing Go file with
blocks, generics, calls, and boolean expressions.

## Reproduction

```sh
go test ./...
go test ./benchmarks -run '^$' -bench 'Benchmark(Scan|Parse|GoFormat|ASTInspect|InspectorBuildAndFilter|TypeCheck)$' -benchmem -count=5
go test ./benchmarks -run '^$' -bench '^BenchmarkPackagesLoadSyntax(Cold|Warm)BuildCache$' -benchmem -benchtime=1x -count=3
GOMAXPROCS=1 go test ./benchmarks -run '^$' -bench '^BenchmarkGoxFormatManyClassicLoops$' -benchmem -benchtime=10x -count=3
go test ./internal/format/doc -run '^$' -bench '^BenchmarkRenderAdversarial(Nesting|Siblings)$' -benchmem -benchtime=3x -count=5
go test ./internal/format/doc -run '^TestRenderBoundsAdversarialDepthAndBreadthAllocations$' -count=1
go test ./internal/format/doc -run '^$' -fuzz '^FuzzRenderDeterministic$' -fuzztime=30s -timeout=45s
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
