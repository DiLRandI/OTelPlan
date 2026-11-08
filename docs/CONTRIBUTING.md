# Contributing

OTelPlan is an implemented, pre-stable Go tool. Start with the
[documentation index](README.md), [architecture](02_ARCHITECTURE.md), and
[roadmap](13_ROADMAP.md). The [acceptance criteria](14_ACCEPTANCE_CRITERIA.md)
remain release-blocking. Current command behavior belongs in [CLI.md](CLI.md),
and verified backend support belongs in [OTELC_BACKEND.md](OTELC_BACKEND.md).

## Issues and proposals

Search existing issues before reporting a bug. Include a minimal reproduction,
the OTelPlan revision, Go and backend versions, command, and expected versus
actual behavior. Use dummy application data and redact credentials, private
source, environment values, and telemetry. Report suspected vulnerabilities
through the [private security process](SECURITY.md).

For a feature proposal, explain the concrete problem and why the existing policy
or CLI does not solve it. Discuss architecture or compatibility changes before
implementing them. Avoid adding speculative extension points or dependencies.

## Development setup

Use Go 1.27.x and Linux, the current support and verification target:

```sh
go version
go mod download
make build
```

Use golangci-lint v2.14.0. `make lint` verifies its version and fails for missing
tools or findings. Follow the [testing guide](10_TEST_STRATEGY.md) for the pinned
backend, dependency preparation, offline tests, and vulnerability scanner setup.

## Implementing a change

- Read the owning implementation, existing tests, and relevant specifications.
  Confirm the reported behavior before changing it.
- Keep each PR focused on one concern. Add a regression test for a behavior change
  before fixing it where practical, and cover meaningful failure paths.
- Preserve the policy → CodeModel → resolver → ResolvedPlan → validation →
  lockfile → backend boundaries. Keep OTelC syntax inside its adapter.
- Preserve exact selection, source immutability, isolated builds, explicit capture,
  deterministic output, stable diagnostic codes, and caller-owned cancellation.
- Keep policy YAML, locks, and CLI JSON compatibility visible in review. Domain
  models belong internally; public Go types need a demonstrated SDK consumer.
- Fix lint findings directly. Obtain explicit maintainer approval before adding a
  suppression or exclusion, ignoring a finding, or disabling a lint rule.
- Format changed Go files with `gofmt`. Keep detailed documentation under `docs/`
  and link to authoritative references instead of duplicating them.

## Verification

Run the authoritative local gates from the repository root:

```sh
make check GOLANGCI_LINT=/absolute/path/to/golangci-lint
```

This runs unit tests, race tests, vet, lint, and quality-gate regressions. For
behavior changes, also run the real pinned OTelC path and vulnerability gate:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh a-m
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh remaining
make vuln GOVULNCHECK=/absolute/path/to/govulncheck
```

Without `OTELPLAN_OTELC`, backend integration tests skip. Report that explicitly;
ordinary unit-test success does not prove trace E2E behavior. Dependency preparation
may require network access. Offline regressions must prepare dependencies first,
then deny lookup while preserving their assertions and source snapshots.

For documentation changes, check relative links, run `git diff --check` and
`go test ./...`, and verify documented behavior against the implementation.
Record the exact checks run, skipped checks, and any remaining upstream limitation.
Never describe an unperformed check as passing.

## Pull requests

Use the PR template to explain the problem, resulting behavior, verification,
compatibility risk, and dependencies. Inspect the final diff for unrelated files
and sensitive values before publishing.

Split dependent changes into reviewable PRs targeting the preceding branch and
state the parent PR in each description. Refresh from the latest `main` when the
parent has merged. Keep each PR's tests and scope independently understandable.
Implementation or testing agents must not merge PRs without explicit authorization.

Contributions are covered by the repository's [Apache License 2.0](../LICENSE).
