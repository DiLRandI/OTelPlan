# Implementation Roadmap

## Current status

The scan/init/inspect/explain/validate/lock/diff/compile/build workflow is
implemented. Three architecture layouts have real pinned-backend trace tests,
including independent cold-cache online and prepared-cache offline execution.
This is a pre-stable implementation, not a completed release.

| Phase | Status and remaining evidence |
| --- | --- |
| Foundation | Semantic discovery, policy matching, exact resolution, exclusions, inspect/explain implemented. |
| Safety/reproducibility | Static context/error/capture checks, static recursion and manual-span warnings, configurable trace-volume thresholds, canonical policy/build fingerprints, resolution locks and drift implemented. |
| OTelC backend | Real rules/hooks/attributes and isolated compile/build implemented. Some generic, variadic, main-package, and panic capabilities are limited by the documented pin/scoping constraints. |
| Hardening | Workspaces, local replacements, tags, isolated vendor builds, advisory call graphs, safe verbose diagnostics, and opt-in engine cache implemented. Opt-in CLI cache controls and read-only cache reuse are implemented. Bounded writable cache retention, CLI/runtime benchmark harnesses, and initial synthetic performance reports are implemented. Candidate-specific performance/security review and Linux release acceptance remain. |
| Initialization | Deterministic scoring/evidence, exact starter policy, interactive review, and no default capture implemented. Pinned full-source Memos and Task regressions cover web/API and CLI workflow operations, determinism, exact resolution, and source safety. |
| Stable v1 | Compatibility/upgrade guidance and stable-line contract commitments are documented. Local Linux archives, checksum/notice verification, initial CLI/runtime performance reports, and a current capability changelog are implemented. Candidate-specific contract/acceptance/performance/security review, standardized SBOM/verifiable provenance publication, and an RC cycle remain. |

