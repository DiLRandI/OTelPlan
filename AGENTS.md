# Agent Instructions

Read `docs/README.md`, `docs/CODEX_IMPLEMENTATION_PROMPT.md`, and all specification documents before implementing.

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
