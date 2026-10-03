# Documentation

OTelPlan is implemented and pre-stable. The CLI and backend references describe
current behavior. Product and component specifications retain the intended
architecture and release requirements; the roadmap separates implemented work
from remaining acceptance gates.

| Area | Reference |
| --- | --- |
| Product | [Vision](00_PRODUCT_VISION.md), [requirements](01_PRD.md), [competitive gap](12_COMPETITIVE_GAP.md) |
| Architecture | [Components and data flow](02_ARCHITECTURE.md) |
| Policy | [Policy reference](04_POLICY_SPEC.md), [JSON Schema](../schemas/otelplan.schema.json), [example](../examples/otelplan.yaml) |
| CLI | [Commands, flags, JSON, diagnostics, and exit codes](CLI.md) |
| Discovery | [Semantic analysis and effective build inputs](03_DISCOVERY_ENGINE.md) |
| Backend | [Compiler architecture](06_COMPILER_BACKEND.md), [pinned OTelC implementation and support matrix](OTELC_BACKEND.md) |
| Runtime | [Span lifecycle and context semantics](07_RUNTIME_HOOKS.md) |
| Safety | [Privacy, capture, and cardinality](08_VALIDATION_SAFETY.md) |
| Reproducibility | [Lockfile and artifact ownership](09_LOCKFILE_REPRODUCIBILITY.md) |
| Testing | [Local checks, CI, and integration tests](10_TEST_STRATEGY.md) |
| Performance | [Requirements and benchmark commands](11_PERFORMANCE.md) |
| Roadmap | [Implementation status and remaining work](13_ROADMAP.md) |
| Acceptance criteria | [Release-blocking requirements](14_ACCEPTANCE_CRITERIA.md) |
| Architectural decisions | [Decision log](15_DECISIONS.md) |
| External references | [Upstream sources](REFERENCES.md) |
| Contributor implementation instructions | [Implementation constraints](CODEX_IMPLEMENTATION_PROMPT.md), [agent instructions](../AGENTS.md) |

The [architecture fixture README](../internal/cli/testdata/architectures/README.md)
stays beside its test inputs because it explains their independent online/offline
execution scenarios. User-facing documentation belongs here. Package API details
belong in GoDoc; avoid creating additional package READMEs for short descriptions.