| Work | Status and next evidence |
| --- | --- |
| Everyday CI | The single Linux development job runs tests, vet, and CLI build. The first hosted Check passed in 2m57s, compared with the 33m42s failing baseline. See [development verification](DEVELOPMENT.md#authoritative-quality-gates) and [PR #305](https://github.com/DiLRandI/OTelPlan/pull/305). |
| Go review | The structured review covers production, tests, and owned fixtures. PRs #307 through #319 are merged. Local quality and real adapter normal/race checks passed on the #319 tip. Its hosted validation passed all jobs except the A–M race shard, which exhausted the 20-minute CLI package timeout. See [run 43776667254](https://github.com/DiLRandI/OTelPlan/actions/runs/43776667254). |
| Documentation | [PR #320](https://github.com/DiLRandI/OTelPlan/pull/320) removed obsolete plans. This branch consolidates current policy, safety, architecture, runtime, compatibility, and development references. The acceptance criteria remain the release gate. |
| Release | The release workflow separates four expensive pinned CLI build tests into a fourth backend shard. Full hosted validation on the revised workflow is required. Candidate security and performance review, SPDX SBOM, authenticated provenance, protected publishing, packaged CLI acceptance, and an RC review still remain. |

Linux is the supported verification target. Record the exact revision and results
for each candidate using the [candidate acceptance record](RELEASING.md#candidate-acceptance-record).
The maintainer authorizes merges, tags, and publication separately.

## Go readiness review

Review against [Effective Go](https://go.dev/doc/effective_go),
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), and the owning
standard-library contracts. Cover boundaries, error identity, cancellation and
cleanup, deterministic output, allocations, and behavior-focused tests. Record
review coverage here instead of opening a second tracker. Production and test
sources count toward this review; fixture and generated-source checks remain
with their owning runtime/discovery/compiler packages.

| Scope | Review result and remaining work |
| --- | --- |
| `cmd/otelplan` | Signal cancellation and deferred signal cleanup reviewed. No change needed. Integration behavior is covered by CLI cancellation tests; the CLI package review remains. |
| `internal/model`, `pkg/model` | Definitions, tags, compatibility aliases, canonical symbol helpers, and public compatibility tests inspected. Keep existing alias/interface contracts. Model source and public compatibility test review is complete. HasErrors now performs an early-return presence query on `refactor/diagnostic-error-predicate`; Errors and Warnings filtering remains unchanged. Full local unit/race/vet/lint/gate checks pass. The 1,000-error benchmark falls from 236,896 B and 11 allocations per query to zero; the warning-only case remains allocation-free. [Raw method samples](benchmarks/diagnostic-presence-go1.27.4-linux-amd64.txt) retain source identities and scope limits. |
| `internal/policy` | All production and test files reviewed. Remove the production dependency on compatibility aliases and initialize safe defaults through the policy zero value. Retain exhaustive keyed literals required by the current linter without adding exceptions. Existing defaults/explicit-false, no-capture, unknown-field/document-count, diagnostic JSON/order, schema, and non-mutation regressions protect behavior. Focused and full unit/race/vet/lint/gate checks pass on Go 1.27.4. Optional backend tests skipped in these local checks; no assertions or fixture inputs changed. |
| `internal/suggest` | All four production/test files reviewed. Eligibility, scoring, BFS root ordering, duplicate-edge handling, conservative-call evidence, and caller immutability need no behavioral change. Retain canonical parser usage through the existing compatibility boundary. Existing deterministic ranking and entrypoint tests cover these decisions; candidate-wide backend/OSS checks remain separate. |
| `internal/resolve` | All production and test files reviewed. Preserve selector validation even after earlier mismatches, exclusion-before-conflict behavior, deterministic provenance/diagnostics, context/error strategies, and input ownership. The glob memo cache now uses one map and comma-ok lookup instead of a second visited map on [PR #308](https://github.com/DiLRandI/OTelPlan/pull/308). Added repeated-** backtracking cases pass; focused race/vet/lint checks pass. In three 100-iteration Go 1.27.4 allocation samples, recursive match fell from 13,536 to 7,096 B/op and miss from 26,656 to 13,656 B/op. Literal matching remains 64 B/op. [Raw allocation samples](benchmarks/glob-cache-go1.27.4-linux-amd64.txt) retain source hashes and environment. Ignore concurrent-load timing; full unit/race/vet/lint/gate checks pass on Go 1.27.4, with optional backend tests skipped in these checks. No matcher behavior change is intended. |
| `internal/testenv` | Both source files reviewed. Cache cleanup has an independent bounded context; proxy counters, server/root cleanup, manifest isolation, and byte snapshots support the owning integration tests. No change needed. Snapshots check membership and bytes, not all filesystem metadata. |
| `internal/validate` | All five production and thirteen test files reviewed. Promoted-field ambiguity/cycles, scalar bounds, explicit Stringer conversion, error-result aliases/indexes, privacy acknowledgment, ordered/redacted diagnostics, and plan-size limits retain their contracts. Production imports now name internal models directly in [PR #309](https://github.com/DiLRandI/OTelPlan/pull/309), avoiding accidental SDK expansion when internal models grow. Existing tests still exercise public aliases. Focused checks before/after the import change and full unit/race/vet/lint/gate checks pass on Go 1.27.4. Optional backend tests skipped in the full local checks; candidate-wide acceptance remains separate. |
| `internal/lockfile` | All production and test sources reviewed. Preserve canonical wire bytes, JSON numeric equality, resolution/build ownership, and rooted manifest reads. Regressions confirmed false drift and hidden compiler/flag changes from blanket checkout-text replacement. The targeted correction and CLI relocation/drift regression pass full quality and vulnerability gates. Architecture normal/race passed. The A-M race run hit a checksum-service preparation outage; its affected offline test passed a verified normal/race retry, and the remaining shard passed both modes. The correction is [PR #310](https://github.com/DiLRandI/OTelPlan/pull/310), merged; final candidate acceptance remains separate. |
| Compiler cancellation | A Linux regression proved that context cancellation stopped the backend but left its child running. The targeted process-group cancellation fix preserves context error identity and passes focused race/vet/lint. Full unit/race/vet/lint/gate and vulnerability checks pass on Go 1.27.4. All three pinned real-backend normal/race shards pass on Go 1.27.4. The focused fix is [PR #311](https://github.com/DiLRandI/OTelPlan/pull/311), merged; its hosted Check passed in 2m27s. Compiler production sources covering workspaces, module selection, artifacts/publication, build arguments/environment, and outputs have been inspected; the contract-test review is complete; deterministic file-identity regressions are [PR #313](https://github.com/DiLRandI/OTelPlan/pull/313), with full local checks and hosted Check passed. |
| Test contracts | [PR #312](https://github.com/DiLRandI/OTelPlan/pull/312) now checks valid lockfile inputs before every diff-classification case. The signature mutation was corrected without weakening expected classifications; full local checks and hosted Check passed. Artifact-read tests in [PR #313](https://github.com/DiLRandI/OTelPlan/pull/313) cover different opened file identity despite identical bytes and reject a directory handle before reading. Focused and full local unit/race/vet/lint/gate checks pass; production code and lint policy are unchanged. |
| Discovery review | All production sources inspected, including build inputs, isolated manifests, typed inventory, cache identity/storage/retention, call graphs, recursion, manual spans, and typed entrypoint readers. All discovery test sources reviewed, including owned inline fixtures. Build-mode/tag precedence, alternate manifests/workspaces, typed HTTP/gRPC/NATS/Connect evidence, conservative call graphs, recursion/manual-span metadata, privacy, cache invalidation, retention, and source preservation retain their contracts. A bounded public-API regression confirmed FIFO lookup could block and then become a cache miss. The correction in [PR #315](https://github.com/DiLRandI/OTelPlan/pull/315) uses nonblocking opens and opened-file type checks, returning ErrCacheStorage and preserving directory/FIFO entries in writable/read-only modes. Retention keeps its existing preserve/ignore semantics for nonregular entries. The strengthened FIFO regression fails on the unmodified #314 tip and passes with the correction. Focused discovery race tests, full unit/race/vet/pinned-lint/gate checks, and both vulnerability scans pass on the combined stack with Go 1.27.4. Optional real-backend tests were not rerun for this restacked change; the earlier backend run hit a Memos dependency-download IPv6 outage and is not green aggregate evidence. |
| CLI lockfile reads | The real CLI blocked on a FIFO for eight seconds. A new public-command regression fails on the old code for lock, lock --check, lock --dry-run, diff, and validate. Plain lock also replaced the FIFO after it was released. The correction in [PR #316](https://github.com/DiLRandI/OTelPlan/pull/316) uses a nonblocking rooted open and checks the opened file type before reading. All five FIFO cases and the existing directory/internal-symlink/external-symlink race tests pass, preserving source bytes and FIFO entries. Full unit/race/vet/pinned-lint/gate checks, both vulnerability scans, and the CLI build pass on Go 1.27.4. The rebuilt CLI returns exit 6 with OTP6001 for all five command variants within the bound, preserving the FIFO and source. Optional backend tests were not rerun for this lock-read correction. |
| Build output safety | A real CLI build through a symlinked parent directory returned success and replaced the selected policy with an ELF executable on the previously verified #314 binary. Lexical destination checks miss this alias. The correction in [PR #317](https://github.com/DiLRandI/OTelPlan/pull/317) compares declared/resolved destination and metadata paths while allowing ordinary symlink-directory output. Unit regressions cover policy, missing lock, alternate manifest/workspace, root aliases, and selected policy symlink targets. Unit and actual pinned-backend regressions fail on the old code and pass with race detection after the fix; build/preview both reject the unsafe destination and preserve source. Full unit/race/vet/pinned-lint/gate checks and both vulnerability scans pass on Go 1.27.4. The affected real-backend build suites pass in normal/race modes with the exact patched backend binary. The rebuilt CLI rejects the unsafe alias with exit 2/OTP1005 and preserves the policy; ordinary aliased output still builds an ELF executable and emits the expected five spans. Full combined-candidate backend acceptance remains separate. |
| Backend verification identity | Several CLI integration tests put only the directory of OTELPLAN_OTELC on PATH. A custom filename such as otelc-go1.27.4 can therefore select its sibling otelc, which is still Go 1.27.3 locally. Earlier Go 1.27.4 harness runs do not prove every CLI scenario used the patched backend binary. Current alias regressions use an isolated directory containing the exact Go 1.27.4 binary as otelc, digest 83df41b52796aea04a1f80f546ef1c7a1e2efe6df1f8b05be28c724d4ed40491. The correction in [PR #318](https://github.com/DiLRandI/OTelPlan/pull/318) replaces all 13 affected CLI setups with a private PATH alias to the exact requested executable. The new regression executed the wrong sibling before the correction and now passes with a custom filename containing spaces. Focused race/lint and full unit/race/vet/pinned-lint/gate checks pass. Using a real Go 1.27.4 backend with a custom filename and an adjacent executable that fails if selected, pinned build, offline-proxy-bypass, and all three architecture layouts in cold-online/prepared-offline scenarios pass in normal/race modes. Rerun all combined-candidate backend gates before release acceptance. |
| Engine model boundary | `refactor/engine-internal-models` switches 54 production files to the internal domain types and moves the two symbol-ID constructors there. Public type aliases and constructor names remain compatible; the parser keeps its existing compatibility implementation and error causes, with three explicit parser-only bridges. Its stable error-identity regression passes before and after the change. Focused race tests, the complete unit/race/vet/pinned-lint/gate checks, and the entire real pinned-backend adapter test package in normal/race modes pass on Go 1.27.4. Public SDK compatibility and independent wire/generated-source contracts pass. Full combined-candidate CLI/backend/security/performance acceptance remains separate. |
| Remaining | `internal/cli` production and test source review is complete, including inline, architecture, wire-golden, and runtime-benchmark fixtures. Preserve flag/error precedence, wire formats, read-only previews, redaction, context/stream ownership, exact selection, and publication checks; the concrete lock/output fixes and backend test-harness follow-up are recorded above. All `internal/backend/otelc` production sources have now been inspected for exact rules, hook lifecycle, typed accessors, privacy, pinning, executable/version boundaries, and deterministic bundle partitioning. All 19 backend test files and their owned/shared/generated fixtures are now reviewed. Exact rules, deterministic partitions and source hashes, nil context and panic preservation, concurrent invocation state, typed scalar bounds, Stringer isolation, error identity/recording, source snapshots, and runtime dependency isolation retain their contracts. Raw-backend fixtures exclude pin-owned debug/runtime outputs; CLI/compiler isolation tests protect caller-owned source separately. Final candidate security/performance and release acceptance remain separate. |

## Phase 1 — Foundation

Deliver:

- project loader;
- canonical symbol model;
- policy parser/schema;
- package/file/symbol/function/method/receiver matching;
- interface implementation matching;
- exclusions;
- inspect/explain;
- deterministic diagnostics.

Exit gate: multiple architecture fixtures resolve correctly.

## Phase 2 — Safety and reproducibility

Deliver:

- context analysis;
- error-result analysis;
- attribute type resolution;
- secret/PII/cardinality validation;
- lockfile;
- diff;
- stable JSON CLI.

Exit gate: policy can be safely reviewed and frozen in CI.

## Phase 3 — OTelC backend

Deliver:

- backend abstraction;
- pinned `otelc` support;
- capability validation;
- generated exact backend rules;
- runtime hooks;
- build isolation;
- compile/build commands.

Exit gate: real fixture produces correct business spans with unchanged source.

## Phase 4 — Production hardening

Deliver:

- call-graph-assisted discovery;
- cache;
- performance benchmarks;
- go.work;
- vendor mode;
- build tags;
- generics;
- diagnostics documentation;
- Linux CI and release verification;
- compatibility matrix.

Exit gate: beta quality.

## Phase 5 — Intelligent initialization

Deliver deterministic suggestion engine:

- call-path analysis;
- interface boundary recognition;
- infrastructure adjacency;
- entrypoint proximity;
- span-noise scoring.

`init` produces a strong starter policy without claiming certainty.

Exit gate: representative open-source Go applications produce useful, explainable suggestions.

## Phase 6 — Stable v1

Requirements:

- policy schema stability commitment;
- lockfile stability commitment;
- documented migration commands;
- compatibility policy;
- security review;
- performance report;
- full docs and examples;
- at least one release candidate cycle.

## Explicitly defer

Until core quality is proven:

- LLM-based inference;
- hosted SaaS;
- vendor-specific exporters;
- IDE plugin;
- GUI;
- automatic source code modification.
