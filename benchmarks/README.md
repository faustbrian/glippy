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
go test ./internal/format/doc -run '^$' -bench '^BenchmarkRenderAdversarial(Nesting|Siblings)$' -benchmem -benchtime=3x -count=3
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

## Renderer Complexity Probe

The same host rendered 20,000 nested groups with a fit budget of 32 in a median
166 us and one allocation after cached flat summaries were added. Wide
sibling-group medians were 134 us for 1,000, 272 us for 2,000, and 760 us for
4,000; allocations were 73,530, 198,458, and 435,002-435,040 bytes
respectively. Individual samples had substantial scheduling outliers, including
a 22.1 ms nested-group sample, so these values are not latency budgets. The
scaling and source inspection support bounded per-group lookahead without
continuation stack copies; Phase 1 still requires larger adversarial benchmarks
and fuzz timeouts before the complexity risk can close.
