# Implementation Roadmap

This is not an "MVP with fake value." Each phase must be production-quality for the capability it introduces.

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
