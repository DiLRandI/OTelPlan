# OTelPlan

Policy-driven business tracing for Go, without editing application source.

OTelPlan analyzes Go packages, resolves a human-owned instrumentation policy to
exact functions and methods, validates context and capture safety, and builds
OpenTelemetry business spans through a pinned OTelC backend.

It adds application operations such as checkout, pricing, or authorization to the
traces your application already collects. Selection uses real symbols, types,
interfaces, and policy rules. It does not require Service/Repository naming or a
particular directory layout.

## How it works

```text
Go packages -> CodeModel ----+
                            v
otelplan.yaml -> resolver -> ResolvedPlan -> validation -> otelplan.lock
                                                 |
                                                 v
                                    pinned backend rules + hooks
                                                 |
                                                 v
                                      isolated instrumented build
```

Discovery and initialization provide suggestions. The reviewed policy determines
which operations become spans. Inspect and explain show the exact decisions.

## Quick start

Build with Go 1.27.4 or a later 1.27.x patch:

```sh
git clone https://github.com/DiLRandI/OTelPlan.git
cd OTelPlan
go build -o bin/otelplan ./cmd/otelplan
```

Put the resulting binary on `PATH`, then run these commands in your Go project:

```sh
otelplan scan ./...
otelplan init --non-interactive
# Review the generated otelplan.yaml before continuing.
otelplan inspect
otelplan validate --strict
otelplan lock
otelplan lock --check
```

A minimal policy selects an exact declaration discovered by `scan`:

```yaml
apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
defaults:
  context: {mode: require}
  errors: {record: true}
rules:
  - id: authorize
    match:
      symbols: ["example.com/shop/internal/payment.(*Processor).Authorize"]
```

Replace the example symbol with one from your project. This policy captures no
arguments or results. See the [policy reference](docs/POLICY.md) for
selectors, templates, and explicit attributes.

## Compile and build

Install the verified OTelC v1.1.0 executable on `PATH` using the
[backend instructions](docs/OTELC_BACKEND.md#build-the-pinned-backend). Then:

```sh
otelplan compile --output .otelplan/build
otelplan build --locked -- -o bin/api ./cmd/api
```

Compile publishes verified generated artifacts. Build runs in disposable copies
of module/workspace state and publishes verified binaries. Your application owns
its OpenTelemetry SDK, exporter, and Collector configuration; generated hooks do
not install them.

See [the CLI reference](docs/CLI.md) for every command, flag, JSON response, exit
code, offline requirements, and lock/build ownership contract.

## Safety and design

- Application Go source and module/workspace manifests remain unchanged.
- Policy stays independent of OTelC rule syntax.
- Context propagation and unsupported target shapes fail validation explicitly.
- Attribute capture requires explicit policy mappings and safety checks.
- Lock generation is deterministic; check/diff operations do not write project files.
- Analysis is local. Online Go operations may download pinned dependencies.

## Documentation

Start with the [documentation index](docs/README.md) for policy, CLI, architecture,
backend support, security, compatibility, and release references.

## Development

See [contributing](docs/DEVELOPMENT.md) for setup, review expectations, and reporting bugs.

```sh
make build
make check GOLANGCI_LINT=/path/to/golangci-lint
```

The required linter is v2.14.0. `make check` runs tests, race tests, vet, lint, and
quality-gate regressions. Missing tools and failing checks return nonzero.
Set `OTELPLAN_OTELC=/absolute/path/to/otelc` to run real backend integration tests;
otherwise those tests skip locally. See [testing instructions](docs/DEVELOPMENT.md).

## Status and compatibility

OTelPlan is implemented and pre-stable. Policy, lockfile, and CLI JSON contracts
use `v1alpha1`; release acceptance is still pending. Linux is the supported
platform and the target for CI and release verification.

Vendor builds are isolated and require prepared dependency caches for offline
use. Generic root spans and concrete typed attributes have documented limits.
Built-in variadic element types are supported; application-defined elements,
`package main` targets, generic context replacement, and panic-value observation
remain unsupported. See [the support matrix](docs/OTELC_BACKEND.md#support-matrix).

## License

[Apache License 2.0](LICENSE).
