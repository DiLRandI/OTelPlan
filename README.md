# OTelPlan

**Policy-driven, zero-source-change business tracing for Go.**

OTelPlan is a proposed Go developer tool that analyzes an existing Go codebase, helps a developer identify meaningful business operations, stores that decision in a source-controlled policy, validates the policy, and compiles it to OpenTelemetry compile-time instrumentation.

OTelPlan does **not** replace OpenTelemetry or `otelc`. It sits above them.

```text
Existing Go code
      |
      v
  otelplan scan
      |
      v
Code model + suggestions
      |
      v
 otelplan.yaml       <-- human-owned source of truth
      |
      v
validate / inspect / diff
      |
      v
 otelplan compile
      |
      v
pinned otelc rules + hooks
      |
      v
instrumented Go binary
      |
      v
OTLP -> Collector -> New Relic / Grafana / Honeycomb / etc.
```

## Core value

Existing auto-instrumentation is good at technical boundaries such as HTTP, gRPC, SQL, Redis, Kafka, and AWS SDK calls.

OTelPlan targets the missing layer:

```text
Checkout
├── Cart.Calculate
├── Pricing.ApplyDiscounts
├── Payment.Authorize
└── Order.Persist
```

The user should be able to get these business spans **without adding `tracer.Start()` calls to the application source**.

## Non-negotiable design principles

1. **Architecture agnostic.** Never depend on names such as `Service`, `Repository`, `UseCase`, or a particular folder layout.
2. **Discovery is advisory; policy is authoritative.**
3. **No source modification required.**
4. **OpenTelemetry-native and vendor-neutral.**
5. **Safe by default.** Do not capture arguments, return values, secrets, or high-cardinality data unless explicitly allowed.
6. **Deterministic and reviewable.** The resolved instrumentation set is lockable and diffable.
7. **Backend isolation.** `otelc` is a backend implementation, not OTelPlan's public configuration format.
8. **Fail loudly rather than silently produce broken trace propagation.**

## Repository starter

This bundle is intentionally specification-first. It contains the contracts Codex should implement.

Start with:

```bash
git init
go mod tidy
```

Then give Codex `docs/CODEX_IMPLEMENTATION_PROMPT.md`.

## Documents

- `docs/01_PRD.md` — product and functional requirements
- `docs/02_ARCHITECTURE.md` — system architecture
- `docs/03_DISCOVERY_ENGINE.md` — Go code analysis model
- `docs/04_POLICY_SPEC.md` — `otelplan.yaml` contract
- `docs/05_CLI_SPEC.md` — CLI commands and exit codes
- `docs/06_COMPILER_BACKEND.md` — backend and `otelc` integration
- `docs/07_RUNTIME_HOOKS.md` — span lifecycle and context behavior
- `docs/08_VALIDATION_SAFETY.md` — privacy, cardinality, safety
- `docs/09_LOCKFILE_REPRODUCIBILITY.md` — deterministic builds
- `docs/10_TEST_STRATEGY.md` — tests and fixtures
- `docs/11_PERFORMANCE.md` — performance requirements
- `docs/12_COMPETITIVE_GAP.md` — why the project should exist
- `docs/13_ROADMAP.md` — implementation phases
- `docs/14_ACCEPTANCE_CRITERIA.md` — release gates
- `docs/15_DECISIONS.md` — architectural decisions
- `docs/CODEX_IMPLEMENTATION_PROMPT.md` — implementation instruction
- `schemas/otelplan.schema.json` — initial JSON Schema
- `examples/otelplan.yaml` — full example
- `examples/otelplan.minimal.yaml` — minimal example
- `SOURCES.md` — external references used in the specification

## Toolchain baseline

The specification targets **Go 1.27.x** and should be implemented and tested with the latest patch version available in that line. At the time this specification was prepared, Go 1.27.1 was the latest stable patch release.

The `otelc` version must be explicitly pinned by the project. Do not use `@latest` in reproducible CI builds.
