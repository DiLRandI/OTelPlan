# Competitive Gap

## What already exists

### OpenTelemetry `otelc`

Provides production-ready Go compile-time instrumentation without source changes, rule-driven matching, instrumentation packages, and before/after hooks.

**OTelPlan should use it, not compete with its compiler weaving engine.**

### OpenTelemetry Go eBPF / zero-code instrumentation

Provides runtime zero-code coverage for supported libraries/protocols.

Strong for infrastructure visibility; not a general human-owned business-operation policy system.

### tracegen

Generates OpenTelemetry tracing decorators from public Go interfaces.

It requires an interface-centric code shape and documented constraints such as `context.Context` on every interface method.

### GoWrap

Generic interface decorator generation. Useful, but still centered on explicit interface wrapping/code generation.

### New Relic Go Easy Instrumentation

Analyzes source and generates a diff suggesting New Relic instrumentation changes. The user reviews and applies the source changes.

## Gap OTelPlan is targeting

The combination:

1. arbitrary existing Go architecture;
2. semantic code inventory;
3. architecture-independent selectors;
4. human-editable policy;
5. exact preview/explain/diff;
6. privacy/cardinality validation;
7. source-code unchanged;
8. reproducible lockfile;
9. compile to OpenTelemetry-native backend.

That combination is the product.

## Kill criteria

Do not continue the project if the upstream `otelc` project itself adds all of the following as a cohesive supported UX:

- semantic project scanner;
- interface/type/package/file selection DSL;
- business-operation discovery;
- resolved-target preview/explain;
- privacy/cardinality policy validation;
- deterministic instrumentation lockfile;
- source-level instrumentation diff.

If upstream covers that end-to-end, OTelPlan should instead contribute upstream rather than duplicate it.

## Strategic rule

OTelPlan's moat cannot be "we know how to emit an otelc YAML file."

Its value must be **code intelligence + policy safety + reproducibility + UX**.
