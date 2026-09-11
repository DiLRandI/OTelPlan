The adapter pins `otelc v1.1.0`. Other versions fail compatibility validation. Executable checks verify the reported version and compute a SHA-256 binary digest.

The capability map follows the pinned [hook API](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.1.0/pkg/hook/context.go) and [trampoline implementation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.1.0/tool/internal/instrument/trampoline.go). Argument indexes in backend hooks include the receiver; OTelPlan indexes exclude it.

Generic targets are rejected because this backend disables parameter/result APIs, including context replacement. See [upstream issue 1280](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/issues/1280). After hooks run through a deferred trampoline; the API does not expose the application panic value, so panic observation is not advertised.

Run the real executable identity check with `OTELPLAN_OTELC=/path/to/otelc go test ./internal/backend/otelc`. Generated instrumentation and trace correctness need separate end-to-end checks.

`RenderRules` converts a resolved plan into deterministic function-entry rules and stable before/after hook bindings. It preserves value/pointer receiver identity and rejects duplicate targets, inconsistent identities, and instrumentation of the generated hook package. Main-package targets require a verified isolated-build mapping and are currently rejected by generation.

`OTELPLAN_OTELC=/path/to/otelc go test ./internal/backend/otelc -run TestGeneratedRulesWithPinnedBackend` exercises generated rules through the real pinned compiler, verifying nested trace parentage and returned-error events. The test compiles generated lifecycle hooks with the race detector and exercises methods, explicit roots, nil contexts, disabled error recording, panic propagation, no-op telemetry, and concurrent calls. Compile/build CLI wiring remains separate work.

`RenderHooks` emits deterministic Go source for those bindings. Each invocation uses backend-local span state, replaces the selected context argument, records configured returned errors with a bounded status description, and ends the span on exit. The instrumentation scope is `otelplan.io/business` with the supplied runtime version. Nil context falls back to `context.Background`; explicit root mode starts a separate trace. No SDK or exporter is installed by generated hooks.

This generation stage rejects attribute plans and variadic targets until typed accessor/signature generation is available. It fails without partial output, so captures are never silently omitted. The backend still cannot expose the application panic value; hooks end the span while preserving panic propagation.
