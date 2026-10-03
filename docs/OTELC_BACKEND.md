# OTelC backend

OTelPlan pins `otelc v1.1.0`, source commit
`449ee08a682586adb177e4402845ed404565879f`. Other versions fail compatibility
validation. Compile/build require that executable on `PATH`, verify its reported
version, and compute a SHA-256 digest. Lock/validate use the configured identity
and known capabilities without running the executable.

Backend-specific rule syntax remains inside the adapter. User policies resolve
to exact backend-neutral plans. See [compiler architecture](06_COMPILER_BACKEND.md).

## Build the pinned backend

From a separate checkout:

```sh
git clone --branch v1.1.0 --depth 1 \
  https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation.git
cd opentelemetry-go-compile-instrumentation
git rev-parse HEAD
# Verify 449ee08a682586adb177e4402845ed404565879f before building.
go build -mod=readonly -trimpath \
  -ldflags '-X go.opentelemetry.io/otelc/tool/util.Version=v1.1.0' \
  -o /absolute/output/path/otelc ./tool/cmd/otelc
```

Place the executable on `PATH`. `otelc version` must report v1.1.0.
The CI workflow uses the exact commit rather than a moving upstream branch.

## Support matrix

| Area | Current behavior |
| --- | --- |
| Toolchain | OTelPlan requires Go 1.27.x. Generated runtime module declares Go 1.25 with pinned dependencies. |
| Platforms | Linux is covered by CI and real backend E2E. macOS/Windows CI verification is pending. |
| Selection | Exact importable-package functions and value/pointer receiver methods with Go bodies. |
| Context | Unique `context.Context` argument is replaced with the child context for ordinary declarations. Explicit root fallback is supported. |
| Errors | Configured returned-error indexes are recorded with a bounded status description. Return values remain unchanged. |
| Attributes | Explicit constants and typed scalar argument/result field paths, including private/imported named types. |
| Generics | Explicit root spans, returned errors, and captures through concrete types. Generic context replacement and unbound capture types are rejected. Some generic receiver methods fail under the pin. |
| Variadics | Built-in element types supported through typed hooks. Application-defined element types rejected. |
| `package main` | Instrumentation targets rejected pending verified command-specific build scoping. Building a command that calls instrumented library packages is supported. |
| Panics | Deferred after hooks end spans and preserve panic propagation. Panic values are not observable. |
| Modules/workspaces | Isolated copies preserve dependency-selection validation, local replacements, and explicit source-selection flags. |
| Vendor mode | Isolated combined workspace vendoring with content verification; dependency preparation required for offline use. |

## Exact rules and runtime hooks

`RenderRules` emits deterministic function-entry rules and stable hook bindings.
It preserves receiver identity, rejects duplicate/inconsistent targets, and
prevents instrumentation of generated hook packages. The capability map follows
the pinned [hook API](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.1.0/pkg/hook/context.go)
and [trampoline implementation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.1.0/tool/internal/instrument/trampoline.go).
Backend argument indexes include receivers; policy indexes exclude them.

Each invocation owns its span state. Before hooks inherit the selected parent
context and replace the application context argument. Nil context falls back to
`context.Background`. Policy `context.mode: root` permits a root only when no
context argument is available; it does not force a root for declarations that
already accept context. After hooks record configured errors and end the span.

The instrumentation scope is `otelplan.io/business` with the supplied runtime
version. Generated hooks install no SDK or exporter. The application must
configure its normal OpenTelemetry provider/exporter for recording spans.

## Attribute capture

Argument/constant attributes bind at entry; result attributes bind at exit.
Extraction is guarded by `span.IsRecording()`. Typed package-local helpers avoid
importing application packages into the generated runtime and avoid triggering
otherwise unused package initializers. No reflection or object serialization is
used for capture.

Helpers return a primitive value and availability flag. Wrong types, nil paths,
unsigned overflow above `math.MaxInt64`, and non-finite floats are omitted. Zero,
false, and empty string remain valid values. Privacy/classification/cardinality
checks run before generation. Generation fails without partial output for
unsupported captures. Review explicitly configured constants because they remain
in policies, locks, and generated files.

## Generics and upstream limits

The pin replaces generic HookContext parameter/result APIs with panics.
OTelPlan uses direct hook parameters for generic root spans, errors, and concrete
captures; context replacement remains rejected. See
[upstream issue 1280](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/issues/1280).
Captures requiring an unbound target type parameter, such as `Request[T]`, cannot
be named by the generated helper and are rejected.

Some generic receiver after-trampoline calls cannot infer type arguments when
results do not carry them. The retained regression expects a failed build and
verifies no binary publication or source modification. The fix for
[upstream issue 1218](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/issues/1218)
is merged upstream but absent from v1.1.0. Upstream issue closure does not change
the capabilities of this pin.

The compiler identifies command packages with `-p main`, losing their canonical
import identity. The pinned file predicates do not establish command-specific
scope. OTelPlan rejects main-package targets before generation to prevent wider
selection than the resolved plan.

Variadic application-defined element types require a verified naming/scoping
strategy. Importing the application into generated hooks could introduce cycles
or initializer side effects, so OTelPlan rejects them. Deferred after hooks do
not expose application panic values; no panic observation is advertised.

## Compile/build integration

`RenderBundle` collects hooks, exact `*.otelc.yaml` rules, optional accessor helper
files/provider stub, pinned standalone runtime module/checksums, and
`manifest.json`. Payload files have SHA-256 hashes; the manifest does not hash
itself. Compile stages and compiles the generated module, verifies its artifacts,
and publishes the bundle. It reuses identical output and validates existing file
ownership before `--clean` replacement.

Build generates the bundle, copies modules/workspaces/local replacements, adds
the runtime, verifies analyzed application and runtime module selection, invokes
the verified backend, checks artifact integrity again, hashes outputs, and
publishes them. Temporary copies are removed. Neither command writes resolution
lock state. See [CLI contracts](CLI.md#lock-ownership) for independent ownership
of lock resolution and build metadata.

## Vendor builds

Vendor discovery honors explicit module mode and fingerprints vendor metadata.
Build uses `go work vendor` in the disposable combined workspace so the runtime
and its pinned dependencies are available alongside application dependencies.

Application vendor files must match the regenerated copies byte-for-byte.
Additional files in analyzed vendor directories, modified vendor code, or module
selection changes cause failure. There is no silent fallback to module mode and
no rewrite of the source vendor tree. Modules needed for vendoring must be
available in the module cache, including for `--offline`; an existing vendor tree
alone cannot supply the added runtime dependencies.

## Verification

From OTelPlan with the built backend:

```sh
OTELPLAN_OTELC=/absolute/path/to/otelc go test ./internal/backend/otelc
OTELPLAN_OTELC=/absolute/path/to/otelc go test ./internal/cli
OTELPLAN_OTELC=/absolute/path/to/otelc go test -race -timeout=20m ./...
```

Tests verify executable identity, generated rules/hooks/accessors, private and
imported types, exact targets, parentage, returned errors, nil omission, no-op
telemetry, panic propagation, variadics, generic root captures, concurrent calls,
module/vendor isolation, artifact checks, and source immutability. Architecture
E2E tests independently cover cold-cache online and prepared-cache offline paths.
See [testing](10_TEST_STRATEGY.md) and the
[fixture README](../internal/cli/testdata/architectures/README.md).
