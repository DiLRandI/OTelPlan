# Codex Implementation Prompt

You are implementing the OTelPlan repository from the specifications in this repository.

## Objective

Build a production-quality Go CLI named `otelplan`.

OTelPlan analyzes arbitrary Go codebases, resolves a human-owned instrumentation policy to exact business-operation targets, validates trace correctness and data safety, locks the result for reproducibility, and compiles the plan to a pinned OpenTelemetry `otelc` backend without modifying application source code.

Do not build a toy prototype.

## Source of truth

Read every file in:

```text
docs/
schemas/
examples/
```

before writing implementation code.

If requirements conflict, use this precedence:

1. `docs/14_ACCEPTANCE_CRITERIA.md`
2. `docs/01_PRD.md`
3. component-specific specs
4. `docs/15_DECISIONS.md`
5. README/examples

Document any unresolved conflict before choosing an implementation.

## Critical constraints

1. Do not assume Go architecture from names such as `Service`, `Repository`, `UseCase`, or directory names.
2. Discovery is advisory. `otelplan.yaml` is authoritative.
3. Never modify application `.go` files.
4. Do not silently modify `go.mod`, `go.sum`, or `go.work`.
5. No argument/result capture by default.
6. Keep OTelPlan policy backend-independent.
7. Implement `otelc` through an adapter.
8. Pin backend versions for reproducible workflows.
9. Never silently create disconnected spans when context propagation is required.
10. Every instrumentation decision must be explainable.
11. Do not use reflection for core static analysis when `go/types` can answer the question.
12. Do not use regex parsing of Go source.
13. Keep code self-explanatory; avoid comments unless they explain genuinely non-obvious invariants or compiler/backend behavior.
14. Errors must be wrapped with useful context.
15. Do not add features not described in the specification merely because they seem convenient.

## Go version

Use Go 1.27.x. Start with the latest stable 1.27 patch available in the environment.

## Implementation order

### 1. Domain model

Create stable internal/public models for:

- SymbolID;
- Symbol;
- CodeModel;
- Policy;
- Selector;
- ResolvedPlan;
- ResolvedTarget;
- Diagnostic;
- Lockfile;
- Backend;
- BackendCapabilities.

Avoid circular package dependencies.

### 2. Project analyzer

Use:

- `golang.org/x/tools/go/packages`;
- `go/types`;
- AST only for source metadata;
- SSA later for call-graph-assisted suggestions.

Support:

- functions;
- methods;
- pointer/value receivers;
- interfaces;
- structural interface implementations;
- context parameters;
- error results;
- source locations;
- generated/test metadata.

Write extensive `testdata` fixtures representing different architectures.

### 3. Policy

Implement:

- YAML parsing;
- JSON Schema-compatible data structures;
- package/file/symbol/function/receiver/method selectors;
- interface implementation selectors;
- exclusions;
- templates;
- conflict detection.

Do not implement "last rule wins."

### 4. Resolver and inspect/explain

Resolve broad rules to exact canonical symbols.

Implement:

```bash
otelplan scan
otelplan inspect
otelplan explain <symbol>
```

Every target must carry provenance back to policy rule IDs.

### 5. Validators

Implement stable diagnostic codes.

At minimum:

- invalid selector;
- unresolved symbol;
- conflicting rules;
- missing context;
- multiple contexts;
- suspicious secret/PII attributes;
- cardinality warning;
- too-broad plans;
- unsupported backend capability.

### 6. Lockfile and diff

Canonical deterministic serialization.

Implement:

```bash
otelplan lock
otelplan lock --check
otelplan diff
```

No timestamps in digest material.

### 7. Backend abstraction

Implement backend interface before `otelc`.

Do not allow policy package to import `internal/backend/otelc`.

### 8. OTelC backend

Before implementation, inspect the exact pinned `otelc` version's current rule schema and hook API.

Do not copy assumptions from old docs.

Implement capability mapping and fail when a requested OTelPlan plan cannot be represented safely.

Generate backend files into `.otelplan/build` or an explicit output directory.

### 9. Runtime hooks

Use official OpenTelemetry APIs.

Required semantics:

- start `SpanKindInternal`;
- inherit explicit context;
- replace context arg when backend supports it;
- set only explicit safe attributes;
- record configured returned errors;
- end exactly once;
- do not change function behavior;
- do not swallow panic.

### 10. Build

Implement:

```bash
otelplan compile
otelplan build -- <go build args>
```

Prefer isolated generated/module state.

Add a source immutability integration test.

## CLI quality

Use a mature, small CLI parsing library only if it materially improves maintainability; otherwise standard library is acceptable.

All commands must support:

```text
--format=text
--format=json
```

No prompts in non-interactive/CI commands.

Stable exit codes are specified in `docs/05_CLI_SPEC.md`.

## Testing

Required before declaring a phase complete:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Add strong linting.

Test multiple architecture styles. A single `FooService` fixture is specifically insufficient.

For the backend E2E tests, verify emitted trace relationships, not merely successful compilation.

## Performance

Benchmark runtime hook overhead and static analysis.

Do not state unsupported performance claims.

## Security

Do not transmit source code or project metadata externally.

No CLI analytics/telemetry by default.

Never emit raw secrets in diagnostics.

## Documentation

As implementation evolves:

- keep `examples/otelplan.yaml` executable/valid;
- keep JSON Schema synchronized with Go policy types;
- add command examples;
- maintain a backend compatibility matrix;
- document any upstream `otelc` limitation rather than hiding it.

## Deliverable definition

The repository is not complete until all release-blocking criteria in `docs/14_ACCEPTANCE_CRITERIA.md` are either:

- implemented and tested; or
- explicitly marked as blocked by a verified upstream limitation with a linked issue and a failing/disabled test that captures the limitation.

Do not substitute mocks for final backend E2E proof.
