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
