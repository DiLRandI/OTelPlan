The adapter pins `otelc v1.1.0`. Other versions fail compatibility validation. Executable checks verify the reported version and compute a SHA-256 binary digest.

The capability map follows the pinned [hook API](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.1.0/pkg/hook/context.go) and [trampoline implementation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.1.0/tool/internal/instrument/trampoline.go). Argument indexes in backend hooks include the receiver; OTelPlan indexes exclude it.

Generic targets are rejected because this backend disables parameter/result APIs, including context replacement. See [upstream issue 1280](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/issues/1280). After hooks run through a deferred trampoline; the API does not expose the application panic value, so panic observation is not advertised.

Run the real executable identity check with `OTELPLAN_OTELC=/path/to/otelc go test ./internal/backend/otelc`. Generated instrumentation and trace correctness need separate end-to-end checks.
