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
| Safety/reproducibility | Static context/error/capture checks, canonical policy/build fingerprints, resolution locks and drift implemented. Dedicated recursion/manual-span warnings and some configuration controls remain goals. |
| OTelC backend | Real rules/hooks/attributes and isolated compile/build implemented. Some generic, variadic, main-package, and panic capabilities are limited by the documented pin/scoping constraints. |
| Hardening | Workspaces, local replacements, tags, isolated vendor builds, advisory call graphs, safe verbose diagnostics, and opt-in engine cache implemented. CLI cache controls, runtime benchmark/report, clean lint, security verification, and multi-platform confidence remain. |
| Initialization | Deterministic scoring/evidence, exact starter policy, interactive review, and no default capture implemented. Representative open-source application usefulness gate still needs evidence. |
| Stable v1 | Contract stability/migration commitments, release candidate cycle, security review, complete performance report, and release automation remain. |

Immediate priorities are documentation consolidation and the full lint backlog,
then remaining feature/performance/safety acceptance work. Cross-platform CI and
lightweight OSS/release hygiene follow. Original phase requirements below remain
the intended scope; an implemented feature still needs its release evidence.

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
- multi-platform CI;
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
