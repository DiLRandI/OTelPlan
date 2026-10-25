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
- Linux builds;
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

## 6. Authoritative quality gates

```sh
make test
make test-race
make vet
make lint GOLANGCI_LINT=/path/to/golangci-lint
make test-gates
```

`make check` runs all of these. The required linter version is v2.14.0, verified
with the export data emitted by Go 1.27.3. Tool absence, version mismatch,
and real lint failures return nonzero. No broad suppression is an acceptable
substitute for fixing a finding.

GitHub Actions runs unit, race, vet, lint, pinned OTelC integration/E2E,
dependency-vulnerability scans, and quality-gate tests as separate Linux jobs.
The aggregate check requires every
job to pass. Linux is the current support and release-verification target.

The network-dependent vulnerability gate is explicit:

```sh
GOBIN="$PWD/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
make vuln
```

`GOVULNCHECK=/path/to/govulncheck` overrides the executable. The gate requires
the pinned scanner version, scans source including tests, then checks all modules
used by the CLI without filtering by reachability. Text mode returns nonzero for
findings; JSON mode is not used as a pass/fail gate. Missing tools, scanner errors,
and version mismatches also fail. CI requires this separate gate; `make check`
keeps its existing local checks without adding a vulnerability-database lookup.
Results depend on the current Go vulnerability database and supplement, rather
than replace, source review and other security verification.

## 7. Real pinned backend

Build the exact backend using [these instructions](OTELC_BACKEND.md#build-the-pinned-backend).
From OTelPlan:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc go test -timeout=20m ./...
OTELPLAN_OTELC=/absolute/path/to/otelc go test -race -timeout=20m ./...
```

Both runs use an explicit per-package timeout because cold-cache instrumented
builds can exceed Go's default 10-minute package budget on CI runners. CI divides
the workload into complementary shards, each running normal and race tests:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh a-m
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh remaining
```

The first selects top-level tests matching `^Test[A-M]`; the second skips exactly
that pattern and retains every other test, example, and fuzz seed. Together they
retain the full suite, including future test names. Each shard keeps the
20-minute package timeout and 45-minute job limit. Matrix failures do not cancel
the other shard, and the aggregate check requires both to pass. Quality-gate
tests verify selection arguments, backend requirements, and failure propagation
for both normal and race commands.

Without `OTELPLAN_OTELC`, backend integration tests skip locally. A configured
missing or incompatible executable fails. Do not treat skipped tests as trace
E2E evidence.

`TestArchitectureTracesWithPinnedBackend` tests package, feature, and hexagonal
layouts in independent `cold-cache-online` and `prepared-cache-offline` scenarios.
Each starts with a fresh test-owned module cache. Offline first proves discovery
fails without dependencies, prepares pinned dependencies in disposable state,
then retains `--offline` and rejects/counts HTTP(S) proxy requests throughout the
workflow. This covers dependency resolution without depending on a developer's
cache. It is not an operating-system network sandbox. See the
[fixture instructions](../internal/cli/testdata/architectures/README.md).

## 8. Documented upstream failures

Generic receiver shapes affected by the pinned backend's missing type-argument
inference retain expected failed-build tests with source/output safety assertions.
Generic context replacement, unsupported variadic types, and main-package targets
have validation regressions. Upstream issue closure does not imply that a fix is
present in the pinned executable; see [backend limits](OTELC_BACKEND.md).

## 9. Pinned open-source starter regression

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc go test ./internal/cli \
  -run '^TestMemosStarterAcceptance$' -count=1 -timeout=20m
```

The expensive integration profile analyzes the complete Memos v0.26.0 Go module,
pinned to source commit `1696c6c41428f98c06fcaf8838d43bcbf784c413` and verified
module checksums. It copies the source into disposable state, prepares dependencies
in a private cache, then denies dependency network access before running OTelPlan.
It runs no application server, database, or Memos code.

The regression checks explainable API-operation suggestions, repeated identical
previews, exact-symbol policy generation, no default capture, validation and
inspection, deterministic resolution locks, read-only checks, and unchanged source
and module files. Generated protobuf/Connect files and embedded frontend assets
remain part of the input; the test does not replace or simplify application code.

Before Connect entrypoint recognition, the same source produced 19 database-driver
suggestions and one API operation. With typed handler evidence, all 20 suggestions
identify API operations with known static paths from their Connect handlers.
Scoring weights are unchanged. This is one application regression, not proof of
usefulness across all architectures or a performance benchmark. Broader OSS
coverage remains part of the initialization acceptance gate.
