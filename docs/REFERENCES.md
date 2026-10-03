# External references

These sources informed the product boundary and implementation. Upstream main
may differ from the pinned backend. See [the current backend reference](OTELC_BACKEND.md)
for verified capabilities and limitations.

## OpenTelemetry Go compile-time instrumentation

- OpenTelemetry documentation — Go compile-time instrumentation  
  https://opentelemetry.io/docs/zero-code/go/compile-time/

  Key facts used:
  - stable/production-ready as of v1.0.0;
  - instruments during build without source changes;
  - wraps/intercepts the Go build via `-toolexec`;
  - supports popular library instrumentation.

- OTelC UX design  
  https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/main/docs/ux-design.md

  Key facts used:
  - application-specific/source-controlled configuration;
  - instrumentation packages;
  - rule files;
  - pointcut/advice model;
  - before/after advice examples;
  - clean-room usage concept.

- OTelC getting started  
  https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/main/docs/getting-started.md

  Key facts used:
  - `otelc go build`;
  - `-toolexec` integration;
  - generated setup/build artifacts;
  - supported integrations.

## Existing adjacent projects

- tracegen  
  https://github.com/KazanExpress/tracegen

  Generates OpenTelemetry decorators for public interfaces and documents interface/context restrictions.

- GoWrap  
  https://github.com/hexdigest/gowrap

  Generic decorator generation for Go interfaces.

- New Relic Go Easy Instrumentation  
  https://docs.newrelic.com/docs/apm/agents/go-agent/installation/install-automation-new-relic-go/

  Analyzes Go source and creates a reviewable `.diff` containing source instrumentation suggestions.

## Go toolchain

- Go release history  
  https://go.dev/doc/devel/release

  The repository requires Go 1.27.x; use `go.mod` and local verification as the
  authority for the supported toolchain.

## Important implementation rule

External documentation is not an API guarantee.

For any backend update, inspect its exact source, rule schema, CLI, hook API, and
tests. Retain compatibility and real trace tests against the pin. OTelPlan
currently uses v1.1.0 at `449ee08a682586adb177e4402845ed404565879f`.
