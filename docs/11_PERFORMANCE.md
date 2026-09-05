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
