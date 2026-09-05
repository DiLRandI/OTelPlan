# Product Vision

## One-sentence definition

OTelPlan is an architecture-agnostic policy compiler that turns a Go application's real program structure plus a human-owned tracing policy into zero-source-change OpenTelemetry business tracing.

## Problem

A Go team can usually auto-instrument transport and infrastructure boundaries, but meaningful business operations still tend to require manual spans.

Manual instrumentation causes:

- observability concerns inside business code;
- inconsistent span naming and error handling;
- partial coverage;
- vendor coupling;
- review overhead;
- instrumentation drift as code evolves.

Naming-convention-based generators are not sufficient because real Go projects organize code differently.

## Product thesis

The hard problem is not "how to start a span." OpenTelemetry already solves that.

The valuable problem is:

> Which operations in this particular codebase should become spans, under what policy, and how can that decision remain safe, reproducible, inspectable, and source-code independent?

## Product boundary

OTelPlan owns:

- code discovery;
- symbol resolution;
- selection policy;
- semantic validation;
- trace-safety validation;
- preview and diff;
- lockfile generation;
- backend compilation;
- compatibility checks.

OTelPlan does not own:

- OTLP transport;
- SDK exporter implementation;
- collector configuration;
- backend-specific dashboards;
- general HTTP/DB/Redis instrumentation already provided by the ecosystem;
- a custom Go compiler weaving engine.

## Primary user

A Go engineer or platform team with an existing application who wants business-level spans without manually adding tracing code.

## Success experience

```bash
otelplan init ./...
otelplan inspect
$EDITOR otelplan.yaml
otelplan validate
otelplan lock
otelplan build ./cmd/api
```

The engineer sees exact spans before building, receives warnings about unsafe or broken selections, commits the policy and lockfile, and builds with no modifications to application `.go` files.
