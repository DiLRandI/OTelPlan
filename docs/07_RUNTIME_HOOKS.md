# Runtime Hook Semantics

## 1. Objective

Instrument a selected function/method as one OpenTelemetry span while preserving application behavior.

## 2. Span lifecycle

For every invocation:

```text
enter
  -> derive parent context
  -> start span
  -> optionally replace context argument
  -> set safe entry attributes
  -> execute original target
  -> inspect result/error
  -> set safe exit attributes
  -> record error/status
  -> end span
return
```

Span end must occur exactly once.

## 3. Tracer identity

Instrumentation scope name:

```text
otelplan.io/business
```

Version should be the OTelPlan runtime version.

Future policy may support custom scope names only if a concrete use case requires it.

## 4. Span kind

Business spans default to `SpanKindInternal`.

Do not guess client/server/producer/consumer based on naming.

Protocol-specific span kinds belong to proper semantic-convention instrumentations.

## 5. Parent context

Preferred source: explicit `context.Context` parameter.

If multiple context parameters exist:

- default is validation error;
- policy may select an index/path in a future schema.

Nil contexts must be handled without panic. Follow OpenTelemetry API expectations and a defined fallback policy.

## 6. No context

Default diagnostic:

```text
OTP3001 missing-context
```

Default outcome: target is invalid in `require` mode.

Explicit root mode may start a root span, but the user must understand it will not join the expected parent trace.

## 7. Error semantics

For each configured error result index:

- non-nil error -> `RecordError`;
- set error status with a bounded description strategy;
- do not emit unbounded error strings as attributes;
- do not alter returned error.

## 8. Panic semantics

OTelPlan never recovers a panic merely to continue execution.

If the backend allows observing panic/defer-equivalent exit safely:

- record exception;
- end span;
- allow panic to continue.

Otherwise document the limitation and validate accordingly.

## 9. Attribute extraction

Entry attributes may use parameters.

Exit attributes may use results.

Extraction failures must never change application behavior.

A generated accessor must be statically type-checked; avoid runtime reflection where possible.

## 10. Performance

Runtime hook hot path should:

- avoid reflection;
- avoid JSON marshaling;
- avoid heap allocations where practical;
- skip expensive attribute work when span is not recording where possible.

## 11. Semantic conventions

OTelPlan business spans should not invent protocol semantic convention attributes.

User-defined business attributes are permitted under user-controlled names.

Reserved `otel.*` keys are rejected.
