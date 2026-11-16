# Agent Instructions

Read `docs/README.md`, the current component references, `schemas/`, and `examples/` before implementing. Use the release-blocking acceptance criteria and owning component contracts when requirements conflict; record unresolved conflicts instead of weakening behavior.

`docs/CLI.md` and `docs/OTELC_BACKEND.md` describe current user-facing behavior. Keep detailed documentation under `docs/`; keep GoDoc concise. Do not add public Go types without a demonstrated need for a supported SDK.

Do not reduce the project to naming-convention matching.

The product must remain:

- architecture agnostic;
- policy driven;
- source-code independent;
- OpenTelemetry native;
- safe by default;
- deterministic and explainable.

Treat `docs/14_ACCEPTANCE_CRITERIA.md` as release-blocking.

Keep each PR focused and stacked on the immediately preceding open branch.
After its parent merges, restack onto the current surviving parent or main.
Do not merge PRs, create release tags, or publish releases without explicit maintainer approval.
