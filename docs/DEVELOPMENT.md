# Development and verification

OTelPlan is an implemented, pre-stable Go tool. Start with the
[documentation index](README.md), [architecture](ARCHITECTURE.md), and
[roadmap](ROADMAP.md). The [acceptance criteria](14_ACCEPTANCE_CRITERIA.md)
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

Use Go 1.27.4 or a later 1.27.x patch and Linux, the current support and verification target:

```sh
go version
go mod download
make build
```

Use golangci-lint v2.14.0. `make lint` verifies its version and fails for missing
tools or findings. Follow the [real backend and offline setup](#real-pinned-backend) for the pinned
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

From the repository root, run `make check` with the pinned linter. For behavior
changes, also use the [real backend](#real-pinned-backend) and
[vulnerability checks](#authoritative-quality-gates). Record skipped backend
tests explicitly. Prepare offline dependencies in disposable state before
denying lookup, and keep source snapshot assertions.

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

## Independent wire contracts

Inspection and explanation wire contracts use checked-in independent JSON:

```text
internal/cli/testdata/wire-contracts/project/
internal/cli/testdata/wire-contracts/inspect*.json
internal/cli/testdata/wire-contracts/explain-*.json
```

`TestInspectionWireContracts` runs the real CLI pipeline against a disposable copy
of the fixture with offline resolution, checks source immutability, and compares
complete responses without decoding through product model types. Object-key order
and whitespace are ignored; field names, omission, values, numeric representation,
and array order remain significant. Expected files cover context/error strategies,
explicit attribute sources and redaction, failure diagnostics, rule provenance,
and local/global skips.
Tests never generate or overwrite goldens. Contract changes require an intentional,
reviewed update to the checked-in expected JSON.

`TestLockfileWireContracts` checks the fixed JSON under
`internal/lockfile/testdata/wire-contracts/` against canonical marshaling, parsing,
and persisted writes. It compares exact bytes, including indentation and the final
newline. Unsorted typed inputs cover resolution-only and build-metadata sections,
attribute sources, Stringer/static symbol metadata, source positions, and lossless
maximum-width integer constants. The expected files are independent of model
decoding and are never rewritten by the tests.

## Authoritative quality gates

```sh
make test
make test-race
make vet
make lint GOLANGCI_LINT=/path/to/golangci-lint
make test-gates
```

`make check` runs all of these. The required linter version is v2.14.0, verified
with Go 1.27.4. Tool absence, version mismatch,
and real lint failures return nonzero. No broad suppression is an acceptable
substitute for fixing a finding.

GitHub Actions separates everyday development from release acceptance:

- `Check` runs one Linux job for `go test ./...`, `go vet ./...`, and
  `go build ./cmd/otelplan` on PRs and pushes to main. It leaves
  `OTELPLAN_OTELC` empty, so optional real-backend, cold-cache architecture, and
  OSS compatibility tests report skips through their existing test guards.
  New commits cancel superseded Check runs for the same ref. The job has a
  10-minute limit; aim for about five minutes.
- `Release validation` is explicitly started with `workflow_dispatch` on the
  intended committed ref. It retains full unit/race, vet, pinned lint,
  vulnerability, quality-gate, and real pinned-backend normal/race checks.
  Its `release-validation` aggregate requires every job and backend shard to
  succeed. There are no publishing permissions or tag/release operations.

Select the candidate ref in Actions, or invoke and inspect it with:

```sh
gh workflow run release-validation.yml --ref CANDIDATE_BRANCH_OR_TAG
gh run list --workflow release-validation.yml
```

Record the run's exact head SHA and every result in the candidate acceptance
record. A green Check is development feedback and cannot authorize publication.
Release acceptance also requires the documented performance review, reproducible
packaging, extracted-binary exercise, SBOM/provenance, and RC review in
[releasing](RELEASING.md). Linux is the support and release-verification target.

The network-dependent vulnerability gate is explicit:

```sh
GOBIN="$PWD/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
make vuln
```

`GOVULNCHECK=/path/to/govulncheck` overrides the executable. The gate requires
the pinned scanner version, scans source including tests, then checks all modules
used by the CLI without filtering by reachability. Text mode returns nonzero for
findings; JSON mode is not used as a pass/fail gate. Missing tools, scanner errors,
and version mismatches also fail. Release validation requires this separate gate.
`make check` keeps its existing local checks without adding a vulnerability-database lookup.
Results depend on the current Go vulnerability database and supplement, rather
than replace, source review and other security verification.

## Real pinned backend

Build the exact backend using [these instructions](OTELC_BACKEND.md#build-the-pinned-backend).
From OTelPlan:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc go test -timeout=20m ./...
OTELPLAN_OTELC=/absolute/path/to/otelc go test -race -timeout=20m ./...
```

`OTELPLAN_OTELC` selects the exact executable, including a custom filename.
CLI integration tests expose that file as `otelc` in a private temporary PATH
directory; adjacent executables are not selected. Check `go version -m` for both
the CLI and selected backend when recording patched-toolchain evidence.

Both runs use an explicit per-package timeout because cold-cache instrumented
builds can exceed Go's default 10-minute package budget on CI runners. CI divides
the workload into complementary shards, each running normal and race tests:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh a-m
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh remaining
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh architecture
OTELPLAN_OTELC=/absolute/path/to/otelc sh scripts/test-otelc-shard.sh cli-build
```

The A–M shard selects top-level tests matching `^Test[A-M]`, except the four
long pinned CLI build tests and the architecture trace test. The `cli-build`
and `architecture` shards run those tests. The `remaining` shard keeps every
other test, example, and fuzz seed. Each shard uses verbose output, disables
result caching with `-count=1`, and keeps the 20-minute per-package timeout and
45-minute job limit. The release gate requires all four shards to pass.

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

## Upstream limitation regressions

Generic receiver shapes affected by the pinned backend's missing type-argument
inference retain expected failed-build tests with source/output safety assertions.
Generic context replacement, unsupported variadic types, and main-package targets
have validation regressions. Upstream issue closure does not imply that a fix is
present in the pinned executable; see [backend limits](OTELC_BACKEND.md).

## Pinned open-source starter regressions

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
Scoring weights are unchanged.

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc go test ./internal/cli \
  -run '^TestTaskStarterAcceptance$' -count=1 -timeout=20m
```

The complementary CLI workflow regression analyzes the complete Task v3.44.1
module at source commit `72991d4f041e18d925d073e54bc7d264a6d361fb`, with verified
module checksums. Expected suggestions include task execution, command execution,
taskfile reading, and remote reads, with static call and interface evidence.
It checks deterministic previews and locks, exact policy and inspected targets,
strict validation, no capture defaults, and read-only checks.

Task dependency preparation uses a separate copy of its manifests because
`go mod download all` adds transitive checksums. The analyzed application retains
the original source and manifests. Network access is denied before CLI execution;
no Taskfile, shell task, or Task application code runs. Both OSS regressions use
the expensive integration profile enabled by `OTELPLAN_OTELC`. They provide
complementary web/API and CLI workflow evidence; performance measurements belong
in the [performance report](PERFORMANCE.md).

## Hosted release timeout on the combined Go changes

In [run 43776667254](https://github.com/DiLRandI/OTelPlan/actions/runs/43776667254)
on commit `3c313b5` with Go 1.27.4, the A–M normal CLI suite passed in 695.971
seconds. The race suite reached 1200.277 seconds. The four pinned CLI build
tests had already passed and accounted for about 499 seconds in the race run.
Memos also passed. At the alarm, the listed deny-pattern tests were only two
to five seconds into execution. The logs support a cumulative timeout and show
no failed assertion before the alarm. Unit, race, vet, lint, vulnerability,
quality gates, architecture, and remaining-backend jobs passed.

## Historical CI timing evidence

The baseline is [main run 42559538581](https://github.com/DiLRandI/OTelPlan/actions/runs/42559538581)
on revision `36e5cc2e61fb32d816b2bafe79f7a2fc3b023ab3`, Go 1.27.3,
Linux GitHub-hosted runners. Its nine jobs took 33m42s from run creation to
completion. Unit took 2m47s, race 6m18s, and the failing OTelC A-M job 33m33s.
These are observed wall times, including setup and cleanup, not performance SLOs.

The A-M normal CLI package passed in 738.260s. Its subsequent race run hit Go's
cumulative 20-minute package timeout. At the alarm, active analysis/inspection
tests had been running only 2-4 seconds; the architecture test had already
finished. The stack dump shows Go package loading and subprocess waits, not a
failed trace assertion or evidence of a stuck architecture build. The serial
cold-cache architecture workload and later parallel tests shared one package
budget. Isolating that workload gives each invocation its own unchanged budget.
No product resource leak is established by this log. Retain further run evidence
before treating the timeout correction as verified on hosted runners.

The replacement [PR run 43648581990](https://github.com/DiLRandI/OTelPlan/actions/runs/43648581990)
passed on `1f9af1428f38ab0a491fd1750c12724dcb849f70`. It took 2m57s from
creation to completion, with one job running for 2m54s on Go 1.27.4. Tests, vet, and CLI build
all passed. This single hosted sample meets the five-minute development target;
it excludes the deliberately separate release gates and does not prove their
success. The local non-backend suite took 65.79s on Go 1.27.3.

Local real-backend verification of the CI slice passed all three shards in both
normal and race modes with Go 1.27.3, `GOMAXPROCS=4`, and OTelC v1.1.0 built
from the pinned source. Architecture took 309.44s, A-M 1640.08s, and remaining
385.82s, each including both modes. The A-M CLI package took 636.046s normally
and 1001.021s under race, within its unchanged 1200s budget. Other local Go
checks ran concurrently, so these are compatibility and timeout-budget evidence,
not hosted duration or benchmark claims. The Go source and runner were unchanged
by the documentation-only `29d4942` update during this run.

Go 1.27.3 now fails the vulnerability gate; the patched-toolchain baseline and
separate Go 1.27.4 architecture evidence are in
[PR #306](https://github.com/DiLRandI/OTelPlan/pull/306). The workflow is now available on main; historical evidence does not replace
combined-tip validation. Exact backend selection was corrected in PR #318;
see the current [roadmap](ROADMAP.md) for refreshed runs.
