# Performance Requirements

## 1. CLI analysis

Targets for a medium repository after warm Go caches:

- 100 packages: scan should feel interactive;
- repeated unchanged scan should reuse cache;
- JSON output should stream or avoid unnecessary duplication for large projects.

Do not set hard millisecond SLOs until benchmarks exist.

## 2. Memory

Avoid holding duplicate AST/type graphs.

Store normalized metadata separately from compiler structures only when needed.

Release SSA/call-graph structures after suggestion analysis.

## 3. Runtime instrumentation overhead

OTelPlan must benchmark generated business spans separately from `otelc` itself.

Benchmarks:

- target with non-recording span;
- sampled/recording span;
- 0 attributes;
- 2 primitive attributes;
- error path.

Report:

```text
ns/op
B/op
allocs/op
```

Do not advertise "zero overhead."

## 4. Plan size

Compiler should handle at least:

- 2,000 exact targets;
- hundreds of policy rules;
- multiple modules in a workspace.

Very broad policies must trigger usability/cardinality warnings even if technically supported.

## 5. Benchmark regression gate

After a stable baseline exists, CI should fail on statistically meaningful regressions beyond an agreed threshold rather than an arbitrary initial threshold.

## Current benchmarks

```sh
go test ./internal/discovery -run '^$' -bench 'BenchmarkLoadGeneratedSymbols|BenchmarkAnalysisCache' -benchmem
go test ./internal/resolve -run '^$' -bench . -benchmem
```

`BenchmarkAnalysisCache` compares equivalent uncached and cached engine metadata
for 100 functions importing context. Record the revision, Go version, platform,
CPU, commands, and samples with any published result. The CLI opts into
this cache only with `--cache-dir`; these engine benchmark timings are not a
representative CLI performance report. Generated-hook runtime benchmarks and
a representative performance report remain pending.

## Large-plan regression workload

```sh
go test ./internal/compiler -run '^TestLargeWorkspacePlan$' -count=1
OTELPLAN_OTELC=/absolute/path/to/otelc go test -race ./internal/compiler \
  -run '^TestLargeWorkspacePlanWithBackend$' -count=1 -timeout=20m
```

The fixture has two workspace modules, 100 operation packages, 2,000 exact
targets, and 200 policy rules. It verifies semantic discovery, exact resolution,
retained span-volume warnings, deterministic bundle rendering, source/module
immutability, and real pinned-backend execution. All 2,000 business spans must
match the resolved plan and their parent trace. Another 100 similarly named
functions execute without instrumentation.

The original single hook package made the pin's hook lookup repeatedly parse
all 4,000 lifecycle functions. Its real-backend test reached the 20-minute test
timeout. Bounded hook packages keep each parse to at most 128 lifecycle functions.
One verified fixed run completed in 74.86 seconds on Go 1.27.2, Linux/amd64,
Intel Core i7-11800H, with `otelc v1.1.0` at source commit
`449ee08a682586adb177e4402845ed404565879f`. This elapsed time includes fixture
setup, analysis, compilation, execution, and assertions. It is regression
evidence, not a build-time SLO or a statistically controlled benchmark report.

## Medium-project CLI scan baseline

```sh
GOMAXPROCS=1 taskset -c 15 go test ./internal/cli -run '^$' \
  -bench '^BenchmarkCLIStaticAnalysis$' -benchtime=3x -count=3 -benchmem
```

Measured on 2026-10-31 at clean revision
`e37eccfb4cc6580379d4ba614c1526b029e20dcf`, Go 1.27.3, Linux/amd64,
Intel Core i7-11800H, logical CPU 15, `GOMAXPROCS=1`. No competing Go workloads
ran during measurement. The fixture contains 100 independent packages, 2,000
context-aware functions, and standard-library dependencies. Go caches are warmed;
`CGO_ENABLED=0`, workspace and ambient Go flags are disabled, and scans use
`--offline --format=json`.

Preflight checks every expected symbol, verifies cache creation and byte-identical
uncached/cached/repeated JSON, and hashes all inputs to reject added, removed, or
changed files. The analysis cache is outside the fixture. Timing includes CLI
argument handling, discovery or cache validation/reuse, Go subprocesses, and JSON
rendering to a discard writer. It excludes OS process startup and preflight work.
Each sample averages three scans; the table shows medians of three samples.

| Analysis cache | ms/op | Go B/op | Go allocs/op |
| --- | ---: | ---: | ---: |
| uncached | 477.687 | 194595893 | 1829250 |
| cached | 105.353 | 45030792 | 48303 |

[Raw analysis samples](benchmarks/analysis-go1.27.3-linux-amd64.txt) preserve every
measurement and its command. Allocations describe the Go process running
`CLI.Run`, including rendering; they exclude Go subprocess allocations and are
not peak resident memory. This synthetic fixture measures one medium-project
shape. CGO, external dependencies, workspaces, call-graph suggestions, and actual
application complexity need their own profiles before broader latency claims.
