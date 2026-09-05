# Test Strategy

## 1. Test pyramid

### Unit tests

Cover:

- glob matching;
- symbol canonicalization;
- interface implementation;
- policy parsing;
- precedence;
- template rendering;
- diagnostics;
- lock canonicalization;
- secret/cardinality detection.

### Golden tests

Fixture:

```text
testdata/projects/<case>
testdata/policies/<case>.yaml
testdata/golden/<case>.resolved.json
```

Any behavior change requires intentional golden update.

### Integration tests

Build real fixture applications through the backend.

Verify:

- application source unchanged;
- instrumented binary builds;
- expected spans are emitted;
- parent-child relationships are correct;
- errors are recorded;
- no selected attributes leak forbidden values.

Use an in-memory/test exporter where the generated runtime permits it, or an OTLP test receiver.

### Compatibility tests

Matrix:

- supported Go versions;
- supported `otelc` versions;
- Linux/macOS build where backend supports;
- pointer/value receivers;
- interfaces;
- generics;
- build tags;
- go.work;
- vendor mode.

### End-to-end CLI tests

Exercise:

```text
init
scan
inspect
validate
lock
diff
compile
build
```

## 2. Required fixture architectures

Do not test only one project style.

Fixtures:

1. package-oriented:

```text
internal/service/user.go
internal/service/order.go
```

2. feature-oriented:

```text
internal/user/service.go
internal/order/service.go
```

3. clean/hexagonal:

```text
internal/domain
internal/application
internal/ports
internal/adapters
```

4. concrete structs with no interfaces;

5. interfaces with multiple implementations;

6. functional style with package functions and few receiver methods;

7. generics-heavy package;

8. no-context legacy functions;

9. generated mocks mixed with real code.

## 3. Source immutability test

Before every end-to-end build:

1. hash tracked source files;
2. run OTelPlan;
3. hash again;
4. assert equality.

## 4. Trace correctness tests

Example expected hierarchy:

```text
HTTP server span (existing otelc integration)
└── checkout.Checkout.Submit
    ├── pricing.Calculator.Calculate
    └── payment.Processor.Authorize
```

The test must verify trace ID continuity and parent span IDs.

## 5. Failure tests

- malformed YAML;
- stale symbol;
- ambiguous match;
- duplicate conflicting rule;
- missing context;
- secret attribute;
- unsupported backend capability;
- backend executable missing;
- Go compile failure.

## 6. Quality gates

Before merge:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Add `golangci-lint` with a strong project configuration once implementation starts.
