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
CLI measurement. The medium-project CLI profile below measures the scan command
separately. Generated-hook runtime measurements are documented below.

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

## Generated business-span benchmarks

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/benchmark-runtime.sh 3
```

The optional argument is a positive sample count, defaulting to three. The
backend must match the supported `otelc v1.1.0` pin. The harness uses a disposable
copy of the synthetic fixture, prepares pinned dependencies, and builds the CLI,
a normal Go baseline, a verified generated bundle, and an instrumented binary
through `otelplan build`. Preparation may download dependencies. It checks that
fixture source and module files remain unchanged and removes temporary files on
exit. Run it without competing local test/build workloads when collecting data.

Before timing, both binaries check original return behavior. The instrumented
binary also verifies exactly three business spans with the expected scope,
internal kind, parent/trace relationships, explicit attributes, and returned
error event/status. The `--instrumented` fixture flag changes verification and
labels only; the fixture never creates business spans itself.

The six cases combine SDK `NeverSample` and `AlwaysSample` with zero attributes,
two primitive argument attributes, and a preallocated returned error. Provider
setup and the parent span are outside timing. No span processor or exporter is
installed during measurements. Targets use `//go:noinline` so baseline calls
are retained. Each sample reports iterations, `ns/op`, `B/op`, and `allocs/op` in
Go benchmark format. The harness prints revision/worktree state, Go/platform/CPU
identity, and backend version/digest. Compile/build time is excluded.

These are single-threaded synthetic business-operation measurements, not an
exporter benchmark or an application throughput claim. Record raw samples and
the full environment before comparing revisions. No regression threshold is
enforced until a stable baseline and an agreed statistical policy exist.

## Initial runtime baseline

Measured on 2026-10-31 at clean revision
`bc94f3cd84abad178e37a00a0764ed40bea612fe`, based on runtime fix #286.
Go 1.27.3, Linux/amd64, Intel Core i7-11800H. The process was pinned to logical
CPU 15 with `GOMAXPROCS=1`; no other Go test/build workload ran concurrently.
OTelC v1.1.0 was built from `449ee08a682586adb177e4402845ed404565879f`, executable
SHA-256 `39830dc7c191b2368720061515606e3d21a770b1dd05595168ea60529b2728fa`.

```sh
GOMAXPROCS=1 OTELPLAN_OTELC=/absolute/path/to/otelc \
  taskset -c 15 sh scripts/benchmark-runtime.sh 3
```

Medians of three samples per case follow. Bytes and allocations are for the
instrumented call; all baseline cases reported zero bytes and allocations.
Added time is the difference between the corresponding medians.

| Sampling | Operation | Baseline ns/op | Instrumented ns/op | Added ns/op | B/op | Allocs/op |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| nonrecording | zero | 4.910 | 540.5 | 535.6 | 392 | 11 |
| nonrecording | two | 4.966 | 581.2 | 576.2 | 440 | 12 |
| nonrecording | error | 7.920 | 597.0 | 589.1 | 424 | 12 |
| recording | zero | 4.912 | 728.6 | 723.7 | 776 | 11 |
| recording | two | 4.970 | 1050.0 | 1045.0 | 1144 | 16 |
| recording | error | 7.770 | 1253.0 | 1245.2 | 1296 | 17 |

[Raw samples](benchmarks/runtime-go1.27.3-linux-amd64.txt) retain every result and
the measured revision. Provider/parent setup, compilation, and exporter work are
outside timing. The measured calls include operation dispatch and return checks
in both binaries. Added time includes the pinned backend trampoline, generated
lifecycle/context work, and OpenTelemetry SDK work.

Three sequential samples on one developer machine establish an initial workload
baseline. They do not establish application latency or a regression threshold.
CPU frequency, scheduling, and background activity can affect comparisons.
The separate medium-project CLI baseline above measures scan performance.
