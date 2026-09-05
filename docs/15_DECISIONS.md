# Architectural Decision Log

## ADR-001 — OTelC is a backend, not the product

**Decision:** Reuse `otelc` for compile-time weaving.

**Reason:** Reimplementing the compiler instrumentation engine has little product value and creates major maintenance risk.

## ADR-002 — Policy is authoritative

**Decision:** Scanning/discovery may suggest but cannot silently determine instrumentation.

**Reason:** Go architectures are diverse and business meaning cannot be reliably inferred from naming conventions.

## ADR-003 — No required naming conventions

**Decision:** `Service`, `Repository`, `UseCase`, folder conventions, etc. are never mandatory.

**Reason:** The product must work with package-oriented, feature-oriented, clean architecture, and custom projects.

## ADR-004 — YAML public policy

**Decision:** Human-owned configuration is YAML with a published JSON Schema.

**Reason:** Readable in code review and friendly to comments; JSON Schema gives editor validation.

## ADR-005 — Exact resolved lockfile

**Decision:** Broad policy compiles to exact canonical targets stored in a lockfile.

**Reason:** New code must not become silently instrumented in production.

## ADR-006 — No capture by default

**Decision:** Parameters/results are never captured automatically.

**Reason:** Privacy, credentials, cardinality, and cost risk.

## ADR-007 — Missing context defaults to failure/skip

**Decision:** Do not fake parent-child tracing.

**Reason:** A disconnected span can create misleading observability.

## ADR-008 — Backend capability negotiation

**Decision:** Features are validated against a pinned backend version.

**Reason:** `otelc` capabilities evolve.

## ADR-009 — No source modification

**Decision:** The primary workflow is clean-room/source-independent.

**Reason:** This is a core product differentiator from patch-generation tools.

## ADR-010 — Static analysis first, AI later

**Decision:** Base product is deterministic.

**Reason:** Users need reproducibility and explainability. AI can later assist discovery only behind explicit opt-in.
