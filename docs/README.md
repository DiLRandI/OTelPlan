# Documentation

OTelPlan is implemented and pre-stable. The CLI and backend references describe
current behavior. The component references describe the architecture and product contracts;
the roadmap separates implemented work from remaining acceptance gates.
The [acceptance criteria](14_ACCEPTANCE_CRITERIA.md) remain release-blocking.

| Area | Reference |
| --- | --- |
| CLI | [Commands, flags, JSON, diagnostics, and exit codes](CLI.md) |
| Policy and safety | [Selectors, capture, privacy, and limits](POLICY.md), [JSON Schema](../schemas/otelplan.schema.json), [example](../examples/otelplan.yaml) |
| Architecture | [Components, discovery, build isolation, and design invariants](ARCHITECTURE.md) |
| Backend | [Pinned OTelC setup, support matrix, and span lifecycle](OTELC_BACKEND.md) |
| Compatibility | [Lockfile ownership, automation, and upgrades](COMPATIBILITY.md) |
| Security | [Private reporting, support status, and operational boundaries](SECURITY.md) |
| Development | [Setup, tests, CI, and pull requests](DEVELOPMENT.md), [agent instructions](../AGENTS.md) |
| Performance | [Requirements and benchmark evidence](PERFORMANCE.md) |
| Releases | [Linux packaging, verification, and candidate review](RELEASING.md), [changelog](CHANGELOG.md) |
| Roadmap | [Implementation status and remaining work](ROADMAP.md), [release-blocking acceptance criteria](14_ACCEPTANCE_CRITERIA.md) |

The [architecture fixture README](../internal/cli/testdata/architectures/README.md)
stays beside its test inputs because it explains their independent online/offline
execution scenarios. User-facing documentation belongs here. Package API details
belong in GoDoc; avoid creating additional package READMEs for short descriptions.
