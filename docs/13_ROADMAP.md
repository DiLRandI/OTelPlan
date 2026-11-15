# Implementation Roadmap

This is not an "MVP with fake value." Each phase must be production-quality for the capability it introduces.

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

Earlier documentation consolidation and lint cleanup are implemented. The v1.0
readiness review must reassess the active documents and the entire Go codebase.

Current readiness work, in priority order:

| Work | Status and next evidence |
| --- | --- |
| Everyday CI | [PR #305](https://github.com/DiLRandI/OTelPlan/pull/305) is merged. One job runs tests, vet, and CLI build; full validation moves to a manual workflow. Hosted Check passed in 2m57s versus the 33m42s failing baseline. All three local real-backend normal/race shards passed with unchanged timeouts. Manual hosted release validation is now available on main; a candidate-specific run remains required. |
| Toolchain security | [PR #306](https://github.com/DiLRandI/OTelPlan/pull/306), based on #305, requires Go 1.27.4 and is merged. Hosted Check passed in 2m23s. Full local checks, both vulnerability scans, patched-toolchain architecture normal/race tests, and two identical real Linux archive builds passed. Candidate-wide final acceptance remains. |
| Go review | [PR #307](https://github.com/DiLRandI/OTelPlan/pull/307) contains policy cleanup and awaits review; hosted Check passed in 2m22s. [PR #308](https://github.com/DiLRandI/OTelPlan/pull/308) contains glob cache cleanup with full local checks passed. Coverage and remaining packages are listed below; neither PR establishes whole-codebase completion. |
| Documentation | Reassess navigation, overlapping specifications, and historical instructions after CI and Go review. |
| Release | Candidate-specific acceptance/security/performance review, SBOM/provenance, protected publishing implementation, and a complete v1.0 RC cycle remain. No tag, release, or merge is authorized. |

The CI split and Go 1.27.4 baseline are merged. Open PRs #307 through #314
form a linear review stack, each based on its immediate predecessor; #307 is
based on main. All three
local backend normal/race shards passed. Next, publish the verified diagnostic presence-query cleanup, then continue
the discovery/cache contract tests and the CLI/backend source and test reviews without merging these PRs.
Candidate-wide acceptance, security/performance review, SBOM/provenance, and the
actual RC cycle remain. See
[CI execution and timeout evidence](10_TEST_STRATEGY.md#6-authoritative-quality-gates).

Remaining feature/performance/safety acceptance work follows. Linux is the current
support and release-verification target. Lightweight OSS/release hygiene follows. Original phase requirements below remain
the intended scope; an implemented feature still needs its release evidence.
Use the [candidate acceptance record](RELEASING.md#candidate-acceptance-record)
to review one immutable revision. The documented publishing design is not an
implemented release workflow.

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
| `internal/lockfile` | All production and test sources reviewed. Preserve canonical wire bytes, JSON numeric equality, resolution/build ownership, and rooted manifest reads. Regressions confirmed false drift and hidden compiler/flag changes from blanket checkout-text replacement. The targeted correction and CLI relocation/drift regression pass full quality and vulnerability gates. Architecture normal/race passed. The A-M race run hit a checksum-service preparation outage; its affected offline test passed a verified normal/race retry, and the remaining shard passed both modes. The correction is [PR #310](https://github.com/DiLRandI/OTelPlan/pull/310), awaiting review; final candidate acceptance remains separate. |
| Compiler cancellation | A Linux regression proved that context cancellation stopped the backend but left its child running. The targeted process-group cancellation fix preserves context error identity and passes focused race/vet/lint. Full unit/race/vet/lint/gate and vulnerability checks pass on Go 1.27.4. All three pinned real-backend normal/race shards pass on Go 1.27.4. The focused fix is [PR #311](https://github.com/DiLRandI/OTelPlan/pull/311), awaiting review; its hosted Check passed in 2m27s. Compiler production sources covering workspaces, module selection, artifacts/publication, build arguments/environment, and outputs have been inspected; the contract-test review is complete; deterministic file-identity regressions are [PR #313](https://github.com/DiLRandI/OTelPlan/pull/313), with full local checks and hosted Check passed. |
| Test contracts | [PR #312](https://github.com/DiLRandI/OTelPlan/pull/312) now checks valid lockfile inputs before every diff-classification case. The signature mutation was corrected without weakening expected classifications; full local checks and hosted Check passed. Artifact-read tests in [PR #313](https://github.com/DiLRandI/OTelPlan/pull/313) cover different opened file identity despite identical bytes and reject a directory handle before reading. Focused and full local unit/race/vet/lint/gate checks pass; production code and lint policy are unchanged. |
| Discovery review | All production sources inspected, including build inputs, isolated manifests, typed inventory, cache identity/storage/retention, call graphs, recursion, manual spans, and typed entrypoint readers. Test review remains in progress. Direct requested-key lookup of nonregular cache entries lacks a bounded-read regression; investigate before changing its storage-error behavior. |
| Remaining | `internal/cli`, `internal/discovery`, and `internal/backend/otelc` still need their structured review, including owned fixtures and generated code. |

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
